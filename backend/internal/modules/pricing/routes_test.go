package pricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
)

type stubLoader struct{}

func (stubLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	// super_admin on the platform: tenant routes must still ignore it.
	return authctx.Principal{UserID: uuid.New(), UserInternal: 42, IsSuperAdmin: true}, nil
}

type stubResolver map[string]orgctx.Scope

func (s stubResolver) Resolve(_ context.Context, ref string, _ int64) (orgctx.Scope, error) {
	scope, ok := s[ref]
	if !ok {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	return scope, nil
}

func ok(_ http.ResponseWriter, _ *http.Request) {}

// TestPricingPermissionGates checks the route guards with the template role
// grants of migration 000062: read for branch managers / sales / accounting
// / member, write and activate separately, nothing for delivery staff.
func TestPricingPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := []string{rbac.PermTenantPricingRead}
	resolver := stubResolver{
		"owner-org":    {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: []string{rbac.PermTenantPricingRead, rbac.PermTenantPricingWrite, rbac.PermTenantPricingActivate}},
		"sales-org":    {ID: 2, Slug: "sales-org", MemberRole: "rezervasyon_satis", Permissions: read},
		"writer-org":   {ID: 3, Slug: "writer-org", MemberRole: "custom", Permissions: []string{rbac.PermTenantPricingRead, rbac.PermTenantPricingWrite}},
		"delivery-org": {ID: 4, Slug: "delivery-org", MemberRole: "teslim_iade", Permissions: []string{rbac.PermTenantOneWayRead}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, pricinghandler.New(usecase.New(nil, nil), nil), EngineHandlers{
		ExportPlans: ok, ImportPlans: ok, SamplePlans: ok, ExportRules: ok,
	}, tokens, stubLoader{}, resolver)

	do := func(method, path, org, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr.Code
	}
	id := uuid.NewString()
	plan := "/v1/tenant/rate-plans/" + id
	rule := "/v1/tenant/date-rules/" + id
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/rate-plans", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/rate-plans", "other-org", "", http.StatusForbidden},
		{"delivery staff cannot read prices", http.MethodGet, "/v1/tenant/rate-plans/meta", "delivery-org", "", http.StatusForbidden},
		{"delivery staff cannot preview", http.MethodPost, "/v1/tenant/pricing/preview", "delivery-org", "{}", http.StatusForbidden},
		{"sales reads plan meta", http.MethodGet, "/v1/tenant/rate-plans/meta", "sales-org", "", http.StatusOK},
		{"sales reads rule meta", http.MethodGet, "/v1/tenant/date-rules/meta", "sales-org", "", http.StatusOK},
		{"sales exports the price list", http.MethodPost, "/v1/tenant/rate-plans/export", "sales-org", "{}", http.StatusOK},
		{"sales cannot create a plan", http.MethodPost, "/v1/tenant/rate-plans", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot edit a plan", http.MethodPatch, plan, "sales-org", "{}", http.StatusForbidden},
		{"sales cannot save the matrix", http.MethodPut, "/v1/tenant/rate-plans/matrix", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot import", http.MethodPost, "/v1/tenant/rate-plans/import", "sales-org", "", http.StatusForbidden},
		{"sales cannot activate a plan", http.MethodPost, plan + "/activate", "sales-org", "", http.StatusForbidden},
		{"sales cannot create a rule", http.MethodPost, "/v1/tenant/date-rules", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot delete a rule", http.MethodDelete, rule, "sales-org", "", http.StatusForbidden},
		{"writer passes the plan write gate", http.MethodPost, "/v1/tenant/rate-plans", "writer-org", "{not json", http.StatusBadRequest},
		{"writer passes the matrix write gate", http.MethodPut, "/v1/tenant/rate-plans/matrix", "writer-org", "{not json", http.StatusBadRequest},
		{"writer imports", http.MethodPost, "/v1/tenant/rate-plans/import", "writer-org", "", http.StatusOK},
		{"writer cannot activate a plan", http.MethodPost, plan + "/activate", "writer-org", "", http.StatusForbidden},
		{"writer cannot deactivate a plan", http.MethodPost, plan + "/deactivate", "writer-org", "", http.StatusForbidden},
		{"writer cannot activate a matrix", http.MethodPost, "/v1/tenant/rate-plans/matrix/activate", "writer-org", "{}", http.StatusForbidden},
		{"writer cannot activate a rule", http.MethodPost, rule + "/activate", "writer-org", "", http.StatusForbidden},
		{"owner passes the activate gate", http.MethodPost, "/v1/tenant/rate-plans/matrix/activate", "owner-org", "{not json", http.StatusBadRequest},
		{"unknown body fields are rejected", http.MethodPost, "/v1/tenant/rate-plans", "owner-org", `{"stock":3}`, http.StatusBadRequest},
		{"bad uuid", http.MethodGet, "/v1/tenant/rate-plans/not-a-uuid", "owner-org", "", http.StatusBadRequest},
		{"bad matrix priority", http.MethodGet, "/v1/tenant/rate-plans/matrix?priority=x", "owner-org", "", http.StatusBadRequest},
		{"bad sort field", http.MethodGet, "/v1/tenant/date-rules?sort=stock", "owner-org", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestLiveResourcesNeedPricingRead(t *testing.T) {
	t.Parallel()
	reg := resourceevents.NewRegistry()
	RegisterLiveResources(reg)
	for _, res := range []string{ResourceRatePlans, ResourceDateRules} {
		perm, ok := reg.Permission(res)
		if !ok || perm != rbac.PermTenantPricingRead {
			t.Errorf("%s: permission = %q %v", res, perm, ok)
		}
	}
}
