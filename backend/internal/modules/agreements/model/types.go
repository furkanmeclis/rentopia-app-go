// Package model holds the API shapes of corporate agreements (TEC-723,
// docs/design/corporate-agreements.md). Money is pkg/money (JSON decimal
// strings), dates are rentaltime.Date ("2006-01-02", Europe/Istanbul).
package model

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

// Resource is the engine / activity / live slug of agreements.
const Resource = "tenant.agreements"

// Statuses.
const (
	StatusDraft      = "draft"
	StatusActive     = "active"
	StatusSuspended  = "suspended"
	StatusTerminated = "terminated"
)

// Statuses lists the agreement statuses.
var Statuses = []string{StatusDraft, StatusActive, StatusSuspended, StatusTerminated}

// Validity states (computed against today).
const (
	ValidityUpcoming = "upcoming"
	ValidityCurrent  = "current"
	ValidityExpired  = "expired"
)

// Validities lists the validity states.
var Validities = []string{ValidityUpcoming, ValidityCurrent, ValidityExpired}

// Invoice periods.
const (
	InvoiceMonthly   = "monthly"
	InvoicePerRental = "per_rental"
)

// InvoicePeriods lists the invoice periods.
var InvoicePeriods = []string{InvoiceMonthly, InvoicePerRental}

// Contact roles.
var ContactRoles = []string{"signatory", "billing", "operations", "other"}

// Transitions allowed by SetStatus (from → to).
var Transitions = map[string][]string{
	StatusDraft:     {StatusActive, StatusTerminated},
	StatusActive:    {StatusSuspended, StatusTerminated},
	StatusSuspended: {StatusActive, StatusTerminated},
}

// Ref is an embedded record.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code,omitempty"`
	Name string    `json:"name"`
}

// CustomerRef is the agreement's corporate customer (masked identity).
type CustomerRef struct {
	UUID           uuid.UUID `json:"uuid"`
	CustomerType   string    `json:"customer_type"`
	DisplayName    string    `json:"display_name"`
	IdentityMasked string    `json:"identity_masked,omitempty"`
}

// Actor is a user embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// AccountRef is the customer's ledger account.
type AccountRef struct {
	UUID            uuid.UUID `json:"uuid"`
	Code            string    `json:"code"`
	PaymentTermDays int32     `json:"payment_term_days"`
}

// Exposure is the open-account position of the agreement's customer.
type Exposure struct {
	CreditLimit money.Money `json:"credit_limit"`
	Balance     money.Money `json:"balance"`
	Pending     money.Money `json:"pending"`
	Available   money.Money `json:"available"`
}

// Contact is an authorised contact (a contact of the corporate customer).
type Contact struct {
	UUID        uuid.UUID `json:"uuid"`
	ContactUUID uuid.UUID `json:"contact_uuid"`
	FullName    string    `json:"full_name"`
	Title       string    `json:"title"`
	// PhoneMasked / EmailMasked: contacts are masked like every customer
	// read (reveal goes through the customers API).
	PhoneMasked string    `json:"phone_masked"`
	EmailMasked string    `json:"email_masked"`
	IsPrimary   bool      `json:"is_primary"`
	Role        string    `json:"role"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

// Driver is a permitted driver: a driver record or an individual customer.
type Driver struct {
	UUID           uuid.UUID        `json:"uuid"`
	Kind           string           `json:"kind"`
	DriverUUID     *uuid.UUID       `json:"driver_uuid"`
	CustomerUUID   *uuid.UUID       `json:"customer_uuid"`
	Name           string           `json:"name"`
	Owner          string           `json:"owner"`
	LicenceMasked  string           `json:"licence_masked,omitempty"`
	LicenceExpires *rentaltime.Date `json:"licence_expires_on,omitempty"`
	Note           string           `json:"note"`
	CreatedAt      time.Time        `json:"created_at"`
}

// Agreement is the detail of an agreement.
type Agreement struct {
	UUID                    uuid.UUID        `json:"uuid"`
	Code                    string           `json:"code"`
	Name                    string           `json:"name"`
	Status                  string           `json:"status"`
	Validity                string           `json:"validity"`
	Customer                CustomerRef      `json:"customer"`
	Branch                  *Ref             `json:"branch"`
	ValidFrom               rentaltime.Date  `json:"valid_from"`
	ValidTo                 *rentaltime.Date `json:"valid_to"`
	InvoicePeriod           string           `json:"invoice_period"`
	PaymentTermDays         int32            `json:"payment_term_days"`
	OpenAccount             bool             `json:"open_account"`
	CreditLimit             *money.Money     `json:"credit_limit"`
	CouponEligible          bool             `json:"coupon_eligible"`
	MonthlyKmAllowance      *int32           `json:"monthly_km_allowance"`
	MaintenanceIntervalDays *int32           `json:"maintenance_interval_days"`
	MaintenanceIntervalKm   *int32           `json:"maintenance_interval_km"`
	Currency                string           `json:"currency"`
	Notes                   string           `json:"notes"`
	TerminatedAt            *time.Time       `json:"terminated_at"`
	TerminationReason       string           `json:"termination_reason"`
	Account                 *AccountRef      `json:"account"`
	Exposure                *Exposure        `json:"exposure"`
	Contacts                []Contact        `json:"contacts"`
	Drivers                 []Driver         `json:"drivers"`
	CreatedBy               *Actor           `json:"created_by"`
	CreatedAt               time.Time        `json:"created_at"`
	UpdatedAt               time.Time        `json:"updated_at"`
}

// ListItem is a row of the agreements list.
type ListItem struct {
	UUID             uuid.UUID        `json:"uuid"`
	Code             string           `json:"code"`
	Name             string           `json:"name"`
	Status           string           `json:"status"`
	Validity         string           `json:"validity"`
	Customer         CustomerRef      `json:"customer"`
	Branch           *Ref             `json:"branch"`
	ValidFrom        rentaltime.Date  `json:"valid_from"`
	ValidTo          *rentaltime.Date `json:"valid_to"`
	InvoicePeriod    string           `json:"invoice_period"`
	PaymentTermDays  int32            `json:"payment_term_days"`
	OpenAccount      bool             `json:"open_account"`
	CreditLimit      *money.Money     `json:"credit_limit"`
	CouponEligible   bool             `json:"coupon_eligible"`
	Currency         string           `json:"currency"`
	ReservationCount int64            `json:"reservation_count"`
	TariffCount      int64            `json:"tariff_count"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// Input is the create (POST) and edit (PATCH) body. On create the
// customer, name and valid_from are required; PATCH changes the fields
// sent (the customer never changes).
type Input struct {
	CustomerUUID            *uuid.UUID       `json:"customer_uuid"`
	Name                    *string          `json:"name"`
	BranchUUID              *uuid.UUID       `json:"branch_uuid"`
	ClearBranch             bool             `json:"clear_branch"`
	ValidFrom               *rentaltime.Date `json:"valid_from"`
	ValidTo                 *rentaltime.Date `json:"valid_to"`
	ClearValidTo            bool             `json:"clear_valid_to"`
	InvoicePeriod           *string          `json:"invoice_period"`
	PaymentTermDays         *int32           `json:"payment_term_days"`
	OpenAccount             *bool            `json:"open_account"`
	CreditLimit             *money.Money     `json:"credit_limit"`
	CouponEligible          *bool            `json:"coupon_eligible"`
	MonthlyKmAllowance      *int32           `json:"monthly_km_allowance"`
	MaintenanceIntervalDays *int32           `json:"maintenance_interval_days"`
	MaintenanceIntervalKm   *int32           `json:"maintenance_interval_km"`
	Notes                   *string          `json:"notes"`
	// Activate creates the agreement active (create only).
	Activate bool `json:"activate"`
}

// StatusInput is the body of POST …/{uuid}/status.
type StatusInput struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// ContactInput is the body of POST …/{uuid}/contacts.
type ContactInput struct {
	ContactUUID *uuid.UUID `json:"contact_uuid"`
	Role        string     `json:"role"`
	Note        string     `json:"note"`
}

// DriverInput is the body of POST …/{uuid}/drivers: a driver record or an
// individual customer.
type DriverInput struct {
	DriverUUID   *uuid.UUID `json:"driver_uuid"`
	CustomerUUID *uuid.UUID `json:"customer_uuid"`
	Note         string     `json:"note"`
}

// RentalRow is a reservation sold under the agreement with its rental.
type RentalRow struct {
	ReservationUUID uuid.UUID   `json:"reservation_uuid"`
	Reference       string      `json:"reference"`
	Status          string      `json:"status"`
	StartsAt        time.Time   `json:"starts_at"`
	EndsAt          time.Time   `json:"ends_at"`
	Product         Ref         `json:"product"`
	Customer        CustomerRef `json:"customer"`
	Total           money.Money `json:"total"`
	Currency        string      `json:"currency"`
	OpenAccount     bool        `json:"open_account"`
	Rental          *RentalRef  `json:"rental"`
	RentAccrued     money.Money `json:"rent_accrued"`
	RentOpen        money.Money `json:"rent_open"`
	Plate           string      `json:"plate"`
	VehicleUUID     *uuid.UUID  `json:"vehicle_uuid"`
}

// RentalRef is the rental of an agreement reservation.
type RentalRef struct {
	UUID           uuid.UUID  `json:"uuid"`
	Status         string     `json:"status"`
	RentBilling    string     `json:"rent_billing"`
	ActualPickupAt *time.Time `json:"actual_pickup_at"`
	ActualReturnAt *time.Time `json:"actual_return_at"`
}

// Period is one billing period row (every revision).
type Period struct {
	UUID           uuid.UUID        `json:"uuid"`
	Rental         Ref              `json:"rental"`
	RentalStatus   string           `json:"rental_status"`
	No             int32            `json:"no"`
	Revision       int32            `json:"revision"`
	StartsOn       rentaltime.Date  `json:"starts_on"`
	EndsOn         rentaltime.Date  `json:"ends_on"`
	Days           int32            `json:"days"`
	Partial        bool             `json:"partial"`
	Amount         money.Money      `json:"amount"`
	Currency       string           `json:"currency"`
	KmAllowance    *int32           `json:"km_allowance"`
	MaintenanceDue bool             `json:"maintenance_due"`
	AccrualKey     string           `json:"accrual_key"`
	Status         string           `json:"status"`
	PostedAt       *time.Time       `json:"posted_at"`
	ClosedAt       *time.Time       `json:"closed_at"`
	EntryUUID      *uuid.UUID       `json:"entry_uuid"`
	EntryDate      *rentaltime.Date `json:"entry_date"`
	DueDate        *rentaltime.Date `json:"due_date"`
}

// BillingPlan is the billing plan of the agreement's rentals.
type BillingPlan struct {
	Periods []Period    `json:"periods"`
	Planned money.Money `json:"planned"`
	Posted  money.Money `json:"posted"`
}

// StatementRow is one ledger entry of the agreement with the running
// balance.
type StatementRow struct {
	EntryUUID   uuid.UUID       `json:"entry_uuid"`
	EntryDate   rentaltime.Date `json:"entry_date"`
	DueDate     rentaltime.Date `json:"due_date"`
	Kind        string          `json:"kind"`
	Direction   string          `json:"direction"`
	Purpose     string          `json:"purpose"`
	Amount      money.Money     `json:"amount"`
	Debit       money.Money     `json:"debit"`
	Credit      money.Money     `json:"credit"`
	Balance     money.Money     `json:"balance"`
	Description string          `json:"description"`
	Reason      string          `json:"reason"`
	AccrualKey  string          `json:"accrual_key"`
	Periodic    bool            `json:"periodic"`
	Reservation Ref             `json:"reservation"`
	RentalUUID  *uuid.UUID      `json:"rental_uuid"`
}

// Statement is the agreement's statement.
type Statement struct {
	Rows    []StatementRow `json:"rows"`
	Debit   money.Money    `json:"debit"`
	Credit  money.Money    `json:"credit"`
	Balance money.Money    `json:"balance"`
}

// Facet is one value of a faceted filter.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	Status        []Facet `json:"status"`
	Customer      []Facet `json:"customer"`
	Branch        []Facet `json:"branch"`
	InvoicePeriod []Facet `json:"invoice_period"`
	Validity      []Facet `json:"validity"`
}

// ActivityEntry is one audit event of an agreement.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *Actor          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// Filter selects agreements.
type Filter struct {
	Q              string
	Statuses       []string
	CustomerUUIDs  []uuid.UUID
	BranchUUIDs    []uuid.UUID
	InvoicePeriods []string
	Validities     []string
	ValidOnFrom    *rentaltime.Date
	ValidOnTo      *rentaltime.Date
	SortField      string
	SortDesc       bool
	Limit, Offset  int32
}

// QueryKeys are the list filters (HTTP list, export).
var QueryKeys = []string{"status", "customer", "branch", "invoice_period", "validity", "valid_from", "valid_to"}

// Sortable maps the sortable list fields.
var Sortable = map[string]string{
	"code": "code", "name": "name", "customer": "customer", "status": "status",
	"valid_from": "valid_from", "valid_to": "valid_to", "created_at": "created_at",
}

// FilterFromQuery maps list query params to a filter.
func FilterFromQuery(q map[string]string) Filter {
	f := Filter{
		Q:              strings.TrimSpace(q["q"]),
		Statuses:       allowed(split(q["status"]), Statuses),
		CustomerUUIDs:  splitUUIDs(q["customer"]),
		BranchUUIDs:    splitUUIDs(q["branch"]),
		InvoicePeriods: allowed(split(q["invoice_period"]), InvoicePeriods),
		Validities:     allowed(split(q["validity"]), Validities),
	}
	if d, err := rentaltime.ParseDate(strings.TrimSpace(q["valid_from"])); err == nil {
		f.ValidOnFrom = &d
	}
	if d, err := rentaltime.ParseDate(strings.TrimSpace(q["valid_to"])); err == nil {
		f.ValidOnTo = &d
	}
	return f
}

func split(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitUUIDs(v string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, p := range split(v) {
		if id, err := uuid.Parse(p); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func allowed(v, set []string) []string {
	out := []string{}
	for _, x := range v {
		for _, s := range set {
			if x == s {
				out = append(out, x)
				break
			}
		}
	}
	return out
}
