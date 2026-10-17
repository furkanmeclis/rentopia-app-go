package siteleads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	leadhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/usecase"
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

func TestTenantPermissionGates(t *testing.T) {
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
		"no-read":    {ID: 3, Slug: "no-read", Permissions: []string{}},
		"owner-org":  {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: []string{rbac.PermTenantSiteLeadsRead, rbac.PermTenantSiteLeadsManage}},
		"member-org": {ID: 2, Slug: "member-org", MemberRole: "member", Permissions: []string{rbac.PermTenantSiteLeadsRead}},
	}
	mux := http.NewServeMux()
	// The use case is never reached in these cases (gates or body decoding fail first).
	RegisterRoutes(mux, leadhandler.New(usecase.New(nil, "test-salt"), nil), tokens, stubLoader{}, resolver)

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

	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing header", http.MethodGet, "/v1/tenant/site/leads", "", "", 400},
		{"non-member", http.MethodGet, "/v1/tenant/site/leads", "other-org", "", 403},
		{"no read permission", http.MethodGet, "/v1/tenant/site/leads/meta", "no-read", "", 403},
		{"read meta", http.MethodGet, "/v1/tenant/site/leads/meta", "member-org", "", 200},
		{"cannot manage", http.MethodPatch, "/v1/tenant/site/leads/" + uuid.NewString(), "member-org", "{}", 403},
		{"manage gate", http.MethodPatch, "/v1/tenant/site/leads/" + uuid.NewString(), "owner-org", "{bad", 400},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}
