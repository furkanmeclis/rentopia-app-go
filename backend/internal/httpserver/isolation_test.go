package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RT-A01: cross-cutting organization isolation over the real router.
//
// Two organizations get data in every tenant resource. Then, for every
// /v1/tenant route registered on the server (enumerated from the route
// literals in the module sources, cross-checked against the mux and
// openapi.yaml, so a new route is covered automatically):
//
//   - a member of A naming organization B (X-Organization: B) gets 403;
//   - a member of A calling a route with path parameters filled with B's
//     uuids (and A's parent with B's child) gets 404 and never B's data;
//   - every GET without path parameters, called in A, returns none of B's
//     identifiers;
//   - mutations without path parameters have an explicit case (a body or
//     query naming B's records) or an allowlist entry with the reason.
//
// Afterwards B's records read the same as before. A route without a case
// fails the test (TestTenantIsolationRT_A01 → "uncovered route").

// isolationAllowlist lists tenant routes without path parameters whose body
// names no record of the organization (nothing to swap for B's uuids), with
// the test that covers them otherwise.
var isolationAllowlist = map[string]string{
	"POST /v1/tenant/branches":               "body has no foreign references (code, name)",
	"POST /v1/tenant/segments":               "body has no foreign references (code, name)",
	"POST /v1/tenant/roles":                  "body names permission slugs only; role isolation is the {uuid} cases",
	"PUT /v1/tenant/settings":                "settings document of the caller's organization; versions are the {version} cases",
	"POST /v1/tenant/notifications/read-all": "marks the caller's rows of the current organization (notifications scope test)",
	"POST /v1/tenant/branches/import":        "imports match natural keys inside the organization (adapter tests); the job is the {uuid} cases",
	"POST /v1/tenant/vehicles/import":        "imports match natural keys inside the organization (adapter tests); the job is the {uuid} cases",
}

type isoOrg struct {
	org    orgmodel.Organization
	owner  db.User
	marker string
	ids    map[string]string // key → uuid (or version number)
}

func TestTenantIsolationRT_A01(t *testing.T) {
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

	const secret = "isolation-test-access-secret-0123456789"
	cfg := config.Config{
		App:        config.AppConfig{Name: "Isolation", Env: "test"},
		JWT:        config.JWTConfig{AccessSecret: secret, RefreshSecret: secret, AccessTTL: time.Hour, RefreshTTL: time.Hour},
		Encryption: config.EncryptionConfig{Key: strings.Repeat("k", 32)},
		Tenancy:    config.TenancyConfig{Enabled: true},
		Bulk:       config.BulkConfig{SyncMax: 1, RollbackHours: 24},
	}
	if host := os.Getenv("TEST_MEILI_HOST"); host != "" {
		cfg.Search = config.SearchConfig{
			Enabled: true, Driver: "meilisearch", MeiliHost: host, MeiliKey: os.Getenv("TEST_MEILI_KEY"),
			IndexPrefix: "iso" + strings.ReplaceAll(uuid.NewString()[:8], "-", ""),
		}
	}
	mem := &memStorage{m: storage.NewMemory()}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, log, Deps{DB: pool, Queries: q, Storage: mem})
	if err != nil {
		t.Fatal(err)
	}
	handler := srv.http.Handler
	tokens, err := jwt.NewManager(secret, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// --- Organizations, users ---------------------------------------------
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	var users []int64
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-iso-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Iso", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, u.ID)
		return u
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	alice, bob := newUser("alice"), newUser("bob")
	newOrg := func(name string, owner db.User, marker string) *isoOrg {
		org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: name + " " + suffix, OwnerUserID: owner.ID})
		if err != nil {
			t.Fatal(err)
		}
		return &isoOrg{org: org, owner: owner, marker: marker, ids: map[string]string{}}
	}
	orgA := newOrg("Iso A", alice, "AMARK"+suffix)
	orgB := newOrg("Iso B", bob, "BMARK"+suffix)
	t.Cleanup(func() {
		bg := context.Background()
		for _, stmt := range []string{
			"DELETE FROM notifications WHERE user_id = ANY($1)",
			"DELETE FROM export_jobs WHERE actor_id = ANY($1)",
			"DELETE FROM import_jobs WHERE actor_id = ANY($1)",
			"DELETE FROM bulk_jobs WHERE actor_id = ANY($1)",
			"DELETE FROM activity_events WHERE actor_user_id = ANY($1)",
		} {
			_, _ = pool.Exec(bg, stmt, users)
		}
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.org.ID, orgB.org.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", users)
	})
	// A second member of B (a user of B only; never a member of A).
	carol := newUser("carol")
	if _, err := orgSvc.AddMember(ctx, orgB.org.UUID, carol.Uuid, orgctx.RoleMember); err != nil {
		t.Fatal(err)
	}
	orgB.ids["member"] = carol.Uuid.String()
	orgA.ids["member"] = alice.Uuid.String()

	// --- HTTP helpers -----------------------------------------------------
	type resp struct {
		code int
		raw  []byte
		data any
	}
	do := func(user db.User, orgRef, method, path, contentType string, body io.Reader) resp {
		t.Helper()
		// cms_user: the seeded platform role of signed-in users that grants
		// notifications.read (the tenant inbox routes need it). Tenant
		// permissions still come from the membership only.
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid, Roles: []string{"cms_user"}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Authorization", "Bearer "+access)
		if orgRef != "" {
			req.Header.Set(orgctx.HeaderName, orgRef)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		out := resp{code: rr.Code, raw: rr.Body.Bytes()}
		var env map[string]any
		if json.Unmarshal(out.raw, &env) == nil {
			out.data = env["data"]
		}
		return out
	}
	call := func(o *isoOrg, method, path string, body any) resp {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		return do(o.owner, o.org.Slug, method, path, "application/json", &buf)
	}
	upload := func(o *isoOrg, method, path, field, filename string, content []byte, extra map[string]string) resp {
		t.Helper()
		var form bytes.Buffer
		mw := multipart.NewWriter(&form)
		for k, v := range extra {
			_ = mw.WriteField(k, v)
		}
		fw, _ := mw.CreateFormFile(field, filename)
		_, _ = fw.Write(content)
		_ = mw.Close()
		return do(o.owner, o.org.Slug, method, path, mw.FormDataContentType(), &form)
	}
	str := func(r resp, keys ...string) string {
		var cur any = r.data
		for _, k := range keys {
			m, _ := cur.(map[string]any)
			cur = m[k]
		}
		s, _ := cur.(string)
		return s
	}
	must := func(r resp, want int, step string) resp {
		t.Helper()
		if r.code != want {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, want, r.raw)
		}
		return r
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	day := func(days int) string { return time.Now().UTC().AddDate(0, 0, days).Format(time.RFC3339) }
	date := func(days int) string { return time.Now().UTC().AddDate(0, 0, days).Format("2006-01-02") }

	// --- Seed both organizations through the API ---------------------------
	seed := func(o *isoOrg) {
		ids := o.ids
		m := o.marker
		branches := must(call(o, http.MethodGet, "/v1/tenant/branches?q=MRK", nil), http.StatusOK, "list branches")
		items, _ := branches.data.(map[string]any)["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("default branch: %s", branches.raw)
		}
		ids["home"] = items[0].(map[string]any)["uuid"].(string)
		ids["branch"] = str(must(call(o, http.MethodPost, "/v1/tenant/branches", map[string]any{
			"code": "ISO", "name": m + " Şube", "city": m,
		}), http.StatusCreated, "create branch"), "uuid")
		ids["point"] = str(must(call(o, http.MethodPost, "/v1/tenant/branches/"+ids["branch"]+"/service-points", map[string]any{
			"type": "airport", "name": m + " SAW",
		}), http.StatusCreated, "create service point"), "uuid")
		ids["segment"] = str(must(call(o, http.MethodPost, "/v1/tenant/segments", map[string]any{
			"code": "SEG", "name": m + " Segment",
		}), http.StatusCreated, "create segment"), "uuid")
		ids["group"] = str(must(call(o, http.MethodPost, "/v1/tenant/rental-groups", map[string]any{
			"code": "GRP", "name": m + " Grup", "segment_uuid": ids["segment"],
		}), http.StatusCreated, "create group"), "uuid")
		ids["product"] = str(must(call(o, http.MethodPost, "/v1/tenant/products", map[string]any{
			"code": "PRD", "title": m + " Ürün", "segment_uuid": ids["segment"],
			"guarantee": map[string]any{"type": "GROUP", "rental_group_uuid": ids["group"]},
		}), http.StatusCreated, "create product"), "uuid")
		must(call(o, http.MethodPatch, "/v1/tenant/products/"+ids["product"], map[string]any{
			"conditions": map[string]any{"min_driver_age": 25},
		}), http.StatusOK, "version product")
		versions := must(call(o, http.MethodGet, "/v1/tenant/products/"+ids["product"]+"/versions", nil), http.StatusOK, "versions")
		vItems := pageItems(t, versions.data)
		ids["productVersion"] = vItems[0].(map[string]any)["uuid"].(string)
		ids["image"] = str(must(upload(o, http.MethodPost, "/v1/tenant/products/"+ids["product"]+"/images", "file", "front.png", png, nil),
			http.StatusCreated, "upload image"), "uuid")
		ids["vehicle"] = str(must(call(o, http.MethodPost, "/v1/tenant/vehicles", map[string]any{
			"plate": "34 ISO 1", "manual_entry": true, "manual_brand": "Fiat", "manual_model": m, "model_year": 2024,
			"home_branch_uuid": ids["home"],
		}), http.StatusCreated, "create vehicle"), "uuid")
		v := "/v1/tenant/vehicles/" + ids["vehicle"]
		must(call(o, http.MethodPost, v+"/rental-groups", map[string]any{"rental_group_uuid": ids["group"]}), http.StatusCreated, "vehicle group")
		ids["document"] = str(must(call(o, http.MethodPost, v+"/documents", map[string]any{
			"doc_type": "kasko", "document_number": m, "valid_until": date(20),
		}), http.StatusCreated, "create document"), "uuid")
		must(upload(o, http.MethodPut, v+"/documents/"+ids["document"]+"/file", "file", "kasko.png", png, nil), http.StatusOK, "upload document file")
		ids["maintenance"] = str(must(call(o, http.MethodPost, v+"/maintenance", map[string]any{
			"kind": "planned", "maintenance_type": "periodic", "planned_date": date(30), "note": m,
		}), http.StatusCreated, "plan maintenance"), "uuid")
		block := must(call(o, http.MethodPost, v+"/blocks", map[string]any{
			"kind": "damage", "reason": m, "starts_at": day(10), "ends_at": day(11),
		}), http.StatusCreated, "create block")
		ids["block"] = str(block, "block", "uuid")
		ids["transfer"] = str(must(call(o, http.MethodPost, "/v1/tenant/transfers", map[string]any{
			"vehicle_uuid": ids["vehicle"], "to_branch_uuid": ids["branch"], "planned_departure": day(1), "planned_arrival": day(2),
			"note": m,
		}), http.StatusCreated, "create transfer"), "uuid")
		ids["rule"] = str(must(call(o, http.MethodPost, "/v1/tenant/one-way-rules", map[string]any{
			"from_branch_uuid": ids["home"], "to_branch_uuid": ids["branch"], "fee": 100, "note": m,
		}), http.StatusCreated, "create one-way rule"), "uuid")
		ids["role"] = str(must(call(o, http.MethodPost, "/v1/tenant/roles", map[string]any{
			"key": "iso_role", "name": m + " Rol", "permissions": []string{rbac.PermTenantMembersRead},
		}), http.StatusCreated, "create role"), "uuid")
		settings := must(call(o, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "settings")
		doc := settings.data.(map[string]any)["settings"].(map[string]any)
		doc["company"] = map[string]any{"legal_name": m + " A.Ş.", "tax_number": "4540536920", "tax_office": "Kadıköy", "address": "", "phone": "", "email": "info@iso.test"}
		must(call(o, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": doc, "base_version": 0}), http.StatusOK, "settings v1")
		ids["exportJob"] = str(must(call(o, http.MethodPost, "/v1/tenant/branches/export", map[string]any{"format": "csv"}),
			http.StatusAccepted, "export"), "uuid")
		ids["importJob"] = str(must(upload(o, http.MethodPost, "/v1/tenant/branches/import", "file", "branches.csv",
			[]byte("code,name\nIMP,"+m+" Import\n"), map[string]string{"format": "csv"}), http.StatusCreated, "import upload"), "uuid")
		// Two targets > Bulk.SyncMax: a bulk job row (run inline without a queue).
		group2 := str(must(call(o, http.MethodPost, "/v1/tenant/rental-groups", map[string]any{
			"code": "GRP2", "name": m + " Grup 2", "segment_uuid": ids["segment"],
		}), http.StatusCreated, "create group 2"), "uuid")
		ids["bulkJob"] = str(must(call(o, http.MethodPost, "/v1/tenant/rental-groups/bulk", map[string]any{
			"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{ids["group"], group2}},
		}), http.StatusAccepted, "bulk"), "uuid")
	}
	seed(orgA)
	seed(orgB)
	// B saves two more settings versions, so versions 2 and 3 exist in B only.
	for i := 1; i <= 2; i++ {
		settings := must(call(orgB, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "settings")
		doc := settings.data.(map[string]any)["settings"].(map[string]any)
		doc["company"].(map[string]any)["address"] = fmt.Sprintf("%s adres %d", orgB.marker, i)
		must(call(orgB, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": doc, "base_version": i}), http.StatusOK, "settings B")
	}
	orgA.ids["version"] = "1"
	orgB.ids["version"] = "3"
	// Org-stamped notification of B (bob's inbox in B).
	notif := notifusecase.New(q, nil, []providers.Provider{providers.InappProvider{}}, nil)
	bobID := bob.ID
	if _, err := notif.Enqueue(ctx, notifmodel.EnqueueInput{
		UserID: &bobID, OrganizationID: &orgB.org.ID, Channels: []string{notifmodel.ChannelInapp},
		Title: orgB.marker + " bildirim", Body: orgB.marker, SourceEvent: "test",
	}); err != nil {
		t.Fatal(err)
	}

	// B's identifiers: none of them may appear in a response to A.
	leaks := []string{orgB.marker, orgB.org.UUID.String(), orgB.org.Slug, bob.Uuid.String(), carol.Uuid.String()}
	for k, v := range orgB.ids {
		if k != "version" {
			leaks = append(leaks, v)
		}
	}
	leaked := func(raw []byte) string {
		s := string(raw)
		for _, l := range leaks {
			if strings.Contains(s, l) {
				return l
			}
		}
		return ""
	}
	// Sanity: B's own list shows its marker (the leak check is meaningful).
	if r := call(orgB, http.MethodGet, "/v1/tenant/branches", nil); leaked(r.raw) == "" {
		t.Fatalf("B's list does not contain B's identifiers: %s", r.raw)
	}

	// --- Route table ------------------------------------------------------
	routes := tenantRoutes(t, srv.mux)
	paramRE := regexp.MustCompile(`\{([^}]+)\}`)
	fill := func(pattern string, pick func(key string) string) (string, bool) {
		_, path, _ := strings.Cut(pattern, " ")
		segs := strings.Split(path, "/")
		for i, s := range segs {
			m := paramRE.FindStringSubmatch(s)
			if m == nil {
				continue
			}
			key := paramKey(segs[i-1], m[1])
			if key == "" {
				return "", false
			}
			segs[i] = pick(key)
		}
		return strings.Join(segs, "/"), true
	}
	bodyFor := func(pattern string) (string, io.Reader) {
		method, path, _ := strings.Cut(pattern, " ")
		if strings.HasSuffix(path, "/file") && method == http.MethodPut || strings.HasSuffix(path, "/images") && method == http.MethodPost {
			var form bytes.Buffer
			mw := multipart.NewWriter(&form)
			fw, _ := mw.CreateFormFile("file", "x.png")
			_, _ = fw.Write(png)
			_ = mw.Close()
			return mw.FormDataContentType(), &form
		}
		if method == http.MethodGet || method == http.MethodDelete {
			return "", nil
		}
		// Strict decoders refuse unknown fields: an empty object reaches the
		// lookup (handlers that validate first would answer 400 here, which
		// the 404 check reports).
		return "application/json", strings.NewReader("{}")
	}

	// Snapshot of B's records (as bob) before the attacks.
	snapshot := func() map[string]string {
		out := map[string]string{}
		for _, rt := range routes {
			if !strings.HasPrefix(rt, "GET ") || !strings.Contains(rt, "{") || strings.HasSuffix(rt, "/download") || strings.HasSuffix(rt, "/file") {
				continue
			}
			path, ok := fill(rt, func(k string) string { return orgB.ids[k] })
			if !ok {
				continue
			}
			r := call(orgB, http.MethodGet, path, nil)
			if r.code != http.StatusOK {
				t.Fatalf("B reads its own %s = %d: %s", path, r.code, r.raw)
			}
			b, _ := json.Marshal(r.data)
			out[path] = string(b)
		}
		return out
	}
	before := snapshot()

	covered := map[string]string{}
	// 1. Not a member of B: every route answers 403 with X-Organization: B.
	for _, rt := range routes {
		method, _, _ := strings.Cut(rt, " ")
		path, ok := fill(rt, func(k string) string { return orgB.ids[k] })
		if !ok {
			path, _ = fill(rt, func(string) string { return uuid.NewString() })
		}
		ct, body := bodyFor(rt)
		r := do(alice, orgB.org.Slug, method, path, ct, body)
		if r.code != http.StatusForbidden {
			t.Errorf("non-member %s (X-Organization: B) = %d, want 403: %s", rt, r.code, r.raw)
		}
		if l := leaked(r.raw); l != "" {
			t.Errorf("non-member %s leaked %q", rt, l)
		}
	}

	// 2. Path parameters with B's uuids (and A's parent with B's child).
	for _, rt := range routes {
		if !strings.Contains(rt, "{") {
			continue
		}
		method, _, _ := strings.Cut(rt, " ")
		path, ok := fill(rt, func(k string) string { return orgB.ids[k] })
		if !ok {
			t.Errorf("uncovered route %s: no isolation value for its path parameters (extend paramKey and the seed)", rt)
			continue
		}
		variants := []string{path}
		if strings.Count(rt, "{") > 1 {
			n := 0
			mixed, _ := fill(rt, func(k string) string {
				n++
				if n == 1 {
					return orgA.ids[k]
				}
				return orgB.ids[k]
			})
			variants = append(variants, mixed)
		}
		for _, p := range variants {
			ct, body := bodyFor(rt)
			r := do(alice, orgA.org.Slug, method, p, ct, body)
			if r.code != http.StatusNotFound {
				t.Errorf("%s %s (B's ids, member of A) = %d, want 404: %s", method, p, r.code, r.raw)
			}
			if l := leaked(r.raw); l != "" {
				t.Errorf("%s %s leaked %q: %s", method, p, l, r.raw)
			}
		}
		covered[rt] = "path parameters"
	}

	// 3. GET routes without path parameters: no B identifier in A's answer
	// (with and without filters naming B's records).
	b := orgB.ids
	probes := map[string][]string{
		"GET /v1/tenant/one-way-rules/resolve": {"?from_branch=" + b["home"] + "&to_branch=" + b["branch"] + "&rental_product=" + b["product"]},
		"GET /v1/tenant/fleet/calendar": {
			"?from=" + day(0) + "&to=" + day(14),
			"?from=" + day(0) + "&to=" + day(14) + "&vehicle=" + b["vehicle"],
			"?from=" + day(0) + "&to=" + day(14) + "&home_branch=" + b["home"] + "&rental_group=" + b["group"] + "&segment=" + b["segment"],
		},
		"GET /v1/tenant/vehicles":      {"?home_branch=" + b["home"], "?rental_group=" + b["group"], "?segment=" + b["segment"]},
		"GET /v1/tenant/transfers":     {"?vehicle=" + b["vehicle"], "?from_branch=" + b["home"], "?to_branch=" + b["branch"]},
		"GET /v1/tenant/one-way-rules": {"?from_branch=" + b["home"], "?to_branch=" + b["branch"]},
		"GET /v1/tenant/products":      {"?segment=" + b["segment"], "?rental_group=" + b["group"]},
		"GET /v1/tenant/rental-groups": {"?segment=" + b["segment"]},
		"GET /v1/tenant/fleet/alerts":  {"?vehicle=" + b["vehicle"]},
		"GET /v1/tenant/search":        {"?q=" + orgB.marker},
	}
	for _, rt := range routes {
		method, path, _ := strings.Cut(rt, " ")
		if method != http.MethodGet || strings.Contains(rt, "{") {
			continue
		}
		for _, query := range append([]string{""}, probes[rt]...) {
			r := call(orgA, http.MethodGet, path+query, nil)
			if r.code >= 500 {
				t.Errorf("%s%s = %d: %s", rt, query, r.code, r.raw)
			}
			if l := leaked(r.raw); l != "" {
				t.Errorf("%s%s (member of A) leaked %q: %s", rt, query, l, r.raw)
			}
			// A probe must reach the filter: 200, or the record named by the
			// query is unknown in A ("not_found").
			if query != "" && r.code != http.StatusOK && !(r.code < 500 && bytes.Contains(r.raw, []byte(`"not_found"`))) {
				t.Errorf("probe %s%s = %d (the probe must reach the query): %s", rt, query, r.code, r.raw)
			}
		}
		covered[rt] = "GET leak scan"
	}

	// 4. Mutations without path parameters: bodies naming B's records.
	type bodyCase struct {
		route string
		body  map[string]any
		want  []int
	}
	reject := []int{http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity}
	bulkIDs := func(key string) map[string]any {
		return map[string]any{"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{orgB.ids[key]}}}
	}
	cases := []bodyCase{
		{"POST /v1/tenant/rental-groups", map[string]any{"code": "X1", "name": "x", "segment_uuid": orgB.ids["segment"]}, reject},
		{"POST /v1/tenant/products", map[string]any{"code": "X1", "title": "x", "segment_uuid": orgB.ids["segment"],
			"guarantee": map[string]any{"type": "GROUP", "rental_group_uuid": orgA.ids["group"]}}, reject},
		{"POST /v1/tenant/products", map[string]any{"code": "X2", "title": "x", "segment_uuid": orgA.ids["segment"],
			"guarantee": map[string]any{"type": "GROUP", "rental_group_uuid": orgB.ids["group"]}}, reject},
		{"POST /v1/tenant/products", map[string]any{"code": "X3", "title": "x", "segment_uuid": orgA.ids["segment"],
			"guarantee": map[string]any{"type": "EXACT_VEHICLE", "vehicle_uuid": orgB.ids["vehicle"]}}, reject},
		{"POST /v1/tenant/vehicles", map[string]any{"plate": "34 X 1", "manual_entry": true, "manual_brand": "Fiat", "manual_model": "x",
			"model_year": 2024, "home_branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/vehicles", map[string]any{"plate": "34 X 2", "manual_entry": true, "manual_brand": "Fiat", "manual_model": "x",
			"model_year": 2024, "home_branch_uuid": orgA.ids["home"], "current_branch_uuid": orgB.ids["branch"]}, reject},
		{"POST /v1/tenant/transfers", map[string]any{"vehicle_uuid": orgB.ids["vehicle"], "to_branch_uuid": orgA.ids["branch"],
			"planned_departure": day(3), "planned_arrival": day(4)}, reject},
		{"POST /v1/tenant/transfers", map[string]any{"vehicle_uuid": orgA.ids["vehicle"], "to_branch_uuid": orgB.ids["branch"],
			"planned_departure": day(3), "planned_arrival": day(4)}, reject},
		{"POST /v1/tenant/transfers", map[string]any{"vehicle_uuid": orgA.ids["vehicle"], "to_branch_uuid": orgA.ids["home"],
			"planned_departure": day(3), "planned_arrival": day(4), "responsible_user_uuid": bob.Uuid}, reject},
		{"POST /v1/tenant/one-way-rules", map[string]any{"from_branch_uuid": orgB.ids["home"], "to_branch_uuid": orgA.ids["branch"], "fee": 1}, reject},
		{"POST /v1/tenant/one-way-rules", map[string]any{"from_branch_uuid": orgA.ids["branch"], "to_branch_uuid": orgA.ids["home"],
			"rental_product_uuid": orgB.ids["product"], "fee": 1}, reject},
		{"POST /v1/tenant/branches/bulk", bulkIDs("branch"), nil},
		{"POST /v1/tenant/rental-groups/bulk", bulkIDs("group"), nil},
		{"POST /v1/tenant/products/bulk", bulkIDs("product"), nil},
		{"POST /v1/tenant/vehicles/bulk", map[string]any{"action": "set_status", "params": map[string]any{"status": "maintenance"},
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["vehicle"]}}}, nil},
		{"POST /v1/tenant/one-way-rules/bulk", bulkIDs("rule"), nil},
		// Action parameters naming B's records, on A's vehicle.
		{"POST /v1/tenant/vehicles/bulk", map[string]any{"action": "move_branch", "params": map[string]any{"branch_uuid": orgB.ids["branch"]},
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["vehicle"]}}}, reject},
		{"POST /v1/tenant/vehicles/bulk", map[string]any{"action": "add_rental_group", "params": map[string]any{"rental_group_uuid": orgB.ids["group"]},
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["vehicle"]}}}, reject},
	}
	for _, c := range cases {
		method, path, _ := strings.Cut(c.route, " ")
		r := call(orgA, method, path, c.body)
		if l := leaked(r.raw); l != "" {
			t.Errorf("%s leaked %q: %s", c.route, l, r.raw)
		}
		if c.want == nil { // bulk: refused, or run with nothing succeeding
			summary, _ := asMap(r.data)["summary"].(map[string]any)
			if r.code != http.StatusBadRequest && (r.code != http.StatusOK || summary == nil || summary["succeeded"].(float64) != 0) {
				t.Errorf("%s with B's ids = %d, want 400 or 200 with 0 succeeded: %s", c.route, r.code, r.raw)
			}
		} else if !containsInt(c.want, r.code) {
			t.Errorf("%s naming B's records = %d, want one of %v: %s", c.route, r.code, c.want, r.raw)
		}
		covered[c.route] = "body names B's records"
	}
	// Mutations on A's own records whose body names B's records.
	ownCases := []bodyCase{
		{"PATCH /v1/tenant/rental-groups/" + orgA.ids["group"], map[string]any{"segment_uuid": orgB.ids["segment"]}, reject},
		{"PATCH /v1/tenant/products/" + orgA.ids["product"], map[string]any{"segment_uuid": orgB.ids["segment"]}, reject},
		{"PATCH /v1/tenant/vehicles/" + orgA.ids["vehicle"], map[string]any{"home_branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/vehicles/" + orgA.ids["vehicle"] + "/rental-groups", map[string]any{"rental_group_uuid": orgB.ids["group"]}, reject},
		{"PATCH /v1/tenant/transfers/" + orgA.ids["transfer"], map[string]any{"to_branch_uuid": orgB.ids["branch"]}, reject},
		{"PATCH /v1/tenant/transfers/" + orgA.ids["transfer"], map[string]any{"responsible_user_uuid": carol.Uuid}, reject},
		{"PATCH /v1/tenant/one-way-rules/" + orgA.ids["rule"], map[string]any{"rental_product_uuid": orgB.ids["product"]}, reject},
		{"PATCH /v1/tenant/members/" + alice.Uuid.String(), map[string]any{"role_uuids": []string{orgB.ids["role"]}}, reject},
		{"PATCH /v1/tenant/members/" + alice.Uuid.String(), map[string]any{"branch_scope": "selected", "branch_uuids": []string{orgB.ids["home"]}}, reject},
		{"PUT /v1/tenant/products/" + orgA.ids["product"] + "/images/order", map[string]any{"image_uuids": []string{orgB.ids["image"]}}, reject},
	}
	for _, c := range ownCases {
		method, path, _ := strings.Cut(c.route, " ")
		r := call(orgA, method, path, c.body)
		if l := leaked(r.raw); l != "" {
			t.Errorf("%s leaked %q: %s", c.route, l, r.raw)
		}
		if !containsInt(c.want, r.code) {
			t.Errorf("%s naming B's records = %d, want one of %v: %s", c.route, r.code, c.want, r.raw)
		}
	}

	// 5. Exports of A contain A's rows only (the worker context is scoped
	// to the job's organization).
	for _, rt := range routes {
		method, path, _ := strings.Cut(rt, " ")
		if method != http.MethodPost || !strings.HasSuffix(path, "/export") {
			continue
		}
		r := call(orgA, http.MethodPost, path, map[string]any{"format": "csv"})
		if r.code != http.StatusAccepted || str(r, "status") != "completed" {
			t.Errorf("%s in A = %d: %s", rt, r.code, r.raw)
			continue
		}
		file := do(alice, orgA.org.Slug, http.MethodGet, "/v1/tenant/exports/"+str(r, "uuid")+"/download", "", nil)
		if file.code != http.StatusOK {
			t.Errorf("download %s = %d: %s", rt, file.code, file.raw)
			continue
		}
		if l := leaked(file.raw); l != "" {
			t.Errorf("%s: A's export file contains %q", rt, l)
		}
		if bytes.Count(file.raw, []byte("\n")) < 1 {
			t.Errorf("%s: empty export", rt)
		}
		covered[rt] = "export file scan"
	}
	// A's own export download stays A's (B cannot read it either).
	if r := do(bob, orgB.org.Slug, http.MethodGet, "/v1/tenant/exports/"+orgA.ids["exportJob"]+"/download", "", nil); r.code != http.StatusNotFound {
		t.Errorf("B downloads A's export = %d, want 404", r.code)
	}

	// 6. Search documents (Meilisearch only).
	if cfg.Search.Enabled {
		deadline := time.Now().Add(10 * time.Second)
		for {
			r := call(orgB, http.MethodGet, "/v1/tenant/search?q="+orgB.marker, nil)
			if strings.Contains(string(r.raw), orgB.ids["branch"]) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("B's documents never indexed: %s", r.raw)
			}
			time.Sleep(200 * time.Millisecond)
		}
		if r := call(orgA, http.MethodGet, "/v1/tenant/search?q="+orgB.marker, nil); leaked(r.raw) != "" {
			t.Errorf("A's search returns B's documents: %s", r.raw)
		}
	} else {
		t.Log("search: TEST_MEILI_HOST not set, Meilisearch part skipped")
	}

	// B is unchanged.
	after := snapshot()
	for path, want := range before {
		if after[path] != want {
			t.Errorf("B's %s changed:\n before %s\n after  %s", path, want, after[path])
		}
	}

	// Coverage: every tenant route has an isolation case or a reason.
	for _, rt := range routes {
		if covered[rt] != "" {
			continue
		}
		if reason := isolationAllowlist[rt]; reason != "" {
			continue
		}
		t.Errorf("uncovered route %s: add an isolation case (body naming another organization's records) or an allowlist entry with the reason", rt)
	}
	for rt, reason := range isolationAllowlist {
		if reason == "" {
			continue
		}
		if !containsString(routes, rt) {
			t.Errorf("isolationAllowlist: %s is not a registered route", rt)
		}
	}
}

// paramKey maps a path parameter (with the segment before it) to the seed
// key of the record it names. "" = unknown: the route needs a case.
func paramKey(prev, name string) string {
	switch prev + "/" + name {
	case "branches/uuid":
		return "branch"
	case "service-points/pointUuid":
		return "point"
	case "segments/uuid":
		return "segment"
	case "rental-groups/uuid", "rental-groups/groupUuid":
		return "group"
	case "products/uuid":
		return "product"
	case "versions/versionUuid":
		return "productVersion"
	case "images/imageUuid":
		return "image"
	case "vehicles/uuid":
		return "vehicle"
	case "documents/documentUuid":
		return "document"
	case "maintenance/itemUuid":
		return "maintenance"
	case "blocks/blockUuid":
		return "block"
	case "transfers/uuid":
		return "transfer"
	case "one-way-rules/uuid":
		return "rule"
	case "roles/uuid":
		return "role"
	case "members/userUuid":
		return "member"
	case "exports/uuid":
		return "exportJob"
	case "imports/uuid":
		return "importJob"
	case "bulk/uuid":
		return "bulkJob"
	case "versions/version":
		return "version"
	}
	return ""
}

var tenantRouteRE = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) /v1/tenant(/\S*)?$`)

// tenantRoutes enumerates the /v1/tenant routes: every route pattern
// literal in the backend sources, each checked to be registered on mux, plus
// a cross-check that every /v1/tenant operation of openapi.yaml resolves to
// one of them (a pattern built at runtime would show up there).
func tenantRoutes(t *testing.T, mux *http.ServeMux) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	set := map[string]bool{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil && tenantRouteRE.MatchString(s) {
					set[s] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	routes := make([]string, 0, len(set))
	for rt := range set {
		method, path, _ := strings.Cut(rt, " ")
		concrete := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "00000000-0000-0000-0000-000000000000")
		req := httptest.NewRequest(method, concrete, nil)
		if _, pattern := mux.Handler(req); pattern != rt {
			t.Errorf("route literal %q is not registered on the server (mux pattern %q)", rt, pattern)
			continue
		}
		routes = append(routes, rt)
	}
	sort.Strings(routes)
	if len(routes) < 100 {
		t.Fatalf("found only %d tenant routes", len(routes))
	}

	spec, err := os.ReadFile(filepath.Join(root, "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pathRE := regexp.MustCompile(`^  (/v1/tenant[^:]*):\s*$`)
	opRE := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`)
	current := ""
	for _, line := range strings.Split(string(spec), "\n") {
		if m := pathRE.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if strings.HasPrefix(line, "  /") || (len(line) > 0 && line[0] != ' ') {
			current = ""
			continue
		}
		if m := opRE.FindStringSubmatch(line); m != nil && current != "" {
			method := strings.ToUpper(m[1])
			concrete := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(current, "00000000-0000-0000-0000-000000000000")
			_, pattern := mux.Handler(httptest.NewRequest(method, concrete, nil))
			if !containsString(routes, pattern) {
				t.Errorf("openapi %s %s resolves to %q, which is not an enumerated tenant route", method, current, pattern)
			}
		}
	}
	return routes
}

func pageItems(t *testing.T, data any) []any {
	t.Helper()
	switch v := data.(type) {
	case []any:
		if len(v) > 0 {
			return v
		}
	case map[string]any:
		if items, ok := v["items"].([]any); ok && len(items) > 0 {
			return items
		}
	}
	t.Fatalf("expected a non-empty list: %v", data)
	return nil
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// memStorage is an in-memory storage.Driver for the isolation test.
type memStorage struct {
	storage.Driver
	m *storage.Memory
}

func (d *memStorage) Ping(ctx context.Context) error { return d.m.Ping(ctx) }

func (d *memStorage) Upload(ctx context.Context, f storage.File, path string) error {
	return d.m.Upload(ctx, f, path)
}

func (d *memStorage) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, path)
}

func (d *memStorage) Delete(ctx context.Context, path string) error { return d.m.Delete(ctx, path) }

func (d *memStorage) Exists(ctx context.Context, path string) (bool, error) {
	return d.m.Exists(ctx, path)
}

func (d *memStorage) Bucket() string { return "isolation" }
