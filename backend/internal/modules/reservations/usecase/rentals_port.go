package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The narrow surface rentals (TEC-709) and contracts (TEC-710) use; see
// docs/design/rentals.md. Reservations stay the owner of the plan, the
// price and the resolution tasks.

// Reservation lifecycle events reported to the listener.
const (
	LifecyclePlateFixed     = "plate_fixed"
	LifecycleConfirmed      = "confirmed"
	LifecycleTerminated     = "terminated"
	LifecycleChanged        = "changed"
	LifecycleDriversChanged = "drivers_changed"
)

// LifecycleListener hears reservation changes after they committed
// (rentals: create the rental once the plate is fixed on a confirmed
// reservation, follow cancellations; contracts: re-render documents when
// the people or the terms change). Errors are the listener's to log.
type LifecycleListener interface {
	ReservationChanged(ctx context.Context, orgID int64, reservation uuid.UUID, event string)
}

// SetLifecycleListener wires the listener (nil = none).
func (s *Service) SetLifecycleListener(l LifecycleListener) { s.listener = l }

func (s *Service) notify(ctx context.Context, orgID int64, id uuid.UUID, event string) {
	if s.listener != nil {
		s.listener.ReservationChanged(ctx, orgID, id, event)
	}
}

// RecordPlanEffects records, inside the caller's engine transaction, what
// an engine write did to other reservations (plan_moved events, release
// conflicts as resolution tasks). Rentals call it from their engine hooks.
func (s *Service) RecordPlanEffects(ctx context.Context, tx pgx.Tx, orgID int64, r availmodel.HookResult, cause string, causeRef uuid.UUID) ([]uuid.UUID, error) {
	return s.planEffects(ctx, s.q.WithTx(tx), orgID, r, cause, causeRef)
}

// LateReturnTasks opens a late_return resolution task on every reservation
// whose allocation a late vehicle runs into and the engine could not
// re-plan (the planner already tried to move them when it extended the
// rental). The source is the rental; at most one open task per reservation
// and rental. Returns the reservations that got a task.
func (s *Service) LateReturnTasks(ctx context.Context, orgID int64, rental uuid.UUID, allocations []uuid.UUID, data map[string]any) []uuid.UUID {
	out := []uuid.UUID{}
	sys := unscoped(ctx)
	for _, alloc := range allocations {
		res, err := s.q.GetReservationByAllocationUUID(sys, db.GetReservationByAllocationUUIDParams{OrganizationID: orgID, AllocationUuid: alloc})
		if err != nil || !model.HoldsCapacity(res.Status) {
			continue
		}
		payload := map[string]any{"allocation_uuid": alloc, "rental_uuid": rental}
		for k, v := range data {
			payload[k] = v
		}
		err = s.tx(sys, func(q *db.Queries, _ pgxTx) error {
			return s.openTask(sys, q, orgID, res.ID, model.TaskLateReturn,
				"the vehicle of an earlier rental is late and no other vehicle can take this reservation", "rental", rental, payload)
		})
		if err == nil {
			out = append(out, res.Uuid)
		}
	}
	if len(out) > 0 {
		s.publish(ctx, model.ResourceReservations, "updated", out...)
		s.publish(ctx, model.ResourceTasks, "created", out...)
	}
	return out
}
