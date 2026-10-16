package invoices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
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

func newHandler(t *testing.T) *handler.Handler {
	t.Helper()
	cipher, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := usecase.New(nil, nil, cipher, nil, nil, nil, integrations.NewOperationRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return handler.New(svc, nil)
}

func TestInvoicePermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := rbac.PermTenantInvoicesRead
	resolver := stubResolver{
		"reader-org":  {ID: 2, Slug: "reader-org", MemberRole: "member", Permissions: []string{read}},
		"writer-org":  {ID: 3, Slug: "writer-org", MemberRole: "sube_yoneticisi", Permissions: []string{read, rbac.PermTenantInvoicesWrite}},
		"cancel-org":  {ID: 4, Slug: "cancel-org", MemberRole: "muhasebe", Permissions: []string{read, rbac.PermTenantInvoicesCancel}},
		"finance-org": {ID: 5, Slug: "finance-org", MemberRole: "member", Permissions: []string{rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite}},
	}
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }
	RegisterRoutes(mux, newHandler(t), Engines{Export: ok, Bulk: ok}, tokens, stubLoader{}, resolver)
	do := func(method, path, org string) int {
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr.Code
	}
	n := "/v1/tenant/invoices/" + uuid.NewString()
	cases := []struct {
		name, method, path, org string
		want                    int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/invoices", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/invoices", "other-org", http.StatusForbidden},
		{"finance permissions are not invoice read", http.MethodGet, "/v1/tenant/invoices", "finance-org", http.StatusForbidden},
		{"reader cannot create", http.MethodPost, "/v1/tenant/invoices", "reader-org", http.StatusForbidden},
		{"reader cannot edit", http.MethodPatch, n, "reader-org", http.StatusForbidden},
		{"reader cannot finalize", http.MethodPost, n + "/finalize", "reader-org", http.StatusForbidden},
		{"reader cannot retry", http.MethodPost, n + "/retry", "reader-org", http.StatusForbidden},
		{"writer cannot cancel", http.MethodPost, n + "/cancel", "writer-org", http.StatusForbidden},
		{"writer cannot return", http.MethodPost, n + "/return", "writer-org", http.StatusForbidden},
		{"cancel permission cannot finalize", http.MethodPost, n + "/finalize", "cancel-org", http.StatusForbidden},
		{"writer cannot change settings", http.MethodPut, "/v1/tenant/invoices/settings", "writer-org", http.StatusForbidden},
		{"reader cannot export", http.MethodPost, "/v1/tenant/invoices/export", "reader-org", http.StatusForbidden},
		{"bulk reaches the engine (it checks the action)", http.MethodPost, "/v1/tenant/invoices/bulk", "reader-org", http.StatusAccepted},
		{"finalize passes its gate", http.MethodPost, "/v1/tenant/invoices/nope/finalize", "writer-org", http.StatusBadRequest},
		{"cancel passes its gate", http.MethodPost, "/v1/tenant/invoices/nope/cancel", "cancel-org", http.StatusBadRequest},
		{"bad uuid", http.MethodGet, "/v1/tenant/invoices/nope", "reader-org", http.StatusBadRequest},
		{"reader reads the meta", http.MethodGet, "/v1/tenant/invoices/meta", "reader-org", http.StatusOK},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestNilResolverMountsNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, newHandler(t), Engines{}, nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/invoices", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}

func TestLiveResource(t *testing.T) {
	t.Parallel()
	reg := resourceevents.NewRegistry()
	RegisterLiveResources(reg)
	if perm, ok := reg.Permission(Resource); !ok || perm != rbac.PermTenantInvoicesRead {
		t.Fatalf("live resource %q %v", perm, ok)
	}
}

// The integrator slot is always there and never ready (TEC-685); the fake
// is selectable outside production only.
func TestProvidersPerEnvironment(t *testing.T) {
	t.Parallel()
	prod := Providers("production", nil)
	if len(prod) != 1 || prod[0].Provider() != einvoice.ProviderOfficial || prod[0].Ready() == nil {
		t.Fatalf("production providers %+v", prod)
	}
	if err := prod[0].Check(context.Background(), integrations.Account{}); err == nil {
		t.Fatal("the official slot must report waiting on dependency")
	}
	dev := Providers("development", nil)
	if len(dev) != 2 || dev[1].Provider() != "einvoice_fake" || dev[1].Ready() != nil {
		t.Fatalf("development providers %+v", dev)
	}
}
