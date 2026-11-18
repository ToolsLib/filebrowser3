package fbhttp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
	"golang.org/x/crypto/bcrypt"
)

// shareResponse is the client-facing representation of a share. It deliberately
// omits the server-side secrets of share.Link — the bcrypt PasswordHash (which
// would be crackable offline) and the bypass Token — exposing only whether the
// share is password-protected via HasPassword.
type shareResponse struct {
	Hash        string `json:"hash"`
	Path        string `json:"path"`
	UserID      uint   `json:"userID"`
	Expire      int64  `json:"expire"`
	HasPassword bool   `json:"hasPassword"`
}

func toShareResponse(l *share.Link) *shareResponse {
	return &shareResponse{
		Hash:        l.Hash,
		Path:        l.Path,
		UserID:      l.UserID,
		Expire:      l.Expire,
		HasPassword: l.PasswordHash != "",
	}
}

func toShareResponses(links []*share.Link) []*shareResponse {
	res := make([]*shareResponse, 0, len(links))
	for _, l := range links {
		res = append(res, toShareResponse(l))
	}
	return res
}

func withPermShare(fn handleFunc) handleFunc {
	return withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if !d.user.Perm.Share || !d.user.Perm.Download {
			return http.StatusForbidden, nil
		}

		return fn(w, r, d)
	})
}

var shareListHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	var (
		s   []*share.Link
		err error
	)
	if d.user.Perm.Admin {
		s, err = d.store.Share.All()
	} else {
		s, err = d.store.Share.FindByUserID(d.user.ID)
	}
	if errors.Is(err, fberrors.ErrNotExist) {
		return renderJSON(w, r, []*shareResponse{})
	}

	if err != nil {
		return http.StatusInternalServerError, err
	}

	sort.Slice(s, func(i, j int) bool {
		if s[i].UserID != s[j].UserID {
			return s[i].UserID < s[j].UserID
		}
		return s[i].Expire < s[j].Expire
	})

	return renderJSON(w, r, toShareResponses(s))
})

var shareGetsHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	var (
		s   []*share.Link
		err error
	)
	if d.user.Perm.Admin {
		s, err = getSharesForAdminPath(d, r.URL.Path)
	} else {
		s, err = d.store.Share.Gets(r.URL.Path, d.user.ID)
	}
	if errors.Is(err, fberrors.ErrNotExist) {
		return renderJSON(w, r, []*shareResponse{})
	}

	if err != nil {
		return http.StatusInternalServerError, err
	}

	return renderJSON(w, r, toShareResponses(s))
})

func getSharesForAdminPath(d *data, path string) ([]*share.Link, error) {
	links, err := d.store.Share.All()
	if err != nil {
		return nil, err
	}

	adminPath := filepath.Clean(d.user.FullPath(path))
	owners := make(map[uint]*users.User)
	filtered := make([]*share.Link, 0, len(links))
	for _, link := range links {
		owner, ok := owners[link.UserID]
		if !ok {
			owner, err = d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, link.UserID)
			if err != nil && !errors.Is(err, fberrors.ErrNotExist) {
				return nil, err
			}
			owners[link.UserID] = owner // owner is nil on ErrNotExist
		}
		if owner != nil && filepath.Clean(owner.FullPath(link.Path)) == adminPath {
			filtered = append(filtered, link)
		}
	}

	return filtered, nil
}

var shareDeleteHandler = withPermShare(func(_ http.ResponseWriter, r *http.Request, d *data) (int, error) {
	hash := strings.TrimSuffix(r.URL.Path, "/")
	hash = strings.TrimPrefix(hash, "/")

	if hash == "" {
		return http.StatusBadRequest, nil
	}

	link, err := d.store.Share.GetByHash(hash)
	if err != nil {
		return errToStatus(err), err
	}

	if link.UserID != d.user.ID && !d.user.Perm.Admin {
		return http.StatusForbidden, nil
	}

	err = d.store.Share.Delete(hash)
	return errToStatus(err), err
})

var sharePutHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	hash := strings.Trim(r.URL.Path, "/")
	if hash == "" || strings.Contains(hash, "/") {
		return http.StatusBadRequest, nil
	}
	var body share.UpdateBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return http.StatusBadRequest, fmt.Errorf("expected a single JSON object")
	}
	if body.Password == nil && body.Expires == nil {
		return http.StatusBadRequest, fmt.Errorf("no share changes supplied")
	}
	if body.Unit != "" && body.Expires == nil {
		return http.StatusBadRequest, fmt.Errorf("duration unit requires expires")
	}
	if body.Expires != nil {
		if _, err := shareExpiration(*body.Expires, body.Unit, time.Now()); err != nil {
			return http.StatusBadRequest, err
		}
	}
	link, err := d.store.Share.GetByHash(hash)
	if err != nil {
		return errToStatus(err), err
	}
	if link.UserID != d.user.ID && !d.user.Perm.Admin {
		return http.StatusForbidden, nil
	}
	var passwordHash, token string
	if body.Password != nil {
		hashed, status, err := getSharePasswordHash(share.CreateBody{Password: *body.Password})
		if err != nil {
			return status, err
		}
		passwordHash = string(hashed)
		if passwordHash != "" {
			token, err = newShareAccessToken()
			if err != nil {
				return http.StatusInternalServerError, err
			}
		}
	}
	updated, err := d.store.Share.Update(link.Hash, func(current *share.Link) error {
		if current.UserID != d.user.ID && !d.user.Perm.Admin {
			return fberrors.ErrPermissionDenied
		}
		if body.Password != nil {
			current.PasswordHash, current.Token = passwordHash, token
		}
		if body.Expires != nil {
			var err error
			current.Expire, err = shareExpiration(*body.Expires, body.Unit, time.Now())
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return errToStatus(err), err
	}
	w.Header().Set("Cache-Control", "no-store")
	return renderJSON(w, r, toShareResponse(updated))
})

func shareExpiration(expires, unit string, now time.Time) (int64, error) {
	if expires == "" {
		return 0, nil
	}
	num, err := strconv.ParseUint(expires, 10, 64)
	if err != nil || num == 0 {
		return 0, fmt.Errorf("share duration must be a positive whole number; use an empty expires value for a permanent link")
	}
	multiplier := uint64(3600)
	switch unit {
	case "seconds":
		multiplier = 1
	case "minutes":
		multiplier = 60
	case "hours", "":
	case "days":
		multiplier = 86400
	default:
		return 0, fmt.Errorf("invalid share duration unit")
	}
	const maxShareExpiry int64 = 253402300799 // End of year 9999, representable in clients.
	remaining := maxShareExpiry - now.Unix()
	if remaining <= 0 || num > uint64(remaining)/multiplier {
		return 0, fmt.Errorf("share duration is too large")
	}
	return now.Unix() + int64(num*multiplier), nil
}

func newShareAccessToken() (string, error) {
	buffer := make([]byte, 96)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buffer), nil
}

var sharePostHandler = withPermShare(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	// Only allow sharing paths that currently exist. Otherwise a share could be
	// created for a non-existent path and would silently start exposing
	// whatever file later appears there.
	//
	// d.user.Fs is scoped, so Stat also refuses to follow a symlink whose target
	// escapes the user's scope: that returns a permission error here and so
	// blocks creating a share that points out of scope.
	if _, err := d.user.Fs.Stat(r.URL.Path); err != nil {
		return errToStatus(err), err
	}

	var s *share.Link
	var body share.CreateBody
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return http.StatusBadRequest, fmt.Errorf("failed to decode body: %w", err)
		}
		defer r.Body.Close()
	}

	bytes := make([]byte, 6)
	_, err := rand.Read(bytes)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	str := base64.URLEncoding.EncodeToString(bytes)

	expire, err := shareExpiration(body.Expires, body.Unit, time.Now())
	if err != nil {
		return http.StatusBadRequest, err
	}

	hash, status, err := getSharePasswordHash(body)
	if err != nil {
		return status, err
	}

	var token string
	if len(hash) > 0 {
		token, err = newShareAccessToken()
		if err != nil {
			return http.StatusInternalServerError, err
		}
	}

	s = &share.Link{
		Path:         r.URL.Path,
		Hash:         str,
		Expire:       expire,
		UserID:       d.user.ID,
		PasswordHash: string(hash),
		Token:        token,
	}

	if err := d.store.Share.Save(s); err != nil {
		return http.StatusInternalServerError, err
	}

	return renderJSON(w, r, toShareResponse(s))
})

func getSharePasswordHash(body share.CreateBody) (data []byte, statuscode int, err error) {
	if body.Password == "" {
		return nil, 0, nil
	}
	if len(body.Password) > 72 {
		return nil, http.StatusBadRequest, fmt.Errorf("share password must be at most 72 UTF-8 bytes")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to hash password: %w", err)
	}

	return hash, 0, nil
}
