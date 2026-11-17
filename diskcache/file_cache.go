package diskcache

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
)

const DefaultMaxBytes int64 = 512 << 20
const maxEntryBytes int64 = 16 << 20
const maxEntries = 8192
const maxAge = 7 * 24 * time.Hour

type entry struct {
	size    int64
	touched time.Time
}
type FileCache struct {
	fs       afero.Fs
	once     sync.Once
	mu       sync.Mutex
	entries  map[string]entry
	bytes    int64
	maxBytes int64
}

func New(fs afero.Fs, root string) *FileCache { return NewWithLimit(fs, root, DefaultMaxBytes) }
func NewWithLimit(fs afero.Fs, root string, maxBytes int64) *FileCache {
	return &FileCache{fs: afero.NewBasePathFs(fs, root), entries: make(map[string]entry), maxBytes: maxBytes}
}
func (f *FileCache) initialize() {
	f.once.Do(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = afero.Walk(f.fs, "/", func(name string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			base := filepath.Base(name)
			if strings.HasPrefix(base, ".preview-") {
				_ = f.fs.Remove(name)
				return nil
			}
			if len(base) != 40 {
				return nil
			}
			if _, err := hex.DecodeString(base); err != nil {
				return nil
			}
			if time.Since(info.ModTime()) > maxAge {
				_ = f.fs.Remove(name)
				return nil
			}
			name = strings.TrimLeft(filepath.Clean(name), `/\`)
			f.entries[name] = entry{size: info.Size(), touched: info.ModTime()}
			f.bytes += info.Size()
			f.pruneLocked()
			return nil
		})
	})
}
func (f *FileCache) pruneLocked() {
	for f.bytes > f.maxBytes || len(f.entries) > maxEntries {
		var oldest string
		var stamp time.Time
		for name, e := range f.entries {
			if oldest == "" || e.touched.Before(stamp) {
				oldest = name
				stamp = e.touched
			}
		}
		if oldest == "" {
			return
		}
		if err := f.fs.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return
		}
		f.bytes -= f.entries[oldest].size
		delete(f.entries, oldest)
	}
}
func (f *FileCache) Store(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if int64(len(value)) > min(maxEntryBytes, f.maxBytes) {
		return nil
	}
	f.initialize()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	name := f.getFileName(key)
	if err := f.fs.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	tmp, err := afero.TempFile(f.fs, filepath.Dir(name), ".preview-")
	if err != nil {
		return err
	}
	tempName := tmp.Name()
	defer func() { _ = f.fs.Remove(tempName) }()
	if _, err = tmp.Write(value); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = f.fs.Rename(tempName, name); err != nil {
		return err
	}
	if old, ok := f.entries[name]; ok {
		f.bytes -= old.size
	}
	f.entries[name] = entry{size: int64(len(value)), touched: time.Now()}
	f.bytes += int64(len(value))
	f.pruneLocked()
	return nil
}
func (f *FileCache) Open(ctx context.Context, key string) (afero.File, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	f.initialize()
	name := f.getFileName(key)
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := f.fs.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, false, err
	}
	if info.Size() > maxEntryBytes || time.Since(info.ModTime()) > maxAge {
		_ = file.Close()
		_ = f.fs.Remove(name)
		if old, ok := f.entries[name]; ok {
			f.bytes -= old.size
			delete(f.entries, name)
		}
		return nil, false, nil
	}
	if old, ok := f.entries[name]; ok {
		old.touched = time.Now()
		f.entries[name] = old
	}
	return file, true, nil
}
func (f *FileCache) Load(ctx context.Context, key string) ([]byte, bool, error) {
	file, ok, err := f.Open(ctx, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, maxEntryBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(value)) > maxEntryBytes {
		return nil, false, nil
	}
	return value, true, nil
}
func (f *FileCache) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.initialize()
	f.mu.Lock()
	defer f.mu.Unlock()
	name := f.getFileName(key)
	if err := f.fs.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if old, ok := f.entries[name]; ok {
		f.bytes -= old.size
		delete(f.entries, name)
	}
	return nil
}
func (f *FileCache) getFileName(key string) string {
	hash := sha1.Sum([]byte(key))
	name := hex.EncodeToString(hash[:])
	return filepath.Join(name[:1], name[1:3], name)
}
