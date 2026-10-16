package kabis

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
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
	svc, err := usecase.New(nil, nil, cipher, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler.New(svc, nil)
}

func TestKabisPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := rbac.PermTenantKabisRead
	resolver := stubResolver{
		"reader-org":   {ID: 2, Slug: "reader-org", MemberRole: "member", Permissions: []string{read}},
		"retry-org":    {ID: 3, Slug: "retry-org", MemberRole: "sube_yoneticisi", Permissions: []string{read, rbac.PermTenantKabisRetry}},
		"reconcile-or": {ID: 4, Slug: "reconcile-or", MemberRole: "admin", Permissions: []string{read, rbac.PermTenantKabisReconcile}},
		"rentals-org":  {ID: 5, Slug: "rentals-org", MemberRole: "member", Permissions: []string{rbac.PermTenantRentalsRead, rbac.PermTenantIntegrationsManage}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, newHandler(t), tokens, stubLoader{}, resolver)
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
	n := "/v1/tenant/kabis-notifications/" + uuid.NewString()
	cases := []struct {
		name, method, path, org string
		want                    int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/kabis-notifications", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/kabis-notifications", "other-org", http.StatusForbidden},
		{"rentals / integrations permissions are not KABİS read", http.MethodGet, "/v1/tenant/kabis-notifications", "rentals-org", http.StatusForbidden},
		{"no facets without read", http.MethodGet, "/v1/tenant/kabis-notifications/facets", "rentals-org", http.StatusForbidden},
		{"reader cannot retry", http.MethodPost, n + "/retry", "reader-org", http.StatusForbidden},
		{"reader cannot mark manual", http.MethodPost, n + "/mark-manual", "reader-org", http.StatusForbidden},
		{"reader cannot reconcile", http.MethodPost, n + "/reconcile", "reader-org", http.StatusForbidden},
		{"retry permission cannot reconcile", http.MethodPost, n + "/reconcile", "retry-org", http.StatusForbidden},
		{"retry permission cannot mark manual", http.MethodPost, n + "/mark-manual", "retry-org", http.StatusForbidden},
		{"reconcile permission cannot retry", http.MethodPost, n + "/retry", "reconcile-or", http.StatusForbidden},
		{"retry passes its gate", http.MethodPost, "/v1/tenant/kabis-notifications/nope/retry", "retry-org", http.StatusBadRequest},
		{"reconcile passes its gate", http.MethodPost, "/v1/tenant/kabis-notifications/nope/reconcile", "reconcile-or", http.StatusBadRequest},
		{"bad uuid", http.MethodGet, "/v1/tenant/kabis-notifications/nope", "reader-org", http.StatusBadRequest},
		{"reader reads the meta", http.MethodGet, "/v1/tenant/kabis-notifications/meta", "reader-org", http.StatusOK},
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
	RegisterRoutes(mux, newHandler(t), nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/kabis-notifications", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}

func TestLiveResource(t *testing.T) {
	t.Parallel()
	reg := resourceevents.NewRegistry()
	RegisterLiveResources(reg)
	if perm, ok := reg.Permission(Resource); !ok || perm != rbac.PermTenantKabisRead {
		t.Fatalf("live resource %q %v", perm, ok)
	}
}

// The official slot is always there and never ready (TEC-682); the fake is
// selectable outside production only.
func TestProvidersPerEnvironment(t *testing.T) {
	t.Parallel()
	prod := Providers("production", nil)
	if len(prod) != 1 || prod[0].Provider() != kabis.ProviderOfficial || prod[0].Ready() == nil {
		t.Fatalf("production providers %+v", prod)
	}
	if err := prod[0].Check(context.Background(), integrations.Account{}); err == nil {
		t.Fatal("the official slot must report waiting on dependency")
	}
	dev := Providers("development", nil)
	if len(dev) != 2 || dev[1].Provider() != "kabis_fake" || dev[1].Ready() != nil {
		t.Fatalf("development providers %+v", dev)
	}
}
