package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/google/uuid"
)

// Document kinds the rentals flow asks contracts for (contracts module).
const (
	DocRentalContract = "rental_contract"
	DocHandoverForm   = "handover_form"
	DocReturnForm     = "return_form"
)

// reservationFacts loads the reservation a rental is created from, in the
// caller's branch scope.
func (s *Service) reservationFacts(ctx context.Context, orgID int64, id uuid.UUID) (db.GetRentalReservationFactsRow, error) {
	row, err := s.q.GetRentalReservationFacts(ctx, db.GetRentalReservationFactsParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) || (err == nil && !inScope(ctx, row.PickupBranchID, row.ReturnBranchID)) {
		return row, validation("reservation_uuid", "not_found", "reservation was not found")
	}
	return row, err
}

// Create opens the rental of a confirmed reservation. vehicle_uuid fixes
// that plate first (the reservation's plan moves through the engine);
// without it the reservation's current plate is fixed. Fixing the plate is
// what creates the rental (the reservation lifecycle listener); Create
// returns it.
func (s *Service) Create(ctx context.Context, orgID int64, in model.CreateInput) (model.Rental, error) {
	if in.ReservationUUID == nil {
		return model.Rental{}, validation("reservation_uuid", "required", "reservation_uuid is required")
	}
	if len(in.Note) > 2000 {
		return model.Rental{}, validation("note", "too_long", "note is at most 2000 characters")
	}
	res, err := s.reservationFacts(ctx, orgID, *in.ReservationUUID)
	if err != nil {
		return model.Rental{}, err
	}
	if res.Status != "confirmed" {
		return model.Rental{}, stateErr("reservation_not_confirmed", "only a confirmed reservation can become a rental")
	}
	if existing, err := s.q.GetLiveRentalUUIDByReservation(ctx, db.GetLiveRentalUUIDByReservationParams{OrganizationID: orgID, ReservationID: res.ID}); err == nil {
		if in.VehicleUUID == nil {
			return s.Get(ctx, orgID, existing)
		}
	}
	if !res.AllocationStatus.Valid || res.AllocationStatus.String != availmodel.StatusActive {
		return model.Rental{}, stateErr("allocation_not_active", "the reservation does not hold a vehicle")
	}
	vehicle := in.VehicleUUID
	if vehicle == nil {
		v, err := s.q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: res.AllocationVehicleID.Int64})
		if err != nil {
			return model.Rental{}, err
		}
		vehicle = &v.Uuid
	}
	needFix := !res.AllocationPlateFixed.Bool || in.VehicleUUID != nil
	if needFix {
		// The listener creates the rental once the plate is fixed.
		if err := s.reservations.FixPlate(ctx, orgID, res.Uuid, *vehicle); err != nil {
			return model.Rental{}, err
		}
	}
	id, _, err := s.ensure(ctx, orgID, res.Uuid, in.Note)
	if err != nil {
		return model.Rental{}, err
	}
	if id == uuid.Nil {
		return model.Rental{}, stateErr("not_ready", "the reservation is not ready for a rental (confirmed, plate fixed)")
	}
	return s.Get(ctx, orgID, id)
}

// ensure creates the rental of a reservation that is confirmed and has a
// fixed plate (idempotent: the live rental is returned). A rental still in
// preparation follows a plate change. Returns uuid.Nil when the reservation
// is not ready.
func (s *Service) ensure(ctx context.Context, orgID int64, resUUID uuid.UUID, note string) (uuid.UUID, bool, error) {
	sys := unscoped(ctx)
	res, err := s.q.GetRentalReservationFacts(sys, db.GetRentalReservationFactsParams{OrganizationID: orgID, Uuid: resUUID})
	if err != nil {
		return uuid.Nil, false, err
	}
	ready := res.Status == "confirmed" && res.AllocationStatus.Valid && res.AllocationStatus.String == availmodel.StatusActive &&
		res.AllocationPlateFixed.Bool
	existing, err := s.q.GetLiveRentalUUIDByReservation(sys, db.GetLiveRentalUUIDByReservationParams{OrganizationID: orgID, ReservationID: res.ID})
	if err == nil {
		if ready {
			s.followPlate(sys, orgID, existing, res)
		}
		return existing, false, nil
	}
	if !notFound(err) {
		return uuid.Nil, false, err
	}
	if !ready {
		return uuid.Nil, false, nil
	}
	var created uuid.UUID
	var taskIDs []uuid.UUID
	err = s.tx(sys, func(q *db.Queries) error {
		row, err := q.InsertRental(sys, db.InsertRentalParams{
			OrganizationID: orgID, Reference: res.Reference, ReservationID: res.ID, VehicleID: res.AllocationVehicleID.Int64,
			AllocationID: res.AllocationID, SettingsVersion: res.SettingsVersion, PolicySnapshot: res.PolicySnapshot,
			Note: note, CreatedBy: int8Ptr(actorID(ctx)),
		})
		if err != nil {
			return err
		}
		created = row.Uuid
		if err := q.CopyReservationDriversToRental(sys, db.CopyReservationDriversToRentalParams{
			OrganizationID: orgID, RentalID: row.ID, ReservationID: res.ID,
		}); err != nil {
			return err
		}
		v, _ := q.GetReservationVehicleByID(sys, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: res.AllocationVehicleID.Int64})
		if err := event(ctx, q, orgID, row.ID, model.EventCreated, "", model.StatusPreparation, "", map[string]any{
			"reservation_uuid": res.Uuid, "plate": v.Plate,
		}); err != nil {
			return err
		}
		taskIDs, err = s.openPickupTasks(sys, q, orgID, row.ID, res.AllocationVehicleID.Int64, res.PickupBranchID, res.StartsAt.Time)
		return err
	})
	if uniqueViolation(err, "uq_rentals_organization_reservation") {
		id, gerr := s.q.GetLiveRentalUUIDByReservation(sys, db.GetLiveRentalUUIDByReservationParams{OrganizationID: orgID, ReservationID: res.ID})
		return id, false, gerr
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	s.prepareDocuments(ctx, orgID, res.ID, created, []string{DocRentalContract}, "rental_created")
	s.publish(ctx, model.ResourceRentals, "created", created)
	s.publish(ctx, model.ResourceOperationTasks, "created", taskIDs...)
	return created, true, nil
}

// openPickupTasks opens the preparation and handover tasks of a rental.
func (s *Service) openPickupTasks(ctx context.Context, q *db.Queries, orgID, rentalID, vehicleID, branchID int64, pickupAt time.Time) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	prep := 60
	if b, err := q.GetRentalBranchByID(ctx, db.GetRentalBranchByIDParams{OrganizationID: orgID, ID: branchID}); err == nil && b.PrepMinutes > 0 {
		prep = int(b.PrepMinutes)
	}
	for _, t := range []struct {
		typ string
		due time.Time
	}{
		{model.TaskPreparation, pickupAt.Add(-time.Duration(prep) * time.Minute)},
		{model.TaskHandover, pickupAt},
	} {
		id, ok, err := openTask(ctx, q, orgID, t.typ, branchID, rentalID, vehicleID, t.due, "")
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// followPlate moves a rental still in preparation to the reservation's new
// fixed plate (its preparation / handover tasks follow).
func (s *Service) followPlate(ctx context.Context, orgID int64, id uuid.UUID, res db.GetRentalReservationFactsRow) {
	c, err := s.q.GetRentalCore(ctx, db.GetRentalCoreParams{OrganizationID: orgID, Uuid: id})
	if err != nil || c.Status != model.StatusPreparation || c.VehicleID == res.AllocationVehicleID.Int64 {
		return
	}
	var tasks []uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.SetRentalVehicle(ctx, db.SetRentalVehicleParams{
			OrganizationID: orgID, ID: c.ID, VehicleID: res.AllocationVehicleID.Int64, AllocationID: res.AllocationID,
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		to, _ := q.GetReservationVehicleByID(ctx, db.GetReservationVehicleByIDParams{OrganizationID: orgID, ID: res.AllocationVehicleID.Int64})
		if err := event(ctx, q, orgID, c.ID, model.EventPlateChanged, "", "", "", map[string]any{
			"from_plate": c.VehiclePlate, "to_plate": to.Plate,
		}); err != nil {
			return err
		}
		closed, err := q.FinishRentalTasks(ctx, db.FinishRentalTasksParams{
			OrganizationID: orgID, RentalID: int8Of(c.ID), Status: model.TaskCancelled, CompletedAt: ts(s.clock()),
			ActorID: int8Ptr(actorID(ctx)), CompletionNote: "plate changed",
			TaskTypes: []string{model.TaskPreparation, model.TaskHandover},
		})
		if err != nil {
			return err
		}
		opened, err := s.openPickupTasks(ctx, q, orgID, c.ID, res.AllocationVehicleID.Int64, res.PickupBranchID, res.StartsAt.Time)
		tasks = append(closed, opened...)
		return err
	})
	if err != nil {
		slog.Default().Warn("rental_follow_plate_failed", "rental", id, "error", err)
		return
	}
	s.prepareDocuments(ctx, orgID, res.ID, id, nil, "plate_changed")
	s.publish(ctx, model.ResourceRentals, "updated", id)
	s.publish(ctx, model.ResourceOperationTasks, "updated", tasks...)
}

// prepareDocuments asks contracts to create / refresh documents (best
// effort) and records the new versions on the rental timeline.
func (s *Service) prepareDocuments(ctx context.Context, orgID, reservationID int64, rental uuid.UUID, kinds []string, reason string) {
	docs, err := s.documents.Prepare(unscoped(ctx), orgID, reservationID, kinds, reason)
	if err != nil {
		slog.Default().Warn("rental_documents_prepare_failed", "rental", rental, "reason", reason, "error", err)
		return
	}
	if len(docs) == 0 || rental == uuid.Nil {
		return
	}
	c, err := s.q.GetRentalCore(ctx, db.GetRentalCoreParams{OrganizationID: orgID, Uuid: rental})
	if err != nil {
		return
	}
	_ = s.tx(ctx, func(q *db.Queries) error {
		return event(ctx, q, orgID, c.ID, model.EventDocumentPrepared, "", "", reason, map[string]any{"documents": docs})
	})
}

// ReservationChanged implements the reservation lifecycle listener: a
// fixed plate on a confirmed reservation creates the rental (or moves a
// rental in preparation), a cancellation / no-show cancels a rental still in
// preparation, changes and driver changes are reflected and re-render the
// documents.
func (s *Service) ReservationChanged(ctx context.Context, orgID int64, reservation uuid.UUID, ev string) {
	sys := unscoped(ctx)
	switch ev {
	case resusecase.LifecyclePlateFixed, resusecase.LifecycleConfirmed:
		if _, _, err := s.ensure(ctx, orgID, reservation, ""); err != nil {
			slog.Default().Warn("rental_ensure_failed", "reservation", reservation, "error", err)
		}
		return
	}
	res, err := s.q.GetRentalReservationFacts(sys, db.GetRentalReservationFactsParams{OrganizationID: orgID, Uuid: reservation})
	if err != nil {
		return
	}
	id, err := s.q.GetLiveRentalUUIDByReservation(sys, db.GetLiveRentalUUIDByReservationParams{OrganizationID: orgID, ReservationID: res.ID})
	hasRental := err == nil
	switch ev {
	case resusecase.LifecycleTerminated:
		if hasRental {
			if _, err := s.cancel(sys, orgID, id, "reservation "+res.Status); err != nil {
				slog.Default().Warn("rental_cancel_after_reservation_failed", "rental", id, "error", err)
			}
		}
	case resusecase.LifecycleChanged, resusecase.LifecycleDriversChanged:
		if hasRental {
			s.reflectReservation(sys, orgID, id, res, ev)
		}
		s.prepareDocuments(ctx, orgID, res.ID, uuidOr(hasRental, id), nil, ev)
	}
}

func uuidOr(ok bool, id uuid.UUID) uuid.UUID {
	if ok {
		return id
	}
	return uuid.Nil
}

// reflectReservation records a reservation change on its rental: the
// timeline, the drivers (copied again) and the due times of live tasks.
func (s *Service) reflectReservation(ctx context.Context, orgID int64, id uuid.UUID, res db.GetRentalReservationFactsRow, ev string) {
	c, err := s.q.GetRentalCore(ctx, db.GetRentalCoreParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		action := model.EventReservationChanged
		if ev == resusecase.LifecycleDriversChanged {
			action = model.EventDriversChanged
			if err := q.DeleteRentalDrivers(ctx, db.DeleteRentalDriversParams{OrganizationID: orgID, RentalID: c.ID}); err != nil {
				return err
			}
			if err := q.CopyReservationDriversToRental(ctx, db.CopyReservationDriversToRentalParams{
				OrganizationID: orgID, RentalID: c.ID, ReservationID: res.ID,
			}); err != nil {
				return err
			}
		}
		if err := q.RescheduleRentalTasks(ctx, db.RescheduleRentalTasksParams{
			OrganizationID: orgID, RentalID: int8Of(c.ID), TaskType: model.TaskHandover, DueAt: res.StartsAt,
		}); err != nil {
			return err
		}
		if err := q.RescheduleRentalTasks(ctx, db.RescheduleRentalTasksParams{
			OrganizationID: orgID, RentalID: int8Of(c.ID), TaskType: model.TaskReturn, DueAt: res.EndsAt,
		}); err != nil {
			return err
		}
		return event(ctx, q, orgID, c.ID, action, "", "", "", map[string]any{
			"planned_pickup_at": res.StartsAt.Time.UTC(), "planned_return_at": res.EndsAt.Time.UTC(),
		})
	})
	if err != nil {
		slog.Default().Warn("rental_reflect_reservation_failed", "rental", id, "error", err)
		return
	}
	s.publish(ctx, model.ResourceRentals, "updated", id)
}

// openTask opens an operation task (no-op when the same live task exists).
func openTask(ctx context.Context, q *db.Queries, orgID int64, typ string, branchID, rentalID, vehicleID int64, due time.Time, note string) (uuid.UUID, bool, error) {
	id, err := q.InsertOperationTask(ctx, db.InsertOperationTaskParams{
		OrganizationID: orgID, TaskType: typ, BranchID: branchID, RentalID: int8Of(rentalID), VehicleID: int8Of(vehicleID),
		DueAt: ts(due), Note: note, CreatedBy: int8Ptr(actorID(ctx)),
	})
	if notFound(err) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

