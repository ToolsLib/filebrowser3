package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/filebrowser/filebrowser/v2/runner"
)

const (
	maxCommandBytes   = 16 << 10
	commandRunTimeout = 10 * time.Minute
	commandHeartbeat  = 15 * time.Second
)

type commandEvent struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Code *int   `json:"code,omitempty"`
}

var commandsHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if !d.server.EnableExec || !d.user.Perm.Execute {
		return http.StatusForbidden, nil
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		return http.StatusMethodNotAllowed, nil
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCommandBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return http.StatusBadRequest, errors.New("command is empty")
	}
	command, name, err := runner.ParseCommand(d.settings, raw)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if !slices.Contains(d.user.Commands, name) {
		return http.StatusForbidden, nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), commandRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = d.user.FullPath(r.URL.Path)
	return streamCommand(ctx, w, r, cancel, cmd, commandHeartbeat)
})

// streamCommand keeps stdout/stderr behind a bounded OS pipe. Waiting for the
// parent also cleans up its process group/job so descendants cannot keep the
// pipe (and the request's work slot) open after the command has finished.
func streamCommand(ctx context.Context, w http.ResponseWriter, r *http.Request, cancel context.CancelFunc, cmd *exec.Cmd, heartbeat time.Duration) (int, error) {
	output, writer, err := os.Pipe()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer output.Close()
	cmd.Stdout, cmd.Stderr = writer, writer
	stop, err := startCommand(cmd)
	_ = writer.Close()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		stop()
		close(done)
	}()
	defer func() {
		cancel()
		_ = output.Close()
		<-done
	}()

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	encoder := json.NewEncoder(w)
	send := func(event commandEvent) error {
		// The shared idleWriter uses the same bounded write deadline.
		_ = controller.SetWriteDeadline(time.Now().Add(streamIdleTimeout))
		if err := encoder.Encode(event); err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := send(commandEvent{Type: "start"}); err != nil {
		return 0, err
	}
	type readResult struct {
		data string
		err  error
	}
	chunks := make(chan readResult)
	go func() {
		buffer := make([]byte, 4096+utf8.UTFMax)
		pending := 0
		for {
			n, err := output.Read(buffer[pending : pending+4096])
			n += pending
			end := n
			if err == nil {
				for start := n - 1; start >= 0 && n-start < utf8.UTFMax; start-- {
					if utf8.RuneStart(buffer[start]) {
						if !utf8.FullRune(buffer[start:n]) {
							end = start
						}
						break
					}
				}
			}
			select {
			case chunks <- readResult{data: string(buffer[:end]), err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
			pending = copy(buffer, buffer[end:n])
		}
	}()
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	waitForExit := done
	outputDone := false
	for {
		if outputDone && waitForExit == nil {
			if ctx.Err() != nil {
				if r.Context().Err() == nil {
					_ = send(commandEvent{Type: "error", Data: "Command timed out."})
				}
				return 0, nil
			}
			code := 0
			if waitErr != nil {
				var exitErr *exec.ExitError
				if !errors.As(waitErr, &exitErr) {
					_ = send(commandEvent{Type: "error", Data: "Unable to wait for command."})
					return 0, waitErr
				}
				code = exitErr.ExitCode()
			}
			return 0, send(commandEvent{Type: "exit", Code: &code})
		}
		select {
		case <-waitForExit:
			waitForExit = nil
		case <-ctx.Done():
			if r.Context().Err() == nil {
				_ = send(commandEvent{Type: "error", Data: "Command timed out."})
			}
			return 0, nil
		case <-ticker.C:
			if err := send(commandEvent{Type: "heartbeat"}); err != nil {
				return 0, err
			}
		case chunk := <-chunks:
			if chunk.data != "" {
				if err := send(commandEvent{Type: "output", Data: chunk.data}); err != nil {
					return 0, err
				}
			}
			if chunk.err == nil {
				continue
			}
			if !errors.Is(chunk.err, io.EOF) {
				_ = send(commandEvent{Type: "error", Data: "Unable to read command output."})
				return 0, chunk.err
			}
			chunks = nil
			outputDone = true
		}
	}
}
