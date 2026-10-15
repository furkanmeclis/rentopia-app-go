package model

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Fleet calendar (TEC-696): GET /v1/tenant/fleet/calendar answers one page
// of vehicles (rows) with what keeps each one busy inside [from, to) (bars).
// Bars come from the vehicle_busy_intervals view (blocks, transfers and the
// availability engine's allocations: holds / reservations, TEC-697) plus
// maintenance days (vehicle_maintenance).

// Calendar layers. Blocks and transfers need their own read permission,
// holds and reservations tenant.availability.read; maintenance is part of
// the vehicle (tenant.vehicles.read).
const (
	LayerBlock       = "block"
	LayerTransfer    = "transfer"
	LayerHold        = "hold"
	LayerReservation = "reservation"
	LayerMaintenance = "maintenance"
)

// Calendar limits.
const (
	// CalendarMaxDays is the widest window one request may ask for (a
	// quarter: the month zoom with a little slack on both sides).
	CalendarMaxDays = 93
	// CalendarDefaultLimit / CalendarMaxLimit are the vehicle page sizes.
	CalendarDefaultLimit = 50
	CalendarMaxLimit     = 200
)

// Maintenance marker states on the calendar.
const (
	MaintenanceStateScheduled = "scheduled"
	MaintenanceStateDue       = "due"
	MaintenanceStateOverdue   = "overdue"
	MaintenanceStateDone      = "done"
)

// CalendarQueryKeys are the vehicle filters of the calendar (comma-separated
// multi values, same meaning as the vehicles list).
var CalendarQueryKeys = []string{"status", "home_branch", "current_branch", "rental_group", "segment", "vehicle", "is_active"}

// CalendarFilter selects the window and the vehicle page.
type CalendarFilter struct {
	From, To           time.Time
	Limit, Offset      int32
	Q                  string
	VehicleUUIDs       []uuid.UUID
	Statuses           []string
	HomeBranchUUIDs    []uuid.UUID
	CurrentBranchUUIDs []uuid.UUID
	GroupUUIDs         []uuid.UUID
	SegmentUUIDs       []uuid.UUID
	IsActive           *bool
	// BranchIDs is the caller's branch scope (set by the use case).
	BranchIDs []int64
}

// CalendarVehicle is one row of the calendar.
type CalendarVehicle struct {
	UUID          uuid.UUID `json:"uuid"`
	Plate         string    `json:"plate"`
	Label         string    `json:"label"`
	ModelYear     int16     `json:"model_year"`
	Status        string    `json:"status"`
	IsActive      bool      `json:"is_active"`
	HomeBranch    BranchRef `json:"home_branch"`
	CurrentBranch BranchRef `json:"current_branch"`
}

// CalendarBlock are the block details of a block bar.
type CalendarBlock struct {
	Kind       string     `json:"kind"`
	Reason     string     `json:"reason"`
	ReleasedAt *time.Time `json:"released_at"`
}

// CalendarTransfer are the transfer details of a transfer bar: the planned
// range next to the actual departure / arrival.
type CalendarTransfer struct {
	Status           string     `json:"status"`
	PlannedDeparture time.Time  `json:"planned_departure"`
	PlannedArrival   time.Time  `json:"planned_arrival"`
	ActualDeparture  *time.Time `json:"actual_departure"`
	ActualArrival    *time.Time `json:"actual_arrival"`
	FromBranch       BranchRef  `json:"from_branch"`
	ToBranch         BranchRef  `json:"to_branch"`
	// InScope: the caller may open the transfer (its from OR to branch is
	// in the caller's branch scope). The bar is shown either way because
	// it belongs to a visible vehicle.
	InScope bool `json:"in_scope"`
}

// CalendarAllocation are the details of a hold / reservation bar. The bar
// itself spans the period including the preparation buffer; StartsAt /
// EndsAt are the customer range.
type CalendarAllocation struct {
	Product      ProductRef `json:"product"`
	PickupBranch BranchRef  `json:"pickup_branch"`
	ReturnBranch BranchRef  `json:"return_branch"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       time.Time  `json:"ends_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
	PlateFixed   bool       `json:"plate_fixed"`
	Channel      string     `json:"channel"`
	// Reservation is the reservation the allocation belongs to (TEC-706);
	// nil for a plain hold. Its bar may be dragged onto another vehicle
	// row while the plate is not fixed (POST …/reservations/{uuid}/plate).
	Reservation *CalendarReservation `json:"reservation"`
}

// CalendarReservation is the reservation of an allocation bar.
type CalendarReservation struct {
	UUID      uuid.UUID `json:"uuid"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
}

// CalendarInterval is one bar. StartsAt / EndsAt are the real bounds (EndsAt
// nil = open-ended block); ClipStart / ClipEnd are the bounds inside the
// requested window, ready to draw.
type CalendarInterval struct {
	Kind      string     `json:"kind"`
	UUID      uuid.UUID  `json:"uuid"`
	StartsAt  time.Time  `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
	ClipStart time.Time  `json:"clip_start"`
	ClipEnd   time.Time  `json:"clip_end"`
	// ContinuesBefore / ContinuesAfter: the interval runs past the window.
	ContinuesBefore bool                `json:"continues_before"`
	ContinuesAfter  bool                `json:"continues_after"`
	Block           *CalendarBlock      `json:"block"`
	Transfer        *CalendarTransfer   `json:"transfer"`
	Allocation      *CalendarAllocation `json:"allocation"`
}

// CalendarMaintenance is a maintenance day marker (open plan or done item).
type CalendarMaintenance struct {
	UUID            uuid.UUID `json:"uuid"`
	Kind            string    `json:"kind"`
	MaintenanceType string    `json:"maintenance_type"`
	// Date is the planned day (open plans) or the done day (yyyy-MM-dd).
	Date            string `json:"date"`
	State           string `json:"state"`
	PlannedKm       *int32 `json:"planned_km"`
	DoneKm          *int32 `json:"done_km"`
	ServiceProvider string `json:"service_provider"`
	Note            string `json:"note"`
}

// CalendarRow is a vehicle with its bars.
type CalendarRow struct {
	Vehicle     CalendarVehicle       `json:"vehicle"`
	Intervals   []CalendarInterval    `json:"intervals"`
	Maintenance []CalendarMaintenance `json:"maintenance"`
}

// Calendar is the answer of GET /v1/tenant/fleet/calendar.
type Calendar struct {
	From   time.Time     `json:"from"`
	To     time.Time     `json:"to"`
	Items  []CalendarRow `json:"items"`
	Total  int64         `json:"total"`
	Limit  int32         `json:"limit"`
	Offset int32         `json:"offset"`
	// Layers are the bar kinds the caller may see (block / transfer need
	// tenant.blocks.read / tenant.transfers.read, hold / reservation
	// tenant.availability.read).
	Layers []string `json:"layers"`
}

// CalendarFilterFromQuery maps the calendar query params (the window is
// parsed by the handler) to a filter; multi-value params are
// comma-separated.
func CalendarFilterFromQuery(query map[string]string) CalendarFilter {
	f := CalendarFilter{
		Q:                  strings.TrimSpace(query["q"]),
		VehicleUUIDs:       SplitUUIDs(query["vehicle"]),
		Statuses:           SplitList(query["status"]),
		HomeBranchUUIDs:    SplitUUIDs(query["home_branch"]),
		CurrentBranchUUIDs: SplitUUIDs(query["current_branch"]),
		GroupUUIDs:         SplitUUIDs(query["rental_group"]),
		SegmentUUIDs:       SplitUUIDs(query["segment"]),
	}
	if v, err := strconv.ParseBool(strings.TrimSpace(query["is_active"])); err == nil {
		f.IsActive = &v
	}
	return f
}
