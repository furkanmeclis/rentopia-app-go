package usecase

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// futureTolerance lets an "actual" time be a little ahead of the server
// clock (device clocks).
const futureTolerance = 5 * time.Minute

// core loads the lifecycle facts of a rental in the caller's branch scope.
func (s *Service) core(ctx context.Context, orgID int64, id uuid.UUID) (db.GetRentalCoreRow, error) {
	row, err := s.q.GetRentalCore(ctx, db.GetRentalCoreParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if !inScope(ctx, row.PlannedPickupBranchID, row.PlannedReturnBranchID, row.ActualReturnBranchID.Int64) {
		return row, ErrNotFound
	}
	return row, nil
}

// place resolves a branch (and an optional point of it) of the
// organization; nil branch = def.
func (s *Service) place(ctx context.Context, orgID int64, field string, branch, point *uuid.UUID, def int64) (db.GetRentalBranchRow, pgtype.Int8, error) {
	var b db.GetRentalBranchRow
	var err error
	if branch != nil {
		b, err = s.q.GetRentalBranch(ctx, db.GetRentalBranchParams{OrganizationID: orgID, Uuid: *branch})
		if notFound(err) {
			return b, pgtype.Int8{}, validation(field+"_uuid", "not_found", "branch was not found")
		}
	} else {
		var row db.GetRentalBranchByIDRow
		row, err = s.q.GetRentalBranchByID(ctx, db.GetRentalBranchByIDParams{OrganizationID: orgID, ID: def})
		b = db.GetRentalBranchRow(row)
	}
	if err != nil {
		return b, pgtype.Int8{}, err
	}
	if point == nil {
		return b, pgtype.Int8{}, nil
	}
	p, err := s.q.GetReservationPointByUUID(ctx, db.GetReservationPointByUUIDParams{OrganizationID: orgID, Uuid: *point})
	if notFound(err) || (err == nil && p.BranchID != b.ID) {
		return b, pgtype.Int8{}, validation("point_uuid", "not_found", "service point was not found at the branch")
	}
	if err != nil {
		return b, pgtype.Int8{}, err
	}
	return b, pgtype.Int8{Int64: p.ID, Valid: true}, nil
}

// actualTime defaults to now and refuses the future.
func (s *Service) actualTime(field string, at *time.Time) (time.Time, error) {
	now := s.clock()
	if at == nil {
		return now, nil
	}
	t := at.UTC().Truncate(time.Microsecond)
	if t.After(now.Add(futureTolerance)) {
		return t, validation(field, "in_future", "the time cannot be in the future")
	}
	return t, nil
}

func checkReading(ve *ValidationError, prefix string, km *int32, fuel *int16, minKm int32) {
	switch {
	case km == nil:
		ve.add(prefix+"km", "required", "the odometer reading is required")
	case *km < 0 || *km > 5000000:
		ve.add(prefix+"km", "out_of_range", "the odometer reading is out of range")
	case *km < minKm:
		ve.add(prefix+"km", "below_previous", "the odometer reading is below the previous one")
	}
	switch {
	case fuel == nil:
		ve.add(prefix+"fuel", "required", "the fuel level is required (0-100)")
	case *fuel < 0 || *fuel > 100:
		ve.add(prefix+"fuel", "out_of_range", "the fuel level is 0-100")
	}
}

// vehicleChange applies a vehicle status / location change in the caller's
// transaction (vehicle_events records km and fuel).
func (s *Service) vehicleChange(ctx context.Context, q *db.Queries, orgID int64, vehicle uuid.UUID, change vehicleusecase.OperationalChange) error {
	if s.vehicles == nil {
		return nil
	}
	_, err := s.vehicles.WithQueries(q).ApplyOperationalChange(unscoped(ctx), orgID, vehicle, change)
	return err
}

// Handover delivers the vehicle: km / fuel out, actual time and place; the
// reservation's allocation becomes a rental allocation (same row), the
// vehicle is rented, the preparation / handover tasks close and the return
// task opens. Without an approved rental contract (when the organization
// has an active rental contract template) the handover needs
// tenant.signature.waive and a reason.
func (s *Service) Handover(ctx context.Context, orgID int64, id uuid.UUID, in model.HandoverInput) (model.Rental, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Rental{}, err
	}
	if c.Status != model.StatusPreparation {
		return model.Rental{}, stateErr("invalid_transition", "only a rental in preparation can be handed over")
	}
	if c.ReservationStatus != "confirmed" {
		return model.Rental{}, stateErr("reservation_not_confirmed", "the reservation is not confirmed")
	}
	if !c.AllocationUuid.Valid || c.AllocationStatus.String != availmodel.StatusActive {
		return model.Rental{}, stateErr("allocation_not_active", "the reservation no longer holds the vehicle")
	}
	at, err := s.actualTime("at", in.At)
	ve := &ValidationError{}
	if err != nil {
		ve.add("at", "in_future", "the time cannot be in the future")
	}
	checkReading(ve, "", in.Km, in.Fuel, c.VehicleOdometerKm)
	if len(in.Note) > 2000 {
		ve.add("note", "too_long", "note is at most 2000 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.Rental{}, err
	}
	branch, point, err := s.place(ctx, orgID, "branch", in.BranchUUID, in.PointUUID, c.PlannedPickupBranchID)
	if err != nil {
		return model.Rental{}, err
	}
	if in.PointUUID == nil && in.BranchUUID == nil && c.PlannedPickupPointID.Valid {
		point = c.PlannedPickupPointID
	}
	contract, err := s.documents.ContractState(unscoped(ctx), orgID, c.ReservationID)
	if err != nil {
		return model.Rental{}, err
	}
	var waiver []byte
	if contract.Required && !contract.Approved {
		reason := strings.TrimSpace(in.WaiverReason)
		if reason == "" {
			return model.Rental{}, stateErr("contract_not_approved", "the rental contract is not approved (record the approval or waive it with a reason)")
		}
		if !has(ctx, rbac.PermTenantSignatureWaive) {
			return model.Rental{}, ErrForbidden
		}
		if _, err := requireReason("waiver_reason", reason); err != nil {
			return model.Rental{}, err
		}
		waiver = mustJSON(map[string]any{"reason": reason, "actor_id": actorID(ctx), "at": s.clock(), "document_uuid": contract.DocumentUUID})
	}
	var tasks []uuid.UUID
	var touched []uuid.UUID
	_, err = s.avail.StartRental(unscoped(ctx), orgID, c.AllocationUuid.Bytes, at,
		s.hookOf(func(hctx context.Context, q *db.Queries, tx pgx.Tx, r availmodel.HookResult) error {
			tasks = nil
			n, err := q.SetRentalHandover(hctx, db.SetRentalHandoverParams{
				OrganizationID: orgID, ID: c.ID, ActualPickupAt: ts(at), BranchID: int8Of(branch.ID), PointID: point,
				ActorID: int8Ptr(actorID(ctx)), ContractWaiver: waiver,
			})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrConcurrent
			}
			leg, err := q.InsertRentalVehicle(hctx, db.InsertRentalVehicleParams{
				OrganizationID: orgID, RentalID: c.ID, VehicleID: c.VehicleID, AllocationID: c.AllocationID, Reason: "handover",
				OutAt: ts(at), OutBranchID: int8Of(branch.ID), OutKm: int4Ptr(in.Km), OutFuel: int2Ptr(in.Fuel), Note: strings.TrimSpace(in.Note),
			})
			if err != nil {
				return err
			}
			if err := s.vehicleChange(ctx, q, orgID, c.VehicleUuid, vehicleusecase.OperationalChange{
				Status: vehiclemodel.StatusRented, OdometerKm: in.Km, FuelLevel: in.Fuel, Note: "rental " + c.Reference,
			}); err != nil {
				return err
			}
			if err := event(ctx, q, orgID, c.ID, model.EventHandedOver, c.Status, model.StatusActive, strings.TrimSpace(in.Note), map[string]any{
				"at": at, "branch": branch.Code, "km": in.Km, "fuel": in.Fuel, "plate": c.VehiclePlate,
				"early_minutes": int(math.Max(0, c.PlannedPickupAt.Time.Sub(at).Minutes())),
			}); err != nil {
				return err
			}
			if waiver != nil {
				if err := event(ctx, q, orgID, c.ID, model.EventContractWaived, "", "", strings.TrimSpace(in.WaiverReason), nil); err != nil {
					return err
				}
			}
			closed, err := q.FinishRentalTasks(hctx, db.FinishRentalTasksParams{
				OrganizationID: orgID, RentalID: int8Of(c.ID), Status: model.TaskDone, CompletedAt: ts(s.clock()),
				ActorID: int8Ptr(actorID(ctx)), CompletionNote: "handed over", TaskTypes: []string{model.TaskPreparation, model.TaskHandover},
			})
			if err != nil {
				return err
			}
			tasks = append(tasks, closed...)
			if tid, ok, err := openTask(hctx, q, orgID, model.TaskReturn, c.PlannedReturnBranchID, c.ID, c.VehicleID, c.PlannedReturnAt.Time, ""); err != nil {
				return err
			} else if ok {
				tasks = append(tasks, tid)
			}
			t, err := s.reservations.RecordPlanEffects(hctx, tx, orgID, r, "rental", c.Uuid)
			touched = t
			if err != nil {
				return err
			}
			if in.Finalize != nil {
				return in.Finalize(hctx, tx, leg.ID)
			}
			return nil
		}))
	if err != nil {
		return model.Rental{}, mapEngineErr(err)
	}
	s.prepareDocuments(ctx, orgID, c.ReservationID, c.Uuid, []string{DocHandoverForm}, "handover")
	s.publish(ctx, model.ResourceOperationTasks, "updated", tasks...)
	s.publish(ctx, "tenant.reservations", "updated", append(touched, c.ReservationUuid)...)
	return s.after(ctx, orgID, id)
}

// Return records the vehicle coming back: km / fuel in, the actual time and
// branch (kept next to the planned ones). The engine allocation ends at the
// actual return at the actual branch (its preparation applies before the
// next sale; the vehicle's current branch moves there); later commitments
// the change breaks get late_return tasks. A return after the planned
// return + tolerance proposes the late fee of the reservation's policy
// version (only when the policy defines an extra hour fee).
func (s *Service) Return(ctx context.Context, orgID int64, id uuid.UUID, in model.ReturnInput) (model.Rental, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Rental{}, err
	}
	if c.Status != model.StatusActive {
		return model.Rental{}, stateErr("invalid_transition", "only a rental under way can be returned")
	}
	if !c.AllocationUuid.Valid || c.AllocationStatus.String != availmodel.StatusActive {
		return model.Rental{}, stateErr("allocation_not_active", "the rental no longer holds the vehicle")
	}
	at, err := s.actualTime("at", in.At)
	ve := &ValidationError{}
	if err != nil {
		ve.add("at", "in_future", "the time cannot be in the future")
	} else if c.ActualPickupAt.Valid && !at.After(c.ActualPickupAt.Time) {
		ve.add("at", "before_pickup", "the return must be after the handover")
	}
	minKm := int32(0)
	if legs, err := s.q.ListRentalVehicles(ctx, db.ListRentalVehiclesParams{OrganizationID: orgID, RentalID: c.ID}); err == nil && len(legs) > 0 {
		if last := legs[len(legs)-1]; last.OutKm.Valid {
			minKm = last.OutKm.Int32
		}
	}
	checkReading(ve, "", in.Km, in.Fuel, minKm)
	if len(in.Note) > 2000 || len(in.DamageNote) > 2000 {
		ve.add("note", "too_long", "notes are at most 2000 characters")
	}
	if err := ve.orNil(); err != nil {
		return model.Rental{}, err
	}
	branch, point, err := s.place(ctx, orgID, "branch", in.BranchUUID, in.PointUUID, c.PlannedReturnBranchID)
	if err != nil {
		return model.Rental{}, err
	}
	policy := model.PolicyFromSnapshot(c.PolicySnapshot)
	late := int32(0)
	if at.After(c.PlannedReturnAt.Time) {
		late = int32(at.Sub(c.PlannedReturnAt.Time) / time.Minute)
	}
	var tasks, touched []uuid.UUID
	res, err := s.avail.SetRentalEnd(unscoped(ctx), orgID, c.AllocationUuid.Bytes, availmodel.RentalEnd{
		EndsAt: at, ReturnBranchID: branch.ID, ReturnPointID: point.Int64, SetPoint: true, Force: true,
		Hook: s.hookOf(func(hctx context.Context, q *db.Queries, tx pgx.Tx, r availmodel.HookResult) error {
			tasks = nil
			n, err := q.SetRentalReturn(hctx, db.SetRentalReturnParams{
				OrganizationID: orgID, ID: c.ID, ActualReturnAt: ts(at), BranchID: int8Of(branch.ID), PointID: point, LateMinutes: late,
				ActorID: int8Ptr(actorID(ctx)), ReturnNote: strings.TrimSpace(in.Note), DamageNote: strings.TrimSpace(in.DamageNote),
			})
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrConcurrent
			}
			if _, err := q.CloseRentalVehicleLeg(hctx, db.CloseRentalVehicleLegParams{
				OrganizationID: orgID, RentalID: c.ID, VehicleID: c.VehicleID, InAt: ts(at), InBranchID: int8Of(branch.ID),
				InKm: int4Ptr(in.Km), InFuel: int2Ptr(in.Fuel),
			}); err != nil {
				return err
			}
			if err := s.vehicleChange(ctx, q, orgID, c.VehicleUuid, vehicleusecase.OperationalChange{
				Status: vehiclemodel.StatusReturnCheck, CurrentBranchID: branch.ID, OdometerKm: in.Km, FuelLevel: in.Fuel,
				Note: "rental " + c.Reference + " returned",
			}); err != nil {
				return err
			}
			data := map[string]any{
				"at": at, "branch": branch.Code, "km": in.Km, "fuel": in.Fuel, "late_minutes": late,
				"planned_return_at": c.PlannedReturnAt.Time.UTC(), "elsewhere": branch.ID != c.PlannedReturnBranchID,
			}
			if err := event(ctx, q, orgID, c.ID, model.EventReturned, c.Status, model.StatusReturnCheck, strings.TrimSpace(in.Note), data); err != nil {
				return err
			}
			if early := c.PlannedReturnAt.Time.Sub(at); early > 0 {
				// Early return: no automatic refund or fee; the policy text
				// of the reservation's version decides, a person acts on it.
				if err := event(ctx, q, orgID, c.ID, model.EventEarlyReturn, "", "", "", map[string]any{
					"early_minutes": int(early / time.Minute), "policy": policy.EarlyReturn, "settings_version": policy.SettingsVersion,
				}); err != nil {
					return err
				}
			}
			if err := s.proposeLateFee(ctx, q, orgID, c, policy, late, at); err != nil {
				return err
			}
			closed, err := q.FinishRentalTasks(hctx, db.FinishRentalTasksParams{
				OrganizationID: orgID, RentalID: int8Of(c.ID), Status: model.TaskDone, CompletedAt: ts(s.clock()),
				ActorID: int8Ptr(actorID(ctx)), CompletionNote: "returned", TaskTypes: []string{model.TaskReturn},
			})
			if err != nil {
				return err
			}
			tasks = append(tasks, closed...)
			t, err := s.reservations.RecordPlanEffects(hctx, tx, orgID, r, "rental", c.Uuid)
			touched = t
			if err != nil {
				return err
			}
			if in.Finalize != nil {
				leg, err := q.GetRentalLegForInspection(hctx, db.GetRentalLegForInspectionParams{OrganizationID: orgID, RentalID: c.ID, VehicleID: c.VehicleID})
				if err != nil {
					return err
				}
				return in.Finalize(hctx, tx, leg)
			}
			return nil
		}),
	})
	if err != nil {
		return model.Rental{}, mapEngineErr(err)
	}
	s.lateReturnTasks(ctx, orgID, c, res.Affected, map[string]any{"cause": "return", "late_minutes": late, "clipped": res.Clipped})
	s.prepareDocuments(ctx, orgID, c.ReservationID, c.Uuid, []string{DocReturnForm}, "return")
	s.publish(ctx, model.ResourceOperationTasks, "updated", tasks...)
	s.publish(ctx, "tenant.reservations", "updated", append(touched, c.ReservationUuid)...)
	return s.after(ctx, orgID, id)
}

// proposeLateFee proposes the policy late fee: beyond the tolerance of the
// reservation's policy version, every started hour since the planned
// return costs the policy's extra hour fee. No fee is computed without a
// defined fee (a person may still propose a manual charge).
func (s *Service) proposeLateFee(ctx context.Context, q *db.Queries, orgID int64, c db.GetRentalCoreRow, p model.Policy, late int32, at time.Time) error {
	fee, ok := model.LateFee(p, late, c.PlannedReturnAt.Time, at)
	if !ok {
		return nil
	}
	if err := q.DeleteProposedPolicyLateCharge(ctx, db.DeleteProposedPolicyLateChargeParams{OrganizationID: orgID, RentalID: c.ID}); err != nil {
		return err
	}
	if n, err := q.CountLivePolicyLateCharges(ctx, db.CountLivePolicyLateChargesParams{OrganizationID: orgID, RentalID: c.ID}); err != nil || n > 0 {
		return err // an approved policy fee exists already
	}
	currency := strings.TrimSpace(c.Currency)
	if currency == "" {
		currency = "TRY"
	}
	chargeID, err := q.InsertRentalCharge(ctx, db.InsertRentalChargeParams{
		OrganizationID: orgID, RentalID: c.ID, Kind: model.ChargeLateReturn, Source: model.SourcePolicy, Currency: currency,
		Amount: fee.Amount.Numeric(), SettingsVersion: int32(p.SettingsVersion), Calculation: mustJSON(fee.Calculation),
		Reason: "late return", CreatedBy: int8Ptr(actorID(ctx)),
	})
	if err != nil {
		return err
	}
	return event(ctx, q, orgID, c.ID, model.EventChargeProposed, "", "", "", map[string]any{
		"charge_uuid": chargeID, "kind": model.ChargeLateReturn, "amount": fee.Amount.String(), "source": model.SourcePolicy,
	})
}

// lateReturnTasks opens late_return tasks on the reservations a late or
// elsewhere-returned vehicle runs into and records them on the rental.
func (s *Service) lateReturnTasks(ctx context.Context, orgID int64, c db.GetRentalCoreRow, affected []availmodel.Affected, data map[string]any) {
	if len(affected) == 0 {
		return
	}
	allocs := make([]uuid.UUID, 0, len(affected))
	for _, a := range affected {
		if a.Kind == availmodel.KindReservation || a.Kind == availmodel.KindHold {
			allocs = append(allocs, a.UUID)
		}
	}
	got := s.reservations.LateReturnTasks(ctx, orgID, c.Uuid, allocs, data)
	if len(got) == 0 {
		return
	}
	_ = s.tx(ctx, func(q *db.Queries) error {
		return event(ctx, q, orgID, c.ID, model.EventLateReturnTasks, "", "", "", map[string]any{"reservations": got})
	})
}

// CompleteReturn finishes the return check: the vehicle goes to cleaning
// (a cleaning task opens at the actual return branch, due after its
// preparation time) or straight back to available.
func (s *Service) CompleteReturn(ctx context.Context, orgID int64, id uuid.UUID, in model.CompleteReturnInput) (model.Rental, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Rental{}, err
	}
	if c.Status != model.StatusReturnCheck {
		return model.Rental{}, stateErr("invalid_transition", "only a returned rental can finish its check")
	}
	status := in.VehicleStatus
	if status == "" {
		status = vehiclemodel.StatusCleaning
	}
	if !model.Contains(model.ReturnVehicleStatuses, status) {
		return model.Rental{}, validation("vehicle_status", "invalid", "vehicle_status must be cleaning, available, maintenance or preparing")
	}
	// A disputed or unsigned return form waits for an authorized review
	// (TEC-713, RT-A19) before the return check can finish.
	if n, err := s.q.CountPendingInspectionReviews(ctx, db.CountPendingInspectionReviewsParams{OrganizationID: orgID, RentalID: c.ID}); err != nil {
		return model.Rental{}, err
	} else if n > 0 {
		return model.Rental{}, stateErr("review_pending", "the return form waits for an authorized review")
	}
	if len(in.Note) > 2000 {
		return model.Rental{}, validation("note", "too_long", "note is at most 2000 characters")
	}
	var tasks []uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.SetRentalChecked(ctx, db.SetRentalCheckedParams{OrganizationID: orgID, ID: c.ID, ActorID: int8Ptr(actorID(ctx)), CheckedAt: ts(s.clock())})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		if err := s.vehicleChange(ctx, q, orgID, c.VehicleUuid, vehicleusecase.OperationalChange{Status: status, Note: "rental " + c.Reference + " checked"}); err != nil {
			return err
		}
		branchID := c.ActualReturnBranchID.Int64
		prep := 60
		if b, err := q.GetRentalBranchByID(ctx, db.GetRentalBranchByIDParams{OrganizationID: orgID, ID: branchID}); err == nil && b.PrepMinutes > 0 {
			prep = int(b.PrepMinutes)
		}
		// The vehicle's next step is an operation task of the return branch;
		// finishing it makes the vehicle available.
		taskType := map[string]string{
			vehiclemodel.StatusCleaning: model.TaskCleaning, vehiclemodel.StatusMaintenance: model.TaskMaintenance,
			vehiclemodel.StatusPreparing: model.TaskPreparation,
		}[status]
		if taskType != "" {
			tid, ok, err := openTask(ctx, q, orgID, taskType, branchID, c.ID, c.VehicleID, s.clock().Add(time.Duration(prep)*time.Minute), "")
			if err != nil {
				return err
			}
			if ok {
				tasks = append(tasks, tid)
			}
		}
		if in.DepositTask {
			tid, ok, err := openTask(ctx, q, orgID, model.TaskDeposit, branchID, c.ID, c.VehicleID, s.clock().Add(24*time.Hour), "")
			if err != nil {
				return err
			}
			if ok {
				tasks = append(tasks, tid)
			}
		}
		return event(ctx, q, orgID, c.ID, model.EventChecked, c.Status, model.StatusReturned, strings.TrimSpace(in.Note), map[string]any{"vehicle_status": status})
	})
	if err != nil {
		return model.Rental{}, err
	}
	s.publish(ctx, model.ResourceOperationTasks, "created", tasks...)
	return s.after(ctx, orgID, id)
}

// Cancel withdraws a rental still in preparation (the reservation stays as
// it is; a later plate fix opens a new rental). Its live tasks are
// cancelled and its documents voided.
func (s *Service) Cancel(ctx context.Context, orgID int64, id uuid.UUID, reason string) (model.Rental, error) {
	if _, err := s.core(ctx, orgID, id); err != nil {
		return model.Rental{}, err
	}
	r, err := requireReason("reason", reason)
	if err != nil {
		return model.Rental{}, err
	}
	if _, err := s.cancel(ctx, orgID, id, r); err != nil {
		return model.Rental{}, err
	}
	return s.after(ctx, orgID, id)
}

func (s *Service) cancel(ctx context.Context, orgID int64, id uuid.UUID, reason string) (bool, error) {
	c, err := s.q.GetRentalCore(ctx, db.GetRentalCoreParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return false, err
	}
	if c.Status != model.StatusPreparation {
		return false, stateErr("invalid_transition", "only a rental in preparation can be cancelled")
	}
	var tasks []uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		n, err := q.CancelRental(ctx, db.CancelRentalParams{OrganizationID: orgID, ID: c.ID, CancelledAt: ts(s.clock()), CancelReason: reason})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConcurrent
		}
		tasks, err = q.FinishRentalTasks(ctx, db.FinishRentalTasksParams{
			OrganizationID: orgID, RentalID: int8Of(c.ID), Status: model.TaskCancelled, CompletedAt: ts(s.clock()),
			ActorID: int8Ptr(actorID(ctx)), CompletionNote: "rental cancelled",
		})
		if err != nil {
			return err
		}
		return event(ctx, q, orgID, c.ID, model.EventCancelled, c.Status, model.StatusCancelled, reason, nil)
	})
	if err != nil {
		return false, err
	}
	if err := s.documents.Void(unscoped(ctx), orgID, c.ID); err != nil {
		_ = err // best effort; the documents stay readable
	}
	s.publish(ctx, model.ResourceRentals, "updated", id)
	s.publish(ctx, model.ResourceOperationTasks, "updated", tasks...)
	return true, nil
}

// Extend moves the planned return of a rental (in preparation or under
// way) later through the reservation's extension rule (TEC-698: the added
// days are priced with the band of the new length; a collision answers
// EXTENSION_CONFLICT with alternatives). The reservation listener re-renders
// the documents (an approved contract gets a new version: re-approval).
func (s *Service) Extend(ctx context.Context, orgID int64, id uuid.UUID, in model.ExtendInput) (model.Rental, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Rental{}, err
	}
	if c.Status != model.StatusPreparation && c.Status != model.StatusActive {
		return model.Rental{}, stateErr("invalid_transition", "only a rental in preparation or under way can be extended")
	}
	if in.EndsAt == nil {
		return model.Rental{}, validation("ends_at", "required", "ends_at is required")
	}
	if !in.EndsAt.After(c.PlannedReturnAt.Time) {
		return model.Rental{}, validation("ends_at", "not_later", "an extension must end after the planned return")
	}
	if len(in.Reason) > 500 {
		return model.Rental{}, validation("reason", "too_long", "reason is at most 500 characters")
	}
	from := c.PlannedReturnAt.Time.UTC()
	out, err := s.reservations.Extend(ctx, orgID, c.ReservationUuid, in.EndsAt.UTC(), strings.TrimSpace(in.Reason))
	if err != nil {
		return model.Rental{}, err
	}
	err = s.tx(ctx, func(q *db.Queries) error {
		if c.OverdueAt.Valid && in.EndsAt.After(s.clock()) {
			if _, err := q.ClearRentalOverdue(ctx, db.ClearRentalOverdueParams{OrganizationID: orgID, ID: c.ID}); err != nil {
				return err
			}
		}
		return event(ctx, q, orgID, c.ID, model.EventExtended, "", "", strings.TrimSpace(in.Reason), map[string]any{
			"from": from, "to": in.EndsAt.UTC(), "difference": out.Difference, "total": out.Total,
		})
	})
	if err != nil {
		return model.Rental{}, err
	}
	return s.after(ctx, orgID, id)
}

// Swap moves a rental under way to another vehicle: the current vehicle is
// returned (km / fuel in, cleaning task) and the new one handed over (km /
// fuel out) as separate legs of the same rental; the engine moves the rest
// of the range atomically (the new vehicle must keep the sold guarantee).
// The documents are re-rendered (a vehicle change needs a new approval).
func (s *Service) Swap(ctx context.Context, orgID int64, id uuid.UUID, in model.SwapInput) (model.Rental, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Rental{}, err
	}
	if c.Status != model.StatusActive {
		return model.Rental{}, stateErr("invalid_transition", "only a rental under way can change its vehicle")
	}
	if !c.AllocationUuid.Valid || c.AllocationStatus.String != availmodel.StatusActive {
		return model.Rental{}, stateErr("allocation_not_active", "the rental no longer holds the vehicle")
	}
	ve := &ValidationError{}
	if in.VehicleUUID == nil {
		ve.add("vehicle_uuid", "required", "vehicle_uuid is required")
	}
	at, err := s.actualTime("at", in.At)
	if err != nil {
		ve.add("at", "in_future", "the time cannot be in the future")
	}
	minKm := int32(0)
	if legs, err := s.q.ListRentalVehicles(ctx, db.ListRentalVehiclesParams{OrganizationID: orgID, RentalID: c.ID}); err == nil && len(legs) > 0 {
		if last := legs[len(legs)-1]; last.OutKm.Valid {
			minKm = last.OutKm.Int32
		}
	}
	checkReading(ve, "in_", in.InKm, in.InFuel, minKm)
	checkReading(ve, "out_", in.OutKm, in.OutFuel, 0)
	reason, rerr := requireReason("reason", in.Reason)
	if rerr != nil {
		ve.add("reason", "required", "a reason is required (3-500 characters)")
	}
	if err := ve.orNil(); err != nil {
		return model.Rental{}, err
	}
	target, err := s.q.GetRentalVehicleByUUID(ctx, db.GetRentalVehicleByUUIDParams{OrganizationID: orgID, Uuid: *in.VehicleUUID})
	if notFound(err) {
		return model.Rental{}, validation("vehicle_uuid", "not_found", "vehicle was not found")
	}
	if err != nil {
		return model.Rental{}, err
	}
	branch, _, err := s.place(ctx, orgID, "branch", in.BranchUUID, nil, c.PlannedPickupBranchID)
	if err != nil {
		return model.Rental{}, err
	}
	var tasks, touched []uuid.UUID
	oldAlloc := c.AllocationID
	_, err = s.avail.SwapRentalVehicle(unscoped(ctx), orgID, c.AllocationUuid.Bytes, availmodel.RentalSwap{
		VehicleUUID: target.Uuid, At: at, BranchID: branch.ID, ActorID: actorID(ctx), Note: "rental " + c.Reference + " swap",
		Hook: s.hookOf(func(hctx context.Context, q *db.Queries, tx pgx.Tx, r availmodel.HookResult) error {
			tasks = nil
			newAlloc := pgtype.Int8{Int64: r.AllocationID, Valid: true}
			if n, err := q.SetRentalVehicle(hctx, db.SetRentalVehicleParams{OrganizationID: orgID, ID: c.ID, VehicleID: target.ID, AllocationID: newAlloc}); err != nil {
				return err
			} else if n != 1 {
				return ErrConcurrent
			}
			if n, err := q.SetReservationAllocationForRental(hctx, db.SetReservationAllocationForRentalParams{
				OrganizationID: orgID, ID: c.ReservationID, NewAllocationID: newAlloc, OldAllocationID: oldAlloc,
			}); err != nil {
				return err
			} else if n != 1 {
				return ErrConcurrent
			}
			if _, err := q.CloseRentalVehicleLeg(hctx, db.CloseRentalVehicleLegParams{
				OrganizationID: orgID, RentalID: c.ID, VehicleID: c.VehicleID, InAt: ts(at), InBranchID: int8Of(branch.ID),
				InKm: int4Ptr(in.InKm), InFuel: int2Ptr(in.InFuel),
			}); err != nil {
				return err
			}
			if _, err := q.InsertRentalVehicle(hctx, db.InsertRentalVehicleParams{
				OrganizationID: orgID, RentalID: c.ID, VehicleID: target.ID, AllocationID: newAlloc, Reason: "swap",
				OutAt: ts(at), OutBranchID: int8Of(branch.ID), OutKm: int4Ptr(in.OutKm), OutFuel: int2Ptr(in.OutFuel), Note: reason,
			}); err != nil {
				return err
			}
			if err := s.vehicleChange(ctx, q, orgID, c.VehicleUuid, vehicleusecase.OperationalChange{
				Status: vehiclemodel.StatusReturnCheck, CurrentBranchID: branch.ID, OdometerKm: in.InKm, FuelLevel: in.InFuel,
				Note: "rental " + c.Reference + " swap (returned)",
			}); err != nil {
				return err
			}
			if err := s.vehicleChange(ctx, q, orgID, target.Uuid, vehicleusecase.OperationalChange{
				Status: vehiclemodel.StatusRented, OdometerKm: in.OutKm, FuelLevel: in.OutFuel, Note: "rental " + c.Reference + " swap (handed over)",
			}); err != nil {
				return err
			}
			// The reservation's plate history and timeline follow.
			if err := q.EndVehicleAssignments(hctx, db.EndVehicleAssignmentsParams{OrganizationID: orgID, ReservationID: c.ReservationID}); err != nil {
				return err
			}
			if err := q.InsertVehicleAssignment(hctx, db.InsertVehicleAssignmentParams{
				OrganizationID: orgID, ReservationID: c.ReservationID, VehicleID: int8Of(target.ID), RentalProductID: r.ProductID,
				ProductVersion: r.ProductVersion, Kind: "fixed", Source: "staff", Note: "rental swap: " + reason, ActorID: int8Ptr(actorID(ctx)),
			}); err != nil {
				return err
			}
			if err := q.InsertReservationEvent(hctx, db.InsertReservationEventParams{
				OrganizationID: orgID, ReservationID: c.ReservationID, Action: "plate_changed", Reason: reason,
				Data:    mustJSON(map[string]any{"from_plate": c.VehiclePlate, "to_plate": target.Plate, "fixed": true, "source": "rental_swap"}),
				ActorID: int8Ptr(actorID(ctx)),
			}); err != nil {
				return err
			}
			if err := event(ctx, q, orgID, c.ID, model.EventSwapped, "", "", reason, map[string]any{
				"at": at, "branch": branch.Code, "from_plate": c.VehiclePlate, "to_plate": target.Plate,
				"in_km": in.InKm, "in_fuel": in.InFuel, "out_km": in.OutKm, "out_fuel": in.OutFuel,
			}); err != nil {
				return err
			}
			prep := int(branch.PrepMinutes)
			if prep <= 0 {
				prep = 60
			}
			if tid, ok, err := openTask(hctx, q, orgID, model.TaskCleaning, branch.ID, c.ID, c.VehicleID, at.Add(time.Duration(prep)*time.Minute), "swap"); err != nil {
				return err
			} else if ok {
				tasks = append(tasks, tid)
			}
			// The return task now names the new vehicle.
			closed, err := q.FinishRentalTasks(hctx, db.FinishRentalTasksParams{
				OrganizationID: orgID, RentalID: int8Of(c.ID), Status: model.TaskCancelled, CompletedAt: ts(s.clock()),
				ActorID: int8Ptr(actorID(ctx)), CompletionNote: "vehicle swapped", TaskTypes: []string{model.TaskReturn},
			})
			if err != nil {
				return err
			}
			tasks = append(tasks, closed...)
			if tid, ok, err := openTask(hctx, q, orgID, model.TaskReturn, c.PlannedReturnBranchID, c.ID, target.ID, c.PlannedReturnAt.Time, ""); err != nil {
				return err
			} else if ok {
				tasks = append(tasks, tid)
			}
			t, err := s.reservations.RecordPlanEffects(hctx, tx, orgID, r, "rental", c.Uuid)
			touched = t
			return err
		}),
	})
	if err != nil {
		return model.Rental{}, mapEngineErr(err)
	}
	s.prepareDocuments(ctx, orgID, c.ReservationID, c.Uuid, nil, "vehicle_swapped")
	s.publish(ctx, model.ResourceOperationTasks, "updated", tasks...)
	s.publish(ctx, "tenant.reservations", "updated", append(touched, c.ReservationUuid)...)
	return s.after(ctx, orgID, id)
}

// after publishes the change and returns the fresh rental.
func (s *Service) after(ctx context.Context, orgID int64, id uuid.UUID) (model.Rental, error) {
	s.publish(ctx, model.ResourceRentals, "updated", id)
	r, err := s.Get(ctx, orgID, id)
	if errors.Is(err, ErrNotFound) {
		// The action moved the rental out of the caller's branch scope
		// (returned elsewhere): read it without the scope.
		return s.Get(unscoped(ctx), orgID, id)
	}
	return r, err
}
