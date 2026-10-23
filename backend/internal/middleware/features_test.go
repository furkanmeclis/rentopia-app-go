package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// RT-A28: a feature outside the organization's plan answers the standard
// envelope with 403 FEATURE_NOT_ENABLED and the feature key; an enabled
// feature passes.
func TestRequireFeature(t *testing.T) {
	t.Parallel()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	serve := func(scope *orgctx.Scope, guard func(http.Handler) http.Handler) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenant/public-api/keys", nil)
		if scope != nil {
			req = req.WithContext(orgctx.WithScope(req.Context(), *scope))
		}
		rr := httptest.NewRecorder()
		guard(ok).ServeHTTP(rr, req)
		return rr
	}

	management := &orgctx.Scope{ID: 1, Features: []string{}}
	rr := serve(management, RequireFeature(features.PublicAPI))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("management plan: status %d", rr.Code)
	}
	var env response.Envelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Success || env.Error == nil || env.Error.Code != response.CodeFeatureNotEnabled {
		t.Fatalf("envelope: %s", rr.Body.String())
	}
	if len(env.Error.Details) != 1 || env.Error.Details[0].Field != "feature" || env.Error.Details[0].Code != "public_api" {
		t.Fatalf("details: %+v", env.Error.Details)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type %q", rr.Header().Get("Content-Type"))
	}

	complete := &orgctx.Scope{ID: 1, Features: features.AllStrings()}
	if rr := serve(complete, RequireFeature(features.PublicAPI)); rr.Code != http.StatusNoContent {
		t.Fatalf("complete plan: status %d", rr.Code)
	}

	// Any of several features (the channel manager).
	obiletOnly := &orgctx.Scope{ID: 1, Features: []string{"channel.obilet"}}
	if rr := serve(obiletOnly, RequireAnyFeature(features.ChannelYolcu360, features.ChannelObilet)); rr.Code != http.StatusNoContent {
		t.Fatalf("any feature: status %d", rr.Code)
	}
	if rr := serve(management, RequireAnyFeature(features.ChannelYolcu360, features.ChannelObilet)); rr.Code != http.StatusForbidden ||
		errorCode(t, rr) != response.CodeFeatureNotEnabled {
		t.Fatalf("no channel feature: %d %s", rr.Code, rr.Body.String())
	}

	// A scope built without a feature source is not gated.
	if rr := serve(&orgctx.Scope{ID: 1}, RequireFeature(features.SMS)); rr.Code != http.StatusNoContent {
		t.Fatalf("scope without features: status %d", rr.Code)
	}
	// Route wiring bug: no organization scope.
	if rr := serve(nil, RequireFeature(features.SMS)); rr.Code != http.StatusInternalServerError {
		t.Fatalf("without RequireOrganization: status %d", rr.Code)
	}
}
