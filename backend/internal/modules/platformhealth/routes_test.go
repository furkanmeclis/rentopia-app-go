package platformhealth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type stubLoader struct{ perms []string }

func (s stubLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{UserID: uuid.New(), UserInternal: 42, Permissions: s.perms}, nil
}

func setup(t *testing.T, perms []string) (*http.ServeMux, string) {
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
	// Only paths that answer before reaching the database are exercised.
	svc := usecase.New(nil, healthcheck.NewRegistry())
	RegisterRoutes(mux, handler.New(svc), tokens, stubLoader{perms: perms})
	return mux, access
}

func call(mux *http.ServeMux, path, token string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr.Code
}

func TestRoutesRequirePlatformHealthRead(t *testing.T) {
	t.Parallel()
	mux, token := setup(t, nil)
	for _, p := range []string{"/v1/platform/usage", "/v1/platform/usage/summary", "/v1/platform/health", "/v1/platform/health/issues"} {
		if code := call(mux, p, ""); code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s = %d", p, code)
		}
		if code := call(mux, p, token); code != http.StatusForbidden {
			t.Fatalf("no permission %s = %d", p, code)
		}
	}

	ok, rt := setup(t, []string{rbac.PermPlatformHealthRead})
	for _, p := range []string{"/v1/platform/usage/meta", "/v1/platform/health/issues/meta"} {
		if code := call(ok, p, rt); code != http.StatusOK {
			t.Fatalf("%s = %d", p, code)
		}
	}
	for _, p := range []string{
		"/v1/platform/usage?sort=bogus",
		"/v1/platform/usage?status=deleted",
		"/v1/platform/usage?month=2026-13",
		"/v1/platform/usage?page=2",
		"/v1/platform/usage/summary?month=abc",
		"/v1/platform/health/issues?sort=bogus",
		"/v1/platform/health/issues?severity=fatal",
		"/v1/platform/health/issues?source=Bad%20Code",
	} {
		if code := call(ok, p, rt); code != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", p, code)
		}
	}
}
