package middleware

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// RequireFeature answers 403 FEATURE_NOT_ENABLED unless the current
// organization's plan (with its overrides) enables the feature (TEC-674).
// The features come with the resolved orgctx.Scope (cached with it), so
// the check costs no query. A scope built without features (nil: a
// resolver with no feature source, as in unit tests) is not gated, like
// features.Require with a nil policy; the organizations resolver always
// sets them. Must run after RequireOrganization.
func RequireFeature(f features.Feature) func(http.Handler) http.Handler {
	return RequireAnyFeature(f)
}

// RequireAnyFeature allows the request when at least one of the features
// is enabled (a route shared by several features, e.g. the channel
// manager of Yolcu360 and Obilet; the handler then checks the record's own
// feature). Must run after RequireOrganization.
func RequireAnyFeature(fs ...features.Feature) func(http.Handler) http.Handler {
	if len(fs) == 0 {
		panic("middleware: RequireAnyFeature needs a feature")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, ok := orgctx.From(r.Context())
			if !ok {
				response.InternalErr(w, r, errors.New("RequireFeature without RequireOrganization"), "")
				return
			}
			if scope.Features == nil {
				next.ServeHTTP(w, r)
				return
			}
			for _, f := range fs {
				if scope.HasFeature(string(f)) {
					next.ServeHTTP(w, r)
					return
				}
			}
			response.FeatureNotEnabled(w, r, string(fs[0]))
		})
	}
}
