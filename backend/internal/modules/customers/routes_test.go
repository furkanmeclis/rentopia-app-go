package customers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
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

func TestCustomerPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := rbac.PermTenantCustomersRead
	resolver := stubResolver{
		"owner-org": {ID: 1, Slug: "owner-org", Permissions: []string{
			read, rbac.PermTenantCustomersWrite, rbac.PermTenantCustomersReveal, rbac.PermTenantCustomersExport,
		}},
		"member-org":     {ID: 2, Slug: "member-org", Permissions: []string{read}},
		"accounting-org": {ID: 3, Slug: "accounting-org", Permissions: []string{read, rbac.PermTenantCustomersExport}},
	}
	exported := false
	mux := http.NewServeMux()
	// The use case is never reached in these cases (gates or body decoding fail first).
	h := handler.New(usecase.New(nil, nil, nil, nil, nil), nil)
	RegisterRoutes(mux, h, EngineHandlers{Export: func(w http.ResponseWriter, _ *http.Request) {
		exported = true
		w.WriteHeader(http.StatusAccepted)
	}}, tokens, stubLoader{}, resolver)

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
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/customers", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/customers", "other-org", "", http.StatusForbidden},
		{"member cannot create", http.MethodPost, "/v1/tenant/customers", "member-org", "{}", http.StatusForbidden},
		{"member cannot reveal", http.MethodPost, "/v1/tenant/customers/" + id + "/reveal", "member-org", `{"reason":"x"}`, http.StatusForbidden},
		{"member cannot anonymize", http.MethodPost, "/v1/tenant/customers/" + id + "/anonymize", "member-org", "{}", http.StatusForbidden},
		{"owner without anonymize permission", http.MethodPost, "/v1/tenant/customers/" + id + "/anonymize", "owner-org", "{}", http.StatusForbidden},
		{"member cannot download documents", http.MethodGet, "/v1/tenant/customers/" + id + "/documents/" + id + "/file", "member-org", "", http.StatusForbidden},
		{"owner without document.download", http.MethodGet, "/v1/tenant/customers/" + id + "/documents/" + id + "/file", "owner-org", "", http.StatusForbidden},
		{"member cannot export", http.MethodPost, "/v1/tenant/customers/export", "member-org", `{"format":"csv"}`, http.StatusForbidden},
		{"sensitive export needs reveal", http.MethodPost, "/v1/tenant/customers/export", "accounting-org",
			`{"format":"csv","query":{"include_sensitive":"true"}}`, http.StatusForbidden},
		{"member reads meta", http.MethodGet, "/v1/tenant/customers/meta", "member-org", "", http.StatusOK},
		{"owner passes the write gate", http.MethodPost, "/v1/tenant/customers", "owner-org", "{not json", http.StatusBadRequest},
		{"import route not mounted without engine", http.MethodPost, "/v1/tenant/customers/import", "owner-org", "{}", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
	if exported {
		t.Fatal("a refused export reached the export engine")
	}
	if got := do(http.MethodPost, "/v1/tenant/customers/export", "accounting-org", `{"format":"csv"}`); got != http.StatusAccepted || !exported {
		t.Fatalf("masked export by accounting = %d (engine reached: %v)", got, exported)
	}
}
