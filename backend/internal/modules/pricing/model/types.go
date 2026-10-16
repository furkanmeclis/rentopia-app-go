// Package model holds the API shapes of the pricing resources: rate plans
// with duration bands (TEC-700), named date rules (TEC-701) and the price
// preview. Money is pkg/money (JSON decimal strings), dates are
// rentaltime.Date ("2006-01-02").
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

// Engine / activity / live resource slugs.
const (
	ResourceRatePlans = "tenant.rate_plans"
	ResourceDateRules = "tenant.date_rules"
)

// Record statuses (plan / rule rows).
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// Statuses lists the record statuses.
var Statuses = []string{StatusDraft, StatusActive, StatusInactive}

// Version statuses.
const (
	VersionDraft      = "draft"
	VersionActive     = "active"
	VersionSuperseded = "superseded"
)

// Date rule kinds and product scopes (engine constants).
var (
	RuleKinds     = []string{pricing.KindPercent, pricing.KindAmount}
	ProductScopes = []string{pricing.ProductsAll, pricing.ProductsSegments, pricing.ProductsSelected}
)

// MaxPriority bounds plan / rule priorities.
const MaxPriority = 1000

// Actor is a user embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ProductRef is a rental product embedded in a record.
type ProductRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Code  string    `json:"code"`
	Title string    `json:"title"`
}

// BranchRef is a branch embedded in a record.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// AgreementRef is a corporate agreement embedded in a record (TEC-723).
type AgreementRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// SegmentRef is a vehicle segment embedded in a record.
type SegmentRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// ------------------------------------------------------------- rate plans

// Band is one duration band (MaxDays nil = open-ended).
type Band struct {
	MinDays    int         `json:"min_days"`
	MaxDays    *int        `json:"max_days"`
	DailyPrice money.Money `json:"daily_price"`
}

// PlanVersion is one version of a rate plan.
type PlanVersion struct {
	UUID         uuid.UUID  `json:"uuid"`
	Version      int        `json:"version"`
	Status       string     `json:"status"`
	Priority     int        `json:"priority"`
	ChangeNote   string     `json:"change_note"`
	Bands        []Band     `json:"bands"`
	CreatedAt    time.Time  `json:"created_at"`
	CreatedBy    *Actor     `json:"created_by"`
	ActivatedAt  *time.Time `json:"activated_at"`
	ActivatedBy  *Actor     `json:"activated_by"`
	SupersededAt *time.Time `json:"superseded_at"`
}

// RatePlan is a rate plan. List rows carry the effective (draft, else
// active) priority, band count and lowest daily price; the detail adds the
// active and draft versions.
type RatePlan struct {
	UUID           uuid.UUID     `json:"uuid"`
	Name           string        `json:"name"`
	Scope          string        `json:"scope"`
	Product        ProductRef    `json:"rental_product"`
	Branch         *BranchRef    `json:"branch"`
	Channel        *string       `json:"channel"`
	Agreement      *AgreementRef `json:"agreement"`
	Status         string        `json:"status"`
	CurrentVersion int           `json:"current_version"`
	ActiveVersion  *int          `json:"active_version"`
	HasDraft       bool          `json:"has_draft"`
	Priority       int           `json:"priority"`
	BandCount      int64         `json:"band_count"`
	PriceFrom      *money.Money  `json:"price_from"`
	ActivatedAt    *time.Time    `json:"activated_at"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	Active         *PlanVersion  `json:"active,omitempty"`
	Draft          *PlanVersion  `json:"draft,omitempty"`
}

// BandInput is a band in a request (the price as a decimal string or
// number, validated with field details).
type BandInput struct {
	MinDays    int     `json:"min_days"`
	MaxDays    *int    `json:"max_days"`
	DailyPrice Decimal `json:"daily_price"`
}

// RatePlanCreateInput is the POST body. The plan starts as a draft
// (version 1); activation is a separate action.
type RatePlanCreateInput struct {
	RentalProductUUID *uuid.UUID `json:"rental_product_uuid"`
	Scope             string     `json:"scope"`
	BranchUUID        *uuid.UUID `json:"branch_uuid"`
	Channel           *string    `json:"channel"`
	// AgreementUUID is the corporate agreement of scope "agreement".
	AgreementUUID *uuid.UUID  `json:"agreement_uuid"`
	Name          string      `json:"name"`
	Priority      *int        `json:"priority"`
	Bands         []BandInput `json:"bands"`
	ChangeNote    string      `json:"change_note"`
}

// RatePlanPatchInput is the PATCH body: Name changes the plan; Priority,
// Bands and ChangeNote change the draft version (created from the active
// version when there is none). Product and scope target are immutable.
type RatePlanPatchInput struct {
	Name       *string      `json:"name"`
	Priority   *int         `json:"priority"`
	Bands      *[]BandInput `json:"bands"`
	ChangeNote *string      `json:"change_note"`
}

// ActivateInput is the optional body of the activate actions.
type ActivateInput struct {
	ChangeNote *string `json:"change_note"`
}

// ActivationCheck is the answer of GET …/{uuid}/activation-check: what
// activation would refuse (empty issues = ready).
type ActivationCheck struct {
	Ready  bool    `json:"ready"`
	Issues []Issue `json:"issues"`
}

// Issue is one activation problem (pricing.Issue plus the record it
// conflicts with, if any).
type Issue struct {
	Field   string         `json:"field"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Params  map[string]int `json:"params,omitempty"`
	Ref     *IssueRef      `json:"ref,omitempty"`
}

// IssueRef names a conflicting plan / rule.
type IssueRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// RatePlanFilter selects rate plans of one organization.
type RatePlanFilter struct {
	Limit, Offset  int32
	Q              string
	ProductUUIDs   []uuid.UUID
	BranchUUIDs    []uuid.UUID
	Scopes         []string
	Channels       []string
	Statuses       []string
	HasDraft       *bool
	AgreementUUIDs []uuid.UUID
	SortField      string
	SortDesc       bool
	BranchIDs      []int64
}

// RatePlanQueryKeys are the plan list filters (HTTP list, export).
var RatePlanQueryKeys = []string{"rental_product", "branch", "scope", "channel", "status", "has_draft", "agreement"}

// RatePlanFilterFromQuery maps list query params to a filter.
func RatePlanFilterFromQuery(query map[string]string) RatePlanFilter {
	f := RatePlanFilter{
		Q:              strings.TrimSpace(query["q"]),
		ProductUUIDs:   SplitUUIDs(query["rental_product"]),
		BranchUUIDs:    SplitUUIDs(query["branch"]),
		Scopes:         Allowed(SplitList(query["scope"]), pricing.Scopes),
		Channels:       SplitList(query["channel"]),
		Statuses:       Allowed(SplitList(query["status"]), Statuses),
		AgreementUUIDs: SplitUUIDs(query["agreement"]),
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["has_draft"])); err == nil {
		f.HasDraft = &v
	}
	return f
}

// RatePlanFacets are the dynamic filter options of the plan list.
type RatePlanFacets struct {
	RentalProduct []Facet `json:"rental_product"`
	Branch        []Facet `json:"branch"`
	Scope         []Facet `json:"scope"`
	Channel       []Facet `json:"channel"`
	Status        []Facet `json:"status"`
}

// ------------------------------------------------------------------ matrix

// MatrixTarget is the scope target a matrix edits: every product's plan of
// one scope / branch / channel / priority.
type MatrixTarget struct {
	Scope      string     `json:"scope"`
	BranchUUID *uuid.UUID `json:"branch_uuid"`
	Channel    *string    `json:"channel"`
	Priority   int        `json:"priority"`
}

// MatrixBand is one band column (MaxDays nil = open-ended).
type MatrixBand struct {
	MinDays int  `json:"min_days"`
	MaxDays *int `json:"max_days"`
}

// MatrixRow is one product row: Prices align with Matrix.Bands (nil = the
// product's plan has no such band). Source tells which version the prices
// come from (draft, active, or none without a plan).
type MatrixRow struct {
	Product  ProductRef     `json:"rental_product"`
	Segment  SegmentRef     `json:"segment"`
	PlanUUID *uuid.UUID     `json:"plan_uuid"`
	Status   *string        `json:"plan_status"`
	HasDraft bool           `json:"has_draft"`
	Source   *string        `json:"source"`
	Prices   []*money.Money `json:"prices"`
}

// Matrix is the product × band price grid of one scope target.
type Matrix struct {
	Scope    string       `json:"scope"`
	Branch   *BranchRef   `json:"branch"`
	Channel  *string      `json:"channel"`
	Priority int          `json:"priority"`
	Bands    []MatrixBand `json:"bands"`
	Rows     []MatrixRow  `json:"rows"`
	Drafts   int          `json:"drafts"`
}

// MatrixRowInput is one product row of a matrix save (Prices align with
// MatrixSaveInput.Bands; null / "" = no band).
type MatrixRowInput struct {
	RentalProductUUID uuid.UUID  `json:"rental_product_uuid"`
	Prices            []*Decimal `json:"prices"`
}

// MatrixSaveInput is the PUT body of the matrix: rows whose prices differ
// from the plan's current prices become drafts (plans are created as
// needed); rows without any price are left alone.
type MatrixSaveInput struct {
	MatrixTarget
	Bands      []MatrixBand     `json:"bands"`
	Rows       []MatrixRowInput `json:"rows"`
	ChangeNote string           `json:"change_note"`
}

// MatrixSaveResult reports what a matrix save did.
type MatrixSaveResult struct {
	Created   int    `json:"created"`
	Updated   int    `json:"updated"`
	Unchanged int    `json:"unchanged"`
	Matrix    Matrix `json:"matrix"`
}

// MatrixActivateResult reports a matrix activation.
type MatrixActivateResult struct {
	Activated int `json:"activated"`
}

// ------------------------------------------------------------- date rules

// RuleVersion is one version of a date rule.
type RuleVersion struct {
	UUID    uuid.UUID `json:"uuid"`
	Version int       `json:"version"`
	Status  string    `json:"status"`
	// [StartDate, EndDate): EndDate is the first day not covered.
	StartDate    rentaltime.Date `json:"start_date"`
	EndDate      rentaltime.Date `json:"end_date"`
	ProductScope string          `json:"product_scope"`
	Products     []ProductRef    `json:"rental_products"`
	Segments     []SegmentRef    `json:"segments"`
	AllBranches  bool            `json:"all_branches"`
	Branches     []BranchRef     `json:"branches"`
	// Channels nil = every channel.
	Channels     []string   `json:"channels"`
	Kind         string     `json:"kind"`
	Value        string     `json:"value"`
	Priority     int        `json:"priority"`
	ChangeNote   string     `json:"change_note"`
	CreatedAt    time.Time  `json:"created_at"`
	CreatedBy    *Actor     `json:"created_by"`
	ActivatedAt  *time.Time `json:"activated_at"`
	ActivatedBy  *Actor     `json:"activated_by"`
	SupersededAt *time.Time `json:"superseded_at"`
}

// DateRule is a named date rule; list rows carry the effective (draft,
// else active) values, the detail adds both versions.
type DateRule struct {
	UUID           uuid.UUID        `json:"uuid"`
	Name           string           `json:"name"`
	Status         string           `json:"status"`
	CurrentVersion int              `json:"current_version"`
	ActiveVersion  *int             `json:"active_version"`
	HasDraft       bool             `json:"has_draft"`
	StartDate      *rentaltime.Date `json:"start_date"`
	EndDate        *rentaltime.Date `json:"end_date"`
	Kind           string           `json:"kind"`
	Value          string           `json:"value"`
	Priority       int              `json:"priority"`
	ProductScope   string           `json:"product_scope"`
	AllBranches    bool             `json:"all_branches"`
	AllChannels    bool             `json:"all_channels"`
	ActivatedAt    *time.Time       `json:"activated_at"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	Active         *RuleVersion     `json:"active,omitempty"`
	Draft          *RuleVersion     `json:"draft,omitempty"`
}

// DateRuleFields are the versioned fields of a date rule in a request.
type DateRuleFields struct {
	StartDate    *rentaltime.Date `json:"start_date"`
	EndDate      *rentaltime.Date `json:"end_date"`
	ProductScope *string          `json:"product_scope"`
	ProductUUIDs *[]uuid.UUID     `json:"rental_product_uuids"`
	SegmentUUIDs *[]uuid.UUID     `json:"segment_uuids"`
	AllBranches  *bool            `json:"all_branches"`
	BranchUUIDs  *[]uuid.UUID     `json:"branch_uuids"`
	// AllChannels true = every channel (Channels ignored).
	AllChannels *bool     `json:"all_channels"`
	Channels    *[]string `json:"channels"`
	Kind        *string   `json:"kind"`
	Value       *Decimal  `json:"value"`
	Priority    *int      `json:"priority"`
	ChangeNote  *string   `json:"change_note"`
}

// DateRuleInput is the POST / PATCH body (PATCH: nil = unchanged; the
// versioned fields go into the draft version).
type DateRuleInput struct {
	Name *string `json:"name"`
	DateRuleFields
}

// DateRuleFilter selects date rules of one organization.
type DateRuleFilter struct {
	Limit, Offset int32
	Q             string
	Statuses      []string
	Kinds         []string
	HasDraft      *bool
	DateFrom      *rentaltime.Date
	DateTo        *rentaltime.Date
	SortField     string
	SortDesc      bool
}

// DateRuleQueryKeys are the rule list filters (HTTP list, export).
var DateRuleQueryKeys = []string{"status", "kind", "has_draft", "date_from", "date_to"}

// DateRuleFilterFromQuery maps list query params to a filter.
func DateRuleFilterFromQuery(query map[string]string) DateRuleFilter {
	f := DateRuleFilter{
		Q:        strings.TrimSpace(query["q"]),
		Statuses: Allowed(SplitList(query["status"]), Statuses),
		Kinds:    Allowed(SplitList(query["kind"]), RuleKinds),
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["has_draft"])); err == nil {
		f.HasDraft = &v
	}
	if d, err := rentaltime.ParseDate(strings.TrimSpace(query["date_from"])); err == nil {
		f.DateFrom = &d
	}
	if d, err := rentaltime.ParseDate(strings.TrimSpace(query["date_to"])); err == nil {
		f.DateTo = &d
	}
	return f
}

// DateRuleFacets are the dynamic filter options of the rule list.
type DateRuleFacets struct {
	Status []Facet `json:"status"`
	Kind   []Facet `json:"kind"`
}

// ----------------------------------------------------------------- preview

// PlanDraftInput is an unsaved rate plan for the preview (for the preview
// product).
type PlanDraftInput struct {
	Scope      string      `json:"scope"`
	BranchUUID *uuid.UUID  `json:"branch_uuid"`
	Channel    *string     `json:"channel"`
	Priority   *int        `json:"priority"`
	Bands      []BandInput `json:"bands"`
}

// RuleDraftInput is an unsaved date rule for the preview.
type RuleDraftInput struct {
	Name string `json:"name"`
	DateRuleFields
}

// DiscountInput is a campaign / coupon for the preview (TEC-703 hook).
type DiscountInput struct {
	Name  string  `json:"name"`
	Kind  string  `json:"kind"`
	Value Decimal `json:"value"`
}

// PreviewInput is the POST /v1/tenant/pricing/preview body. Current prices
// come from active plans and rules; the proposed side additionally applies
// the draft of RatePlanUUID / DateRuleUUID or the unsaved RatePlanDraft /
// DateRuleDraft.
type PreviewInput struct {
	RentalProductUUID *uuid.UUID `json:"rental_product_uuid"`
	BranchUUID        *uuid.UUID `json:"branch_uuid"`
	ReturnBranchUUID  *uuid.UUID `json:"return_branch_uuid"`
	Channel           string     `json:"channel"`
	StartAt           *time.Time `json:"start_at"`
	EndAt             *time.Time `json:"end_at"`
	DeliveryFee       *Decimal   `json:"delivery_fee"`
	// Deliveries are the service point delivery / collection fees a quote
	// resolved (TEC-724; set by reservations, not part of the HTTP body).
	Deliveries    []pricing.DeliveryFee `json:"-"`
	Discount      *DiscountInput        `json:"discount"`
	RatePlanUUID  *uuid.UUID            `json:"rate_plan_uuid"`
	RatePlanDraft *PlanDraftInput       `json:"rate_plan_draft"`
	DateRuleUUID  *uuid.UUID            `json:"date_rule_uuid"`
	DateRuleDraft *RuleDraftInput       `json:"date_rule_draft"`
	// Extras are the selected extras (TEC-702): priced per rental product
	// with the charge type, quantity bounds and line cap of the catalog.
	Extras []PreviewExtraInput `json:"extras"`
	// CouponCode is a campaign code (TEC-703); without one, the
	// repeat-customer campaigns of CustomerUUID are tried. Not combined
	// with a manual Discount.
	CouponCode string `json:"coupon_code"`
	// CustomerUUID is the customer of the sale (customer scope,
	// per-customer limits, repeat-customer campaigns). No foreign key until
	// customers exist (TEC-705).
	CustomerUUID *uuid.UUID `json:"customer_uuid"`
	// AgreementUUID prices the sale under a corporate agreement (TEC-723):
	// its tariffs beat the public plans and its coupon eligibility decides
	// whether a campaign may apply.
	AgreementUUID *uuid.UUID `json:"agreement_uuid"`
}

// PreviewExtraInput is one selected extra.
type PreviewExtraInput struct {
	ExtraUUID *uuid.UUID `json:"extra_uuid"`
	Quantity  *int       `json:"quantity"`
}

// CouponResult explains whether the coupon code (or an automatic
// campaign) applies to one side of the preview: status "applied" or
// "rejected" with the reason (not_found, inactive, not_started, expired,
// channel, scope, customer_scope, customer_required, customer_limit,
// limit, min_amount, corporate, not_eligible) and its params.
type CouponResult struct {
	Code         string            `json:"code"`
	CampaignUUID *uuid.UUID        `json:"campaign_uuid"`
	CampaignName string            `json:"campaign_name"`
	CampaignType string            `json:"campaign_type,omitempty"`
	Status       string            `json:"status"`
	Reason       string            `json:"reason"`
	Params       map[string]string `json:"params"`
}

// PreviewSide is one priced variant: a breakdown or the engine error.
type PreviewSide struct {
	Breakdown *pricing.Breakdown `json:"breakdown"`
	Error     *PreviewError      `json:"error"`
	// Coupon is the coupon / campaign decision of this side (nil = no code
	// and no automatic campaign).
	Coupon *CouponResult `json:"coupon"`
	// Agreement says which corporate agreement the side was priced under
	// and whether its own tariff applied (nil = a public sale).
	Agreement *AgreementPricing `json:"agreement,omitempty"`
}

// AgreementPricing is the agreement part of a priced side (TEC-723).
type AgreementPricing struct {
	UUID           uuid.UUID `json:"uuid"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	TariffApplied  bool      `json:"tariff_applied"`
	CouponEligible bool      `json:"coupon_eligible"`
	InvoicePeriod  string    `json:"invoice_period"`
}

// PreviewError is a pricing engine refusal (no plan, ambiguity …).
type PreviewError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Refs    []string `json:"refs"`
}

// Preview compares current and proposed prices.
type Preview struct {
	Current  PreviewSide `json:"current"`
	Proposed PreviewSide `json:"proposed"`
	// Difference is proposed total - current total (nil when either side
	// failed).
	Difference *money.Money `json:"difference"`
	Warnings   []string     `json:"warnings"`
}

// ----------------------------------------------------------------- shared

// Facet is one value of a faceted filter with its row count.
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// ActivityEntry is one audit event.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *Actor          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// Decimal is an amount / percentage in a request: a JSON string or number,
// validated by the use case (so a bad value is a field detail, not a body
// decode error).
type Decimal string

// UnmarshalJSON accepts "1300.50", "1300,50", 1300.5 or null ("").
func (d *Decimal) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*d = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*d = Decimal(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&n); err != nil {
		return errors.New("must be a number or a decimal string")
	}
	*d = Decimal(n.String())
	return nil
}

// SplitList parses a comma-separated faceted filter value.
func SplitList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// SplitUUIDs parses a comma-separated list of uuids (invalid ones dropped).
func SplitUUIDs(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range SplitList(raw) {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// Allowed keeps the values that are in allowed.
func Allowed(values, allowed []string) []string {
	out := []string{}
	for _, v := range values {
		for _, a := range allowed {
			if v == a {
				out = append(out, v)
				break
			}
		}
	}
	return out
}
