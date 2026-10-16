package model

import (
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Supplier invoice statuses.
const (
	InvoiceOpen          = "open"
	InvoicePartiallyPaid = "partially_paid"
	InvoicePaid          = "paid"
	InvoiceVoid          = "void"
)

// InvoiceStatuses in display order.
var InvoiceStatuses = []string{InvoiceOpen, InvoicePartiallyPaid, InvoicePaid, InvoiceVoid}

// Invoice due states (computed).
const (
	DueOverdue = "overdue"
	DueSoon    = "due_soon"
	DueLater   = "later"
	DueSettled = "settled"
)

// DueStates in display order.
var DueStates = []string{DueOverdue, DueSoon, DueLater, DueSettled}

// Supplier payment statuses.
const (
	SupplierPaymentPosted = "posted"
	SupplierPaymentVoid   = "void"
)

// SupplierPaymentStatuses in display order.
var SupplierPaymentStatuses = []string{SupplierPaymentPosted, SupplierPaymentVoid}

// Supplier payment methods.
var SupplierPaymentMethods = []string{"bank_transfer", "cash", "card", "cheque", "other"}

// InvoiceKey is the accrual key of a supplier invoice (single accrual).
func InvoiceKey(invoiceID int64) string {
	return "supplier_invoice:" + strconv.FormatInt(invoiceID, 10)
}

// InvoiceLine is one line of a supplier invoice (VAT breakdown).
type InvoiceLine struct {
	LineNo      int32       `json:"line_no"`
	Description string      `json:"description"`
	NetAmount   money.Money `json:"net_amount"`
	VATRate     money.Rate  `json:"vat_rate"`
	VATAmount   money.Money `json:"vat_amount"`
	TotalAmount money.Money `json:"total_amount"`
}

// InvoicePayment is a payment allocated to an invoice.
type InvoicePayment struct {
	PaymentUUID uuid.UUID   `json:"payment_uuid"`
	PaymentDate Date        `json:"payment_date"`
	Status      string      `json:"status"`
	Method      string      `json:"method"`
	Amount      money.Money `json:"amount"`
	CashAccount Ref         `json:"cash_account"`
}

// SupplierInvoice is one supplier invoice (gider faturası).
type SupplierInvoice struct {
	UUID        uuid.UUID        `json:"uuid"`
	InvoiceNo   string           `json:"invoice_no"`
	InvoiceDate Date             `json:"invoice_date"`
	DueDate     Date             `json:"due_date"`
	Supplier    Ref              `json:"supplier"`
	Account     Ref              `json:"account"`
	Category    Ref              `json:"category"`
	Branch      *Ref             `json:"branch"`
	Vehicle     *Ref             `json:"vehicle"`
	Description string           `json:"description"`
	Currency    string           `json:"currency"`
	NetAmount   money.Money      `json:"net_amount"`
	VATAmount   money.Money      `json:"vat_amount"`
	TotalAmount money.Money      `json:"total_amount"`
	PaidAmount  money.Money      `json:"paid_amount"`
	OpenAmount  money.Money      `json:"open_amount"`
	Status      string           `json:"status"`
	DueState    string           `json:"due_state"`
	ExpenseUUID *uuid.UUID       `json:"expense_uuid"`
	Attachment  *Attachment      `json:"attachment"`
	Lines       []InvoiceLine    `json:"lines"`
	Payments    []InvoicePayment `json:"payments"`
	VoidReason  string           `json:"void_reason"`
	VoidedAt    *time.Time       `json:"voided_at"`
	CreatedBy   *Actor           `json:"created_by"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// InvoiceLineInput is one line of a new invoice.
type InvoiceLineInput struct {
	Description string       `json:"description"`
	NetAmount   *money.Money `json:"net_amount"`
	VATRate     *money.Rate  `json:"vat_rate"`
}

// SupplierInvoiceInput creates / patches a supplier invoice. Supplier,
// amounts, lines, invoice date and currency are fixed once posted (void
// and record again); the number, due date, category, branch, vehicle and
// description stay editable.
type SupplierInvoiceInput struct {
	SupplierUUID *uuid.UUID         `json:"supplier_uuid"`
	InvoiceNo    *string            `json:"invoice_no"`
	InvoiceDate  *string            `json:"invoice_date"`
	DueDate      *string            `json:"due_date"`
	Currency     *string            `json:"currency"`
	CategoryUUID *uuid.UUID         `json:"category_uuid"`
	BranchUUID   *uuid.UUID         `json:"branch_uuid"`
	ClearBranch  bool               `json:"clear_branch"`
	VehicleUUID  *uuid.UUID         `json:"vehicle_uuid"`
	ClearVehicle bool               `json:"clear_vehicle"`
	Description  *string            `json:"description"`
	NetAmount    *money.Money       `json:"net_amount"`
	TotalAmount  *money.Money       `json:"total_amount"`
	VATRate      *money.Rate        `json:"vat_rate"`
	Lines        []InvoiceLineInput `json:"lines"`
}

// SupplierInvoiceFilter filters supplier invoices.
type SupplierInvoiceFilter struct {
	Limit, Offset int32
	Q             string
	SortField     string
	SortDesc      bool
	SupplierUUIDs []uuid.UUID
	Statuses      []string
	DueStates     []string
	CategoryUUIDs []uuid.UUID
	BranchUUIDs   []uuid.UUID
	From, To      *time.Time
	DueTo         *time.Time
	// WindowDays is the "due soon" window (default DueSoonDays).
	WindowDays int
	IDs        []int64
}

// SupplierAllocation is the part of a payment applied to one invoice.
type SupplierAllocation struct {
	InvoiceUUID uuid.UUID   `json:"invoice_uuid"`
	InvoiceNo   string      `json:"invoice_no"`
	DueDate     Date        `json:"due_date"`
	Amount      money.Money `json:"amount"`
}

// SupplierPayment is one payment to a supplier.
type SupplierPayment struct {
	UUID            uuid.UUID            `json:"uuid"`
	Supplier        Ref                  `json:"supplier"`
	Account         Ref                  `json:"account"`
	CashAccount     Ref                  `json:"cash_account"`
	CashAccountKind string               `json:"cash_account_kind"`
	Amount          money.Money          `json:"amount"`
	AllocatedAmount money.Money          `json:"allocated_amount"`
	UnallocatedAmt  money.Money          `json:"unallocated_amount"`
	Currency        string               `json:"currency"`
	PaymentDate     Date                 `json:"payment_date"`
	Method          string               `json:"method"`
	Reference       string               `json:"reference"`
	Description     string               `json:"description"`
	Branch          *Ref                 `json:"branch"`
	LedgerEntryUUID *uuid.UUID           `json:"ledger_entry_uuid"`
	MovementUUID    *uuid.UUID           `json:"movement_uuid"`
	Allocations     []SupplierAllocation `json:"allocations"`
	Status          string               `json:"status"`
	VoidReason      string               `json:"void_reason"`
	VoidedAt        *time.Time           `json:"voided_at"`
	CreatedBy       *Actor               `json:"created_by"`
	CreatedAt       time.Time            `json:"created_at"`
}

// AllocationInput applies part of a payment to one invoice.
type AllocationInput struct {
	InvoiceUUID *uuid.UUID   `json:"invoice_uuid"`
	Amount      *money.Money `json:"amount"`
}

// SupplierPaymentInput records a payment. Without allocations the amount
// is applied to the supplier's open invoices oldest due date first
// (auto_allocate, default true); what is left is an advance.
type SupplierPaymentInput struct {
	SupplierUUID    *uuid.UUID        `json:"supplier_uuid"`
	CashAccountUUID *uuid.UUID        `json:"cash_account_uuid"`
	Amount          *money.Money      `json:"amount"`
	PaymentDate     *string           `json:"payment_date"`
	Method          string            `json:"method"`
	Reference       string            `json:"reference"`
	Description     string            `json:"description"`
	Allocations     []AllocationInput `json:"allocations"`
	AutoAllocate    *bool             `json:"auto_allocate"`
}

// SupplierPaymentFilter filters supplier payments.
type SupplierPaymentFilter struct {
	Limit, Offset    int32
	Q                string
	SortField        string
	SortDesc         bool
	SupplierUUIDs    []uuid.UUID
	CashAccountUUIDs []uuid.UUID
	Statuses         []string
	Methods          []string
	InvoiceUUID      *uuid.UUID
	From, To         *time.Time
	IDs              []int64
}

// VoidInput voids an invoice or a payment with a reason.
type VoidInput struct {
	Reason string `json:"reason"`
}

// PayablesSummary is the open supplier invoices by due state.
type PayablesSummary struct {
	Currency       string      `json:"currency"`
	WindowDays     int         `json:"window_days"`
	OverdueAmount  money.Money `json:"overdue_amount"`
	OverdueCount   int64       `json:"overdue_count"`
	DueSoonAmount  money.Money `json:"due_soon_amount"`
	DueSoonCount   int64       `json:"due_soon_count"`
	OpenAmount     money.Money `json:"open_amount"`
	OpenCount      int64       `json:"open_count"`
	AttentionCount int64       `json:"attention_count"`
}
