package supportaccess_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// claimsLoader turns the token claims into a principal (no database).
type claimsLoader struct{}

func (claimsLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return authctx.Principal{}, err
	}
	p := authctx.Principal{UserID: id, UserInternal: 7, Roles: c.Roles, IsSuperAdmin: c.IsSuperAdmin}
	if c.ImpersonatorID != nil {
		imp, err := uuid.Parse(*c.ImpersonatorID)
		if err != nil {
			return authctx.Principal{}, err
		}
		p.ImpersonatorUserID = &imp
	}
	return p, nil
}

// memberResolver: the caller is an owner of organization 1 ("acme").
type memberResolver struct{}

func (memberResolver) Resolve(_ context.Context, ref string, _ int64) (orgctx.Scope, error) {
	if ref != "acme" {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	return orgctx.Scope{ID: 1, Slug: "acme", Status: orgctx.StatusActive, Permissions: []string{
		rbac.PermTenantSettingsRead, rbac.PermTenantSettingsWrite, rbac.PermTenantSupportAccessManage,
	}}, nil
}

// grants is a settable GrantLookup.
type grants struct{ g *orgctx.SupportGrant }

func (s *grants) ActiveGrant(context.Context, int64) (*orgctx.SupportGrant, error) { return s.g, nil }

// noRepo fails the test if a handler reaches persistence.
type noRepo struct{ usecase.Repository }

type env struct {
	t      *testing.T
	mux    *http.ServeMux
	tokens *jwt.Manager
	grants *grants
	seen   []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	tokens, err := jwt.NewManager("support-access-test-secret-0123456789", time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, mux: http.NewServeMux(), tokens: tokens, grants: &grants{}}
	resolver := supportaccess.GuardResolver(memberResolver{}, e.grants)
	authn := middleware.Authenticate(tokens, claimsLoader{})
	org := middleware.RequireOrganization(resolver)
	// Stand-ins for any tenant route (settings): reads and a change.
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope := orgctx.Must(r.Context())
		if scope.SupportGrant != nil {
			e.seen = append(e.seen, scope.SupportGrant.UUID.String())
		}
		w.WriteHeader(http.StatusOK)
	})
	e.mux.Handle("GET /v1/tenant/settings", middleware.Chain(ok, authn, org))
	e.mux.Handle("PUT /v1/tenant/settings", middleware.Chain(ok, authn, org))
	supportaccess.RegisterRoutes(e.mux, handler.New(usecase.New(noRepo{}), nil), tokens, claimsLoader{}, resolver)
	return e
}

func (e *env) do(method, path string, impersonated bool, body string) (int, string) {
	e.t.Helper()
	in := jwt.AccessInput{UserID: uuid.New(), Roles: []string{rbac.RoleSuperAdmin}, IsSuperAdmin: true}
	if impersonated {
		staff := uuid.New()
		in.ImpersonatorID = &staff
	}
	tok, _, err := e.tokens.IssueAccess(in)
	if err != nil {
		e.t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set(orgctx.HeaderName, "acme")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	e.mux.ServeHTTP(rr, req)
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &env)
	return rr.Code, env.Error.Code
}

func (e *env) expect(method, path string, impersonated bool, wantStatus int, wantCode string) {
	e.t.Helper()
	status, code := e.do(method, path, impersonated, `{}`)
	if status != wantStatus || code != wantCode {
		e.t.Errorf("%s %s (impersonated=%v) = %d %q, want %d %q", method, path, impersonated, status, code, wantStatus, wantCode)
	}
}

func TestImpersonatedTenantRequestsNeedAGrant(t *testing.T) {
	e := newEnv(t)

	// Members are not affected.
	e.expect(http.MethodGet, "/v1/tenant/settings", false, http.StatusOK, "")
	e.expect(http.MethodPut, "/v1/tenant/settings", false, http.StatusOK, "")

	// No grant: refused, super admin included (the token is a super admin's).
	e.expect(http.MethodGet, "/v1/tenant/settings", true, http.StatusForbidden, "SUPPORT_GRANT_REQUIRED")
	e.expect(http.MethodPut, "/v1/tenant/settings", true, http.StatusForbidden, "SUPPORT_GRANT_REQUIRED")
	e.expect(http.MethodGet, "/v1/tenant/support-access", true, http.StatusForbidden, "SUPPORT_GRANT_REQUIRED")

	// read_only: reads pass, every change is refused.
	ro := &orgctx.SupportGrant{ID: 1, UUID: uuid.New(), Scope: orgctx.SupportScopeReadOnly, ExpiresAt: time.Now().Add(time.Hour)}
	e.grants.g = ro
	e.expect(http.MethodGet, "/v1/tenant/settings", true, http.StatusOK, "")
	e.expect(http.MethodPut, "/v1/tenant/settings", true, http.StatusForbidden, "SUPPORT_READ_ONLY")
	e.expect(http.MethodPost, "/v1/tenant/support-access", true, http.StatusForbidden, "SUPPORT_READ_ONLY")
	if len(e.seen) != 1 || e.seen[0] != ro.UUID.String() {
		t.Fatalf("grant on the scope = %v", e.seen)
	}

	// full: changes pass, but a support session never manages grants.
	e.grants.g = &orgctx.SupportGrant{ID: 2, UUID: uuid.New(), Scope: orgctx.SupportScopeFull, ExpiresAt: time.Now().Add(time.Hour)}
	e.expect(http.MethodPut, "/v1/tenant/settings", true, http.StatusOK, "")
	e.expect(http.MethodPost, "/v1/tenant/support-access", true, http.StatusForbidden, "FORBIDDEN")
	e.expect(http.MethodPost, "/v1/tenant/support-access/"+uuid.NewString()+"/revoke", true, http.StatusForbidden, "FORBIDDEN")

	// Revoked / expired (the lookup finds nothing): refused again.
	e.grants.g = nil
	e.expect(http.MethodGet, "/v1/tenant/settings", true, http.StatusForbidden, "SUPPORT_GRANT_REQUIRED")
}

func TestOpenValidatesBeforePersistence(t *testing.T) {
	e := newEnv(t)
	status, code := e.do(http.MethodPost, "/v1/tenant/support-access", false, `{"scope":"full","duration_hours":200}`)
	if status != http.StatusBadRequest || code != "VALIDATION_ERROR" {
		t.Fatalf("open with 200h = %d %q", status, code)
	}
}
