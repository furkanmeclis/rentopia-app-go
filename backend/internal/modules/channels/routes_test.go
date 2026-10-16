package channels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
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
	svc, err := usecase.New(nil, nil, cipher, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler.New(svc, nil)
}

func TestChannelPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := rbac.PermTenantChannelsRead
	resolver := stubResolver{
		"reader-org":  {ID: 2, Slug: "reader-org", MemberRole: "member", Permissions: []string{read}},
		"writer-org":  {ID: 3, Slug: "writer-org", MemberRole: "admin", Permissions: []string{read, rbac.PermTenantChannelsWrite}},
		"sync-org":    {ID: 4, Slug: "sync-org", MemberRole: "rezervasyon_satis", Permissions: []string{read, rbac.PermTenantChannelsSync}},
		"integration": {ID: 5, Slug: "integration", MemberRole: "member", Permissions: []string{rbac.PermTenantIntegrationsManage}},
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
	acc := "/v1/tenant/channel-accounts/" + uuid.NewString()
	mp := "/v1/tenant/channel-mappings/" + uuid.NewString()
	msg := "/v1/tenant/channel-messages/" + uuid.NewString()
	cases := []struct {
		name, method, path, org string
		want                    int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/channel-accounts", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/channel-accounts", "other-org", http.StatusForbidden},
		{"integrations permission is not channel read", http.MethodGet, "/v1/tenant/channel-mappings", "integration", http.StatusForbidden},
		{"no message facets without read", http.MethodGet, "/v1/tenant/channel-messages/facets", "integration", http.StatusForbidden},
		{"reader cannot change settings", http.MethodPatch, acc, "reader-org", http.StatusForbidden},
		{"reader cannot push", http.MethodPost, acc + "/push", "reader-org", http.StatusForbidden},
		{"reader cannot pull", http.MethodPost, acc + "/pull", "reader-org", http.StatusForbidden},
		{"reader cannot simulate", http.MethodPost, acc + "/simulate", "reader-org", http.StatusForbidden},
		{"reader cannot create a mapping", http.MethodPost, "/v1/tenant/channel-mappings", "reader-org", http.StatusForbidden},
		{"reader cannot delete a mapping", http.MethodDelete, mp, "reader-org", http.StatusForbidden},
		{"reader cannot retry", http.MethodPost, msg + "/retry", "reader-org", http.StatusForbidden},
		{"writer cannot retry", http.MethodPost, msg + "/retry", "writer-org", http.StatusForbidden},
		{"writer cannot push", http.MethodPost, acc + "/push", "writer-org", http.StatusForbidden},
		{"sync cannot edit a mapping", http.MethodPatch, mp, "sync-org", http.StatusForbidden},
		{"sync cannot change settings", http.MethodPatch, acc, "sync-org", http.StatusForbidden},
		{"sync passes its gate", http.MethodPost, "/v1/tenant/channel-messages/nope/ignore", "sync-org", http.StatusBadRequest},
		{"writer passes its gate", http.MethodPatch, "/v1/tenant/channel-mappings/nope", "writer-org", http.StatusBadRequest},
		{"bad uuid", http.MethodGet, "/v1/tenant/channel-accounts/nope", "reader-org", http.StatusBadRequest},
		{"bad list filter", http.MethodGet, "/v1/tenant/channel-messages?status=nope", "reader-org", http.StatusBadRequest},
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
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/channel-accounts", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}

func TestLiveResources(t *testing.T) {
	t.Parallel()
	reg := resourceevents.NewRegistry()
	RegisterLiveResources(reg)
	for _, r := range []string{model.ResourceAccounts, model.ResourceMappings, model.ResourceMessages} {
		if perm, ok := reg.Permission(r); !ok || perm != rbac.PermTenantChannelsRead {
			t.Fatalf("live resource %s: %q %v", r, perm, ok)
		}
	}
}

// Yolcu360 is always registered; the fake outside production only.
func TestProvidersPerEnvironment(t *testing.T) {
	t.Parallel()
	prod := Providers("production", nil)
	if len(prod) != 1 || prod[0].Provider() != "yolcu360" || prod[0].Channel() != "yolcu360" {
		t.Fatalf("production providers %+v", prod)
	}
	dev := Providers("development", nil)
	if len(dev) != 2 || dev[1].Provider() != "yolcu360_fake" || dev[1].Channel() != "yolcu360" {
		t.Fatalf("development providers %+v", dev)
	}
}
