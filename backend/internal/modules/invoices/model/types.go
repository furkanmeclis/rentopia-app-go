// Package model holds the types of e-invoices / e-archive invoices
// (TEC-722, docs/design/einvoice.md).
package model

import (
	"time"

	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/ubl"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Resource is the engine / live list / activity resource.
const Resource = "tenant.invoices"

// Invoice statuses.
const (
	StatusDraft          = "draft"
	StatusQueued         = "queued"
	StatusSent           = "sent"
	StatusAwaitingResult = "awaiting_result"
	StatusSucceeded      = "succeeded"
	StatusFailed         = "failed"
	StatusCancelled      = "cancelled"
)

// Statuses lists every status (facets, validation).
var Statuses = []string{StatusDraft, StatusQueued, StatusSent, StatusAwaitingResult, StatusSucceeded, StatusFailed, StatusCancelled}

// Profiles and types.
const (
	ProfileTemel  = ubl.ProfileTemel
	ProfileTicari = ubl.ProfileTicari
	ProfileEArsiv = ubl.ProfileEArsiv
	TypeSatis     = ubl.TypeSatis
	TypeIade      = ubl.TypeIade
)

// Profiles lists the profiles the module issues.
var Profiles = []string{ProfileTemel, ProfileTicari, ProfileEArsiv}

// Types lists the invoice types.
var Types = []string{TypeSatis, TypeIade}

// Line kinds (tax mapping keys).
const (
	KindRent   = "rent"
	KindExtra  = "extra"
	KindOneWay = "one_way"
	KindCharge = "charge"
	KindToll   = "toll"
)

// LineKinds lists the line kinds in display order.
var LineKinds = []string{KindRent, KindExtra, KindOneWay, KindCharge, KindToll}

// Job kinds on the integration runner.
const (
	JobKindSend   = "einvoice.send"
	JobKindCancel = "einvoice.cancel"
)

// Hold reasons of a send waiting on a dependency.
const (
	HoldNoAccount          = "no_account"
	HoldAdapterUnverified  = "adapter_unverified"
	HoldAdapterUnavailable = "adapter_unavailable"
)

// Taxpayer sources.
const (
	TaxpayerProvider     = "provider"
	TaxpayerCache        = "cache"
	TaxpayerCustomerFlag = "customer_flag"
)

// Link release reasons.
const (
	ReleaseCancelled = "cancelled"
	ReleaseReturned  = "returned"
	ReleaseRemoved   = "removed"
)

// Contains reports whether v is in list.
func Contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Ref names a related record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// RentalRef names a rental.
type RentalRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
}

// InvoiceRef names another invoice (the returned one / the return).
type InvoiceRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Number *string   `json:"number"`
	Status string    `json:"status,omitempty"`
}

// Actor is a user who acted.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email,omitempty"`
}

// Receiver is the invoice's receiver snapshot. The identity number is
// write-only: the API shows its hint.
type Receiver struct {
	Type              string     `json:"type"`
	IdentityType      *string    `json:"identity_type"`
	IdentityHint      string     `json:"identity_hint"`
	Name              string     `json:"name"`
	FirstName         string     `json:"first_name"`
	LastName          string     `json:"last_name"`
	TaxOffice         string     `json:"tax_office"`
	Address           string     `json:"address"`
	District          string     `json:"district"`
	City              string     `json:"city"`
	PostalCode        string     `json:"postal_code"`
	Country           string     `json:"country"`
	Email             string     `json:"email"`
	EInvoice          bool       `json:"einvoice"`
	Alias             string     `json:"alias"`
	TaxpayerSource    string     `json:"taxpayer_source"`
	TaxpayerCheckedAt *time.Time `json:"taxpayer_checked_at"`
}

// Line is one invoice line.
type Line struct {
	UUID            uuid.UUID   `json:"uuid"`
	Position        int32       `json:"position"`
	Kind            string      `json:"kind"`
	Name            string      `json:"name"`
	Days            int32       `json:"days"`
	EntryUUID       *uuid.UUID  `json:"entry_uuid"`
	Gross           money.Money `json:"gross_amount"`
	Net             money.Money `json:"net_amount"`
	VATRate         money.Rate  `json:"vat_rate"`
	VAT             money.Money `json:"vat_amount"`
	Total           money.Money `json:"total_amount"`
	ExemptionCode   string      `json:"exemption_code"`
	ExemptionReason string      `json:"exemption_reason"`
}

// Link is a ledger item the invoice covers (ledger_entry_links).
type Link struct {
	EntryUUID     uuid.UUID   `json:"entry_uuid"`
	Kind          string      `json:"entry_kind"`
	Purpose       string      `json:"purpose"`
	EntryAmount   money.Money `json:"entry_amount"`
	Amount        money.Money `json:"amount"`
	EntryDate     string      `json:"entry_date"`
	Description   string      `json:"description"`
	Rental        *RentalRef  `json:"rental"`
	Released      bool        `json:"released"`
	ReleasedAt    *time.Time  `json:"released_at"`
	ReleaseReason string      `json:"release_reason"`
}

// Invoice is a list row.
type Invoice struct {
	UUID          uuid.UUID   `json:"uuid"`
	Number        *string     `json:"number"`
	Profile       string      `json:"profile"`
	Type          string      `json:"invoice_type"`
	Status        string      `json:"status"`
	IssueDate     string      `json:"issue_date"`
	Currency      string      `json:"currency"`
	ReceiverType  string      `json:"receiver_type"`
	ReceiverName  string      `json:"receiver_name"`
	ReceiverHint  string      `json:"receiver_identity_hint"`
	EInvoice      bool        `json:"receiver_einvoice"`
	LineExtension money.Money `json:"line_extension_amount"`
	Tax           money.Money `json:"tax_amount"`
	Payable       money.Money `json:"payable_amount"`
	Linked        money.Money `json:"linked_amount"`
	Rounding      money.Money `json:"rounding_amount"`
	Branch        Ref         `json:"branch"`
	Account       *Ref        `json:"account"`
	Rental        *RentalRef  `json:"rental"`
	Returns       *InvoiceRef `json:"returns"`
	ExternalRef   string      `json:"external_ref"`
	HoldReason    string      `json:"hold_reason"`
	LastError     string      `json:"last_error"`
	JobStatus     *string     `json:"job_status"`
	Attempts      int32       `json:"attempts"`
	NeedsAction   bool        `json:"needs_action"`
	ProblemCount  int         `json:"problem_count"`
	Returned      bool        `json:"returned"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// JobState is the state of the send / cancel job.
type JobState struct {
	UUID          uuid.UUID  `json:"uuid"`
	Status        string     `json:"status"`
	Attempts      int32      `json:"attempts"`
	MaxAttempts   int32      `json:"max_attempts"`
	LastError     string     `json:"last_error"`
	Provider      string     `json:"provider"`
	ProviderRef   string     `json:"provider_ref"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	Busy          bool       `json:"busy"`
}

// Actions the invoice allows now (the caller's permissions aside).
type Actions struct {
	Edit        bool `json:"edit"`
	Validate    bool `json:"validate"`
	Finalize    bool `json:"finalize"`
	Retry       bool `json:"retry"`
	Cancel      bool `json:"cancel"`
	Return      bool `json:"return"`
	DownloadXML bool `json:"download_xml"`
	DownloadPDF bool `json:"download_pdf"`
	Preview     bool `json:"preview"`
}

// Detail is an invoice with its receiver, lines, ledger links, validation
// problems, the send / cancel jobs and their attempts.
type Detail struct {
	Invoice
	Receiver         Receiver              `json:"receiver"`
	Note             string                `json:"note"`
	Lines            []Line                `json:"lines"`
	Links            []Link                `json:"links"`
	ValidationErrors []ubl.Problem         `json:"validation_errors"`
	ValidatedAt      *time.Time            `json:"validated_at"`
	ReturnedBy       *InvoiceRef           `json:"returned_by"`
	Job              *JobState             `json:"job"`
	CancelJob        *JobState             `json:"cancel_job"`
	AttemptLog       []jobsusecase.Attempt `json:"attempt_log"`
	CancelAttemptLog []jobsusecase.Attempt `json:"cancel_attempt_log"`
	Actions          Actions               `json:"actions"`
	CancelReason     string                `json:"cancel_reason"`
	CancelledAt      *time.Time            `json:"cancelled_at"`
	CancelledBy      *Actor                `json:"cancelled_by"`
	FinalizedAt      *time.Time            `json:"finalized_at"`
	SentAt           *time.Time            `json:"sent_at"`
	SucceededAt      *time.Time            `json:"succeeded_at"`
	ReturnedAt       *time.Time            `json:"returned_at"`
	CreatedBy        *Actor                `json:"created_by"`
}

// Item is a ledger item that may be invoiced (a live accrual no standing
// invoice covers).
type Item struct {
	EntryUUID   uuid.UUID   `json:"entry_uuid"`
	Purpose     string      `json:"purpose"`
	Kind        string      `json:"line_kind"`
	ChargeKind  *string     `json:"charge_kind"`
	Amount      money.Money `json:"amount"`
	Currency    string      `json:"currency"`
	EntryDate   string      `json:"entry_date"`
	Description string      `json:"description"`
	Account     Ref         `json:"account"`
	Rental      *RentalRef  `json:"rental"`
	Branch      *Ref        `json:"branch"`
}

// UninvoicedRental is a rental with invoiceable items (bulk picker).
type UninvoicedRental struct {
	UUID         uuid.UUID   `json:"uuid"`
	Reference    string      `json:"reference"`
	Status       string      `json:"status"`
	CustomerName string      `json:"customer_name"`
	Branch       Ref         `json:"branch"`
	OpenAmount   money.Money `json:"open_amount"`
	ItemCount    int64       `json:"item_count"`
	FirstDate    string      `json:"first_date"`
	PickupAt     *time.Time  `json:"actual_pickup_at"`
	ReturnAt     *time.Time  `json:"actual_return_at"`
}

// CreateInput makes a draft: from a rental (its invoiceable items), or an
// account's items (all of them, or the entries named).
type CreateInput struct {
	RentalUUID     *uuid.UUID  `json:"rental_uuid"`
	AccountUUID    *uuid.UUID  `json:"account_uuid"`
	EntryUUIDs     []uuid.UUID `json:"entry_uuids"`
	BranchUUID     *uuid.UUID  `json:"branch_uuid"`
	IssueDate      *string     `json:"issue_date"`
	Note           *string     `json:"note"`
	IdempotencyKey string      `json:"idempotency_key"`
}

// ReceiverInput edits the receiver of a draft (nil = unchanged).
type ReceiverInput struct {
	Type           *string `json:"type"`
	IdentityType   *string `json:"identity_type"`
	IdentityNumber *string `json:"identity_number"`
	Name           *string `json:"name"`
	FirstName      *string `json:"first_name"`
	LastName       *string `json:"last_name"`
	TaxOffice      *string `json:"tax_office"`
	Address        *string `json:"address"`
	District       *string `json:"district"`
	City           *string `json:"city"`
	PostalCode     *string `json:"postal_code"`
	Country        *string `json:"country"`
	Email          *string `json:"email"`
}

// LinePatch edits one line (the amount follows its ledger item).
type LinePatch struct {
	UUID            uuid.UUID `json:"uuid"`
	Name            *string   `json:"name"`
	VATRate         *string   `json:"vat_rate"`
	ExemptionCode   *string   `json:"exemption_code"`
	ExemptionReason *string   `json:"exemption_reason"`
}

// PatchInput edits a draft.
type PatchInput struct {
	IssueDate        *string        `json:"issue_date"`
	Note             *string        `json:"note"`
	Profile          *string        `json:"profile"`
	Receiver         *ReceiverInput `json:"receiver"`
	Lines            []LinePatch    `json:"lines"`
	RemoveEntryUUIDs []uuid.UUID    `json:"remove_entry_uuids"`
}

// CancelInput cancels an invoice.
type CancelInput struct {
	Reason string `json:"reason"`
}

// ReturnInput creates the IADE (return) draft of an issued invoice.
type ReturnInput struct {
	Reason    string  `json:"reason"`
	IssueDate *string `json:"issue_date"`
}

// ListFilter filters the invoice list.
type ListFilter struct {
	Q           string
	Statuses    []string
	Profiles    []string
	Types       []string
	BranchUUIDs []uuid.UUID
	AccountUUID *uuid.UUID
	RentalUUID  *uuid.UUID
	From, To    *string
	NeedsAction *bool
	SortField   string
	SortDesc    bool
	Limit       int32
	Offset      int32
}

// ItemFilter filters invoiceable items.
type ItemFilter struct {
	AccountUUID *uuid.UUID
	RentalUUID  *uuid.UUID
}

// RentalFilter filters the uninvoiced rentals picker.
type RentalFilter struct {
	Q      string
	UUIDs  []uuid.UUID
	Limit  int32
	Offset int32
}

// Facet is one value with its count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the list filter counts.
type Facets struct {
	Status      []Facet `json:"status"`
	Profile     []Facet `json:"profile"`
	Branch      []Facet `json:"branch"`
	NeedsAction int64   `json:"needs_action"`
}

// RetryResult is the answer of a retry.
type RetryResult struct {
	Action string `json:"action"`
	Status string `json:"status"`
}

// TaxMapping maps a line kind to its VAT rate / exemption.
type TaxMapping struct {
	LineKind        string     `json:"line_kind"`
	VATRate         money.Rate `json:"vat_rate"`
	ExemptionCode   string     `json:"exemption_code"`
	ExemptionReason string     `json:"exemption_reason"`
	Default         bool       `json:"default"`
}

// Supplier is the organization as invoice supplier (company settings +
// invoice settings).
type Supplier struct {
	LegalName  string `json:"legal_name"`
	TaxNumber  string `json:"tax_number"`
	TaxOffice  string `json:"tax_office"`
	Address    string `json:"address"`
	District   string `json:"district"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
}

// Connection is the state of the organization's integrator account.
type Connection struct {
	// State: none, waiting (official slot, TEC-685), test (fake adapter),
	// unavailable (adapter missing in this environment), ready.
	State       string     `json:"state"`
	AccountUUID *uuid.UUID `json:"account_uuid"`
	Provider    string     `json:"provider"`
	Name        string     `json:"name"`
}

// Settings are the invoice settings of the organization.
type Settings struct {
	EFaturaSeries      string       `json:"efatura_series"`
	EArsivSeries       string       `json:"earsiv_series"`
	SupplierDistrict   string       `json:"supplier_district"`
	SupplierCity       string       `json:"supplier_city"`
	SupplierPostalCode string       `json:"supplier_postal_code"`
	DefaultNote        string       `json:"default_note"`
	TaxpayerCacheHours int32        `json:"taxpayer_cache_hours"`
	TaxMappings        []TaxMapping `json:"tax_mappings"`
	Supplier           Supplier     `json:"supplier"`
	// Missing lists the supplier fields an invoice still needs
	// (supplier.tax_number, supplier.district, …).
	Missing    []string   `json:"missing"`
	Connection Connection `json:"connection"`
	UpdatedAt  *time.Time `json:"updated_at"`
}

// TaxMappingInput edits one mapping.
type TaxMappingInput struct {
	LineKind        string `json:"line_kind"`
	VATRate         string `json:"vat_rate"`
	ExemptionCode   string `json:"exemption_code"`
	ExemptionReason string `json:"exemption_reason"`
}

// SettingsInput edits the settings (nil = unchanged).
type SettingsInput struct {
	EFaturaSeries      *string           `json:"efatura_series"`
	EArsivSeries       *string           `json:"earsiv_series"`
	SupplierDistrict   *string           `json:"supplier_district"`
	SupplierCity       *string           `json:"supplier_city"`
	SupplierPostalCode *string           `json:"supplier_postal_code"`
	DefaultNote        *string           `json:"default_note"`
	TaxpayerCacheHours *int32            `json:"taxpayer_cache_hours"`
	TaxMappings        []TaxMappingInput `json:"tax_mappings"`
}

// PortalInvoice is an issued invoice as the customer portal shows it.
type PortalInvoice struct {
	UUID         uuid.UUID   `json:"uuid"`
	Number       string      `json:"number"`
	Profile      string      `json:"profile"`
	Type         string      `json:"invoice_type"`
	IssueDate    string      `json:"issue_date"`
	Payable      money.Money `json:"payable_amount"`
	Currency     string      `json:"currency"`
	DownloadPath string      `json:"download_path"`
}
