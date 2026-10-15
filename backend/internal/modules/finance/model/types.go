// Package model holds the finance types (TEC-717 ledger, TEC-720 cash and
// bank accounts, expenses): docs/design/ledger.md and
// docs/design/cash-accounts.md.
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine / live list resources.
const (
	ResourceAccounts     = "tenant.finance_accounts"
	ResourceEntries      = "tenant.ledger_entries"
	ResourceCashAccounts = "tenant.cash_accounts"
	ResourceMovements    = "tenant.cash_movements"
	ResourceExpenses     = "tenant.expenses"
	ResourceCategories   = "tenant.expense_categories"
)

// Party kinds of an account.
const (
	PartyCustomer = "customer"
	PartySupplier = "supplier"
)

// Account record statuses.
const (
	AccountActive   = "active"
	AccountArchived = "archived"
)

// Balance statuses (computed).
const (
	BalanceReceivable = "receivable"
	BalancePayable    = "payable"
	BalanceDueSoon    = "due_soon"
	BalanceOverdue    = "overdue"
	BalanceClosed     = "closed"
)

// BalanceStatuses in display order.
var BalanceStatuses = []string{BalanceOverdue, BalanceDueSoon, BalanceReceivable, BalancePayable, BalanceClosed}

// Entry kinds.
const (
	KindAccrual    = "accrual"
	KindCollection = "collection"
	KindPayment    = "payment"
	KindRefund     = "refund"
	KindOffset     = "offset"
	KindAdjustment = "adjustment"
	KindReversal   = "reversal"
)

// EntryKinds in display order.
var EntryKinds = []string{KindAccrual, KindCollection, KindPayment, KindRefund, KindOffset, KindAdjustment, KindReversal}

// Directions.
const (
	Debit  = "debit"
	Credit = "credit"
)

// Purposes.
const (
	PurposeRent       = "rent"
	PurposeCharge     = "charge"
	PurposePrepayment = "prepayment"
	PurposeDeposit    = "deposit"
	PurposeGeneral    = "general"
)

// Purposes in display order.
var Purposes = []string{PurposeRent, PurposeCharge, PurposePrepayment, PurposeDeposit, PurposeGeneral}

// Source types.
const (
	SourceRental        = "rental"
	SourceRentalCharge  = "rental_charge"
	SourceReservation   = "reservation"
	SourcePayment       = "payment"
	SourcePaymentRefund = "payment_refund"
	SourceInvoice       = "invoice"
	SourceTollCharge    = "toll_charge"
	SourceCashMovement  = "cash_movement"
	SourceManual        = "manual"
)

// System reasons of automatic reversals (shown translated in the panel).
const (
	ReasonAmountChanged   = "system:amount_changed"
	ReasonNoLongerDue     = "system:no_longer_due"
	ReasonAccountChanged  = "system:account_changed"
	ReasonRentalCancelled = "system:rental_cancelled"
)

// Document link types.
const (
	DocumentInvoice    = "invoice"
	DocumentStatement  = "statement"
	DocumentTollCharge = "toll_charge"
)

// DueSoonDays is the "due soon" window.
const DueSoonDays = 7

// Date is a calendar day (YYYY-MM-DD in JSON).
type Date = string

// Actor is a user reference.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Ref is a reference to another record (uuid, code / reference, name).
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code,omitempty"`
	Name string    `json:"name,omitempty"`
}

// Balance is the computed money state of an account (or a scope of one).
type Balance struct {
	Currency    string      `json:"currency"`
	Debit       money.Money `json:"debit"`
	Credit      money.Money `json:"credit"`
	Balance     money.Money `json:"balance"`
	Overdue     money.Money `json:"overdue"`
	DueSoon     money.Money `json:"due_soon"`
	DepositHeld money.Money `json:"deposit_held"`
	Status      string      `json:"status"`
}

// Account is one customer / supplier account with its balance.
type Account struct {
	UUID            uuid.UUID  `json:"uuid"`
	Code            string     `json:"code"`
	PartyKind       string     `json:"party_kind"`
	Name            string     `json:"name"`
	Customer        *Ref       `json:"customer"`
	CustomerType    string     `json:"customer_type,omitempty"`
	Branch          *Ref       `json:"branch"`
	Currency        string     `json:"currency"`
	PaymentTermDays int32      `json:"payment_term_days"`
	Status          string     `json:"status"`
	Note            string     `json:"note"`
	Balance         Balance    `json:"balance"`
	EntryCount      int64      `json:"entry_count"`
	LastEntryAt     *time.Time `json:"last_entry_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// AccountFilter filters the accounts list.
type AccountFilter struct {
	Limit, Offset   int32
	Q               string
	SortField       string
	SortDesc        bool
	BalanceStatuses []string
	Statuses        []string
	PartyKinds      []string
	BranchUUIDs     []uuid.UUID
	MinBalance      *money.Money
	MaxBalance      *money.Money
	// DueBefore: only accounts with open charges due before this day.
	DueBefore *time.Time
	// AccountIDs restricts the list (bulk / export of selected rows).
	AccountIDs []int64
}

// AccountPatch changes an account's terms.
type AccountPatch struct {
	PaymentTermDays *int32  `json:"payment_term_days"`
	Note            *string `json:"note"`
	Status          *string `json:"status"`
}

// AccountCreateInput opens the account of a customer explicitly.
type AccountCreateInput struct {
	CustomerUUID    *uuid.UUID `json:"customer_uuid"`
	PaymentTermDays *int32     `json:"payment_term_days"`
	Note            *string    `json:"note"`
}

// Entry is one ledger entry.
type Entry struct {
	UUID              uuid.UUID   `json:"uuid"`
	Account           Ref         `json:"account"`
	Kind              string      `json:"kind"`
	Direction         string      `json:"direction"`
	Purpose           string      `json:"purpose"`
	Amount            money.Money `json:"amount"`
	Currency          string      `json:"currency"`
	EntryDate         Date        `json:"entry_date"`
	DueDate           Date        `json:"due_date"`
	Description       string      `json:"description"`
	Reason            string      `json:"reason"`
	SourceType        string      `json:"source_type"`
	SourceUUID        *uuid.UUID  `json:"source_uuid"`
	AccrualGeneration *int32      `json:"accrual_generation"`
	OffsetGroup       *uuid.UUID  `json:"offset_group"`
	Rental            *Ref        `json:"rental"`
	Reservation       *Ref        `json:"reservation"`
	Charge            *Ref        `json:"charge"`
	PaymentUUID       *uuid.UUID  `json:"payment_uuid"`
	ReversesUUID      *uuid.UUID  `json:"reverses_uuid"`
	ReversedByUUID    *uuid.UUID  `json:"reversed_by_uuid"`
	AdjustsUUID       *uuid.UUID  `json:"adjusts_uuid"`
	Branch            *Ref        `json:"branch"`
	Links             []Link      `json:"links"`
	RunningBalance    money.Money `json:"running_balance"`
	CreatedBy         *Actor      `json:"created_by"`
	CreatedAt         time.Time   `json:"created_at"`
}

// Link is a document pointing at an entry (an invoice, TEC-722).
type Link struct {
	DocumentType string `json:"document_type"`
	DocumentRef  string `json:"document_ref"`
}

// EntryFilter filters entries (statement / ledger list).
type EntryFilter struct {
	Limit, Offset int32
	Q             string
	SortDesc      bool
	AccountUUID   *uuid.UUID
	RentalUUID    *uuid.UUID
	Kinds         []string
	Purposes      []string
	Directions    []string
	BranchUUIDs   []uuid.UUID
	From, To      *time.Time
	EntryUUID     *uuid.UUID
}

// Statement is an account with a page of its entries.
type Statement struct {
	Account Account     `json:"account"`
	Opening money.Money `json:"opening"`
	Entries []Entry     `json:"items"`
	Total   int64       `json:"total"`
	Limit   int32       `json:"limit"`
	Offset  int32       `json:"offset"`
}

// RentalLedger is the money state of one rental and its entries.
type RentalLedger struct {
	RentalUUID  uuid.UUID   `json:"rental_uuid"`
	Reference   string      `json:"reference"`
	Account     *Ref        `json:"account"`
	Currency    string      `json:"currency"`
	Debit       money.Money `json:"debit"`
	Credit      money.Money `json:"credit"`
	Balance     money.Money `json:"balance"`
	DepositHeld money.Money `json:"deposit_held"`
	Entries     []Entry     `json:"entries"`
}

// CustomerLedger is the account of a customer (nil until the first
// posting) with its latest entries.
type CustomerLedger struct {
	CustomerUUID uuid.UUID `json:"customer_uuid"`
	Account      *Account  `json:"account"`
	Entries      []Entry   `json:"entries"`
}

// RentalBalance is one row of the per-rental breakdown.
type RentalBalance struct {
	RentalUUID      *uuid.UUID  `json:"rental_uuid"`
	Reference       string      `json:"reference"`
	RentalStatus    string      `json:"rental_status"`
	ReservationUUID uuid.UUID   `json:"reservation_uuid"`
	Account         *Ref        `json:"account"`
	Debit           money.Money `json:"debit"`
	Credit          money.Money `json:"credit"`
	Balance         money.Money `json:"balance"`
	DepositHeld     money.Money `json:"deposit_held"`
	LastEntryAt     *time.Time  `json:"last_entry_at"`
}

// BranchBalance is one row of the per-branch breakdown.
type BranchBalance struct {
	Branch  *Ref        `json:"branch"`
	Debit   money.Money `json:"debit"`
	Credit  money.Money `json:"credit"`
	Balance money.Money `json:"balance"`
}

// DueBucket is open receivables by due date.
type DueBucket struct {
	Key    string      `json:"key"`
	Amount money.Money `json:"amount"`
}

// Summary is the organization total with its breakdowns.
type Summary struct {
	Currency       string           `json:"currency"`
	Receivable     money.Money      `json:"receivable"`
	Payable        money.Money      `json:"payable"`
	Net            money.Money      `json:"net"`
	Overdue        money.Money      `json:"overdue"`
	DueSoon        money.Money      `json:"due_soon"`
	DepositHeld    money.Money      `json:"deposit_held"`
	AccountsByStat map[string]int64 `json:"accounts_by_status"`
	ByBranch       []BranchBalance  `json:"by_branch"`
	ByDue          []DueBucket      `json:"by_due"`
}

// CollectionInput records money received on an account (not through the
// payments module): bank transfer of a corporate customer, a deposit.
type CollectionInput struct {
	Amount          *money.Money `json:"amount"`
	Purpose         string       `json:"purpose"`
	Method          string       `json:"method"`
	EntryDate       *string      `json:"entry_date"`
	RentalUUID      *uuid.UUID   `json:"rental_uuid"`
	BranchUUID      *uuid.UUID   `json:"branch_uuid"`
	CashAccountUUID *uuid.UUID   `json:"cash_account_uuid"`
	Description     string       `json:"description"`
}

// RefundInput records money paid back to the party.
type RefundInput struct {
	Amount          *money.Money `json:"amount"`
	Purpose         string       `json:"purpose"`
	EntryDate       *string      `json:"entry_date"`
	RentalUUID      *uuid.UUID   `json:"rental_uuid"`
	BranchUUID      *uuid.UUID   `json:"branch_uuid"`
	CashAccountUUID *uuid.UUID   `json:"cash_account_uuid"`
	Reason          string       `json:"reason"`
}

// AdjustmentInput posts a correction that is not the exact opposite of one
// entry (permission finance.ledger.adjust, reason).
type AdjustmentInput struct {
	Direction        string       `json:"direction"`
	Amount           *money.Money `json:"amount"`
	Purpose          string       `json:"purpose"`
	EntryDate        *string      `json:"entry_date"`
	DueDate          *string      `json:"due_date"`
	RentalUUID       *uuid.UUID   `json:"rental_uuid"`
	BranchUUID       *uuid.UUID   `json:"branch_uuid"`
	AdjustsEntryUUID *uuid.UUID   `json:"adjusts_entry_uuid"`
	Reason           string       `json:"reason"`
}

// OffsetInput applies a held deposit to the rent / charges (mahsup).
type OffsetInput struct {
	Amount     *money.Money `json:"amount"`
	RentalUUID *uuid.UUID   `json:"rental_uuid"`
	Purpose    string       `json:"purpose"`
	Reason     string       `json:"reason"`
}

// ReverseInput reverses one entry.
type ReverseInput struct {
	Reason string `json:"reason"`
}

// StatementSendInput sends the statement (and a pay link) to the customer.
type StatementSendInput struct {
	PayLink    bool       `json:"pay_link"`
	RentalUUID *uuid.UUID `json:"rental_uuid"`
}

// StatementSent is the answer of a statement send.
type StatementSent struct {
	PhoneMasked string      `json:"phone_masked"`
	MessageUUID uuid.UUID   `json:"message_uuid"`
	Balance     money.Money `json:"balance"`
	PayLinkURL  string      `json:"pay_link_url,omitempty"`
	PayLinkUUID *uuid.UUID  `json:"pay_link_uuid,omitempty"`
}
