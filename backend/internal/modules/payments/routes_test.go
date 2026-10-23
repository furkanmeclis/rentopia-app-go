package payments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type stubLoader struct{}

func (stubLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{UserID: uuid.New(), UserInternal: 7, IsSuperAdmin: true}, nil
}

type stubResolver map[string]orgctx.Scope

func (s stubResolver) Resolve(_ context.Context, ref string, _ int64) (orgctx.Scope, error) {
	scope, ok := s[ref]
	if !ok {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	return scope, nil
}

// The permission triad of the template roles (migration 000080): each
// route answers 403 without its slug and passes the gate with it (a
// malformed body / path then answers 400 before the database).
func TestPaymentPermissionGates(t *testing.T) {
	t.Parallel()
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	read := rbac.PermTenantPaymentsRead
	resolver := stubResolver{
		"sales":      {ID: 1, Slug: "sales", Permissions: []string{read, rbac.PermTenantPaymentsCreateLink, rbac.PermTenantPaymentCollect}},
		"accounting": {ID: 2, Slug: "accounting", Permissions: []string{read, rbac.PermTenantPaymentRefund}},
		"delivery":   {ID: 3, Slug: "delivery", Permissions: []string{read, rbac.PermTenantPaymentCollect}},
		"member":     {ID: 4, Slug: "member", Permissions: []string{rbac.PermTenantReservationsRead}},
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, handler.New(usecase.New(nil, nil, nil, nil, usecase.Config{}, iyzico.New(iyzico.Config{})), nil), tokens, stubLoader{}, resolver)
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
		{"member cannot list payments", http.MethodGet, "/v1/tenant/payments", "member", "", http.StatusForbidden},
		{"sales cannot refund", http.MethodPost, "/v1/tenant/payments/" + id + "/refund", "sales", "{}", http.StatusForbidden},
		{"accounting cannot create links", http.MethodPost, "/v1/tenant/reservations/" + id + "/payment-links", "accounting", "{}", http.StatusForbidden},
		{"accounting cannot collect", http.MethodPost, "/v1/tenant/reservations/" + id + "/collections", "accounting", "{}", http.StatusForbidden},
		{"delivery cannot cancel links", http.MethodPost, "/v1/tenant/payment-links/" + id + "/cancel", "delivery", "", http.StatusForbidden},
		{"delivery cannot verify", http.MethodPost, "/v1/tenant/payments/" + id + "/verify", "delivery", "", http.StatusForbidden},
		{"sales passes the link gate", http.MethodPost, "/v1/tenant/reservations/not-a-uuid/payment-links", "sales", "{}", http.StatusBadRequest},
		{"delivery passes the collect gate", http.MethodPost, "/v1/tenant/reservations/not-a-uuid/collections", "delivery", "{}", http.StatusBadRequest},
		{"accounting passes the refund gate", http.MethodPost, "/v1/tenant/payments/not-a-uuid/refund", "accounting", "{}", http.StatusBadRequest},
		{"not a member", http.MethodGet, "/v1/tenant/payments", "other", "", http.StatusForbidden},
		{"member cannot list orphans", http.MethodGet, "/v1/tenant/payments/orphans", "member", "", http.StatusForbidden},
		{"accounting cannot match orphans", http.MethodPost, "/v1/tenant/payments/orphans/" + id + "/match", "accounting", "{}", http.StatusForbidden},
		{"delivery passes the match gate", http.MethodPost, "/v1/tenant/payments/orphans/not-a-uuid/match", "delivery", "{}", http.StatusBadRequest},
		{"member cannot read the reconciliation", http.MethodGet, "/v1/tenant/payments/reconciliation?since=2026-01-01T00:00:00Z", "member", "", http.StatusForbidden},
		{"reconciliation needs since", http.MethodGet, "/v1/tenant/payments/reconciliation", "sales", "", http.StatusBadRequest},
		{"sales cannot resolve items", http.MethodPost, "/v1/tenant/payments/reconciliation-items/" + id + "/resolve", "sales", "{}", http.StatusForbidden},
		{"accounting passes the resolve gate", http.MethodPost, "/v1/tenant/payments/reconciliation-items/not-a-uuid/resolve", "accounting", "{}", http.StatusBadRequest},
		{"public webhook needs no auth (malformed body)", http.MethodPost, "/v1/public/payments/iyzico/webhook", "", "{}", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got := do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}
