// Package model holds the types of the campaigns resource (TEC-703):
// coupon codes and repeat-customer campaigns, their usage counters and
// redemptions. docs/design/campaigns.md.
package model

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

// Resource is the engine slug (activity, live lists, io, bulk).
const Resource = "tenant.campaigns"

// Campaign types.
const (
	TypeCoupon         = "coupon"
	TypeRepeatCustomer = "repeat_customer"
)

// Types lists the campaign types.
var Types = []string{TypeCoupon, TypeRepeatCustomer}

// Discount kinds.
const (
	KindPercent = "percent"
	KindAmount  = "amount"
)

// Kinds lists the discount kinds.
var Kinds = []string{KindPercent, KindAmount}

// Product scopes.
const (
	ScopeAll      = "all"
	ScopeSegments = "segments"
	ScopeProducts = "products"
)

// Scopes lists the product scopes.
var Scopes = []string{ScopeAll, ScopeSegments, ScopeProducts}

// Computed states (lists, facets).
const (
	StateActive    = "active"
	StateScheduled = "scheduled"
	StateExpired   = "expired"
	StateExhausted = "exhausted"
	StateInactive  = "inactive"
)

// States lists the computed states.
var States = []string{StateActive, StateScheduled, StateExhausted, StateExpired, StateInactive}

// Redemption statuses.
const (
	RedemptionReserved = "reserved"
	RedemptionConsumed = "consumed"
	RedemptionReleased = "released"
)

// RedemptionStatuses lists the redemption statuses.
var RedemptionStatuses = []string{RedemptionReserved, RedemptionConsumed, RedemptionReleased}

// Rejection reasons: why a code / campaign does not apply. The preview
// shows them; Redeem returns them as *RejectionError.
const (
	ReasonNotFound         = "not_found"
	ReasonInactive         = "inactive"
	ReasonNotStarted       = "not_started"
	ReasonExpired          = "expired"
	ReasonChannel          = "channel"
	ReasonScope            = "scope"
	ReasonCustomerScope    = "customer_scope"
	ReasonCustomerRequired = "customer_required"
	ReasonCustomerLimit    = "customer_limit"
	ReasonLimit            = "limit"
	ReasonMinAmount        = "min_amount"
	ReasonCorporate        = "corporate"
	ReasonNotEligible      = "not_eligible"
)

// ProductRef is a rental product target.
type ProductRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Code  string    `json:"code"`
	Title string    `json:"title"`
}

// SegmentRef is a segment target.
type SegmentRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// Campaign is one campaign / coupon.
type Campaign struct {
	UUID          uuid.UUID        `json:"uuid"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	CampaignType  string           `json:"campaign_type"`
	Code          *string          `json:"code"`
	DiscountKind  string           `json:"discount_kind"`
	DiscountValue money.Money      `json:"discount_value"`
	MaxDiscount   *money.Money     `json:"max_discount"`
	MinSubtotal   *money.Money     `json:"min_subtotal"`
	StartDate     *rentaltime.Date `json:"start_date"`
	EndDate       *rentaltime.Date `json:"end_date"`
	// Channels nil = every sales channel.
	Channels     []string     `json:"channels"`
	ProductScope string       `json:"product_scope"`
	Products     []ProductRef `json:"products"`
	Segments     []SegmentRef `json:"segments"`
	// CustomerUUIDs nil = every customer.
	CustomerUUIDs       []uuid.UUID `json:"customer_uuids"`
	UsageLimit          *int        `json:"usage_limit"`
	PerCustomerLimit    *int        `json:"per_customer_limit"`
	UsedCount           int         `json:"used_count"`
	Remaining           *int        `json:"remaining"`
	MinCompletedRentals int         `json:"min_completed_rentals"`
	ReleaseOnCancel     bool        `json:"release_on_cancel"`
	CorporateEligible   bool        `json:"corporate_eligible"`
	IsActive            bool        `json:"is_active"`
	State               string      `json:"state"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
}

// CampaignInput is the create / patch body (absent = unchanged on patch).
type CampaignInput struct {
	Name                *string               `json:"name"`
	Description         *string               `json:"description"`
	CampaignType        *string               `json:"campaign_type"`
	Code                Optional[string]      `json:"code"`
	DiscountKind        *string               `json:"discount_kind"`
	DiscountValue       *Decimal              `json:"discount_value"`
	MaxDiscount         Optional[Decimal]     `json:"max_discount"`
	MinSubtotal         Optional[Decimal]     `json:"min_subtotal"`
	StartDate           Optional[string]      `json:"start_date"`
	EndDate             Optional[string]      `json:"end_date"`
	Channels            Optional[[]string]    `json:"channels"`
	ProductScope        *string               `json:"product_scope"`
	RentalProductUUIDs  *[]uuid.UUID          `json:"rental_product_uuids"`
	SegmentUUIDs        *[]uuid.UUID          `json:"segment_uuids"`
	CustomerUUIDs       Optional[[]uuid.UUID] `json:"customer_uuids"`
	UsageLimit          Optional[int]         `json:"usage_limit"`
	PerCustomerLimit    Optional[int]         `json:"per_customer_limit"`
	MinCompletedRentals *int                  `json:"min_completed_rentals"`
	ReleaseOnCancel     *bool                 `json:"release_on_cancel"`
	CorporateEligible   *bool                 `json:"corporate_eligible"`
	IsActive            *bool                 `json:"is_active"`
}

// ListFilter is the campaigns list query.
type ListFilter struct {
	Limit, Offset int32
	Q             string
	Types         []string
	Kinds         []string
	States        []string
	Channel       string
	SortField     string
	SortDesc      bool
}

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options.
type Facets struct {
	CampaignType []Facet `json:"campaign_type"`
	DiscountKind []Facet `json:"discount_kind"`
	State        []Facet `json:"state"`
}

// Actor is the user who made a change.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Redemption is one use of a campaign by a reservation.
type Redemption struct {
	UUID            uuid.UUID   `json:"uuid"`
	ReservationUUID uuid.UUID   `json:"reservation_uuid"`
	CustomerUUID    *uuid.UUID  `json:"customer_uuid"`
	Code            *string     `json:"code"`
	Subtotal        money.Money `json:"subtotal"`
	DiscountAmount  money.Money `json:"discount_amount"`
	Status          string      `json:"status"`
	ReleaseReason   string      `json:"release_reason"`
	UsageReturned   bool        `json:"usage_returned"`
	CreatedBy       *Actor      `json:"created_by"`
	CreatedAt       time.Time   `json:"created_at"`
	ConsumedAt      *time.Time  `json:"consumed_at"`
	ReleasedAt      *time.Time  `json:"released_at"`
}

// ActivityEntry is one audit event.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *Actor          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// CouponResult explains whether a code / automatic campaign applies to a
// quote (preview).
type CouponResult struct {
	Code         string     `json:"code"`
	CampaignUUID *uuid.UUID `json:"campaign_uuid"`
	CampaignName string     `json:"campaign_name"`
	CampaignType string     `json:"campaign_type,omitempty"`
	// Status is "applied" or "rejected".
	Status string `json:"status"`
	// Reason is the rejection reason (ReasonX), "" when applied.
	Reason string `json:"reason"`
	// Params carry the numbers of a rejection (min_subtotal …).
	Params map[string]string `json:"params"`
}

// ------------------------------------------------------------ decoding

// Decimal is an amount / percentage in a request: a JSON string or number,
// validated by the use case.
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

// Optional distinguishes an absent field from null (Value nil) and a
// value.
type Optional[T any] struct {
	Set   bool
	Value *T
}

// UnmarshalJSON marks the field as present.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if strings.TrimSpace(string(b)) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// QueryKeys are the list filters an export honours.
var QueryKeys = []string{"q", "campaign_type", "discount_kind", "state", "channel"}

// FilterFromQuery maps list query params to a filter (engine adapters).
func FilterFromQuery(query map[string]string) ListFilter {
	return ListFilter{
		Q: strings.TrimSpace(query["q"]), Types: SplitList(query["campaign_type"]),
		Kinds: SplitList(query["discount_kind"]), States: SplitList(query["state"]),
		Channel: strings.TrimSpace(query["channel"]),
	}
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
