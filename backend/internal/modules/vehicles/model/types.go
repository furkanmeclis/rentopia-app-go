// Package model holds the API shapes of the vehicles tenant resource
// (TEC-692). A vehicle record is physical reality only: what is sold lives in
// rental products, when it is busy lives in the capacity tables.
package model

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Resource is the stable slug of the vehicles resource (activity, io, bulk,
// live events).
const Resource = "tenant.vehicles"

// Operational statuses. Transitions are permissive (any → any) but every
// change is recorded in vehicle_events.
const (
	StatusAvailable    = "available"
	StatusPreparing    = "preparing"
	StatusRented       = "rented"
	StatusReturnCheck  = "return_check"
	StatusCleaning     = "cleaning"
	StatusMaintenance  = "maintenance"
	StatusDamaged      = "damaged"
	StatusInTransfer   = "in_transfer"
	StatusOutOfService = "out_of_service"
)

// Statuses lists the operational statuses.
var Statuses = []string{
	StatusAvailable, StatusPreparing, StatusRented, StatusReturnCheck, StatusCleaning,
	StatusMaintenance, StatusDamaged, StatusInTransfer, StatusOutOfService,
}

// Ownership values.
const (
	OwnershipOwned    = "owned"
	OwnershipSupplied = "supplied"
)

// Ownerships lists the ownership values.
var Ownerships = []string{OwnershipOwned, OwnershipSupplied}

// DefaultPool is the fleet pool of a vehicle created without one.
const DefaultPool = "MAIN"

// Physical attribute values (same sets as rental groups).
var (
	Transmissions = []string{"manual", "automatic"}
	Fuels         = []string{"petrol", "diesel", "hybrid", "electric", "lpg"}
	BodyTypes     = []string{
		"hatchback", "sedan", "suv", "crossover", "station_wagon", "mpv", "van", "pickup", "coupe", "cabrio",
	}
)

// BranchRef is a branch embedded in a vehicle.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// SegmentRef is a vehicle segment embedded in a group reference.
type SegmentRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// GroupRef is a rental group embedded in a vehicle.
type GroupRef struct {
	UUID     uuid.UUID  `json:"uuid"`
	Code     string     `json:"code"`
	Name     string     `json:"name"`
	IsActive bool       `json:"is_active"`
	Segment  SegmentRef `json:"segment"`
}

// CatalogRef is the platform catalog type of a vehicle. ModelUUID / BrandUUID
// are the ids the tenant catalog pickers use (grouped model, display brand).
type CatalogRef struct {
	TypeUUID  uuid.UUID  `json:"type_uuid"`
	TypeName  string     `json:"type_name"`
	TypeCode  string     `json:"type_code"`
	ModelUUID *uuid.UUID `json:"model_uuid"`
	ModelName string     `json:"model_name"`
	BrandUUID *uuid.UUID `json:"brand_uuid"`
	BrandName string     `json:"brand_name"`
	// Years are the valid model years of the type.
	Years []int32 `json:"years"`
}

// Vehicle is the API projection of a vehicle.
type Vehicle struct {
	UUID  uuid.UUID `json:"uuid"`
	Plate string    `json:"plate"`
	VIN   string    `json:"vin"`
	// ManualEntry: the vehicle is not in the catalog; Brand / Model /
	// TypeName come from the manual fields.
	ManualEntry bool        `json:"manual_entry"`
	Catalog     *CatalogRef `json:"catalog"`
	// Brand / Model / TypeName are the display names (catalog or manual).
	Brand    string `json:"brand"`
	Model    string `json:"model"`
	TypeName string `json:"type_name"`
	// Label is "<brand> <model> <type>".
	Label              string     `json:"label"`
	ModelYear          int16      `json:"model_year"`
	Transmission       *string    `json:"transmission"`
	Fuel               *string    `json:"fuel"`
	BodyType           *string    `json:"body_type"`
	Seats              *int16     `json:"seats"`
	Luggage            *int16     `json:"luggage"`
	AirConditioning    bool       `json:"air_conditioning"`
	AttributesVerified bool       `json:"attributes_verified"`
	Color              string     `json:"color"`
	HomeBranch         BranchRef  `json:"home_branch"`
	CurrentBranch      BranchRef  `json:"current_branch"`
	FleetPool          string     `json:"fleet_pool"`
	Ownership          string     `json:"ownership"`
	SupplierName       string     `json:"supplier_name"`
	InServiceDate      *string    `json:"in_service_date"`
	OutOfServiceDate   *string    `json:"out_of_service_date"`
	InService          bool       `json:"in_service"`
	OdometerKm         int32      `json:"odometer_km"`
	FuelLevel          *int16     `json:"fuel_level"`
	Status             string     `json:"status"`
	StatusChangedAt    time.Time  `json:"status_changed_at"`
	KaskoValue         *string    `json:"kasko_value"`
	KaskoPeriod        *string    `json:"kasko_period"`
	RentalGroups       []GroupRef `json:"rental_groups"`
	IsActive           bool       `json:"is_active"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// VehicleInput is the create / patch body. Nil fields are left unchanged on
// patch and take their defaults on create.
type VehicleInput struct {
	Plate              *string        `json:"plate"`
	VIN                *string        `json:"vin"`
	ManualEntry        *bool          `json:"manual_entry"`
	CatalogTypeUUID    OptionalUUID   `json:"catalog_type_uuid"`
	ManualBrand        *string        `json:"manual_brand"`
	ManualModel        *string        `json:"manual_model"`
	ManualType         *string        `json:"manual_type"`
	ModelYear          *int16         `json:"model_year"`
	Transmission       OptionalString `json:"transmission"`
	Fuel               OptionalString `json:"fuel"`
	BodyType           OptionalString `json:"body_type"`
	Seats              OptionalInt    `json:"seats"`
	Luggage            OptionalInt    `json:"luggage"`
	AirConditioning    *bool          `json:"air_conditioning"`
	AttributesVerified *bool          `json:"attributes_verified"`
	Color              *string        `json:"color"`
	HomeBranchUUID     *uuid.UUID     `json:"home_branch_uuid"`
	CurrentBranchUUID  *uuid.UUID     `json:"current_branch_uuid"`
	FleetPool          *string        `json:"fleet_pool"`
	Ownership          *string        `json:"ownership"`
	SupplierName       *string        `json:"supplier_name"`
	InServiceDate      OptionalString `json:"in_service_date"`
	OutOfServiceDate   OptionalString `json:"out_of_service_date"`
	OdometerKm         *int32         `json:"odometer_km"`
	FuelLevel          OptionalInt    `json:"fuel_level"`
	// Status is the initial status on create; on patch a different status
	// is a recorded transition (same as POST /status without a note).
	Status   *string `json:"status"`
	IsActive *bool   `json:"is_active"`
}

// StatusInput is the body of POST /v1/tenant/vehicles/{uuid}/status.
type StatusInput struct {
	Status     string      `json:"status"`
	OdometerKm *int32      `json:"odometer_km"`
	FuelLevel  OptionalInt `json:"fuel_level"`
	Note       string      `json:"note"`
}

// GroupMembershipInput is the body of POST /{uuid}/rental-groups.
type GroupMembershipInput struct {
	RentalGroupUUID uuid.UUID `json:"rental_group_uuid"`
}

// Actor is the user behind an event or an activity entry.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Event is one entry of a vehicle's status history.
type Event struct {
	UUID       uuid.UUID `json:"uuid"`
	EventType  string    `json:"event_type"`
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	OdometerKm *int32    `json:"odometer_km"`
	FuelLevel  *int16    `json:"fuel_level"`
	Note       string    `json:"note"`
	Actor      *Actor    `json:"actor"`
	CreatedAt  time.Time `json:"created_at"`
}

// Membership is a rental group of a vehicle with its current eligibility
// (attributes may have changed since the vehicle joined).
type Membership struct {
	Group    GroupRef  `json:"group"`
	Eligible bool      `json:"eligible"`
	Reasons  []string  `json:"reasons"`
	AddedAt  time.Time `json:"added_at"`
}

// Suggestion is a live rental group the vehicle is not a member of, with
// the eligibility check result.
type Suggestion struct {
	Group    GroupRef `json:"group"`
	Eligible bool     `json:"eligible"`
	Reasons  []string `json:"reasons"`
}

// GroupsView is the answer of GET /{uuid}/rental-groups.
type GroupsView struct {
	Items       []Membership `json:"items"`
	Suggestions []Suggestion `json:"suggestions"`
}

// Valuation is one TSB kasko value period.
type Valuation struct {
	Period string `json:"period"`
	Value  string `json:"value"`
}

// KaskoView is the answer of GET /{uuid}/kasko.
type KaskoView struct {
	ModelYear int16       `json:"model_year"`
	Catalog   *CatalogRef `json:"catalog"`
	Items     []Valuation `json:"items"`
}

// ProductVehicles is the answer of GET /v1/tenant/products/{uuid}/eligible-vehicles.
type ProductVehicles struct {
	GuaranteeType string    `json:"guarantee_type"`
	Items         []Vehicle `json:"items"`
	Total         int64     `json:"total"`
	Limit         int32     `json:"limit"`
	Offset        int32     `json:"offset"`
}

// ListFilter selects vehicles of one organization.
type ListFilter struct {
	Limit, Offset      int32
	Q                  string
	IsActive           *bool
	Statuses           []string
	Ownerships         []string
	Pools              []string
	HomeBranchUUIDs    []uuid.UUID
	CurrentBranchUUIDs []uuid.UUID
	GroupUUIDs         []uuid.UUID
	SegmentUUIDs       []uuid.UUID
	SortField          string
	SortDesc           bool
	// BranchIDs is the caller's branch scope (orgctx.BranchScope): nil means
	// every branch. Set by the usecase, never from the request.
	BranchIDs []int64
}

// QueryKeys are the list filters (HTTP list, export and bulk "all matching").
var QueryKeys = []string{
	"is_active", "status", "ownership", "pool", "home_branch", "current_branch", "rental_group", "segment",
}

// FilterFromQuery maps list query params to a filter; multi-value params
// are comma-separated.
func FilterFromQuery(query map[string]string) ListFilter {
	f := ListFilter{
		Q:                  strings.TrimSpace(query["q"]),
		Statuses:           SplitList(query["status"]),
		Ownerships:         SplitList(query["ownership"]),
		Pools:              upper(SplitList(query["pool"])),
		HomeBranchUUIDs:    splitUUIDs(query["home_branch"]),
		CurrentBranchUUIDs: splitUUIDs(query["current_branch"]),
		GroupUUIDs:         splitUUIDs(query["rental_group"]),
		SegmentUUIDs:       splitUUIDs(query["segment"]),
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["is_active"])); err == nil {
		f.IsActive = &v
	}
	return f
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

func upper(list []string) []string {
	for i := range list {
		list[i] = strings.ToUpper(list[i])
	}
	return list
}

func splitUUIDs(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range SplitList(raw) {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// Facet is one value of a faceted filter with its row count. Label is set
// for references (branch / group / segment names).
type Facet struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the vehicles list.
type Facets struct {
	Status        []Facet `json:"status"`
	Ownership     []Facet `json:"ownership"`
	Pool          []Facet `json:"pool"`
	IsActive      []Facet `json:"is_active"`
	HomeBranch    []Facet `json:"home_branch"`
	CurrentBranch []Facet `json:"current_branch"`
	RentalGroup   []Facet `json:"rental_group"`
	Segment       []Facet `json:"segment"`
}

// ActivityEntry is one audit event of a vehicle.
type ActivityEntry struct {
	UUID      uuid.UUID       `json:"uuid"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Actor     *Actor          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

// OptionalString distinguishes an absent JSON field from an explicit null.
type OptionalString struct {
	Set   bool
	Value *string
}

func (o *OptionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalInt distinguishes an absent JSON field from an explicit null.
type OptionalInt struct {
	Set   bool
	Value *int64
}

func (o *OptionalInt) UnmarshalJSON(b []byte) error {
	o.Set = true
	if isNull(b) {
		o.Value = nil
		return nil
	}
	var v int64
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

// Str returns a pointer to v (optional field helpers).
func Str(v string) *string { return &v }

// Int returns an OptionalInt set to v.
func Int(v int64) OptionalInt { return OptionalInt{Set: true, Value: &v} }

// String returns an OptionalString set to v.
func String(v string) OptionalString { return OptionalString{Set: true, Value: &v} }
