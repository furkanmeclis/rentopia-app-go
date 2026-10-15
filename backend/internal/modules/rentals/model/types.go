// Package model holds the types of rentals and operation tasks (TEC-709):
// statuses, the API shapes, inputs, filters and the ports to contracts.
// docs/design/rentals.md is the design.
package model

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Live list / activity resources.
const (
	ResourceRentals        = "tenant.rentals"
	ResourceOperationTasks = "tenant.operation_tasks"
)

// Rental statuses.
const (
	StatusPreparation = "preparation"
	StatusActive      = "active"
	StatusReturnCheck = "return_check"
	StatusReturned    = "returned"
	StatusCancelled   = "cancelled"
)

// Statuses in display order.
var Statuses = []string{StatusPreparation, StatusActive, StatusReturnCheck, StatusReturned, StatusCancelled}

// Timeline actions.
const (
	EventCreated             = "created"
	EventHandedOver          = "handed_over"
	EventReturned            = "returned"
	EventChecked             = "return_checked"
	EventCancelled           = "cancelled"
	EventExtended            = "extended"
	EventSwapped             = "vehicle_swapped"
	EventPlateChanged        = "plate_changed"
	EventOverdue             = "overdue"
	EventOverdueExtended     = "overdue_extended"
	EventLateReturnTasks     = "late_return_tasks"
	EventPlanMoved           = "plan_moved"
	EventChargeProposed      = "charge_proposed"
	EventChargeApproved      = "charge_approved"
	EventChargeWaived        = "charge_waived"
	EventContractWaived      = "contract_waived"
	EventEarlyReturn         = "early_return"
	EventReservationChanged  = "reservation_changed"
	EventDriversChanged      = "drivers_changed"
	EventDocumentPrepared    = "document_prepared"
	EventDocumentApproved    = "document_approved"
	EventReapprovalRequired  = "reapproval_required"
	EventVehicleStatusFailed = "vehicle_status_warning"
)

// Charge kinds, sources and statuses.
const (
	ChargeLateReturn  = "late_return"
	ChargeEarlyReturn = "early_return"
	ChargeDamage      = "damage"
	ChargeFuel        = "fuel"
	ChargeMileage     = "mileage"
	ChargeOther       = "other"

	SourcePolicy = "policy"
	SourceManual = "manual"

	ChargeProposed = "proposed"
	ChargeApproved = "approved"
	ChargeWaived   = "waived"
)

// ChargeKinds lists the manual charge kinds (late_return policy fees are
// computed, a manual late fee is allowed too).
var ChargeKinds = []string{ChargeLateReturn, ChargeEarlyReturn, ChargeDamage, ChargeFuel, ChargeMileage, ChargeOther}

// Operation task types and statuses.
const (
	TaskPreparation = "preparation"
	TaskHandover    = "handover"
	TaskReturn      = "return"
	TaskCleaning    = "cleaning"

	TaskOpen       = "open"
	TaskInProgress = "in_progress"
	TaskDone       = "done"
	TaskCancelled  = "cancelled"
)

// TaskTypes / TaskStatuses in display order.
var (
	TaskTypes    = []string{TaskPreparation, TaskHandover, TaskReturn, TaskCleaning}
	TaskStatuses = []string{TaskOpen, TaskInProgress, TaskDone, TaskCancelled}
)

// OverdueHorizon is how far the engine allocation of a late vehicle is
// extended at a time (the sweep extends again when it passes).
const OverdueHorizon = 2 * time.Hour

// Ref is a uuid with a display label.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code,omitempty"`
	Name string    `json:"name"`
}

// VehicleRef is a vehicle embedded in an answer.
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

// CustomerRef is the contract party (display only).
type CustomerRef struct {
	UUID         uuid.UUID `json:"uuid"`
	CustomerType string    `json:"customer_type"`
	DisplayName  string    `json:"display_name"`
}

// Place is a planned or actual place (branch + optional service point).
type Place struct {
	Branch *Ref `json:"branch"`
	Point  *Ref `json:"point"`
}

// Leg is one vehicle of the rental (handover / return record).
type Leg struct {
	UUID      uuid.UUID  `json:"uuid"`
	Reason    string     `json:"reason"`
	Vehicle   VehicleRef `json:"vehicle"`
	OutAt     *time.Time `json:"out_at"`
	OutBranch *Ref       `json:"out_branch"`
	OutKm     *int32     `json:"out_km"`
	OutFuel   *int16     `json:"out_fuel"`
	InAt      *time.Time `json:"in_at"`
	InBranch  *Ref       `json:"in_branch"`
	InKm      *int32     `json:"in_km"`
	InFuel    *int16     `json:"in_fuel"`
	// DrivenKm = in − out (nil while the leg is open).
	DrivenKm *int32 `json:"driven_km"`
	Note     string `json:"note"`
}

// Driver is a driver of the rental (masked snapshot).
type Driver struct {
	UUID        *uuid.UUID `json:"uuid"`
	Role        string     `json:"role"`
	Name        string     `json:"name"`
	LicenceHint string     `json:"licence_hint"`
}

// Charge is a charge beyond the sold price.
type Charge struct {
	UUID            uuid.UUID       `json:"uuid"`
	Kind            string          `json:"kind"`
	Source          string          `json:"source"`
	Status          string          `json:"status"`
	Currency        string          `json:"currency"`
	Amount          money.Money     `json:"amount"`
	SettingsVersion int32           `json:"settings_version"`
	Calculation     json.RawMessage `json:"calculation"`
	Reason          string          `json:"reason"`
	DecidedAt       *time.Time      `json:"decided_at"`
	DecidedBy       *Actor          `json:"decided_by"`
	DecisionNote    string          `json:"decision_note"`
	CreatedBy       *Actor          `json:"created_by"`
	CreatedAt       time.Time       `json:"created_at"`
}

// Policy is the policy part of the reservation's settings version that
// rental charges use.
type Policy struct {
	SettingsVersion        int    `json:"settings_version"`
	LateReturnGraceMinutes *int   `json:"late_return_grace_minutes"`
	ExtraHourFee           string `json:"extra_hour_fee"`
	EarlyReturn            string `json:"early_return"`
	Extension              string `json:"extension"`
	Fuel                   string `json:"fuel"`
	Mileage                string `json:"mileage"`
}

// PolicyFromSnapshot reads the reservation policy snapshot
// ({settings_version, policies: {...}}).
func PolicyFromSnapshot(raw []byte) Policy {
	var snap struct {
		SettingsVersion int `json:"settings_version"`
		Policies        struct {
			EarlyReturn            string `json:"early_return"`
			Extension              string `json:"extension"`
			LateReturnGraceMinutes *int   `json:"late_return_grace_minutes"`
			ExtraHourFee           string `json:"extra_hour_fee"`
			Fuel                   string `json:"fuel"`
			Mileage                string `json:"mileage"`
		} `json:"policies"`
	}
	_ = json.Unmarshal(raw, &snap)
	return Policy{
		SettingsVersion: snap.SettingsVersion, LateReturnGraceMinutes: snap.Policies.LateReturnGraceMinutes,
		ExtraHourFee: strings.TrimSpace(snap.Policies.ExtraHourFee), EarlyReturn: snap.Policies.EarlyReturn,
		Extension: snap.Policies.Extension, Fuel: snap.Policies.Fuel, Mileage: snap.Policies.Mileage,
	}
}

// Grace returns the late tolerance in minutes (0 when not defined).
func (p Policy) Grace() int {
	if p.LateReturnGraceMinutes != nil && *p.LateReturnGraceMinutes > 0 {
		return *p.LateReturnGraceMinutes
	}
	return 0
}

// ReservationRef is the reservation a rental belongs to.
type ReservationRef struct {
	UUID          uuid.UUID   `json:"uuid"`
	Reference     string      `json:"reference"`
	Status        string      `json:"status"`
	Currency      string      `json:"currency"`
	TotalAmount   money.Money `json:"total_amount"`
	PaidAmount    money.Money `json:"paid_amount"`
	PaymentStatus string      `json:"payment_status"`
}

// ContractState tells whether the handover may happen without a waiver.
type ContractState struct {
	// Required: an active rental contract template exists.
	Required bool `json:"required"`
	// Approved: the rental contract's current version is approved.
	Approved     bool       `json:"approved"`
	DocumentUUID *uuid.UUID `json:"document_uuid"`
}

// Rental is a rental as the detail answer shows it.
type Rental struct {
	UUID        uuid.UUID      `json:"uuid"`
	Reference   string         `json:"reference"`
	Status      string         `json:"status"`
	Reservation ReservationRef `json:"reservation"`
	Customer    CustomerRef    `json:"customer"`
	Product     Ref            `json:"product"`
	Vehicle     VehicleRef     `json:"vehicle"`
	// Planned = the reservation's (extensions move the return).
	PlannedPickupAt time.Time `json:"planned_pickup_at"`
	PlannedReturnAt time.Time `json:"planned_return_at"`
	PlannedPickup   Place     `json:"planned_pickup"`
	PlannedReturn   Place     `json:"planned_return"`
	// Actual facts.
	ActualPickupAt *time.Time `json:"actual_pickup_at"`
	ActualReturnAt *time.Time `json:"actual_return_at"`
	ActualPickup   Place      `json:"actual_pickup"`
	ActualReturn   Place      `json:"actual_return"`
	OverdueAt      *time.Time `json:"overdue_at"`
	LateMinutes    int32      `json:"late_minutes"`
	// AllocationEndsAt is where the engine keeps the vehicle busy until
	// (the planned return, the expected return of a late vehicle, or the
	// actual return).
	AllocationEndsAt *time.Time      `json:"allocation_ends_at"`
	AllocationKind   string          `json:"allocation_kind"`
	Policy           Policy          `json:"policy"`
	ContractWaiver   json.RawMessage `json:"contract_waiver"`
	Contract         ContractState   `json:"contract"`
	Legs             []Leg           `json:"legs"`
	Drivers          []Driver        `json:"drivers"`
	Charges          []Charge        `json:"charges"`
	ReturnNote       string          `json:"return_note"`
	DamageNote       string          `json:"damage_note"`
	Note             string          `json:"note"`
	CancelReason     string          `json:"cancel_reason"`
	CheckedAt        *time.Time      `json:"checked_at"`
	CancelledAt      *time.Time      `json:"cancelled_at"`
	ClosedAt         *time.Time      `json:"closed_at"`
	CreatedBy        *Actor          `json:"created_by"`
	DeliveredBy      *Actor          `json:"delivered_by"`
	ReturnedBy       *Actor          `json:"returned_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// ListItem is a row of the rentals list.
type ListItem struct {
	UUID            uuid.UUID   `json:"uuid"`
	Reference       string      `json:"reference"`
	Status          string      `json:"status"`
	ReservationUUID uuid.UUID   `json:"reservation_uuid"`
	Customer        CustomerRef `json:"customer"`
	Product         Ref         `json:"product"`
	Vehicle         VehicleRef  `json:"vehicle"`
	PickupBranch    Ref         `json:"pickup_branch"`
	ReturnBranch    Ref         `json:"return_branch"`
	ActualReturn    *Ref        `json:"actual_return_branch"`
	PlannedPickupAt time.Time   `json:"planned_pickup_at"`
	PlannedReturnAt time.Time   `json:"planned_return_at"`
	ActualPickupAt  *time.Time  `json:"actual_pickup_at"`
	ActualReturnAt  *time.Time  `json:"actual_return_at"`
	OverdueAt       *time.Time  `json:"overdue_at"`
	LateMinutes     int32       `json:"late_minutes"`
	OpenCharges     int64       `json:"open_charges"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// Event is one entry of the rental timeline.
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

// FacetValue is one option of a list facet.
type FacetValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the rentals list.
type Facets struct {
	Status  []FacetValue `json:"status"`
	Branch  []FacetValue `json:"branch"`
	Product []FacetValue `json:"product"`
}

// QueryKeys are the list filter query keys (exports honour them too).
var QueryKeys = []string{"status", "branch", "product", "vehicle", "customer", "reservation", "overdue"}

// Sortable maps the sortable list fields.
var Sortable = map[string]string{
	"reference": "reference", "status": "status", "planned_pickup_at": "planned_pickup_at",
	"planned_return_at": "planned_return_at", "created_at": "created_at",
}

// ListFilter selects a page of rentals.
type ListFilter struct {
	Q                string
	Statuses         []string
	BranchUUIDs      []uuid.UUID
	ProductUUIDs     []uuid.UUID
	VehicleUUIDs     []uuid.UUID
	CustomerUUIDs    []uuid.UUID
	ReservationUUIDs []uuid.UUID
	OverdueOnly      bool
	SortField        string
	SortDesc         bool
	Limit, Offset    int32
}

// FilterFromQuery builds a filter from comma-joined query values.
func FilterFromQuery(v map[string]string) ListFilter {
	f := ListFilter{Q: strings.TrimSpace(v["q"])}
	f.Statuses = allowed(split(v["status"]), Statuses)
	f.BranchUUIDs = uuids(v["branch"])
	f.ProductUUIDs = uuids(v["product"])
	f.VehicleUUIDs = uuids(v["vehicle"])
	f.CustomerUUIDs = uuids(v["customer"])
	f.ReservationUUIDs = uuids(v["reservation"])
	f.OverdueOnly = v["overdue"] == "true" || v["overdue"] == "1"
	return f
}

// --------------------------------------------------------------- inputs

// CreateInput is the body of POST /v1/tenant/rentals: a rental from a
// confirmed reservation. VehicleUUID fixes that plate first (the plate of
// the reservation is fixed when omitted).
type CreateInput struct {
	ReservationUUID *uuid.UUID `json:"reservation_uuid"`
	VehicleUUID     *uuid.UUID `json:"vehicle_uuid"`
	Note            string     `json:"note"`
}

// HandoverInput records the delivery of the vehicle.
type HandoverInput struct {
	// At defaults to now.
	At *time.Time `json:"at"`
	// BranchUUID / PointUUID default to the planned pick-up.
	BranchUUID *uuid.UUID `json:"branch_uuid"`
	PointUUID  *uuid.UUID `json:"point_uuid"`
	Km         *int32     `json:"km"`
	Fuel       *int16     `json:"fuel"`
	Note       string     `json:"note"`
	// WaiverReason hands over without an approved rental contract
	// (tenant.signature.waive).
	WaiverReason string `json:"waiver_reason"`
}

// ReturnInput records the vehicle coming back.
type ReturnInput struct {
	At         *time.Time `json:"at"`
	BranchUUID *uuid.UUID `json:"branch_uuid"`
	PointUUID  *uuid.UUID `json:"point_uuid"`
	Km         *int32     `json:"km"`
	Fuel       *int16     `json:"fuel"`
	Note       string     `json:"note"`
	DamageNote string     `json:"damage_note"`
}

// CompleteReturnInput finishes the return check.
type CompleteReturnInput struct {
	// VehicleStatus after the check: cleaning (default, opens a cleaning
	// task) or available.
	VehicleStatus string `json:"vehicle_status"`
	Note          string `json:"note"`
}

// ExtendInput extends a rental under way (the reservation's extension
// rule prices the added days).
type ExtendInput struct {
	EndsAt *time.Time `json:"ends_at"`
	Reason string     `json:"reason"`
}

// SwapInput changes the vehicle of a rental under way.
type SwapInput struct {
	VehicleUUID *uuid.UUID `json:"vehicle_uuid"`
	At          *time.Time `json:"at"`
	BranchUUID  *uuid.UUID `json:"branch_uuid"`
	// The returned vehicle.
	InKm   *int32 `json:"in_km"`
	InFuel *int16 `json:"in_fuel"`
	// The new vehicle.
	OutKm   *int32 `json:"out_km"`
	OutFuel *int16 `json:"out_fuel"`
	Reason  string `json:"reason"`
}

// ReasonInput carries a reason.
type ReasonInput struct {
	Reason string `json:"reason"`
}

// ChargeInput proposes a manual charge.
type ChargeInput struct {
	Kind   string       `json:"kind"`
	Amount *money.Money `json:"amount"`
	Reason string       `json:"reason"`
}

// DecisionInput approves or waives a charge.
type DecisionInput struct {
	Note string `json:"note"`
}

// --------------------------------------------------------------- tasks

// Task is an operation task.
type Task struct {
	UUID           uuid.UUID   `json:"uuid"`
	TaskType       string      `json:"task_type"`
	Status         string      `json:"status"`
	Branch         Ref         `json:"branch"`
	Rental         *TaskRental `json:"rental"`
	Vehicle        *VehicleRef `json:"vehicle"`
	DueAt          time.Time   `json:"due_at"`
	Note           string      `json:"note"`
	Assignee       *Actor      `json:"assignee"`
	StartedAt      *time.Time  `json:"started_at"`
	CompletedAt    *time.Time  `json:"completed_at"`
	CompletedBy    *Actor      `json:"completed_by"`
	CompletionNote string      `json:"completion_note"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// TaskRental is the rental of a task.
type TaskRental struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
}

// TaskFilter selects a page of operation tasks.
type TaskFilter struct {
	Q             string
	Statuses      []string
	TaskTypes     []string
	BranchUUIDs   []uuid.UUID
	RentalUUIDs   []uuid.UUID
	VehicleUUIDs  []uuid.UUID
	AssigneeUUIDs []uuid.UUID
	DueFrom       *time.Time
	DueTo         *time.Time
	SortField     string
	SortDesc      bool
	Limit, Offset int32
}

// TaskQueryKeys are the task list filter keys.
var TaskQueryKeys = []string{"status", "task_type", "branch", "rental", "vehicle", "assignee", "due_from", "due_to"}

// TaskSortable maps the sortable task list fields.
var TaskSortable = map[string]string{"due_at": "due_at", "status": "status", "created_at": "created_at"}

// TaskFilterFromQuery builds a task filter from comma-joined query values.
func TaskFilterFromQuery(v map[string]string) TaskFilter {
	f := TaskFilter{Q: strings.TrimSpace(v["q"])}
	f.Statuses = allowed(split(v["status"]), TaskStatuses)
	f.TaskTypes = allowed(split(v["task_type"]), TaskTypes)
	f.BranchUUIDs = uuids(v["branch"])
	f.RentalUUIDs = uuids(v["rental"])
	f.VehicleUUIDs = uuids(v["vehicle"])
	f.AssigneeUUIDs = uuids(v["assignee"])
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v["due_from"])); err == nil {
		f.DueFrom = &t
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v["due_to"])); err == nil {
		f.DueTo = &t
	}
	return f
}

// TaskFacets are the dynamic filter options of the task list.
type TaskFacets struct {
	Status   []FacetValue `json:"status"`
	TaskType []FacetValue `json:"task_type"`
	Branch   []FacetValue `json:"branch"`
}

// AssignInput assigns a member (null = unassign).
type AssignInput struct {
	UserUUID *uuid.UUID `json:"user_uuid"`
}

// NoteInput carries an optional note.
type NoteInput struct {
	Note string `json:"note"`
}

// --------------------------------------------------------------- ports

// Documents is the contracts module as rentals see it (TEC-710). Every
// call is best effort for the rental flow except ContractState.
type Documents interface {
	// ContractState: is a rental contract required, and is its current
	// version approved?
	ContractState(ctx context.Context, orgID int64, reservationID int64) (ContractState, error)
	// Prepare creates or refreshes the documents of a reservation that
	// depend on what changed (kinds empty = every live document; the
	// rental contract is created when it does not exist yet). Returns the
	// documents that got a new version.
	Prepare(ctx context.Context, orgID int64, reservationID int64, kinds []string, reason string) ([]uuid.UUID, error)
	// Void withdraws the rental's documents (rental cancelled).
	Void(ctx context.Context, orgID int64, rentalID int64) error
}

// NoDocuments is the default (contracts not wired).
type NoDocuments struct{}

// ContractState implements Documents.
func (NoDocuments) ContractState(context.Context, int64, int64) (ContractState, error) {
	return ContractState{}, nil
}

// Prepare implements Documents.
func (NoDocuments) Prepare(context.Context, int64, int64, []string, string) ([]uuid.UUID, error) {
	return nil, nil
}

// Void implements Documents.
func (NoDocuments) Void(context.Context, int64, int64) error { return nil }

// --------------------------------------------------------------- helpers

func split(raw string) []string {
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func uuids(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range split(raw) {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func allowed(values, set []string) []string {
	out := []string{}
	for _, v := range values {
		for _, a := range set {
			if v == a {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// Contains reports whether set holds v.
func Contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
