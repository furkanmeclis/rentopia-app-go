package reservations

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
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

// The permission triad of the template roles (migration 000075): each
// route answers 403 without its slug and reaches the handler with it.
func TestReservationPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := rbac.PermTenantReservationsRead
	resolver := stubResolver{
		// rezervasyon_satis: no eligibility override, tasks read only.
		"sales": {ID: 1, Slug: "sales", Permissions: []string{
			read, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel, rbac.PermTenantReservationsConfirm,
			rbac.PermTenantReservationsExport, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite, rbac.PermTenantResolutionTasksRead,
		}},
		// teslim_iade: reservations read, tasks read / write.
		"delivery": {ID: 2, Slug: "delivery", Permissions: []string{read, rbac.PermTenantResolutionTasksRead, rbac.PermTenantResolutionTasksWrite}},
		// muhasebe: read / export, quotes read.
		"accounting": {ID: 3, Slug: "accounting", Permissions: []string{read, rbac.PermTenantReservationsExport, rbac.PermTenantQuotesRead}},
		"member":     {ID: 4, Slug: "member", Permissions: []string{read}},
		// rezervasyon_satis after 000080: the approval queue slug.
		"approver": {ID: 5, Slug: "approver", Permissions: []string{read, rbac.PermTenantReservationsApprove}},
	}
	exported := 0
	mux := http.NewServeMux()
	h := handler.New(usecase.New(nil, nil, nil, nil, nil), nil)
	RegisterRoutes(mux, h, EngineHandlers{Export: func(w http.ResponseWriter, _ *http.Request) {
		exported++
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
	r := "/v1/tenant/reservations/"
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, "/v1/tenant/reservations", "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/reservations", "other", "", http.StatusForbidden},
		{"member cannot create", http.MethodPost, "/v1/tenant/reservations", "member", "{}", http.StatusForbidden},
		{"member cannot cancel", http.MethodPost, r + id + "/cancel", "member", `{"reason":"x"}`, http.StatusForbidden},
		{"accounting cannot confirm", http.MethodPost, r + id + "/confirm", "accounting", "", http.StatusForbidden},
		{"delivery cannot approve", http.MethodPost, r + id + "/approve", "delivery", "", http.StatusForbidden},
		{"confirm alone cannot approve", http.MethodPost, r + id + "/approve", "sales", "", http.StatusForbidden},
		{"confirm alone cannot accept", http.MethodPost, r + id + "/accept", "sales", "{}", http.StatusForbidden},
		{"accounting cannot reject", http.MethodPost, r + id + "/reject", "accounting", `{"reason":"xyz"}`, http.StatusForbidden},
		{"approver passes the accept gate", http.MethodPost, r + id + "/accept", "approver", "{not json", http.StatusBadRequest},
		{"approver passes the reject gate", http.MethodPost, r + id + "/reject", "approver", "{not json", http.StatusBadRequest},
		{"accounting cannot quote", http.MethodPost, "/v1/tenant/quotes", "accounting", "{}", http.StatusForbidden},
		{"member cannot read quotes", http.MethodGet, "/v1/tenant/quotes", "member", "", http.StatusForbidden},
		{"sales cannot resolve tasks", http.MethodPost, "/v1/tenant/resolution-tasks/" + id + "/resolve", "sales", "{}", http.StatusForbidden},
		{"member cannot read tasks", http.MethodGet, "/v1/tenant/resolution-tasks", "member", "", http.StatusForbidden},
		{"member cannot export", http.MethodPost, r + "export", "member", `{"format":"csv"}`, http.StatusForbidden},
		{"change quote needs quotes.write", http.MethodPost, r + id + "/quotes", "delivery", "{}", http.StatusForbidden},
		{"member reads meta", http.MethodGet, r + "meta", "member", "", http.StatusOK},
		{"sales passes the write gate", http.MethodPost, "/v1/tenant/reservations", "sales", "{not json", http.StatusBadRequest},
		{"bulk route not mounted without engine", http.MethodPost, r + "bulk", "sales", "{}", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
	if exported != 0 {
		t.Fatal("a refused export reached the engine")
	}
	if got := do(http.MethodPost, r+"export", "accounting", `{"format":"csv"}`); got != http.StatusAccepted || exported != 1 {
		t.Fatalf("export by accounting = %d", got)
	}
}

// Live lists: the three resources are registered with their read slugs.
func TestLiveResources(t *testing.T) {
	reg := resourceevents.NewRegistry()
	RegisterLiveResources(reg)
	for resource, perm := range map[string]string{
		model.ResourceReservations: rbac.PermTenantReservationsRead,
		model.ResourceQuotes:       rbac.PermTenantQuotesRead,
		model.ResourceTasks:        rbac.PermTenantResolutionTasksRead,
	} {
		if got, ok := reg.Permission(resource); !ok || got != perm {
			t.Errorf("%s: %q %v, want %q", resource, got, ok, perm)
		}
	}
}
