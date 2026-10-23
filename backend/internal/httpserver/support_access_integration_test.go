package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/stepup"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// TestSupportAccessGrants (TEC-678) runs the real server: impersonation
// without a grant is refused, a read_only grant lets support staff read but
// not change (PUT → 403 SUPPORT_READ_ONLY), a full grant lets them act
// (events carry imp + support_grant_uuid), revoke and expiry end access on
// the next request.
func TestSupportAccessGrants(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()

	const secret = "support-access-test-access-secret-0123"
	cfg := config.Config{
		App:        config.AppConfig{Name: "Support", Env: "test"},
		JWT:        config.JWTConfig{AccessSecret: secret, RefreshSecret: secret, AccessTTL: time.Hour, RefreshTTL: time.Hour},
		Encryption: config.EncryptionConfig{Key: strings.Repeat("k", 32)},
		Tenancy:    config.TenancyConfig{Enabled: true},
		SMS:        config.SMSConfig{LogProvider: true},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := New(cfg, log, Deps{DB: pool, Queries: q, Redis: rdb, Storage: &memStorage{m: storage.NewMemory()}})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.http.Handler
	tokens, err := jwt.NewManager(secret, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	var users []int64
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-sup-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Sup", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, u.ID)
		return u
	}
	owner, staff := newUser("owner"), newUser("staff")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Support " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE actor_user_id = ANY($1) OR organization_id = $2", users, org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM refresh_tokens WHERE user_id = ANY($1)", users)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", users)
	})
	// Platform staff: a super admin with a fresh step-up (impersonation needs it).
	if _, err := stepup.NewStore(rdb, cfg.App.Env).SetGrant(ctx, staff.Uuid, "password", time.Hour); err != nil {
		t.Fatal(err)
	}

	issue := func(u db.User, superAdmin bool) string {
		in := jwt.AccessInput{UserID: u.Uuid, Roles: []string{"cms_user"}}
		if superAdmin {
			in = jwt.AccessInput{UserID: u.Uuid, Roles: []string{rbac.RoleSuperAdmin}, IsSuperAdmin: true}
		}
		tok, _, err := tokens.IssueAccess(in)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	type result struct {
		code    int
		errCode string
		data    map[string]any
	}
	do := func(token, method, path string, body any) result {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if strings.HasPrefix(path, "/v1/tenant/") {
			req.Header.Set(orgctx.HeaderName, org.Slug)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var env struct {
			Data  map[string]any `json:"data"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return result{code: rr.Code, errCode: env.Error.Code, data: env.Data}
	}
	expect := func(r result, code int, errCode, step string) result {
		t.Helper()
		if r.code != code || r.errCode != errCode {
			t.Fatalf("%s = %d %q, want %d %q", step, r.code, r.errCode, code, errCode)
		}
		return r
	}
	ownerTok, staffTok := issue(owner, false), issue(staff, true)
	impersonate := "/v1/platform/users/" + owner.Uuid.String() + "/impersonate"

	// 1. No grant: impersonation refused, super admin included.
	expect(do(staffTok, http.MethodPost, impersonate, nil), http.StatusForbidden, "SUPPORT_GRANT_REQUIRED", "impersonate without grant")
	// A support token minted anyway (or kept from an earlier grant) cannot enter.
	staffID := staff.Uuid
	stale, _, _ := tokens.IssueAccess(jwt.AccessInput{UserID: owner.Uuid, Roles: []string{"cms_user"}, ImpersonatorID: &staffID})
	expect(do(stale, http.MethodGet, "/v1/tenant/settings", nil), http.StatusForbidden, "SUPPORT_GRANT_REQUIRED", "tenant read without grant")

	// 2. read_only grant opened by the owner.
	ro := expect(do(ownerTok, http.MethodPost, "/v1/tenant/support-access", map[string]any{
		"scope": "read_only", "duration_hours": 2, "note": "fatura sorunu",
	}), http.StatusCreated, "", "open read_only")
	roUUID, _ := ro.data["uuid"].(string)
	if ro.data["status"] != "active" || ro.data["scope"] != "read_only" {
		t.Fatalf("opened grant = %v", ro.data)
	}
	imp := expect(do(staffTok, http.MethodPost, impersonate, nil), http.StatusOK, "", "impersonate with grant")
	supTok, _ := imp.data["access_token"].(string)
	if supTok == "" {
		t.Fatalf("impersonation answer = %v", imp.data)
	}
	settings := expect(do(supTok, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "", "read under read_only")
	expect(do(supTok, http.MethodPut, "/v1/tenant/settings", settings.data), http.StatusForbidden, "SUPPORT_READ_ONLY", "PUT under read_only")
	expect(do(supTok, http.MethodPost, "/v1/tenant/support-access", map[string]any{"scope": "full", "duration_hours": 1}),
		http.StatusForbidden, "SUPPORT_READ_ONLY", "support session opens a grant")

	// 3. full grant replaces it: changes pass and are tagged.
	full := expect(do(ownerTok, http.MethodPost, "/v1/tenant/support-access", map[string]any{
		"scope": "full", "duration_hours": 1,
	}), http.StatusCreated, "", "open full")
	fullUUID, _ := full.data["uuid"].(string)
	list := expect(do(ownerTok, http.MethodGet, "/v1/tenant/support-access?status=revoked", nil), http.StatusOK, "", "list revoked")
	if items, _ := list.data["items"].([]any); len(items) != 1 || items[0].(map[string]any)["uuid"] != roUUID {
		t.Fatalf("revoked grants = %v", list.data)
	}
	expect(do(supTok, http.MethodPost, "/v1/tenant/branches", map[string]any{"code": "SUP", "name": "Destek Şube"}),
		http.StatusCreated, "", "create under full")
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM activity_events
		WHERE organization_id = $1 AND resource = 'tenant.branches' ORDER BY id DESC LIMIT 1`, org.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var tagged map[string]any
	_ = json.Unmarshal(payload, &tagged)
	if tagged["imp"] != staff.Uuid.String() || tagged["support_grant_uuid"] != fullUUID {
		t.Fatalf("activity payload = %s, want imp %s and grant %s", payload, staff.Uuid, fullUUID)
	}
	expect(do(supTok, http.MethodPost, "/v1/tenant/support-access/"+fullUUID+"/revoke", nil),
		http.StatusForbidden, "FORBIDDEN", "support session revokes")
	platform := expect(do(staffTok, http.MethodGet, "/v1/platform/support-grants", nil), http.StatusOK, "", "platform list")
	found := false
	for _, it := range platform.data["items"].([]any) {
		if it.(map[string]any)["uuid"] == fullUUID {
			found = true
		}
	}
	if !found {
		t.Fatalf("platform list misses the active grant: %v", platform.data)
	}

	// 4. Revoke ends access on the next request.
	expect(do(ownerTok, http.MethodPost, "/v1/tenant/support-access/"+fullUUID+"/revoke", nil), http.StatusOK, "", "revoke")
	expect(do(supTok, http.MethodGet, "/v1/tenant/settings", nil), http.StatusForbidden, "SUPPORT_GRANT_REQUIRED", "read after revoke")
	expect(do(ownerTok, http.MethodPost, "/v1/tenant/support-access/"+fullUUID+"/revoke", nil), http.StatusNotFound, "NOT_FOUND", "revoke twice")

	// 5. Expiry is checked at request time.
	exp := expect(do(ownerTok, http.MethodPost, "/v1/tenant/support-access", map[string]any{
		"scope": "full", "duration_hours": 1,
	}), http.StatusCreated, "", "open again")
	expect(do(supTok, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "", "read under new grant")
	if _, err := pool.Exec(ctx, `UPDATE support_grants SET expires_at = now() - interval '1 second'
		WHERE organization_id = $1 AND uuid = $2`, org.ID, exp.data["uuid"]); err != nil {
		t.Fatal(err)
	}
	expect(do(supTok, http.MethodGet, "/v1/tenant/settings", nil), http.StatusForbidden, "SUPPORT_GRANT_REQUIRED", "read after expiry")
	expect(do(staffTok, http.MethodPost, impersonate, nil), http.StatusForbidden, "SUPPORT_GRANT_REQUIRED", "impersonate after expiry")
	// Members keep working regardless.
	expect(do(ownerTok, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "", "member read")
}
