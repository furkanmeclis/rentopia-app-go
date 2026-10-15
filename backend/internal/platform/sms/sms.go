// Package sms is the SMS provider layer (TEC-734, docs/design/sms.md).
// Providers (Netgsm, the development log provider) implement Provider; the
// tenant SMS module sends through the integration job runner, so every
// provider call is an integration_attempts row and an unknown outcome is
// checked (delivery report) before anything is sent again.
//
// Provider calls take the organization's integration account (sealed
// credentials, see integrations.Vault); numbers are E.164 ("+905321234567")
// and never logged in full.
package sms

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
)

// Send classes.
const (
	// ClassOTP: one-time codes. Own queue (critical), never subject to İYS
	// marketing consent, body redacted in the log.
	ClassOTP = "otp"
	// ClassTransactional: notices the customer needs for a contract they
	// entered (reservation / contract changes). No İYS consent needed.
	ClassTransactional = "transactional"
	// ClassInformational: optional notices; sent only with the customer's
	// open İYS SMS consent.
	ClassInformational = "informational"
)

// ValidClass reports whether c is a send class.
func ValidClass(c string) bool {
	return c == ClassOTP || c == ClassTransactional || c == ClassInformational
}

// Message is one SMS to one number.
type Message struct {
	// To is the normalized number ("+905321234567").
	To   string
	Body string
	// Class picks the provider endpoint where it matters (Netgsm OTP).
	Class string
}

// SendResult is an accepted message.
type SendResult struct {
	// MessageID is the provider's reference (Netgsm jobid / bulkid).
	MessageID string
	Code      string
}

// DeliveryState is the delivery report state of a message.
type DeliveryState string

// Delivery states.
const (
	DeliveryPending     DeliveryState = "pending"
	DeliveryDelivered   DeliveryState = "delivered"
	DeliveryUndelivered DeliveryState = "undelivered"
	// DeliveryNotFound: the provider has no message with this reference.
	DeliveryNotFound DeliveryState = "not_found"
)

// Delivery is a delivery report.
type Delivery struct {
	State DeliveryState
	// Code is the provider's raw status / error code.
	Code string
	At   *time.Time
}

// Rejected is a definitive provider answer that the message was not
// accepted (bad credentials, unknown header, invalid number …). Nothing was
// queued at the provider.
type Rejected struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *Rejected) Error() string {
	return fmt.Sprintf("sms provider rejected the request (code %s): %s", e.Code, e.Message)
}

// Provider sends SMS for an organization's account.
type Provider interface {
	integrations.Adapter
	// Send hands one message to the provider. A *Rejected error is a
	// definitive "not sent"; any other error means the outcome is unknown.
	Send(ctx context.Context, account integrations.Account, msg Message) (SendResult, error)
	// Status reads the delivery report of a sent message.
	// integrations.ErrUnsupported when the provider has no lookup.
	Status(ctx context.Context, account integrations.Account, messageID string) (Delivery, error)
}

// ErrNoSandbox: the provider has no sandbox endpoint configured.
var ErrNoSandbox = errors.New("sms: this provider has no sandbox endpoint configured; use live mode or set the sandbox base URL")
