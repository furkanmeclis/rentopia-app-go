package whatsapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/whatsappfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/wuzapi"
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

func TestPermissionGates(t *testing.T) {
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
		"notify-org": {ID: 2, Slug: "notify-org", MemberRole: "admin", Permissions: []string{
			rbac.PermTenantNotificationSettingsRead, rbac.PermTenantNotificationSettingsWrite,
		}},
		"manage-org": {ID: 3, Slug: "manage-org", MemberRole: "admin", Permissions: []string{rbac.PermTenantIntegrationsManage}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, usecase.Config{})), tokens, stubLoader{}, resolver)
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
	routes := []struct{ method, path string }{
		{http.MethodGet, "/v1/tenant/whatsapp/session"},
		{http.MethodPatch, "/v1/tenant/whatsapp/session"},
		{http.MethodDelete, "/v1/tenant/whatsapp/session"},
		{http.MethodPost, "/v1/tenant/whatsapp/session/connect"},
		{http.MethodGet, "/v1/tenant/whatsapp/session/qr"},
		{http.MethodPost, "/v1/tenant/whatsapp/session/refresh"},
		{http.MethodPost, "/v1/tenant/whatsapp/session/logout"},
		{http.MethodPost, "/v1/tenant/whatsapp/test-messages"},
		{http.MethodGet, "/v1/tenant/whatsapp/messages"},
	}
	for _, r := range routes {
		if got := do(r.method, r.path, "", "{}"); got != http.StatusBadRequest {
			t.Errorf("%s %s without organization = %d", r.method, r.path, got)
		}
		if got := do(r.method, r.path, "other-org", "{}"); got != http.StatusForbidden {
			t.Errorf("%s %s not a member = %d", r.method, r.path, got)
		}
		if got := do(r.method, r.path, "notify-org", "{}"); got != http.StatusForbidden {
			t.Errorf("%s %s with notification settings only = %d", r.method, r.path, got)
		}
	}
	// Past the gate: validation before any database access.
	if got := do(http.MethodPost, "/v1/tenant/whatsapp/session/connect", "manage-org", `{"acknowledge_risk":false}`); got != http.StatusBadRequest {
		t.Errorf("connect without the risk notice = %d", got)
	}
	if got := do(http.MethodGet, "/v1/tenant/whatsapp/messages?sort=-phone", "manage-org", ""); got != http.StatusBadRequest {
		t.Errorf("unknown sort = %d", got)
	}
	// The webhook is public; a short token is unknown.
	if got := do(http.MethodPost, "/v1/public/whatsapp/webhook/x", "", "{}"); got != http.StatusNotFound {
		t.Errorf("webhook with an unknown token = %d", got)
	}
}

func TestNilResolverMountsNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, usecase.Config{})), nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/whatsapp/session", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}

// wuzapi is always registered (configured by the environment); the fake
// outside production only.
func TestProvidersPerEnvironment(t *testing.T) {
	t.Parallel()
	prod, fake := Providers(config.WhatsAppConfig{}, "production", nil)
	if len(prod) != 1 || prod[0].Provider() != wuzapi.ProviderCode || prod[0].Configured() || fake != nil {
		t.Fatalf("production providers %+v", prod)
	}
	dev, fake := Providers(config.WhatsAppConfig{WuzapiBaseURL: "http://wuzapi:8080", WuzapiAdminToken: "t"}, "development", nil)
	if len(dev) != 2 || !dev[0].Configured() || dev[1].Provider() != whatsappfake.ProviderCode || fake == nil || len(Adapters(dev)) != 2 {
		t.Fatalf("development providers %+v", dev)
	}
}
