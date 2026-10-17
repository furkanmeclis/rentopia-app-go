// Package whatsapp is the WhatsApp provider layer (TEC-735,
// docs/design/whatsapp.md). A provider runs one WhatsApp Web session per
// organization: wuzapi (self-hosted REST gateway on whatsmeow) in
// production, whatsapp_fake in development and tests.
//
// Sessions are created by the platform, not typed in: the organization's
// integration account (kind whatsapp) carries the generated session token
// in its sealed credentials (CredentialToken). Numbers are E.164
// ("+905321234567") and never logged in full.
package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// Kind is the integration kind of WhatsApp accounts.
const Kind = "whatsapp"

// Credential keys of a WhatsApp integration account (generated, sealed).
const (
	// CredentialToken is the provider session (wuzapi user) token.
	CredentialToken = "session_token"
	// CredentialUserID is the provider's id of the session user.
	CredentialUserID = "session_user_id"
	// CredentialWebhookToken is the secret path segment of the session's
	// webhook URL (the database keeps its hash only).
	CredentialWebhookToken = "webhook_token"
)

// Token returns the session token of an account ("" before a session).
func Token(a integrations.Account) string { return strings.TrimSpace(a.Credentials[CredentialToken]) }

// ErrNoSession: the account has no provider session yet.
var ErrNoSession = errors.New("whatsapp: no session; connect WhatsApp first")

// ErrNotConfigured: the provider has no server configured (base URL /
// admin token missing).
var ErrNotConfigured = errors.New("whatsapp: the WhatsApp gateway is not configured on this server")

// ErrAlreadyLoggedIn: a QR code was asked for a session that is linked.
var ErrAlreadyLoggedIn = errors.New("whatsapp: the session is already linked")

// SessionInput describes a new provider session.
type SessionInput struct {
	// Name is the provider's label of the session ("org-<uuid>").
	Name string
	// Token is the generated session token.
	Token string
	// WebhookURL receives the session's events (receipts, connection).
	WebhookURL string
}

// Session is a created provider session.
type Session struct {
	UserID string
	Token  string
}

// Status is the provider state of a session.
type Status struct {
	// Connected: the gateway holds a socket to WhatsApp.
	Connected bool
	// LoggedIn: a phone is linked (QR scanned).
	LoggedIn bool
	// JID is the linked account ("905321234567:12@s.whatsapp.net"), if
	// the provider tells.
	JID string
}

// Phone returns the E.164 number of the linked account ("" when unknown).
func (s Status) Phone() string {
	j := s.JID
	if i := strings.IndexAny(j, ":@"); i >= 0 {
		j = j[:i]
	}
	j = strings.TrimSpace(j)
	if j == "" {
		return ""
	}
	for _, r := range j {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return "+" + j
}

// TextMessage is one text message.
type TextMessage struct {
	// To is the E.164 number.
	To   string
	Body string
	// ID is the WhatsApp message id we choose (receipts carry it).
	ID          string
	LinkPreview bool
}

// DocumentMessage is one document (PDF …) with an optional caption.
type DocumentMessage struct {
	To       string
	ID       string
	FileName string
	MimeType string
	Caption  string
	Data     []byte
}

// SendResult is an accepted message.
type SendResult struct {
	MessageID string
	Timestamp time.Time
}

// Rejected is a definitive provider answer that the request was not
// accepted (not logged in, invalid number, bad token …): nothing was sent.
type Rejected struct {
	Status  int
	Message string
}

func (e *Rejected) Error() string {
	return fmt.Sprintf("whatsapp provider rejected the request (%d): %s", e.Status, e.Message)
}

// Receipt states.
const (
	ReceiptDelivered = "delivered"
	ReceiptRead      = "read"
)

// Event types (webhook).
const (
	EventReceipt   = "receipt"
	EventConnected = "connected"
	EventLoggedOut = "logged_out"
	EventPaired    = "paired"
	// EventDisconnected: the gateway lost its socket (it reconnects by
	// itself; the session stays linked).
	EventDisconnected = "disconnected"
	EventOther        = "other"
)

// Event is one parsed webhook delivery.
type Event struct {
	Type       string
	Receipt    string
	MessageIDs []string
	At         time.Time
}

// Provider runs organization sessions.
type Provider interface {
	integrations.Adapter
	// Configured reports whether the provider can be used (server set up).
	Configured() bool
	// CreateSession creates the provider session (wuzapi user).
	CreateSession(ctx context.Context, in SessionInput) (Session, error)
	// DeleteSession removes the provider session.
	DeleteSession(ctx context.Context, account integrations.Account) error
	// Connect opens the session's socket (a QR code follows when no phone
	// is linked).
	Connect(ctx context.Context, account integrations.Account) error
	// QR returns the current pairing QR code as a data URL (PNG).
	QR(ctx context.Context, account integrations.Account) (string, error)
	// Status reads the session state.
	Status(ctx context.Context, account integrations.Account) (Status, error)
	// Logout unlinks the phone (the session stays, a new QR can link one).
	Logout(ctx context.Context, account integrations.Account) error
	// OnWhatsApp reports whether a number has a WhatsApp account.
	OnWhatsApp(ctx context.Context, account integrations.Account, phone string) (bool, error)
	// SendText sends a text message. A *Rejected error is a definitive
	// "not sent"; any other error means the outcome is unknown.
	SendText(ctx context.Context, account integrations.Account, msg TextMessage) (SendResult, error)
	// SendDocument sends a document.
	SendDocument(ctx context.Context, account integrations.Account, msg DocumentMessage) (SendResult, error)
	// ParseWebhook parses one webhook delivery.
	ParseWebhook(contentType string, body []byte) ([]Event, error)
}

// Digits returns the provider form of an E.164 number ("905321234567").
func Digits(e164 string) string { return strings.TrimPrefix(strings.TrimSpace(e164), "+") }

// ParseWuzapiWebhook parses a wuzapi webhook delivery: JSON
// ({"type": "ReadReceipt", "state": "Read", "event": {...}}) or, with
// WEBHOOK_FORMAT=form, a form whose jsonData field holds that JSON.
func ParseWuzapiWebhook(contentType string, body []byte) ([]Event, error) {
	raw := body
	mt, _, _ := mime.ParseMediaType(contentType)
	if mt == "application/x-www-form-urlencoded" {
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, fmt.Errorf("whatsapp: webhook form: %w", err)
		}
		raw = []byte(form.Get("jsonData"))
	}
	var p struct {
		Type  string `json:"type"`
		State string `json:"state"`
		Event struct {
			MessageIDs []string `json:"MessageIDs"`
			// Some versions use lower-case keys.
			MessageIDsLower []string `json:"messageIds"`
			Type            string   `json:"Type"`
			Timestamp       string   `json:"Timestamp"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("whatsapp: webhook body: %w", err)
	}
	ev := Event{Type: EventOther}
	if t, err := time.Parse(time.RFC3339Nano, p.Event.Timestamp); err == nil {
		ev.At = t.UTC()
	}
	switch strings.ToLower(p.Type) {
	case "readreceipt", "receipt":
		ids := p.Event.MessageIDs
		if len(ids) == 0 {
			ids = p.Event.MessageIDsLower
		}
		state := strings.ToLower(p.State)
		if state == "" {
			state = strings.ToLower(p.Event.Type)
			if state == "" {
				state = "delivered"
			}
		}
		switch state {
		case "read", "read-self", "played":
			ev.Receipt = ReceiptRead
		case "delivered":
			ev.Receipt = ReceiptDelivered
		default:
			return []Event{{Type: EventOther}}, nil
		}
		ev.Type, ev.MessageIDs = EventReceipt, ids
	case "connected":
		ev.Type = EventConnected
	case "pairsuccess":
		ev.Type = EventPaired
	case "loggedout":
		ev.Type = EventLoggedOut
	case "disconnected", "streamreplaced", "keepalivetimeout":
		ev.Type = EventDisconnected
	}
	return []Event{ev}, nil
}
