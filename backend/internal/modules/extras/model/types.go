// Package model holds the types of the extras resource (TEC-702): the
// catalog, effective-dated per-product prices, branch stock of physical
// equipment and its timed allocations. docs/design/extras.md.
package model

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Engine resource slugs (activity, live lists, io, bulk).
const (
	Resource            = "tenant.extras"
	ResourceAllocations = "tenant.equipment_allocations"
)

// Extra types.
const (
	TypeAdditionalDriver = "additional_driver"
	TypeChildSeat        = "child_seat"
	TypeKmPackage        = "km_package"
	TypeDelivery         = "delivery"
	TypeOther            = "other"
)

// Types lists the extra types in display order.
var Types = []string{TypeAdditionalDriver, TypeChildSeat, TypeKmPackage, TypeDelivery, TypeOther}

// Charge types.
const (
	ChargePerDay    = "per_day"
	ChargePerUnit   = "per_unit"
	ChargePerRental = "per_rental"
)

// ChargeTypes lists the charge types.
var ChargeTypes = []string{ChargePerDay, ChargePerUnit, ChargePerRental}

// Publish statuses.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

// Statuses lists the publish statuses.
var Statuses = []string{StatusDraft, StatusPublished, StatusArchived}

// Allocation kinds and statuses (same vocabulary as vehicle allocations).
const (
	KindHold        = "hold"
	KindReservation = "reservation"
	KindRental      = "rental"

	AllocActive   = "active"
	AllocReleased = "released"
	AllocExpired  = "expired"
)

// AllocationKinds / AllocationStatuses in display order.
var (
	AllocationKinds    = []string{KindHold, KindReservation, KindRental}
	AllocationStatuses = []string{AllocActive, AllocReleased, AllocExpired}
)

// Limits.
const (
	MaxQuantity        = 100
	MaxStock           = 100000
	DefaultHoldMinutes = 30
	MaxHoldMinutes     = 1440
	MaxRangeDays       = 365
)

// Extra is one catalog entry.
type Extra struct {
	UUID        uuid.UUID    `json:"uuid"`
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	ExtraType   string       `json:"extra_type"`
	ChargeType  string       `json:"charge_type"`
	MinQuantity int          `json:"min_quantity"`
	MaxQuantity int          `json:"max_quantity"`
	MaxTotal    *money.Money `json:"max_total"`
	// Channels nil = every sales channel.
	Channels   []string  `json:"channels"`
	Status     string    `json:"status"`
	IsPhysical bool      `json:"is_physical"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// List extras.
	DefaultPrice      *money.Money `json:"default_price"`
	ProductPriceCount int64        `json:"product_price_count"`
	StockTotal        int64        `json:"stock_total"`
}

// ExtraInput is the create / patch body (nil = unchanged on patch).
type ExtraInput struct {
	Code        *string `json:"code"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	ExtraType   *string `json:"extra_type"`
	ChargeType  *string `json:"charge_type"`
	MinQuantity *int    `json:"min_quantity"`
	MaxQuantity *int    `json:"max_quantity"`
	// MaxTotal "" or null clears the cap.
	MaxTotal OptionalDecimal `json:"max_total"`
	// Channels null = every channel.
	Channels   OptionalStrings `json:"channels"`
	Status     *string         `json:"status"`
	IsPhysical *bool           `json:"is_physical"`
	SortOrder  *int            `json:"sort_order"`
	// DefaultPrice sets the default price on create (optional).
	DefaultPrice *Decimal `json:"default_price"`
}

// ListFilter is the extras list query.
type ListFilter struct {
	Limit, Offset int32
	Q             string
	Types         []string
	ChargeTypes   []string
	Statuses      []string
	IsPhysical    *bool
	Channel       string
	SortField     string
	SortDesc      bool
}

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the list.
type Facets struct {
	ExtraType  []Facet `json:"extra_type"`
	ChargeType []Facet `json:"charge_type"`
	Status     []Facet `json:"status"`
	IsPhysical []Facet `json:"is_physical"`
}

// ------------------------------------------------------------------ prices

// PriceRow is one effective-dated price row.
type PriceRow struct {
	UUID      uuid.UUID   `json:"uuid"`
	Price     money.Money `json:"price"`
	ValidFrom time.Time   `json:"valid_from"`
	ValidTo   *time.Time  `json:"valid_to"`
}

// ProductRef is a rental product embedded in an answer.
type ProductRef struct {
	UUID        uuid.UUID `json:"uuid"`
	Code        string    `json:"code"`
	Title       string    `json:"title"`
	Status      string    `json:"status,omitempty"`
	SegmentUUID uuid.UUID `json:"segment_uuid,omitempty"`
	SegmentName string    `json:"segment_name,omitempty"`
}

// ProductPrice is one row of the price matrix: the product's own price
// (nil = falls back to the default) and the price that applies.
type ProductPrice struct {
	Product   ProductRef   `json:"product"`
	Price     *PriceRow    `json:"price"`
	Effective *money.Money `json:"effective"`
	// Source is "product", "default" or "none" (no price: not sellable).
	Source string `json:"source"`
}

// ScheduledPrice is a price row that starts in the future.
type ScheduledPrice struct {
	UUID        uuid.UUID   `json:"uuid"`
	ProductUUID *uuid.UUID  `json:"product_uuid"`
	Price       money.Money `json:"price"`
	ValidFrom   time.Time   `json:"valid_from"`
	ValidTo     *time.Time  `json:"valid_to"`
}

// PriceMatrix is GET /v1/tenant/extras/{uuid}/prices.
type PriceMatrix struct {
	Default   *PriceRow        `json:"default"`
	Items     []ProductPrice   `json:"items"`
	Scheduled []ScheduledPrice `json:"scheduled"`
}

// PriceItemInput sets (price) or removes (price null) the price of one
// product.
type PriceItemInput struct {
	RentalProductUUID *uuid.UUID `json:"rental_product_uuid"`
	Price             *Decimal   `json:"price"`
}

// PricesInput is PUT /v1/tenant/extras/{uuid}/prices. Only the listed
// rows change. EffectiveFrom nil = now.
type PricesInput struct {
	DefaultPrice  OptionalDecimal  `json:"default_price"`
	Items         []PriceItemInput `json:"items"`
	EffectiveFrom *time.Time       `json:"effective_from"`
}

// PriceHistoryEntry is one row of the price history.
type PriceHistoryEntry struct {
	UUID      uuid.UUID   `json:"uuid"`
	Product   *ProductRef `json:"product"`
	Price     money.Money `json:"price"`
	ValidFrom time.Time   `json:"valid_from"`
	ValidTo   *time.Time  `json:"valid_to"`
	CreatedAt time.Time   `json:"created_at"`
	Actor     *Actor      `json:"actor"`
}

// ------------------------------------------------------------------- stock

// BranchRef is a branch embedded in an answer.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// StockRow is the stock of a physical extra at one branch.
type StockRow struct {
	Branch       BranchRef  `json:"branch"`
	BranchActive bool       `json:"branch_active"`
	Quantity     int        `json:"quantity"`
	UpdatedAt    *time.Time `json:"updated_at"`
	// InUseNow is the units busy right now (active, unexpired).
	InUseNow int `json:"in_use_now"`
	// PeakCommitted is the highest concurrent use of future commitments.
	PeakCommitted int `json:"peak_committed"`
}

// StockItemInput sets the count of one branch.
type StockItemInput struct {
	BranchUUID *uuid.UUID `json:"branch_uuid"`
	Quantity   *int       `json:"quantity"`
}

// StockInput is PUT /v1/tenant/extras/{uuid}/stock.
type StockInput struct {
	Items []StockItemInput `json:"items"`
}

// StockResult answers a stock write: the rows and the warnings (a count
// below what is already committed is recorded, not refused).
type StockResult struct {
	Items    []StockRow     `json:"items"`
	Warnings []StockWarning `json:"warnings"`
}

// StockWarning reports a branch whose new count is below its committed
// peak.
type StockWarning struct {
	BranchUUID    uuid.UUID `json:"branch_uuid"`
	Code          string    `json:"code"`
	Quantity      int       `json:"quantity"`
	PeakCommitted int       `json:"peak_committed"`
}

// ------------------------------------------------------------- allocations

// Actor is the user who made a change.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ExtraRef is an extra embedded in an answer.
type ExtraRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// Allocation is one equipment allocation.
type Allocation struct {
	UUID     uuid.UUID `json:"uuid"`
	Extra    ExtraRef  `json:"extra"`
	Branch   BranchRef `json:"branch"`
	Quantity int       `json:"quantity"`
	Kind     string    `json:"kind"`
	Status   string    `json:"status"`
	// Lapsed is true for an active hold past expires_at (no longer counts;
	// the sweep marks it expired).
	Lapsed                bool       `json:"lapsed"`
	StartsAt              time.Time  `json:"starts_at"`
	EndsAt                time.Time  `json:"ends_at"`
	ExpiresAt             *time.Time `json:"expires_at"`
	VehicleAllocationUUID *uuid.UUID `json:"vehicle_allocation_uuid"`
	RefType               *string    `json:"ref_type"`
	RefUUID               *uuid.UUID `json:"ref_uuid"`
	Note                  string     `json:"note"`
	ReleasedAt            *time.Time `json:"released_at"`
	ReleaseReason         string     `json:"release_reason"`
	CreatedBy             *Actor     `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// AllocationFilter is the allocations list query.
type AllocationFilter struct {
	Limit, Offset int32
	Q             string
	ExtraUUID     *uuid.UUID
	BranchUUIDs   []uuid.UUID
	Statuses      []string
	Kinds         []string
	SortField     string
	SortDesc      bool
}

// ReserveInput is ReserveEquipment (POST /v1/tenant/equipment-allocations).
//
// With VehicleAllocationUUID the equipment belongs to that availability
// hold / reservation: kind, expiry and (when omitted) branch and range
// come from it, and it is released / confirmed together with it.
type ReserveInput struct {
	ExtraUUID             *uuid.UUID `json:"extra_uuid"`
	BranchUUID            *uuid.UUID `json:"branch_uuid"`
	Quantity              *int       `json:"quantity"`
	StartsAt              *time.Time `json:"starts_at"`
	EndsAt                *time.Time `json:"ends_at"`
	Kind                  string     `json:"kind"`
	HoldMinutes           *int       `json:"hold_minutes"`
	VehicleAllocationUUID *uuid.UUID `json:"vehicle_allocation_uuid"`
	RefType               string     `json:"ref_type"`
	RefUUID               *uuid.UUID `json:"ref_uuid"`
	Note                  string     `json:"note"`
}

// ConfirmInput is POST /v1/tenant/equipment-allocations/{uuid}/confirm.
type ConfirmInput struct {
	RefType string     `json:"ref_type"`
	RefUUID *uuid.UUID `json:"ref_uuid"`
}

// AvailabilityInput is POST /v1/tenant/extras/availability.
type AvailabilityInput struct {
	BranchUUID *uuid.UUID  `json:"branch_uuid"`
	StartsAt   *time.Time  `json:"starts_at"`
	EndsAt     *time.Time  `json:"ends_at"`
	ExtraUUIDs []uuid.UUID `json:"extra_uuids"`
}

// EquipmentAvailability is the answer per physical extra.
type EquipmentAvailability struct {
	Extra    ExtraRef `json:"extra"`
	Stock    int      `json:"stock"`
	Reserved int      `json:"reserved"`
	Held     int      `json:"held"`
	// Available = stock - peak concurrent use inside the range (>= 0).
	Available int `json:"available"`
}

// AvailabilityResult is POST /v1/tenant/extras/availability.
type AvailabilityResult struct {
	Branch BranchRef               `json:"branch"`
	Items  []EquipmentAvailability `json:"items"`
}

// ActivityEntry is one audit event.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *Actor          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// ------------------------------------------------------------- resolver

// Selection is one extra chosen for a quote.
type Selection struct {
	ExtraUUID *uuid.UUID `json:"extra_uuid"`
	Quantity  *int       `json:"quantity"`
}

// ------------------------------------------------------------ decoding

// Decimal is an amount in a request: a JSON string or number, validated
// by the use case.
type Decimal string

// UnmarshalJSON accepts "150.50", "150,50", 150.5 or null ("").
func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*d = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*d = Decimal(strings.TrimSpace(v))
		return nil
	}
	*d = Decimal(s)
	return nil
}

// OptionalDecimal distinguishes "absent" from null / "" (clear).
type OptionalDecimal struct {
	Set   bool
	Value Decimal
}

// UnmarshalJSON marks the field as present.
func (o *OptionalDecimal) UnmarshalJSON(b []byte) error {
	o.Set = true
	return o.Value.UnmarshalJSON(b)
}

// OptionalStrings distinguishes "absent" from null (every channel).
type OptionalStrings struct {
	Set   bool
	Value []string
}

// UnmarshalJSON marks the field as present; null keeps Value nil.
func (o *OptionalStrings) UnmarshalJSON(b []byte) error {
	o.Set = true
	if strings.TrimSpace(string(b)) == "null" {
		o.Value = nil
		return nil
	}
	var v []string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v == nil {
		v = []string{}
	}
	o.Value = v
	return nil
}

// QueryKeys are the list filters an export honours.
var QueryKeys = []string{"q", "extra_type", "charge_type", "status", "is_physical", "channel"}

// FilterFromQuery maps list query params to a filter (engine adapters).
func FilterFromQuery(query map[string]string) ListFilter {
	f := ListFilter{
		Q: strings.TrimSpace(query["q"]), Types: SplitList(query["extra_type"]),
		ChargeTypes: SplitList(query["charge_type"]), Statuses: SplitList(query["status"]),
		Channel: strings.TrimSpace(query["channel"]),
	}
	switch strings.TrimSpace(query["is_physical"]) {
	case "true":
		v := true
		f.IsPhysical = &v
	case "false":
		v := false
		f.IsPhysical = &v
	}
	return f
}

// SplitList parses a comma-separated multi-value filter.
func SplitList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
