package vehiclerecords

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/usecase"
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

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }

func TestVehicleRecordsPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read, write := rbac.PermTenantVehiclesRead, rbac.PermTenantVehiclesWrite
	resolver := stubResolver{
		"owner-org":    {ID: 1, Slug: "owner-org", MemberRole: "owner", Permissions: []string{read, write, rbac.PermTenantVehiclesExport, rbac.PermTenantDocumentDownload}},
		"reader-org":   {ID: 2, Slug: "reader-org", MemberRole: "member", Permissions: []string{read}},
		"download-org": {ID: 3, Slug: "download-org", MemberRole: "rezervasyon_satis", Permissions: []string{read, rbac.PermTenantDocumentDownload}},
		"fleet-only":   {ID: 4, Slug: "fleet-only", MemberRole: "member", Permissions: []string{rbac.PermTenantFleetRead, rbac.PermTenantDocumentDownload}},
	}
	mux := http.NewServeMux()
	// The use case is never reached in these cases (gates or body decoding
	// fail first); export routes use a stub handler.
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, nil, nil), nil), EngineHandlers{
		ExportDocuments: ok, ExportMaintenance: ok,
	}, tokens, stubLoader{}, resolver)

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
	v, d := uuid.NewString(), uuid.NewString()
	docs := "/v1/tenant/vehicles/" + v + "/documents"
	maint := "/v1/tenant/vehicles/" + v + "/maintenance"
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"missing organization header", http.MethodGet, docs, "", "", http.StatusBadRequest},
		{"not a member", http.MethodGet, docs, "other-org", "", http.StatusForbidden},
		{"fleet read is not vehicles read", http.MethodGet, docs, "fleet-only", "", http.StatusForbidden},
		{"fleet read cannot see alerts", http.MethodGet, "/v1/tenant/fleet/alerts", "fleet-only", "", http.StatusForbidden},
		{"reader cannot create a document", http.MethodPost, docs, "reader-org", "{}", http.StatusForbidden},
		{"reader cannot patch a document", http.MethodPatch, docs + "/" + d, "reader-org", "{}", http.StatusForbidden},
		{"reader cannot delete a document", http.MethodDelete, docs + "/" + d, "reader-org", "", http.StatusForbidden},
		{"reader cannot upload a file", http.MethodPut, docs + "/" + d + "/file", "reader-org", "", http.StatusForbidden},
		{"reader without download cannot download", http.MethodGet, docs + "/" + d + "/file", "reader-org", "", http.StatusForbidden},
		{"reader cannot plan maintenance", http.MethodPost, maint, "reader-org", "{}", http.StatusForbidden},
		{"reader cannot complete maintenance", http.MethodPost, maint + "/" + d + "/complete", "reader-org", "{}", http.StatusForbidden},
		{"reader cannot delete maintenance", http.MethodDelete, maint + "/" + d, "reader-org", "", http.StatusForbidden},
		{"reader cannot export documents", http.MethodPost, "/v1/tenant/vehicle-documents/export", "reader-org", "{}", http.StatusForbidden},
		{"reader cannot export maintenance", http.MethodPost, "/v1/tenant/vehicle-maintenance/export", "reader-org", "{}", http.StatusForbidden},
		{"owner exports documents", http.MethodPost, "/v1/tenant/vehicle-documents/export", "owner-org", "{}", http.StatusAccepted},
		{"owner exports maintenance", http.MethodPost, "/v1/tenant/vehicle-maintenance/export", "owner-org", "{}", http.StatusAccepted},
		{"download needs vehicles read too", http.MethodGet, docs + "/" + d + "/file", "fleet-only", "", http.StatusForbidden},
		{"owner passes the write gate", http.MethodPost, docs, "owner-org", "{not json", http.StatusBadRequest},
		{"unknown body fields are rejected", http.MethodPost, maint, "owner-org", `{"price":3}`, http.StatusBadRequest},
		{"bad vehicle uuid", http.MethodGet, "/v1/tenant/vehicles/nope/documents", "owner-org", "", http.StatusBadRequest},
		{"bad document uuid", http.MethodGet, docs + "/nope/file", "download-org", "", http.StatusBadRequest},
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
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/tenant/fleet/alerts", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("tenant route without tenancy = %d", rr.Code)
	}
}
