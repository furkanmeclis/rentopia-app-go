// Package model holds the payment types of the API (TEC-707,
// docs/design/payments.md §3).
package model

import (
	"encoding/json"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Live list resources.
const (
	ResourcePayments = "tenant.payments"
	ResourceLinks    = "tenant.payment_links"
)

// Payment statuses.
const (
	StatusPending           = "pending"
	StatusSucceeded         = "succeeded"
	StatusFailed            = "failed"
	StatusCancelled         = "cancelled"
	StatusPartiallyRefunded = "partially_refunded"
	StatusRefunded          = "refunded"
)

// Statuses lists the payment statuses in display order.
var Statuses = []string{StatusPending, StatusSucceeded, StatusPartiallyRefunded, StatusRefunded, StatusFailed, StatusCancelled}

// Providers and methods.
const (
	ProviderManual = "manual"

	MethodCheckoutForm = "checkout_form"
	MethodCash         = "cash"
	MethodCardPOS      = "card_pos"
	MethodBankTransfer = "bank_transfer"
)

// ManualMethods are the branch collection methods.
var ManualMethods = []string{MethodCash, MethodCardPOS, MethodBankTransfer}

// Link statuses and purposes.
const (
	LinkActive    = "active"
	LinkPaid      = "paid"
	LinkExpired   = "expired"
	LinkCancelled = "cancelled"

	PurposePrepayment = "prepayment"
	PurposeBalance    = "balance"
	PurposeCustom     = "custom"
)

// LinkStatuses lists the link statuses in display order.
var LinkStatuses = []string{LinkActive, LinkPaid, LinkExpired, LinkCancelled}

// Actor is a user reference.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ReservationRef names the reservation of a payment / link.
type ReservationRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
}

// Payment is a payment as the API shows it (gross / commission / net /
// refunded separately; masked provider facts only).
type Payment struct {
	UUID              uuid.UUID       `json:"uuid"`
	Reservation       ReservationRef  `json:"reservation"`
	LinkUUID          *uuid.UUID      `json:"link_uuid"`
	Provider          string          `json:"provider"`
	Method            string          `json:"method"`
	Status            string          `json:"status"`
	Currency          string          `json:"currency"`
	Amount            money.Money     `json:"amount"`
	GrossAmount       money.Money     `json:"gross_amount"`
	CommissionAmount  money.Money     `json:"commission_amount"`
	NetAmount         money.Money     `json:"net_amount"`
	RefundedAmount    money.Money     `json:"refunded_amount"`
	Refundable        money.Money     `json:"refundable"`
	ProviderPaymentID *string         `json:"provider_payment_id"`
	ProviderStatus    string          `json:"provider_status,omitempty"`
	ProviderData      json.RawMessage `json:"provider_data,omitempty"`
	FailureReason     string          `json:"failure_reason"`
	VerifiedVia       string          `json:"verified_via"`
	VerifiedAt        *time.Time      `json:"verified_at"`
	Outcome           string          `json:"outcome"`
	Note              string          `json:"note"`
	Refunds           []Refund        `json:"refunds,omitempty"`
	CreatedBy         *Actor          `json:"created_by"`
	CreatedAt         time.Time       `json:"created_at"`
}

// Refund is one refund of a payment.
type Refund struct {
	UUID             uuid.UUID   `json:"uuid"`
	Amount           money.Money `json:"amount"`
	Currency         string      `json:"currency"`
	Status           string      `json:"status"`
	ProviderRefundID *string     `json:"provider_refund_id"`
	Reason           string      `json:"reason"`
	FailureReason    string      `json:"failure_reason"`
	CreatedBy        *Actor      `json:"created_by"`
	CreatedAt        time.Time   `json:"created_at"`
}

// Link is a pay-by-link link as the panel shows it (with its URL).
type Link struct {
	UUID        uuid.UUID      `json:"uuid"`
	URL         string         `json:"url"`
	Reservation ReservationRef `json:"reservation"`
	Purpose     string         `json:"purpose"`
	Amount      money.Money    `json:"amount"`
	Currency    string         `json:"currency"`
	Status      string         `json:"status"`
	ExpiresAt   time.Time      `json:"expires_at"`
	PaidAt      *time.Time     `json:"paid_at"`
	CancelledAt *time.Time     `json:"cancelled_at"`
	CreatedBy   *Actor         `json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
}

// CreateLinkInput is POST /v1/tenant/reservations/{uuid}/payment-links.
// Amount defaults to the prepayment due (awaiting_payment) or the
// remaining balance; ExpiresInMinutes to the payment window (capped by it
// while the reservation waits for payment) or 24 hours.
type CreateLinkInput struct {
	Amount           *money.Money `json:"amount"`
	ExpiresInMinutes *int         `json:"expires_in_minutes"`
}

// CollectInput is POST /v1/tenant/reservations/{uuid}/collections: money
// taken at the branch (cash, card POS, bank transfer).
type CollectInput struct {
	Amount *money.Money `json:"amount"`
	Method string       `json:"method"`
	Note   string       `json:"note"`
}

// RefundInput is POST /v1/tenant/payments/{uuid}/refund.
type RefundInput struct {
	Amount *money.Money `json:"amount"`
	Reason string       `json:"reason"`
}

// ListFilter selects a page of payments.
type ListFilter struct {
	Q                string
	ReservationUUIDs []uuid.UUID
	Statuses         []string
	Providers        []string
	SortField        string
	SortDesc         bool
	Limit, Offset    int32
}

// LinkFilter selects a page of links.
type LinkFilter struct {
	ReservationUUIDs []uuid.UUID
	Statuses         []string
	Limit, Offset    int32
}

// Public pay page states.
const (
	PublicReady      = "ready"
	PublicProcessing = "processing"
	PublicPaid       = "paid"
	PublicFailed     = "failed"
	PublicExpired    = "expired"
	PublicCancelled  = "cancelled"
)

// PublicLink is what GET /v1/public/pay/{token} answers: no customer data,
// only the organization, the reservation reference, the dates, the amount
// and where the payment stands (from verified records only).
type PublicLink struct {
	OrganizationName string      `json:"organization_name"`
	Reference        string      `json:"reference"`
	ProductTitle     string      `json:"product_title"`
	StartsAt         time.Time   `json:"starts_at"`
	EndsAt           time.Time   `json:"ends_at"`
	Amount           money.Money `json:"amount"`
	Currency         string      `json:"currency"`
	ExpiresAt        time.Time   `json:"expires_at"`
	State            string      `json:"state"`
	// Result is the reservation outcome of a verified payment: confirmed,
	// reinstated, late_payment_task, recorded ("" until verified).
	Result        string `json:"result"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// CheckoutStart is the answer of POST /v1/public/pay/{token}/checkout.
type CheckoutStart struct {
	PaymentPageURL string `json:"payment_page_url"`
}

// WebhookResult is the answer to a provider notification.
type WebhookResult struct {
	Duplicate bool   `json:"duplicate"`
	Status    string `json:"status"`
}
