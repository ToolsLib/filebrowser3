package files

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/afero"
)

// A local actor swaps a directory link while requests resolve and open files.
// Failures caused by a disappearing link are acceptable; escaping the scope is not.
func TestScopedConcurrentSymlinkSwap(t *testing.T) {
	parent := t.TempDir()
	scope, outside := filepath.Join(parent, "scope"), filepath.Join(parent, "outside")
	inside := filepath.Join(scope, "inside")
	for _, dir := range []string{inside, outside} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []struct{ dir, text string }{{inside, "inside"}, {outside, "OUTSIDE"}} {
		if err := os.WriteFile(filepath.Join(entry.dir, "file"), []byte(entry.text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(scope, "link")
	if err := os.Symlink(inside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(link)
			_ = os.Symlink(outside, link)
			_ = os.Remove(link)
			_ = os.Symlink(inside, link)
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	fs := NewScopedFs(afero.NewOsFs(), scope)
	for i := 0; i < 1000; i++ {
		body, err := afero.ReadFile(fs, "/link/file")
		if err == nil && string(body) == "OUTSIDE" {
			t.Fatal("read outside the scope during link swap")
		}
		file, err := fs.OpenFile("/link/file", os.O_WRONLY|os.O_TRUNC, 0600)
		if err == nil {
			_, _ = file.WriteString("inside")
			_ = file.Close()
		}
	}
	body, err := os.ReadFile(filepath.Join(outside, "file"))
	if err != nil || string(body) != "OUTSIDE" {
		t.Fatalf("outside file changed: %q, %v", body, err)
	}
}

func TestScopedPreservesVirtualAndReadOnlySources(t *testing.T) {
	mem := afero.NewMemMapFs()
	_ = mem.MkdirAll("/virtual", 0700)
	_ = afero.WriteFile(mem, "/virtual/file", []byte("virtual"), 0600)
	body, err := afero.ReadFile(NewScopedFs(mem, "/virtual"), "/file")
	if err != nil || string(body) != "virtual" {
		t.Fatalf("virtual read: %q, %v", body, err)
	}
	// An opaque adapter cannot be bypassed by silently opening the host OS.
	readonly := NewScopedFs(afero.NewReadOnlyFs(mem), "/virtual")
	if _, err := readonly.Create("/file"); err == nil {
		t.Fatal("unsupported read-only adapter was bypassed")
	}
}
