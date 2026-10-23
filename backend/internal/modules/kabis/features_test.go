package kabis

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-674: on a plan without KABİS the notification history stays
// readable (and can be closed out by hand) while a retry, which sends,
// answers 403 FEATURE_NOT_ENABLED.
func TestKabisFeatureGateKeepsHistoryReadable(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	perms := []string{rbac.PermTenantKabisRead, rbac.PermTenantKabisRetry, rbac.PermTenantKabisReconcile}
	resolver := stubResolver{
		"yonetim": {ID: 1, Slug: "yonetim", Permissions: perms, Features: []string{}},
		"komple":  {ID: 2, Slug: "komple", Permissions: perms, Features: []string{"kabis"}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, newHandler(t), tokens, stubLoader{}, resolver)
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
	// Reads pass every gate (an invalid uuid answers 400 from the handler).
	for _, path := range []string{"/v1/tenant/kabis-notifications/meta", "/v1/tenant/kabis-notifications/nope"} {
		if got, code := do(http.MethodGet, path, "yonetim"); got == http.StatusForbidden {
			t.Errorf("GET %s without the feature = %d %s", path, got, code)
		}
	}
	if got, _ := do(http.MethodPost, "/v1/tenant/kabis-notifications/nope/mark-manual", "yonetim"); got != http.StatusBadRequest {
		t.Errorf("mark-manual without the feature = %d, want 400 (past the gates)", got)
	}
	got, code := do(http.MethodPost, "/v1/tenant/kabis-notifications/nope/retry", "yonetim")
	if got != http.StatusForbidden || code != response.CodeFeatureNotEnabled {
		t.Errorf("retry without the feature = %d %s, want 403 FEATURE_NOT_ENABLED", got, code)
	}
	if got, _ := do(http.MethodPost, "/v1/tenant/kabis-notifications/nope/retry", "komple"); got != http.StatusBadRequest {
		t.Errorf("retry with the feature = %d, want 400 (past the gates)", got)
	}
}
