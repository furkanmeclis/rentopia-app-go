package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cataloghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type stubLoader struct{ perms []string }

func (s stubLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{UserID: uuid.New(), UserInternal: 42, Permissions: s.perms}, nil
}

type stubResolver struct{}

func (stubResolver) Resolve(_ context.Context, ref string, userID int64) (orgctx.Scope, error) {
	if ref != "acme" || userID != 42 {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	return orgctx.Scope{ID: 1, UUID: uuid.New(), Slug: "acme", Status: orgctx.StatusActive, MemberRole: orgctx.RoleMember}, nil
}

func setup(t *testing.T, perms []string, resolver orgctx.Resolver) (*http.ServeMux, string) {
	t.Helper()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	// Only paths that answer before reaching the use case are exercised.
	RegisterRoutes(mux, cataloghandler.New(nil, nil), tokens, stubLoader{perms: perms}, resolver)
	return mux, access
}

func call(mux *http.ServeMux, method, path, token, org string) int {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if org != "" {
		req.Header.Set(orgctx.HeaderName, org)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr.Code
}

func TestPlatformRoutesRequireCatalogPermissions(t *testing.T) {
	t.Parallel()
	mux, token := setup(t, nil, nil)
	if code := call(mux, http.MethodGet, "/v1/platform/catalog/brands", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", code)
	}
	if code := call(mux, http.MethodGet, "/v1/platform/catalog/brands", token, ""); code != http.StatusForbidden {
		t.Fatalf("no permission = %d", code)
	}

	readOnly, rt := setup(t, []string{rbac.PermPlatformCatalogRead}, nil)
	if code := call(readOnly, http.MethodGet, "/v1/platform/catalog/types/meta", rt, ""); code != http.StatusOK {
		t.Fatalf("meta = %d", code)
	}
	if code := call(readOnly, http.MethodGet, "/v1/platform/catalog/brands?sort=bogus", rt, ""); code != http.StatusBadRequest {
		t.Fatalf("bad sort = %d", code)
	}
	if code := call(readOnly, http.MethodGet, "/v1/platform/catalog/types?year=abc", rt, ""); code != http.StatusBadRequest {
		t.Fatalf("bad year = %d", code)
	}
	if code := call(readOnly, http.MethodPost, "/v1/platform/catalog/imports/"+uuid.NewString()+"/apply", rt, ""); code != http.StatusForbidden {
		t.Fatalf("apply without write = %d", code)
	}
	if code := call(readOnly, http.MethodGet, "/v1/platform/catalog/display-brands/meta", rt, ""); code != http.StatusOK {
		t.Fatalf("display brands meta = %d", code)
	}
	if code := call(readOnly, http.MethodGet, "/v1/platform/catalog/display-brands?sort=bogus", rt, ""); code != http.StatusBadRequest {
		t.Fatalf("display brands bad sort = %d", code)
	}
	if code := call(mux, http.MethodGet, "/v1/platform/catalog/display-brands", token, ""); code != http.StatusForbidden {
		t.Fatalf("display brands without permission = %d", code)
	}
	for _, path := range []string{
		"/v1/platform/catalog/display-brands/" + uuid.NewString() + "/merge",
		"/v1/platform/catalog/display-brands/" + uuid.NewString() + "/unmerge",
		"/v1/platform/catalog/models/bulk-rules",
	} {
		if code := call(readOnly, http.MethodPost, path, rt, ""); code != http.StatusForbidden {
			t.Fatalf("POST %s without write = %d", path, code)
		}
	}
	// The logo stream only needs a session (tenant forms show it too).
	if code := call(mux, http.MethodGet, "/v1/platform/catalog/display-brands/not-a-uuid/logo", token, ""); code != http.StatusBadRequest {
		t.Fatalf("logo without catalog permission = %d", code)
	}
	if code := call(mux, http.MethodGet, "/v1/platform/catalog/display-brands/"+uuid.NewString()+"/logo", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous logo = %d", code)
	}
}

func TestTenantRoutes(t *testing.T) {
	t.Parallel()
	off, token := setup(t, nil, nil)
	if code := call(off, http.MethodGet, "/v1/tenant/catalog/brands", token, "acme"); code != http.StatusNotFound {
		t.Fatalf("tenancy off = %d, want 404", code)
	}

	on, token := setup(t, nil, stubResolver{})
	if code := call(on, http.MethodGet, "/v1/tenant/catalog/models", token, "other"); code != http.StatusForbidden {
		t.Fatalf("non-member = %d", code)
	}
	if code := call(on, http.MethodGet, "/v1/tenant/catalog/models", token, ""); code != http.StatusBadRequest {
		t.Fatalf("missing organization = %d", code)
	}
	// Members need no catalog permission; the brand filter is validated.
	if code := call(on, http.MethodGet, "/v1/tenant/catalog/models", token, "acme"); code != http.StatusBadRequest {
		t.Fatalf("missing brand = %d", code)
	}
	if code := call(on, http.MethodGet, "/v1/tenant/catalog/types?model=nope", token, "acme"); code != http.StatusBadRequest {
		t.Fatalf("bad model = %d", code)
	}
}
