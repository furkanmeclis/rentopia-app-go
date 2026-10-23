package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms/logsms"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type stubLoader struct{ user int64 }

func (l stubLoader) LoadPrincipal(_ *http.Request, _ jwt.Claims) (authctx.Principal, error) {
	return authctx.Principal{UserID: uuid.New(), UserInternal: l.user, IsSuperAdmin: true}, nil
}

type stubResolver map[string]orgctx.Scope

func (s stubResolver) Resolve(_ context.Context, ref string, _ int64) (orgctx.Scope, error) {
	scope, ok := s[ref]
	if !ok {
		return orgctx.Scope{}, orgctx.ErrNotMember
	}
	return scope, nil
}

type harness struct {
	mux    *http.ServeMux
	access string
}

func newHarness(t *testing.T, runner *usecase.Runner, user int64, resolver stubResolver) harness {
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
	RegisterRoutes(mux, handler.New(runner, nil), tokens, stubLoader{user: user}, resolver)
	return harness{mux: mux, access: access}
}

func (h harness) do(method, path, org, body string) (int, map[string]any) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+h.access)
	req.Header.Set("Content-Type", "application/json")
	if org != "" {
		req.Header.Set(orgctx.HeaderName, org)
	}
	rr := httptest.NewRecorder()
	h.mux.ServeHTTP(rr, req)
	out := map[string]any{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr.Code, out
}

// Reconcile needs tenant.integrations.manage like retry; with it a
// malformed path answers 400 before the database.
func TestReconcilePermissionGates(t *testing.T) {
	t.Parallel()
	h := newHarness(t, usecase.NewRunner(nil, nil, nil), 7, stubResolver{
		"ops":    {ID: 1, Slug: "ops", Permissions: []string{rbac.PermTenantIntegrationsManage}},
		"member": {ID: 2, Slug: "member", Permissions: []string{rbac.PermTenantReservationsRead}},
	})
	id := uuid.NewString()
	cases := []struct {
		name, method, path, org, body string
		want                          int
	}{
		{"member cannot reconcile", http.MethodPost, "/v1/tenant/integration-jobs/" + id + "/reconcile", "member", `{"note":"x"}`, http.StatusForbidden},
		{"member cannot bulk reconcile", http.MethodPost, "/v1/tenant/integration-jobs/reconcile", "member", `{}`, http.StatusForbidden},
		{"ops passes the reconcile gate", http.MethodPost, "/v1/tenant/integration-jobs/not-a-uuid/reconcile", "ops", `{}`, http.StatusBadRequest},
		{"bulk validates the body", http.MethodPost, "/v1/tenant/integration-jobs/reconcile", "ops", `{"note":"restore"}`, http.StatusBadRequest},
		{"bulk refuses final states", http.MethodPost, "/v1/tenant/integration-jobs/reconcile", "ops",
			`{"note":"restore","before":"2026-01-01T00:00:00Z","statuses":["succeeded"]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if got, _ := h.do(tc.method, tc.path, tc.org, tc.body); got != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// uncertainOp submits with an unknown outcome (the job waits in
// needs_verification) and counts provider calls.
type uncertainOp struct{ calls atomic.Int32 }

func (o *uncertainOp) Kind() string { return "test.op" }

func (o *uncertainOp) Submit(context.Context, integrations.Account, integrations.Job) (integrations.Result, error) {
	o.calls.Add(1)
	return integrations.Result{Outcome: integrations.OutcomeUncertain, Message: "timeout"}, nil
}

func (o *uncertainOp) CheckStatus(context.Context, integrations.Account, integrations.Job) (integrations.Result, error) {
	o.calls.Add(1)
	return integrations.Result{Outcome: integrations.OutcomeUnsupported}, nil
}

// The reconcile transitions over HTTP: needs_verification → reconciled
// (200, who / why kept), again → 409 INTEGRATION_JOB_NOT_RECONCILABLE, no
// note → 400, another organization → 404; the bulk route answers the
// count.
func TestReconcileRouteTransitions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "recon-" + suffix, Name: "Recon " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{Email: "recon-" + suffix + "@example.test", PasswordHash: "x", Name: "Rec", Surname: "On", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})
	fc, err := crypto.NewFieldCipher(strings.Repeat("r", 32))
	if err != nil {
		t.Fatal(err)
	}
	vault, err := integrations.NewVault(fc)
	if err != nil {
		t.Fatal(err)
	}
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(logsms.New(nil)))
	a, err := accounts.Connect(ctx, org.ID, accountsusecase.ConnectInput{
		Provider: "log", DisplayName: "Test", Credentials: map[string]string{"msgheader": "RENTOPIA"},
	})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := accounts.AccountByUUID(ctx, org.ID, a.UUID)
	if err != nil {
		t.Fatal(err)
	}
	op := &uncertainOp{}
	ops := integrations.NewOperationRegistry()
	ops.Register("log", op)
	runner := usecase.NewRunner(q, accounts, ops)
	enqueue := func() uuid.UUID {
		j, err := runner.Enqueue(ctx, usecase.EnqueueInput{
			OrganizationID: org.ID, AccountID: acc.ID, Provider: "log", Kind: "test.op",
			IdempotencyKey: "org:test:" + uuid.NewString(), MaxAttempts: 5,
		})
		if err != nil {
			t.Fatal(err)
		}
		return j.UUID
	}
	job := enqueue()
	h := newHarness(t, runner, user.ID, stubResolver{
		"ops":   {ID: org.ID, Slug: "ops", Permissions: []string{rbac.PermTenantIntegrationsManage}},
		"other": {ID: org.ID + 1_000_000, Slug: "other", Permissions: []string{rbac.PermTenantIntegrationsManage}},
	})
	path := "/v1/tenant/integration-jobs/" + job.String() + "/reconcile"

	if code, _ := h.do(http.MethodPost, path, "ops", `{"note":""}`); code != http.StatusBadRequest {
		t.Fatalf("empty note: %d", code)
	}
	if code, _ := h.do(http.MethodPost, path, "other", `{"note":"seen in panel"}`); code != http.StatusNotFound {
		t.Fatalf("other organization: %d", code)
	}
	code, body := h.do(http.MethodPost, path, "ops", `{"note":"seen in panel","external_ref":"NG-77"}`)
	if code != http.StatusOK {
		t.Fatalf("reconcile: %d %v", code, body)
	}
	data, _ := body["data"].(map[string]any)
	rec, _ := data["reconciliation"].(map[string]any)
	by, _ := rec["by"].(map[string]any)
	if data["status"] != "reconciled" || data["reconcilable"] != false || rec["note"] != "seen in panel" ||
		rec["external_ref"] != "NG-77" || by["email"] != user.Email {
		t.Fatalf("reconcile answer: %v", data)
	}
	code, body = h.do(http.MethodPost, path, "ops", `{"note":"again please"}`)
	errBody, _ := body["error"].(map[string]any)
	if code != http.StatusConflict || errBody["code"] != handler.CodeNotReconcilable {
		t.Fatalf("second reconcile: %d %v", code, body)
	}
	calls := op.calls.Load()

	// Bulk: two more jobs; a cut-off before them reconciles none, after them both.
	enqueue()
	enqueue()
	code, body = h.do(http.MethodPost, "/v1/tenant/integration-jobs/reconcile", "ops",
		`{"note":"restore window","before":"2000-01-01T00:00:00Z","statuses":["needs_verification"]}`)
	if data, _ := body["data"].(map[string]any); code != http.StatusOK || data["count"] != float64(0) {
		t.Fatalf("bulk before the jobs: %d %v", code, body)
	}
	before := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	code, body = h.do(http.MethodPost, "/v1/tenant/integration-jobs/reconcile", "ops",
		`{"note":"restore window","before":"`+before+`","statuses":["needs_verification","pending"],"kinds":["test.op"]}`)
	if data, _ := body["data"].(map[string]any); code != http.StatusOK || data["count"] != float64(2) {
		t.Fatalf("bulk: %d %v", code, body)
	}
	if _, err := runner.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	if got := op.calls.Load(); got != calls+2 { // the two submits of the bulk jobs only
		t.Fatalf("provider calls after reconcile: %d, want %d", got, calls+2)
	}
}
