package tolls

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-674: without the hgs feature the toll passes, settings, tags and the
// export stay readable; fetching, importing, reviewing and tag changes
// answer 403 FEATURE_NOT_ENABLED.
func TestTollFeatureGateKeepsHistoryReadable(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	perms := []string{
		rbac.PermTenantTollsRead, rbac.PermTenantTollsExport, rbac.PermTenantTollsImport, rbac.PermTenantTollsReview,
		rbac.PermTenantVehiclesWrite, rbac.PermTenantSettingsWrite,
	}
	resolver := stubResolver{
		"yonetim": {ID: 1, Slug: "yonetim", Permissions: perms, Features: []string{}},
		"komple":  {ID: 2, Slug: "komple", Permissions: perms, Features: []string{"hgs"}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, nil, nil), nil), EngineHandlers{Export: ok, Import: ok, Sample: ok},
		tokens, stubLoader{}, resolver)
	do := func(method, path, org string) (int, string) {
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env response.Envelope
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		code := ""
		if env.Error != nil {
			code = env.Error.Code
		}
		return rr.Code, code
	}
	tags := "/v1/tenant/vehicles/nope/toll-tags"
	reads := []struct{ method, path string }{
		{http.MethodGet, "/v1/tenant/toll-passes/nope"},
		{http.MethodGet, tags},
		{http.MethodPost, "/v1/tenant/toll-passes/export"},
		{http.MethodGet, "/v1/tenant/toll-passes/import/sample"},
	}
	for _, r := range reads {
		if got, code := do(r.method, r.path, "yonetim"); got == http.StatusForbidden {
			t.Errorf("%s %s without the feature = %d %s", r.method, r.path, got, code)
		}
	}
	writes := []struct{ method, path string }{
		{http.MethodPost, "/v1/tenant/toll-passes/fetch"},
		{http.MethodPost, "/v1/tenant/toll-passes/import"},
		{http.MethodPost, "/v1/tenant/toll-passes/nope/assign"},
		{http.MethodPatch, "/v1/tenant/toll-settings"},
		{http.MethodPost, tags},
	}
	for _, w := range writes {
		got, code := do(w.method, w.path, "yonetim")
		if got != http.StatusForbidden || code != response.CodeFeatureNotEnabled {
			t.Errorf("%s %s without the feature = %d %s, want 403 FEATURE_NOT_ENABLED", w.method, w.path, got, code)
		}
	}
	// With the feature the same writes pass the gate (an invalid uuid then
	// answers 400 from the handler; the use case is never reached here).
	for _, path := range []string{"/v1/tenant/toll-passes/nope/assign", tags} {
		if got, code := do(http.MethodPost, path, "komple"); got != http.StatusBadRequest {
			t.Errorf("POST %s with the feature = %d %s, want 400", path, got, code)
		}
	}
}
