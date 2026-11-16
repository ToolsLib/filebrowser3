package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

// MethodJSONAuth is used to identify json auth.
const MethodJSONAuth settings.AuthMethod = "json"

// dummyHash is used to prevent user enumeration timing attacks.
// It MUST be a valid bcrypt hash.
const dummyHash = "$2a$10$O4mEMeOL/nit6zqe.WQXauLRbRlzb3IgLHsa26Pf0N/GiU9b.wK1m"

type jsonCred struct {
	Password  string `json:"password"`
	Username  string `json:"username"`
	ReCaptcha string `json:"recaptcha"`
}

// JSONAuth is a json implementation of an Auther.
type JSONAuth struct {
	ReCaptcha *ReCaptcha `json:"recaptcha" yaml:"recaptcha"`
}

// Auth authenticates the user via a json in content body.
func (a JSONAuth) Auth(r *http.Request, usr users.Store, _ *settings.Settings, srv *settings.Server) (*users.User, error) {
	var cred jsonCred

	if r.Body == nil {
		return nil, os.ErrPermission
	}

	err := json.NewDecoder(r.Body).Decode(&cred)
	if err != nil {
		return nil, os.ErrPermission
	}

	// If ReCaptcha is enabled, check the code.
	if a.ReCaptcha != nil && a.ReCaptcha.Secret != "" {
		ok, err := a.ReCaptcha.OkContext(r.Context(), cred.ReCaptcha)

		if err != nil {
			return nil, err
		}

		if !ok {
			return nil, os.ErrPermission
		}
	}

	u, err := usr.Get(srv.Root, srv.FollowExternalSymlinks, cred.Username)

	hash := dummyHash
	if err == nil {
		hash = u.Password
	}

	if !users.CheckPwd(cred.Password, hash) {
		return nil, os.ErrPermission
	}

	if err != nil {
		return nil, os.ErrPermission
	}

	return u, nil
}

// LoginPage tells that json auth doesn't require a login page.
func (a JSONAuth) LoginPage() bool {
	return true
}

const reCaptchaAPI = "/recaptcha/api/siteverify"

// ReCaptcha identifies a recaptcha connection.
type ReCaptcha struct {
	Host   string `json:"host"`
	Key    string `json:"key"`
	Secret string `json:"secret"`
}

// Ok checks if a reCaptcha responde is correct.
func (r *ReCaptcha) Ok(response string) (bool, error) {
	return r.OkContext(context.Background(), response)
}

func (r *ReCaptcha) OkContext(ctx context.Context, response string) (bool, error) {
	body := url.Values{}
	body.Set("secret", r.Secret)
	body.Add("response", response)

	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Host+reCaptchaAPI, strings.NewReader(body.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var data struct {
		Success bool `json:"success"`
	}

	limited, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil {
		return false, err
	}
	if len(limited) > 64<<10 {
		return false, io.ErrShortBuffer
	}
	err = json.Unmarshal(limited, &data)
	if err != nil {
		return false, err
	}

	return data.Success, nil
}
