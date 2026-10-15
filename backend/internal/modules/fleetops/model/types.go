// Package model holds the API shapes of the fleet operations resources:
// vehicle blocks (TEC-694), vehicle transfers and one-way rules (TEC-695).
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Engine / activity / live resource slugs.
const (
	// ResourceBlocks is the live channel of a vehicle's block list. Block
	// activity is recorded on the vehicle (tenant.vehicles).
	ResourceBlocks      = "tenant.vehicle_blocks"
	ResourceTransfers   = "tenant.transfers"
	ResourceOneWayRules = "tenant.one_way_rules"
	// ResourceVehicles is the vehicles resource (block activity, vehicle
	// list refresh after a status change).
	ResourceVehicles = "tenant.vehicles"
)

// Block kinds.
const (
	BlockMaintenance  = "maintenance"
	BlockDamage       = "damage"
	BlockCleaning     = "cleaning"
	BlockOutOfService = "out_of_service"
	BlockOther        = "other"
)

// BlockKinds lists the block kinds.
var BlockKinds = []string{BlockMaintenance, BlockDamage, BlockCleaning, BlockOutOfService, BlockOther}

// BlockStatus maps a block kind to the vehicle status "araç durumunu da
// güncelle" sets; "other" has none.
var BlockStatus = map[string]string{
	BlockMaintenance:  "maintenance",
	BlockDamage:       "damaged",
	BlockCleaning:     "cleaning",
	BlockOutOfService: "out_of_service",
}

// Block states relative to now.
const (
	BlockScheduled = "scheduled"
	BlockActive    = "active"
	BlockEnded     = "ended"
)

// BlockStates lists the block states (list filter).
var BlockStates = []string{BlockScheduled, BlockActive, BlockEnded}

// Transfer statuses.
const (
	TransferPlanned   = "planned"
	TransferInTransit = "in_transit"
	TransferCompleted = "completed"
	TransferCancelled = "cancelled"
)

// TransferStatuses lists the transfer statuses.
var TransferStatuses = []string{TransferPlanned, TransferInTransit, TransferCompleted, TransferCancelled}

// ArrivalStatuses are the vehicle statuses a completed transfer may leave.
var ArrivalStatuses = []string{"available", "preparing"}

// Affected kinds (items a block change touches): open transfers and the
// active allocations of the availability engine (TEC-697), whose kind is
// the allocation kind.
const (
	AffectedTransfer    = "transfer"
	AffectedHold        = "hold"
	AffectedReservation = "reservation"
)

// Actor is a user embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// BranchRef is a branch embedded in a record.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// VehicleRef is a vehicle embedded in a transfer.
type VehicleRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Plate string    `json:"plate"`
	Label string    `json:"label"`
}

// ProductRef is a rental product embedded in a one-way rule.
type ProductRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Code  string    `json:"code"`
	Title string    `json:"title"`
}

// --------------------------------------------------------------- blocks

// Block is one usage block of a vehicle. EndsAt nil = open-ended.
type Block struct {
	UUID        uuid.UUID  `json:"uuid"`
	VehicleUUID uuid.UUID  `json:"vehicle_uuid"`
	Kind        string     `json:"kind"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at"`
	State       string     `json:"state"`
	Reason      string     `json:"reason"`
	CreatedBy   *Actor     `json:"created_by"`
	ReleasedAt  *time.Time `json:"released_at"`
	ReleasedBy  *Actor     `json:"released_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Affected is an item a block overlaps (transfer, hold or reservation).
// The caller decides what to do with it.
type Affected struct {
	Kind     string     `json:"kind"`
	UUID     uuid.UUID  `json:"uuid"`
	Label    string     `json:"label"`
	Status   string     `json:"status"`
	StartsAt time.Time  `json:"starts_at"`
	EndsAt   *time.Time `json:"ends_at"`
}

// BlockResult is the answer of every block mutation.
type BlockResult struct {
	Block Block `json:"block"`
	// Affected lists the open transfers and the active holds /
	// reservations the block now overlaps.
	Affected []Affected `json:"affected"`
	// VehicleStatus is the vehicle status after the change; StatusChanged
	// tells whether the call changed it (sync_status).
	VehicleStatus string `json:"vehicle_status"`
	StatusChanged bool   `json:"status_changed"`
}

// BlockInput is the create body; PATCH uses the same fields (nil =
// unchanged; ends_at null = open-ended).
type BlockInput struct {
	Kind     *string      `json:"kind"`
	StartsAt *time.Time   `json:"starts_at"`
	EndsAt   OptionalTime `json:"ends_at"`
	Reason   *string      `json:"reason"`
	// SyncStatus also sets the vehicle status (maintenance / damaged /
	// cleaning / out_of_service) when the block covers now. Create only.
	SyncStatus bool `json:"sync_status"`
}

// ReleaseInput is the body of POST …/blocks/{blockUuid}/release ("Engeli
// kapat"). SyncStatus sets the vehicle back to available when its status
// is still the block's status.
type ReleaseInput struct {
	SyncStatus bool `json:"sync_status"`
}

// BlockFilter selects the blocks of one vehicle.
type BlockFilter struct {
	Limit, Offset int32
	Kinds         []string
	States        []string
}

// ------------------------------------------------------------ transfers

// Transfer is a move of a vehicle between two branches.
type Transfer struct {
	UUID             uuid.UUID  `json:"uuid"`
	Vehicle          VehicleRef `json:"vehicle"`
	FromBranch       BranchRef  `json:"from_branch"`
	ToBranch         BranchRef  `json:"to_branch"`
	Responsible      *Actor     `json:"responsible"`
	PlannedDeparture time.Time  `json:"planned_departure"`
	PlannedArrival   time.Time  `json:"planned_arrival"`
	ActualDeparture  *time.Time `json:"actual_departure"`
	ActualArrival    *time.Time `json:"actual_arrival"`
	// Cost is a TRY decimal string ("1250.00"); nil = not entered.
	Cost        *string    `json:"cost"`
	Status      string     `json:"status"`
	Note        string     `json:"note"`
	CancelledAt *time.Time `json:"cancelled_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TransferInput is the create body (vehicle_uuid required) and the PATCH
// body (planned transfers: every field; started ones: cost and note).
type TransferInput struct {
	VehicleUUID         *uuid.UUID   `json:"vehicle_uuid"`
	FromBranchUUID      *uuid.UUID   `json:"from_branch_uuid"`
	ToBranchUUID        *uuid.UUID   `json:"to_branch_uuid"`
	ResponsibleUserUUID OptionalUUID `json:"responsible_user_uuid"`
	PlannedDeparture    *time.Time   `json:"planned_departure"`
	PlannedArrival      *time.Time   `json:"planned_arrival"`
	Cost                OptionalNum  `json:"cost"`
	Note                *string      `json:"note"`
}

// TransferActionInput is the body of start / complete / cancel. At
// defaults to now (start / complete); ArrivalStatus is the vehicle status
// after completion (available | preparing, default available).
type TransferActionInput struct {
	At            *time.Time `json:"at"`
	ArrivalStatus string     `json:"arrival_status"`
	Note          string     `json:"note"`
}

// TransferFilter selects transfers of one organization.
type TransferFilter struct {
	Limit, Offset   int32
	Q               string
	Statuses        []string
	FromBranchUUIDs []uuid.UUID
	ToBranchUUIDs   []uuid.UUID
	VehicleUUID     *uuid.UUID
	SortField       string
	SortDesc        bool
	// BranchIDs is the caller's branch scope; set by the use case.
	BranchIDs []int64
}

// TransferQueryKeys are the transfer list filters (HTTP list and export).
var TransferQueryKeys = []string{"status", "from_branch", "to_branch", "vehicle"}

// TransferFilterFromQuery maps list query params to a filter.
func TransferFilterFromQuery(query map[string]string) TransferFilter {
	f := TransferFilter{
		Q:               strings.TrimSpace(query["q"]),
		Statuses:        SplitList(query["status"]),
		FromBranchUUIDs: SplitUUIDs(query["from_branch"]),
		ToBranchUUIDs:   SplitUUIDs(query["to_branch"]),
	}
	if id, err := uuid.Parse(strings.TrimSpace(query["vehicle"])); err == nil {
		f.VehicleUUID = &id
	}
	return f
}

// TransferFacets are the dynamic filter options of the transfers list.
type TransferFacets struct {
	Status     []Facet `json:"status"`
	FromBranch []Facet `json:"from_branch"`
	ToBranch   []Facet `json:"to_branch"`
}

// Assignee is a member who can be responsible for a transfer.
type Assignee struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// ------------------------------------------------------------ one-way rules

// OneWayRule is a one-way route rule.
type OneWayRule struct {
	UUID       uuid.UUID   `json:"uuid"`
	FromBranch BranchRef   `json:"from_branch"`
	ToBranch   BranchRef   `json:"to_branch"`
	Product    *ProductRef `json:"rental_product"`
	Allowed    bool        `json:"allowed"`
	// Fee is a TRY decimal string ("750.00").
	Fee       string    `json:"fee"`
	Note      string    `json:"note"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OneWayInput is the create / patch body (nil = unchanged / default).
type OneWayInput struct {
	FromBranchUUID    *uuid.UUID   `json:"from_branch_uuid"`
	ToBranchUUID      *uuid.UUID   `json:"to_branch_uuid"`
	RentalProductUUID OptionalUUID `json:"rental_product_uuid"`
	Allowed           *bool        `json:"allowed"`
	Fee               *json.Number `json:"fee"`
	Note              *string      `json:"note"`
	IsActive          *bool        `json:"is_active"`
}

// OneWayQuote is the answer of GET /v1/tenant/one-way-rules/resolve: may a
// rental of product P picked up at A be returned at B, and for which fee.
// Rule nil = no active rule for the route (not allowed).
type OneWayQuote struct {
	Allowed bool       `json:"allowed"`
	Fee     string     `json:"fee"`
	Rule    *uuid.UUID `json:"rule_uuid"`
	// ProductSpecific tells whether a product rule overrode the generic one.
	ProductSpecific bool `json:"product_specific"`
}

// OneWayFilter selects one-way rules of one organization.
type OneWayFilter struct {
	Limit, Offset   int32
	Q               string
	IsActive        *bool
	Allowed         *bool
	FromBranchUUIDs []uuid.UUID
	ToBranchUUIDs   []uuid.UUID
	SortField       string
	SortDesc        bool
	BranchIDs       []int64
}

// OneWayQueryKeys are the rule list filters (HTTP list, export, bulk).
var OneWayQueryKeys = []string{"is_active", "allowed", "from_branch", "to_branch"}

// OneWayFilterFromQuery maps list query params to a filter.
func OneWayFilterFromQuery(query map[string]string) OneWayFilter {
	f := OneWayFilter{
		Q:               strings.TrimSpace(query["q"]),
		FromBranchUUIDs: SplitUUIDs(query["from_branch"]),
		ToBranchUUIDs:   SplitUUIDs(query["to_branch"]),
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["is_active"])); err == nil {
		f.IsActive = &v
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["allowed"])); err == nil {
		f.Allowed = &v
	}
	return f
}

// OneWayFacets are the dynamic filter options of the rules list.
type OneWayFacets struct {
	FromBranch []Facet `json:"from_branch"`
	ToBranch   []Facet `json:"to_branch"`
	IsActive   []Facet `json:"is_active"`
	Allowed    []Facet `json:"allowed"`
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

// OptionalTime distinguishes an absent JSON field from an explicit null.
type OptionalTime struct {
	Set   bool
	Value *time.Time
}

func (o *OptionalTime) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	var v time.Time
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalUUID distinguishes an absent JSON field from an explicit null.
type OptionalUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (o *OptionalUUID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	var v uuid.UUID
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalNum distinguishes an absent JSON number from an explicit null.
type OptionalNum struct {
	Set   bool
	Value *json.Number
}

func (o *OptionalNum) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	switch n := v.(type) {
	case json.Number:
		o.Value = &n
	case string:
		num := json.Number(strings.TrimSpace(n))
		o.Value = &num
	default:
		return errors.New("must be a number")
	}
	return nil
}

func isNull(b []byte) bool {
	return bytes.Equal(bytes.TrimSpace(b), []byte("null"))
}

// Contains reports whether list holds v.
func Contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
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
