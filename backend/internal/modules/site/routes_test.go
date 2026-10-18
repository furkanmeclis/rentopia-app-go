package site

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type articleLoader struct{}

func (articleLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{UserID: uuid.New(), UserInternal: 42, IsSuperAdmin: true}, nil
}

type articleResolver map[string]orgctx.Scope

func (s articleResolver) Resolve(_ context.Context, ref string, _ int64) (orgctx.Scope, error) {
	if scope, ok := s[ref]; ok {
		return scope, nil
	}
	return orgctx.Scope{}, orgctx.ErrNotMember
}

func TestArticleRoutePermissions(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	resolver := articleResolver{
		"reader":  {ID: 1, Slug: "reader", Permissions: []string{rbac.PermTenantSiteRead}},
		"manager": {ID: 2, Slug: "manager", Permissions: []string{rbac.PermTenantSiteManage}},
		"none":    {ID: 3, Slug: "none", Permissions: []string{}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(nil, nil, nil), tokens, articleLoader{}, resolver)
	tests := []struct {
		method, path, org, body string
		want                    int
	}{
		{"GET", "/v1/tenant/site/articles", "", "", 400},
		{"GET", "/v1/tenant/site/articles", "foreign", "", 403},
		{"GET", "/v1/tenant/site/articles", "none", "", 403},
		{"GET", "/v1/tenant/site/articles/meta", "reader", "", 200},
		{"GET", "/v1/tenant/site/media/" + uuid.NewString() + "/file", "none", "", 403},
		{"POST", "/v1/tenant/site/articles", "reader", "{}", 403},
		{"POST", "/v1/tenant/site/articles", "manager", "bad", 400},
		{"PUT", "/v1/tenant/site/articles/" + uuid.NewString(), "reader", "{}", 403},
		{"DELETE", "/v1/tenant/site/articles/" + uuid.NewString(), "reader", "", 403},
		{"POST", "/v1/tenant/site/articles/" + uuid.NewString() + "/publish", "reader", "{}", 403},
		{"POST", "/v1/tenant/site/articles/" + uuid.NewString() + "/unpublish", "reader", "{}", 403},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+access)
		if tc.org != "" {
			req.Header.Set(orgctx.HeaderName, tc.org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s %s (%s): %d want %d: %s", tc.method, tc.path, tc.org, rr.Code, tc.want, rr.Body.String())
		}
	}
}
