package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Security deposits (TEC-719, docs/design/deposits.md).

// ResourceDeposits is the engine / live list resource of deposits.
const ResourceDeposits = "tenant.security_deposits"

// Deposit methods.
const (
	DepositCash          = "cash"
	DepositBankTransfer  = "bank_transfer"
	DepositPOSPreAuth    = "pos_preauth"
	DepositOnlinePreAuth = "online_preauth"
)

// DepositMethods in display order.
var DepositMethods = []string{DepositCash, DepositPOSPreAuth, DepositOnlinePreAuth, DepositBankTransfer}

// Deposit statuses.
const (
	DepositPending           = "pending"
	DepositHeld              = "held"
	DepositPartiallyCaptured = "partially_captured"
	DepositReleased          = "released"
	DepositRefunded          = "refunded"
	DepositCancelled         = "cancelled"
)

// DepositStatuses in display order.
var DepositStatuses = []string{DepositPending, DepositHeld, DepositPartiallyCaptured, DepositReleased, DepositRefunded, DepositCancelled}

// Ledger kinds of deposit postings (purpose deposit; never in the
// receivable balance). A capture pairs deposit_capture with a collection.
const (
	KindDepositHold    = "deposit_hold"
	KindDepositReturn  = "deposit_return"
	KindDepositCapture = "deposit_capture"
)

// SourceSecurityDeposit is the ledger source of deposit postings.
const SourceSecurityDeposit = "security_deposit"

// Cash movement kinds of deposits (apart from collections and refunds).
const (
	MoveDepositIn  = "deposit_in"
	MoveDepositOut = "deposit_out"
)

// What a deduction's captured money settles.
const (
	AppliesCharge  = "charge"
	AppliesRent    = "rent"
	AppliesGeneral = "general"
)

// DefaultHeldOverDays is the "held over N days" default of the list.
const DefaultHeldOverDays = 7

// Deposit is one security deposit / pre-authorization.
type Deposit struct {
	UUID         uuid.UUID   `json:"uuid"`
	Rental       Ref         `json:"rental"`
	RentalStatus string      `json:"rental_status"`
	Reservation  Ref         `json:"reservation"`
	Customer     Ref         `json:"customer"`
	Account      *Ref        `json:"account"`
	Branch       Ref         `json:"branch"`
	Amount       money.Money `json:"amount"`
	Currency     string      `json:"currency"`
	Method       string      `json:"method"`
	Status       string      `json:"status"`
	CashAccount  *Ref        `json:"cash_account"`
	// CashAccountKind is cash or bank.
	CashAccountKind   string      `json:"cash_account_kind,omitempty"`
	POSReference      string      `json:"pos_reference"`
	Provider          string      `json:"provider"`
	ProviderPaymentID string      `json:"provider_payment_id"`
	PaymentPageURL    string      `json:"payment_page_url"`
	AuthExpiresAt     *time.Time  `json:"authorization_expires_at"`
	CapturedAmount    money.Money `json:"captured_amount"`
	ReturnedAmount    money.Money `json:"returned_amount"`
	// OpenAmount is what is still held (amount − captured − returned of an
	// open deposit; zero once closed).
	OpenAmount     money.Money `json:"open_amount"`
	Note           string      `json:"note"`
	HeldAt         *time.Time  `json:"held_at"`
	HeldDays       int         `json:"held_days"`
	ResolvedAt     *time.Time  `json:"resolved_at"`
	ResolutionNote string      `json:"resolution_note"`
	CancelReason   string      `json:"cancel_reason"`
	// CustomerNoticeStatus: none, pending (recorded, waiting for the
	// notification templates), sent.
	CustomerNoticeStatus string         `json:"customer_notice_status"`
	CustomerNotice       map[string]any `json:"customer_notice"`
	DeductionCount       int64          `json:"deduction_count"`
	Deductions           []Deduction    `json:"deductions"`
	// Capabilities say what the caller may do now (UI buttons).
	Capabilities DepositCapabilities `json:"capabilities"`
	CreatedBy    *Actor              `json:"created_by"`
	ResolvedBy   *Actor              `json:"resolved_by"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// DepositCapabilities are the actions open on a deposit for the caller.
type DepositCapabilities struct {
	Verify  bool `json:"verify"`
	Release bool `json:"release"`
	Deduct  bool `json:"deduct"`
	Cancel  bool `json:"cancel"`
	// OnlineCapture: deductions are captured through the provider (one
	// post-auth); otherwise staff capture them by hand (POS slip, till).
	OnlineCapture bool `json:"online_capture"`
}

// Deduction is one line captured from a deposit.
type Deduction struct {
	UUID      uuid.UUID   `json:"uuid"`
	Amount    money.Money `json:"amount"`
	AppliesTo string      `json:"applies_to"`
	Reason    string      `json:"reason"`
	Charge    *Ref        `json:"charge"`
	Damage    *Ref        `json:"damage"`
	Document  *Attachment `json:"document"`
	CreatedBy *Actor      `json:"created_by"`
	CreatedAt time.Time   `json:"created_at"`
}

// DepositFilter filters the deposits list.
type DepositFilter struct {
	Limit, Offset int32
	Q             string
	SortField     string
	SortDesc      bool
	Statuses      []string
	Methods       []string
	BranchUUIDs   []uuid.UUID
	RentalUUID    *uuid.UUID
	From, To      *time.Time
	// HeldOverDays: only deposits held for more than N days.
	HeldOverDays *int
	IDs          []int64
}

// DepositInput takes a deposit for a rental.
type DepositInput struct {
	RentalUUID *uuid.UUID `json:"rental_uuid"`
	// Amount defaults to the product's deposit amount.
	Amount          *money.Money `json:"amount"`
	Method          string       `json:"method"`
	CashAccountUUID *uuid.UUID   `json:"cash_account_uuid"`
	POSReference    string       `json:"pos_reference"`
	// AuthorizationExpiresAt: the pre-auth hold end (POS slip), optional.
	AuthorizationExpiresAt *time.Time `json:"authorization_expires_at"`
	Note                   string     `json:"note"`
	// ClientIP / Locale are filled by the handler (online pre-auth).
	ClientIP string `json:"-"`
	Locale   string `json:"-"`
}

// DeductionInput is one line of a resolution.
type DeductionInput struct {
	Amount            *money.Money `json:"amount"`
	AppliesTo         string       `json:"applies_to"`
	Reason            string       `json:"reason"`
	RentalChargeUUID  *uuid.UUID   `json:"rental_charge_uuid"`
	InspectionDmgUUID *uuid.UUID   `json:"inspection_damage_uuid"`
}

// ResolveInput resolves a held deposit: the deductions are captured and
// the rest is released (pre-auth) or paid back (cash / transfer).
type ResolveInput struct {
	Deductions []DeductionInput `json:"deductions"`
	// RefundCashAccountUUID: where the remainder of a cash / transfer
	// deposit is paid from (default: the account that holds it).
	RefundCashAccountUUID *uuid.UUID `json:"refund_cash_account_uuid"`
	// CaptureCashAccountUUID: the bank account a POS capture settles to
	// (optional; the POS settlement may also be booked later).
	CaptureCashAccountUUID *uuid.UUID `json:"capture_cash_account_uuid"`
	Note                   string     `json:"note"`
	// IdempotencyKey: a replay with the same key answers the resolved
	// deposit without posting anything again.
	IdempotencyKey string `json:"idempotency_key"`
	ClientIP       string `json:"-"`
}

// CancelDepositInput cancels a deposit that was never held.
type CancelDepositInput struct {
	Reason string `json:"reason"`
}

// DepositTotals are the open deposits of the caller's scope.
type DepositTotals struct {
	Currency     string                 `json:"currency"`
	HeldCount    int64                  `json:"held_count"`
	HeldAmount   money.Money            `json:"held_amount"`
	PendingCount int64                  `json:"pending_count"`
	HeldOver     int64                  `json:"held_over"`
	HeldOverDays int                    `json:"held_over_days"`
	ByMethod     map[string]money.Money `json:"by_method"`
}
