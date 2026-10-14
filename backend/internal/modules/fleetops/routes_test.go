package fleetops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fleetopshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles"
	vehiclehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/handler"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
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

// TestFleetOpsPermissionGates checks the route guards with the template
// role grants of migration 000055.
func TestFleetOpsPermissionGates(t *testing.T) {
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
		rbac.PermTenantBlocksRead, rbac.PermTenantBlocksWrite, rbac.PermTenantTransfersRead,
		rbac.PermTenantTransfersWrite, rbac.PermTenantOneWayRead, rbac.PermTenantOneWayWrite,
	}
	resolver := stubResolver{
		"owner-org":    {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: all},
		"sales-org":    {ID: 2, Slug: "sales-org", MemberRole: "rezervasyon_satis", Permissions: []string{rbac.PermTenantBlocksRead, rbac.PermTenantTransfersRead, rbac.PermTenantOneWayRead}},
		"delivery-org": {ID: 3, Slug: "delivery-org", MemberRole: "teslim_iade", Permissions: all[:4]},
		"account-org":  {ID: 4, Slug: "account-org", MemberRole: "muhasebe", Permissions: []string{rbac.PermTenantTransfersRead, rbac.PermTenantOneWayRead}},
		"vehicles-org": {ID: 5, Slug: "vehicles-org", MemberRole: "member", Permissions: []string{rbac.PermTenantVehiclesRead, rbac.PermTenantVehiclesWrite}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, fleetopshandler.New(usecase.New(nil, nil), nil), EngineHandlers{
		ExportTransfers: ok, ExportOneWay: ok, BulkOneWay: ok,
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
	blocks := "/v1/tenant/vehicles/" + id + "/blocks"
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/transfers", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/one-way-rules", "other-org", "", http.StatusForbidden},
		{"vehicles write is not blocks write", http.MethodPost, blocks, "vehicles-org", "{}", http.StatusForbidden},
		{"vehicles read is not blocks read", http.MethodGet, blocks, "vehicles-org", "", http.StatusForbidden},
		{"sales cannot block", http.MethodPost, blocks, "sales-org", "{}", http.StatusForbidden},
		{"sales cannot release", http.MethodPost, blocks + "/" + id + "/release", "sales-org", "", http.StatusForbidden},
		{"sales cannot delete a block", http.MethodDelete, blocks + "/" + id, "sales-org", "", http.StatusForbidden},
		{"sales reads transfer meta", http.MethodGet, "/v1/tenant/transfers/meta", "sales-org", "", http.StatusOK},
		{"sales cannot create a transfer", http.MethodPost, "/v1/tenant/transfers", "sales-org", "{}", http.StatusForbidden},
		{"sales cannot start a transfer", http.MethodPost, "/v1/tenant/transfers/" + id + "/start", "sales-org", "", http.StatusForbidden},
		{"sales cannot list assignees", http.MethodGet, "/v1/tenant/transfers/assignees", "sales-org", "", http.StatusForbidden},
		{"sales cannot create a rule", http.MethodPost, "/v1/tenant/one-way-rules", "sales-org", "{}", http.StatusForbidden},
		{"sales exports rules", http.MethodPost, "/v1/tenant/one-way-rules/export", "sales-org", "{}", http.StatusOK},
		{"sales reaches rules bulk (action permission checked there)", http.MethodPost, "/v1/tenant/one-way-rules/bulk", "sales-org", "{}", http.StatusOK},
		{"delivery passes the block write gate", http.MethodPost, blocks, "delivery-org", "{not json", http.StatusBadRequest},
		{"delivery passes the transfer write gate", http.MethodPost, "/v1/tenant/transfers", "delivery-org", "{not json", http.StatusBadRequest},
		{"delivery cannot read rules", http.MethodGet, "/v1/tenant/one-way-rules/meta", "delivery-org", "", http.StatusForbidden},
		{"accounting reads rules", http.MethodGet, "/v1/tenant/one-way-rules/meta", "account-org", "", http.StatusOK},
		{"accounting exports transfers", http.MethodPost, "/v1/tenant/transfers/export", "account-org", "{}", http.StatusOK},
		{"accounting cannot read blocks", http.MethodGet, blocks, "account-org", "", http.StatusForbidden},
		{"accounting cannot complete a transfer", http.MethodPost, "/v1/tenant/transfers/" + id + "/complete", "account-org", "", http.StatusForbidden},
		{"owner passes the rule write gate", http.MethodPost, "/v1/tenant/one-way-rules", "owner-org", "{not json", http.StatusBadRequest},
		{"unknown body fields are rejected", http.MethodPost, "/v1/tenant/one-way-rules", "owner-org", `{"stock":3}`, http.StatusBadRequest},
		{"bad uuid", http.MethodGet, "/v1/tenant/transfers/not-a-uuid", "owner-org", "", http.StatusBadRequest},
		{"bad resolve query", http.MethodGet, "/v1/tenant/one-way-rules/resolve?from_branch=x", "owner-org", "", http.StatusBadRequest},
		// Fleet calendar: tenant.vehicles.read (blocks / transfers read alone is not enough).
		{"calendar needs vehicles read", http.MethodGet, "/v1/tenant/fleet/calendar", "account-org", "", http.StatusForbidden},
		{"calendar window is validated", http.MethodGet, "/v1/tenant/fleet/calendar", "vehicles-org", "", http.StatusBadRequest},
		{"calendar window too wide", http.MethodGet, "/v1/tenant/fleet/calendar?from=2026-01-01T00:00:00Z&to=2026-06-01T00:00:00Z", "vehicles-org", "", http.StatusBadRequest},
		{"calendar limit must be a number", http.MethodGet, "/v1/tenant/fleet/calendar?from=2026-01-01T00:00:00Z&to=2026-01-08T00:00:00Z&limit=x", "vehicles-org", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body).Code; got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}

	// Rule meta advertises activate / deactivate behind tenant.one_way.write.
	rr := do(http.MethodGet, "/v1/tenant/one-way-rules/meta", "sales-org", "")
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
	if env.Data.Resource != ResourceOneWayRules || len(env.Data.BulkActions) != 2 {
		t.Fatalf("meta = %+v", env.Data)
	}
	for _, a := range env.Data.BulkActions {
		if a.Permission != rbac.PermTenantOneWayWrite {
			t.Errorf("bulk action %s permission = %s", a.ID, a.Permission)
		}
	}
}

// TestRoutesCoexistWithVehicles mounts the vehicles and the block routes
// on one mux (the server does): the patterns must not conflict.
func TestRoutesCoexistWithVehicles(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	resolver := stubResolver{}
	vehicles.RegisterRoutes(mux, vehiclehandler.New(vehicleusecase.New(nil, nil), nil), vehicles.EngineHandlers{
		Export: ok, Import: ok, Sample: ok, Bulk: ok,
	}, tokens, stubLoader{}, resolver)
	RegisterRoutes(mux, fleetopshandler.New(usecase.New(nil, nil), nil), EngineHandlers{
		ExportTransfers: ok, ExportOneWay: ok, BulkOneWay: ok,
	}, tokens, stubLoader{}, resolver)
}

func TestNilResolverMountsNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, fleetopshandler.New(usecase.New(nil, nil), nil), EngineHandlers{}, nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/transfers", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}
