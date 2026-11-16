package fbhttp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filebrowser/filebrowser/v2/sessions"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

const accessCookie = "fb_access"
const refreshCookie = "fb_refresh"

func credentialFingerprint(user *users.User, configuration *settings.Settings) string {
	return secretHash(fmt.Sprintf("%d:%s:%s:%s", user.ID, configuration.AuthMethod, user.Password, secretHash(string(configuration.Key))))
}

func secretHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func sessionSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// Custom headers cannot be attached to a cross-origin form. Fetch metadata and
// Origin checks add defense in depth; forwarded headers never establish trust.
func sessionRequest(r *http.Request) bool {
	if r.Header.Get("X-Requested-With") != "FileBrowser" && r.Header.Get("X-Session-Transport") != "header" {
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.User != nil || !strings.EqualFold(parsed.Host, r.Host) || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return false
		}
	}
	return true
}

func secureSessionCookie(r *http.Request, server *settings.Server) bool {
	if !server.InsecureCookies || r.TLS != nil {
		return true
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, d *data, name, value string, expires time.Time) {
	path := d.server.BaseURL + "/"
	if name == refreshCookie {
		path = d.server.BaseURL + "/api/"
	}
	maxAge := int(time.Until(expires).Seconds())
	if value == "" {
		maxAge = -1
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, HttpOnly: true, Secure: secureSessionCookie(r, d.server), SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: maxAge})
}

func clearSessionCookies(w http.ResponseWriter, r *http.Request, d *data) {
	for _, name := range []string{accessCookie, refreshCookie} {
		setSessionCookie(w, r, d, name, "", time.Unix(1, 0))
	}
	// Remove the legacy root-scoped JavaScript cookie during migration.
	http.SetCookie(w, &http.Cookie{Name: "auth", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureSessionCookie(r, d.server), SameSite: http.SameSiteStrictMode})
}

func sessionError(err error) (int, error) {
	if errors.Is(err, sessions.ErrInvalid) {
		return http.StatusUnauthorized, nil
	}
	if errors.Is(err, sessions.ErrLimit) {
		return http.StatusTooManyRequests, nil
	}
	return http.StatusServiceUnavailable, err
}

func issueSession(w http.ResponseWriter, r *http.Request, d *data, user *users.User, ttl time.Duration) (int, error) {
	id, err := sessionSecret()
	if err != nil {
		return 500, err
	}
	secret, err := sessionSecret()
	if err != nil {
		return 500, err
	}
	now := time.Now()
	expires := now.Add(ttl)
	absolute := now.Add(sessions.AbsoluteLifetime)
	if expires.After(absolute) {
		expires = absolute
	}
	session := &sessions.Session{ID: id, UserID: user.ID, Credential: credentialFingerprint(user, d.settings), RefreshHash: secretHash(secret), Expires: expires, AbsoluteExpires: absolute}
	if err := d.store.Sessions.Create(session); err != nil {
		return sessionError(err)
	}
	d.session = session
	return respondSession(w, r, d, user, id+"."+secret)
}

func respondSession(w http.ResponseWriter, r *http.Request, d *data, user *users.User, refresh string) (int, error) {
	http.SetCookie(w, &http.Cookie{Name: "auth", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureSessionCookie(r, d.server), SameSite: http.SameSiteStrictMode})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if r.Header.Get("X-Session-Transport") == "header" {
		w.Header().Set("X-Refresh-Token", refresh)
	} else {
		setSessionCookie(w, r, d, refreshCookie, refresh, d.session.Expires)
	}
	return printToken(w, r, d, user, time.Until(d.session.Expires))
}

func refreshSession(w http.ResponseWriter, r *http.Request, d *data, ttl time.Duration) (int, error) {
	if !sessionRequest(r) {
		return http.StatusForbidden, nil
	}
	refresh := r.Header.Get("X-Refresh-Token")
	if refresh == "" && r.Header.Get("X-Session-Transport") != "header" {
		if cookie, err := r.Cookie(refreshCookie); err == nil {
			refresh = cookie.Value
		}
	}
	parts := strings.Split(refresh, ".")
	if len(parts) != 2 || len(parts[0]) != 43 || len(parts[1]) != 43 {
		return http.StatusUnauthorized, nil
	}
	session, err := d.store.Sessions.Get(parts[0])
	if err != nil {
		return sessionError(err)
	}
	user, err := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, session.UserID)
	if err != nil || session.Credential != credentialFingerprint(user, d.settings) {
		if revokeErr := d.store.Sessions.Revoke(session.ID); revokeErr != nil {
			return sessionError(revokeErr)
		}
		clearSessionCookies(w, r, d)
		return http.StatusUnauthorized, nil
	}
	secret, err := sessionSecret()
	if err != nil {
		return 500, err
	}
	session, err = d.store.Sessions.Rotate(session.ID, secretHash(parts[1]), secretHash(secret), time.Now().Add(ttl))
	if err != nil {
		clearSessionCookies(w, r, d)
		return sessionError(err)
	}
	d.session = session
	w.Header().Set("X-Renew-Token", "false")
	return respondSession(w, r, d, user, session.ID+"."+secret)
}

func logoutHandler(all bool) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if !sessionRequest(r) {
			return http.StatusForbidden, nil
		}
		// Cookie authentication is deliberately limited to this CSRF-protected
		// operation; ordinary mutations continue to require X-Auth.
		if r.Header.Get("X-Auth") == "" {
			if cookie, err := r.Cookie(accessCookie); err == nil {
				r.Header.Set("X-Auth", cookie.Value)
			}
		}
		status, err := withUser(func(_ http.ResponseWriter, _ *http.Request, d *data) (int, error) {
			var revokeErr error
			if all {
				revokeErr = d.store.Sessions.RevokeUser(d.user.ID)
			} else {
				revokeErr = d.store.Sessions.Revoke(d.session.ID)
			}
			if revokeErr != nil {
				return sessionError(revokeErr)
			}
			return http.StatusNoContent, nil
		})(w, r, d)
		if err == nil && (status == http.StatusNoContent || status == http.StatusUnauthorized) {
			clearSessionCookies(w, r, d)
		}
		return status, err
	}
}
