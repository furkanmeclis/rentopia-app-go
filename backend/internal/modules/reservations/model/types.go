// Package model holds the types of reservations (TEC-706), quotes
// (TEC-704) and resolution tasks (TEC-698). docs/design/reservations.md.
package model

import (
	"encoding/json"
	"strings"
	"time"

	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	pricingmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

// Engine resource slugs (activity, live lists, io / bulk engines).
const (
	ResourceReservations = "tenant.reservations"
	ResourceQuotes       = "tenant.quotes"
	ResourceTasks        = "tenant.resolution_tasks"
)

// RefType is the allocation ref_type of reservations.
const RefType = "reservation"

// Quote purposes and statuses.
const (
	PurposeNew       = "new"
	PurposeChange    = "change"
	PurposeExtension = "extension"
	PurposeUpgrade   = "upgrade"

	QuoteOpen       = "open"
	QuoteUsed       = "used"
	QuoteSuperseded = "superseded"
)

// QuotePurposes / QuoteStatuses in display order.
var (
	QuotePurposes = []string{PurposeNew, PurposeChange, PurposeExtension, PurposeUpgrade}
	QuoteStatuses = []string{QuoteOpen, QuoteUsed, QuoteSuperseded}
)

// Quote line kinds.
const (
	LineDay      = "day"
	LineExtra    = "extra"
	LineOneWay   = "one_way"
	LineDelivery = "delivery"
	LineDiscount = "discount"
)

// Collecting parties.
const (
	CollectorUs      = "us"
	CollectorChannel = "channel"
)

// Driver roles.
const (
	DriverPrimary    = "primary"
	DriverAdditional = "additional"
)

// Resolution task types and statuses.
const (
	TaskBlockConflict     = "block_conflict"
	TaskReleaseConflict   = "release_conflict"
	TaskExtensionConflict = "extension_conflict"
	TaskReplanFailed      = "replan_failed"
	TaskLateReturn        = "late_return"

	TaskOpen     = "open"
	TaskResolved = "resolved"
)

// TaskTypes / TaskStatuses in display order.
var (
	TaskTypes    = []string{TaskBlockConflict, TaskReleaseConflict, TaskExtensionConflict, TaskReplanFailed, TaskLateReturn}
	TaskStatuses = []string{TaskOpen, TaskResolved}
)

// Plate assignment kinds / sources.
const (
	AssignmentPlanned = "planned"
	AssignmentFixed   = "fixed"

	SourceEngine    = "engine"
	SourceStaff     = "staff"
	SourceDrag      = "drag"
	SourceReplan    = "replan"
	SourceUpgrade   = "upgrade"
	SourceExtension = "extension"
)

// Defaults when the settings leave a window unset (minutes).
const (
	DefaultReviewMinutes        = 60
	DefaultPaymentMinutes       = 30
	DefaultQuoteValidityMinutes = 30
)

// MaxQuoteSearchProducts bounds the products priced by one quote search.
const MaxQuoteSearchProducts = 100

// ----------------------------------------------------------------- refs

// BranchRef is a branch embedded in an answer.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// PointRef is a service point embedded in an answer.
type PointRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ProductRef is a rental product (version) embedded in an answer.
type ProductRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Code    string    `json:"code"`
	Title   string    `json:"title"`
	Version int32     `json:"version"`
}

// CustomerRef is a customer with masked contact values.
type CustomerRef struct {
	UUID           uuid.UUID `json:"uuid"`
	CustomerType   string    `json:"customer_type"`
	DisplayName    string    `json:"display_name"`
	IdentityMasked string    `json:"identity_masked"`
	PhoneMasked    string    `json:"phone_masked"`
	EmailMasked    string    `json:"email_masked"`
}

// VehicleRef is a vehicle (plate) embedded in an answer.
type VehicleRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Plate string    `json:"plate"`
	Label string    `json:"label"`
}

// Actor is a user embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ReservationRef names a reservation.
type ReservationRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
}

// --------------------------------------------------------------- quotes

// Where is the pick-up / return place and the range of a quote.
type Where struct {
	PickupBranchUUID *uuid.UUID `json:"pickup_branch_uuid"`
	PickupPointUUID  *uuid.UUID `json:"pickup_point_uuid"`
	ReturnBranchUUID *uuid.UUID `json:"return_branch_uuid"`
	ReturnPointUUID  *uuid.UUID `json:"return_point_uuid"`
	StartsAt         *time.Time `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
}

// ManualDiscountInput is a staff discount (needs tenant.discount.apply).
type ManualDiscountInput struct {
	Kind   string               `json:"kind"`
	Value  pricingmodel.Decimal `json:"value"`
	Reason string               `json:"reason"`
}

// QuoteInput is the body of POST /v1/tenant/quotes.
type QuoteInput struct {
	Where
	Channel        string               `json:"channel"`
	ProductUUID    *uuid.UUID           `json:"product_uuid"`
	CustomerUUID   *uuid.UUID           `json:"customer_uuid"`
	Extras         []ExtraRequest       `json:"extras"`
	CouponCode     string               `json:"coupon_code"`
	ManualDiscount *ManualDiscountInput `json:"manual_discount"`
	DeliveryFee    pricingmodel.Decimal `json:"delivery_fee"`
}

// QuoteSearchInput is the body of POST /v1/tenant/quotes/search: every
// product on sale (or the given ones) with availability and price.
type QuoteSearchInput struct {
	Where
	Channel      string      `json:"channel"`
	ProductUUIDs []uuid.UUID `json:"product_uuids"`
}

// PriceSummary is the price of one product in a quote search.
type PriceSummary struct {
	Currency     money.Currency `json:"currency"`
	BillingDays  int            `json:"billing_days"`
	DailyAverage money.Money    `json:"daily_average"`
	Total        money.Money    `json:"total"`
	Prepayment   money.Money    `json:"prepayment"`
}

// PriceError is a pricing engine refusal (no_plan, no_band …).
type PriceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// QuoteSearchItem is one product of a quote search.
type QuoteSearchItem struct {
	Product      availmodel.ProductRef `json:"product"`
	Availability availmodel.Breakdown  `json:"availability"`
	Sellable     int                   `json:"sellable"`
	Reasons      []string              `json:"reasons"`
	Price        *PriceSummary         `json:"price"`
	PriceError   *PriceError           `json:"price_error"`
}

// QuoteSearchResult is the answer of a quote search.
type QuoteSearchResult struct {
	Request  availmodel.ResolvedRequest `json:"request"`
	Items    []QuoteSearchItem          `json:"items"`
	Warnings []string                   `json:"warnings"`
}

// QuoteLine is one line of a quote (a billing day, an extra, a fee or the
// discount).
type QuoteLine struct {
	Position            int         `json:"position"`
	Kind                string      `json:"kind"`
	DayIndex            *int        `json:"day_index"`
	DayDate             *string     `json:"day_date"`
	DateRuleVersionUUID *uuid.UUID  `json:"date_rule_version_uuid"`
	DateRuleName        string      `json:"date_rule_name"`
	Code                string      `json:"code"`
	Name                string      `json:"name"`
	Quantity            int         `json:"quantity"`
	UnitPrice           money.Money `json:"unit_price"`
	Adjustment          money.Money `json:"adjustment"`
	Amount              money.Money `json:"amount"`
}

// Extension is the priced extra period of an extension quote. Past lines
// stay as they were sold; the added days are priced with the band of the
// NEW total length (and the date rules of each added day).
type Extension struct {
	PreviousEndsAt      time.Time         `json:"previous_ends_at"`
	EndsAt              time.Time         `json:"ends_at"`
	PreviousBillingDays int               `json:"previous_billing_days"`
	BillingDays         int               `json:"billing_days"`
	AddedDays           int               `json:"added_days"`
	Band                pricing.Band      `json:"band"`
	Days                []pricing.DayLine `json:"days"`
	RentalAmount        money.Money       `json:"rental_amount"`
	ExtrasAmount        money.Money       `json:"extras_amount"`
	Amount              money.Money       `json:"amount"`
	Base                money.Money       `json:"base"`
	VAT                 money.Money       `json:"vat"`
}

// Conditions are the rental conditions of a product version.
type Conditions struct {
	GuaranteeType    string  `json:"guarantee_type"`
	GuaranteeLabel   string  `json:"guarantee_label"`
	MinDriverAge     int     `json:"min_driver_age"`
	MinLicenseYears  int     `json:"min_license_years"`
	DepositAmount    string  `json:"deposit_amount"`
	IncludedKmPerDay *int32  `json:"included_km_per_day"`
	ExtraKmFee       string  `json:"extra_km_fee"`
	FuelPolicy       string  `json:"fuel_policy"`
	ProductUUID      string  `json:"product_uuid"`
	ProductCode      string  `json:"product_code"`
	ProductTitle     string  `json:"product_title"`
	Version          int32   `json:"version"`
	VersionUUID      *string `json:"version_uuid,omitempty"`
}

// QuoteBreakdown is the stored breakdown of a quote.
type QuoteBreakdown struct {
	// Pricing is the engine breakdown of the quoted range (an extension
	// quote prices the whole new range to pick the band; only Extension is
	// charged on top of the reservation).
	Pricing   *pricing.Breakdown `json:"pricing"`
	Extension *Extension         `json:"extension"`
	// Conditions of the quoted product version; Previous = the reservation's
	// current ones (change / upgrade quotes: the condition difference).
	Conditions         *Conditions `json:"conditions"`
	PreviousConditions *Conditions `json:"previous_conditions"`
	Warnings           []string    `json:"warnings"`
}

// ManualDiscountView is the applied staff discount.
type ManualDiscountView struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

// QuoteAvailability is the capacity answer of the quoted product.
type QuoteAvailability struct {
	Sellable  int                  `json:"sellable"`
	Breakdown availmodel.Breakdown `json:"breakdown"`
	Reasons   []string             `json:"reasons"`
}

// Quote is a quote as the API shows it.
type Quote struct {
	UUID                uuid.UUID           `json:"uuid"`
	Purpose             string              `json:"purpose"`
	Status              string              `json:"status"`
	Expired             bool                `json:"expired"`
	Reservation         *ReservationRef     `json:"reservation"`
	Customer            *CustomerRef        `json:"customer"`
	Product             ProductRef          `json:"product"`
	PickupBranch        BranchRef           `json:"pickup_branch"`
	ReturnBranch        BranchRef           `json:"return_branch"`
	PickupPoint         *PointRef           `json:"pickup_point"`
	ReturnPoint         *PointRef           `json:"return_point"`
	StartsAt            time.Time           `json:"starts_at"`
	EndsAt              time.Time           `json:"ends_at"`
	Channel             string              `json:"channel"`
	BillingDays         int                 `json:"billing_days"`
	Currency            string              `json:"currency"`
	RentalSubtotal      money.Money         `json:"rental_subtotal"`
	DiscountTotal       money.Money         `json:"discount_total"`
	RentalTotal         money.Money         `json:"rental_total"`
	ExtrasTotal         money.Money         `json:"extras_total"`
	OneWayFee           money.Money         `json:"one_way_fee"`
	DeliveryFee         money.Money         `json:"delivery_fee"`
	BaseTotal           money.Money         `json:"base_total"`
	VATTotal            money.Money         `json:"vat_total"`
	Total               money.Money         `json:"total"`
	Prepayment          money.Money         `json:"prepayment"`
	PrepaymentRule      string              `json:"prepayment_rule"`
	VATRate             *money.Rate         `json:"vat_rate"`
	Difference          *money.Money        `json:"difference"`
	RatePlanVersionUUID *uuid.UUID          `json:"rate_plan_version_uuid"`
	RatePlanVersion     *int32              `json:"rate_plan_version"`
	SettingsVersion     int32               `json:"settings_version"`
	Extras              []ExtraRequest      `json:"extras"`
	CouponCode          *string             `json:"coupon_code"`
	ManualDiscount      *ManualDiscountView `json:"manual_discount"`
	Breakdown           QuoteBreakdown      `json:"breakdown"`
	Lines               []QuoteLine         `json:"lines"`
	Vehicle             *VehicleRef         `json:"vehicle"`
	Availability        *QuoteAvailability  `json:"availability"`
	ValidUntil          time.Time           `json:"valid_until"`
	UsedAt              *time.Time          `json:"used_at"`
	CreatedBy           *Actor              `json:"created_by"`
	CreatedAt           time.Time           `json:"created_at"`
}

// QuoteListItem is a row of the quotes list.
type QuoteListItem struct {
	UUID         uuid.UUID       `json:"uuid"`
	Purpose      string          `json:"purpose"`
	Status       string          `json:"status"`
	Expired      bool            `json:"expired"`
	Product      ProductRef      `json:"product"`
	PickupBranch BranchRef       `json:"pickup_branch"`
	ReturnBranch BranchRef       `json:"return_branch"`
	Customer     *CustomerRef    `json:"customer"`
	Reservation  *ReservationRef `json:"reservation"`
	StartsAt     time.Time       `json:"starts_at"`
	EndsAt       time.Time       `json:"ends_at"`
	Channel      string          `json:"channel"`
	BillingDays  int             `json:"billing_days"`
	Currency     string          `json:"currency"`
	Total        money.Money     `json:"total"`
	Difference   *money.Money    `json:"difference"`
	ValidUntil   time.Time       `json:"valid_until"`
	CreatedAt    time.Time       `json:"created_at"`
}

// QuoteFilter selects a page of quotes.
type QuoteFilter struct {
	Purposes         []string
	Statuses         []string
	Channels         []string
	ProductUUIDs     []uuid.UUID
	ReservationUUIDs []uuid.UUID
	Q                string
	SortField        string
	SortDesc         bool
	Limit, Offset    int32
}

// QuoteSortable maps the sortable quote list fields.
var QuoteSortable = map[string]string{"total": "total", "starts_at": "starts_at", "valid_until": "valid_until", "created_at": "created_at"}

// --------------------------------------------------------- reservations

// DriverInput names a driver of the reservation.
type DriverInput struct {
	DriverUUID uuid.UUID `json:"driver_uuid"`
	Role       string    `json:"role"`
}

// OverrideInput overrides refused driver eligibility (permission
// tenant.reservations.override_eligibility, reason recorded).
type OverrideInput struct {
	Reason string `json:"reason"`
}

// CreateInput is the body of POST /v1/tenant/reservations: a reservation
// from a quote.
type CreateInput struct {
	QuoteUUID           *uuid.UUID           `json:"quote_uuid"`
	CustomerUUID        *uuid.UUID           `json:"customer_uuid"`
	InvoiceCustomerUUID *uuid.UUID           `json:"invoice_customer_uuid"`
	Drivers             []DriverInput        `json:"drivers"`
	EligibilityOverride *OverrideInput       `json:"eligibility_override"`
	ExternalReference   string               `json:"external_reference"`
	CollectingParty     string               `json:"collecting_party"`
	CommissionAmount    pricingmodel.Decimal `json:"commission_amount"`
	Note                string               `json:"note"`
	// RequestOnly records a request without capacity (status request).
	RequestOnly bool `json:"request_only"`
	// ConfirmNow confirms immediately (staff with tenant.reservations.confirm
	// at the counter), skipping the review / payment wait.
	ConfirmNow bool `json:"confirm_now"`
}

// ReasonInput is the body of actions that need a reason (cancel, no-show).
type ReasonInput struct {
	Reason string `json:"reason"`
}

// ChangeQuoteInput is the body of POST …/{uuid}/quotes: a new quote for a
// change (dates / places / product / extras) of the reservation. Missing
// fields keep the reservation's values.
type ChangeQuoteInput struct {
	Where
	ProductUUID    *uuid.UUID           `json:"product_uuid"`
	Extras         *[]ExtraRequest      `json:"extras"`
	ManualDiscount *ManualDiscountInput `json:"manual_discount"`
}

// ApplyChangeInput is the body of POST …/{uuid}/change.
type ApplyChangeInput struct {
	QuoteUUID *uuid.UUID `json:"quote_uuid"`
	Reason    string     `json:"reason"`
	// KeepPrice keeps the current total (free upgrade; needs
	// tenant.price.override).
	KeepPrice bool `json:"keep_price"`
}

// ExtendInput is the body of POST …/{uuid}/extend.
type ExtendInput struct {
	EndsAt *time.Time `json:"ends_at"`
	Reason string     `json:"reason"`
}

// PlateInput is the body of POST …/{uuid}/plate.
type PlateInput struct {
	VehicleUUID *uuid.UUID `json:"vehicle_uuid"`
	// Fix promises the plate (plate_fixed); false moves the provisional
	// plan only (calendar drag).
	Fix    bool   `json:"fix"`
	Reason string `json:"reason"`
	// Source is "staff" (default) or "drag" (calendar).
	Source string `json:"source"`
}

// DriversInput is the body of PUT …/{uuid}/drivers.
type DriversInput struct {
	Drivers             []DriverInput  `json:"drivers"`
	EligibilityOverride *OverrideInput `json:"eligibility_override"`
}

// AssignInput is the body of POST …/{uuid}/assign (staff assignment).
type AssignInput struct {
	UserUUID *uuid.UUID `json:"user_uuid"`
}

// Eligibility is the condition check of one driver.
type Eligibility struct {
	DriverUUID uuid.UUID `json:"driver_uuid"`
	Name       string    `json:"name"`
	Eligible   bool      `json:"eligible"`
	Reasons    []string  `json:"reasons"`
}

// ReservationDriver is a driver of a reservation.
type ReservationDriver struct {
	DriverUUID     *uuid.UUID `json:"driver_uuid"`
	CustomerUUID   *uuid.UUID `json:"customer_uuid"`
	Name           string     `json:"name"`
	LicenceMasked  string     `json:"licence_masked"`
	LicenceClass   string     `json:"licence_class"`
	Role           string     `json:"role"`
	Eligible       bool       `json:"eligible"`
	Reasons        []string   `json:"reasons"`
	OverrideReason string     `json:"override_reason"`
}

// VehiclePlan is the plate plan of a reservation: what was sold (product /
// guarantee), the provisional plate the engine planned and the promised
// plate (when fixed).
type VehiclePlan struct {
	SoldProduct     ProductRef   `json:"sold_product"`
	SoldConditions  Conditions   `json:"sold_conditions"`
	Product         ProductRef   `json:"product"`
	Conditions      Conditions   `json:"conditions"`
	GuaranteeType   string       `json:"guarantee_type"`
	AllocationUUID  *uuid.UUID   `json:"allocation_uuid"`
	AllocationKind  string       `json:"allocation_kind"`
	AllocationState string       `json:"allocation_status"`
	HoldExpiresAt   *time.Time   `json:"hold_expires_at"`
	PlannedVehicle  *VehicleRef  `json:"planned_vehicle"`
	AssignedVehicle *VehicleRef  `json:"assigned_vehicle"`
	PlateFixed      bool         `json:"plate_fixed"`
	History         []Assignment `json:"history"`
}

// Assignment is one entry of the plate / product assignment history.
type Assignment struct {
	Kind      string      `json:"kind"`
	Source    string      `json:"source"`
	Note      string      `json:"note"`
	Vehicle   *VehicleRef `json:"vehicle"`
	Product   ProductRef  `json:"product"`
	Actor     *Actor      `json:"actor"`
	StartedAt time.Time   `json:"started_at"`
	EndedAt   *time.Time  `json:"ended_at"`
}

// Price is the price snapshot of a reservation.
type Price struct {
	Currency      string        `json:"currency"`
	Total         money.Money   `json:"total"`
	Prepayment    money.Money   `json:"prepayment"`
	Paid          money.Money   `json:"paid"`
	Balance       money.Money   `json:"balance"`
	Commission    money.Money   `json:"commission"`
	NetReceivable money.Money   `json:"net_receivable"`
	Snapshot      PriceSnapshot `json:"snapshot"`
}

// PriceSnapshot is the immutable price record of a reservation
// (reservations.quote_snapshot): the accepted quote's breakdown and lines,
// then every extension appended (past lines are never re-priced).
type PriceSnapshot struct {
	QuoteUUID       uuid.UUID           `json:"quote_uuid"`
	Purpose         string              `json:"purpose"`
	Currency        string              `json:"currency"`
	Pricing         *pricing.Breakdown  `json:"pricing"`
	Lines           []QuoteLine         `json:"lines"`
	Extras          []ExtraRequest      `json:"extras"`
	CouponCode      *string             `json:"coupon_code"`
	ManualDiscount  *ManualDiscountView `json:"manual_discount"`
	Extensions      []Extension         `json:"extensions"`
	BaseTotal       money.Money         `json:"base_total"`
	VATTotal        money.Money         `json:"vat_total"`
	Total           money.Money         `json:"total"`
	Prepayment      money.Money         `json:"prepayment"`
	SettingsVersion int32               `json:"settings_version"`
	// KeptPrice: a change applied without charging its difference
	// (tenant.price.override).
	KeptPrice bool `json:"kept_price"`
}

// Actions are the transitions / changes the reservation allows now (the
// caller's permissions aside).
type Actions struct {
	Submit  bool `json:"submit"`
	Approve bool `json:"approve"`
	Confirm bool `json:"confirm"`
	Cancel  bool `json:"cancel"`
	NoShow  bool `json:"no_show"`
	Change  bool `json:"change"`
	Extend  bool `json:"extend"`
	Plate   bool `json:"plate"`
}

// Reservation is a reservation as the detail API shows it.
type Reservation struct {
	UUID                uuid.UUID           `json:"uuid"`
	Reference           string              `json:"reference"`
	Status              string              `json:"status"`
	PaymentStatus       string              `json:"payment_status"`
	CapacityCommitment  string              `json:"capacity_commitment"`
	PickupBranch        BranchRef           `json:"pickup_branch"`
	ReturnBranch        BranchRef           `json:"return_branch"`
	PickupPoint         *PointRef           `json:"pickup_point"`
	ReturnPoint         *PointRef           `json:"return_point"`
	StartsAt            time.Time           `json:"starts_at"`
	EndsAt              time.Time           `json:"ends_at"`
	BillingDays         int                 `json:"billing_days"`
	Customer            CustomerRef         `json:"customer"`
	InvoiceCustomer     *CustomerRef        `json:"invoice_customer"`
	Drivers             []ReservationDriver `json:"drivers"`
	Channel             string              `json:"channel"`
	ExternalReference   *string             `json:"external_reference"`
	CollectingParty     string              `json:"collecting_party"`
	Price               Price               `json:"price"`
	QuoteUUID           *uuid.UUID          `json:"quote_uuid"`
	SettingsVersion     int32               `json:"settings_version"`
	Policy              json.RawMessage     `json:"policy"`
	Plan                VehiclePlan         `json:"plan"`
	ReviewDeadline      *time.Time          `json:"review_deadline"`
	PaymentDeadline     *time.Time          `json:"payment_deadline"`
	AssignedUser        *Actor              `json:"assigned_user"`
	Note                string              `json:"note"`
	EligibilityOverride json.RawMessage     `json:"eligibility_override"`
	OpenTasks           int64               `json:"open_tasks"`
	Actions             Actions             `json:"actions"`
	ConfirmedAt         *time.Time          `json:"confirmed_at"`
	CancelledAt         *time.Time          `json:"cancelled_at"`
	CancelReason        string              `json:"cancel_reason"`
	CreatedBy           *Actor              `json:"created_by"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
}

// ListItem is a row of the reservations list.
type ListItem struct {
	UUID               uuid.UUID   `json:"uuid"`
	Reference          string      `json:"reference"`
	Status             string      `json:"status"`
	PaymentStatus      string      `json:"payment_status"`
	CapacityCommitment string      `json:"capacity_commitment"`
	PickupBranch       BranchRef   `json:"pickup_branch"`
	ReturnBranch       BranchRef   `json:"return_branch"`
	StartsAt           time.Time   `json:"starts_at"`
	EndsAt             time.Time   `json:"ends_at"`
	Product            ProductRef  `json:"product"`
	Customer           CustomerRef `json:"customer"`
	Channel            string      `json:"channel"`
	ExternalReference  *string     `json:"external_reference"`
	Currency           string      `json:"currency"`
	TotalAmount        money.Money `json:"total_amount"`
	PaidAmount         money.Money `json:"paid_amount"`
	Vehicle            *VehicleRef `json:"vehicle"`
	PlateFixed         bool        `json:"plate_fixed"`
	AssignedUser       *Actor      `json:"assigned_user"`
	OpenTasks          int64       `json:"open_tasks"`
	Deadline           *time.Time  `json:"deadline"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
}

// ListFilter selects a page of reservations.
type ListFilter struct {
	Statuses          []string
	PaymentStatuses   []string
	Channels          []string
	ProductUUIDs      []uuid.UUID
	CustomerUUIDs     []uuid.UUID
	BranchUUIDs       []uuid.UUID
	AssignedUserUUIDs []uuid.UUID
	StartsFrom        *time.Time
	StartsTo          *time.Time
	Q                 string
	SortField         string
	SortDesc          bool
	Limit, Offset     int32
}

// Sortable maps the sortable reservation list fields.
var Sortable = map[string]string{
	"reference": "reference", "status": "status", "starts_at": "starts_at", "ends_at": "ends_at",
	"total_amount": "total_amount", "created_at": "created_at",
}

// QueryKeys are the list filters (comma-separated multi values).
var QueryKeys = []string{"status", "payment_status", "channel", "product", "customer", "branch", "assigned_user", "starts_from", "starts_to"}

// FilterFromQuery maps list query params (comma-separated multi values,
// starts_from / starts_to as RFC 3339 or yyyy-MM-dd Istanbul days) to a
// filter.
func FilterFromQuery(query map[string]string) ListFilter {
	f := ListFilter{
		Q:                 strings.TrimSpace(query["q"]),
		Statuses:          pricingmodel.Allowed(pricingmodel.SplitList(query["status"]), Statuses),
		PaymentStatuses:   pricingmodel.Allowed(pricingmodel.SplitList(query["payment_status"]), PaymentStatuses),
		Channels:          pricingmodel.SplitList(query["channel"]),
		ProductUUIDs:      pricingmodel.SplitUUIDs(query["product"]),
		CustomerUUIDs:     pricingmodel.SplitUUIDs(query["customer"]),
		BranchUUIDs:       pricingmodel.SplitUUIDs(query["branch"]),
		AssignedUserUUIDs: pricingmodel.SplitUUIDs(query["assigned_user"]),
	}
	f.StartsFrom = parseInstant(query["starts_from"], false)
	f.StartsTo = parseInstant(query["starts_to"], true)
	return f
}

// parseInstant reads RFC 3339 or a yyyy-MM-dd Istanbul day (endOfDay: the
// next day's start, so the day is included).
func parseInstant(raw string, endOfDay bool) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		v := t.UTC()
		return &v
	}
	if d, err := rentaltime.ParseDate(raw); err == nil {
		if endOfDay {
			d = d.AddDays(1)
		}
		v := d.StartIn().UTC()
		return &v
	}
	return nil
}

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the list's dynamic filter options.
type Facets struct {
	Status        []Facet `json:"status"`
	PaymentStatus []Facet `json:"payment_status"`
	Channel       []Facet `json:"channel"`
	Product       []Facet `json:"product"`
	Branch        []Facet `json:"branch"`
	Customer      []Facet `json:"customer"`
}

// Event is one entry of the reservation timeline.
type Event struct {
	UUID       uuid.UUID       `json:"uuid"`
	Action     string          `json:"action"`
	FromStatus *string         `json:"from_status"`
	ToStatus   *string         `json:"to_status"`
	Reason     string          `json:"reason"`
	Data       json.RawMessage `json:"data"`
	Actor      *Actor          `json:"actor"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ChangeResult is the answer of a change / extension / plate change.
type ChangeResult struct {
	Reservation Reservation `json:"reservation"`
	// Reassigned is the number of other reservations the plan moved.
	Reassigned int `json:"reassigned"`
	// Difference is the price difference applied (nil = none).
	Difference *money.Money `json:"difference"`
	// Warnings: follow-ups that did not succeed (equipment_unavailable).
	Warnings []string `json:"warnings"`
}

// ExtensionAlternatives accompany a refused extension (RT-A31): another
// vehicle for the whole new range, or another product, each with a
// difference quote.
type ExtensionAlternatives struct {
	Reason         string               `json:"reason"`
	ExtensionQuote *Quote               `json:"extension_quote"`
	Vehicles       []VehicleAlternative `json:"vehicles"`
	Products       []ProductAlternative `json:"products"`
}

// VehicleAlternative is a vehicle that can carry the extended range.
type VehicleAlternative struct {
	Vehicle VehicleRef `json:"vehicle"`
	Quote   Quote      `json:"quote"`
}

// ProductAlternative is another sellable product for the extended range.
type ProductAlternative struct {
	Product  availmodel.ProductRef `json:"product"`
	Sellable int                   `json:"sellable"`
	Quote    Quote                 `json:"quote"`
}

// ------------------------------------------------------ resolution tasks

// Task is a resolution task as the API shows it.
type Task struct {
	UUID           uuid.UUID       `json:"uuid"`
	TaskType       string          `json:"task_type"`
	Status         string          `json:"status"`
	Reason         string          `json:"reason"`
	SourceType     string          `json:"source_type"`
	SourceUUID     *uuid.UUID      `json:"source_uuid"`
	Data           json.RawMessage `json:"data"`
	Reservation    TaskReservation `json:"reservation"`
	Assignee       *Actor          `json:"assignee"`
	ResolvedAt     *time.Time      `json:"resolved_at"`
	ResolvedBy     *Actor          `json:"resolved_by"`
	ResolutionNote string          `json:"resolution_note"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// TaskReservation is the reservation of a task.
type TaskReservation struct {
	UUID         uuid.UUID  `json:"uuid"`
	Reference    string     `json:"reference"`
	Status       string     `json:"status"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       time.Time  `json:"ends_at"`
	PickupBranch *BranchRef `json:"pickup_branch"`
}

// TaskFilter selects a page of tasks.
type TaskFilter struct {
	Statuses         []string
	TaskTypes        []string
	ReservationUUIDs []uuid.UUID
	AssigneeUUIDs    []uuid.UUID
	Q                string
	SortField        string
	SortDesc         bool
	Limit, Offset    int32
}

// TaskSortable maps the sortable task list fields.
var TaskSortable = map[string]string{"created_at": "created_at", "starts_at": "starts_at"}

// ResolveInput is the body of POST /v1/tenant/resolution-tasks/{uuid}/resolve.
type ResolveInput struct {
	Note string `json:"note"`
}

// TaskReplanResult is the answer of POST …/resolution-tasks/{uuid}/replan.
type TaskReplanResult struct {
	Task       Task        `json:"task"`
	Replanned  bool        `json:"replanned"`
	Vehicle    *VehicleRef `json:"vehicle"`
	Reassigned int         `json:"reassigned"`
}
