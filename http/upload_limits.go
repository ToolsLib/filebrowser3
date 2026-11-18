package fbhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/filebrowser/filebrowser/v2/settings"
)

type uploadLimits struct {
	DeploymentLimit  int64  `json:"deploymentLimit"`
	DeploymentSource string `json:"deploymentSource"`
	DefaultLimit     int64  `json:"defaultLimit"`
	GlobalLimit      int64  `json:"globalLimit"`
}

func describeUploadLimits(d *data) uploadLimits {
	ceiling := d.server.MaxUploadSize
	if ceiling < 0 {
		ceiling = 0
	}
	if ceiling > settings.MaximumUploadSize {
		ceiling = settings.MaximumUploadSize
	}
	global := d.settings.MaxUploadSize
	if global <= 0 {
		global = settings.DefaultMaxUploadSize
		if ceiling > 0 {
			global = ceiling
		}
	}
	if ceiling > 0 && global > ceiling {
		global = ceiling
	}
	return uploadLimits{ceiling, d.server.MaxUploadSizeSource, settings.DefaultMaxUploadSize, min(global, settings.MaximumUploadSize)}
}

func effectiveUploadLimit(d *data) int64 {
	limit := describeUploadLimits(d).GlobalLimit
	if d.user != nil && d.user.MaxUploadSize > 0 {
		limit = min(limit, d.user.MaxUploadSize)
	}
	return limit
}

var uploadLimitGetHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return renderJSON(w, r, map[string]int64{"maxUploadSize": effectiveUploadLimit(d)})
})

type uploadLimitError struct{ limit int64 }

func (e *uploadLimitError) Error() string {
	return fmt.Sprintf("file exceeds the effective upload limit of %d bytes", e.limit)
}

func writeUploadLimitError(w http.ResponseWriter, limit int64) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code": "upload_too_large", "maxUploadSize": limit,
		"message": fmt.Sprintf("File is too large. Maximum allowed size is %d bytes.", limit),
	})
}

func limitUploadBody(w http.ResponseWriter, r *http.Request, d *data) error {
	limit := effectiveUploadLimit(d)
	if r.ContentLength > limit {
		return &uploadLimitError{limit}
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return nil
}

func uploadWriteResult(err error, d *data) (int, error) {
	var bodyError *http.MaxBytesError
	if errors.As(err, &bodyError) {
		return http.StatusRequestEntityTooLarge, &uploadLimitError{effectiveUploadLimit(d)}
	}
	return errToStatus(err), err
}
