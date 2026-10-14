package vehicles

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	vehiclehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
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

func ok(_ http.ResponseWriter, _ *http.Request) {}

func TestVehiclePermissionGates(t *testing.T) {
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
		rbac.PermTenantVehiclesRead, rbac.PermTenantVehiclesWrite, rbac.PermTenantVehiclesExport,
		rbac.PermTenantVehiclesImport, rbac.PermTenantVehiclesBulk,
	}
	resolver := stubResolver{
		"owner-org":   {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: all},
		"sales-org":   {ID: 2, Slug: "sales-org", MemberRole: "rezervasyon_satis", Permissions: []string{rbac.PermTenantVehiclesRead}},
		"manager-org": {ID: 3, Slug: "manager-org", MemberRole: "sube_yoneticisi", Permissions: all[:3]},
		"fleet-only":  {ID: 4, Slug: "fleet-only", MemberRole: "member", Permissions: []string{rbac.PermTenantFleetRead}},
	}
	mux := http.NewServeMux()
	// The use case is never reached in these cases (gates or body decoding
	// fail first); engine routes use a no-op handler.
	RegisterRoutes(mux, vehiclehandler.New(usecase.New(nil, nil), nil), EngineHandlers{
		Export: ok, Import: ok, Sample: ok, Bulk: ok,
	}, tokens, stubLoader{}, resolver)

	do := func(method, path, org, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}
	id := uuid.NewString()
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/vehicles", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/vehicles", "other-org", "", http.StatusForbidden},
		{"fleet read is not vehicles read", http.MethodGet, "/v1/tenant/vehicles/meta", "fleet-only", "", http.StatusForbidden},
		{"no vehicles read on product vehicles", http.MethodGet, "/v1/tenant/products/" + id + "/eligible-vehicles", "fleet-only", "", http.StatusForbidden},
		{"sales reads meta", http.MethodGet, "/v1/tenant/vehicles/meta", "sales-org", "", http.StatusOK},
		{"sales cannot create", http.MethodPost, "/v1/tenant/vehicles", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot patch", http.MethodPatch, "/v1/tenant/vehicles/" + id, "sales-org", "{}", http.StatusForbidden},
		{"sales cannot delete", http.MethodDelete, "/v1/tenant/vehicles/" + id, "sales-org", "", http.StatusForbidden},
		{"sales cannot change status", http.MethodPost, "/v1/tenant/vehicles/" + id + "/status", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot add a group", http.MethodPost, "/v1/tenant/vehicles/" + id + "/rental-groups", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot remove a group", http.MethodDelete, "/v1/tenant/vehicles/" + id + "/rental-groups/" + id, "sales-org", "", http.StatusForbidden},
		{"sales cannot export", http.MethodPost, "/v1/tenant/vehicles/export", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot import", http.MethodPost, "/v1/tenant/vehicles/import", "sales-org", "", http.StatusForbidden},
		{"sales reaches bulk (action permission checked there)", http.MethodPost, "/v1/tenant/vehicles/bulk", "sales-org", "{}", http.StatusOK},
		{"manager exports", http.MethodPost, "/v1/tenant/vehicles/export", "manager-org", "{}", http.StatusOK},
		{"manager cannot import", http.MethodPost, "/v1/tenant/vehicles/import", "manager-org", "", http.StatusForbidden},
		{"manager cannot download the sample", http.MethodGet, "/v1/tenant/vehicles/import/sample", "manager-org", "", http.StatusForbidden},
		{"owner imports", http.MethodPost, "/v1/tenant/vehicles/import", "owner-org", "", http.StatusOK},
		{"owner passes the write gate", http.MethodPost, "/v1/tenant/vehicles", "owner-org", "{not json", http.StatusBadRequest},
		{"unknown body fields are rejected", http.MethodPost, "/v1/tenant/vehicles", "owner-org", `{"stock":3}`, http.StatusBadRequest},
		{"bad uuid", http.MethodGet, "/v1/tenant/vehicles/not-a-uuid", "owner-org", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body).Code; got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}

	// Meta advertises the bulk actions behind tenant.vehicles.bulk; "Bakıma
	// al" (start_maintenance) is behind tenant.blocks.write.
	rr := do(http.MethodGet, "/v1/tenant/vehicles/meta", "sales-org", "")
	var env struct {
		Data struct {
			Resource    string `json:"resource"`
			BulkActions []struct {
				ID         string `json:"id"`
				Permission string `json:"permission"`
			} `json:"bulk_actions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Resource != Resource || len(env.Data.BulkActions) != 7 {
		t.Fatalf("meta = %+v", env.Data)
	}
	for _, a := range env.Data.BulkActions {
		want := rbac.PermTenantVehiclesBulk
		if a.ID == "start_maintenance" {
			want = rbac.PermTenantBlocksWrite
		}
		if a.Permission != want {
			t.Errorf("bulk action %s permission = %s", a.ID, a.Permission)
		}
	}
}

func TestNilResolverMountsNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, vehiclehandler.New(usecase.New(nil, nil), nil), EngineHandlers{}, nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/vehicles", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}
