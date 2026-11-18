package fbhttp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const maxPatchDrainBytes = 32 << 20
const maxUploadLength = 1 << 50 // Bound arithmetic; allow uploads up to 1 PiB.

func drainRequestBody(r *http.Request) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, maxPatchDrainBytes))
	}
}

func keepUploadActive(ctx context.Context, cache UploadCache, key, id string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := cache.Touch(key, id); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, func() { cancel(); <-done }
}

func uploadCacheStatus(err error) int {
	if errors.Is(err, ErrUploadNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrUploadExists) {
		return http.StatusConflict
	}
	return http.StatusServiceUnavailable
}

func uploadKey(d *data, name string) string { return physicalPathKey(d.user.FullPath(name)) }

func loadUpload(r *http.Request, d *data, cache UploadCache, key string) (UploadState, error) {
	state, err := cache.Get(key)
	if err != nil {
		return state, err
	}
	if state.UserID != d.user.ID || state.ID == "" || state.ID != r.URL.Query().Get("uploadID") {
		return UploadState{}, ErrUploadNotFound
	}
	if !strings.HasPrefix(path.Base(state.Stage), internalWritePrefix+state.ID+"-") || path.Dir(state.Stage) != path.Dir(slashClean(r.URL.Path)) {
		return UploadState{}, fmt.Errorf("invalid upload staging state")
	}
	if state.Length < 0 || state.Length > maxUploadLength {
		return UploadState{}, fmt.Errorf("invalid cached upload length")
	}
	return state, nil
}

func removeUploadStage(fs afero.Fs, state UploadState) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlock, err := lockUploadStage(ctx, fs, state.Stage)
	if err != nil {
		return err
	}
	// Close the lock handle before removing it on Windows.
	defer func() { unlock(); _ = fs.Remove(state.Stage + ".lock") }()
	defer func() { _ = fs.Remove(state.Stage + ".json") }()
	err = fs.Remove(state.Stage)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func commitUpload(d *data, name, key string, state UploadState, cache UploadCache) (int, error) {
	if err := cache.Touch(key, state.ID); err != nil {
		return uploadCacheStatus(err), err
	}
	current, err := d.user.Fs.Stat(name)
	switch {
	case state.Existed:
		if !d.user.Perm.Modify {
			return http.StatusForbidden, nil
		}
		if err != nil || current.IsDir() || current.Size() != state.OriginalSize || current.ModTime().UnixNano() != state.OriginalModified {
			return http.StatusConflict, fmt.Errorf("destination changed during upload")
		}
	case err == nil:
		return http.StatusConflict, fmt.Errorf("destination was created during upload")
	case !os.IsNotExist(err):
		return errToStatus(err), err
	}
	if err := d.user.Fs.Chmod(state.Stage, os.FileMode(state.Mode)); err != nil {
		return errToStatus(err), err
	}
	if err := d.user.Fs.Rename(state.Stage, name); err != nil {
		return errToStatus(err), err
	}
	_ = d.user.Fs.Remove(state.Stage + ".json")
	// Publication already succeeded. Cache deletion failure cannot undo a
	// committed rename; keep its TTL cleanup restricted to private stage names.
	if err := cache.Complete(key, state.ID); err != nil {
		log.Printf("uploaded file committed; cache cleanup failed: %v", err)
	}
	if err := d.RunHook(func() error { return nil }, "upload", name, "", d.user); err != nil {
		log.Printf("uploaded file committed; after-upload hook failed: %v", err)
	}
	return 0, nil
}

func tusPostHandler(cache UploadCache) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if !d.user.Perm.Create || !d.Check(r.URL.Path) || r.URL.Path == "/" {
			return http.StatusForbidden, nil
		}
		length, err := getUploadLength(r)
		if err != nil || length < 0 || length > maxUploadLength {
			return http.StatusBadRequest, fmt.Errorf("invalid upload length")
		}
		if limit := effectiveUploadLimit(d); length > limit {
			return http.StatusRequestEntityTooLarge, &uploadLimitError{limit}
		}
		key := uploadKey(d, r.URL.Path)
		unlock, err := lockFileMutation(r.Context(), key)
		if err != nil {
			return http.StatusRequestTimeout, err
		}
		defer unlock()
		state := UploadState{ID: rand.Text(), UserID: d.user.ID, Length: length, Mode: uint32(d.settings.FileMode)}
		previous, err := d.user.Fs.Stat(r.URL.Path)
		if err == nil {
			if previous.IsDir() {
				return http.StatusBadRequest, fmt.Errorf("cannot upload to a directory")
			}
			if r.URL.Query().Get("override") != "true" {
				return http.StatusConflict, nil
			}
			if !d.user.Perm.Modify {
				return http.StatusForbidden, nil
			}
			state.Existed, state.OriginalSize, state.OriginalModified = true, previous.Size(), previous.ModTime().UnixNano()
			state.Mode = uint32(previous.Mode().Perm())
		} else if !os.IsNotExist(err) {
			return errToStatus(err), err
		}
		if err := d.user.Fs.MkdirAll(path.Dir(r.URL.Path), d.settings.DirMode); err != nil {
			return errToStatus(err), err
		}
		stage, err := afero.TempFile(d.user.Fs, path.Dir(r.URL.Path), internalWritePrefix+state.ID+"-")
		if err != nil {
			return errToStatus(err), err
		}
		state.Stage = slashClean(stage.Name())
		if err := stage.Close(); err != nil {
			_ = d.user.Fs.Remove(state.Stage)
			return http.StatusInternalServerError, err
		}
		journal, err := json.Marshal(uploadJournal{Key: key, State: state})
		if err != nil {
			_ = d.user.Fs.Remove(state.Stage)
			return http.StatusInternalServerError, err
		}
		if err := afero.WriteFile(d.user.Fs, state.Stage+".json", journal, 0600); err != nil {
			_ = d.user.Fs.Remove(state.Stage)
			_ = d.user.Fs.Remove(state.Stage + ".json")
			return errToStatus(err), err
		}
		if err := cache.Register(key, state, func() error { return removeUploadStage(d.user.Fs, state) }); err != nil {
			_ = d.user.Fs.Remove(state.Stage)
			_ = d.user.Fs.Remove(state.Stage + ".json")
			return uploadCacheStatus(err), err
		}
		if length == 0 {
			if status, err := commitUpload(d, r.URL.Path, key, state, cache); status != 0 || err != nil {
				return status, err
			}
		}
		base := strings.TrimSuffix(d.server.BaseURL, "/")
		w.Header().Set("Location", base+"/api/tus"+r.URL.EscapedPath()+"?uploadID="+url.QueryEscape(state.ID))
		return http.StatusCreated, nil
	})
}

func tusHeadHandler(cache UploadCache) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		w.Header().Set("Cache-Control", "no-store")
		if !d.user.Perm.Create || !d.Check(r.URL.Path) {
			return http.StatusForbidden, nil
		}
		key := uploadKey(d, r.URL.Path)
		state, err := loadUpload(r, d, cache, key)
		if err != nil {
			return uploadCacheStatus(err), err
		}
		unlock, err := lockUploadStage(r.Context(), d.user.Fs, state.Stage)
		if err != nil {
			return http.StatusRequestTimeout, err
		}
		defer unlock()
		if _, err := loadUpload(r, d, cache, key); err != nil {
			return uploadCacheStatus(err), err
		}
		info, err := d.user.Fs.Stat(state.Stage)
		if err != nil {
			return errToStatus(err), err
		}
		if err := cache.Touch(key, state.ID); err != nil {
			return uploadCacheStatus(err), err
		}
		w.Header().Set("Upload-Offset", strconv.FormatInt(info.Size(), 10))
		w.Header().Set("Upload-Length", strconv.FormatInt(state.Length, 10))
		return http.StatusOK, nil
	})
}

func tusPatchHandler(cache UploadCache) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		status, err := tusPatchUpload(w, r, d, cache)
		if status >= 400 {
			drainRequestBody(r)
		}
		return status, err
	})
}

type uploadContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r uploadContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func tusPatchUpload(w http.ResponseWriter, r *http.Request, d *data, cache UploadCache) (int, error) {
	if !d.user.Perm.Create || !d.Check(r.URL.Path) {
		return http.StatusForbidden, nil
	}
	if _, err := d.user.Fs.Stat(r.URL.Path); err != nil && !os.IsNotExist(err) {
		return errToStatus(err), err
	}
	if r.Header.Get("Content-Type") != "application/offset+octet-stream" {
		return http.StatusUnsupportedMediaType, nil
	}
	offset, err := getUploadOffset(r)
	if err != nil || offset < 0 {
		return http.StatusBadRequest, fmt.Errorf("invalid upload offset")
	}
	key := uploadKey(d, r.URL.Path)
	localUnlock, err := lockFileMutation(r.Context(), key)
	if err != nil {
		return http.StatusRequestTimeout, err
	}
	defer localUnlock()
	state, err := loadUpload(r, d, cache, key)
	if err != nil {
		return uploadCacheStatus(err), err
	}
	if limit := effectiveUploadLimit(d); state.Length > limit {
		return http.StatusRequestEntityTooLarge, &uploadLimitError{limit}
	}
	unlock, err := lockUploadStage(r.Context(), d.user.Fs, state.Stage)
	if err != nil {
		return http.StatusRequestTimeout, err
	}
	defer func() {
		unlock()
		if _, err := d.user.Fs.Stat(state.Stage); os.IsNotExist(err) {
			_ = d.user.Fs.Remove(state.Stage + ".lock")
		}
	}()
	// Revalidate after the shared OS lock: another replica may have completed
	// or canceled the generation while this request waited.
	if _, err := loadUpload(r, d, cache, key); err != nil {
		return uploadCacheStatus(err), err
	}
	if err := cache.Touch(key, state.ID); err != nil {
		return uploadCacheStatus(err), err
	}
	ctx, stop := keepUploadActive(r.Context(), cache, key, state.ID)
	defer stop()
	file, err := d.user.Fs.OpenFile(state.Stage, os.O_RDWR, 0600)
	if err != nil {
		return errToStatus(err), err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if offset > state.Length || info.Size() != offset {
		return http.StatusConflict, fmt.Errorf("upload offset does not match staged size")
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return http.StatusInternalServerError, err
	}
	remaining := state.Length - offset
	if r.ContentLength > remaining {
		return http.StatusRequestEntityTooLarge, fmt.Errorf("upload exceeds declared length")
	}
	written, copyErr := io.Copy(file, io.LimitReader(uploadContextReader{ctx, r.Body}, remaining+1))
	if copyErr != nil || written > remaining {
		if err := file.Truncate(offset); err != nil {
			return http.StatusInternalServerError, fmt.Errorf("rollback upload chunk: %w", err)
		}
		if copyErr != nil {
			return http.StatusBadRequest, copyErr
		}
		return http.StatusRequestEntityTooLarge, fmt.Errorf("upload exceeds declared length")
	}
	if err := file.Sync(); err != nil {
		_ = file.Truncate(offset)
		return http.StatusInternalServerError, err
	}
	if err := file.Close(); err != nil {
		return http.StatusInternalServerError, err
	}
	if ctx.Err() != nil {
		return http.StatusServiceUnavailable, ctx.Err()
	}
	if err := cache.Touch(key, state.ID); err != nil {
		return uploadCacheStatus(err), err
	}
	if offset+written == state.Length {
		if status, err := commitUpload(d, r.URL.Path, key, state, cache); status != 0 || err != nil {
			return status, err
		}
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(offset+written, 10))
	return http.StatusNoContent, nil
}

func tusDeleteHandler(cache UploadCache) handleFunc {
	return withUser(func(_ http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if !d.user.Perm.Create || !d.Check(r.URL.Path) || r.URL.Path == "/" {
			return http.StatusForbidden, nil
		}
		key := uploadKey(d, r.URL.Path)
		localUnlock, err := lockFileMutation(r.Context(), key)
		if err != nil {
			return http.StatusRequestTimeout, err
		}
		defer localUnlock()
		state, err := loadUpload(r, d, cache, key)
		if err != nil {
			return uploadCacheStatus(err), err
		}
		unlock, err := lockUploadStage(r.Context(), d.user.Fs, state.Stage)
		if err != nil {
			return http.StatusRequestTimeout, err
		}
		defer func() { unlock(); _ = d.user.Fs.Remove(state.Stage + ".lock") }()
		if _, err := loadUpload(r, d, cache, key); err != nil {
			return uploadCacheStatus(err), err
		}
		if err := cache.Complete(key, state.ID); err != nil {
			return uploadCacheStatus(err), err
		}
		if err := d.user.Fs.Remove(state.Stage); err != nil && !os.IsNotExist(err) {
			return errToStatus(err), err
		}
		_ = d.user.Fs.Remove(state.Stage + ".json")
		return http.StatusNoContent, nil
	})
}

func getUploadLength(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
}
func getUploadOffset(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
}
