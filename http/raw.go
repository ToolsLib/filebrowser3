package fbhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	gopath "path"
	"path/filepath"
	"strings"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/fileutils"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/mholt/archives"
)

func parseQueryFiles(r *http.Request, f *files.FileInfo, _ *users.User) ([]string, error) {
	var fileSlice []string
	names := strings.Split(r.URL.Query().Get("files"), ",")

	if len(names) == 0 {
		fileSlice = append(fileSlice, f.Path)
	} else {
		for _, name := range names {
			name, err := url.QueryUnescape(strings.ReplaceAll(name, "+", "%2B"))
			if err != nil {
				return nil, err
			}

			name = slashClean(name)
			fileSlice = append(fileSlice, filepath.Join(f.Path, name))
		}
	}

	return fileSlice, nil
}

func parseQueryAlgorithm(r *http.Request) (string, archives.Archival, error) {
	switch r.URL.Query().Get("algo") {
	case "zip", "true", "":
		return ".zip", archives.Zip{}, nil
	case "tar":
		return ".tar", archives.Tar{}, nil
	case "targz":
		return ".tar.gz", archives.CompressedArchive{Compression: archives.Gz{}, Archival: archives.Tar{}}, nil
	case "tarbz2":
		return ".tar.bz2", archives.CompressedArchive{Compression: archives.Bz2{}, Archival: archives.Tar{}}, nil
	case "tarxz":
		return ".tar.xz", archives.CompressedArchive{Compression: archives.Xz{}, Archival: archives.Tar{}}, nil
	case "tarlz4":
		return ".tar.lz4", archives.CompressedArchive{Compression: archives.Lz4{}, Archival: archives.Tar{}}, nil
	case "tarsz":
		return ".tar.sz", archives.CompressedArchive{Compression: archives.Sz{}, Archival: archives.Tar{}}, nil
	case "tarbr":
		return ".tar.br", archives.CompressedArchive{Compression: archives.Brotli{}, Archival: archives.Tar{}}, nil
	case "tarzst":
		return ".tar.zst", archives.CompressedArchive{Compression: archives.Zstd{}, Archival: archives.Tar{}}, nil
	default:
		return "", nil, errors.New("format not implemented")
	}
}

func setContentDisposition(w http.ResponseWriter, r *http.Request, file *files.FileInfo) {
	if r.URL.Query().Get("inline") == "true" {
		// As per RFC6266 section 4.3
		w.Header().Set("Content-Disposition", "inline; filename*=utf-8''"+url.PathEscape(file.Name))
	} else {
		// As per RFC6266 section 4.3
		w.Header().Set("Content-Disposition", "attachment; filename*=utf-8''"+url.PathEscape(file.Name))
		w.Header().Set("Content-Type", "application/octet-stream")
	}
}

var rawHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if !d.user.Perm.Download {
		return http.StatusAccepted, nil
	}

	file, err := files.NewFileInfo(&files.FileOptions{
		Fs:         d.user.Fs,
		Path:       r.URL.Path,
		Modify:     d.user.Perm.Modify,
		Expand:     false,
		ReadHeader: d.server.TypeDetectionByHeader,
		Checker:    d,
	})
	if err != nil {
		return errToStatus(err), err
	}

	if files.IsNamedPipe(file.Mode) {
		setContentDisposition(w, r, file)
		return 0, nil
	}

	if !file.IsDir {
		return rawFileHandler(w, r, file)
	}

	return rawDirHandler(w, r, d, file)
})

var errArchiveLimit = errors.New("archive exceeds 100000 entries; download smaller folders separately")

const maxArchiveEntries = 100000

func archiveManifest(ctx context.Context, d *data, targets []string, commonPath string) ([]archives.FileInfo, error) {
	result := make([]archives.FileInfo, 0)
	seen := make(map[string]bool)
	names := make(map[string]bool)
	discovered := 0
	var walk func(string) error
	walk = func(target string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := filepath.Clean(target)
		if d.server.CaseInsensitiveFs {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return nil
		}
		if len(seen) >= maxArchiveEntries {
			return errArchiveLimit
		}
		seen[key] = true
		if !d.Check(target) {
			return nil
		}
		info, err := d.user.Fs.Stat(target)
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		if target != commonPath {
			name := strings.TrimPrefix(target, commonPath)
			name = strings.TrimPrefix(name, string(filepath.Separator))
			name = filepath.ToSlash(name)
			name = strings.ReplaceAll(name, "\\", "_")
			if gopath.Clean("/"+name) != "/"+name {
				return fmt.Errorf("refusing unsafe archive entry name: %q", name)
			}
			if names[name] {
				return fmt.Errorf("duplicate archive entry: %q", name)
			}
			names[name] = true
			result = append(result, archives.FileInfo{FileInfo: info, NameInArchive: name, Open: func() (fs.File, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return d.user.Fs.Open(target)
			}})
		}
		if !info.IsDir() {
			return nil
		}
		directory, err := d.user.Fs.Open(target)
		if err != nil {
			return err
		}
		children := make([]string, 0)
		for {
			if err = ctx.Err(); err != nil {
				break
			}
			var batch []string
			batch, err = directory.Readdirnames(256)
			children = append(children, batch...)
			discovered += len(batch)
			if discovered > maxArchiveEntries {
				err = errArchiveLimit
				break
			}
			if err != nil {
				break
			}
		}
		closeErr := directory.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		for _, name := range children {
			if err := walk(filepath.Join(target, name)); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errArchiveLimit) {
					return err
				}
				log.Printf("Failed to get archive file: %v", err)
			}
		}
		return nil
	}
	for _, target := range targets {
		if err := walk(target); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func rawDirHandler(w http.ResponseWriter, r *http.Request, d *data, file *files.FileInfo) (int, error) {
	filenames, err := parseQueryFiles(r, file, d.user)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	extension, archiver, err := parseQueryAlgorithm(r)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	commonDir := fileutils.CommonPrefix(filepath.Separator, filenames...)

	allFiles, err := archiveManifest(r.Context(), d, filenames, commonDir)
	if err != nil {
		if errors.Is(err, errArchiveLimit) {
			return http.StatusRequestEntityTooLarge, err
		}
		return errToStatus(err), err
	}
	name := filepath.Base(commonDir)
	if name == "." || name == "" || name == string(filepath.Separator) {
		if file.Name != "" {
			name = file.Name
		} else {
			actual, statErr := file.Fs.Stat(".")
			if statErr != nil {
				return http.StatusInternalServerError, statErr
			}
			name = actual.Name()
		}
	}
	if len(filenames) > 1 {
		name = "_" + name
	}
	name += extension
	w.Header().Set("Content-Disposition", "attachment; filename*=utf-8''"+url.PathEscape(name))

	if err := archiver.Archive(r.Context(), w, allFiles); err != nil {
		return http.StatusInternalServerError, err
	}

	return 0, nil
}

func rawFileHandler(w http.ResponseWriter, r *http.Request, file *files.FileInfo) (int, error) {
	fd, err := file.Fs.Open(file.Path)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer fd.Close()

	setContentDisposition(w, r, file)
	w.Header().Add("Content-Security-Policy", `script-src 'none';`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private")
	http.ServeContent(w, r, file.Name, file.ModTime, fd)
	return 0, nil
}
