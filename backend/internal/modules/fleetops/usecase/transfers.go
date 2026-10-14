package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TransferSortColumns are the sortable transfer list fields.
var TransferSortColumns = map[string]string{
	"planned_departure": "planned_departure", "planned_arrival": "planned_arrival", "status": "status",
	"cost": "cost", "plate": "plate", "created_at": "created_at", "updated_at": "updated_at",
}

func toTransfer(row repository.TransferRow) model.Transfer {
	return model.Transfer{
		UUID:    row.Uuid,
		Vehicle: model.VehicleRef{UUID: row.VehicleUuid, Plate: row.VehiclePlate, Label: row.VehicleLabel},
		FromBranch: model.BranchRef{
			UUID: row.FromBranchUuid, Code: row.FromBranchCode, Name: row.FromBranchName,
		},
		ToBranch:         model.BranchRef{UUID: row.ToBranchUuid, Code: row.ToBranchCode, Name: row.ToBranchName},
		Responsible:      actorFrom(row.ResponsibleUuid, row.ResponsibleName, row.ResponsibleSurname, row.ResponsibleEmail),
		PlannedDeparture: row.PlannedDeparture.Time.UTC(),
		PlannedArrival:   row.PlannedArrival.Time.UTC(),
		ActualDeparture:  timePtr(row.ActualDeparture),
		ActualArrival:    timePtr(row.ActualArrival),
		Cost:             moneyPtr(row.Cost),
		Status:           row.Status,
		Note:             row.Note,
		CancelledAt:      timePtr(row.CancelledAt),
		CreatedAt:        row.CreatedAt.Time.UTC(),
		UpdatedAt:        row.UpdatedAt.Time.UTC(),
	}
}

// ListTransfers returns one page of transfers in the caller's branch scope.
func (s *Service) ListTransfers(ctx context.Context, orgID int64, f model.TransferFilter) ([]model.Transfer, int64, error) {
	if _, ok := TransferSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "planned_departure", true
	}
	f.BranchIDs = orgctx.BranchScope(ctx)
	rows, total, err := s.repo.ListTransfers(ctx, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Transfer, 0, len(rows))
	for _, row := range rows {
		out = append(out, toTransfer(row))
	}
	return out, total, nil
}

// TransferExportRows returns the transfers matching a filter (export),
// planned departure order, capped at MaxListTargets.
func (s *Service) TransferExportRows(ctx context.Context, orgID int64, f model.TransferFilter) ([]model.Transfer, error) {
	f.Limit, f.Offset, f.SortField, f.SortDesc = MaxListTargets, 0, "planned_departure", false
	out, _, err := s.ListTransfers(ctx, orgID, f)
	return out, err
}

// TransferFacets returns the dynamic filter options in the caller's scope.
func (s *Service) TransferFacets(ctx context.Context, orgID int64) (model.TransferFacets, error) {
	rows, err := s.repo.TransferFacets(ctx, orgID, orgctx.BranchScope(ctx))
	if err != nil {
		return model.TransferFacets{}, err
	}
	out := model.TransferFacets{Status: []model.Facet{}, FromBranch: []model.Facet{}, ToBranch: []model.Facet{}}
	for _, row := range rows {
		f := model.Facet{Value: row.Value, Label: row.Label, Count: row.Count}
		switch row.Facet {
		case "status":
			out.Status = append(out.Status, f)
		case "from_branch":
			out.FromBranch = append(out.FromBranch, f)
		case "to_branch":
			out.ToBranch = append(out.ToBranch, f)
		}
	}
	return out, nil
}

// GetTransfer returns one transfer of the organization.
func (s *Service) GetTransfer(ctx context.Context, orgID int64, id uuid.UUID) (model.Transfer, error) {
	row, err := s.getTransfer(ctx, orgID, id)
	if err != nil {
		return model.Transfer{}, err
	}
	return toTransfer(row), nil
}

func (s *Service) getTransfer(ctx context.Context, orgID int64, id uuid.UUID) (repository.TransferRow, error) {
	row, err := s.repo.GetTransfer(ctx, orgID, id)
	if err != nil {
		return repository.TransferRow{}, mapNotFound(err, ErrTransferNotFound)
	}
	if !eitherInScope(ctx, row.FromBranchID, row.ToBranchID) {
		return repository.TransferRow{}, ErrTransferNotFound
	}
	return row, nil
}

// CreateTransfer plans a transfer. The from branch defaults to where the
// vehicle is at the planned departure (LocationAt) and must be that
// branch; the planned window may not overlap a block (409
// block_overlap) or another open transfer of the vehicle (409
// transfer_overlap).
func (s *Service) CreateTransfer(ctx context.Context, orgID int64, in model.TransferInput) (model.Transfer, error) {
	ve := &ValidationError{}
	if in.VehicleUUID == nil {
		ve.add("vehicle_uuid", "required", "vehicle_uuid is required")
		s.checkRequiredPlan(ve, in)
		return model.Transfer{}, ve
	}
	v, err := s.getVehicle(ctx, orgID, *in.VehicleUUID)
	if err != nil {
		if errors.Is(err, ErrVehicleNotFound) {
			return model.Transfer{}, validation("vehicle_uuid", "not_found", "vehicle was not found")
		}
		return model.Transfer{}, err
	}
	s.checkRequiredPlan(ve, in)
	fields := repository.TransferFields{VehicleID: v.ID}
	s.applyTransferInput(ctx, orgID, ve, &fields, in)
	if err := ve.orNil(); err != nil {
		return model.Transfer{}, err
	}
	// Collisions first (the more useful answer), then where the vehicle is.
	if err := s.checkTransferWindow(ctx, orgID, v.ID, fields, 0); err != nil {
		return model.Transfer{}, err
	}
	if err := s.checkOrigin(ctx, orgID, v, &fields, in.FromBranchUUID == nil, uuid.Nil); err != nil {
		return model.Transfer{}, err
	}
	id, err := s.repo.CreateTransfer(ctx, orgID, fields, actorArg(ctx))
	if err != nil {
		return model.Transfer{}, s.transferWriteErr(ctx, orgID, v.ID, fields, 0, err)
	}
	return s.GetTransfer(ctx, orgID, id)
}

// PatchTransfer re-plans a planned transfer (same rules as create); once
// started only the cost and the note can change.
func (s *Service) PatchTransfer(ctx context.Context, orgID int64, id uuid.UUID, in model.TransferInput) (model.Transfer, error) {
	row, err := s.getTransfer(ctx, orgID, id)
	if err != nil {
		return model.Transfer{}, err
	}
	if in.VehicleUUID != nil && *in.VehicleUUID != row.VehicleUuid {
		return model.Transfer{}, validation("vehicle_uuid", "immutable", "the vehicle of a transfer cannot change")
	}
	ve := &ValidationError{}
	fields := repository.TransferFields{
		VehicleID: row.VehicleID, FromBranchID: row.FromBranchID, ToBranchID: row.ToBranchID,
		PlannedDeparture: row.PlannedDeparture.Time.UTC(), PlannedArrival: row.PlannedArrival.Time.UTC(),
		Cost: row.Cost, Note: row.Note,
	}
	if row.ResponsibleUuid.Valid {
		m, err := s.repo.GetMember(ctx, orgID, row.ResponsibleUuid.Bytes)
		if err == nil {
			fields.ResponsibleUserID = pgtype.Int8{Int64: m.UserID, Valid: true}
		}
	}
	switch row.Status {
	case model.TransferPlanned:
	case model.TransferInTransit, model.TransferCompleted:
		if in.FromBranchUUID != nil || in.ToBranchUUID != nil || in.PlannedDeparture != nil ||
			in.PlannedArrival != nil || in.ResponsibleUserUUID.Set {
			return model.Transfer{}, conflict(ConflictState, "only the cost and the note of a started transfer can change")
		}
		if in.Cost.Set {
			fields.Cost = parseMoney(ve, "cost", in.Cost.Value)
		}
		if in.Note != nil {
			fields.Note = checkNote(ve, "note", *in.Note)
		}
		if err := ve.orNil(); err != nil {
			return model.Transfer{}, err
		}
		if err := s.repo.SetTransferCost(ctx, orgID, id, fields.Cost, fields.Note); err != nil {
			return model.Transfer{}, mapNotFound(err, ErrTransferNotFound)
		}
		return s.GetTransfer(ctx, orgID, id)
	default:
		return model.Transfer{}, conflict(ConflictState, "a cancelled transfer cannot change")
	}
	s.applyTransferInput(ctx, orgID, ve, &fields, in)
	if err := ve.orNil(); err != nil {
		return model.Transfer{}, err
	}
	v, err := s.repo.GetVehicle(ctx, orgID, row.VehicleUuid)
	if err != nil {
		return model.Transfer{}, mapNotFound(err, ErrVehicleNotFound)
	}
	if err := s.checkTransferWindow(ctx, orgID, v.ID, fields, row.ID); err != nil {
		return model.Transfer{}, err
	}
	if err := s.checkOrigin(ctx, orgID, v, &fields, false, row.Uuid); err != nil {
		return model.Transfer{}, err
	}
	if err := s.repo.UpdateTransferPlan(ctx, orgID, id, fields); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Transfer{}, conflict(ConflictState, "the transfer is no longer planned")
		}
		return model.Transfer{}, s.transferWriteErr(ctx, orgID, v.ID, fields, row.ID, err)
	}
	return s.GetTransfer(ctx, orgID, id)
}

// StartTransfer records the departure (at defaults to now): the transfer
// is in transit and the vehicle status becomes in_transfer. The vehicle
// must be at the from branch and not blocked until the planned arrival.
func (s *Service) StartTransfer(ctx context.Context, orgID int64, id uuid.UUID, in model.TransferActionInput) (model.Transfer, error) {
	row, err := s.getTransfer(ctx, orgID, id)
	if err != nil {
		return model.Transfer{}, err
	}
	if row.Status != model.TransferPlanned {
		return model.Transfer{}, conflict(ConflictState, "only a planned transfer can start")
	}
	now := s.clock()
	at := now
	if in.At != nil {
		at = in.At.UTC().Truncate(time.Microsecond)
	}
	if at.After(now.Add(time.Minute)) {
		return model.Transfer{}, validation("at", "in_future", "the departure cannot be in the future")
	}
	v, err := s.repo.GetVehicle(ctx, orgID, row.VehicleUuid)
	if err != nil {
		return model.Transfer{}, mapNotFound(err, ErrVehicleNotFound)
	}
	if v.CurrentBranchID != row.FromBranchID {
		return model.Transfer{}, conflict(ConflictNotAtOrigin, "the vehicle is not at the departure branch")
	}
	if v.Status == "rented" || v.Status == "in_transfer" {
		return model.Transfer{}, conflict(ConflictVehicleBusy, "the vehicle is "+v.Status)
	}
	end := row.PlannedArrival.Time.UTC()
	if !end.After(at) {
		end = at.Add(time.Minute)
	}
	if err := s.checkBlocks(ctx, orgID, v.ID, at, &end); err != nil {
		return model.Transfer{}, err
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if err := tx.StartTransfer(ctx, orgID, id, at); err != nil {
			return err
		}
		_, err := s.vehicleTx(tx).ApplyOperationalChange(unscoped(ctx), orgID, v.Uuid, vehicleusecase.OperationalChange{
			Status: "in_transfer", Note: transferNote(row, in.Note),
		})
		return err
	})
	if err != nil {
		return model.Transfer{}, stateErr(err, "only a planned transfer can start")
	}
	return s.GetTransfer(ctx, orgID, id)
}

// CompleteTransfer records the arrival (at defaults to now): the vehicle
// is at the to branch (current_branch_id) with status arrival_status
// (preparing by default: an arriving vehicle is checked before it is
// sold again; available skips that step).
func (s *Service) CompleteTransfer(ctx context.Context, orgID int64, id uuid.UUID, in model.TransferActionInput) (model.Transfer, error) {
	row, err := s.getTransfer(ctx, orgID, id)
	if err != nil {
		return model.Transfer{}, err
	}
	if row.Status != model.TransferInTransit {
		return model.Transfer{}, conflict(ConflictState, "only a transfer in transit can complete")
	}
	now := s.clock()
	at := now
	if in.At != nil {
		at = in.At.UTC().Truncate(time.Microsecond)
	}
	ve := &ValidationError{}
	if at.After(now.Add(time.Minute)) {
		ve.add("at", "in_future", "the arrival cannot be in the future")
	}
	if row.ActualDeparture.Valid && at.Before(row.ActualDeparture.Time) {
		ve.add("at", "before_departure", "the arrival cannot be before the departure")
	}
	status := strings.TrimSpace(in.ArrivalStatus)
	if status == "" {
		status = "preparing"
	}
	if !model.Contains(model.ArrivalStatuses, status) {
		ve.add("arrival_status", "invalid", "arrival_status must be one of "+strings.Join(model.ArrivalStatuses, ", "))
	}
	if err := ve.orNil(); err != nil {
		return model.Transfer{}, err
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if err := tx.CompleteTransfer(ctx, orgID, id, at); err != nil {
			return err
		}
		_, err := s.vehicleTx(tx).ApplyOperationalChange(unscoped(ctx), orgID, row.VehicleUuid, vehicleusecase.OperationalChange{
			Status: status, CurrentBranchID: row.ToBranchID, Note: transferNote(row, in.Note),
		})
		return err
	})
	if err != nil {
		return model.Transfer{}, stateErr(err, "only a transfer in transit can complete")
	}
	return s.GetTransfer(ctx, orgID, id)
}

// CancelTransfer cancels a planned or in-transit transfer. A vehicle that
// was on the road stays at the from branch and becomes available again.
func (s *Service) CancelTransfer(ctx context.Context, orgID int64, id uuid.UUID, in model.TransferActionInput) (model.Transfer, error) {
	row, err := s.getTransfer(ctx, orgID, id)
	if err != nil {
		return model.Transfer{}, err
	}
	if row.Status != model.TransferPlanned && row.Status != model.TransferInTransit {
		return model.Transfer{}, conflict(ConflictState, "only a planned or in-transit transfer can be cancelled")
	}
	ve := &ValidationError{}
	note := checkNote(ve, "note", in.Note)
	if err := ve.orNil(); err != nil {
		return model.Transfer{}, err
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if err := tx.CancelTransfer(ctx, orgID, id, note); err != nil {
			return err
		}
		if row.Status != model.TransferInTransit {
			return nil
		}
		v, err := tx.GetVehicle(ctx, orgID, row.VehicleUuid)
		if err != nil || v.Status != "in_transfer" {
			return nil
		}
		_, err = s.vehicleTx(tx).ApplyOperationalChange(unscoped(ctx), orgID, row.VehicleUuid, vehicleusecase.OperationalChange{
			Status: "available", Note: transferNote(row, "cancelled"),
		})
		return err
	})
	if err != nil {
		return model.Transfer{}, stateErr(err, "only a planned or in-transit transfer can be cancelled")
	}
	return s.GetTransfer(ctx, orgID, id)
}

// TransferActivity lists the audit events of a transfer.
func (s *Service) TransferActivity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getTransfer(ctx, orgID, id); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, model.ResourceTransfers, id, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return activityEntries(rows), total, nil
}

// Assignees lists the members who can be responsible for a transfer.
func (s *Service) Assignees(ctx context.Context, orgID int64, q string) ([]model.Assignee, error) {
	rows, err := s.repo.ListMembers(ctx, orgID, strings.TrimSpace(q), 100)
	if err != nil {
		return nil, err
	}
	out := make([]model.Assignee, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.Assignee{UUID: row.UserUuid, Name: row.Name, Surname: row.Surname, Email: row.Email})
	}
	return out, nil
}

// LocationAt is where a vehicle is at t (see vehicleusecase.ResolveLocation).
func (s *Service) LocationAt(ctx context.Context, orgID int64, v repository.VehicleRow, t time.Time) (vehicleusecase.Location, error) {
	rows, err := s.repo.Movements(ctx, orgID, v.ID)
	if err != nil {
		return vehicleusecase.Location{}, err
	}
	return vehicleusecase.ResolveLocation(v.CurrentBranchID, s.clock(), vehicleusecase.MovementsFromTransfers(rows), t), nil
}

func (s *Service) checkRequiredPlan(ve *ValidationError, in model.TransferInput) {
	if in.ToBranchUUID == nil {
		ve.add("to_branch_uuid", "required", "to_branch_uuid is required")
	}
	if in.PlannedDeparture == nil {
		ve.add("planned_departure", "required", "planned_departure is required")
	}
	if in.PlannedArrival == nil {
		ve.add("planned_arrival", "required", "planned_arrival is required")
	}
}

func (s *Service) applyTransferInput(ctx context.Context, orgID int64, ve *ValidationError, f *repository.TransferFields, in model.TransferInput) {
	branch := func(field string, id *uuid.UUID, dst *int64) {
		if id == nil {
			return
		}
		b, err := s.repo.GetBranch(ctx, orgID, *id)
		if err != nil {
			ve.add(field, "not_found", "branch was not found")
			return
		}
		*dst = b.ID
	}
	branch("from_branch_uuid", in.FromBranchUUID, &f.FromBranchID)
	branch("to_branch_uuid", in.ToBranchUUID, &f.ToBranchID)
	if in.ResponsibleUserUUID.Set {
		f.ResponsibleUserID = pgtype.Int8{}
		if in.ResponsibleUserUUID.Value != nil {
			m, err := s.repo.GetMember(ctx, orgID, *in.ResponsibleUserUUID.Value)
			if err != nil {
				ve.add("responsible_user_uuid", "not_found", "the responsible user is not a member of the organization")
			} else {
				f.ResponsibleUserID = pgtype.Int8{Int64: m.UserID, Valid: true}
			}
		}
	}
	if in.PlannedDeparture != nil {
		f.PlannedDeparture = in.PlannedDeparture.UTC().Truncate(time.Microsecond)
	}
	if in.PlannedArrival != nil {
		f.PlannedArrival = in.PlannedArrival.UTC().Truncate(time.Microsecond)
	}
	if !ve.has("planned_departure") && !ve.has("planned_arrival") && !f.PlannedArrival.After(f.PlannedDeparture) {
		ve.add("planned_arrival", "before_departure", "planned_arrival must be after planned_departure")
	}
	if in.Cost.Set {
		f.Cost = parseMoney(ve, "cost", in.Cost.Value)
	}
	if in.Note != nil {
		f.Note = checkNote(ve, "note", *in.Note)
	}
}

// checkOrigin enforces "a vehicle is not available at another branch by
// itself": the transfer leaves from where the vehicle is at the planned
// departure (defaulted when the client sent no from branch). It also
// checks the route and the caller's branch scope.
func (s *Service) checkOrigin(ctx context.Context, orgID int64, v repository.VehicleRow, f *repository.TransferFields, defaultFrom bool, exclude uuid.UUID) error {
	rows, err := s.repo.Movements(ctx, orgID, v.ID)
	if err != nil {
		return err
	}
	moves := make([]vehicleusecase.Movement, 0, len(rows))
	for i, m := range vehicleusecase.MovementsFromTransfers(rows) {
		if rows[i].Uuid == exclude {
			continue
		}
		moves = append(moves, m)
	}
	loc := vehicleusecase.ResolveLocation(v.CurrentBranchID, s.clock(), moves, f.PlannedDeparture)
	if loc.InTransit {
		return validation("planned_departure", "in_transit", "the vehicle is in transit at the planned departure")
	}
	if defaultFrom {
		f.FromBranchID = loc.BranchID
	}
	if f.FromBranchID != loc.BranchID {
		return validation("from_branch_uuid", "not_at_branch", "the vehicle is not at this branch at the planned departure")
	}
	if f.FromBranchID == f.ToBranchID {
		return validation("to_branch_uuid", "same_branch", "to_branch_uuid must differ from the departure branch")
	}
	if !eitherInScope(ctx, f.FromBranchID, f.ToBranchID) {
		return validation("to_branch_uuid", "not_found", "branch was not found")
	}
	return nil
}

// checkTransferWindow rejects a planned window that overlaps a block (via
// vehicle_busy_intervals) or another open transfer of the vehicle.
func (s *Service) checkTransferWindow(ctx context.Context, orgID, vehicleID int64, f repository.TransferFields, excludeID int64) error {
	end := f.PlannedArrival
	if err := s.checkBlocks(ctx, orgID, vehicleID, f.PlannedDeparture, &end); err != nil {
		return err
	}
	rows, err := s.repo.OverlappingTransfers(ctx, orgID, vehicleID, f.PlannedDeparture, &end, excludeID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	items := make([]Conflict, 0, len(rows))
	for _, row := range rows {
		items = append(items, Conflict{
			Kind: "transfer", UUID: row.Uuid, StartsAt: row.StartsAt.Time.UTC(), EndsAt: timePtr(row.EndsAt),
		})
	}
	return conflict(ConflictTransferOverlap, "the vehicle already has a transfer in this period", items...)
}

// checkBlocks answers 409 block_overlap when a block of the vehicle
// overlaps [from, to).
func (s *Service) checkBlocks(ctx context.Context, orgID, vehicleID int64, from time.Time, to *time.Time) error {
	rows, err := s.repo.BusyIntervals(ctx, orgID, vehicleID, from, to, []string{"block"})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	items := make([]Conflict, 0, len(rows))
	for _, row := range rows {
		items = append(items, Conflict{
			Kind: row.Kind, UUID: row.RefUuid, StartsAt: row.StartsAt.Time.UTC(), EndsAt: timePtr(row.EndsAt),
		})
	}
	return conflict(ConflictBlockOverlap, "the vehicle is blocked in this period", items...)
}

func (s *Service) transferWriteErr(ctx context.Context, orgID, vehicleID int64, f repository.TransferFields, excludeID int64, err error) error {
	if errors.Is(err, repository.ErrTransferOverlap) {
		if cerr := s.checkTransferWindow(ctx, orgID, vehicleID, f, excludeID); cerr != nil {
			return cerr
		}
		return conflict(ConflictTransferOverlap, "the vehicle already has a transfer in this period")
	}
	return err
}

func stateErr(err error, msg string) error {
	if errors.Is(err, repository.ErrNotFound) {
		return conflict(ConflictState, msg)
	}
	if errors.Is(err, vehicleusecase.ErrNotFound) {
		return ErrVehicleNotFound
	}
	return err
}

func transferNote(row repository.TransferRow, extra string) string {
	note := "transfer " + row.FromBranchCode + " → " + row.ToBranchCode
	if extra = strings.TrimSpace(extra); extra != "" {
		note += ": " + extra
	}
	return note
}
