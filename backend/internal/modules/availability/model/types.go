// Package model holds the types of the availability engine (TEC-697):
// capacity search, allocations (holds / reservation plate plans) and
// channel capacity rules. docs/design/availability-engine.md.
package model

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/engine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Engine resource slugs (activity, live lists).
const (
	ResourceHolds         = "tenant.availability_holds"
	ResourceCapacityRules = "tenant.capacity_rules"
)

// Allocation kinds.
const (
	KindHold        = "hold"
	KindReservation = "reservation"
	KindRental      = "rental"
)

// Allocation statuses.
const (
	StatusActive   = "active"
	StatusReleased = "released"
	StatusExpired  = "expired"
)

// Allocation kinds / statuses in display order.
var (
	Kinds    = []string{KindHold, KindReservation, KindRental}
	Statuses = []string{StatusActive, StatusReleased, StatusExpired}
)

// Channel capacity rule types.
const (
	RuleSalesLimit      = "sales_limit"
	RuleGuaranteedQuota = "guaranteed_quota"
	RuleSafetyMargin    = "safety_margin"
)

// RuleTypes lists the rule types.
var RuleTypes = []string{RuleSalesLimit, RuleGuaranteedQuota, RuleSafetyMargin}

// Capacity failure reasons (CAPACITY_UNAVAILABLE detail code).
const (
	ReasonNoEligibleVehicle = "no_eligible_vehicle"
	ReasonNoVehicle         = "no_vehicle"
	ReasonOneWayNotAllowed  = "one_way_not_allowed"
	ReasonChannelRules      = "channel_rules"
	ReasonNotSellable       = "product_not_sellable"
)

// Hold limits (minutes).
const (
	DefaultHoldMinutes = 30
	MaxHoldMinutes     = 1440
)

// MaxRangeDays is the longest rental range the engine plans.
const MaxRangeDays = 365

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

// ProductRef is a rental product embedded in an answer.
type ProductRef struct {
	UUID          uuid.UUID `json:"uuid"`
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	GuaranteeType string    `json:"guarantee_type,omitempty"`
	Version       int32     `json:"version"`
}

// VehicleRef is the (provisional or fixed) plate of an allocation.
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

// Where is the pick-up / return place of a request: a branch, or a service
// point of a branch (same stock as the branch). Return defaults to pick-up.
type Where struct {
	PickupBranchUUID *uuid.UUID `json:"pickup_branch_uuid"`
	PickupPointUUID  *uuid.UUID `json:"pickup_point_uuid"`
	ReturnBranchUUID *uuid.UUID `json:"return_branch_uuid"`
	ReturnPointUUID  *uuid.UUID `json:"return_point_uuid"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	// Channel is a sales channel code (orgsettings); default "branch".
	Channel string `json:"channel"`
	// FleetPool restricts the vehicles to one pool ("" = any).
	FleetPool string `json:"fleet_pool"`
}

// SearchInput is the body of POST /v1/tenant/availability/search.
type SearchInput struct {
	Where
	// ProductUUIDs limits the answer (empty = every product on sale).
	ProductUUIDs []uuid.UUID `json:"product_uuids"`
}

// ResolvedRequest echoes what the engine searched for.
type ResolvedRequest struct {
	PickupBranch BranchRef `json:"pickup_branch"`
	ReturnBranch BranchRef `json:"return_branch"`
	PickupPoint  *PointRef `json:"pickup_point"`
	ReturnPoint  *PointRef `json:"return_point"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	// ReadyAt = EndsAt + PrepMinutes (preparation of the return branch).
	ReadyAt     time.Time `json:"ready_at"`
	PrepMinutes int       `json:"prep_minutes"`
	Channel     string    `json:"channel"`
	FleetPool   string    `json:"fleet_pool"`
}

// Breakdown explains a capacity number: the vehicle buckets of the engine
// plus the channel rules.
type Breakdown struct {
	engine.Breakdown
	// SafetyMargin units are never sold (most specific margin rule).
	SafetyMargin int `json:"safety_margin"`
	// ReservedForOtherChannels: unused guaranteed quotas of other channels
	// (scaled down when they exceed the physical capacity).
	ReservedForOtherChannels int `json:"reserved_for_other_channels"`
	// OwnQuotaRemaining: unused guaranteed quota of the channel (nil = none).
	OwnQuotaRemaining *int `json:"own_quota_remaining"`
	// ChannelLimitRemaining: what the channel's sales limit still allows
	// (nil = no limit).
	ChannelLimitRemaining *int `json:"channel_limit_remaining"`
	// Sellable is what the channel may sell now for this exact request.
	Sellable int `json:"sellable"`
}

// ProductAvailability is one product of a search answer.
type ProductAvailability struct {
	Product   ProductRef `json:"product"`
	Breakdown Breakdown  `json:"breakdown"`
	Sellable  int        `json:"sellable"`
	// Reasons explain a zero (one_way_not_allowed, no_eligible_vehicle).
	Reasons []string `json:"reasons"`
}

// SearchResult is the answer of a search.
type SearchResult struct {
	Request ResolvedRequest       `json:"request"`
	Items   []ProductAvailability `json:"items"`
}

// HoldInput is the body of POST /v1/tenant/availability/holds.
type HoldInput struct {
	Where
	ProductUUID uuid.UUID `json:"product_uuid"`
	// HoldMinutes is the option time (default: organization payment hold
	// minutes, else 30; at most 1440).
	HoldMinutes *int   `json:"hold_minutes"`
	Note        string `json:"note"`
	// VehicleUUID promises a plate (fixed; never reassigned).
	VehicleUUID *uuid.UUID `json:"vehicle_uuid"`
}

// ReserveInput is the internal API input (holds and reservations).
type ReserveInput struct {
	Where
	ProductUUID uuid.UUID
	Kind        string // hold | reservation
	HoldMinutes *int
	VehicleUUID *uuid.UUID
	Note        string
	RefType     string
	RefUUID     *uuid.UUID
	ActorID     *int64
	// HoldUntil sets the expiry of a hold directly (reservation review /
	// payment windows may exceed MaxHoldMinutes); it wins over HoldMinutes.
	HoldUntil *time.Time
	// Hook runs inside the write transaction (nil = none).
	Hook TxHook
}

// ExtendInput is the body of POST …/holds/{uuid}/extend.
type ExtendInput struct {
	Minutes int `json:"minutes"`
}

// ChangeInput moves an allocation to a new range (and optionally new
// places); nil places keep the current ones.
type ChangeInput struct {
	Where
	// VehicleUUID / FixPlate move the allocation onto one vehicle in the
	// same atomic re-plan (internal callers; not part of an HTTP body).
	VehicleUUID *uuid.UUID `json:"-"`
	FixPlate    bool       `json:"-"`
	// Hook runs inside the write transaction (internal callers).
	Hook TxHook `json:"-"`
}

// TxHook runs inside the engine's write transaction, after the allocation
// row (and every reassignment of the plan) was written. Callers that keep
// their own record in step with the allocation (reservations, TEC-706)
// write it here, so the plan and the record commit or roll back together.
// An error rolls the whole transaction back. The hook may run more than
// once (the engine retries stale plans): it must only write through tx.
type TxHook func(ctx context.Context, tx pgx.Tx, r HookResult) error

// HookResult describes the allocation a write produced.
type HookResult struct {
	AllocationID   int64
	AllocationUUID uuid.UUID
	VehicleID      int64
	ProductID      int64
	ProductVersion int32
	Kind           string
	PlateFixed     bool
	// Moved are the other allocations the plan moved to another vehicle
	// (reassignments and re-planned dependents).
	Moved []MovedAllocation
	// Affected lists what a release could not keep valid.
	Affected []Affected
}

// MovedAllocation is one reassignment of a plan.
type MovedAllocation struct {
	ID            int64
	UUID          uuid.UUID
	FromVehicleID int64
	ToVehicleID   int64
}

// PlanOptions tune a re-plan of an existing allocation.
type PlanOptions struct {
	// VehicleUUID restricts the plan to one vehicle (plate change).
	VehicleUUID *uuid.UUID
	// FixPlate promises the plan's vehicle (plate_fixed).
	FixPlate bool
	Hook     TxHook
}

// Allocation is an allocation (hold / reservation plate plan) as the API
// shows it.
type Allocation struct {
	UUID          uuid.UUID  `json:"uuid"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Product       ProductRef `json:"product"`
	Vehicle       VehicleRef `json:"vehicle"`
	PlateFixed    bool       `json:"plate_fixed"`
	PickupBranch  BranchRef  `json:"pickup_branch"`
	ReturnBranch  BranchRef  `json:"return_branch"`
	PickupPoint   *PointRef  `json:"pickup_point"`
	ReturnPoint   *PointRef  `json:"return_point"`
	StartsAt      time.Time  `json:"starts_at"`
	EndsAt        time.Time  `json:"ends_at"`
	ReadyAt       time.Time  `json:"ready_at"`
	PrepMinutes   int32      `json:"prep_minutes"`
	ExpiresAt     *time.Time `json:"expires_at"`
	Channel       string     `json:"channel"`
	FleetPool     string     `json:"fleet_pool"`
	Note          string     `json:"note"`
	RefType       string     `json:"ref_type"`
	RefUUID       *uuid.UUID `json:"ref_uuid"`
	ReleasedAt    *time.Time `json:"released_at"`
	ReleaseReason string     `json:"release_reason"`
	CreatedBy     *Actor     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// PlanResult is the answer of Reserve / ChangeDates / AssignPlate: the
// allocation and how many other commitments the plan moved.
type PlanResult struct {
	Allocation Allocation `json:"allocation"`
	Reassigned int        `json:"reassigned"`
}

// Affected is an item a release could not keep valid (a later commitment
// that relied on the released one's return branch, or a planned transfer).
type Affected struct {
	Kind string    `json:"kind"`
	UUID uuid.UUID `json:"uuid"`
}

// ReleaseResult is the answer of Release.
type ReleaseResult struct {
	Allocation Allocation `json:"allocation"`
	Reassigned int        `json:"reassigned"`
	Affected   []Affected `json:"affected"`
}

// AllocationFilter selects a page of allocations.
type AllocationFilter struct {
	Kinds         []string
	Statuses      []string
	Limit, Offset int32
	// BranchIDs is the caller's branch scope (set by the use case).
	BranchIDs []int64
}

// ProductAlternative is a sellable product for the same request.
type ProductAlternative struct {
	Product  ProductRef `json:"product"`
	Sellable int        `json:"sellable"`
}

// TimeAlternative is a nearby range where the product is sellable.
type TimeAlternative struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Sellable int       `json:"sellable"`
}

// Alternatives accompany CAPACITY_UNAVAILABLE.
type Alternatives struct {
	Products []ProductAlternative `json:"products"`
	Times    []TimeAlternative    `json:"times"`
}

// CapacityRule is a channel capacity rule.
type CapacityRule struct {
	UUID      uuid.UUID   `json:"uuid"`
	RuleType  string      `json:"rule_type"`
	Channel   *string     `json:"channel"`
	Product   *ProductRef `json:"product"`
	Branch    *BranchRef  `json:"branch"`
	Quantity  int32       `json:"quantity"`
	Note      string      `json:"note"`
	IsActive  bool        `json:"is_active"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// CapacityRuleInput is the full body of a rule create / update (product,
// branch and channel may be null = every product / branch / channel).
type CapacityRuleInput struct {
	RuleType    string     `json:"rule_type"`
	Channel     *string    `json:"channel"`
	ProductUUID *uuid.UUID `json:"product_uuid"`
	BranchUUID  *uuid.UUID `json:"branch_uuid"`
	Quantity    int        `json:"quantity"`
	Note        string     `json:"note"`
	IsActive    *bool      `json:"is_active"`
}

// RuleFilter selects a page of capacity rules.
type RuleFilter struct {
	RuleTypes     []string
	Channels      []string
	Q             string
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// RuleSortable maps the sortable fields of the rules list.
var RuleSortable = map[string]string{"rule_type": "rule_type", "quantity": "quantity", "created_at": "created_at"}
