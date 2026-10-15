package rentals

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	contractsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts"
	contractshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/handler"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
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

// The permission triad of the template roles (migration 000084): every
// route answers 403 without its slug and reaches the handler with it.
func TestRentalAndContractPermissionGates(t *testing.T) {
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
		// teslim_iade: rentals read / write, contracts read, downloads.
		"delivery": {ID: 1, Slug: "delivery", Permissions: []string{
			rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantContractsRead, rbac.PermTenantDocumentDownload,
		}},
		// rezervasyon_satis / muhasebe: read.
		"sales": {ID: 2, Slug: "sales", Permissions: []string{rbac.PermTenantRentalsRead, rbac.PermTenantContractsRead}},
		// sube_yoneticisi: everything.
		"manager": {ID: 3, Slug: "manager", Permissions: []string{
			rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose, rbac.PermTenantContractsRead,
			rbac.PermTenantContractsWrite, rbac.PermTenantContractsActivate, rbac.PermTenantDocumentDownload,
		}},
		"member": {ID: 4, Slug: "member", Permissions: []string{rbac.PermTenantRentalsRead}},
	}
	exported := 0
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, nil, nil), nil), EngineHandlers{Export: func(w http.ResponseWriter, _ *http.Request) {
		exported++
		w.WriteHeader(http.StatusAccepted)
	}}, tokens, stubLoader{}, resolver)
	contractsmodule.RegisterRoutes(mux, contractshandler.New(contractsusecase.New(nil, nil, nil, nil), nil), tokens, stubLoader{}, resolver)
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
	r := "/v1/tenant/rentals/"
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/rentals", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/rentals", "other", "", http.StatusForbidden},
		{"member cannot create", http.MethodPost, "/v1/tenant/rentals", "member", "{}", http.StatusForbidden},
		{"sales cannot hand over", http.MethodPost, r + id + "/handover", "sales", "{}", http.StatusForbidden},
		{"delivery cannot finish the check", http.MethodPost, r + id + "/complete-return", "delivery", "", http.StatusForbidden},
		{"delivery cannot cancel", http.MethodPost, r + id + "/cancel", "delivery", `{"reason":"x"}`, http.StatusForbidden},
		{"sales cannot work tasks", http.MethodPost, "/v1/tenant/operation-tasks/" + id + "/start", "sales", "", http.StatusForbidden},
		{"member cannot read contracts", http.MethodGet, "/v1/tenant/contract-templates", "member", "", http.StatusForbidden},
		{"member cannot read documents", http.MethodGet, "/v1/tenant/documents", "member", "", http.StatusForbidden},
		{"delivery cannot edit templates", http.MethodPost, "/v1/tenant/contract-templates", "delivery", "{}", http.StatusForbidden},
		{"delivery cannot activate", http.MethodPost, "/v1/tenant/contract-templates/" + id + "/activate", "delivery", "", http.StatusForbidden},
		{"sales cannot download", http.MethodGet, "/v1/tenant/documents/" + id + "/versions/" + id + "/pdf", "sales", "", http.StatusForbidden},
		{"member reads meta", http.MethodGet, r + "meta", "member", "", http.StatusOK},
		{"member reads task meta", http.MethodGet, "/v1/tenant/operation-tasks/meta", "member", "", http.StatusOK},
		{"sales reads placeholders", http.MethodGet, "/v1/tenant/contract-templates/placeholders", "sales", "", http.StatusOK},
		{"delivery passes the write gate", http.MethodPost, "/v1/tenant/rentals", "delivery", "{not json", http.StatusBadRequest},
		{"manager passes the close gate", http.MethodPost, r + "not-a-uuid/complete-return", "manager", "", http.StatusBadRequest},
		{"manager passes the activate gate", http.MethodPost, "/v1/tenant/contract-templates/not-a-uuid/activate", "manager", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
	if exported != 0 {
		t.Fatal("a refused export reached the engine")
	}
	if got := do(http.MethodPost, r+"export", "sales", `{"format":"csv"}`); got != http.StatusAccepted || exported != 1 {
		t.Fatalf("export by sales = %d", got)
	}
}

func TestLiveResources(t *testing.T) {
	reg := resourceevents.NewRegistry()
	RegisterLiveResources(reg)
	for resource, perm := range map[string]string{
		model.ResourceRentals:        rbac.PermTenantRentalsRead,
		model.ResourceOperationTasks: rbac.PermTenantRentalsRead,
	} {
		if got, ok := reg.Permission(resource); !ok || got != perm {
			t.Errorf("%s: %q %v, want %q", resource, got, ok, perm)
		}
	}
}
