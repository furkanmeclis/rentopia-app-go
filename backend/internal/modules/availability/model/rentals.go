package model

import (
	"time"

	"github.com/google/uuid"
)

// RentalEnd moves the end of a rental allocation (TEC-709): the actual
// return (time and branch), an early return, or the overdue extension of a
// vehicle that is not back yet.
type RentalEnd struct {
	// EndsAt is the new customer end (the actual or expected return).
	EndsAt time.Time
	// ReturnBranchID is the (actual) return branch; 0 keeps the current one.
	// Its preparation minutes apply after EndsAt.
	ReturnBranchID int64
	// ReturnPointID replaces the return point when SetPoint is true (0 =
	// none).
	ReturnPointID int64
	SetPoint      bool
	// Force records the facts even when no plan keeps every later
	// commitment valid (a physical return always happens): the period is
	// clipped before the next commitment of the vehicle and what the change
	// broke is reported in RentalEndResult.Affected.
	Force bool
	Hook  TxHook
}

// RentalEndResult is the answer of SetRentalEnd.
type RentalEndResult struct {
	Allocation Allocation
	// Affected are later commitments of the vehicle the change left
	// without a valid plan (Force only); the caller re-plans them or opens
	// resolution tasks.
	Affected []Affected
	// Clipped: the period had to end before the next commitment (the
	// vehicle came back later than that commitment could wait).
	Clipped bool
}

// RentalSwap moves a rental under way to another vehicle (mid-rental
// swap): the current allocation ends at At (returned at BranchID, its
// preparation applies) and a new rental allocation on VehicleUUID carries
// the rest of the range with a promised plate. Both writes commit together.
type RentalSwap struct {
	VehicleUUID uuid.UUID
	At          time.Time
	BranchID    int64
	ActorID     *int64
	Note        string
	Hook        TxHook
}
