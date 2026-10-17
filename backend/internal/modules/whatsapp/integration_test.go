package whatsapp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/catalog"
	nemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/model"
	engine "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/whatsappfake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mailbox struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (m *mailbox) Send(_ context.Context, msg mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mailbox) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

type notifier struct {
	mu  sync.Mutex
	got []notifmodel.EnqueueInput
}

func (n *notifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.got = append(n.got, in)
	return nil, nil
}

type env struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	org      int64
	customer int64
	fake     *whatsappfake.Fake
	accounts *accountsusecase.Service
	vault    *integrations.Vault
	svc      *usecase.Service
	eng      *engine.Engine
	mail     *mailbox
	alerts   *notifier
	now      time.Time
}

// newEnv: one organization, the fake WhatsApp provider, the engine with
// the WhatsApp channel and e-mail (fallback), a customer with a mobile
// number and an e-mail address.
func newEnv(t *testing.T, phone string) *env {
	t.Helper()
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
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "wa-" + suffix, Name: "WhatsApp " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID) })
	fc, err := crypto.NewFieldCipher(strings.Repeat("w", 32))
	if err != nil {
		t.Fatal(err)
	}
	vault, err := integrations.NewVault(fc)
	if err != nil {
		t.Fatal(err)
	}
	fake := whatsappfake.New()
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(fake))
	svc := usecase.New(q, accounts, vault, usecase.Config{WebhookBaseURL: "http://backend.test:8080"}, fake)
	alerts := &notifier{}
	svc.SetNotifier(alerts)
	e := &env{ctx: ctx, pool: pool, q: q, org: org.ID, fake: fake, accounts: accounts, vault: vault, svc: svc,
		mail: &mailbox{}, alerts: alerts, now: time.Now().UTC()}
	svc.SetClock(func() time.Time { return e.now })
	e.eng = engine.New(pool, q, "https://app.example.test")
	e.eng.Register(usecase.NewChannel(svc), adapters.NewEmail(e.mail))
	if err := pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', 'Ayşe', 'Yılmaz', $2, 'ayse@example.test') RETURNING id`, org.ID, phone).Scan(&e.customer); err != nil {
		t.Fatal(err)
	}
	if _, err := e.eng.PutRule(ctx, org.ID, catalog.ReservationConfirmed, nemodel.RuleInput{
		Enabled: true, Channels: []string{"whatsapp", "email"}, Fallback: true,
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) account(t *testing.T) integrations.Account {
	t.Helper()
	acc, err := e.accounts.ActiveAccount(e.ctx, e.org, whatsapp.Kind)
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

// link connects and pairs the session.
func (e *env) link(t *testing.T) {
	t.Helper()
	s, err := e.svc.Connect(e.ctx, e.org, model.ConnectInput{AcknowledgeRisk: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != model.StatusPairing || s.Provider != whatsappfake.ProviderCode || s.RiskAcknowledgedAt == nil {
		t.Fatalf("after connect: %+v", s)
	}
	qr, err := e.svc.QR(e.ctx, e.org)
	if err != nil {
		t.Fatal(err)
	}
	if qr.Status != model.StatusPairing || !strings.HasPrefix(qr.QRCode, "data:image/png;base64,") {
		t.Fatalf("qr: %s %d", qr.Status, len(qr.QRCode))
	}
	e.fake.Pair(whatsapp.Token(e.account(t)))
	qr, err = e.svc.QR(e.ctx, e.org)
	if err != nil {
		t.Fatal(err)
	}
	if qr.Status != model.StatusConnected || qr.QRCode != "" {
		t.Fatalf("qr after pairing: %+v", qr)
	}
}

func (e *env) emit(t *testing.T, key string) engine.Emitted {
	t.Helper()
	ref := uuid.New()
	out, err := e.eng.Emit(e.ctx, e.org, engine.EmitInput{
		Event: catalog.ReservationConfirmed, CustomerID: e.customer, RefType: "reservation", RefUUID: &ref, Key: key,
		Vars: map[string]string{"reference": "R-7", "pickup_at": "18.10.2026 10:00", "pickup_branch": "Merkez"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Created {
		t.Fatal("dispatch not created")
	}
	if err := e.eng.Process(e.ctx, e.org, out.Dispatch.ID); err != nil {
		t.Fatal(err)
	}
	return out
}

func attempts(d nemodel.Dispatch) string {
	parts := make([]string, 0, len(d.Attempts))
	for _, a := range d.Attempts {
		parts = append(parts, a.Channel+":"+a.Status)
	}
	return strings.Join(parts, ",")
}

func (e *env) dispatch(t *testing.T, id uuid.UUID) nemodel.Dispatch {
	t.Helper()
	d, err := e.eng.GetDispatch(e.ctx, e.org, id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (e *env) messages(t *testing.T) []model.Message {
	t.Helper()
	items, _, err := e.svc.ListMessages(e.ctx, e.org, model.MessageFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// Connect → QR → linked; the session token is sealed in the integration
// account and never shown; the engine message goes out on WhatsApp and the
// delivery receipt (webhook) settles it; the read receipt is logged.
func TestConnectSendAndReceipts(t *testing.T) {
	e := newEnv(t, "+905321112233")
	if _, err := e.svc.Connect(e.ctx, e.org, model.ConnectInput{}); err == nil {
		t.Fatal("connect without the risk notice")
	}
	e.link(t)
	s, err := e.svc.Session(e.ctx, e.org)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != model.StatusConnected || s.PhoneMasked != "+90 5** *** ** 00" || s.ConnectedAt == nil {
		t.Fatalf("session: %+v", s)
	}
	acc := e.account(t)
	if len(whatsapp.Token(acc)) != 64 || acc.Credentials[whatsapp.CredentialWebhookToken] == "" {
		t.Fatalf("credentials: %v", len(acc.Credentials))
	}
	var hint string
	if err := e.pool.QueryRow(e.ctx, `SELECT credentials_hint::text FROM integration_accounts WHERE organization_id = $1 AND kind = 'whatsapp'`, e.org).Scan(&hint); err != nil {
		t.Fatal(err)
	}
	if hint != "{}" {
		t.Fatalf("hint exposes the token: %s", hint)
	}

	out := e.emit(t, "status:confirmed")
	d := e.dispatch(t, out.Dispatch.Uuid)
	if d.Status != engine.DispatchSending || attempts(d) != "whatsapp:sent" {
		t.Fatalf("after send: %s %s (%s)", d.Status, attempts(d), d.LastError)
	}
	sent := e.fake.Sent()
	if len(sent) != 1 || sent[0].To != "+905321112233" || !strings.Contains(sent[0].Body, "R-7") {
		t.Fatalf("sent: %+v", sent)
	}
	msgs := e.messages(t)
	if len(msgs) != 1 || msgs[0].Status != model.MessageSent || msgs[0].Purpose != "notify."+catalog.ReservationConfirmed ||
		msgs[0].RecipientMasked != "+90 5** *** ** 33" || msgs[0].CustomerName != "Ayşe Yılmaz" || msgs[0].DeliveryUUID == nil {
		t.Fatalf("log: %+v", msgs)
	}

	// Webhook: delivery receipt (JSON), then read receipt (form).
	h := handler.New(e.svc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/public/whatsapp/webhook/{token}", h.Webhook)
	hook := acc.Credentials[whatsapp.CredentialWebhookToken]
	post := func(token, ct, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/v1/public/whatsapp/webhook/"+token, strings.NewReader(body))
		req.Header.Set("Content-Type", ct)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr.Code
	}
	if c := post(strings.Repeat("a", 64), "application/json", `{"type":"ReadReceipt","state":"Delivered","event":{"MessageIDs":["`+sent[0].ID+`"]}}`); c != http.StatusNotFound {
		t.Fatalf("unknown token: %d", c)
	}
	if c := post(hook, "application/json", `{"type":"ReadReceipt","state":"Delivered","event":{"MessageIDs":["`+sent[0].ID+`"],"Timestamp":"2026-10-16T10:00:00Z"}}`); c != http.StatusOK {
		t.Fatalf("receipt: %d", c)
	}
	d = e.dispatch(t, out.Dispatch.Uuid)
	if d.Status != engine.DispatchDelivered || attempts(d) != "whatsapp:delivered" {
		t.Fatalf("after receipt: %s %s", d.Status, attempts(d))
	}
	form := "jsonData=" + strings.NewReplacer("{", "%7B", "}", "%7D", `"`, "%22", ":", "%3A", ",", "%2C", "[", "%5B", "]", "%5D").
		Replace(`{"type":"ReadReceipt","state":"Read","event":{"MessageIDs":["`+sent[0].ID+`"]}}`)
	if c := post(hook, "application/x-www-form-urlencoded", form); c != http.StatusOK {
		t.Fatalf("read receipt: %d", c)
	}
	msgs = e.messages(t)
	if msgs[0].Status != model.MessageRead || msgs[0].ReadAt == nil || msgs[0].DeliveredAt == nil {
		t.Fatalf("after read: %+v", msgs[0])
	}
	if e.mail.count() != 0 {
		t.Fatal("e-mail sent although WhatsApp delivered")
	}
}

// Not linked / number not on WhatsApp → skipped, the e-mail runs; a send
// without a receipt fails after the timeout and the e-mail runs.
func TestFallbacks(t *testing.T) {
	e := newEnv(t, "+905320000000") // not on WhatsApp (fake rule)
	out := e.emit(t, "k1")
	d := e.dispatch(t, out.Dispatch.Uuid)
	if attempts(d) != "whatsapp:skipped,email:sent" || !strings.Contains(d.Attempts[0].Error, "not") {
		t.Fatalf("not set up: %s %+v", attempts(d), d.Attempts)
	}
	e.link(t)
	out = e.emit(t, "k2")
	d = e.dispatch(t, out.Dispatch.Uuid)
	if attempts(d) != "whatsapp:skipped,email:sent" || d.Attempts[0].Error != usecase.ErrNotOnWhatsApp.Error() {
		t.Fatalf("not on WhatsApp: %s %+v", attempts(d), d.Attempts)
	}

	if _, err := e.pool.Exec(e.ctx, `UPDATE customers SET phone = '+905321112244' WHERE organization_id = $1 AND id = $2`, e.org, e.customer); err != nil {
		t.Fatal(err)
	}
	out = e.emit(t, "k3")
	d = e.dispatch(t, out.Dispatch.Uuid)
	if attempts(d) != "whatsapp:sent" {
		t.Fatalf("sent: %s", attempts(d))
	}
	e.now = e.now.Add(usecase.ReceiptTimeout + time.Minute)
	if _, err := e.svc.Watch(e.ctx); err != nil {
		t.Fatal(err)
	}
	d = e.dispatch(t, out.Dispatch.Uuid)
	if d.Status != engine.DispatchDelivered || attempts(d) != "whatsapp:failed,email:sent" {
		t.Fatalf("after timeout: %s %s", d.Status, attempts(d))
	}
	msgs := e.messages(t)
	if msgs[0].Status != model.MessageFailed || !strings.Contains(msgs[0].Error, "receipt") {
		t.Fatalf("log after timeout: %+v", msgs[0])
	}
	if e.mail.count() != 3 {
		t.Fatalf("e-mails: %d", e.mail.count())
	}
}

// Rate limits: per session and minute, per recipient and hour.
func TestRateLimits(t *testing.T) {
	e := newEnv(t, "+905321112233")
	e.link(t)
	two := int32(2)
	if _, err := e.svc.UpdateSettings(e.ctx, e.org, model.SettingsInput{RateLimitPerMinute: &two}); err != nil {
		t.Fatal(err)
	}
	bad := int32(0)
	var ve *usecase.ValidationError
	if _, err := e.svc.UpdateSettings(e.ctx, e.org, model.SettingsInput{RateLimitPerMinute: &bad}); !errors.As(err, &ve) {
		t.Fatalf("invalid limit: %v", err)
	}
	for i, phone := range []string{"+905321110001", "+905321110002"} {
		if _, err := e.svc.TestSend(e.ctx, e.org, model.TestInput{Phone: phone}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if _, err := e.svc.TestSend(e.ctx, e.org, model.TestInput{Phone: "+905321110003"}); !errors.Is(err, usecase.ErrRateLimited) {
		t.Fatalf("third send in a minute: %v", err)
	}
	e.now = e.now.Add(2 * time.Minute)
	sixty := int32(60)
	if _, err := e.svc.UpdateSettings(e.ctx, e.org, model.SettingsInput{RateLimitPerMinute: &sixty}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < usecase.PerRecipientPerHour-1; i++ {
		if _, err := e.svc.TestSend(e.ctx, e.org, model.TestInput{Phone: "+905321110001"}); err != nil {
			t.Fatalf("recipient send %d: %v", i, err)
		}
	}
	if _, err := e.svc.TestSend(e.ctx, e.org, model.TestInput{Phone: "0532 111 00 01"}); !errors.Is(err, usecase.ErrRateLimited) {
		t.Fatalf("recipient limit: %v", err)
	}
	// A rejected number is logged as failed.
	if _, err := e.svc.TestSend(e.ctx, e.org, model.TestInput{Phone: "+905321119999"}); err == nil {
		t.Fatal("rejected number sent")
	}
	msgs := e.messages(t)
	if msgs[0].Status != model.MessageFailed || !msgs[0].Test {
		t.Fatalf("rejected log: %+v", msgs[0])
	}
}

// A linked session that drops is marked disconnected and alerts the
// integration managers once; logout and remove reset the state; a new
// connect after remove uses a new account.
func TestDropAlertLogoutRemove(t *testing.T) {
	e := newEnv(t, "+905321112233")
	e.link(t)
	// An owner (tenant.integrations.manage) gets the alert, in English.
	var userID int64
	email := "wa-owner-" + uuid.NewString()[:8] + "@example.test"
	if err := e.pool.QueryRow(e.ctx, `INSERT INTO users (email, password_hash, name, surname, status, locale)
		VALUES ($1, 'x', 'Owner', 'One', 'active', 'en') RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID) })
	if _, err := e.pool.Exec(e.ctx, `WITH r AS (SELECT id FROM roles WHERE slug = 'owner' AND scope = 'tenant' AND organization_id IS NULL),
		m AS (INSERT INTO organization_members (organization_id, user_id, role_id) SELECT $1, $2, r.id FROM r RETURNING id, role_id)
		INSERT INTO organization_member_roles (organization_id, member_id, role_id) SELECT $1, m.id, m.role_id FROM m`, e.org, userID); err != nil {
		t.Fatal(err)
	}
	token := whatsapp.Token(e.account(t))
	e.fake.Drop(token)
	if _, err := e.svc.Watch(e.ctx); err != nil {
		t.Fatal(err)
	}
	s, _ := e.svc.Session(e.ctx, e.org)
	if s.Status != model.StatusDisconnected || s.DisconnectedAt == nil {
		t.Fatalf("after drop: %+v", s)
	}
	if _, err := e.svc.Watch(e.ctx); err != nil {
		t.Fatal(err)
	}
	// Once, although the second sweep still sees the drop.
	if len(e.alerts.got) != 1 || *e.alerts.got[0].UserID != userID || e.alerts.got[0].Title != "WhatsApp disconnected" ||
		*e.alerts.got[0].ActionURL == "" || !strings.HasSuffix(*e.alerts.got[0].ActionURL, "/settings/whatsapp") {
		t.Fatalf("alerts: %+v", e.alerts.got)
	}
	if _, err := e.svc.TestSend(e.ctx, e.org, model.TestInput{Phone: "+905321112233"}); !errors.Is(err, usecase.ErrNotConnected) {
		t.Fatalf("send while dropped: %v", err)
	}
	// Re-link, then logout.
	e.fake.Pair(token)
	s, err := e.svc.Refresh(e.ctx, e.org)
	if err != nil || s.Status != model.StatusConnected {
		t.Fatalf("relinked: %+v %v", s, err)
	}
	s, err = e.svc.Logout(e.ctx, e.org)
	if err != nil || s.Status != model.StatusLoggedOut || s.PhoneMasked != "" {
		t.Fatalf("logout: %+v %v", s, err)
	}
	first := e.account(t).UUID
	s, err = e.svc.Remove(e.ctx, e.org)
	if err != nil || s.Status != model.StatusNotConnected {
		t.Fatalf("remove: %+v %v", s, err)
	}
	e.link(t)
	if e.account(t).UUID == first {
		t.Fatal("connect after remove reused the disabled account")
	}
}

// Webhook tokens are per session: another organization's token never
// touches this organization's messages.
func TestWebhookIsolation(t *testing.T) {
	a := newEnv(t, "+905321112233")
	b := newEnv(t, "+905321112255")
	a.link(t)
	b.link(t)
	if _, err := a.svc.TestSend(a.ctx, a.org, model.TestInput{Phone: "+905321112233"}); err != nil {
		t.Fatal(err)
	}
	msgA := a.messages(t)[0]
	hookB := b.account(t).Credentials[whatsapp.CredentialWebhookToken]
	body := `{"type":"ReadReceipt","state":"Read","event":{"MessageIDs":["` + msgA.ProviderMessageID + `"]}}`
	if err := b.svc.HandleWebhook(b.ctx, hookB, "application/json", []byte(body)); err != nil {
		t.Fatal(err)
	}
	if got := a.messages(t)[0]; got.Status != model.MessageSent {
		t.Fatalf("B's webhook changed A's message: %s", got.Status)
	}
	if err := a.svc.HandleWebhook(a.ctx, "short", "application/json", []byte(body)); !errors.Is(err, usecase.ErrWebhookUnknown) {
		t.Fatalf("short token: %v", err)
	}
}

// The fake's receipts callback drives receipts in development.
func TestFakeReceiptsCallback(t *testing.T) {
	e := newEnv(t, "+905321112233")
	e.fake.ReceiptDelay = 20 * time.Millisecond
	e.fake.SetReceipts(e.svc.ApplyReceipt)
	e.link(t)
	out := e.emit(t, "cb")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if d := e.dispatch(t, out.Dispatch.Uuid); d.Status == engine.DispatchDelivered {
			msgs := e.messages(t)
			for msgs[0].Status != model.MessageRead && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
				msgs = e.messages(t)
			}
			if msgs[0].Status != model.MessageRead {
				t.Fatalf("not read: %+v", msgs[0])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no delivery through the receipts callback")
}
