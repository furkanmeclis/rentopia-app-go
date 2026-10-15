package tolls

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
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

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }

func TestTollPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := rbac.PermTenantTollsRead
	resolver := stubResolver{
		"owner-org": {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: []string{
			read, rbac.PermTenantTollsExport, rbac.PermTenantTollsImport, rbac.PermTenantTollsReview,
			rbac.PermTenantVehiclesWrite, rbac.PermTenantSettingsWrite,
		}},
		"reader-org":   {ID: 2, Slug: "reader-org", MemberRole: "member", Permissions: []string{read}},
		"reviewer-org": {ID: 3, Slug: "reviewer-org", MemberRole: "sube_yoneticisi", Permissions: []string{read, rbac.PermTenantTollsReview}},
		"vehicles-org": {ID: 4, Slug: "vehicles-org", MemberRole: "member", Permissions: []string{rbac.PermTenantVehiclesRead, rbac.PermTenantVehiclesWrite}},
	}
	mux := http.NewServeMux()
	// The use case is never reached here (gates or path validation fail
	// first); the engine routes use a stub handler.
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, nil, nil), nil), EngineHandlers{Export: ok, Import: ok, Sample: ok},
		tokens, stubLoader{}, resolver)

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
	p := "/v1/tenant/toll-passes/" + uuid.NewString()
	tags := "/v1/tenant/vehicles/" + uuid.NewString() + "/toll-tags"
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/toll-passes", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/toll-passes", "other-org", "", http.StatusForbidden},
		{"vehicles permission is not tolls read", http.MethodGet, "/v1/tenant/toll-passes", "vehicles-org", "", http.StatusForbidden},
		{"vehicles permission cannot read tags", http.MethodGet, tags, "vehicles-org", "", http.StatusForbidden},
		{"reader cannot export", http.MethodPost, "/v1/tenant/toll-passes/export", "reader-org", "{}", http.StatusForbidden},
		{"reader cannot import", http.MethodPost, "/v1/tenant/toll-passes/import", "reader-org", "", http.StatusForbidden},
		{"reader cannot download the sample", http.MethodGet, "/v1/tenant/toll-passes/import/sample", "reader-org", "", http.StatusForbidden},
		{"reader cannot fetch", http.MethodPost, "/v1/tenant/toll-passes/fetch", "reader-org", "", http.StatusForbidden},
		{"reviewer cannot import", http.MethodPost, "/v1/tenant/toll-passes/import", "reviewer-org", "", http.StatusForbidden},
		{"reader cannot assign", http.MethodPost, p + "/assign", "reader-org", "{}", http.StatusForbidden},
		{"reader cannot unassign", http.MethodPost, p + "/unassign", "reader-org", "{}", http.StatusForbidden},
		{"reader cannot ignore", http.MethodPost, p + "/ignore", "reader-org", "{}", http.StatusForbidden},
		{"reader cannot rematch", http.MethodPost, p + "/rematch", "reader-org", "{}", http.StatusForbidden},
		{"reader cannot list candidates", http.MethodGet, p + "/candidates", "reader-org", "", http.StatusForbidden},
		{"reviewer cannot change the fee", http.MethodPatch, "/v1/tenant/toll-settings", "reviewer-org", "{}", http.StatusForbidden},
		{"reader cannot add a tag", http.MethodPost, tags, "reader-org", "{}", http.StatusForbidden},
		{"reader cannot end a tag", http.MethodPatch, tags + "/" + uuid.NewString(), "reader-org", "{}", http.StatusForbidden},
		{"reader cannot delete a tag", http.MethodDelete, tags + "/" + uuid.NewString(), "reader-org", "", http.StatusForbidden},
		{"owner exports", http.MethodPost, "/v1/tenant/toll-passes/export", "owner-org", "{}", http.StatusAccepted},
		{"owner imports", http.MethodPost, "/v1/tenant/toll-passes/import", "owner-org", "", http.StatusAccepted},
		{"reviewer passes the review gate", http.MethodPost, "/v1/tenant/toll-passes/nope/assign", "reviewer-org", "{}", http.StatusBadRequest},
		{"owner passes the tag gate", http.MethodPost, "/v1/tenant/vehicles/nope/toll-tags", "owner-org", "{}", http.StatusBadRequest},
		{"bad pass uuid", http.MethodGet, "/v1/tenant/toll-passes/nope", "reader-org", "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestNilResolverMountsNothing(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, nil, nil), nil), EngineHandlers{}, nil, stubLoader{}, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/toll-passes", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}

func TestLiveResource(t *testing.T) {
	t.Parallel()
	reg := resourceevents.NewRegistry()
	RegisterLiveResources(reg)
	if perm, ok := reg.Permission(Resource); !ok || perm != rbac.PermTenantTollsRead {
		t.Fatalf("live resource %q %v", perm, ok)
	}
}

func TestFakeProviderOnlyOutsideProduction(t *testing.T) {
	t.Parallel()
	if got := Providers("production", nil); len(got) != 0 {
		t.Fatalf("production providers %d", len(got))
	}
	if got := Providers("development", nil); len(got) != 1 || got[0].Provider() != "hgs_fake" {
		t.Fatalf("development providers %+v", got)
	}
}
