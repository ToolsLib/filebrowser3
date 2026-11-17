package fbhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/filebrowser/filebrowser/v2/auth"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/version"
)

func handleWithStaticData(w http.ResponseWriter, r *http.Request, d *data, fSys fs.FS, file, contentType string, cached ...*template.Template) (int, error) {
	w.Header().Set("Content-Type", contentType)

	auther, err := d.store.Auth.Get(d.settings.AuthMethod)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	data := map[string]interface{}{
		"CSPNonce":              r.Context().Value(nonceContextKey{}),
		"Name":                  d.settings.Branding.Name,
		"DisableExternal":       d.settings.Branding.DisableExternal,
		"DisableUsedPercentage": d.settings.Branding.DisableUsedPercentage,
		"Color":                 d.settings.Branding.Color,
		"BaseURL":               d.server.BaseURL,
		"Version":               version.Version,
		"StaticURL":             path.Join(d.server.BaseURL, "/static"),
		"Signup":                d.settings.Signup,
		"NoAuth":                d.settings.AuthMethod == auth.MethodNoAuth,
		"AuthMethod":            d.settings.AuthMethod,
		"LogoutPage":            d.settings.LogoutPage,
		"LoginPage":             auther.LoginPage(),
		"CSS":                   false,
		"ReCaptcha":             false,
		"Theme":                 d.settings.Branding.Theme,
		"EnableThumbs":          d.server.EnableThumbnails,
		"ResizePreview":         d.server.ResizePreview,
		"EnableExec":            d.server.EnableExec,
		"TusSettings":           d.settings.Tus,
		"HideLoginButton":       d.settings.HideLoginButton,
	}

	if d.settings.Branding.Files != "" {
		fPath := filepath.Join(d.settings.Branding.Files, "custom.css")
		_, err := os.Stat(fPath)

		if err != nil && !os.IsNotExist(err) {
			log.Printf("couldn't load custom styles: %v", err)
		}

		if err == nil {
			data["CSS"] = true
		}
	}

	if d.settings.AuthMethod == auth.MethodJSONAuth {
		auther := auther.(*auth.JSONAuth)

		if auther.ReCaptcha != nil {
			data["ReCaptcha"] = auther.ReCaptcha.Key != "" && auther.ReCaptcha.Secret != ""
			data["ReCaptchaHost"] = auther.ReCaptcha.Host
			data["ReCaptchaKey"] = auther.ReCaptcha.Key
		}
	}

	b, err := json.Marshal(data)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	data["Json"] = template.JS(strings.ReplaceAll(string(b), `'`, `\'`))

	var index *template.Template
	if len(cached) > 0 {
		index = cached[0]
	} else {
		fileContents, readErr := fs.ReadFile(fSys, file)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				return http.StatusNotFound, readErr
			}
			return http.StatusInternalServerError, readErr
		}
		index, err = template.New("index").Delims("[{[", "]}]").Parse(string(fileContents))
		if err != nil {
			return http.StatusInternalServerError, err
		}
	}
	err = index.Execute(w, data)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return 0, nil
}

func getStaticHandlers(store *storage.Storage, server *settings.Server, assetsFs fs.FS) (index, static http.Handler) {
	var once sync.Once
	var parsed *template.Template
	var parseErr error
	immutable := regexp.MustCompile(`^(assets/[^/]+-[A-Za-z0-9_-]{8,}\.|ace/[^/]+/)`)
	index = handle(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if r.Method != http.MethodGet {
			return http.StatusNotFound, nil
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		once.Do(func() {
			var body []byte
			body, parseErr = fs.ReadFile(assetsFs, "public/index.html")
			if parseErr == nil {
				parsed, parseErr = template.New("index").Delims("[{[", "]}]").Parse(string(body))
			}
		})
		if parseErr != nil {
			if errors.Is(parseErr, os.ErrNotExist) {
				return http.StatusNotFound, parseErr
			}
			return http.StatusInternalServerError, parseErr
		}
		return handleWithStaticData(w, r, d, assetsFs, "public/index.html", "text/html; charset=utf-8", parsed)
	}, "", store, server)

	static = handle(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if r.Method != http.MethodGet {
			return http.StatusNotFound, nil
		}

		if strings.HasSuffix(r.URL.Path, "/") {
			return http.StatusNotFound, nil
		}

		const maxAge = 86400 // 1 day
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%v", maxAge))
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if d.settings.Branding.Files != "" {
			if strings.HasPrefix(r.URL.Path, "img/") {
				fPath := filepath.Join(d.settings.Branding.Files, r.URL.Path)
				_, err := os.Stat(fPath)
				if err != nil && !os.IsNotExist(err) {
					log.Printf("could not load branding file override: %v", err)
				} else if err == nil {
					http.ServeFile(w, r, fPath)
					return 0, nil
				}
			} else if r.URL.Path == "custom.css" && d.settings.Branding.Files != "" {
				http.ServeFile(w, r, filepath.Join(d.settings.Branding.Files, "custom.css"))
				return 0, nil
			}
		}

		if immutable.MatchString(r.URL.Path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		supported := make([]string, 0, 2)
		suffixes := map[string]string{"br": ".br", "gzip": ".gz"}
		for _, coding := range []string{"br", "gzip"} {
			if info, err := fs.Stat(assetsFs, r.URL.Path+suffixes[coding]); err == nil && !info.IsDir() {
				supported = append(supported, coding)
			}
		}
		w.Header().Add("Vary", "Accept-Encoding")
		coding := chooseEncoding(r.Header.Get("Accept-Encoding"), supported...)
		if coding == "not-acceptable" {
			return http.StatusNotAcceptable, nil
		}
		if coding != "" {
			f, err := assetsFs.Open(r.URL.Path + suffixes[coding])
			if err != nil {
				return errToStatus(err), err
			}
			defer f.Close()
			contentType := mime.TypeByExtension(filepath.Ext(r.URL.Path))
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			w.Header().Set("Content-Encoding", coding)
			if seekable, ok := f.(io.ReadSeeker); ok {
				http.ServeContent(w, r, r.URL.Path, time.Time{}, seekable)
			} else {
				body, err := io.ReadAll(f)
				if err != nil {
					return http.StatusInternalServerError, err
				}
				http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(body))
			}
			return 0, nil
		}
		http.FileServer(http.FS(assetsFs)).ServeHTTP(w, r)

		return 0, nil
	}, "/static/", store, server)

	return index, static
}
