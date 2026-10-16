// Package channel is the contract between the notification engine
// (TEC-733) and the channel adapters that deliver its messages: SMS, e-mail,
// web push today, WhatsApp next (TEC-735).
//
// A new channel is one type implementing Channel, registered on the engine
// with Engine.Register. The engine never names a provider: the rules list
// channel names (Known), a channel without a registered adapter (or whose
// Send answers OutcomeSkipped) is logged as skipped and the rule's next
// channel runs.
//
// Channels whose result arrives later (a delivery report) answer
// OutcomeAccepted with ExternalUUID set and report the final result through
// the Reporter handed to them by SetReporter (ReportingChannel).
package channel

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Channel names (notification_rules.channels, notification_deliveries.channel).
const (
	SMS      = "sms"
	Email    = "email"
	WhatsApp = "whatsapp"
	Push     = "push"
)

// Known lists every channel name a rule may use, in the default display
// order. A known channel may have no adapter yet (WhatsApp before TEC-735).
var Known = []string{SMS, Email, WhatsApp, Push}

// IsKnown reports whether name is a known channel.
func IsKnown(name string) bool {
	for _, k := range Known {
		if k == name {
			return true
		}
	}
	return false
}

// Audiences.
const (
	AudienceCustomer = "customer"
	AudienceStaff    = "staff"
)

// Recipient is who a message goes to. Contact fields are filled from the
// customer (or member) record at send time; they are never stored on the
// dispatch.
type Recipient struct {
	Audience   string
	CustomerID int64
	UserID     int64
	Name       string
	Phone      string
	Email      string
	Language   string
}

// Message is one rendered message for one channel.
type Message struct {
	// DeliveryUUID identifies the attempt (notification_deliveries.uuid).
	DeliveryUUID uuid.UUID
	Event        string
	Recipient    Recipient
	Subject      string
	Body         string
	// RedactedBody is Body without the bearer parts (links), for channel
	// logs that keep the text (sms_messages). Empty when Body has none.
	RedactedBody string
	// Link is the sign-in link of the message (customer portal), if any.
	Link    string
	RefType string
	RefUUID *uuid.UUID
	Test    bool
}

// Outcome is the result of one send.
type Outcome string

const (
	// OutcomeDelivered: delivered (confirmed by the provider).
	OutcomeDelivered Outcome = "delivered"
	// OutcomeSent: handed to the provider; final, no later report
	// (SMTP, web push).
	OutcomeSent Outcome = "sent"
	// OutcomeAccepted: queued at the channel; the final result arrives
	// through the Reporter (ExternalUUID identifies the message).
	OutcomeAccepted Outcome = "accepted"
	// OutcomeFailed: the send failed; the next channel runs (fallback).
	OutcomeFailed Outcome = "failed"
	// OutcomeSkipped: the channel cannot reach this recipient (no
	// provider account, no number / address / subscription); the next
	// channel runs.
	OutcomeSkipped Outcome = "skipped"
)

// Result is what a channel reports for one send.
type Result struct {
	Outcome         Outcome
	Provider        string
	ProviderRef     string
	ExternalUUID    *uuid.UUID
	RecipientMasked string
	Error           string
}

// Channel delivers rendered messages.
type Channel interface {
	// Name is one of Known.
	Name() string
	// Supports reports whether the channel can serve an audience.
	Supports(audience string) bool
	// Send delivers one message. A returned error counts as OutcomeFailed
	// with the error text; it never stops the dispatch.
	Send(ctx context.Context, orgID int64, msg Message) (Result, error)
}

// Report is a channel's final delivery result for an accepted message.
// The delivery is found by DeliveryUUID (Message.DeliveryUUID, when the
// channel kept it) or else by ExternalUUID (Result.ExternalUUID).
type Report struct {
	Channel      string
	DeliveryUUID *uuid.UUID
	ExternalUUID uuid.UUID
	Delivered    bool
	ProviderRef  string
	Error        string
}

// Reporter receives delivery reports (implemented by the engine).
type Reporter interface {
	ReportDelivery(ctx context.Context, orgID int64, r Report)
}

// ReportingChannel is a channel with delivery reports.
type ReportingChannel interface {
	Channel
	SetReporter(r Reporter)
}

// MaskEmail keeps the first letter of the local part and the domain.
func MaskEmail(v string) string {
	v = strings.TrimSpace(v)
	at := strings.LastIndex(v, "@")
	if at <= 0 {
		return ""
	}
	return v[:1] + "***" + v[at:]
}
