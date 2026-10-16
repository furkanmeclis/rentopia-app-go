package profitability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
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

// TestProfitabilityPermissionGates: reading needs tenant.finance.reports.read,
// exporting also tenant.report.export; a platform super admin without a
// membership gets nothing; filters are validated before any query.
func TestProfitabilityPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	resolver := stubResolver{
		"reader-org":   {ID: 1, Slug: "reader-org", MemberRole: "sube_yoneticisi", Permissions: []string{rbac.PermTenantFinanceReportsRead}},
		"exporter-org": {ID: 2, Slug: "exporter-org", MemberRole: "muhasebe", Permissions: []string{rbac.PermTenantFinanceReportsRead, rbac.PermTenantReportExport}},
		"sales-org":    {ID: 3, Slug: "sales-org", MemberRole: "rezervasyon_satis", Permissions: []string{rbac.PermTenantVehiclesRead, rbac.PermTenantReportExport}},
	}
	exported := 0
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil)), func(w http.ResponseWriter, _ *http.Request) {
		exported++
		w.WriteHeader(http.StatusAccepted)
	}, tokens, stubLoader{}, resolver)

	do := func(method, path, org string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr.Code
	}
	const base = "/v1/tenant/reports/vehicle-profitability"
	vehicle := base + "/vehicles/" + uuid.NewString()
	for _, c := range []struct {
		method, path, org string
		want              int
	}{
		{http.MethodGet, base, "", http.StatusBadRequest},
		{http.MethodGet, base, "unknown-org", http.StatusForbidden},
		{http.MethodGet, base, "sales-org", http.StatusForbidden},
		{http.MethodGet, vehicle, "sales-org", http.StatusForbidden},
		{http.MethodPost, base + "/export", "sales-org", http.StatusForbidden},
		{http.MethodPost, base + "/export", "reader-org", http.StatusForbidden},
		{http.MethodPost, base + "/export", "exporter-org", http.StatusAccepted},
		// Validation answers before the (nil) database is touched.
		{http.MethodGet, base + "?basis=invoice", "reader-org", http.StatusBadRequest},
		{http.MethodGet, base + "?allocation=km", "reader-org", http.StatusBadRequest},
		{http.MethodGet, base + "?from=03.01.2026", "reader-org", http.StatusBadRequest},
		{http.MethodGet, base + "?branch=x", "reader-org", http.StatusBadRequest},
		{http.MethodGet, base + "/vehicles/not-a-uuid", "reader-org", http.StatusNotFound},
	} {
		if got := do(c.method, c.path, c.org); got != c.want {
			t.Errorf("%s %s (%s) = %d, want %d", c.method, c.path, c.org, got, c.want)
		}
	}
	if exported != 1 {
		t.Errorf("export handler ran %d times", exported)
	}

	// Tenancy off: nothing is mounted.
	off := http.NewServeMux()
	RegisterRoutes(off, handler.New(usecase.New(nil)), nil, tokens, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	off.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, base, nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("tenancy off = %d", rr.Code)
	}
}
