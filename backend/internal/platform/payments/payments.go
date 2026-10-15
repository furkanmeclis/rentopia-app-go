// Package payments is the payment provider layer (TEC-707,
// docs/design/payments.md §3): the Provider interface every payment
// adapter implements (iyzico first) on top of the integration skeleton.
//
// Card data never touches Rentopia: CreateCheckout opens the provider's
// hosted, 3DS-protected checkout (iyzico Checkout Form) and returns its URL;
// the result is read back with Retrieve. A browser return is never proof:
// only Retrieve (directly, or after a webhook whose signature VerifyWebhook
// accepted) confirms a payment.
package payments

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Result statuses (provider statuses are mapped onto these).
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusPending   = "pending"
)

// Buyer is the payer data the provider requires for fraud checks (iyzico
// asks for a buyer, a billing address and an IP). No card data.
type Buyer struct {
	ID             string
	Name           string
	Surname        string
	Email          string
	Phone          string
	IdentityNumber string
	Address        string
	City           string
	Country        string
	IP             string
}

// CheckoutRequest opens a hosted checkout.
type CheckoutRequest struct {
	// ConversationID is our order reference (echoed back by the provider
	// and in webhooks); BasketID names the order on the provider side.
	ConversationID string
	BasketID       string
	Amount         money.Money
	Currency       string
	CallbackURL    string
	Locale         string
	ItemID         string
	ItemName       string
	ItemCategory   string
	Buyer          Buyer
}

// Checkout is an opened checkout session.
type Checkout struct {
	Token          string
	PaymentPageURL string
	ExpiresAt      *time.Time
}

// RetrieveRequest reads a checkout result.
type RetrieveRequest struct {
	Token          string
	ConversationID string
	Locale         string
}

// Result is a verified provider answer about one payment.
type Result struct {
	Status         string
	ProviderStatus string
	PaymentID      string
	Token          string
	ConversationID string
	BasketID       string
	Currency       string
	// Price is the basket amount; PaidPrice what the customer paid
	// (installment interest included); Commission the provider's fees.
	Price         money.Money
	PaidPrice     money.Money
	Commission    money.Money
	FailureReason string
	// Data holds masked, non-sensitive facts (card family / association,
	// last four digits, installment, fraud status).
	Data map[string]any
}

// RefundRequest refunds (part of) a payment.
type RefundRequest struct {
	PaymentID      string
	ConversationID string
	Amount         money.Money
	Currency       string
	IP             string
	Locale         string
}

// RefundResult is the provider's answer to a refund.
type RefundResult struct {
	RefundID string
	Amount   money.Money
}

// Webhook is a parsed (not yet verified) provider notification.
type Webhook struct {
	EventType      string
	PaymentID      string
	Token          string
	ConversationID string
	Status         string
	ReferenceCode  string
	MerchantID     string
}

// Provider is one payment provider adapter.
type Provider interface {
	integrations.Adapter
	// CreateCheckout opens a hosted (3DS) checkout / pay-with-provider page.
	CreateCheckout(ctx context.Context, account integrations.Account, in CheckoutRequest) (Checkout, error)
	// Retrieve reads (and verifies, when the provider signs answers) the
	// result of a checkout.
	Retrieve(ctx context.Context, account integrations.Account, in RetrieveRequest) (Result, error)
	// Refund refunds a payment partially or fully.
	Refund(ctx context.Context, account integrations.Account, in RefundRequest) (RefundResult, error)
	// ParseWebhook reads a notification body (no trust yet: the
	// organization, hence the secret, is found from its conversation id).
	ParseWebhook(header http.Header, body []byte) (Webhook, error)
	// VerifyWebhook checks the notification signature with the account's
	// secret.
	VerifyWebhook(account integrations.Account, hook Webhook, header http.Header, body []byte) error
}

// PreAuthorizer is the optional authorize-then-capture flow (deposits).
// iyzico's pre-auth needs the card on the merchant side (non-hosted), so
// the iyzico adapter does not implement it; a provider that offers a
// hosted pre-auth can.
type PreAuthorizer interface {
	PreAuth(ctx context.Context, account integrations.Account, in CheckoutRequest) (Checkout, error)
	Capture(ctx context.Context, account integrations.Account, paymentID string, amount money.Money) (Result, error)
	Void(ctx context.Context, account integrations.Account, paymentID string) error
}

var (
	// ErrInvalidSignature: a webhook or an answer did not verify.
	ErrInvalidSignature = errors.New("payments: invalid signature")
	// ErrMalformed: the provider body could not be read.
	ErrMalformed = errors.New("payments: malformed provider message")
)

// ProviderError is a provider refusal (error code + message, never card
// data); Temporary marks network / 5xx failures worth a retry.
type ProviderError struct {
	Code      string
	Message   string
	Temporary bool
}

func (e *ProviderError) Error() string {
	if e.Code != "" {
		return "payments: provider error " + e.Code + ": " + e.Message
	}
	return "payments: provider error: " + e.Message
}
