package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/placement"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ReservationChanged implements the reservation lifecycle listener
// (after rentals): a confirmed reservation with a delivery gets its
// delivery (handover) task and, when the return is a delivery too, its
// collection (return) task; a change re-plans them (time, place, branch),
// dropping the delivery cancels a task that has no rental yet, and a
// cancelled, expired or no-show reservation cancels them. Rental tasks
// follow the reservation's place too (placement).
func (s *Service) ReservationChanged(ctx context.Context, orgID int64, reservation uuid.UUID, _ string) {
	if err := s.SyncReservation(ctx, orgID, reservation); err != nil {
		s.log.Warn("field_tasks_sync_failed", "reservation", reservation, "error", err)
	}
}

// Rental statuses after which a side is done.
const (
	rentalReturnCheck = "return_check"
	rentalReturned    = "returned"
)

// SyncReservation brings the delivery tasks of a reservation in line with
// it (idempotent).
func (s *Service) SyncReservation(ctx context.Context, orgID int64, reservation uuid.UUID) error {
	sys := unscoped(ctx)
	res, err := s.q.GetFieldReservation(sys, db.GetFieldReservationParams{OrganizationID: orgID, Uuid: reservation})
	if notFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	actor := actorArg(ctx)
	var created, updated []uuid.UUID
	switch res.Status {
	case resmodel.StatusCancelled, resmodel.StatusExpired, resmodel.StatusNoShow:
		ids, err := s.q.CancelReservationFieldTasks(sys, db.CancelReservationFieldTasksParams{
			OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: res.ID, Valid: true}, At: ts(s.clock()), ActorID: actor, Note: "reservation " + res.Status,
		})
		if err != nil {
			return err
		}
		s.publish(ctx, "updated", ids...)
		return nil
	case resmodel.StatusConfirmed:
	default:
		// Waiting reservations get their tasks once confirmed.
		return nil
	}
	live, err := s.q.ListReservationFieldTasks(sys, db.ListReservationFieldTasksParams{OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: res.ID, Valid: true}})
	if err != nil {
		return err
	}
	byType := map[string]db.ListReservationFieldTasksRow{}
	for _, t := range live {
		byType[t.TaskType] = t
	}
	rental := res.RentalStatus.String
	sides := []struct {
		typ      string
		place    placement.Place
		branchID int64
		due      pgtype.Timestamptz
		done     bool
	}{
		{
			model.TypeHandover, placement.Of(res.PickupPointID, res.PickupPointDelivery, res.PickupDelivery), res.PickupBranchID,
			res.StartsAt, rental == rentalActive || rental == rentalReturnCheck || rental == rentalReturned,
		},
		{
			model.TypeReturn, placement.Of(res.ReturnPointID, res.ReturnPointDelivery, res.ReturnDelivery), res.ReturnBranchID,
			res.EndsAt, rental == rentalReturnCheck || rental == rentalReturned,
		},
	}
	err = s.tx(sys, func(q *db.Queries) error {
		for _, side := range sides {
			if side.done {
				continue
			}
			existing, ok := byType[side.typ]
			switch {
			case ok && !side.place.Delivery && !existing.RentalID.Valid && existing.Origin == model.OriginReservation:
				// The delivery was dropped before the rental: nothing to do there.
				ids, err := q.CancelReservationFieldTasks(sys, db.CancelReservationFieldTasksParams{
					OrganizationID: orgID, ReservationID: pgtype.Int8{Int64: res.ID, Valid: true}, At: ts(s.clock()), ActorID: actor,
					Note: "delivery removed from the reservation", TaskTypes: []string{side.typ},
				})
				if err != nil {
					return err
				}
				updated = append(updated, ids...)
			case ok:
				if err := q.SetOperationTaskPlace(sys, side.place.Params(orgID, existing.ID, actor)); err != nil {
					return err
				}
				if err := q.RescheduleFieldTask(sys, db.RescheduleFieldTaskParams{
					OrganizationID: orgID, ID: existing.ID, DueAt: side.due, BranchID: side.branchID, ActorID: actor,
				}); err != nil {
					return err
				}
				updated = append(updated, existing.Uuid)
			case side.place.Delivery:
				p := side.place
				row, err := q.InsertFieldTask(sys, db.InsertFieldTaskParams{
					OrganizationID: orgID, TaskType: side.typ, Status: model.StatusPlanned, Origin: model.OriginReservation,
					BranchID: side.branchID, ReservationID: pgtype.Int8{Int64: res.ID, Valid: true}, DueAt: side.due,
					PlannedMinutes: defaultMinutes[side.typ], LocationKind: p.Kind, PointID: p.PointID, Address: p.Address,
					District: p.District, City: p.City, Latitude: p.Latitude, Longitude: p.Longitude,
					ContactName: p.ContactName, ContactPhone: p.ContactPhone, FlightNumber: p.FlightNumber,
					MeetingNote: p.MeetingNote, CreatedBy: actor,
				})
				if notFound(err) {
					continue
				}
				if err != nil {
					return err
				}
				created = append(created, row.Uuid)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.publish(ctx, "created", created...)
	s.publish(ctx, "updated", updated...)
	return nil
}
