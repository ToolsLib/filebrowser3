package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/spf13/afero"
)

type uploadJournal struct {
	Key   string      `json:"key"`
	State UploadState `json:"state"`
}

// CleanupUploads finds orphaned generations after a restart. It only considers
// recognized journals and their sibling staging names, never destination paths.
// A Redis outage fails closed; live shared state and held OS locks are skipped.
func CleanupUploads(root string, cache UploadCache, before time.Time, apply bool) ([]string, error) {
	fs := files.NewScopedFs(afero.NewOsFs(), root)
	var found []string
	err := afero.Walk(fs, "/", func(name string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !info.Mode().IsRegular() || !strings.HasPrefix(info.Name(), internalWritePrefix) || !strings.HasSuffix(info.Name(), ".json") || !info.ModTime().Before(before) {
			return nil
		}
		file, err := fs.Open(name)
		if err != nil {
			return err
		}
		var journal uploadJournal
		decodeErr := json.NewDecoder(io.LimitReader(file, 64<<10)).Decode(&journal)
		_ = file.Close()
		stage := strings.TrimSuffix(name, ".json")
		if decodeErr != nil || journal.Key == "" || journal.State.ID == "" || !strings.HasPrefix(filepath.Base(stage), internalWritePrefix+journal.State.ID+"-") {
			return nil
		}
		if cache != nil {
			state, err := cache.Get(journal.Key)
			if err != nil && !errors.Is(err, ErrUploadNotFound) {
				return err
			}
			if err == nil && state.ID == journal.State.ID {
				return nil
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		_, lockErr := fs.Stat(stage + ".lock")
		lockWasAbsent := os.IsNotExist(lockErr)
		unlock, err := lockUploadStage(ctx, fs, stage)
		cancel()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		if err != nil {
			return err
		}
		defer func() {
			unlock()
			if apply || lockWasAbsent {
				_ = fs.Remove(stage + ".lock")
			}
		}()
		found = append(found, stage)
		if !apply {
			return nil
		}
		if err := fs.Remove(stage); err != nil && !os.IsNotExist(err) {
			return err
		}
		return fs.Remove(name)
	})
	return found, err
}
