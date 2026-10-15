package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	customeradapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recordingPublisher struct {
	mu   sync.Mutex
	pubs map[string][]string // channel → actions
}

func (p *recordingPublisher) Publish(_ context.Context, channel string, data any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pubs == nil {
		p.pubs = map[string][]string{}
	}
	if ev, ok := data.(resourceevents.Event); ok {
		p.pubs[channel] = append(p.pubs[channel], ev.Action)
	}
	return nil
}

func (p *recordingPublisher) actions(channel string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string{}, p.pubs[channel]...)
}

// TestCustomersPrivacy (TEC-705) drives /v1/tenant/customers over the real
// server: sensitive plaintext (TCKN, phone, e-mail, licence number) never
// appears in list / detail / sub-list / search document / export answers
// without an explicit reveal; reveal needs tenant.customers.reveal and a
// reason and writes an activity event; an export with include_sensitive
// needs the reveal permission and is recorded; the database holds no
// plaintext identity or licence number; anonymization wipes the personal
// data; mutations publish on the organization's live channel. Runs with
// TEST_DATABASE_URL.
func TestCustomersPrivacy(t *testing.T) {
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

	const secret = "customers-privacy-test-access-secret-0123"
	cfg := config.Config{
		App:        config.AppConfig{Name: "Privacy", Env: "test"},
		JWT:        config.JWTConfig{AccessSecret: secret, RefreshSecret: secret, AccessTTL: time.Hour, RefreshTTL: time.Hour},
		Encryption: config.EncryptionConfig{Key: strings.Repeat("p", 32)},
		Tenancy:    config.TenancyConfig{Enabled: true},
		Bulk:       config.BulkConfig{SyncMax: 10, RollbackHours: 24},
	}
	pub := &recordingPublisher{}
	srv, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Deps{
		DB: pool, Queries: q, Storage: &memStorage{m: storage.NewMemory()}, Realtime: pub,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := srv.http.Handler
	tokens, err := jwt.NewManager(secret, time.Hour, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	var userIDs []int64
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-cust-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		return u
	}
	owner, member, accountant := newUser("owner"), newUser("member"), newUser("accountant")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Privacy " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM export_jobs WHERE actor_id = ANY($1)", userIDs)
		_, _ = pool.Exec(bg, "DELETE FROM activity_events WHERE actor_user_id = ANY($1)", userIDs)
		_, _ = pool.Exec(bg, "DELETE FROM notifications WHERE user_id = ANY($1)", userIDs)
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", userIDs)
	})
	if _, err := orgSvc.AddMember(ctx, org.UUID, member.Uuid, orgctx.RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.AddMember(ctx, org.UUID, accountant.Uuid, "muhasebe"); err != nil {
		t.Fatal(err)
	}

	type resp struct {
		code int
		raw  []byte
		data any
	}
	do := func(user db.User, method, path, contentType string, body io.Reader) resp {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid, Roles: []string{"cms_user"}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
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
	call := func(user db.User, method, path string, body any) resp {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		return do(user, method, path, "application/json", &buf)
	}
	must := func(r resp, want int, step string) resp {
		t.Helper()
		if r.code != want {
			t.Fatalf("%s = %d, want %d: %s", step, r.code, want, r.raw)
		}
		return r
	}
	field := func(r resp, key string) string {
		m, _ := r.data.(map[string]any)
		s, _ := m[key].(string)
		return s
	}

	// Plaintext values that must never leak without a reveal.
	const (
		tckn    = "10000000146"
		phone   = "+905321234512"
		email   = "ayse.yilmaz@domain.test"
		licence = "TRB998877"
	)
	secrets := []string{tckn, phone, "5321234512", email, licence}
	leaked := func(raw []byte) string {
		for _, s := range secrets {
			if bytes.Contains(raw, []byte(s)) {
				return s
			}
		}
		return ""
	}

	created := must(call(owner, http.MethodPost, "/v1/tenant/customers", map[string]any{
		"customer_type": "individual", "first_name": "Ayşe", "last_name": "Yılmaz", "identity_type": "tckn",
		"identity_number": "100 000 001 46", "nationality": "tr", "phone": "0532 123 45 12", "email": email,
		"tags": []string{"VIP"},
	}), http.StatusCreated, "create customer")
	id := field(created, "uuid")
	if l := leaked(created.raw); l != "" {
		t.Fatalf("create answer leaked %q: %s", l, created.raw)
	}
	if got := field(created, "identity_masked"); got != "100******46" {
		t.Fatalf("identity_masked = %q", got)
	}
	if got := field(created, "phone_masked"); got != "+90 5** *** ** 12" {
		t.Fatalf("phone_masked = %q", got)
	}
	if got := field(created, "email_masked"); got != "a***@d***.test" {
		t.Fatalf("email_masked = %q", got)
	}
	base := "/v1/tenant/customers/" + id
	// Duplicate identity in the organization: 409 on identity_number.
	dup := call(owner, http.MethodPost, "/v1/tenant/customers", map[string]any{
		"customer_type": "individual", "first_name": "X", "last_name": "Y", "identity_type": "tckn", "identity_number": tckn,
	})
	if dup.code != http.StatusConflict || !bytes.Contains(dup.raw, []byte(`"identity_number"`)) {
		t.Fatalf("duplicate identity = %d: %s", dup.code, dup.raw)
	}
	// Bad checksum: 400 on identity_number.
	if r := call(owner, http.MethodPost, "/v1/tenant/customers", map[string]any{
		"customer_type": "individual", "first_name": "X", "last_name": "Y", "identity_type": "tckn", "identity_number": "10000000147",
	}); r.code != http.StatusBadRequest || !bytes.Contains(r.raw, []byte("invalid_checksum")) {
		t.Fatalf("bad checksum = %d: %s", r.code, r.raw)
	}
	driver := must(call(owner, http.MethodPost, base+"/drivers", map[string]any{
		"copy_from_customer": true, "birth_date": "1990-05-01", "licence_number": "trb-998877", "licence_class": "b",
		"licence_issued_on": "2012-06-01",
	}), http.StatusCreated, "customer is also driver")
	driverID := field(driver, "uuid")
	if l := leaked(driver.raw); l != "" {
		t.Fatalf("driver answer leaked %q", l)
	}
	if field(driver, "licence_masked") != "******877" || field(driver, "identity_masked") != "100******46" {
		t.Fatalf("driver masks: %s", driver.raw)
	}

	// The database holds ciphertext, blind indexes and hints only.
	var plainRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM customers c
		WHERE c.organization_id = $1 AND (c::text LIKE '%' || $2 || '%')`, org.ID, tckn).Scan(&plainRows); err != nil {
		t.Fatal(err)
	}
	var plainDrivers int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM drivers d
		WHERE d.organization_id = $1 AND (d::text LIKE '%' || $2 || '%' OR d::text LIKE '%' || $3 || '%')`,
		org.ID, tckn, licence).Scan(&plainDrivers); err != nil {
		t.Fatal(err)
	}
	if plainRows != 0 || plainDrivers != 0 {
		t.Fatalf("plaintext identity / licence stored: customers=%d drivers=%d", plainRows, plainDrivers)
	}

	// Reads as a member (tenant.customers.read only) and as the owner, no
	// reveal: nothing sensitive in any answer.
	for _, u := range []db.User{member, owner} {
		for _, path := range []string{
			"/v1/tenant/customers", "/v1/tenant/customers?q=Ayşe", "/v1/tenant/customers?q=" + tckn,
			"/v1/tenant/customers?q=0532", base, base + "/drivers", base + "/drivers/" + driverID, base + "/activity",
			base + "/consents", base + "/contacts", base + "/documents", "/v1/tenant/customers/facets",
		} {
			r := must(call(u, http.MethodGet, path, nil), http.StatusOK, "GET "+path)
			if l := leaked(r.raw); l != "" {
				t.Errorf("%s (%s) leaked %q: %s", path, u.Name, l, r.raw)
			}
		}
	}
	// Exact identity lookup by blind index, digits lookup by phone.
	for _, qy := range []string{tckn, "0532 123", "Yılmaz"} {
		r := must(call(member, http.MethodGet, "/v1/tenant/customers?q="+strings.ReplaceAll(qy, " ", "%20"), nil), http.StatusOK, "search "+qy)
		if !bytes.Contains(r.raw, []byte(id)) {
			t.Errorf("q=%s does not find the customer: %s", qy, r.raw)
		}
	}
	if r := must(call(member, http.MethodGet, "/v1/tenant/customers?q=1000000", nil), http.StatusOK, "partial"); bytes.Contains(r.raw, []byte(id)) {
		t.Errorf("a partial identity number must not match")
	}

	// Search document (Cmd+K): masked hint and last digits only.
	doc, err := customeradapters.NewSearch(q).Document(ctx, searchengine.TenantDocumentID(org.ID, id))
	if err != nil {
		t.Fatal(err)
	}
	rawDoc, _ := json.Marshal(doc)
	if l := leaked(rawDoc); l != "" {
		t.Fatalf("search document leaked %q: %s", l, rawDoc)
	}
	if !bytes.Contains(rawDoc, []byte(`"4512"`)) || !bytes.Contains(rawDoc, []byte("100******46")) {
		t.Fatalf("search document lacks the masked keywords: %s", rawDoc)
	}

	// Export without include_sensitive (owner): masked.
	exportFile := func(user db.User, query map[string]any) (resp, []byte) {
		t.Helper()
		r := call(user, http.MethodPost, "/v1/tenant/customers/export", map[string]any{"format": "csv", "query": query})
		if r.code != http.StatusAccepted {
			return r, nil
		}
		file := must(do(user, http.MethodGet, "/v1/tenant/exports/"+field(r, "uuid")+"/download", "", nil), http.StatusOK, "download")
		return r, file.raw
	}
	if _, file := exportFile(owner, map[string]any{}); file == nil || leaked(file) != "" || !bytes.Contains(file, []byte("100******46")) {
		t.Fatalf("masked export: %s", file)
	}
	// Accounting has export but not reveal: include_sensitive is 403.
	if r, _ := exportFile(accountant, map[string]any{"include_sensitive": "true"}); r.code != http.StatusForbidden {
		t.Fatalf("include_sensitive without reveal = %d: %s", r.code, r.raw)
	}
	// The member has no export permission at all.
	if r, _ := exportFile(member, map[string]any{}); r.code != http.StatusForbidden {
		t.Fatalf("member export = %d", r.code)
	}
	// Owner with include_sensitive: plaintext in the file, activity recorded.
	if _, file := exportFile(owner, map[string]any{"include_sensitive": "true"}); !bytes.Contains(file, []byte(tckn)) || !bytes.Contains(file, []byte(email)) {
		t.Fatalf("sensitive export lacks plaintext: %s", file)
	}
	var sensitiveExports int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events WHERE organization_id = $1
		AND action = 'customers.sensitive_export_requested' AND actor_user_id = $2`, org.ID, owner.ID).Scan(&sensitiveExports); err != nil {
		t.Fatal(err)
	}
	if sensitiveExports != 1 {
		t.Fatalf("sensitive export activity events = %d", sensitiveExports)
	}

	// Reveal: member 403, owner without a reason 400, with a reason 200 and
	// an activity event naming the reason.
	if r := call(member, http.MethodPost, base+"/reveal", map[string]any{"reason": "kontrol"}); r.code != http.StatusForbidden || leaked(r.raw) != "" {
		t.Fatalf("member reveal = %d: %s", r.code, r.raw)
	}
	if r := call(owner, http.MethodPost, base+"/reveal", map[string]any{"reason": " "}); r.code != http.StatusBadRequest {
		t.Fatalf("reveal without reason = %d: %s", r.code, r.raw)
	}
	revealed := must(call(owner, http.MethodPost, base+"/reveal", map[string]any{"reason": "Kiralama sözleşmesi"}), http.StatusOK, "reveal")
	if field(revealed, "identity_number") != tckn || field(revealed, "phone") != phone || field(revealed, "email") != email {
		t.Fatalf("reveal answer: %s", revealed.raw)
	}
	if !bytes.Contains(revealed.raw, []byte(licence)) {
		t.Fatalf("reveal lacks the driver licence: %s", revealed.raw)
	}
	var reason string
	if err := pool.QueryRow(ctx, `SELECT payload->>'reason' FROM activity_events WHERE organization_id = $1
		AND action = 'customers.revealed' AND resource = 'tenant.customers' AND resource_uuid = $2`, org.ID, id).Scan(&reason); err != nil {
		t.Fatalf("reveal activity event: %v", err)
	}
	if reason != "Kiralama sözleşmesi" {
		t.Fatalf("reveal reason = %q", reason)
	}
	// No activity payload of the customer carries a plaintext value.
	var payloads string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(string_agg(payload::text, ' '), '') FROM activity_events
		WHERE organization_id = $1 AND resource = 'tenant.customers'`, org.ID).Scan(&payloads); err != nil {
		t.Fatal(err)
	}
	if l := leaked([]byte(payloads)); l != "" {
		t.Fatalf("activity payload leaked %q", l)
	}

	// Documents: the file needs tenant.document.download + reveal.
	docRow := must(call(owner, http.MethodPost, base+"/documents", map[string]any{"doc_type": "identity"}), http.StatusCreated, "document")
	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	fw, _ := mw.CreateFormFile("file", "kimlik.png")
	_, _ = fw.Write(append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...))
	_ = mw.Close()
	filePath := base + "/documents/" + field(docRow, "uuid") + "/file"
	must(do(owner, http.MethodPut, filePath, mw.FormDataContentType(), &form), http.StatusOK, "upload")
	if r := do(member, http.MethodGet, filePath, "", nil); r.code != http.StatusForbidden {
		t.Fatalf("member download = %d", r.code)
	}
	must(do(owner, http.MethodGet, filePath, "", nil), http.StatusOK, "owner download")

	// Live list: mutations publish on the organization's channel.
	channel := "org:" + org.UUID.String() + ":resource:tenant.customers"
	if got := pub.actions(channel); len(got) < 2 || got[0] != resourceevents.ActionCreated {
		t.Fatalf("live events on %s: %v", channel, got)
	}

	// Anonymize: the row stays, every personal value is gone.
	if r := call(member, http.MethodPost, base+"/anonymize", map[string]any{"reason": "KVKK"}); r.code != http.StatusForbidden {
		t.Fatalf("member anonymize = %d", r.code)
	}
	anon := must(call(owner, http.MethodPost, base+"/anonymize", map[string]any{"reason": "KVKK başvurusu"}), http.StatusOK, "anonymize")
	if m, _ := anon.data.(map[string]any); m["anonymized"] != true || m["display_name"] != "" || m["identity_masked"] != "" {
		t.Fatalf("anonymized customer: %s", anon.raw)
	}
	if r := call(owner, http.MethodPatch, base, map[string]any{"note": "x"}); r.code != http.StatusConflict {
		t.Fatalf("patch anonymized = %d", r.code)
	}
	var leftovers int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM drivers WHERE organization_id = $1 AND (first_name <> '' OR licence_enc <> '' OR identity_enc <> ''))
		+ (SELECT COUNT(*) FROM customer_documents WHERE organization_id = $1)
		+ (SELECT COUNT(*) FROM customers WHERE organization_id = $1 AND (phone <> '' OR email <> '' OR identity_enc <> ''))`,
		org.ID).Scan(&leftovers); err != nil {
		t.Fatal(err)
	}
	if leftovers != 0 {
		t.Fatalf("personal data left after anonymization: %d rows", leftovers)
	}
	// The identity number is free again for a new customer.
	must(call(owner, http.MethodPost, "/v1/tenant/customers", map[string]any{
		"customer_type": "individual", "first_name": "Yeni", "last_name": "Kayıt", "identity_type": "tckn", "identity_number": tckn,
	}), http.StatusCreated, "identity reusable after anonymization")
}
