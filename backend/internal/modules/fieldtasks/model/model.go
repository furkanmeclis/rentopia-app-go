// Package model holds the field task model (TEC-724, RT-A29): statuses and
// their transitions, the task view, the inputs and the delivery details of
// a reservation. docs/design/field-tasks.md is the design.
//
// A field task is an operation_tasks row: the rental flow, a reservation
// with a delivery or staff open them; this module plans, assigns and works
// them.
package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Engine / live resources.
const (
	// Resource is the board's live list (tenant.field_tasks.read).
	Resource = "tenant.field_tasks"
	// ResourceMine is the executors' live list (tenant.field_tasks.execute):
	// the same events, for members who only see their own tasks.
	ResourceMine = "tenant.my_field_tasks"
	// ResourceOperationTasks is the rentals' list of the same rows.
	ResourceOperationTasks = "tenant.operation_tasks"
)

// Statuses.
const (
	StatusPlanned       = "planned"
	StatusAssigned      = "assigned"
	StatusEnRoute       = "en_route"
	StatusIssueReported = "issue_reported"
	StatusCompleted     = "completed"
	StatusCancelled     = "cancelled"
)

// Statuses in display order; LiveStatuses are still to be done.
var (
	Statuses     = []string{StatusPlanned, StatusAssigned, StatusEnRoute, StatusIssueReported, StatusCompleted, StatusCancelled}
	LiveStatuses = []string{StatusPlanned, StatusAssigned, StatusEnRoute, StatusIssueReported}
)

// IsLive reports whether a status is still to be done.
func IsLive(status string) bool { return contains(LiveStatuses, status) }

// Types. Handover / return at a service point or an address are the
// delivery and the collection.
const (
	TypePreparation = "preparation"
	TypeHandover    = "handover"
	TypeReturn      = "return"
	TypeCleaning    = "cleaning"
	TypeMaintenance = "maintenance"
	TypeDeposit     = "deposit"
	TypeTransfer    = "transfer"
	TypeOther       = "other"
)

// Types in display order; ManualTypes can be opened by staff (handover and
// return come from rentals and reservations, deposit from the return form).
var (
	Types       = []string{TypeHandover, TypeReturn, TypeTransfer, TypePreparation, TypeCleaning, TypeMaintenance, TypeDeposit, TypeOther}
	ManualTypes = []string{TypeTransfer, TypePreparation, TypeCleaning, TypeMaintenance, TypeOther}
)

// Origins.
const (
	OriginRental      = "rental"
	OriginReservation = "reservation"
	OriginManual      = "manual"
	OriginInspection  = "inspection"
)

// Location kinds.
const (
	LocationBranch  = "branch"
	LocationPoint   = "point"
	LocationAddress = "address"
)

// LocationKinds in display order.
var LocationKinds = []string{LocationBranch, LocationPoint, LocationAddress}

// Actions of the state machine.
const (
	ActionAssign   = "assign"
	ActionUnassign = "unassign"
	ActionStart    = "start"
	ActionReport   = "report_issue"
	ActionComplete = "complete"
	ActionCancel   = "cancel"
)

// transitions is the single table of allowed moves (from, action) -> to.
// Assign from en_route / issue_reported hands the task to someone else (it
// starts over as assigned); start from issue_reported resumes the trip.
var transitions = map[string]map[string]string{
	StatusPlanned: {
		ActionAssign: StatusAssigned, ActionStart: StatusEnRoute, ActionComplete: StatusCompleted,
		ActionCancel: StatusCancelled,
	},
	StatusAssigned: {
		ActionAssign: StatusAssigned, ActionUnassign: StatusPlanned, ActionStart: StatusEnRoute,
		ActionReport: StatusIssueReported, ActionComplete: StatusCompleted, ActionCancel: StatusCancelled,
	},
	StatusEnRoute: {
		ActionAssign: StatusAssigned, ActionUnassign: StatusPlanned, ActionReport: StatusIssueReported,
		ActionComplete: StatusCompleted, ActionCancel: StatusCancelled,
	},
	StatusIssueReported: {
		ActionAssign: StatusAssigned, ActionUnassign: StatusPlanned, ActionStart: StatusEnRoute,
		ActionComplete: StatusCompleted, ActionCancel: StatusCancelled,
	},
}

// Next returns the status an action leads to from a status (ok false: the
// move is not allowed).
func Next(from, action string) (string, bool) {
	to, ok := transitions[from][action]
	return to, ok
}

// Actor is a user embedded in a record.
type Actor struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
	Email   string    `json:"email"`
}

// Ref names a branch.
type Ref struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// PointRef names a service point.
type PointRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// RecordRef names a rental or a reservation.
type RecordRef struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
}

// VehicleRef names a vehicle.
type VehicleRef struct {
	UUID  uuid.UUID `json:"uuid"`
	Plate string    `json:"plate"`
}

// TransferRef is the fleet transfer of a transfer task.
type TransferRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Status string    `json:"status"`
}

// Location is where a task happens.
type Location struct {
	Kind      string    `json:"kind"`
	Point     *PointRef `json:"point"`
	Address   string    `json:"address"`
	District  string    `json:"district"`
	City      string    `json:"city"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
}

// Contact is who staff meet (the delivery details, else the customer).
type Contact struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// Task is a field task.
type Task struct {
	UUID           uuid.UUID    `json:"uuid"`
	TaskType       string       `json:"task_type"`
	Status         string       `json:"status"`
	Origin         string       `json:"origin"`
	Delivery       bool         `json:"delivery"`
	Branch         Ref          `json:"branch"`
	ToBranch       *Ref         `json:"to_branch"`
	Location       Location     `json:"location"`
	Contact        Contact      `json:"contact"`
	FlightNumber   string       `json:"flight_number"`
	MeetingNote    string       `json:"meeting_note"`
	Note           string       `json:"note"`
	DueAt          time.Time    `json:"due_at"`
	PlannedMinutes int32        `json:"planned_minutes"`
	PlannedEndAt   time.Time    `json:"planned_end_at"`
	StartedAt      *time.Time   `json:"started_at"`
	CompletedAt    *time.Time   `json:"completed_at"`
	AssignedAt     *time.Time   `json:"assigned_at"`
	Assignee       *Actor       `json:"assignee"`
	CompletedBy    *Actor       `json:"completed_by"`
	CompletionNote string       `json:"completion_note"`
	IssueNote      string       `json:"issue_note"`
	IssueAt        *time.Time   `json:"issue_reported_at"`
	Rental         *RecordRef   `json:"rental"`
	Reservation    *RecordRef   `json:"reservation"`
	Vehicle        *VehicleRef  `json:"vehicle"`
	Transfer       *TransferRef `json:"transfer"`
	// Inspection is the form a handover / return task continues in
	// ("handover" / "return"; empty for other tasks or before the rental).
	Inspection string    `json:"inspection"`
	Actions    []string  `json:"actions"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Event is one timeline entry.
type Event struct {
	UUID       uuid.UUID      `json:"uuid"`
	Action     string         `json:"action"`
	FromStatus *string        `json:"from_status"`
	ToStatus   *string        `json:"to_status"`
	Note       string         `json:"note"`
	Data       map[string]any `json:"data"`
	Actor      *Actor         `json:"actor"`
	CreatedAt  time.Time      `json:"created_at"`
}

// FacetValue is one option of a dynamic filter.
type FacetValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// Facets are the dynamic filter options of the board.
type Facets struct {
	Status       []FacetValue `json:"status"`
	TaskType     []FacetValue `json:"task_type"`
	LocationKind []FacetValue `json:"location_kind"`
	Branch       []FacetValue `json:"branch"`
	Assignee     []FacetValue `json:"assignee"`
}

// Assignee is a member who can work field tasks.
type Assignee struct {
	UUID        uuid.UUID   `json:"uuid"`
	Name        string      `json:"name"`
	Surname     string      `json:"surname"`
	Email       string      `json:"email"`
	BranchScope string      `json:"branch_scope"`
	BranchUUIDs []uuid.UUID `json:"branch_uuids"`
}

// Filter selects a page of tasks.
type Filter struct {
	Q                 string
	Statuses          []string
	TaskTypes         []string
	LocationKinds     []string
	BranchUUIDs       []uuid.UUID
	AssigneeUUIDs     []uuid.UUID
	IncludeUnassigned bool
	Unassigned        *bool
	ReservationUUIDs  []uuid.UUID
	DueFrom, DueTo    *time.Time
	SortField         string
	SortDesc          bool
	Limit, Offset     int32
	// AssigneeID is set by the use case for "my tasks" (never from the
	// request).
	AssigneeID int64
}

// QueryKeys are the list filter keys.
var QueryKeys = []string{"status", "task_type", "location_kind", "branch", "assignee", "unassigned", "reservation", "due_from", "due_to"}

// Sortable lists the sortable fields.
var Sortable = map[string]bool{"due_at": true, "status": true, "task_type": true, "created_at": true}

// FilterFromQuery builds a filter from comma-joined query values.
// "assignee=none" (alone or with uuids) includes unassigned tasks.
func FilterFromQuery(v map[string]string) Filter {
	f := Filter{Q: strings.TrimSpace(v["q"])}
	f.Statuses = allowed(split(v["status"]), Statuses)
	f.TaskTypes = allowed(split(v["task_type"]), Types)
	f.LocationKinds = allowed(split(v["location_kind"]), LocationKinds)
	f.BranchUUIDs = uuids(split(v["branch"]))
	for _, raw := range split(v["assignee"]) {
		if raw == "none" {
			f.IncludeUnassigned = true
			continue
		}
		if id, err := uuid.Parse(raw); err == nil {
			f.AssigneeUUIDs = append(f.AssigneeUUIDs, id)
		}
	}
	if f.IncludeUnassigned && len(f.AssigneeUUIDs) == 0 {
		yes := true
		f.Unassigned, f.IncludeUnassigned = &yes, false
	}
	switch strings.TrimSpace(v["unassigned"]) {
	case "true":
		yes := true
		f.Unassigned = &yes
	case "false":
		no := false
		f.Unassigned = &no
	}
	f.ReservationUUIDs = uuids(split(v["reservation"]))
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v["due_from"])); err == nil {
		f.DueFrom = &t
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v["due_to"])); err == nil {
		f.DueTo = &t
	}
	return f
}

// Delivery is the delivery detail of a pick-up or a return (stored on
// quotes and reservations, copied onto the task).
type Delivery struct {
	Address      string   `json:"address"`
	District     string   `json:"district"`
	City         string   `json:"city"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	ContactName  string   `json:"contact_name"`
	ContactPhone string   `json:"contact_phone"`
	FlightNumber string   `json:"flight_number"`
	MeetingNote  string   `json:"meeting_note"`
}

// Normalize trims the strings and upper-cases the flight number.
func (d *Delivery) Normalize() {
	if d == nil {
		return
	}
	d.Address = strings.TrimSpace(d.Address)
	d.District = strings.TrimSpace(d.District)
	d.City = strings.TrimSpace(d.City)
	d.ContactName = strings.TrimSpace(d.ContactName)
	d.ContactPhone = strings.TrimSpace(d.ContactPhone)
	d.FlightNumber = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(d.FlightNumber), " ", ""))
	d.MeetingNote = strings.TrimSpace(d.MeetingNote)
}

// Empty reports whether nothing was given.
func (d *Delivery) Empty() bool {
	return d == nil || (d.Address == "" && d.District == "" && d.City == "" && d.Latitude == nil && d.Longitude == nil &&
		d.ContactName == "" && d.ContactPhone == "" && d.FlightNumber == "" && d.MeetingNote == "")
}

// CreateInput opens a task by hand.
type CreateInput struct {
	TaskType       string     `json:"task_type"`
	BranchUUID     *uuid.UUID `json:"branch_uuid"`
	ToBranchUUID   *uuid.UUID `json:"to_branch_uuid"`
	VehicleUUID    *uuid.UUID `json:"vehicle_uuid"`
	DueAt          *time.Time `json:"due_at"`
	PlannedMinutes *int32     `json:"planned_minutes"`
	AssigneeUUID   *uuid.UUID `json:"assignee_uuid"`
	Note           string     `json:"note"`
	PlaceInput
}

// PlaceInput is the place, contact and meeting details of a task.
type PlaceInput struct {
	PointUUID    *uuid.UUID `json:"point_uuid"`
	Address      *string    `json:"address"`
	District     *string    `json:"district"`
	City         *string    `json:"city"`
	Latitude     *float64   `json:"latitude"`
	Longitude    *float64   `json:"longitude"`
	ContactName  *string    `json:"contact_name"`
	ContactPhone *string    `json:"contact_phone"`
	FlightNumber *string    `json:"flight_number"`
	MeetingNote  *string    `json:"meeting_note"`
}

// PatchInput re-plans a live task (nil = unchanged; clear_point drops the
// point, clear_address the address).
type PatchInput struct {
	DueAt          *time.Time `json:"due_at"`
	PlannedMinutes *int32     `json:"planned_minutes"`
	Note           *string    `json:"note"`
	ClearPoint     bool       `json:"clear_point"`
	PlaceInput
}

// AssignInput assigns a member (null = unassign).
type AssignInput struct {
	UserUUID *uuid.UUID `json:"user_uuid"`
}

// NoteInput carries a note (an issue needs one, so does a cancellation).
type NoteInput struct {
	Note string `json:"note"`
}

// DeliveryPoint is a delivery point as the wizard offers it.
type DeliveryPoint struct {
	UUID             uuid.UUID  `json:"uuid"`
	Name             string     `json:"name"`
	Type             string     `json:"type"`
	Description      string     `json:"description"`
	Branch           Ref        `json:"branch"`
	Fee              string     `json:"fee"`
	FeeSource        string     `json:"fee_source"`
	DeliveryExtra    *Ref       `json:"delivery_extra"`
	RequiresAddress  bool       `json:"requires_address"`
	ServiceDistricts []string   `json:"service_districts"`
	MinNoticeMinutes int32      `json:"min_notice_minutes"`
	Conditions       string     `json:"conditions"`
	OnlineSelectable bool       `json:"online_selectable"`
	WorkingHours     any        `json:"working_hours"`
	ExtraUUID        *uuid.UUID `json:"-"`
}

func split(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func allowed(in, set []string) []string {
	out := []string{}
	for _, v := range in {
		if contains(set, v) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func uuids(in []string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range in {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Contains reports whether v is in set.
func Contains(set []string, v string) bool { return contains(set, v) }
