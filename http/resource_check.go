package fbhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

var resourceCheckHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	var input struct {
		Base  string   `json:"base"`
		Paths []string `json:"paths"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMetadataBytes))
	if err := decoder.Decode(&input); err != nil {
		return 400, err
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return 400, nil
	}
	if len(input.Paths) > 1000 || !strings.HasPrefix(input.Base, "/") {
		return 400, nil
	}
	base := slashClean(input.Base)
	if !d.Check(base) {
		return 403, nil
	}
	type entry struct {
		Path     string    `json:"path"`
		Name     string    `json:"name"`
		Size     int64     `json:"size"`
		Modified time.Time `json:"modified"`
		IsDir    bool      `json:"isDir"`
	}
	entries := make([]entry, 0)
	seen := make(map[string]bool)
	for _, relative := range input.Paths {
		if err := r.Context().Err(); err != nil {
			return 0, err
		}
		if relative == "" || len(relative) > 4096 || strings.ContainsAny(relative, "\\\x00") || strings.HasPrefix(relative, "/") {
			return 400, nil
		}
		for _, part := range strings.Split(relative, "/") {
			if part == ".." || part == "." || part == "" {
				return 400, nil
			}
		}
		target := path.Join(base, relative)
		if !d.Check(target) {
			return 403, nil
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		info, err := d.user.Fs.Stat(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return errToStatus(err), err
		}
		entries = append(entries, entry{target, relative, info.Size(), info.ModTime(), info.IsDir()})
	}
	return renderJSON(w, r, entries)
})
