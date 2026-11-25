package fbhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

// Run the test executable instead of platform-specific shell commands. The
// environment gate and exact -test.run selection keep normal tests inert.
func TestCommandProcessHelper(_ *testing.T) {
	if os.Getenv("FILEBROWSER_COMMAND_TEST_HELPER") != "1" {
		return
	}
	index := slices.Index(os.Args, "--")
	if index < 0 || len(os.Args) < index+3 {
		os.Exit(2)
	}
	mode, marker := os.Args[index+1], os.Args[index+2]
	switch mode {
	case "echo":
		fmt.Print(strings.Repeat("a", 4095) + "🌍\npartial")
		fmt.Fprint(os.Stderr, " stderr")
	case "exit":
		fmt.Print("failure output")
		os.Exit(7)
	case "hold", "closed-output":
		fmt.Print("ready")
		if mode == "closed-output" {
			_ = os.Stdout.Close()
			_ = os.Stderr.Close()
		}
		for i := 0; i < 2000; i++ {
			if _, err := os.Stat(marker); err == nil {
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(3)
	case "tree":
		child := exec.Command(os.Args[0], "-test.run=^TestCommandProcessHelper$", "--", "child", marker)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(4)
		}
		_ = child.Wait()
	case "child":
		fmt.Print("child-ready")
		time.Sleep(1500 * time.Millisecond)
		_ = os.WriteFile(marker, []byte("survived"), 0o600)
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func commandFixture(t *testing.T, enabled bool, perm users.Permissions) (http.Handler, string, string) {
	t.Helper()
	t.Setenv("FILEBROWSER_COMMAND_TEST_HELPER", "1")
	root := t.TempDir()
	key := []byte("command-test-key")
	st := scopedUserStorage(t, root, perm, key)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.Users.Get(root, false, uint(1))
	if err != nil {
		t.Fatal(err)
	}
	user.Commands = []string{filepath.ToSlash(executable)}
	if err := st.Users.Save(user); err != nil {
		t.Fatal(err)
	}
	server := &settings.Server{EnableExec: enabled}
	return requestLimits(handle(commandsHandler, "/api/command", st, server), server), signToken(t, st, perm, key), root
}

func helperCommand(t *testing.T, mode, marker string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return strconv.Quote(filepath.ToSlash(executable)) + " -test.run=^TestCommandProcessHelper$ -- " + mode + " " + strconv.Quote(filepath.ToSlash(marker))
}

func commandRequest(ctx context.Context, t *testing.T, target, token, body string) *http.Request {
	t.Helper()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, target+"/api/command/", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("X-Auth", token)
	r.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return r
}

func readCommandEvent(t *testing.T, decoder *json.Decoder) commandEvent {
	t.Helper()
	var event commandEvent
	if err := decoder.Decode(&event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestCommandPreflight(t *testing.T) {
	for _, entry := range []struct {
		name             string
		enabled, execute bool
		body             string
		auth             bool
		status           int
	}{
		{"disabled", false, true, "ignored", true, 403},
		{"no-permission", true, false, "ignored", true, 403},
		{"no-auth", true, true, "ignored", false, 401},
		{"empty", true, true, "  ", true, 400},
		{"oversized", true, true, strings.Repeat("x", maxCommandBytes+1), true, 413},
		{"disallowed", true, true, "not-allowed", true, 403},
		{"malformed", true, true, "\"unclosed", true, 400},
	} {
		t.Run(entry.name, func(t *testing.T) {
			// The existing Windows command parser deliberately accepts unclosed
			// quotes; this still cannot bypass the command allowlist.
			if entry.name == "malformed" && runtime.GOOS == "windows" {
				entry.status = 403
			}
			handler, token, _ := commandFixture(t, entry.enabled, users.Permissions{Execute: entry.execute})
			if !entry.auth {
				token = ""
			}
			r := commandRequest(context.Background(), t, "http://example.test", token, entry.body)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != entry.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, entry.status, w.Body.String())
			}
			if strings.Contains(w.Header().Get("Content-Type"), "ndjson") {
				t.Fatal("preflight failure started a stream")
			}
		})
	}
}

func TestCommandStreamOutputAndExit(t *testing.T) {
	for _, mode := range []string{"echo", "exit"} {
		t.Run(mode, func(t *testing.T) {
			handler, token, root := commandFixture(t, true, users.Permissions{Execute: true})
			server := httptest.NewServer(handler)
			defer server.Close()
			client := &http.Client{Timeout: 5 * time.Second}
			response, err := client.Do(commandRequest(context.Background(), t, server.URL, token, helperCommand(t, mode, filepath.Join(root, "marker"))))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("status=%d: %s", response.StatusCode, body)
			}
			decoder := json.NewDecoder(response.Body)
			if readCommandEvent(t, decoder).Type != "start" {
				t.Fatal("missing start")
			}
			var output strings.Builder
			for {
				event := readCommandEvent(t, decoder)
				if event.Type == "output" {
					output.WriteString(event.Data)
					continue
				}
				if event.Type == "heartbeat" {
					continue
				}
				wantCode, wantOutput := 0, strings.Repeat("a", 4095)+"🌍\npartial stderr"
				if mode == "exit" {
					wantCode, wantOutput = 7, "failure output"
				}
				if event.Type != "exit" || event.Code == nil || *event.Code != wantCode || output.String() != wantOutput {
					t.Fatalf("unexpected result: %s code=%v, output length=%d", event.Type, event.Code, output.Len())
				}
				break
			}
		})
	}
}

func TestCommandRejectsLegacyGetAndCookieOnlyPost(t *testing.T) {
	handler, token, _ := commandFixture(t, true, users.Permissions{Execute: true})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		r := commandRequest(context.Background(), t, "http://example.test", token, "not-allowed")
		r.Method = method
		want := http.StatusMethodNotAllowed
		if method == http.MethodPost {
			r.Header.Del("X-Auth")
			r.AddCookie(&http.Cookie{Name: accessCookie, Value: token})
			want = http.StatusUnauthorized
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: status=%d, want=%d", method, w.Code, want)
		}
	}
}

type failedCommandFlush struct{ *httptest.ResponseRecorder }

func (*failedCommandFlush) FlushError() error { return io.ErrClosedPipe }

func TestCommandFlushErrorPassesThroughLimits(t *testing.T) {
	var flushErr error
	handler := requestLimits(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flushErr = http.NewResponseController(w).Flush()
	}), &settings.Server{})
	handler.ServeHTTP(&failedCommandFlush{httptest.NewRecorder()}, httptest.NewRequest(http.MethodPost, "/api/command/", http.NoBody))
	if flushErr != io.ErrClosedPipe {
		t.Fatalf("stream flush failure was hidden by middleware: %v", flushErr)
	}
}

func TestCommandWriteFailureCancelsAndReaps(t *testing.T) {
	t.Setenv("FILEBROWSER_COMMAND_TEST_HELPER", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandProcessHelper$", "--", "hold", filepath.Join(t.TempDir(), "never"))
	w := &failedCommandFlush{httptest.NewRecorder()}
	r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	_, err := streamCommand(ctx, w, r, cancel, cmd, commandHeartbeat)
	if err != io.ErrClosedPipe || cmd.ProcessState == nil {
		t.Fatalf("write failure did not reap the process: %v, %v", err, cmd.ProcessState)
	}
	if ctx.Err() == nil {
		t.Fatal("write failure did not cancel the command")
	}
}

func TestCommandStreamFlushesThroughReverseProxy(t *testing.T) {
	handler, token, root := commandFixture(t, true, users.Permissions{Execute: true})
	origin := httptest.NewServer(handler)
	defer origin.Close()
	target, _ := url.Parse(origin.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	server := httptest.NewServer(proxy)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	marker := filepath.Join(root, "release")
	response, err := http.DefaultClient.Do(commandRequest(ctx, t, server.URL, token, helperCommand(t, "hold", marker)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Accel-Buffering") != "no" || response.Header.Get("Cache-Control") != "no-store, no-transform" || response.Header.Get("Content-Encoding") != "" {
		t.Fatalf("unsafe streaming headers: %v", response.Header)
	}
	decoder := json.NewDecoder(response.Body)
	var output string
	for !strings.Contains(output, "ready") {
		event := readCommandEvent(t, decoder)
		if event.Type == "exit" {
			t.Fatal("output was buffered until exit")
		}
		output += event.Data
	}
	// Only allow completion after output has crossed the proxy. A buffered
	// response would deadlock here until the test's request deadline.
	if err := os.WriteFile(marker, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		event := readCommandEvent(t, decoder)
		if event.Type == "exit" {
			if event.Code == nil || *event.Code != 0 {
				t.Fatal("command failed")
			}
			break
		}
	}
}

func TestCommandCancellationKillsProcessTree(t *testing.T) {
	for _, throughProxy := range []bool{false, true} {
		t.Run(fmt.Sprintf("proxy=%v", throughProxy), func(t *testing.T) {
			handler, token, root := commandFixture(t, true, users.Permissions{Execute: true})
			done := make(chan struct{})
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r); close(done) }))
			defer origin.Close()
			targetURL := origin.URL
			if throughProxy {
				target, _ := url.Parse(targetURL)
				proxy := httputil.NewSingleHostReverseProxy(target)
				proxy.FlushInterval = -1
				server := httptest.NewServer(proxy)
				defer server.Close()
				targetURL = server.URL
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			marker := filepath.Join(root, "escaped-child")
			response, err := http.DefaultClient.Do(commandRequest(ctx, t, targetURL, token, helperCommand(t, "tree", marker)))
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(response.Body)
			var output string
			for !strings.Contains(output, "child-ready") {
				output += readCommandEvent(t, decoder).Data
			}
			cancel()
			_ = response.Body.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("cancelled command did not release the handler")
			}
			time.Sleep(1700 * time.Millisecond)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("descendant survived cancellation: %v", err)
			}
		})
	}
}

func TestCommandHeartbeatsAfterOutputClosesAndTimeout(t *testing.T) {
	t.Setenv("FILEBROWSER_COMMAND_TEST_HELPER", "1")
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 350*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandProcessHelper$", "--", "closed-output", filepath.Join(root, "never"))
		_, _ = streamCommand(ctx, w, r, cancel, cmd, 40*time.Millisecond)
	}))
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Post(server.URL, "text/plain", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(response.Body)
	heartbeats := 0
	for {
		event := readCommandEvent(t, decoder)
		if event.Type == "heartbeat" {
			heartbeats++
		}
		if event.Type == "error" {
			if event.Data != "Command timed out." || heartbeats < 2 {
				t.Fatalf("timeout=%q, heartbeats=%d", event.Data, heartbeats)
			}
			break
		}
	}
}
