package fbhttp

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/asticode/go-astisub"

	"github.com/filebrowser/filebrowser/v2/files"
)

var srtLineBreakTag = regexp.MustCompile(`(?i)<br(?:\s+[^>]*)?\s*/?>`)

const maxSubtitleInput = 10 << 20
const maxSubtitleOutput = 16 << 20

var subtitleConversions = make(chan struct{}, 2)
var errSubtitleLimit = errors.New("subtitle conversion exceeds size limit")

type subtitleBuffer struct{ bytes.Buffer }

func (b *subtitleBuffer) Write(p []byte) (int, error) {
	if len(p) > maxSubtitleOutput-b.Len() {
		return 0, errSubtitleLimit
	}
	return b.Buffer.Write(p)
}

func (b *subtitleBuffer) WriteString(value string) (int, error) { return b.Write([]byte(value)) }

var subtitleHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
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

	if file.IsDir {
		return http.StatusBadRequest, nil
	}

	return subtitleFileHandler(w, r, file)
})

func subtitleFileHandler(w http.ResponseWriter, r *http.Request, file *files.FileInfo) (int, error) {
	// if its not a subtitle file, reject
	if !files.IsSupportedSubtitle(file.Name) {
		return http.StatusBadRequest, nil
	}

	info, err := file.Fs.Stat(file.Path)
	if err != nil {
		return errToStatus(err), err
	}
	if !info.Mode().IsRegular() {
		return http.StatusBadRequest, nil
	}
	if info.Size() > maxSubtitleInput {
		return http.StatusRequestEntityTooLarge, errSubtitleLimit
	}
	fd, err := file.Fs.Open(file.Path)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer fd.Close()
	info, err = fd.Stat()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !info.Mode().IsRegular() {
		return http.StatusBadRequest, nil
	}
	if info.Size() > maxSubtitleInput {
		return http.StatusRequestEntityTooLarge, errSubtitleLimit
	}
	converting := !strings.EqualFold(file.Extension, ".vtt") && !strings.HasSuffix(strings.ToLower(file.Name), ".vtt")
	if converting {
		select {
		case subtitleConversions <- struct{}{}:
			defer func() { <-subtitleConversions }()
		default:
			return http.StatusTooManyRequests, nil
		}
	}

	// load subtitle for conversion to vtt
	var sub *astisub.Subtitles
	if converting {
		content, readErr := io.ReadAll(io.LimitReader(fd, maxSubtitleInput+1))
		if readErr != nil {
			return http.StatusInternalServerError, readErr
		}
		if len(content) > maxSubtitleInput {
			return http.StatusRequestEntityTooLarge, errSubtitleLimit
		}
		if strings.HasSuffix(strings.ToLower(file.Name), ".srt") {
			sub, err = astisub.ReadFromSRT(bytes.NewReader(normalizeSRTLineBreaks(content)))
		} else {
			sub, err = astisub.ReadFromSSA(bytes.NewReader(content))
		}
	}
	if err != nil {
		return http.StatusInternalServerError, err
	}

	setContentDisposition(w, r, file)
	w.Header().Add("Content-Security-Policy", `script-src 'none';`)
	w.Header().Set("Cache-Control", "private")
	// force type to text/vtt
	w.Header().Set("Content-Type", "text/vtt")

	// serve vtt file directly
	if sub == nil {
		http.ServeContent(w, r, file.Name, file.ModTime, fd)
		return 0, nil
	}

	// convert others to vtt and serve from buffer
	var buf = &subtitleBuffer{}
	err = sub.WriteToWebVTT(buf)
	if err != nil {
		if errors.Is(err, errSubtitleLimit) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusInternalServerError, err
	}
	http.ServeContent(w, r, file.Name, file.ModTime, bytes.NewReader(buf.Bytes()))
	return 0, nil
}

func normalizeSRTLineBreaks(content []byte) []byte {
	return srtLineBreakTag.ReplaceAll(content, []byte("\n"))
}
