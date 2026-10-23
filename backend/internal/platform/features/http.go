package features

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// WriteError answers a NotEnabledError with 403 FEATURE_NOT_ENABLED and
// reports whether err was one; module error mappers call it first.
func WriteError(w http.ResponseWriter, r *http.Request, err error) bool {
	f, ok := FeatureOf(err)
	if !ok {
		return false
	}
	response.RecordFailure(w, err)
	response.FeatureNotEnabled(w, r, string(f))
	return true
}
