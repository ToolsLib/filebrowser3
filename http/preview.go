//go:generate go-enum --sql --marshal --names --file $GOFILE
package fbhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/img"
)

/*
ENUM(
thumb
big
)
*/
type PreviewSize int

type ImgService interface {
	FormatFromExtension(ext string) (img.Format, error)
	Resize(ctx context.Context, in io.Reader, width, height int, out io.Writer, options ...img.Option) error
}

type FileCache interface {
	Store(ctx context.Context, key string, value []byte) error
	Load(ctx context.Context, key string) ([]byte, bool, error)
	Delete(ctx context.Context, key string) error
}

func previewHandler(imgSvc ImgService, fileCache FileCache, enableThumbnails, resizePreview bool) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if !d.user.Perm.Download {
			return http.StatusAccepted, nil
		}
		vars := mux.Vars(r)

		previewSize, err := ParsePreviewSize(vars["size"])
		if err != nil {
			return http.StatusBadRequest, err
		}

		file, err := files.NewFileInfo(&files.FileOptions{
			Fs: d.user.Fs,
			// Preview reads its path from mux.Vars, not r.URL.Path, so it does
			// not get the canonicalization withUser applies.
			Path:       slashClean(vars["path"]),
			Modify:     d.user.Perm.Modify,
			Expand:     true,
			ReadHeader: d.server.TypeDetectionByHeader,
			Checker:    d,
		})
		if err != nil {
			return errToStatus(err), err
		}

		setContentDisposition(w, r, file)

		switch file.Type {
		case "image":
			return handleImagePreview(w, r, imgSvc, fileCache, file, previewSize, enableThumbnails, resizePreview)
		default:
			return http.StatusNotImplemented, fmt.Errorf("can't create preview for %s type", file.Type)
		}
	})
}

func handleImagePreview(
	w http.ResponseWriter,
	r *http.Request,
	imgSvc ImgService,
	fileCache FileCache,
	file *files.FileInfo,
	previewSize PreviewSize,
	enableThumbnails, resizePreview bool,
) (int, error) {
	if (previewSize == PreviewSizeBig && !resizePreview) ||
		(previewSize == PreviewSizeThumb && !enableThumbnails) {
		return rawFileHandler(w, r, file)
	}

	format, err := imgSvc.FormatFromExtension(file.Extension)
	// Unsupported extensions directly return the raw data
	if errors.Is(err, img.ErrUnsupportedFormat) || format == img.FormatGif {
		return rawFileHandler(w, r, file)
	}
	if err != nil {
		return errToStatus(err), err
	}

	cacheKey := previewCacheKey(file, previewSize)
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(cacheKey)))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=60")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return 0, nil
	}
	if cache, ok := fileCache.(interface {
		Open(context.Context, string) (afero.File, bool, error)
	}); ok {
		fd, found, openErr := cache.Open(r.Context(), cacheKey)
		if openErr != nil && r.Context().Err() != nil {
			return 0, r.Context().Err()
		}
		if found {
			defer fd.Close()
			http.ServeContent(w, r, file.Name, file.ModTime, fd)
			return 0, nil
		}
	}
	resizedImage, ok, err := fileCache.Load(r.Context(), cacheKey)
	if err != nil && r.Context().Err() != nil {
		return 0, r.Context().Err()
	}
	if !ok {
		resizedImage, err = sharedPreview(r.Context(), fmt.Sprintf("%p:%p:%s", imgSvc, fileCache, cacheKey), func(ctx context.Context) ([]byte, error) {
			if cached, found, loadErr := fileCache.Load(ctx, cacheKey); loadErr == nil && found {
				return cached, nil
			}
			return createPreview(ctx, imgSvc, fileCache, file, previewSize)
		})
		if err != nil {
			if errors.Is(err, errPreviewBusy) {
				return http.StatusServiceUnavailable, err
			}
			return errToStatus(err), err
		}
	}

	http.ServeContent(w, r, file.Name, file.ModTime, bytes.NewReader(resizedImage))

	return 0, nil
}

func createPreview(ctx context.Context, imgSvc ImgService, fileCache FileCache,
	file *files.FileInfo, previewSize PreviewSize) ([]byte, error) {
	fd, err := file.Fs.Open(file.Path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	var (
		width   int
		height  int
		options []img.Option
	)

	switch previewSize {
	case PreviewSizeBig:
		width = 1080
		height = 1080
		options = append(options, img.WithMode(img.ResizeModeFit), img.WithQuality(img.QualityMedium))
	case PreviewSizeThumb:
		width = 256
		height = 256
		options = append(options, img.WithMode(img.ResizeModeFill), img.WithQuality(img.QualityLow), img.WithFormat(img.FormatJpeg))
	default:
		return nil, img.ErrUnsupportedFormat
	}

	buf := &bytes.Buffer{}
	if err := imgSvc.Resize(ctx, uploadContextReader{ctx: ctx, reader: fd}, width, height, buf, options...); err != nil {
		return nil, err
	}

	if err := fileCache.Store(ctx, previewCacheKey(file, previewSize), buf.Bytes()); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Printf("failed to cache resized image: %v", err)
	}

	return buf.Bytes(), nil
}

func previewCacheKey(f *files.FileInfo, previewSize PreviewSize) string {
	return fmt.Sprintf("v2:%s:%d:%d:%d", f.RealPath(), f.ModTime.UnixNano(), f.Size, previewSize)
}
