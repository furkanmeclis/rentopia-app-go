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
	"POST /v1/tenant/branches":                     "body has no foreign references (code, name)",
	"POST /v1/tenant/segments":                     "body has no foreign references (code, name)",
	"POST /v1/tenant/roles":                        "body names permission slugs only; role isolation is the {uuid} cases",
	"PUT /v1/tenant/settings":                      "settings document of the caller's organization; versions are the {version} cases",
	"POST /v1/tenant/notifications/read-all":       "marks the caller's rows of the current organization (notifications scope test)",
	"POST /v1/tenant/branches/import":              "imports match natural keys inside the organization (adapter tests); the job is the {uuid} cases",
	"POST /v1/tenant/vehicles/import":              "imports match natural keys inside the organization (adapter tests); the job is the {uuid} cases",
	"PATCH /v1/tenant/organization":                "profile of the caller's organization (name only); no foreign references",
	"PUT /v1/tenant/organization/logo":             "logo of the caller's organization; the key is built from the scope (storage prefix check)",
	"DELETE /v1/tenant/organization/logo":          "logo of the caller's organization; no ids",
	"POST /v1/tenant/rate-plans/import":            "price list rows match product / branch codes inside the organization (pricing usecase tests); the job is the {uuid} cases",
	"POST /v1/tenant/customers":                    "body has no foreign references (names, identity, contact values)",
	"POST /v1/tenant/customers/import":             "rows match identity blind indexes keyed by the organization (customers usecase tests); the job is the {uuid} cases",
	"POST /v1/tenant/extras":                       "body has no foreign references (code, name, enums, channel codes, default price)",
	"POST /v1/tenant/integrations":                 "body has no foreign references (provider code, display name, mode, the organization's own credentials)",
	"POST /v1/tenant/contract-templates":           "body has no foreign references (kind, name, editor content, body with placeholders)",
	"POST /v1/tenant/inspection-templates":         "body has no foreign references (name, photo slots, checklist items)",
	"POST /v1/tenant/finance/expense-categories":   "body has no foreign references (code, name, sort order)",
	"POST /v1/tenant/finance/cash-accounts/import": "rows match account / branch codes inside the organization (finance adapters); the job is the {uuid} cases",
	"POST /v1/tenant/finance/movements/import":     "rows match account / category codes inside the organization (finance adapters); the job is the {uuid} cases",
	"POST /v1/tenant/finance/expenses/import":      "rows match category / branch / plate / account codes inside the organization; the job is the {uuid} cases",
	"POST /v1/tenant/toll-passes/import":           "rows match provider transaction ids and plates / tags inside the organization (tolls tests); the job is the {uuid} cases",
	"POST /v1/tenant/toll-passes/fetch":            "no body; fetches with the caller's organization's active toll account (tolls fetch test)",
	"PATCH /v1/tenant/toll-settings":               "processing fee / fetch options of the caller's organization; no foreign references",
	"PUT /v1/tenant/invoices/settings":             "invoice settings of the caller's organization (series, supplier address, tax mapping); no foreign references",
	"POST /v1/tenant/suppliers":                    "body has no foreign references (title, tax number, contact and billing values)",
	"POST /v1/tenant/suppliers/import":             "rows match supplier code / tax number inside the organization (suppliers adapters); the job is the {uuid} cases",
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
		// SMS (TEC-734): the log provider sends without a network.
		SMS: config.SMSConfig{LogProvider: true},
	}
	if host := os.Getenv("TEST_MEILI_HOST"); host != "" {
		cfg.Search = config.SearchConfig{
			Enabled: true, Driver: "meilisearch", MeiliHost: host, MeiliKey: os.Getenv("TEST_MEILI_KEY"),
			IndexPrefix: "iso" + strings.ReplaceAll(uuid.NewString()[:8], "-", ""),
		}
	}
	mem := &memStorage{m: storage.NewMemory()}
	// Server errors are printed with -v (the defence-in-depth checks below
	// log expected errors).
	logOut := io.Discard
	if testing.Verbose() {
		logOut = os.Stderr
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelError}))
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
		cur := r.data
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
		ids["ratePlan"] = str(must(call(o, http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
			"rental_product_uuid": ids["product"], "scope": "common", "name": m + " Tarife",
			"bands": []map[string]any{{"min_days": 1, "max_days": 3, "daily_price": "1500"}, {"min_days": 4, "daily_price": "1300"}},
		}), http.StatusCreated, "create rate plan"), "uuid")
		must(call(o, http.MethodPost, "/v1/tenant/rate-plans/"+ids["ratePlan"]+"/activate", nil), http.StatusOK, "activate rate plan")
		must(call(o, http.MethodPatch, "/v1/tenant/rate-plans/"+ids["ratePlan"], map[string]any{"priority": 1}), http.StatusOK, "draft rate plan")
		ids["dateRule"] = str(must(call(o, http.MethodPost, "/v1/tenant/date-rules", map[string]any{
			"name": m + " YAZ", "start_date": date(40), "end_date": date(60), "kind": "percent", "value": "20",
			"product_scope": "segments", "segment_uuids": []string{ids["segment"]},
			"all_branches": false, "branch_uuids": []string{ids["branch"]},
		}), http.StatusCreated, "create date rule"), "uuid")
		// Availability (TEC-697): the published product gets a hold at the
		// transfer's destination (the vehicle is there from day 2) and a
		// channel capacity rule.
		must(call(o, http.MethodPatch, "/v1/tenant/products/"+ids["product"], map[string]any{"status": "published"}),
			http.StatusOK, "publish product")
		ids["hold"] = str(must(call(o, http.MethodPost, "/v1/tenant/availability/holds", map[string]any{
			"product_uuid": ids["product"], "pickup_branch_uuid": ids["branch"], "starts_at": day(4), "ends_at": day(5),
			"hold_minutes": 600, "note": m,
		}), http.StatusCreated, "create hold"), "allocation", "uuid")
		ids["capacityRule"] = str(must(call(o, http.MethodPost, "/v1/tenant/capacity-rules", map[string]any{
			"rule_type": "safety_margin", "quantity": 0, "product_uuid": ids["product"], "note": m,
		}), http.StatusCreated, "create capacity rule"), "uuid")
		// Extras (TEC-702): a physical child seat with a product price, stock
		// at the second branch and an equipment hold there.
		ids["extra"] = str(must(call(o, http.MethodPost, "/v1/tenant/extras", map[string]any{
			"code": "SEAT", "name": m + " Koltuk", "extra_type": "child_seat", "charge_type": "per_day", "max_quantity": 2,
			"is_physical": true, "status": "published", "default_price": "100",
		}), http.StatusCreated, "create extra"), "uuid")
		must(call(o, http.MethodPut, "/v1/tenant/extras/"+ids["extra"]+"/prices", map[string]any{
			"items": []map[string]any{{"rental_product_uuid": ids["product"], "price": "150"}},
		}), http.StatusOK, "extra prices")
		must(call(o, http.MethodPut, "/v1/tenant/extras/"+ids["extra"]+"/stock", map[string]any{
			"items": []map[string]any{{"branch_uuid": ids["branch"], "quantity": 2}},
		}), http.StatusOK, "extra stock")
		ids["equipmentAllocation"] = str(must(call(o, http.MethodPost, "/v1/tenant/equipment-allocations", map[string]any{
			"extra_uuid": ids["extra"], "branch_uuid": ids["branch"], "starts_at": day(4), "ends_at": day(5), "hold_minutes": 600, "note": m,
		}), http.StatusCreated, "equipment hold"), "uuid")
		// Campaigns (TEC-703): a coupon scoped to the product.
		ids["campaign"] = str(must(call(o, http.MethodPost, "/v1/tenant/campaigns", map[string]any{
			"name": m + " Kampanya", "campaign_type": "coupon", "code": "ISO10", "discount_kind": "percent", "discount_value": "10",
			"product_scope": "products", "rental_product_uuids": []string{ids["product"]}, "usage_limit": 5,
		}), http.StatusCreated, "create campaign"), "uuid")
		ids["role"] = str(must(call(o, http.MethodPost, "/v1/tenant/roles", map[string]any{
			"key": "iso_role", "name": m + " Rol", "permissions": []string{rbac.PermTenantMembersRead},
		}), http.StatusCreated, "create role"), "uuid")
		ids["invitation"] = str(must(call(o, http.MethodPost, "/v1/tenant/invitations", map[string]any{
			"email": strings.ToLower(m) + "-invitee@iso.test", "role_uuids": []string{ids["role"]},
			"branch_scope": "selected", "branch_uuids": []string{ids["branch"]},
		}), http.StatusCreated, "create invitation"), "uuid")
		settings := must(call(o, http.MethodGet, "/v1/tenant/settings", nil), http.StatusOK, "settings")
		doc := settings.data.(map[string]any)["settings"].(map[string]any)
		doc["company"] = map[string]any{"legal_name": m + " A.Ş.", "tax_number": "4540536920", "tax_office": "Kadıköy", "address": "", "phone": "", "email": "info@iso.test"}
		must(call(o, http.MethodPut, "/v1/tenant/settings", map[string]any{"settings": doc, "base_version": 0}), http.StatusOK, "settings v1")
		// Customers (TEC-705): a corporate customer with a contact, a consent,
		// a driver and a licence scan.
		ids["customer"] = str(must(call(o, http.MethodPost, "/v1/tenant/customers", map[string]any{
			"customer_type": "corporate", "company_title": m + " Lojistik", "identity_type": "vkn",
			"identity_number": "4540536920", "phone": "0532 111 22 33", "email": "filo@iso.test", "tags": []string{m},
		}), http.StatusCreated, "create customer"), "uuid")
		c := "/v1/tenant/customers/" + ids["customer"]
		ids["contact"] = str(must(call(o, http.MethodPost, c+"/contacts", map[string]any{
			"full_name": m + " Kişi", "phone": "0532 444 55 66",
		}), http.StatusCreated, "create contact"), "uuid")
		must(call(o, http.MethodPost, c+"/consents", map[string]any{"channel": "sms", "action": "grant", "text_version": "v1"}),
			http.StatusOK, "grant consent")
		ids["driver"] = str(must(call(o, http.MethodPost, c+"/drivers", map[string]any{
			"first_name": m, "last_name": "Sürücü", "birth_date": "1990-01-01", "licence_number": "B123456",
			"licence_issued_on": "2010-01-01",
		}), http.StatusCreated, "create driver"), "uuid")
		ids["customerDocument"] = str(must(call(o, http.MethodPost, c+"/documents", map[string]any{
			"doc_type": "driving_licence", "driver_uuid": ids["driver"],
		}), http.StatusCreated, "create customer document"), "uuid")
		must(upload(o, http.MethodPut, c+"/documents/"+ids["customerDocument"]+"/file", "file", "ehliyet.png", png, nil),
			http.StatusOK, "upload customer document file")
		// Reservations (TEC-704 / TEC-706 / TEC-698): a quote at the transfer's
		// destination after the hold, a reservation from it (confirmed: auto
		// approval, no prepayment), an open quote, and a resolution task from
		// a block over the reservation (the only vehicle cannot be re-planned).
		ids["quote"] = str(must(call(o, http.MethodPost, "/v1/tenant/quotes", map[string]any{
			"pickup_branch_uuid": ids["branch"], "starts_at": day(6), "ends_at": day(7), "product_uuid": ids["product"],
			"customer_uuid": ids["customer"],
		}), http.StatusCreated, "create quote"), "uuid")
		ids["reservation"] = str(must(call(o, http.MethodPost, "/v1/tenant/reservations", map[string]any{
			"quote_uuid": ids["quote"], "customer_uuid": ids["customer"], "drivers": []map[string]any{{"driver_uuid": ids["driver"]}},
			"note": m,
		}), http.StatusCreated, "create reservation"), "uuid")
		ids["quoteOpen"] = str(must(call(o, http.MethodPost, "/v1/tenant/quotes", map[string]any{
			"pickup_branch_uuid": ids["branch"], "starts_at": day(8), "ends_at": day(9), "product_uuid": ids["product"],
		}), http.StatusCreated, "create open quote"), "uuid")
		must(call(o, http.MethodPost, v+"/blocks", map[string]any{
			"kind": "damage", "reason": m, "starts_at": day(6), "ends_at": day(7),
		}), http.StatusCreated, "block over the reservation")
		tasks := must(call(o, http.MethodGet, "/v1/tenant/resolution-tasks?reservation="+ids["reservation"], nil), http.StatusOK, "tasks")
		taskItems := pageItems(t, tasks.data)
		if len(taskItems) != 1 {
			t.Fatalf("resolution task from the block: %s", tasks.raw)
		}
		ids["task"] = taskItems[0].(map[string]any)["uuid"].(string)
		// Customer portal (TEC-715): a cancellation request of the
		// reservation, as the portal stores it.
		var resID, custID int64
		if err := pool.QueryRow(ctx, "SELECT id, customer_id FROM reservations WHERE organization_id = $1 AND uuid = $2",
			o.org.ID, ids["reservation"]).Scan(&resID, &custID); err != nil {
			t.Fatal(err)
		}
		creq, err := q.InsertCustomerRequest(ctx, db.InsertCustomerRequestParams{
			OrganizationID: o.org.ID, ReservationID: resID, CustomerID: custID, RequestType: "cancel", Status: "open",
			Message: m + " iptal", Currency: "TRY",
		})
		if err != nil {
			t.Fatal(err)
		}
		ids["customerRequest"] = creq.Uuid.String()
		// Integrations / payments (TEC-676 / TEC-707): an iyzico account
		// (stored, never called here), a pay-by-link link for the
		// reservation's balance and a branch collection.
		ids["integrationAccount"] = str(must(call(o, http.MethodPost, "/v1/tenant/integrations", map[string]any{
			"provider": "iyzico", "display_name": m + " iyzico", "mode": "sandbox",
			"credentials": map[string]any{"api_key": m + "-api-key", "secret_key": m + "-secret-key"},
		}), http.StatusCreated, "connect integration"), "uuid")
		// SMS account (log provider) and a test send: its integration job
		// (TEC-751) is the {uuid} of /v1/tenant/integration-jobs.
		ids["smsAccount"] = str(must(call(o, http.MethodPost, "/v1/tenant/integrations", map[string]any{
			"provider": "log", "display_name": m + " sms", "credentials": map[string]any{"msgheader": "RENTOPIA"},
		}), http.StatusCreated, "connect sms"), "uuid")
		ids["integrationJob"] = str(must(call(o, http.MethodPost, "/v1/tenant/integrations/"+ids["smsAccount"]+"/test-sms", map[string]any{
			"phone": "05321234567",
		}), http.StatusAccepted, "test sms"), "job_uuid")
		// Toll passes (TEC-725): a tag on the vehicle, the development
		// provider account and a fetch (inline without a queue) → passes
		// of the tag, in review (the vehicle was on no rental).
		ids["tollTag"] = str(must(call(o, http.MethodPost, "/v1/tenant/vehicles/"+ids["vehicle"]+"/toll-tags", map[string]any{
			"provider": "hgs_fake", "tag_number": m + "TAG", "valid_from": time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339),
			"note": m,
		}), http.StatusCreated, "toll tag"), "uuid")
		must(call(o, http.MethodPost, "/v1/tenant/integrations", map[string]any{
			"provider": "hgs_fake", "display_name": m + " HGS", "credentials": map[string]string{"api_key": "fake-" + m},
		}), http.StatusCreated, "toll provider account")
		must(call(o, http.MethodPost, "/v1/tenant/toll-passes/fetch", nil), http.StatusAccepted, "toll fetch")
		tollPasses := pageItems(t, must(call(o, http.MethodGet, "/v1/tenant/toll-passes?limit=5", nil), http.StatusOK, "toll passes").data)
		if len(tollPasses) == 0 {
			t.Fatal("the toll fetch created no pass")
		}
		ids["tollPass"] = tollPasses[0].(map[string]any)["uuid"].(string)
		ids["paymentLink"] = str(must(call(o, http.MethodPost, "/v1/tenant/reservations/"+ids["reservation"]+"/payment-links", map[string]any{
			"amount": "100",
		}), http.StatusCreated, "payment link"), "uuid")
		ids["payment"] = str(must(call(o, http.MethodPost, "/v1/tenant/reservations/"+ids["reservation"]+"/collections", map[string]any{
			"amount": "10", "method": "cash", "note": m,
		}), http.StatusCreated, "collection"), "uuid")
		// Rentals / contracts (TEC-709 / TEC-710): an active terms template,
		// a reservation whose fixed plate created a rental (preparation +
		// handover tasks) and its terms document (one version).
		ids["contractTemplate"] = str(must(call(o, http.MethodPost, "/v1/tenant/contract-templates", map[string]any{
			"kind": "terms", "name": m + " Koşullar", "body_html": "<p>" + m + " {{renter.full_name}} {{vehicle.plate}}</p>",
		}), http.StatusCreated, "create contract template"), "uuid")
		must(call(o, http.MethodPost, "/v1/tenant/contract-templates/"+ids["contractTemplate"]+"/activate", nil), http.StatusOK, "activate template")
		rentalQuote := str(must(call(o, http.MethodPost, "/v1/tenant/quotes", map[string]any{
			"pickup_branch_uuid": ids["branch"], "starts_at": day(12), "ends_at": day(13), "product_uuid": ids["product"],
			"customer_uuid": ids["customer"],
		}), http.StatusCreated, "create rental quote"), "uuid")
		ids["rentalReservation"] = str(must(call(o, http.MethodPost, "/v1/tenant/reservations", map[string]any{
			"quote_uuid": rentalQuote, "customer_uuid": ids["customer"], "drivers": []map[string]any{{"driver_uuid": ids["driver"]}},
		}), http.StatusCreated, "create rental reservation"), "uuid")
		ids["rental"] = str(must(call(o, http.MethodPost, "/v1/tenant/rentals", map[string]any{
			"reservation_uuid": ids["rentalReservation"], "vehicle_uuid": ids["vehicle"], "note": m,
		}), http.StatusCreated, "create rental"), "uuid")
		opTasks := must(call(o, http.MethodGet, "/v1/tenant/operation-tasks?rental="+ids["rental"], nil), http.StatusOK, "operation tasks")
		opItems := pageItems(t, opTasks.data)
		if len(opItems) == 0 {
			t.Fatalf("operation tasks of the rental: %s", opTasks.raw)
		}
		ids["operationTask"] = opItems[0].(map[string]any)["uuid"].(string)
		// Field tasks (TEC-724): a cleaning task at the service point,
		// assigned to the owner.
		ids["fieldTask"] = str(must(call(o, http.MethodPost, "/v1/tenant/field-tasks", map[string]any{
			"task_type": "cleaning", "branch_uuid": ids["branch"], "vehicle_uuid": ids["vehicle"], "due_at": day(9),
			"point_uuid": ids["point"], "note": m, "assignee_uuid": o.owner.Uuid,
		}), http.StatusCreated, "create field task"), "uuid")
		ids["charge"] = uuid.NewString()
		// KABİS (TEC-726): a start notification of the rental with its
		// integration job, waiting on a dependency (the handover hook and
		// the runner are covered by the kabis tests; the seed stores the
		// rows the routes read).
		var kRental, kBranch, kJob int64
		if err := pool.QueryRow(ctx, `SELECT r.id, res.pickup_branch_id FROM rentals r
			JOIN reservations res ON res.organization_id = r.organization_id AND res.id = r.reservation_id
			WHERE r.organization_id = $1 AND res.organization_id = $1 AND r.uuid = $2`, o.org.ID, ids["rental"]).Scan(&kRental, &kBranch); err != nil {
			t.Fatal(err)
		}
		ids["kabisNotification"] = uuid.NewString()
		if err := pool.QueryRow(ctx, `INSERT INTO integration_jobs (organization_id, direction, provider, kind, idempotency_key, status,
			payload, ref_type, ref_uuid, last_error)
			VALUES ($1, 'outbound', 'kabis', 'kabis.rental_start', $2, 'waiting_dependency', $3, 'kabis_notification', $4, 'waiting on dependency')
			RETURNING id`, o.org.ID, fmt.Sprintf("org:%d:kabis:%s", o.org.ID, ids["kabisNotification"]),
			fmt.Sprintf(`{"notification_uuid": %q, "rental_reference": %q}`, ids["kabisNotification"], m), ids["kabisNotification"]).Scan(&kJob); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO kabis_notifications (uuid, organization_id, rental_id, branch_id, rental_branch_id, event,
			cause, integration_job_id, occurred_at, plate, summary, hold_reason)
			VALUES ($1, $2, $3, $4, $4, 'rental_start', 'handover', $5, NOW(), '34 ISO 1', $6, 'no_account')`,
			ids["kabisNotification"], o.org.ID, kRental, kBranch, kJob,
			fmt.Sprintf(`{"renter": {"first_name": %q}, "vehicle": {"plate": "34 ISO 1"}}`, m)); err != nil {
			t.Fatal(err)
		}
		// Reported through the manual channel with a receipt (the
		// attachment route streams it).
		must(upload(o, http.MethodPost, "/v1/tenant/kabis-notifications/"+ids["kabisNotification"]+"/mark-manual", "file", "makbuz.png", png,
			map[string]string{"reason": m + " elle bildirildi", "reference": m + "-KBS"}), http.StatusOK, "kabis mark manual")
		// Inspections (TEC-712 / TEC-714): the handover form of the rental
		// with a damage and an uploaded photo; its template is the default
		// one created on first use.
		insp := must(call(o, http.MethodPost, "/v1/tenant/inspections", map[string]any{
			"rental_uuid": ids["rental"], "kind": "handover",
		}), http.StatusCreated, "start inspection")
		ids["inspection"] = str(insp, "uuid")
		ids["inspectionTemplate"] = str(insp, "template", "uuid")
		withDamage := must(call(o, http.MethodPost, "/v1/tenant/inspections/"+ids["inspection"]+"/damages", map[string]any{
			"zone": "hood", "description": m,
		}), http.StatusCreated, "inspection damage")
		ids["inspectionDamage"] = asMap(asMap(withDamage.data)["damages"].([]any)[0])["uuid"].(string)
		photo := must(call(o, http.MethodPost, "/v1/tenant/inspections/"+ids["inspection"]+"/photos", map[string]any{
			"slot": "front", "content_type": "image/png", "size_bytes": len(png),
		}), http.StatusCreated, "inspection photo")
		ids["inspectionPhoto"] = str(photo, "photo", "uuid")
		must(upload(o, http.MethodPut, "/v1/tenant/inspections/"+ids["inspection"]+"/photos/"+ids["inspectionPhoto"]+"/file", "file", "front.png", png, nil),
			http.StatusOK, "upload inspection photo")
		docResp := must(call(o, http.MethodPost, "/v1/tenant/documents", map[string]any{
			"kind": "terms", "reservation_uuid": ids["rentalReservation"],
		}), http.StatusOK, "prepare document")
		docData := asMap(docResp.data)["document"].(map[string]any)
		ids["contractDocument"] = docData["uuid"].(string)
		ids["documentVersion"] = docData["current_version"].(map[string]any)["uuid"].(string)
		// Remote approval (TEC-711): a link for the renter (revoked, so the
		// version stays open) and an in-person approval of the version.
		docPath := "/v1/tenant/documents/" + ids["contractDocument"]
		docHash := docData["current_version"].(map[string]any)["content_hash"].(string)
		linkResp := must(call(o, http.MethodPost, docPath+"/versions/"+ids["documentVersion"]+"/approval-links", map[string]any{
			"content_hash": docHash, "signer_role": "renter",
		}), http.StatusCreated, "approval link")
		ids["approvalLink"] = asMap(asMap(linkResp.data)["link"])["uuid"].(string)
		must(call(o, http.MethodPost, docPath+"/approval-links/"+ids["approvalLink"]+"/revoke", nil), http.StatusOK, "revoke approval link")
		approved := must(call(o, http.MethodPost, docPath+"/versions/"+ids["documentVersion"]+"/approve", map[string]any{
			"content_hash": docHash, "approver_name": m + " Onaylayan",
		}), http.StatusOK, "approve document")
		ids["approval"] = asMap(asMap(approved.data)["approvals"].([]any)[0])["uuid"].(string)
		// Finance (TEC-717 / TEC-720): the branch collection above posted
		// the customer's account and its first entry; a till, a bank
		// account, a transfer, a category and a paid expense with a receipt.
		accounts := pageItems(t, must(call(o, http.MethodGet, "/v1/tenant/finance/accounts", nil), http.StatusOK, "finance accounts").data)
		ids["financeAccount"] = accounts[0].(map[string]any)["uuid"].(string)
		entries := pageItems(t, must(call(o, http.MethodGet, "/v1/tenant/finance/entries?account="+ids["financeAccount"], nil),
			http.StatusOK, "ledger entries").data)
		ids["ledgerEntry"] = entries[0].(map[string]any)["uuid"].(string)
		ids["cashAccount"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/cash-accounts", map[string]any{
			"code": "KASA", "name": m + " Kasa", "kind": "cash", "branch_uuid": ids["home"], "opening_balance": "1000",
		}), http.StatusCreated, "create till"), "uuid")
		ids["bankAccount"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/cash-accounts", map[string]any{
			"code": "BANKA", "name": m + " Banka", "kind": "bank",
		}), http.StatusCreated, "create bank account"), "uuid")
		transfer := must(call(o, http.MethodPost, "/v1/tenant/finance/transfers", map[string]any{
			"from_account_uuid": ids["cashAccount"], "to_account_uuid": ids["bankAccount"], "amount": "100", "description": m,
		}), http.StatusCreated, "transfer")
		ids["cashMovement"] = asMap(asMap(transfer.data)["out"])["uuid"].(string)
		// Collections (TEC-718): a cash collection into the till, kept as
		// credit (the branch payment above is a collection too).
		ids["collection"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/collections", map[string]any{
			"account_uuid": ids["financeAccount"], "amount": "50", "method": "cash", "cash_account_uuid": ids["cashAccount"],
			"payer_name": m + " Ödeyen", "reference_no": m, "allocation_mode": "none",
		}), http.StatusCreated, "create collection"), "uuid")
		ids["expenseCategory"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/expense-categories", map[string]any{
			"code": "iso_cat", "name": m + " Kategori",
		}), http.StatusCreated, "create expense category"), "uuid")
		ids["expense"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/expenses", map[string]any{
			"category_uuid": ids["expenseCategory"], "total_amount": "120", "vat_rate": "20", "description": m + " gider",
			"branch_uuid": ids["home"], "vehicle_uuid": ids["vehicle"], "cash_account_uuid": ids["cashAccount"],
		}), http.StatusCreated, "create expense"), "uuid")
		must(upload(o, http.MethodPut, "/v1/tenant/finance/expenses/"+ids["expense"]+"/file", "file", "fis.png", png, nil),
			http.StatusOK, "upload expense receipt")
		// Suppliers (TEC-721): a card, an invoice with its file and a payment.
		ids["supplier"] = str(must(call(o, http.MethodPost, "/v1/tenant/suppliers", map[string]any{
			"title": m + " Tedarikçi", "supplier_type": "service", "note": m,
		}), http.StatusCreated, "create supplier"), "uuid")
		ids["supplierInvoice"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/supplier-invoices", map[string]any{
			"supplier_uuid": ids["supplier"], "invoice_no": "F-1", "category_uuid": ids["expenseCategory"], "total_amount": "240",
			"vat_rate": "20", "branch_uuid": ids["home"], "description": m + " fatura",
		}), http.StatusCreated, "create supplier invoice"), "uuid")
		must(upload(o, http.MethodPut, "/v1/tenant/finance/supplier-invoices/"+ids["supplierInvoice"]+"/file", "file", "fatura.png", png, nil),
			http.StatusOK, "upload supplier invoice")
		ids["supplierPayment"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/supplier-payments", map[string]any{
			"supplier_uuid": ids["supplier"], "cash_account_uuid": ids["bankAccount"], "amount": "100", "description": m + " ödeme",
		}), http.StatusCreated, "create supplier payment"), "uuid")
		// Security deposits (TEC-719): a POS pre-auth resolved with one
		// deduction (and its document), then a second one still held.
		ids["deposit"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/deposits", map[string]any{
			"rental_uuid": ids["rental"], "method": "pos_preauth", "pos_reference": "PRV-" + m, "amount": "500", "note": m,
		}), http.StatusCreated, "take deposit"), "uuid")
		resolved := must(call(o, http.MethodPost, "/v1/tenant/finance/deposits/"+ids["deposit"]+"/resolve", map[string]any{
			"deductions": []map[string]any{{"amount": "100", "applies_to": "general", "reason": m + " kesinti"}}, "idempotency_key": m,
		}), http.StatusOK, "resolve deposit")
		ids["depositDeduction"] = asMap(asMap(resolved.data)["deductions"].([]any)[0])["uuid"].(string)
		must(upload(o, http.MethodPut, "/v1/tenant/finance/deposits/"+ids["deposit"]+"/deductions/"+ids["depositDeduction"]+"/file",
			"file", "fatura.png", png, nil), http.StatusOK, "upload deduction document")
		ids["heldDeposit"] = str(must(call(o, http.MethodPost, "/v1/tenant/finance/deposits", map[string]any{
			"rental_uuid": ids["rental"], "method": "pos_preauth", "pos_reference": "PRV2-" + m, "amount": "400",
		}), http.StatusCreated, "take second deposit"), "uuid")
		// E-invoices (TEC-722): a draft of the rental's customer stored as
		// the module stores it (the accrual / link flow is covered by the
		// invoices tests), its receiver completed through the API and
		// finalized: numbered, XML stored, the send waiting on the
		// integrator dependency (no e-invoice account).
		must(call(o, http.MethodPut, "/v1/tenant/invoices/settings", map[string]any{
			"supplier_district": "Kadıköy", "supplier_city": "İstanbul",
		}), http.StatusOK, "invoice settings")
		var invBranch, invAccount, invCustomer, invRental int64
		if err := pool.QueryRow(ctx, `SELECT b.id, a.id, c.id, r.id FROM branches b, accounts a, customers c, rentals r
			WHERE b.organization_id = $1 AND b.uuid = $2 AND a.organization_id = $1 AND a.uuid = $3
			  AND c.organization_id = $1 AND c.uuid = $4 AND r.organization_id = $1 AND r.uuid = $5`,
			o.org.ID, ids["home"], ids["financeAccount"], ids["customer"], ids["rental"]).Scan(&invBranch, &invAccount, &invCustomer, &invRental); err != nil {
			t.Fatal(err)
		}
		var invID int64
		var invUUID string
		if err := pool.QueryRow(ctx, `INSERT INTO invoices (organization_id, branch_id, account_id, customer_id, rental_id, profile, invoice_type,
			issue_date, receiver_type, receiver_name, line_extension_amount, tax_amount, payable_amount, linked_amount, note)
			VALUES ($1, $2, $3, $4, $5, 'EARSIVFATURA', 'SATIS', CURRENT_DATE, 'corporate', $6, 100, 20, 120, 120, $7)
			RETURNING id, uuid::text`, o.org.ID, invBranch, invAccount, invCustomer, invRental, m+" Lojistik", m).Scan(&invID, &invUUID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO invoice_lines (organization_id, invoice_id, position, kind, name, gross_amount, net_amount, vat_rate, vat_amount)
			VALUES ($1, $2, 1, 'rent', $3, 120, 100, 20, 20)`, o.org.ID, invID, m+" kiralama"); err != nil {
			t.Fatal(err)
		}
		ids["invoice"] = invUUID
		must(call(o, http.MethodPatch, "/v1/tenant/invoices/"+invUUID, map[string]any{"receiver": map[string]any{
			"identity_type": "vkn", "identity_number": "4540536920", "tax_office": "Kadıköy", "district": "Moda", "city": "İstanbul",
		}}), http.StatusOK, "invoice receiver")
		must(call(o, http.MethodPost, "/v1/tenant/invoices/"+invUUID+"/finalize", nil), http.StatusOK, "finalize invoice")
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
			if !strings.HasPrefix(rt, "GET ") || !strings.Contains(rt, "{") || strings.HasSuffix(rt, "/download") || strings.HasSuffix(rt, "/file") ||
				strings.HasSuffix(rt, "/pdf") {
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
		"GET /v1/tenant/vehicles": {
			"?home_branch=" + b["home"], "?rental_group=" + b["group"], "?segment=" + b["segment"], "?supplier=" + b["supplier"],
		},
		"GET /v1/tenant/suppliers": {"?q=" + orgB.marker},
		"GET /v1/tenant/finance/supplier-invoices": {
			"?supplier=" + b["supplier"], "?category=" + b["expenseCategory"], "?branch=" + b["home"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/finance/supplier-payments": {
			"?supplier=" + b["supplier"], "?cash_account=" + b["bankAccount"], "?invoice=" + b["supplierInvoice"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/transfers":             {"?vehicle=" + b["vehicle"], "?from_branch=" + b["home"], "?to_branch=" + b["branch"]},
		"GET /v1/tenant/one-way-rules":         {"?from_branch=" + b["home"], "?to_branch=" + b["branch"]},
		"GET /v1/tenant/products":              {"?segment=" + b["segment"], "?rental_group=" + b["group"]},
		"GET /v1/tenant/rental-groups":         {"?segment=" + b["segment"]},
		"GET /v1/tenant/fleet/alerts":          {"?vehicle=" + b["vehicle"]},
		"GET /v1/tenant/search":                {"?q=" + orgB.marker},
		"GET /v1/tenant/rate-plans":            {"?rental_product=" + b["product"], "?branch=" + b["branch"]},
		"GET /v1/tenant/equipment-allocations": {"?extra=" + b["extra"], "?branch=" + b["branch"]},
		"GET /v1/tenant/customers":             {"?q=" + orgB.marker, "?tag=" + orgB.marker, "?q=4540536920"},
		"GET /v1/tenant/reservations": {
			"?product=" + b["product"], "?customer=" + b["customer"], "?branch=" + b["branch"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/quotes":           {"?reservation=" + b["reservation"], "?product=" + b["product"]},
		"GET /v1/tenant/resolution-tasks": {"?reservation=" + b["reservation"], "?assignee=" + bob.Uuid.String()},
		"GET /v1/tenant/payments":         {"?reservation=" + b["reservation"], "?q=" + orgB.marker},
		"GET /v1/tenant/payment-links":    {"?reservation=" + b["reservation"]},
		"GET /v1/tenant/integrations":     {"?q=" + orgB.marker},
		"GET /v1/tenant/rentals": {
			"?reservation=" + b["rentalReservation"], "?vehicle=" + b["vehicle"], "?branch=" + b["branch"], "?product=" + b["product"],
			"?customer=" + b["customer"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/operation-tasks": {
			"?rental=" + b["rental"], "?vehicle=" + b["vehicle"], "?branch=" + b["branch"], "?assignee=" + bob.Uuid.String(),
		},
		"GET /v1/tenant/field-tasks": {
			"?branch=" + b["branch"], "?assignee=" + bob.Uuid.String(), "?reservation=" + b["rentalReservation"], "?q=" + orgB.marker,
			"?assignee=none",
		},
		"GET /v1/tenant/field-tasks/mine": {"?q=" + orgB.marker},
		"GET /v1/tenant/delivery-points":  {"?branch=" + b["branch"], "?product=" + b["product"]},
		"GET /v1/tenant/documents":        {"?reservation=" + b["rentalReservation"], "?rental=" + b["rental"]},
		"GET /v1/tenant/inspections":      {"?rental=" + b["rental"]},
		"GET /v1/tenant/toll-passes": {
			"?vehicle=" + b["vehicle"], "?rental=" + b["rental"], "?q=" + orgB.marker, "?review=1",
		},
		"GET /v1/tenant/kabis-notifications": {
			"?rental=" + b["rental"], "?q=" + orgB.marker, "?needs_action=1", "?status=waiting_dependency",
		},
		// Operations overview (TEC-716): B's branches as the filter are
		// unknown in A (not_found); the plain call and the badges are
		// leak-scanned.
		"GET /v1/tenant/overview":           {"?branch=" + b["branch"], "?branch=" + b["home"]},
		"GET /v1/tenant/contract-templates": {"?q=" + orgB.marker},
		"GET /v1/tenant/customer-requests":  {"?reservation=" + b["reservation"], "?q=" + orgB.marker},
		// Finance: filters naming B's records are unknown in A.
		"GET /v1/tenant/finance/accounts": {"?branch=" + b["home"], "?q=" + orgB.marker},
		"GET /v1/tenant/finance/entries": {
			"?account=" + b["financeAccount"], "?rental=" + b["rental"], "?branch=" + b["home"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/finance/cash-accounts": {"?branch=" + b["home"], "?q=" + orgB.marker},
		"GET /v1/tenant/finance/movements": {
			"?account=" + b["cashAccount"], "?category=" + b["expenseCategory"], "?branch=" + b["home"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/finance/expenses": {
			"?supplier=" + b["supplier"], "?category=" + b["expenseCategory"], "?vehicle=" + b["vehicle"], "?branch=" + b["home"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/finance/deposits": {
			"?branch=" + b["home"], "?rental=" + b["rental"], "?q=" + orgB.marker, "?held_over_days=0",
		},
		"GET /v1/tenant/finance/collections": {
			"?account=" + b["financeAccount"], "?rental=" + b["rental"], "?branch=" + b["home"],
			"?cash_account=" + b["cashAccount"], "?q=" + orgB.marker,
		},
		"GET /v1/tenant/finance/rental-balances": {"?q=" + orgB.marker},
		// Invoices (TEC-722): filters naming B's records are unknown in A.
		"GET /v1/tenant/invoices": {
			"?rental=" + b["rental"], "?account=" + b["financeAccount"], "?branch=" + b["home"], "?q=" + orgB.marker, "?needs_action=true",
		},
		"GET /v1/tenant/invoices/items":              {"?rental=" + b["rental"], "?account=" + b["financeAccount"]},
		"GET /v1/tenant/invoices/uninvoiced-rentals": {"?q=" + orgB.marker},
		"GET /v1/tenant/finance/expense-categories":  {"?q=" + orgB.marker},
		// Vehicle profitability (TEC-727): B's branch / rental group as a
		// filter is unknown in A (not_found); the plain call is leak-scanned.
		"GET /v1/tenant/reports/vehicle-profitability": {
			"?branch=" + b["home"], "?rental_group=" + b["group"], "?q=" + orgB.marker, "?basis=cash&allocation=revenue",
		},
		"GET /v1/tenant/rate-plans/matrix": {
			"?scope=branch&branch=" + b["branch"],
			"?scope=branch_channel&branch=" + b["home"] + "&channel=web",
		},
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
			if query != "" && r.code != http.StatusOK && (r.code >= 500 || !bytes.Contains(r.raw, []byte(`"not_found"`))) {
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
		{"POST /v1/tenant/customers/bulk", map[string]any{"action": "add_tag", "params": map[string]any{"tag": "x"},
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["customer"]}}}, nil},
		// Reservations: quotes / reservations / bulk naming B's records.
		{"POST /v1/tenant/quotes", map[string]any{"pickup_branch_uuid": orgA.ids["branch"], "starts_at": day(20), "ends_at": day(21),
			"product_uuid": orgB.ids["product"]}, reject},
		{"POST /v1/tenant/quotes", map[string]any{"pickup_branch_uuid": orgB.ids["branch"], "starts_at": day(20), "ends_at": day(21),
			"product_uuid": orgA.ids["product"]}, reject},
		{"POST /v1/tenant/quotes", map[string]any{"pickup_branch_uuid": orgA.ids["branch"], "starts_at": day(20), "ends_at": day(21),
			"product_uuid": orgA.ids["product"], "customer_uuid": orgB.ids["customer"]}, reject},
		{"POST /v1/tenant/quotes/search", map[string]any{"pickup_branch_uuid": orgB.ids["branch"], "starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/quotes/search", map[string]any{"pickup_branch_uuid": orgA.ids["branch"], "starts_at": day(20), "ends_at": day(21),
			"product_uuids": []string{orgB.ids["product"]}}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"quote_uuid": orgB.ids["quoteOpen"], "customer_uuid": orgA.ids["customer"]}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"quote_uuid": orgA.ids["quoteOpen"], "customer_uuid": orgB.ids["customer"]}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"quote_uuid": orgA.ids["quoteOpen"], "customer_uuid": orgA.ids["customer"],
			"invoice_customer_uuid": orgB.ids["customer"]}, reject},
		{"POST /v1/tenant/reservations", map[string]any{"quote_uuid": orgA.ids["quoteOpen"], "customer_uuid": orgA.ids["customer"],
			"drivers": []map[string]any{{"driver_uuid": orgB.ids["driver"]}}}, reject},
		{"POST /v1/tenant/reservations/bulk", map[string]any{"action": "cancel",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["reservation"]}, "params": map[string]any{"reason": "izolasyon"}}}, nil},
		{"POST /v1/tenant/reservations/bulk", map[string]any{"action": "assign_staff",
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["reservation"]}, "params": map[string]any{"user_uuid": bob.Uuid.String()}}}, nil},
		// Field tasks: bodies naming B's branches, vehicle, point, member;
		// bulk assign of B's task or to B's member.
		{"POST /v1/tenant/field-tasks", map[string]any{"task_type": "cleaning", "branch_uuid": orgB.ids["branch"], "due_at": day(9)}, reject},
		{"POST /v1/tenant/field-tasks", map[string]any{"task_type": "cleaning", "branch_uuid": orgA.ids["branch"], "due_at": day(9),
			"vehicle_uuid": orgB.ids["vehicle"]}, reject},
		{"POST /v1/tenant/field-tasks", map[string]any{"task_type": "cleaning", "branch_uuid": orgA.ids["branch"], "due_at": day(9),
			"point_uuid": orgB.ids["point"]}, reject},
		{"POST /v1/tenant/field-tasks", map[string]any{"task_type": "transfer", "branch_uuid": orgA.ids["home"], "due_at": day(9),
			"vehicle_uuid": orgA.ids["vehicle"], "to_branch_uuid": orgB.ids["branch"]}, reject},
		{"POST /v1/tenant/field-tasks", map[string]any{"task_type": "cleaning", "branch_uuid": orgA.ids["branch"], "due_at": day(9),
			"assignee_uuid": bob.Uuid}, reject},
		{"POST /v1/tenant/field-tasks/bulk", map[string]any{"action": "assign",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["fieldTask"]}}}, nil},
		{"POST /v1/tenant/field-tasks/bulk", map[string]any{"action": "assign",
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["fieldTask"]}, "params": map[string]any{"user_uuid": bob.Uuid.String()}}}, nil},
		// Rentals / contracts: bodies naming B's reservation / vehicle.
		{"POST /v1/tenant/rentals", map[string]any{"reservation_uuid": orgB.ids["rentalReservation"]}, reject},
		{"POST /v1/tenant/rentals", map[string]any{"reservation_uuid": orgA.ids["rentalReservation"], "vehicle_uuid": orgB.ids["vehicle"]}, reject},
		{"POST /v1/tenant/documents", map[string]any{"kind": "terms", "reservation_uuid": orgB.ids["rentalReservation"]}, reject},
		{"POST /v1/tenant/inspections", map[string]any{"kind": "handover", "rental_uuid": orgB.ids["rental"]}, reject},
		{"POST /v1/tenant/contract-templates/preview", map[string]any{"kind": "terms", "body_html": "<p>x</p>",
			"reservation_uuid": orgB.ids["rentalReservation"]}, reject},
		{"POST /v1/tenant/invitations", map[string]any{"email": "x1@iso.test", "role_uuids": []string{orgB.ids["role"]}, "branch_scope": "all"}, reject},
		{"POST /v1/tenant/invitations", map[string]any{"email": "x2@iso.test", "role_uuids": []string{orgA.ids["role"]},
			"branch_scope": "selected", "branch_uuids": []string{orgB.ids["branch"]}}, reject},
		// Pricing: bodies naming B's products, branches, segments, plans, rules.
		{"POST /v1/tenant/rate-plans", map[string]any{"rental_product_uuid": orgB.ids["product"], "scope": "common"}, reject},
		{"POST /v1/tenant/rate-plans", map[string]any{"rental_product_uuid": orgA.ids["product"], "scope": "branch",
			"branch_uuid": orgB.ids["branch"]}, reject},
		{"PUT /v1/tenant/rate-plans/matrix", map[string]any{"scope": "common", "bands": []map[string]any{{"min_days": 1, "max_days": nil}},
			"rows": []map[string]any{{"rental_product_uuid": orgB.ids["product"], "prices": []string{"100"}}}}, reject},
		{"PUT /v1/tenant/rate-plans/matrix", map[string]any{"scope": "branch", "branch_uuid": orgB.ids["branch"],
			"bands": []map[string]any{{"min_days": 1, "max_days": nil}},
			"rows":  []map[string]any{{"rental_product_uuid": orgA.ids["product"], "prices": []string{"100"}}}}, reject},
		{"POST /v1/tenant/rate-plans/matrix/activate", map[string]any{"scope": "branch", "branch_uuid": orgB.ids["branch"]}, reject},
		{"POST /v1/tenant/date-rules", map[string]any{"name": "X1", "start_date": date(1), "end_date": date(2), "kind": "amount", "value": 10,
			"product_scope": "products", "rental_product_uuids": []string{orgB.ids["product"]}}, reject},
		{"POST /v1/tenant/date-rules", map[string]any{"name": "X2", "start_date": date(1), "end_date": date(2), "kind": "amount", "value": 10,
			"product_scope": "segments", "segment_uuids": []string{orgB.ids["segment"]}}, reject},
		{"POST /v1/tenant/date-rules", map[string]any{"name": "X3", "start_date": date(1), "end_date": date(2), "kind": "amount", "value": 10,
			"all_branches": false, "branch_uuids": []string{orgB.ids["branch"]}}, reject},
		{"POST /v1/tenant/pricing/preview", previewBody(orgB.ids["product"], orgA.ids["home"], nil), reject},
		{"POST /v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgB.ids["home"], nil), reject},
		{"POST /v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgA.ids["home"], map[string]any{"rate_plan_uuid": orgB.ids["ratePlan"]}), reject},
		{"POST /v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgA.ids["home"], map[string]any{"date_rule_uuid": orgB.ids["dateRule"]}), reject},
		{"POST /v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgA.ids["home"], map[string]any{"return_branch_uuid": orgB.ids["branch"]}), reject},
		{"POST /v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgA.ids["home"], map[string]any{
			"rate_plan_draft": map[string]any{"scope": "branch", "branch_uuid": orgB.ids["branch"], "bands": []map[string]any{{"min_days": 1, "daily_price": 1}}},
		}), reject},
		{"POST /v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgA.ids["home"], map[string]any{
			"date_rule_draft": map[string]any{"start_date": date(1), "end_date": date(3), "kind": "percent", "value": 10,
				"product_scope": "segments", "segment_uuids": []string{orgB.ids["segment"]}},
		}), reject},
		// Availability: search / hold / capacity rule bodies naming B's records.
		{"POST /v1/tenant/availability/search", map[string]any{"pickup_branch_uuid": orgB.ids["branch"],
			"starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/availability/search", map[string]any{"pickup_branch_uuid": orgA.ids["branch"],
			"starts_at": day(20), "ends_at": day(21), "product_uuids": []string{orgB.ids["product"]}}, reject},
		{"POST /v1/tenant/availability/search", map[string]any{"pickup_point_uuid": orgB.ids["point"],
			"starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/availability/holds", map[string]any{"product_uuid": orgB.ids["product"],
			"pickup_branch_uuid": orgA.ids["branch"], "starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/availability/holds", map[string]any{"product_uuid": orgA.ids["product"],
			"pickup_branch_uuid": orgA.ids["branch"], "return_branch_uuid": orgB.ids["branch"], "starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/availability/holds", map[string]any{"product_uuid": orgA.ids["product"],
			"pickup_branch_uuid": orgA.ids["branch"], "starts_at": day(20), "ends_at": day(21), "vehicle_uuid": orgB.ids["vehicle"]}, reject},
		{"POST /v1/tenant/capacity-rules", map[string]any{"rule_type": "safety_margin", "quantity": 1,
			"product_uuid": orgB.ids["product"]}, reject},
		{"POST /v1/tenant/capacity-rules", map[string]any{"rule_type": "safety_margin", "quantity": 1,
			"branch_uuid": orgB.ids["branch"]}, reject},
		// Extras / equipment / campaigns / preview extras naming B's records.
		{"POST /v1/tenant/extras/availability", map[string]any{"branch_uuid": orgB.ids["branch"], "starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/extras/availability", map[string]any{"branch_uuid": orgA.ids["branch"], "starts_at": day(20), "ends_at": day(21),
			"extra_uuids": []string{orgB.ids["extra"]}}, reject},
		{"POST /v1/tenant/extras/bulk", map[string]any{"action": "archive", "target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["extra"]}}}, nil},
		{"POST /v1/tenant/equipment-allocations", map[string]any{"extra_uuid": orgB.ids["extra"], "branch_uuid": orgA.ids["branch"],
			"starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/equipment-allocations", map[string]any{"extra_uuid": orgA.ids["extra"], "branch_uuid": orgB.ids["branch"],
			"starts_at": day(20), "ends_at": day(21)}, reject},
		{"POST /v1/tenant/equipment-allocations", map[string]any{"extra_uuid": orgA.ids["extra"], "vehicle_allocation_uuid": orgB.ids["hold"]}, reject},
		{"POST /v1/tenant/campaigns", map[string]any{"name": "X1", "campaign_type": "coupon", "code": "XB1", "discount_kind": "amount",
			"discount_value": 10, "product_scope": "products", "rental_product_uuids": []string{orgB.ids["product"]}}, reject},
		{"POST /v1/tenant/campaigns", map[string]any{"name": "X2", "campaign_type": "coupon", "code": "XB2", "discount_kind": "amount",
			"discount_value": 10, "product_scope": "segments", "segment_uuids": []string{orgB.ids["segment"]}}, reject},
		{"POST /v1/tenant/campaigns/bulk", bulkIDs("campaign"), nil},
		{"POST /v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgA.ids["home"], map[string]any{
			"extras": []map[string]any{{"extra_uuid": orgB.ids["extra"], "quantity": 1}},
		}), reject},
		// Action parameters naming B's records, on A's vehicle.
		{"POST /v1/tenant/vehicles/bulk", map[string]any{"action": "move_branch", "params": map[string]any{"branch_uuid": orgB.ids["branch"]},
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["vehicle"]}}}, reject},
		{"POST /v1/tenant/vehicles/bulk", map[string]any{"action": "add_rental_group", "params": map[string]any{"rental_group_uuid": orgB.ids["group"]},
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["vehicle"]}}}, reject},
		// Finance: bodies naming B's customer, branch, accounts, category,
		// vehicle; bulk targets / params of B.
		{"POST /v1/tenant/finance/accounts", map[string]any{"customer_uuid": orgB.ids["customer"]}, reject},
		{"POST /v1/tenant/finance/cash-accounts", map[string]any{"code": "X1", "name": "x", "kind": "cash", "branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/finance/transfers", map[string]any{"from_account_uuid": orgA.ids["cashAccount"], "to_account_uuid": orgB.ids["bankAccount"],
			"amount": "1"}, reject},
		{"POST /v1/tenant/finance/transfers", map[string]any{"from_account_uuid": orgB.ids["cashAccount"], "to_account_uuid": orgA.ids["bankAccount"],
			"amount": "1"}, reject},
		{"POST /v1/tenant/finance/expenses", map[string]any{"category_uuid": orgB.ids["expenseCategory"], "total_amount": "1"}, reject},
		{"POST /v1/tenant/finance/expenses", map[string]any{"category_uuid": orgA.ids["expenseCategory"], "total_amount": "1",
			"branch_uuid": orgA.ids["home"], "supplier_uuid": orgB.ids["supplier"]}, reject},
		{"POST /v1/tenant/finance/supplier-invoices", map[string]any{"supplier_uuid": orgB.ids["supplier"], "invoice_no": "X",
			"category_uuid": orgA.ids["expenseCategory"], "total_amount": "1", "branch_uuid": orgA.ids["home"]}, reject},
		{"POST /v1/tenant/finance/supplier-invoices", map[string]any{"supplier_uuid": orgA.ids["supplier"], "invoice_no": "X",
			"category_uuid": orgB.ids["expenseCategory"], "total_amount": "1", "branch_uuid": orgA.ids["home"]}, reject},
		{"POST /v1/tenant/finance/supplier-invoices", map[string]any{"supplier_uuid": orgA.ids["supplier"], "invoice_no": "X",
			"category_uuid": orgA.ids["expenseCategory"], "total_amount": "1", "branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/finance/supplier-invoices", map[string]any{"supplier_uuid": orgA.ids["supplier"], "invoice_no": "X",
			"category_uuid": orgA.ids["expenseCategory"], "total_amount": "1", "vehicle_uuid": orgB.ids["vehicle"]}, reject},
		{"POST /v1/tenant/finance/supplier-payments", map[string]any{"supplier_uuid": orgB.ids["supplier"],
			"cash_account_uuid": orgA.ids["bankAccount"], "amount": "1"}, reject},
		{"POST /v1/tenant/finance/supplier-payments", map[string]any{"supplier_uuid": orgA.ids["supplier"],
			"cash_account_uuid": orgB.ids["bankAccount"], "amount": "1"}, reject},
		{"POST /v1/tenant/finance/supplier-payments", map[string]any{"supplier_uuid": orgA.ids["supplier"],
			"cash_account_uuid": orgA.ids["bankAccount"], "amount": "1",
			"allocations": []map[string]any{{"invoice_uuid": orgB.ids["supplierInvoice"], "amount": "1"}}}, reject},
		{"POST /v1/tenant/suppliers/bulk", map[string]any{"action": "archive",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["supplier"]}}}, nil},
		{"POST /v1/tenant/finance/expenses", map[string]any{"category_uuid": orgA.ids["expenseCategory"], "total_amount": "1",
			"branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/finance/expenses", map[string]any{"category_uuid": orgA.ids["expenseCategory"], "total_amount": "1",
			"branch_uuid": orgA.ids["home"], "vehicle_uuid": orgB.ids["vehicle"]}, reject},
		{"POST /v1/tenant/finance/expenses", map[string]any{"category_uuid": orgA.ids["expenseCategory"], "total_amount": "1",
			"branch_uuid": orgA.ids["home"], "cash_account_uuid": orgB.ids["cashAccount"]}, reject},
		{"POST /v1/tenant/finance/movements/bulk", map[string]any{"action": "set_category",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["cashMovement"]}, "params": map[string]any{"category_uuid": orgA.ids["expenseCategory"]}}}, nil},
		{"POST /v1/tenant/finance/movements/bulk", map[string]any{"action": "set_category",
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["cashMovement"]}, "params": map[string]any{"category_uuid": orgB.ids["expenseCategory"]}}}, reject},
		{"POST /v1/tenant/finance/collections", map[string]any{"account_uuid": orgB.ids["financeAccount"], "amount": "1",
			"method": "cash", "cash_account_uuid": orgA.ids["cashAccount"]}, reject},
		{"POST /v1/tenant/finance/collections", map[string]any{"account_uuid": orgA.ids["financeAccount"], "amount": "1",
			"method": "cash", "cash_account_uuid": orgB.ids["cashAccount"]}, reject},
		{"POST /v1/tenant/finance/collections", map[string]any{"rental_uuid": orgB.ids["rental"], "amount": "1",
			"method": "cash", "cash_account_uuid": orgA.ids["cashAccount"]}, reject},
		{"POST /v1/tenant/finance/collections", map[string]any{"account_uuid": orgA.ids["financeAccount"], "amount": "1",
			"method": "cash", "cash_account_uuid": orgA.ids["cashAccount"], "branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/finance/collections", map[string]any{"account_uuid": orgA.ids["financeAccount"], "amount": "1",
			"method": "cash", "cash_account_uuid": orgA.ids["cashAccount"], "allocation_mode": "manual",
			"allocations": []map[string]any{{"entry_uuid": orgB.ids["ledgerEntry"]}}}, reject},
		{"POST /v1/tenant/finance/collections/bulk", map[string]any{"action": "allocate",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["collection"]}}}, nil},
		{"POST /v1/tenant/finance/expenses/bulk", map[string]any{"action": "set_category",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["expense"]}, "params": map[string]any{"category_uuid": orgA.ids["expenseCategory"]}}}, nil},
		{"POST /v1/tenant/finance/deposits", map[string]any{"rental_uuid": orgB.ids["rental"], "method": "pos_preauth", "pos_reference": "X"}, reject},
		{"POST /v1/tenant/finance/deposits", map[string]any{"rental_uuid": orgA.ids["rental"], "method": "cash",
			"cash_account_uuid": orgB.ids["cashAccount"]}, reject},
		{"POST /v1/tenant/finance/expenses/bulk", map[string]any{"action": "set_category",
			"target": map[string]any{"scope": "ids", "ids": []string{orgA.ids["expense"]}, "params": map[string]any{"category_uuid": orgB.ids["expenseCategory"]}}}, reject},
		// Invoices: drafts naming B's rental / account / entry / branch; bulk
		// of B's invoices and rentals.
		{"POST /v1/tenant/invoices", map[string]any{"rental_uuid": orgB.ids["rental"]}, reject},
		{"POST /v1/tenant/invoices", map[string]any{"account_uuid": orgB.ids["financeAccount"]}, reject},
		{"POST /v1/tenant/invoices", map[string]any{"account_uuid": orgA.ids["financeAccount"], "entry_uuids": []string{orgB.ids["ledgerEntry"]}}, reject},
		{"POST /v1/tenant/invoices", map[string]any{"rental_uuid": orgA.ids["rental"], "branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/invoices/bulk", map[string]any{"action": "send",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["invoice"]}}}, nil},
		{"POST /v1/tenant/invoices/bulk", map[string]any{"action": "create_for_rentals",
			"target": map[string]any{"scope": "ids", "ids": []string{orgB.ids["rental"]}}}, nil},
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
		{"POST /v1/tenant/finance/deposits/" + orgA.ids["heldDeposit"] + "/resolve", map[string]any{"deductions": []map[string]any{{
			"amount": "10", "applies_to": "general", "reason": "izolasyon", "inspection_damage_uuid": orgB.ids["inspectionDamage"]}}}, reject},
		{"POST /v1/tenant/finance/deposits/" + orgA.ids["heldDeposit"] + "/resolve", map[string]any{"deductions": []map[string]any{{
			"amount": "10", "reason": "izolasyon", "rental_charge_uuid": orgB.ids["charge"]}}}, reject},
		{"POST /v1/tenant/reservations/" + orgA.ids["reservation"] + "/plate", map[string]any{"vehicle_uuid": orgB.ids["vehicle"]}, reject},
		{"POST /v1/tenant/finance/accounts/" + orgA.ids["financeAccount"] + "/collections", map[string]any{"amount": "1",
			"cash_account_uuid": orgB.ids["cashAccount"]}, reject},
		{"POST /v1/tenant/finance/accounts/" + orgA.ids["financeAccount"] + "/collections", map[string]any{"amount": "1",
			"rental_uuid": orgB.ids["rental"]}, reject},
		{"POST /v1/tenant/finance/collections/" + orgA.ids["collection"] + "/allocations", map[string]any{"mode": "manual",
			"items": []map[string]any{{"entry_uuid": orgB.ids["ledgerEntry"]}}}, reject},
		{"POST /v1/tenant/finance/collections/" + orgA.ids["collection"] + "/allocations", map[string]any{"mode": "auto",
			"rental_uuid": orgB.ids["rental"]}, reject},
		{"POST /v1/tenant/finance/collections/" + orgA.ids["collection"] + "/allocations/release", map[string]any{
			"entry_uuid": orgB.ids["ledgerEntry"], "reason": "izolasyon"}, reject},
		{"POST /v1/tenant/finance/collections/" + orgA.ids["collection"] + "/refund", map[string]any{"amount": "1",
			"reason": "izolasyon", "cash_account_uuid": orgB.ids["cashAccount"]}, reject},
		{"POST /v1/tenant/finance/accounts/" + orgA.ids["financeAccount"] + "/adjustments", map[string]any{"amount": "1",
			"direction": "debit", "reason": "izolasyon", "adjusts_entry_uuid": orgB.ids["ledgerEntry"]}, reject},
		{"POST /v1/tenant/finance/cash-accounts/" + orgA.ids["cashAccount"] + "/movements", map[string]any{"kind": "collection",
			"amount": "1", "ledger_entry_uuid": orgB.ids["ledgerEntry"]}, reject},
		{"POST /v1/tenant/finance/cash-accounts/" + orgA.ids["cashAccount"] + "/movements", map[string]any{"kind": "collection",
			"amount": "1", "category_uuid": orgB.ids["expenseCategory"]}, reject},
		{"PATCH /v1/tenant/finance/expenses/" + orgA.ids["expense"], map[string]any{"category_uuid": orgB.ids["expenseCategory"]}, reject},
		{"PATCH /v1/tenant/finance/expenses/" + orgA.ids["expense"], map[string]any{"supplier_uuid": orgB.ids["supplier"]}, reject},
		{"PATCH /v1/tenant/finance/supplier-invoices/" + orgA.ids["supplierInvoice"], map[string]any{"category_uuid": orgB.ids["expenseCategory"]}, reject},
		{"PATCH /v1/tenant/finance/supplier-invoices/" + orgA.ids["supplierInvoice"], map[string]any{"branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/finance/cash-accounts/" + orgA.ids["bankAccount"] + "/movements", map[string]any{"kind": "supplier_payment",
			"amount": "1", "supplier_uuid": orgB.ids["supplier"]}, reject},
		{"PATCH /v1/tenant/vehicles/" + orgA.ids["vehicle"], map[string]any{"ownership": "supplied", "supplier_uuid": orgB.ids["supplier"]}, reject},
		{"PATCH /v1/tenant/finance/cash-accounts/" + orgA.ids["cashAccount"], map[string]any{"branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/reservations/" + orgA.ids["reservation"] + "/change", map[string]any{"quote_uuid": orgB.ids["quoteOpen"]}, reject},
		{"POST /v1/tenant/reservations/" + orgA.ids["reservation"] + "/quotes", map[string]any{"product_uuid": orgB.ids["product"]}, reject},
		{"POST /v1/tenant/reservations/" + orgA.ids["reservation"] + "/quotes", map[string]any{"pickup_branch_uuid": orgB.ids["branch"]}, reject},
		{"PUT /v1/tenant/reservations/" + orgA.ids["reservation"] + "/drivers", map[string]any{
			"drivers": []map[string]any{{"driver_uuid": orgB.ids["driver"]}}}, reject},
		{"POST /v1/tenant/reservations/" + orgA.ids["reservation"] + "/assign", map[string]any{"user_uuid": bob.Uuid}, reject},
		{"POST /v1/tenant/resolution-tasks/" + orgA.ids["task"] + "/assign", map[string]any{"user_uuid": carol.Uuid}, reject},
		{"POST /v1/tenant/operation-tasks/" + orgA.ids["operationTask"] + "/assign", map[string]any{"user_uuid": carol.Uuid}, reject},
		{"POST /v1/tenant/field-tasks/" + orgA.ids["fieldTask"] + "/assign", map[string]any{"user_uuid": carol.Uuid}, reject},
		{"POST /v1/tenant/field-tasks/" + orgA.ids["fieldTask"] + "/assign", map[string]any{"user_uuid": bob.Uuid}, reject},
		{"PATCH /v1/tenant/field-tasks/" + orgA.ids["fieldTask"], map[string]any{"point_uuid": orgB.ids["point"]}, reject},
		{"POST /v1/tenant/rentals/" + orgA.ids["rental"] + "/handover", map[string]any{"km": 100, "fuel": 50, "branch_uuid": orgB.ids["branch"]}, reject},
		{"POST /v1/tenant/rentals/" + orgA.ids["rental"] + "/handover", map[string]any{"km": 100, "fuel": 50,
			"branch_uuid": orgA.ids["branch"], "point_uuid": orgB.ids["point"]}, reject},
		{"PATCH /v1/tenant/inspections/" + orgA.ids["inspection"], map[string]any{"branch_uuid": orgB.ids["branch"]}, reject},
		{"POST /v1/tenant/toll-passes/" + orgA.ids["tollPass"] + "/assign", map[string]any{"rental_uuid": orgB.ids["rental"], "reason": "yabancı kiralama"}, reject},
		{"PATCH /v1/tenant/inspections/" + orgA.ids["inspection"], map[string]any{"verification": map[string]any{"driver_uuid": orgB.ids["driver"]}}, reject},
		{"POST /v1/tenant/inspections/" + orgA.ids["inspection"] + "/photos", map[string]any{"slot": "damage", "damage_uuid": orgB.ids["inspectionDamage"],
			"content_type": "image/png", "size_bytes": 10}, reject},
		{"POST /v1/tenant/inspections/" + orgA.ids["inspection"] + "/media-links", map[string]any{"photo_uuids": []string{orgB.ids["inspectionPhoto"]}}, []int{http.StatusOK}},
		{"PATCH /v1/tenant/rental-groups/" + orgA.ids["group"], map[string]any{"segment_uuid": orgB.ids["segment"]}, reject},
		{"PATCH /v1/tenant/products/" + orgA.ids["product"], map[string]any{"segment_uuid": orgB.ids["segment"]}, reject},
		{"PATCH /v1/tenant/vehicles/" + orgA.ids["vehicle"], map[string]any{"home_branch_uuid": orgB.ids["home"]}, reject},
		{"POST /v1/tenant/customers/" + orgA.ids["customer"] + "/documents", map[string]any{"doc_type": "other", "driver_uuid": orgB.ids["driver"]}, reject},
		{"POST /v1/tenant/vehicles/" + orgA.ids["vehicle"] + "/rental-groups", map[string]any{"rental_group_uuid": orgB.ids["group"]}, reject},
		{"PATCH /v1/tenant/transfers/" + orgA.ids["transfer"], map[string]any{"to_branch_uuid": orgB.ids["branch"]}, reject},
		{"PATCH /v1/tenant/transfers/" + orgA.ids["transfer"], map[string]any{"responsible_user_uuid": carol.Uuid}, reject},
		{"PATCH /v1/tenant/one-way-rules/" + orgA.ids["rule"], map[string]any{"rental_product_uuid": orgB.ids["product"]}, reject},
		{"PATCH /v1/tenant/members/" + alice.Uuid.String(), map[string]any{"role_uuids": []string{orgB.ids["role"]}}, reject},
		{"PATCH /v1/tenant/members/" + alice.Uuid.String(), map[string]any{"branch_scope": "selected", "branch_uuids": []string{orgB.ids["home"]}}, reject},
		{"PUT /v1/tenant/products/" + orgA.ids["product"] + "/images/order", map[string]any{"image_uuids": []string{orgB.ids["image"]}}, reject},
		{"PATCH /v1/tenant/date-rules/" + orgA.ids["dateRule"], map[string]any{"segment_uuids": []string{orgB.ids["segment"]}}, reject},
		{"PATCH /v1/tenant/date-rules/" + orgA.ids["dateRule"], map[string]any{"all_branches": false, "branch_uuids": []string{orgB.ids["home"]}}, reject},
		{"PUT /v1/tenant/capacity-rules/" + orgA.ids["capacityRule"], map[string]any{"rule_type": "safety_margin", "quantity": 1,
			"product_uuid": orgB.ids["product"]}, reject},
		{"PUT /v1/tenant/extras/" + orgA.ids["extra"] + "/prices", map[string]any{
			"items": []map[string]any{{"rental_product_uuid": orgB.ids["product"], "price": "1"}}}, reject},
		{"PUT /v1/tenant/extras/" + orgA.ids["extra"] + "/stock", map[string]any{
			"items": []map[string]any{{"branch_uuid": orgB.ids["branch"], "quantity": 1}}}, reject},
		{"PATCH /v1/tenant/campaigns/" + orgA.ids["campaign"], map[string]any{"product_scope": "segments", "segment_uuids": []string{orgB.ids["segment"]}}, reject},
		{"PATCH /v1/tenant/campaigns/" + orgA.ids["campaign"], map[string]any{"rental_product_uuids": []string{orgB.ids["product"]}}, reject},
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

	// A coupon code of B is unknown in A: the preview answers, the coupon is
	// rejected and nothing of B leaks.
	if r := call(orgA, http.MethodPost, "/v1/tenant/pricing/preview", previewBody(orgA.ids["product"], orgA.ids["home"], map[string]any{
		"coupon_code": "NOTINA",
	})); r.code != http.StatusOK || leaked(r.raw) != "" || !strings.Contains(string(r.raw), `"not_found"`) {
		t.Errorf("preview with an unknown coupon code = %d: %s", r.code, r.raw)
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

	// B is unchanged by everything A did.
	after := snapshot()
	for path, want := range before {
		if after[path] != want {
			t.Errorf("B's %s changed:\n before %s\n after  %s", path, want, after[path])
		}
	}

	// Engine files live under the organization prefix.
	keyOf := func(table, id string) string {
		t.Helper()
		var key string
		if err := pool.QueryRow(ctx, "SELECT file_key FROM "+table+" WHERE uuid = $1", id).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	for _, c := range []struct{ table, id, kind string }{
		{"export_jobs", orgB.ids["exportJob"], "exports"}, {"import_jobs", orgB.ids["importJob"], "imports"},
	} {
		if key := keyOf(c.table, c.id); !strings.HasPrefix(key, storage.OrganizationPrefix(orgB.org.UUID.String())+c.kind+"/") {
			t.Errorf("%s file key %q is not under organizations/{B}/%s/", c.table, key, c.kind)
		}
	}
	// Defence in depth: a row pointing at another organization's object is
	// not streamed (export download, vehicle document file).
	aExportKey := keyOf("export_jobs", orgA.ids["exportJob"])
	bExportKey := keyOf("export_jobs", orgB.ids["exportJob"])
	if _, err := pool.Exec(ctx, "UPDATE export_jobs SET file_key = $1 WHERE uuid = $2", aExportKey, orgB.ids["exportJob"]); err != nil {
		t.Fatal(err)
	}
	if r := do(bob, orgB.org.Slug, http.MethodGet, "/v1/tenant/exports/"+orgB.ids["exportJob"]+"/download", "", nil); r.code != http.StatusNotFound {
		t.Errorf("export row pointing at A's file = %d, want 404", r.code)
	}
	if _, err := pool.Exec(ctx, "UPDATE export_jobs SET file_key = $1 WHERE uuid = $2", bExportKey, orgB.ids["exportJob"]); err != nil {
		t.Fatal(err)
	}
	var aDocKey, bDocKey string
	for _, p := range []struct {
		id  string
		dst *string
	}{{orgA.ids["document"], &aDocKey}, {orgB.ids["document"], &bDocKey}} {
		if err := pool.QueryRow(ctx, "SELECT file_object_key FROM vehicle_documents WHERE uuid = $1", p.id).Scan(p.dst); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE vehicle_documents SET file_object_key = $1 WHERE uuid = $2", aDocKey, orgB.ids["document"]); err != nil {
		t.Fatal(err)
	}
	bDocFile := "/v1/tenant/vehicles/" + orgB.ids["vehicle"] + "/documents/" + orgB.ids["document"] + "/file"
	if r := call(orgB, http.MethodGet, bDocFile, nil); r.code == http.StatusOK {
		t.Errorf("document row pointing at A's file was streamed")
	}
	if _, err := pool.Exec(ctx, "UPDATE vehicle_documents SET file_object_key = $1 WHERE uuid = $2", bDocKey, orgB.ids["document"]); err != nil {
		t.Fatal(err)
	}
	if r := call(orgB, http.MethodGet, bDocFile, nil); r.code != http.StatusOK {
		t.Errorf("B's own document file = %d: %s", r.code, r.raw)
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
	case "invitations/uuid":
		return "invitation"
	case "exports/uuid":
		return "exportJob"
	case "imports/uuid":
		return "importJob"
	case "bulk/uuid":
		return "bulkJob"
	case "holds/uuid":
		return "hold"
	case "capacity-rules/uuid":
		return "capacityRule"
	case "versions/version":
		return "version"
	case "rate-plans/uuid":
		return "ratePlan"
	case "date-rules/uuid":
		return "dateRule"
	case "customers/uuid":
		return "customer"
	case "contacts/contactUuid":
		return "contact"
	case "drivers/driverUuid":
		return "driver"
	case "documents/docUuid":
		return "customerDocument"
	case "extras/uuid":
		return "extra"
	case "equipment-allocations/uuid":
		return "equipmentAllocation"
	case "campaigns/uuid":
		return "campaign"
	case "reservations/uuid":
		return "reservation"
	case "quotes/uuid":
		return "quote"
	case "resolution-tasks/uuid":
		return "task"
	case "integrations/uuid":
		return "integrationAccount"
	case "integration-jobs/uuid":
		return "integrationJob"
	case "payments/uuid":
		return "payment"
	case "payment-links/uuid":
		return "paymentLink"
	case "rentals/uuid":
		return "rental"
	case "charges/chargeUuid":
		return "charge"
	case "operation-tasks/uuid":
		return "operationTask"
	case "field-tasks/uuid":
		return "fieldTask"
	case "contract-templates/uuid":
		return "contractTemplate"
	case "documents/uuid":
		return "contractDocument"
	case "versions/documentVersionUuid":
		return "documentVersion"
	case "inspections/uuid":
		return "inspection"
	case "inspection-templates/uuid":
		return "inspectionTemplate"
	case "damages/damageUuid":
		return "inspectionDamage"
	case "photos/photoUuid":
		return "inspectionPhoto"
	case "approval-links/linkUuid":
		return "approvalLink"
	case "approvals/approvalUuid":
		return "approval"
	case "customer-requests/uuid":
		return "customerRequest"
	case "accounts/uuid":
		return "financeAccount"
	case "entries/uuid":
		return "ledgerEntry"
	case "cash-accounts/uuid":
		return "cashAccount"
	case "movements/uuid":
		return "cashMovement"
	case "expense-categories/uuid":
		return "expenseCategory"
	case "expenses/uuid":
		return "expense"
	case "deposits/uuid":
		return "deposit"
	case "deductions/deduction":
		return "depositDeduction"
	case "collections/uuid":
		return "collection"
	case "toll-passes/uuid":
		return "tollPass"
	case "suppliers/uuid":
		return "supplier"
	case "supplier-invoices/uuid":
		return "supplierInvoice"
	case "supplier-payments/uuid":
		return "supplierPayment"
	case "toll-tags/tagUuid":
		return "tollTag"
	case "kabis-notifications/uuid":
		return "kabisNotification"
	case "invoices/uuid":
		return "invoice"
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

// previewBody is a pricing preview request (8 days from tomorrow) plus extra
// fields.
func previewBody(product, branch string, extra map[string]any) map[string]any {
	start := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Hour)
	body := map[string]any{
		"rental_product_uuid": product, "branch_uuid": branch, "channel": "web",
		"start_at": start.Format(time.RFC3339), "end_at": start.Add(8 * 24 * time.Hour).Format(time.RFC3339),
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
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
