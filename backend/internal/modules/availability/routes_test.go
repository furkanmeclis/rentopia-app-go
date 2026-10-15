package availability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	availabilityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
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

// TestAvailabilityPermissionGates checks the route guards with the template
// role grants of migration 000065 (requests that pass a gate stop at the
// body / path validation, before the service).
func TestAvailabilityPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	all := []string{
		rbac.PermTenantAvailabilityRead, rbac.PermTenantAvailabilityHold,
		rbac.PermTenantCapacityRulesRead, rbac.PermTenantCapacityRulesWrite,
	}
	read := []string{rbac.PermTenantAvailabilityRead, rbac.PermTenantCapacityRulesRead}
	resolver := stubResolver{
		"owner-org":    {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: all},
		"sales-org":    {ID: 2, Slug: "sales-org", MemberRole: "rezervasyon_satis", Permissions: append([]string{rbac.PermTenantAvailabilityHold}, read...)},
		"account-org":  {ID: 3, Slug: "account-org", MemberRole: "muhasebe", Permissions: read},
		"vehicles-org": {ID: 4, Slug: "vehicles-org", MemberRole: "member", Permissions: []string{rbac.PermTenantVehiclesRead}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, availabilityhandler.New(nil, nil), tokens, stubLoader{}, resolver)
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
	holds := "/v1/tenant/availability/holds"
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodPost, "/v1/tenant/availability/search", "", "{}", http.StatusBadRequest},
		{"not a member", http.MethodPost, "/v1/tenant/availability/search", "other-org", "{}", http.StatusForbidden},
		{"vehicles read is not availability read", http.MethodPost, "/v1/tenant/availability/search", "vehicles-org", "{}", http.StatusForbidden},
		{"vehicles read cannot list holds", http.MethodGet, holds, "vehicles-org", "", http.StatusForbidden},
		{"accounting passes the search gate", http.MethodPost, "/v1/tenant/availability/search", "account-org", "{not json", http.StatusBadRequest},
		{"accounting cannot put a hold", http.MethodPost, holds, "account-org", "{}", http.StatusForbidden},
		{"accounting cannot release a hold", http.MethodDelete, holds + "/" + id, "account-org", "", http.StatusForbidden},
		{"accounting cannot extend a hold", http.MethodPost, holds + "/" + id + "/extend", "account-org", "{}", http.StatusForbidden},
		{"sales passes the hold gate", http.MethodPost, holds, "sales-org", "{not json", http.StatusBadRequest},
		{"sales passes the release gate", http.MethodDelete, holds + "/not-a-uuid", "sales-org", "", http.StatusBadRequest},
		{"sales cannot write capacity rules", http.MethodPost, "/v1/tenant/capacity-rules", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot delete capacity rules", http.MethodDelete, "/v1/tenant/capacity-rules/" + id, "sales-org", "", http.StatusForbidden},
		{"vehicles read cannot read rules", http.MethodGet, "/v1/tenant/capacity-rules/" + id, "vehicles-org", "", http.StatusForbidden},
		{"owner passes the rule write gate", http.MethodPost, "/v1/tenant/capacity-rules", "owner-org", "{not json", http.StatusBadRequest},
		{"unknown body fields are rejected", http.MethodPost, "/v1/tenant/availability/search", "owner-org", `{"stock":3}`, http.StatusBadRequest},
		{"bad uuid", http.MethodGet, holds + "/not-a-uuid", "owner-org", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestNilResolverMountsNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, availabilityhandler.New(nil, nil), nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/tenant/availability/search", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}
