package reports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
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

// TestReportPermissionGates: reading needs tenant.finance.reports.read,
// exporting also tenant.report.export; a platform super admin without a
// membership gets nothing; filters are validated before any query.
func TestReportPermissionGates(t *testing.T) {
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
	exported := map[string]int{}
	factory := func(resource string, _ ...string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			exported[resource]++
			w.WriteHeader(http.StatusAccepted)
		}
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil)), factory, tokens, stubLoader{}, resolver)
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
	type tc struct {
		method, path, org string
		want              int
	}
	cases := []tc{}
	for _, rep := range Reports {
		base := "/v1/tenant/reports/" + rep.Path
		cases = append(cases,
			tc{http.MethodGet, base, "", http.StatusBadRequest},
			tc{http.MethodGet, base, "unknown-org", http.StatusForbidden},
			tc{http.MethodGet, base, "sales-org", http.StatusForbidden},
			tc{http.MethodPost, base + "/export", "sales-org", http.StatusForbidden},
			tc{http.MethodPost, base + "/export", "reader-org", http.StatusForbidden},
			tc{http.MethodPost, base + "/export", "exporter-org", http.StatusAccepted},
			// Validation answers before the (nil) database is touched.
			tc{http.MethodGet, base + "?from=03.01.2026", "reader-org", http.StatusBadRequest},
			tc{http.MethodGet, base + "?branch=x", "reader-org", http.StatusBadRequest},
		)
	}
	cases = append(cases,
		tc{http.MethodGet, "/v1/tenant/reports/occupancy?group_by=category", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/occupancy?rental_group=x", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/income-expense?group_by=rental_group", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/income-expense?group_by=vehicle", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/channels?date_basis=return", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/aging?party=employee", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/aging?as_of=yesterday", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/branches?basis=invoice", "reader-org", http.StatusBadRequest},
		tc{http.MethodGet, "/v1/tenant/reports/branches?allocation=km", "reader-org", http.StatusBadRequest},
	)
	for _, c := range cases {
		if got := do(c.method, c.path, c.org); got != c.want {
			t.Errorf("%s %s (%s) = %d, want %d", c.method, c.path, c.org, got, c.want)
		}
	}
	for _, rep := range Reports {
		if exported[rep.Resource] != 1 {
			t.Errorf("export handler of %s ran %d times", rep.Resource, exported[rep.Resource])
		}
	}

	// Tenancy off: nothing is mounted.
	off := http.NewServeMux()
	RegisterRoutes(off, handler.New(usecase.New(nil, nil)), nil, tokens, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	off.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/reports/occupancy", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("tenancy off = %d", rr.Code)
	}
}
