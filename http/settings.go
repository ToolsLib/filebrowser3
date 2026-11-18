package fbhttp

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

type settingsData struct {
	Signup                bool                  `json:"signup"`
	HideLoginButton       bool                  `json:"hideLoginButton"`
	CreateUserDir         bool                  `json:"createUserDir"`
	MinimumPasswordLength uint                  `json:"minimumPasswordLength"`
	UserHomeBasePath      string                `json:"userHomeBasePath"`
	Defaults              settings.UserDefaults `json:"defaults"`
	AuthMethod            settings.AuthMethod   `json:"authMethod"`
	Rules                 []rules.Rule          `json:"rules"`
	Branding              settings.Branding     `json:"branding"`
	Tus                   settings.Tus          `json:"tus"`
	Shell                 []string              `json:"shell"`
	Commands              map[string][]string   `json:"commands"`
	MaxUploadSize         int64                 `json:"maxUploadSize"`
	DownloadRate          int64                 `json:"downloadRate"`
	UploadLimits          uploadLimits          `json:"uploadLimits"`
}

var settingsGetHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	data := &settingsData{
		Signup:                d.settings.Signup,
		HideLoginButton:       d.settings.HideLoginButton,
		CreateUserDir:         d.settings.CreateUserDir,
		MinimumPasswordLength: d.settings.MinimumPasswordLength,
		UserHomeBasePath:      d.settings.UserHomeBasePath,
		Defaults:              d.settings.Defaults,
		AuthMethod:            d.settings.AuthMethod,
		Rules:                 d.settings.Rules,
		Branding:              d.settings.Branding,
		Tus:                   d.settings.Tus,
		Shell:                 d.settings.Shell,
		Commands:              d.settings.Commands,
		MaxUploadSize:         d.settings.MaxUploadSize,
		DownloadRate:          d.settings.DownloadRate,
		UploadLimits:          describeUploadLimits(d),
	}

	return renderJSON(w, r, data)
})

var settingsPutHandler = withAdmin(func(_ http.ResponseWriter, r *http.Request, d *data) (int, error) {
	req := &settingsData{}
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if req.MaxUploadSize < 0 || req.MaxUploadSize > settings.MaximumUploadSize || req.Defaults.MaxUploadSize < 0 || req.Defaults.MaxUploadSize > settings.MaximumUploadSize {
		return http.StatusBadRequest, fmt.Errorf("upload limits must be between 0 and %d bytes", settings.MaximumUploadSize)
	}

	if !users.ValidDownloadRate(&req.DownloadRate) || !users.ValidDownloadRate(req.Defaults.DownloadRate) {
		return http.StatusBadRequest, fmt.Errorf("download speeds must be between 0 and %d bytes/second", users.MaximumDownloadRate)
	}
	d.settings.Signup = req.Signup
	d.settings.CreateUserDir = req.CreateUserDir
	d.settings.MinimumPasswordLength = req.MinimumPasswordLength
	d.settings.UserHomeBasePath = req.UserHomeBasePath
	d.settings.Defaults = req.Defaults
	d.settings.Rules = req.Rules
	d.settings.Branding = req.Branding
	d.settings.Tus = req.Tus
	d.settings.Shell = req.Shell
	d.settings.Commands = req.Commands
	d.settings.HideLoginButton = req.HideLoginButton
	d.settings.MaxUploadSize = req.MaxUploadSize
	d.settings.DownloadRate = req.DownloadRate

	err = d.store.Settings.Save(d.settings)
	return errToStatus(err), err
})
