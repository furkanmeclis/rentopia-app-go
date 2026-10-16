package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Cash account kinds.
const (
	CashKindCash = "cash"
	CashKindBank = "bank"
)

// Movement kinds.
const (
	MoveTransferIn      = "transfer_in"
	MoveTransferOut     = "transfer_out"
	MoveCollection      = "collection"
	MoveSupplierPayment = "supplier_payment"
	MoveExpense         = "expense"
	MoveRefund          = "refund"
	MoveAdjustment      = "adjustment"
	MoveReversal        = "reversal"
)

// MovementKinds in display order.
var MovementKinds = []string{
	MoveCollection, MoveExpense, MoveRefund, MoveSupplierPayment, MoveTransferIn, MoveTransferOut, MoveAdjustment, MoveReversal,
}

// ManualMovementKinds can be entered on an account (transfers, expenses and
// reversals have their own endpoints).
var ManualMovementKinds = []string{MoveCollection, MoveRefund, MoveSupplierPayment, MoveAdjustment}

// Movement directions.
const (
	DirIn  = "in"
	DirOut = "out"
)

// Expense statuses.
const (
	ExpenseRecorded = "recorded"
	ExpenseVoid     = "void"
)

// DefaultCategory is one seeded expense category.
type DefaultCategory struct {
	Code, Name string
	Sort       int32
}

// DefaultCategories are seeded for every organization (migration 000097
// for existing ones, the organization hook for new ones).
var DefaultCategories = []DefaultCategory{
	{"maintenance", "Bakım", 10},
	{"insurance", "Sigorta", 20},
	{"repair", "Onarım", 30},
	{"fuel", "Yakıt", 40},
	{"cleaning", "Temizlik", 50},
	{"vehicle_procurement", "Araç tedariki", 60},
	{"operations", "Operasyon", 70},
	{"other", "Diğer", 80},
}

// CashAccount is a till or a bank account with its balance.
type CashAccount struct {
	UUID           uuid.UUID   `json:"uuid"`
	Code           string      `json:"code"`
	Name           string      `json:"name"`
	Kind           string      `json:"kind"`
	Branch         *Ref        `json:"branch"`
	Currency       string      `json:"currency"`
	OpeningBalance money.Money `json:"opening_balance"`
	OpeningDate    Date        `json:"opening_date"`
	BankName       string      `json:"bank_name"`
	IBAN           string      `json:"iban"`
	Status         string      `json:"status"`
	Note           string      `json:"note"`
	TotalIn        money.Money `json:"total_in"`
	TotalOut       money.Money `json:"total_out"`
	Balance        money.Money `json:"balance"`
	MovementCount  int64       `json:"movement_count"`
	LastMovementAt *time.Time  `json:"last_movement_at"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// CashAccountInput creates / patches a cash account.
type CashAccountInput struct {
	Code           *string      `json:"code"`
	Name           *string      `json:"name"`
	Kind           *string      `json:"kind"`
	BranchUUID     *uuid.UUID   `json:"branch_uuid"`
	ClearBranch    bool         `json:"clear_branch"`
	Currency       *string      `json:"currency"`
	OpeningBalance *money.Money `json:"opening_balance"`
	OpeningDate    *string      `json:"opening_date"`
	BankName       *string      `json:"bank_name"`
	IBAN           *string      `json:"iban"`
	Note           *string      `json:"note"`
	Status         *string      `json:"status"`
}

// CashAccountFilter filters the cash accounts list.
type CashAccountFilter struct {
	Limit, Offset int32
	Q             string
	SortField     string
	SortDesc      bool
	Kinds         []string
	Statuses      []string
	BranchUUIDs   []uuid.UUID
	HeadOffice    bool
	IDs           []int64
}

// Movement is one cash movement.
type Movement struct {
	UUID           uuid.UUID   `json:"uuid"`
	Account        Ref         `json:"account"`
	AccountKind    string      `json:"account_kind"`
	Kind           string      `json:"kind"`
	Direction      string      `json:"direction"`
	Amount         money.Money `json:"amount"`
	Currency       string      `json:"currency"`
	MovementDate   Date        `json:"movement_date"`
	Description    string      `json:"description"`
	Reason         string      `json:"reason"`
	CounterAccount *Ref        `json:"counter_account"`
	Category       *Ref        `json:"category"`
	TransferGroup  *uuid.UUID  `json:"transfer_group"`
	LedgerEntry    *uuid.UUID  `json:"ledger_entry_uuid"`
	ExpenseUUID    *uuid.UUID  `json:"expense_uuid"`
	ReversesUUID   *uuid.UUID  `json:"reverses_uuid"`
	ReversedByUUID *uuid.UUID  `json:"reversed_by_uuid"`
	Branch         *Ref        `json:"branch"`
	RunningBalance money.Money `json:"running_balance"`
	CreatedBy      *Actor      `json:"created_by"`
	CreatedAt      time.Time   `json:"created_at"`
}

// MovementFilter filters movements.
type MovementFilter struct {
	Limit, Offset int32
	Q             string
	SortField     string
	SortDesc      bool
	AccountUUID   *uuid.UUID
	AccountUUIDs  []uuid.UUID
	Kinds         []string
	Directions    []string
	CategoryUUIDs []uuid.UUID
	BranchUUIDs   []uuid.UUID
	From, To      *time.Time
	IDs           []int64
}

// MovementInput records a movement on an account.
type MovementInput struct {
	Kind            string       `json:"kind"`
	Direction       string       `json:"direction"`
	Amount          *money.Money `json:"amount"`
	MovementDate    *string      `json:"movement_date"`
	Description     string       `json:"description"`
	Reason          string       `json:"reason"`
	CategoryUUID    *uuid.UUID   `json:"category_uuid"`
	LedgerEntryUUID *uuid.UUID   `json:"ledger_entry_uuid"`
	// SupplierUUID makes a supplier_payment a supplier payment of that
	// supplier (ledger entry + FIFO allocation to its open invoices).
	SupplierUUID *uuid.UUID `json:"supplier_uuid"`
}

// TransferInput moves money between two accounts (atomic).
type TransferInput struct {
	FromAccountUUID *uuid.UUID   `json:"from_account_uuid"`
	ToAccountUUID   *uuid.UUID   `json:"to_account_uuid"`
	Amount          *money.Money `json:"amount"`
	MovementDate    *string      `json:"movement_date"`
	Description     string       `json:"description"`
}

// Transfer is the two movements of a transfer.
type Transfer struct {
	Group uuid.UUID `json:"transfer_group"`
	Out   Movement  `json:"out"`
	In    Movement  `json:"in"`
}

// Category is an expense category.
type Category struct {
	UUID         uuid.UUID `json:"uuid"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	IsSystem     bool      `json:"is_system"`
	IsActive     bool      `json:"is_active"`
	SortOrder    int32     `json:"sort_order"`
	ExpenseCount int64     `json:"expense_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CategoryInput creates / patches a category.
type CategoryInput struct {
	Code      *string `json:"code"`
	Name      *string `json:"name"`
	IsActive  *bool   `json:"is_active"`
	SortOrder *int32  `json:"sort_order"`
}

// Expense is one expense record.
type Expense struct {
	UUID         uuid.UUID   `json:"uuid"`
	Category     Ref         `json:"category"`
	ExpenseDate  Date        `json:"expense_date"`
	Description  string      `json:"description"`
	SupplierName string      `json:"supplier_name"`
	DocumentNo   string      `json:"document_no"`
	Currency     string      `json:"currency"`
	NetAmount    money.Money `json:"net_amount"`
	VATRate      money.Rate  `json:"vat_rate"`
	VATAmount    money.Money `json:"vat_amount"`
	TotalAmount  money.Money `json:"total_amount"`
	Branch       *Ref        `json:"branch"`
	Vehicle      *Ref        `json:"vehicle"`
	CashAccount  *Ref        `json:"cash_account"`
	MovementUUID *uuid.UUID  `json:"movement_uuid"`
	// Supplier is the supplier card (TEC-721); SupplierInvoice the invoice
	// the row was generated from (read-only then).
	Supplier        *Ref        `json:"supplier"`
	SupplierInvoice *Ref        `json:"supplier_invoice"`
	Attachment      *Attachment `json:"attachment"`
	Status          string      `json:"status"`
	VoidReason      string      `json:"void_reason"`
	VoidedAt        *time.Time  `json:"voided_at"`
	CreatedBy       *Actor      `json:"created_by"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// Attachment describes the receipt / invoice file (streamed by the API at
// /v1/tenant/finance/expenses/{uuid}/attachment; never a storage URL).
type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	URL         string `json:"url"`
}

// ExpenseInput creates / patches an expense. Amounts, VAT and the paying
// account are fixed after creation (void and record again).
type ExpenseInput struct {
	CategoryUUID    *uuid.UUID   `json:"category_uuid"`
	ExpenseDate     *string      `json:"expense_date"`
	Description     *string      `json:"description"`
	SupplierName    *string      `json:"supplier_name"`
	SupplierUUID    *uuid.UUID   `json:"supplier_uuid"`
	ClearSupplier   bool         `json:"clear_supplier"`
	DocumentNo      *string      `json:"document_no"`
	Currency        *string      `json:"currency"`
	NetAmount       *money.Money `json:"net_amount"`
	TotalAmount     *money.Money `json:"total_amount"`
	VATRate         *money.Rate  `json:"vat_rate"`
	BranchUUID      *uuid.UUID   `json:"branch_uuid"`
	ClearBranch     bool         `json:"clear_branch"`
	VehicleUUID     *uuid.UUID   `json:"vehicle_uuid"`
	ClearVehicle    bool         `json:"clear_vehicle"`
	CashAccountUUID *uuid.UUID   `json:"cash_account_uuid"`
}

// ExpenseFilter filters expenses.
type ExpenseFilter struct {
	Limit, Offset int32
	Q             string
	SortField     string
	SortDesc      bool
	CategoryUUIDs []uuid.UUID
	BranchUUIDs   []uuid.UUID
	VehicleUUIDs  []uuid.UUID
	SupplierUUIDs []uuid.UUID
	Statuses      []string
	From, To      *time.Time
	IDs           []int64
}

// ReportRow is one group of a report.
type ReportRow struct {
	Key      string                 `json:"key"`
	Label    string                 `json:"label"`
	Currency string                 `json:"currency"`
	Count    int64                  `json:"count"`
	Values   map[string]money.Money `json:"values"`
}

// Report is an aggregate over a period.
type Report struct {
	GroupBy string                 `json:"group_by"`
	From    Date                   `json:"from"`
	To      Date                   `json:"to"`
	Rows    []ReportRow            `json:"rows"`
	Totals  map[string]money.Money `json:"totals"`
}
