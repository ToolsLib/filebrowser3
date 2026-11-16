package fbhttp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"

	"github.com/filebrowser/filebrowser/v2/auth"
	"github.com/filebrowser/filebrowser/v2/storage"
)

type nonceContextKey struct{}

// Wrap the router itself: mux middleware does not protect SPA fallbacks.
func securityHeaders(next http.Handler, store *storage.Storage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bytes := make([]byte, 24)
		if _, err := rand.Read(bytes); err != nil {
			http.Error(w, "cannot initialize request", http.StatusInternalServerError)
			return
		}
		nonce := base64.RawURLEncoding.EncodeToString(bytes)
		scripts, frames, connections := "", "", ""
		if settings, err := store.Settings.Get(); err == nil && settings.AuthMethod == auth.MethodJSONAuth {
			if method, err := store.Auth.Get(auth.MethodJSONAuth); err == nil {
				if jsonAuth, ok := method.(*auth.JSONAuth); ok && jsonAuth.ReCaptcha != nil {
					captcha := jsonAuth.ReCaptcha
					if host, err := url.Parse(captcha.Host); err == nil && host.Scheme == "https" && host.Host != "" && captcha.Key != "" && captcha.Secret != "" {
						origin := host.Scheme + "://" + host.Host
						// reCAPTCHA's secondary resources use its documented Google origins.
						scripts = " " + origin + " https://www.gstatic.com"
						frames = " " + origin + " https://www.google.com https://recaptcha.google.com"
						connections = " " + origin + " https://www.google.com"
					}
				}
			}
		}
		policy := []string{
			"default-src 'self'", "script-src 'self' 'nonce-" + nonce + "'" + scripts,
			"style-src 'self' 'unsafe-inline'", "img-src 'self' data: blob:",
			"font-src 'self' data:", "media-src 'self' blob:", "worker-src 'self' blob:",
			"connect-src 'self'" + connections, "frame-src 'self' blob:" + frames,
			"manifest-src 'self' blob:", "object-src 'none'", "base-uri 'self'",
			"form-action 'self'", "frame-ancestors 'none'",
		}
		w.Header().Set("Content-Security-Policy", strings.Join(policy, "; ")+";")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), nonceContextKey{}, nonce)))
	})
}
