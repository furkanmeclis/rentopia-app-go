package usecase

import (
	"context"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Event actions of the reservation timeline (besides the transition
// actions of the state machine).
const (
	EventCreated          = "created"
	EventChanged          = "changed"
	EventExtended         = "extended"
	EventUpgraded         = "upgraded"
	EventPlateChanged     = "plate_changed"
	EventPlanMoved        = "plan_moved"
	EventReplanned        = "replanned"
	EventPlanBroken       = "plan_broken"
	EventDriversChanged   = "drivers_changed"
	EventAssigned         = "assigned"
	EventPaymentRecorded  = "payment_recorded"
	EventEquipmentWarning = "equipment_warning"
	EventCouponWarning    = "coupon_warning"
	EventApproved         = "approved"
)

// event writes one timeline entry.
func event(ctx context.Context, q *db.Queries, orgID, reservationID int64, action string, from, to, reason string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	return q.InsertReservationEvent(ctx, db.InsertReservationEventParams{
		OrganizationID: orgID, ReservationID: reservationID, Action: action, FromStatus: textOf(from), ToStatus: textOf(to),
		Reason: reason, Data: mustJSON(data), ActorID: int8Ptr(actorID(ctx)),
	})
}

// transition is one status change.
type transition struct {
	id             int64
	from, to       string
	action, reason string
	data           map[string]any
	reviewDeadline *time.Time
	payDeadline    *time.Time
	allocationID   int64
}

// apply moves the status optimistically (ErrConcurrent when someone else
// moved it first) and writes the event.
func (s *Service) apply(ctx context.Context, q *db.Queries, orgID int64, t transition) error {
	if !model.CanTransition(t.from, t.action, t.to) {
		return stateErr("invalid_transition", "the reservation cannot go from "+t.from+" to "+t.to)
	}
	now := s.clock()
	p := db.TransitionReservationParams{
		OrganizationID: orgID, ID: t.id, FromStatus: t.from, ToStatus: t.to,
		CapacityCommitment: model.CommitmentOf(t.to), ReviewDeadline: tsPtr(t.reviewDeadline), PaymentDeadline: tsPtr(t.payDeadline),
		AllocationID: int8Of(t.allocationID),
	}
	if t.to == model.StatusConfirmed {
		p.ConfirmedAt = ts(now)
	}
	if model.IsTerminal(t.to) {
		p.CancelledAt = ts(now)
		p.CancelReason = t.reason
		if p.CancelReason == "" {
			p.CancelReason = t.to
		}
	}
	n, err := q.TransitionReservation(ctx, p)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConcurrent
	}
	if model.IsTerminal(t.to) {
		if _, err := q.ResolveReservationTasks(ctx, db.ResolveReservationTasksParams{
			OrganizationID: orgID, ReservationID: t.id, ResolvedAt: ts(now), ResolvedBy: int8Ptr(actorID(ctx)),
			ResolutionNote: "reservation " + t.to,
		}); err != nil {
			return err
		}
	}
	if err := event(ctx, q, orgID, t.id, t.action, t.from, t.to, t.reason, t.data); err != nil {
		return err
	}
	return s.notifyTransition(ctx, q, orgID, t)
}

// planEffects records what an engine write did to OTHER reservations: the
// allocations it moved (event plan_moved, the engine's re-plan succeeded)
// and the ones it could not keep valid (event plan_broken + a resolution
// task; the reservation is never cancelled automatically).
func (s *Service) planEffects(ctx context.Context, q *db.Queries, orgID int64, r availmodel.HookResult, cause string, causeRef uuid.UUID) ([]uuid.UUID, error) {
	touched := []uuid.UUID{}
	for _, m := range r.Moved {
		res, err := q.GetReservationByAllocation(ctx, db.GetReservationByAllocationParams{OrganizationID: orgID, AllocationID: pgtype.Int8{Int64: m.ID, Valid: true}})
		if notFound(err) {
			continue // a plain hold, not a reservation
		}
		if err != nil {
			return nil, err
		}
		from, _ := q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: m.FromVehicleID})
		to, _ := q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: m.ToVehicleID})
		if err := event(ctx, q, orgID, res.ID, EventPlanMoved, "", "", "", map[string]any{
			"cause": cause, "cause_uuid": causeRef, "from_plate": from.Plate, "to_plate": to.Plate,
		}); err != nil {
			return nil, err
		}
		touched = append(touched, res.Uuid)
	}
	for _, a := range r.Affected {
		if a.Kind != availmodel.KindReservation && a.Kind != availmodel.KindHold {
			continue
		}
		res, err := q.GetReservationByAllocationUUID(ctx, db.GetReservationByAllocationUUIDParams{OrganizationID: orgID, AllocationUuid: a.UUID})
		if notFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := s.openTask(ctx, q, orgID, res.ID, model.TaskReleaseConflict, "a released commitment left this reservation without a valid plate plan",
			cause, causeRef, map[string]any{"allocation_uuid": a.UUID}); err != nil {
			return nil, err
		}
		touched = append(touched, res.Uuid)
	}
	return touched, nil
}

// PlanMoved records reassignments the engine made outside a reservation
// flow (a vehicle leaving a rental group: its commitments were re-planned
// onto other vehicles) as plan_moved events on the reservations moved. It
// runs in the engine's transaction (availability PlanMovedHandler).
func (s *Service) PlanMoved(ctx context.Context, q *db.Queries, orgID int64, moved []availmodel.MovedAllocation, cause string, causeRef uuid.UUID) error {
	_, err := s.planEffects(ctx, q, orgID, availmodel.HookResult{Moved: moved}, cause, causeRef)
	return err
}

// openTask opens a resolution task (at most one open per reservation, type
// and source) and records plan_broken on the reservation.
func (s *Service) openTask(ctx context.Context, q *db.Queries, orgID, reservationID int64, taskType, reason, sourceType string, source uuid.UUID, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	src := pgtype.UUID{}
	if source != uuid.Nil {
		src = pgtype.UUID{Bytes: source, Valid: true}
	}
	id, err := q.InsertResolutionTask(ctx, db.InsertResolutionTaskParams{
		OrganizationID: orgID, ReservationID: reservationID, TaskType: taskType, Reason: reason,
		SourceType: sourceType, SourceUuid: src, Data: mustJSON(data),
	})
	if notFound(err) {
		return nil // an open task for this source exists already
	}
	if err != nil {
		return err
	}
	data["task_uuid"] = id
	data["task_type"] = taskType
	return event(ctx, q, orgID, reservationID, EventPlanBroken, "", "", reason, data)
}

// assignment records a plate / product assignment (ending the current one).
func assignment(ctx context.Context, q *db.Queries, orgID, reservationID, vehicleID, productID int64, version int32, fixed bool, source, note string) error {
	if err := q.EndVehicleAssignments(ctx, db.EndVehicleAssignmentsParams{OrganizationID: orgID, ReservationID: reservationID}); err != nil {
		return err
	}
	kind := model.AssignmentPlanned
	if fixed {
		kind = model.AssignmentFixed
	}
	return q.InsertVehicleAssignment(ctx, db.InsertVehicleAssignmentParams{
		OrganizationID: orgID, ReservationID: reservationID, VehicleID: int8Of(vehicleID), RentalProductID: productID,
		ProductVersion: version, Kind: kind, Source: source, Note: note, ActorID: int8Ptr(actorID(ctx)),
	})
}
