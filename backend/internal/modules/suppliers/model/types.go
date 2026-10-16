// Package model holds the supplier types (TEC-721, docs/design/suppliers.md).
package model

import (
	"encoding/json"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Resource is the engine / live list slug of suppliers.
const Resource = "tenant.suppliers"

// SearchSpec is the Cmd+K spec id.
const SearchSpec = "suppliers"

// Supplier types.
const (
	TypeService         = "service"
	TypeInsurance       = "insurance"
	TypeVehicleSupplier = "vehicle_supplier"
	TypeParts           = "parts"
	TypeFuel            = "fuel"
	TypeOther           = "other"
)

// Types in display order.
var Types = []string{TypeService, TypeInsurance, TypeVehicleSupplier, TypeParts, TypeFuel, TypeOther}

// Statuses of a supplier card.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Statuses in display order.
var Statuses = []string{StatusActive, StatusArchived}

// Due states of a supplier (computed from its open invoices).
var DueStates = []string{"overdue", "due_soon", "open", "settled"}

// Tax number kinds (derived from the length).
const (
	TaxKindVKN  = "vkn"
	TaxKindTCKN = "tckn"
)

// DefaultPaymentTermDays is the default invoice term of a new supplier.
const DefaultPaymentTermDays = 30

// MaxContacts bounds the contact persons of a supplier.
const MaxContacts = 10

// Contact is a contact person of a supplier.
type Contact struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

// Ref is a reference to another record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code,omitempty"`
	Name string    `json:"name,omitempty"`
}

// Supplier is a supplier card with its payables figures.
type Supplier struct {
	UUID            uuid.UUID `json:"uuid"`
	Code            string    `json:"code"`
	Title           string    `json:"title"`
	SupplierType    string    `json:"supplier_type"`
	TaxNumber       string    `json:"tax_number"`
	TaxKind         string    `json:"tax_kind"`
	TaxOffice       string    `json:"tax_office"`
	Phone           string    `json:"phone"`
	Email           string    `json:"email"`
	Website         string    `json:"website"`
	Address         string    `json:"address"`
	City            string    `json:"city"`
	BillingAddress  string    `json:"billing_address"`
	BillingEmail    string    `json:"billing_email"`
	EInvoice        bool      `json:"e_invoice"`
	BankName        string    `json:"bank_name"`
	IBAN            string    `json:"iban"`
	PaymentTermDays int32     `json:"payment_term_days"`
	Contacts        []Contact `json:"contacts"`
	Status          string    `json:"status"`
	Note            string    `json:"note"`
	// Account is the supplier's ledger account (nil until the first
	// invoice / payment). Balance is its debits minus credits (negative =
	// we owe); Payable the positive amount owed.
	Account       *Ref        `json:"account"`
	Balance       money.Money `json:"balance"`
	Payable       money.Money `json:"payable"`
	OpenAmount    money.Money `json:"open_amount"`
	OverdueAmount money.Money `json:"overdue_amount"`
	DueSoonAmount money.Money `json:"due_soon_amount"`
	OpenInvoices  int64       `json:"open_invoices"`
	NextDueDate   *string     `json:"next_due_date"`
	VehicleCount  int64       `json:"vehicle_count"`
	DueState      string      `json:"due_state"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// Input creates / patches a supplier (nil = unchanged on patch).
type Input struct {
	Code            *string    `json:"code"`
	Title           *string    `json:"title"`
	SupplierType    *string    `json:"supplier_type"`
	TaxNumber       *string    `json:"tax_number"`
	TaxOffice       *string    `json:"tax_office"`
	Phone           *string    `json:"phone"`
	Email           *string    `json:"email"`
	Website         *string    `json:"website"`
	Address         *string    `json:"address"`
	City            *string    `json:"city"`
	BillingAddress  *string    `json:"billing_address"`
	BillingEmail    *string    `json:"billing_email"`
	EInvoice        *bool      `json:"e_invoice"`
	BankName        *string    `json:"bank_name"`
	IBAN            *string    `json:"iban"`
	PaymentTermDays *int32     `json:"payment_term_days"`
	Contacts        *[]Contact `json:"contacts"`
	Status          *string    `json:"status"`
	Note            *string    `json:"note"`
}

// Filter filters the supplier list.
type Filter struct {
	Limit, Offset int32
	Q             string
	SortField     string
	SortDesc      bool
	Types         []string
	Statuses      []string
	Cities        []string
	DueStates     []string
	IDs           []int64
}

// Facet is one option of a dynamic filter.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the supplier list.
type Facets struct {
	SupplierType []Facet `json:"supplier_type"`
	Status       []Facet `json:"status"`
	City         []Facet `json:"city"`
}

// ActivityActor is the user behind an activity entry.
type ActivityActor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ActivityEntry is one audit event of a supplier.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *ActivityActor  `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}
