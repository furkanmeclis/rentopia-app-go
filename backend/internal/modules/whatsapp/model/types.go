// Package model holds the WhatsApp channel's API types (TEC-735).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Resource is the activity resource of the WhatsApp session.
const Resource = "tenant.whatsapp"

// Session statuses. StatusNotConnected: no active WhatsApp account (or no
// provider session yet).
const (
	StatusNotConnected = "not_connected"
	StatusCreated      = "created"
	StatusPairing      = "pairing"
	StatusConnected    = "connected"
	StatusDisconnected = "disconnected"
	StatusLoggedOut    = "logged_out"
)

// Message statuses.
const (
	MessageQueued    = "queued"
	MessageSent      = "sent"
	MessageDelivered = "delivered"
	MessageRead      = "read"
	MessageFailed    = "failed"
)

// MessageStatuses lists the message statuses (filters).
var MessageStatuses = []string{MessageQueued, MessageSent, MessageDelivered, MessageRead, MessageFailed}

// Message kinds.
const (
	KindText     = "text"
	KindDocument = "document"
)

// Rate limit bounds (messages per minute per session).
const (
	MinRateLimit     = 1
	MaxRateLimit     = 60
	DefaultRateLimit = 20
)

// ProviderInfo is a selectable provider.
type ProviderInfo struct {
	Code       string `json:"code"`
	Configured bool   `json:"configured"`
}

// Session is GET /v1/tenant/whatsapp/session.
type Session struct {
	Status             string         `json:"status"`
	Provider           string         `json:"provider"`
	AccountUUID        *uuid.UUID     `json:"account_uuid"`
	PhoneMasked        string         `json:"phone_masked"`
	ConnectedAt        *time.Time     `json:"connected_at"`
	DisconnectedAt     *time.Time     `json:"disconnected_at"`
	LastStatusAt       *time.Time     `json:"last_status_at"`
	LastError          string         `json:"last_error"`
	RateLimitPerMinute int32          `json:"rate_limit_per_minute"`
	RiskAcknowledgedAt *time.Time     `json:"risk_acknowledged_at"`
	Providers          []ProviderInfo `json:"providers"`
}

// QR is GET /v1/tenant/whatsapp/session/qr: the pairing code while the
// session waits for a phone, empty once linked.
type QR struct {
	Status string `json:"status"`
	QRCode string `json:"qr_code"`
}

// ConnectInput is POST /v1/tenant/whatsapp/session/connect.
type ConnectInput struct {
	// Provider picks the provider when no WhatsApp account exists yet
	// (empty: the first configured one).
	Provider string `json:"provider"`
	// AcknowledgeRisk must be true: the number risk notice was read.
	AcknowledgeRisk bool `json:"acknowledge_risk"`
}

// SettingsInput is PATCH /v1/tenant/whatsapp/session.
type SettingsInput struct {
	RateLimitPerMinute *int32 `json:"rate_limit_per_minute"`
}

// TestInput is POST /v1/tenant/whatsapp/test-messages.
type TestInput struct {
	Phone string `json:"phone"`
}

// Message is one row of the message log.
type Message struct {
	UUID              uuid.UUID  `json:"uuid"`
	Kind              string     `json:"kind"`
	Purpose           string     `json:"purpose"`
	DeliveryUUID      *uuid.UUID `json:"delivery_uuid"`
	CustomerUUID      *uuid.UUID `json:"customer_uuid"`
	CustomerName      string     `json:"customer_name"`
	RecipientMasked   string     `json:"recipient_masked"`
	ProviderMessageID string     `json:"provider_message_id"`
	Status            string     `json:"status"`
	Error             string     `json:"error"`
	Test              bool       `json:"test"`
	SentAt            *time.Time `json:"sent_at"`
	DeliveredAt       *time.Time `json:"delivered_at"`
	ReadAt            *time.Time `json:"read_at"`
	FailedAt          *time.Time `json:"failed_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// MessageFilter selects a page of the message log.
type MessageFilter struct {
	Q                      string
	Statuses, Kinds        []string
	CreatedFrom, CreatedTo *time.Time
	SortField              string
	SortDesc               bool
	Limit, Offset          int32
}
