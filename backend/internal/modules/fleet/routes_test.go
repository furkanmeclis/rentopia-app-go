package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fleethandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
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

func TestFleetPermissionGates(t *testing.T) {
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
		"owner-org":  {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: []string{rbac.PermTenantFleetRead, rbac.PermTenantFleetWrite}},
		"member-org": {ID: 2, Slug: "member-org", MemberRole: "member", Permissions: []string{rbac.PermTenantFleetRead}},
		"no-fleet":   {ID: 3, Slug: "no-fleet", MemberRole: "member", Permissions: []string{}},
	}
	mux := http.NewServeMux()
	// The use case is never reached in these cases (gates or body decoding fail first).
	RegisterRoutes(mux, fleethandler.New(usecase.New(nil, nil, nil), nil), EngineHandlers{}, tokens, stubLoader{}, resolver)

	do := func(method, path, org, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+access)
		if org != "" {
			req.Header.Set(orgctx.HeaderName, org)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}
	id := uuid.NewString()
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/products", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/segments", "other-org", "", http.StatusForbidden},
		{"no fleet permission", http.MethodGet, "/v1/tenant/rental-groups/meta", "no-fleet", "", http.StatusForbidden},
		{"member reads segments meta", http.MethodGet, "/v1/tenant/segments/meta", "member-org", "", http.StatusOK},
		{"member reads groups meta", http.MethodGet, "/v1/tenant/rental-groups/meta", "member-org", "", http.StatusOK},
		{"member reads products meta", http.MethodGet, "/v1/tenant/products/meta", "member-org", "", http.StatusOK},
		{"member cannot create a segment", http.MethodPost, "/v1/tenant/segments", "member-org", "{}", http.StatusForbidden},
		{"member cannot patch a group", http.MethodPatch, "/v1/tenant/rental-groups/" + id, "member-org", "{}", http.StatusForbidden},
		{"member cannot delete a product", http.MethodDelete, "/v1/tenant/products/" + id, "member-org", "", http.StatusForbidden},
		{"member cannot upload images", http.MethodPost, "/v1/tenant/products/" + id + "/images", "member-org", "", http.StatusForbidden},
		{"member cannot reorder images", http.MethodPut, "/v1/tenant/products/" + id + "/images/order", "member-org", "{}", http.StatusForbidden},
		{"owner passes the write gate", http.MethodPost, "/v1/tenant/products", "owner-org", "{not json", http.StatusBadRequest},
		{"unknown body fields are rejected", http.MethodPost, "/v1/tenant/rental-groups", "owner-org", `{"stock":3}`, http.StatusBadRequest},
		{"export not mounted without engine", http.MethodPost, "/v1/tenant/products/export", "owner-org", "{}", http.StatusMethodNotAllowed},
		{"bulk not mounted without engine", http.MethodPost, "/v1/tenant/rental-groups/bulk", "owner-org", "{}", http.StatusMethodNotAllowed},
		{"public image with a bad uuid", http.MethodGet, "/v1/public/media/rental-products/x/images/y", "", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body).Code; got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}

	// Meta advertises the bulk actions behind tenant.fleet.write.
	rr := do(http.MethodGet, "/v1/tenant/products/meta", "member-org", "")
	var env struct {
		Data struct {
			Resource    string `json:"resource"`
			BulkActions []struct {
				ID         string `json:"id"`
				Permission string `json:"permission"`
			} `json:"bulk_actions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Resource != ResourceRentalProducts || len(env.Data.BulkActions) != 2 ||
		env.Data.BulkActions[0].Permission != rbac.PermTenantFleetWrite {
		t.Fatalf("products meta = %+v", env.Data)
	}
}

func TestNilResolverMountsOnlyPublicMedia(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, fleethandler.New(usecase.New(nil, nil, nil), nil), EngineHandlers{}, nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/segments", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}
