// Package whatsappfake is the development / test WhatsApp provider
// (whatsapp_fake). It is registered outside production only.
//
// Behaviour:
//   - sessions live in memory; after Connect a session links by itself
//     after AutoPair (development) or when Pair is called (tests). A
//     session this process does not know (created by the API, read by the
//     worker) reports linked, so the worker's health sweep agrees with the
//     panel;
//   - numbers ending in 0000 are not on WhatsApp, numbers ending in 9999
//     are rejected on send;
//   - sent messages are kept (Sent); when Receipts is set, a delivery
//     receipt follows after ReceiptDelay and a read receipt after twice
//     that, delivered through the callback in this process.
package whatsappfake

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"image/png"
	"strings"
	"sync"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp"
)

// ProviderCode is the integration provider code.
const ProviderCode = "whatsapp_fake"

// LinkedJID is the account the fake links ("+90 555 000 00 00").
const LinkedJID = "905550000000:1@s.whatsapp.net"

// ReceiptFunc receives a fake receipt for a sent message.
type ReceiptFunc func(ctx context.Context, orgID int64, messageID, state string, at time.Time)

// Sent is one sent message.
type Sent struct {
	OrgID    int64
	To       string
	Body     string
	ID       string
	Document string
}

type session struct {
	connectedAt time.Time
	paired      bool
	loggedOut   bool
}

// Fake is the in-memory provider.
type Fake struct {
	// AutoPair links a connected session after this long (0 = only Pair).
	AutoPair time.Duration
	// ReceiptDelay is the delay of the delivery receipt (0 = 2s).
	ReceiptDelay time.Duration

	mu       sync.Mutex
	sessions map[string]*session
	deleted  map[string]bool
	sent     []Sent
	receipts ReceiptFunc
	now      func() time.Time
}

// New builds the fake.
func New() *Fake {
	return &Fake{sessions: map[string]*session{}, deleted: map[string]bool{}, now: time.Now}
}

// SetReceipts installs the receipt callback (nil = no receipts).
func (f *Fake) SetReceipts(fn ReceiptFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.receipts = fn
}

// Kind implements integrations.Adapter.
func (*Fake) Kind() string { return whatsapp.Kind }

// Provider implements integrations.Adapter.
func (*Fake) Provider() string { return ProviderCode }

// Fields implements integrations.Adapter.
func (*Fake) Fields() []integrations.Field { return []integrations.Field{} }

// Configured implements whatsapp.Provider.
func (*Fake) Configured() bool { return true }

// Check implements integrations.Adapter.
func (f *Fake) Check(ctx context.Context, a integrations.Account) error {
	st, err := f.Status(ctx, a)
	if err != nil {
		return err
	}
	if !st.LoggedIn {
		return &whatsapp.Rejected{Status: 409, Message: "not logged in"}
	}
	return nil
}

func (f *Fake) get(a integrations.Account) (string, *session, error) {
	t := whatsapp.Token(a)
	if t == "" {
		return "", nil, whatsapp.ErrNoSession
	}
	if f.deleted[t] {
		return t, nil, &whatsapp.Rejected{Status: 401, Message: "unauthorized"}
	}
	return t, f.sessions[t], nil
}

func (f *Fake) linked(s *session) bool {
	if s == nil {
		return true // created by another process
	}
	if s.loggedOut {
		return false
	}
	if s.paired {
		return true
	}
	return f.AutoPair > 0 && !s.connectedAt.IsZero() && f.now().Sub(s.connectedAt) >= f.AutoPair
}

// CreateSession implements whatsapp.Provider.
func (f *Fake) CreateSession(_ context.Context, in whatsapp.SessionInput) (whatsapp.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[in.Token] = &session{}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return whatsapp.Session{UserID: "fake-" + hex.EncodeToString(b), Token: in.Token}, nil
}

// DeleteSession implements whatsapp.Provider.
func (f *Fake) DeleteSession(_ context.Context, a integrations.Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t := whatsapp.Token(a); t != "" {
		delete(f.sessions, t)
		f.deleted[t] = true
	}
	return nil
}

// Connect implements whatsapp.Provider.
func (f *Fake) Connect(_ context.Context, a integrations.Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, s, err := f.get(a)
	if err != nil {
		return err
	}
	if s == nil {
		s = &session{}
		f.sessions[t] = s
	}
	if s.connectedAt.IsZero() || s.loggedOut {
		s.connectedAt, s.loggedOut, s.paired = f.now(), false, false
	}
	return nil
}

// QR implements whatsapp.Provider: a real QR code (PNG data URL) of a
// placeholder text that changes every 20 seconds, like WhatsApp's.
func (f *Fake) QR(_ context.Context, a integrations.Account) (string, error) {
	f.mu.Lock()
	t, s, err := f.get(a)
	linked := err == nil && f.linked(s)
	f.mu.Unlock()
	if err != nil {
		return "", err
	}
	if s == nil || s.connectedAt.IsZero() {
		return "", &whatsapp.Rejected{Status: 400, Message: "not connected"}
	}
	if linked {
		return "", whatsapp.ErrAlreadyLoggedIn
	}
	if len(t) > 8 {
		t = t[:8]
	}
	content := "whatsapp-fake:" + t + ":" + f.now().UTC().Truncate(20*time.Second).Format(time.RFC3339)
	code, err := qr.Encode(content, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	scaled, err := barcode.Scale(code, 256, 256)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Status implements whatsapp.Provider.
func (f *Fake) Status(_ context.Context, a integrations.Account) (whatsapp.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, s, err := f.get(a)
	if err != nil {
		return whatsapp.Status{}, err
	}
	if f.linked(s) {
		return whatsapp.Status{Connected: true, LoggedIn: true, JID: LinkedJID}, nil
	}
	return whatsapp.Status{Connected: s != nil && !s.connectedAt.IsZero() && !s.loggedOut}, nil
}

// Pair links a session (tests).
func (f *Fake) Pair(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sessions[token]
	if s == nil {
		s = &session{}
		f.sessions[token] = s
	}
	s.paired, s.loggedOut = true, false
	if s.connectedAt.IsZero() {
		s.connectedAt = f.now()
	}
}

// Drop unlinks a session as if the phone logged out (tests).
func (f *Fake) Drop(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s := f.sessions[token]; s != nil {
		s.loggedOut, s.paired = true, false
	} else {
		f.sessions[token] = &session{loggedOut: true}
	}
}

// Logout implements whatsapp.Provider.
func (f *Fake) Logout(_ context.Context, a integrations.Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, s, err := f.get(a)
	if err != nil {
		return err
	}
	if s == nil {
		s = &session{}
		f.sessions[t] = s
	}
	s.loggedOut, s.paired, s.connectedAt = true, false, time.Time{}
	return nil
}

// OnWhatsApp implements whatsapp.Provider.
func (f *Fake) OnWhatsApp(_ context.Context, a integrations.Account, phone string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, _, err := f.get(a); err != nil {
		return false, err
	}
	return !strings.HasSuffix(whatsapp.Digits(phone), "0000"), nil
}

func (f *Fake) send(a integrations.Account, s Sent) (whatsapp.SendResult, error) {
	f.mu.Lock()
	_, sess, err := f.get(a)
	if err == nil && !f.linked(sess) {
		err = &whatsapp.Rejected{Status: 409, Message: "not logged in"}
	}
	if err == nil && strings.HasSuffix(whatsapp.Digits(s.To), "9999") {
		err = &whatsapp.Rejected{Status: 400, Message: "invalid recipient"}
	}
	if err != nil {
		f.mu.Unlock()
		return whatsapp.SendResult{}, err
	}
	s.OrgID = a.OrganizationID
	f.sent = append(f.sent, s)
	cb, delay := f.receipts, f.ReceiptDelay
	f.mu.Unlock()
	if delay <= 0 {
		delay = 2 * time.Second
	}
	if cb != nil {
		org, id := a.OrganizationID, s.ID
		go func() {
			time.Sleep(delay)
			cb(context.Background(), org, id, whatsapp.ReceiptDelivered, time.Now().UTC())
			time.Sleep(delay)
			cb(context.Background(), org, id, whatsapp.ReceiptRead, time.Now().UTC())
		}()
	}
	return whatsapp.SendResult{MessageID: s.ID, Timestamp: f.now().UTC()}, nil
}

// SendText implements whatsapp.Provider.
func (f *Fake) SendText(_ context.Context, a integrations.Account, m whatsapp.TextMessage) (whatsapp.SendResult, error) {
	return f.send(a, Sent{To: m.To, Body: m.Body, ID: m.ID})
}

// SendDocument implements whatsapp.Provider.
func (f *Fake) SendDocument(_ context.Context, a integrations.Account, m whatsapp.DocumentMessage) (whatsapp.SendResult, error) {
	return f.send(a, Sent{To: m.To, Body: m.Caption, ID: m.ID, Document: m.FileName})
}

// ParseWebhook implements whatsapp.Provider (the wuzapi format).
func (*Fake) ParseWebhook(contentType string, body []byte) ([]whatsapp.Event, error) {
	return whatsapp.ParseWuzapiWebhook(contentType, body)
}

// Sent returns the sent messages.
func (f *Fake) Sent() []Sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Sent(nil), f.sent...)
}

var _ whatsapp.Provider = (*Fake)(nil)
