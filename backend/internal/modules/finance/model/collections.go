package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// ResourceCollections is the engine / live list slug of collections.
const ResourceCollections = "tenant.finance_collections"

// Collection methods.
const (
	MethodCash         = "cash"
	MethodBankTransfer = "bank_transfer"
	MethodCardPOS      = "card_pos"
	MethodOnline       = "online"
	MethodOther        = "other"
)

// CollectionMethods in display order; BranchMethods are recorded in the
// panel (online collections come from the payments module).
var (
	CollectionMethods = []string{MethodCash, MethodBankTransfer, MethodCardPOS, MethodOnline, MethodOther}
	BranchMethods     = []string{MethodCash, MethodBankTransfer, MethodCardPOS, MethodOther}
)

// Providers of a collection.
const (
	ProviderManual = "manual"
	ProviderIyzico = "iyzico"
)

// Collection statuses.
const (
	CollectionRecorded          = "recorded"
	CollectionPartiallyRefunded = "partially_refunded"
	CollectionRefunded          = "refunded"
	CollectionVoid              = "void"
)

// CollectionStatuses in display order.
var CollectionStatuses = []string{CollectionRecorded, CollectionPartiallyRefunded, CollectionRefunded, CollectionVoid}

// Allocation states of a collection (computed).
const (
	AllocUnallocated        = "unallocated"
	AllocPartiallyAllocated = "partially_allocated"
	AllocAllocated          = "allocated"
	AllocNotAllocatable     = "not_allocatable"
)

// AllocationStatuses in display order.
var AllocationStatuses = []string{AllocUnallocated, AllocPartiallyAllocated, AllocAllocated, AllocNotAllocatable}

// Open item statuses (computed per accrual).
const (
	ItemOpen          = "open"
	ItemPartiallyPaid = "partially_paid"
	ItemPaid          = "paid"
)

// Settlement states of an online collection.
const (
	SettlementNone    = "none"
	SettlementPending = "pending"
	SettlementSettled = "settled"
)

// Allocation modes of a create / allocate request.
const (
	AllocateAuto   = "auto"
	AllocateManual = "manual"
	AllocateNone   = "none"
)

// Allocation row kinds and sources.
const (
	AllocKindAllocate = "allocate"
	AllocKindRelease  = "release"

	AllocSourceManual   = "manual"
	AllocSourceAuto     = "auto"
	AllocSourceRefund   = "refund"
	AllocSourceVoid     = "void"
	AllocSourceReversed = "accrual_reversed"
	AllocSourceReleased = "released"
)

// CommissionCategory is the expense category of provider commissions.
const CommissionCategory = "payment_commission"

// Collection is one receipt of money from a party with its allocation
// state.
type Collection struct {
	UUID             uuid.UUID   `json:"uuid"`
	Code             string      `json:"code"`
	Account          Ref         `json:"account"`
	Method           string      `json:"method"`
	Provider         string      `json:"provider"`
	Purpose          string      `json:"purpose"`
	Status           string      `json:"status"`
	AllocationStatus string      `json:"allocation_status"`
	Currency         string      `json:"currency"`
	Amount           money.Money `json:"amount"`
	GrossAmount      money.Money `json:"gross_amount"`
	CommissionAmount money.Money `json:"commission_amount"`
	NetAmount        money.Money `json:"net_amount"`
	RefundedAmount   money.Money `json:"refunded_amount"`
	AllocatedAmount  money.Money `json:"allocated_amount"`
	// UnallocatedAmount is the party's credit from this collection.
	UnallocatedAmount money.Money `json:"unallocated_amount"`
	// Refundable is what may still be paid back.
	Refundable        money.Money `json:"refundable"`
	CollectionDate    Date        `json:"collection_date"`
	ReferenceNo       string      `json:"reference_no"`
	PayerName         string      `json:"payer_name"`
	Note              string      `json:"note"`
	Branch            *Ref        `json:"branch"`
	CashAccount       *Ref        `json:"cash_account"`
	CashAccountKind   string      `json:"cash_account_kind,omitempty"`
	Rental            *Ref        `json:"rental"`
	Reservation       *Ref        `json:"reservation"`
	PaymentUUID       *uuid.UUID  `json:"payment_uuid"`
	EntryUUID         uuid.UUID   `json:"entry_uuid"`
	MovementUUID      *uuid.UUID  `json:"movement_uuid"`
	CommissionExpense *uuid.UUID  `json:"commission_expense_uuid"`
	SettlementStatus  string      `json:"settlement_status"`
	SettledAt         *time.Time  `json:"settled_at"`
	VoidReason        string      `json:"void_reason"`
	VoidedAt          *time.Time  `json:"voided_at"`
	Voidable          bool        `json:"voidable"`
	CreatedBy         *Actor      `json:"created_by"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	// Detail only.
	Allocations []Allocation       `json:"allocations,omitempty"`
	Refunds     []CollectionRefund `json:"refunds,omitempty"`
}

// Allocation is the net amount of a collection applied to one open item.
type Allocation struct {
	EntryUUID   uuid.UUID   `json:"entry_uuid"`
	Purpose     string      `json:"purpose"`
	Description string      `json:"description"`
	EntryDate   Date        `json:"entry_date"`
	DueDate     Date        `json:"due_date"`
	ItemAmount  money.Money `json:"item_amount"`
	Amount      money.Money `json:"amount"`
	// ItemOpen / ItemStatus describe the open item over every collection.
	ItemOpen    money.Money `json:"item_open"`
	ItemStatus  string      `json:"item_status"`
	Reversed    bool        `json:"reversed"`
	Rental      *Ref        `json:"rental"`
	ChargeKind  string      `json:"charge_kind,omitempty"`
	AllocatedAt time.Time   `json:"allocated_at"`
}

// CollectionRefund is a refund of a collection: a provider refund (pending
// / succeeded / failed) or a manual one.
type CollectionRefund struct {
	UUID             uuid.UUID   `json:"uuid"`
	Amount           money.Money `json:"amount"`
	Currency         string      `json:"currency"`
	Status           string      `json:"status"`
	Reason           string      `json:"reason"`
	FailureReason    string      `json:"failure_reason"`
	ProviderRefundID *string     `json:"provider_refund_id"`
	EntryUUID        *uuid.UUID  `json:"entry_uuid"`
	MovementUUID     *uuid.UUID  `json:"movement_uuid"`
	CashAccount      *Ref        `json:"cash_account"`
	CreatedBy        *Actor      `json:"created_by"`
	CreatedAt        time.Time   `json:"created_at"`
}

// OpenItem is a live accrual of an account with its allocation state.
type OpenItem struct {
	EntryUUID   uuid.UUID   `json:"entry_uuid"`
	Purpose     string      `json:"purpose"`
	Description string      `json:"description"`
	EntryDate   Date        `json:"entry_date"`
	DueDate     Date        `json:"due_date"`
	Amount      money.Money `json:"amount"`
	Allocated   money.Money `json:"allocated"`
	Open        money.Money `json:"open"`
	Status      string      `json:"status"`
	Overdue     bool        `json:"overdue"`
	Currency    string      `json:"currency"`
	Rental      *Ref        `json:"rental"`
	ChargeKind  string      `json:"charge_kind,omitempty"`
}

// OpenItems is the answer of the open items endpoint.
type OpenItems struct {
	Account Ref         `json:"account"`
	Items   []OpenItem  `json:"items"`
	Open    money.Money `json:"open"`
	// Credit is the unallocated money of the account's collections.
	Credit   money.Money `json:"credit"`
	Currency string      `json:"currency"`
}

// CollectionFilter filters the collections list.
type CollectionFilter struct {
	Limit, Offset      int32
	Q                  string
	SortField          string
	SortDesc           bool
	Methods            []string
	Statuses           []string
	AllocationStatuses []string
	BranchUUIDs        []uuid.UUID
	CashAccountUUIDs   []uuid.UUID
	AccountUUID        *uuid.UUID
	RentalUUID         *uuid.UUID
	From, To           *time.Time
	IDs                []int64
}

// AllocationItem names an open item and the amount to apply.
type AllocationItem struct {
	EntryUUID uuid.UUID    `json:"entry_uuid"`
	Amount    *money.Money `json:"amount"`
}

// CollectionCreateInput records an in-branch collection.
type CollectionCreateInput struct {
	AccountUUID     *uuid.UUID   `json:"account_uuid"`
	RentalUUID      *uuid.UUID   `json:"rental_uuid"`
	Amount          *money.Money `json:"amount"`
	Method          string       `json:"method"`
	Purpose         string       `json:"purpose"`
	CollectionDate  *string      `json:"collection_date"`
	BranchUUID      *uuid.UUID   `json:"branch_uuid"`
	CashAccountUUID *uuid.UUID   `json:"cash_account_uuid"`
	ReferenceNo     string       `json:"reference_no"`
	PayerName       string       `json:"payer_name"`
	Note            string       `json:"note"`
	// AllocationMode: auto (oldest first, default), manual (Allocations)
	// or none (keep it all as credit).
	AllocationMode string           `json:"allocation_mode"`
	Allocations    []AllocationItem `json:"allocations"`
	IdempotencyKey *uuid.UUID       `json:"idempotency_key"`
}

// AllocateInput applies a collection's credit to open items.
type AllocateInput struct {
	Mode           string           `json:"mode"`
	Items          []AllocationItem `json:"items"`
	RentalUUID     *uuid.UUID       `json:"rental_uuid"`
	IdempotencyKey *uuid.UUID       `json:"idempotency_key"`
}

// ReleaseInput takes a collection's allocation off one open item.
type ReleaseInput struct {
	EntryUUID *uuid.UUID `json:"entry_uuid"`
	Reason    string     `json:"reason"`
}

// CollectionRefundInput pays (part of) a collection back.
type CollectionRefundInput struct {
	Amount          *money.Money `json:"amount"`
	Reason          string       `json:"reason"`
	CashAccountUUID *uuid.UUID   `json:"cash_account_uuid"`
	RefundDate      *string      `json:"refund_date"`
}

// CollectionVoidInput cancels a collection recorded by mistake.
type CollectionVoidInput struct {
	Reason string `json:"reason"`
}
