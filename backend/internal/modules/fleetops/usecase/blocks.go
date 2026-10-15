package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/google/uuid"
)

// blockState places a block relative to now.
func blockState(start time.Time, end *time.Time, now time.Time) string {
	switch {
	case start.After(now):
		return model.BlockScheduled
	case endsBefore(end, now):
		return model.BlockEnded
	default:
		return model.BlockActive
	}
}

func (s *Service) toBlock(row repository.BlockRow, vehicleUUID uuid.UUID, now time.Time) model.Block {
	start := row.StartsAt.Time.UTC()
	end := timePtr(row.EndsAt)
	return model.Block{
		UUID: row.Uuid, VehicleUUID: vehicleUUID, Kind: row.Kind, StartsAt: start, EndsAt: end,
		State: blockState(start, end, now), Reason: row.Reason,
		CreatedBy:  actorFrom(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		ReleasedAt: timePtr(row.ReleasedAt),
		ReleasedBy: actorFrom(row.ReleasedByUuid, row.ReleasedByName, row.ReleasedBySurname, row.ReleasedByEmail),
		CreatedAt:  row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
}

// ListBlocks lists the blocks of a vehicle, newest start first.
func (s *Service) ListBlocks(ctx context.Context, orgID int64, vehicleUUID uuid.UUID, f model.BlockFilter) ([]model.Block, int64, error) {
	v, err := s.getVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return nil, 0, err
	}
	kinds := make([]string, 0, len(f.Kinds))
	for _, k := range f.Kinds {
		if model.Contains(model.BlockKinds, k) {
			kinds = append(kinds, k)
		}
	}
	states := make([]string, 0, len(f.States))
	for _, st := range f.States {
		if model.Contains(model.BlockStates, st) {
			states = append(states, st)
		}
	}
	f.Kinds, f.States = kinds, states
	now := s.clock()
	rows, total, err := s.repo.ListBlocks(ctx, orgID, v.ID, f, now)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Block, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.toBlock(row, v.Uuid, now))
	}
	return out, total, nil
}

// CreateBlock blocks a vehicle for [starts_at, ends_at) (starts_at
// defaults to now, a missing ends_at is open-ended). With sync_status and a
// block that covers now, the vehicle status follows the block kind. The
// result lists the open transfers the block overlaps.
func (s *Service) CreateBlock(ctx context.Context, orgID int64, vehicleUUID uuid.UUID, in model.BlockInput) (model.BlockResult, error) {
	v, err := s.getVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return model.BlockResult{}, err
	}
	now := s.clock()
	ve := &ValidationError{}
	fields := repository.BlockFields{StartsAt: now}
	if in.Kind == nil {
		ve.add("kind", "required", "kind is required")
	}
	s.applyBlockInput(ve, &fields, in)
	syncStatus := ""
	if in.SyncStatus {
		syncStatus = s.syncStatusFor(ve, fields, now)
	}
	if err := ve.orNil(); err != nil {
		return model.BlockResult{}, err
	}
	if err := s.checkBlockOverlap(ctx, orgID, v.ID, fields, 0); err != nil {
		return model.BlockResult{}, err
	}
	var id uuid.UUID
	changed := false
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		var err error
		id, err = tx.CreateBlock(ctx, orgID, v.ID, fields, actorArg(ctx))
		if err != nil {
			return err
		}
		if syncStatus != "" && syncStatus != v.Status {
			_, err = s.vehicleTx(tx).ApplyOperationalChange(unscoped(ctx), orgID, v.Uuid, vehicleusecase.OperationalChange{
				Status: syncStatus, Note: blockNote(fields),
			})
			changed = err == nil
		}
		return err
	})
	if err != nil {
		return model.BlockResult{}, s.blockWriteErr(ctx, orgID, v.ID, fields, 0, err)
	}
	return s.reactToAffected(ctx, orgID, v.ID, v.Uuid, id, changed)
}

// reactToAffected hands the allocations a block overlaps to the affected
// handler (re-plan / resolution tasks) and answers the state after it.
func (s *Service) reactToAffected(ctx context.Context, orgID, vehicleID int64, vehicleUUID, blockUUID uuid.UUID, changed bool) (model.BlockResult, error) {
	res, err := s.blockResult(ctx, orgID, vehicleID, vehicleUUID, blockUUID, changed)
	if err != nil || s.affected == nil {
		return res, err
	}
	allocs := []uuid.UUID{}
	for _, a := range res.Affected {
		if a.Kind == model.AffectedHold || a.Kind == model.AffectedReservation {
			allocs = append(allocs, a.UUID)
		}
	}
	if len(allocs) == 0 {
		return res, nil
	}
	s.affected.AllocationsAffected(ctx, orgID, "block", blockUUID, allocs)
	return s.blockResult(ctx, orgID, vehicleID, vehicleUUID, blockUUID, changed)
}

// PatchBlock changes the kind, range or reason of a block ("süreyi uzat"
// is a new ends_at). The vehicle status is not touched.
func (s *Service) PatchBlock(ctx context.Context, orgID int64, vehicleUUID, blockUUID uuid.UUID, in model.BlockInput) (model.BlockResult, error) {
	v, err := s.getVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return model.BlockResult{}, err
	}
	row, err := s.repo.GetBlock(ctx, orgID, v.ID, blockUUID)
	if err != nil {
		return model.BlockResult{}, mapNotFound(err, ErrBlockNotFound)
	}
	fields := repository.BlockFields{
		Kind: row.Kind, StartsAt: row.StartsAt.Time.UTC(), EndsAt: timePtr(row.EndsAt), Reason: row.Reason,
	}
	ve := &ValidationError{}
	if in.SyncStatus {
		ve.add("sync_status", "create_only", "sync_status is only accepted when a block is created")
	}
	s.applyBlockInput(ve, &fields, in)
	if err := ve.orNil(); err != nil {
		return model.BlockResult{}, err
	}
	if err := s.checkBlockOverlap(ctx, orgID, v.ID, fields, row.ID); err != nil {
		return model.BlockResult{}, err
	}
	if err := s.repo.UpdateBlock(ctx, orgID, v.ID, blockUUID, fields); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.BlockResult{}, ErrBlockNotFound
		}
		return model.BlockResult{}, s.blockWriteErr(ctx, orgID, v.ID, fields, row.ID, err)
	}
	return s.reactToAffected(ctx, orgID, v.ID, v.Uuid, blockUUID, false)
}

// ReleaseBlock is "Engeli kapat": an active block ends now. With
// sync_status the vehicle goes back to available when its status is still
// the one the block kind sets.
func (s *Service) ReleaseBlock(ctx context.Context, orgID int64, vehicleUUID, blockUUID uuid.UUID, in model.ReleaseInput) (model.BlockResult, error) {
	v, err := s.getVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return model.BlockResult{}, err
	}
	row, err := s.repo.GetBlock(ctx, orgID, v.ID, blockUUID)
	if err != nil {
		return model.BlockResult{}, mapNotFound(err, ErrBlockNotFound)
	}
	now := s.clock()
	start, end := row.StartsAt.Time.UTC(), timePtr(row.EndsAt)
	switch blockState(start, end, now) {
	case model.BlockScheduled:
		return model.BlockResult{}, validation("block", "not_started", "the block has not started yet; delete it instead")
	case model.BlockEnded:
		return model.BlockResult{}, validation("block", "already_ended", "the block has already ended")
	}
	if !now.After(start) {
		// A block that starts this very instant would become empty.
		return model.BlockResult{}, validation("block", "not_started", "the block has not started yet; delete it instead")
	}
	changed := false
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if err := tx.ReleaseBlock(ctx, orgID, v.ID, blockUUID, now, actorArg(ctx)); err != nil {
			return err
		}
		if in.SyncStatus && model.BlockStatus[row.Kind] != "" && v.Status == model.BlockStatus[row.Kind] {
			_, err := s.vehicleTx(tx).ApplyOperationalChange(unscoped(ctx), orgID, v.Uuid, vehicleusecase.OperationalChange{
				Status: "available", Note: "block released",
			})
			changed = err == nil
			return err
		}
		return nil
	})
	if err != nil {
		return model.BlockResult{}, mapNotFound(err, ErrBlockNotFound)
	}
	return s.blockResult(ctx, orgID, v.ID, v.Uuid, blockUUID, changed)
}

// DeleteBlock soft-deletes a block (entered by mistake). The vehicle
// status is not touched.
func (s *Service) DeleteBlock(ctx context.Context, orgID int64, vehicleUUID, blockUUID uuid.UUID) error {
	v, err := s.getVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return err
	}
	return mapNotFound(s.repo.DeleteBlock(ctx, orgID, v.ID, blockUUID), ErrBlockNotFound)
}

// StartMaintenance is "Bakıma al" for one vehicle (vehicles bulk action):
// an open-ended maintenance block from now, optionally with the status
// change. It returns the block and the previous status ("" when the
// status was not changed).
func (s *Service) StartMaintenance(ctx context.Context, orgID int64, vehicleUUID uuid.UUID, reason string, syncStatus bool) (uuid.UUID, string, error) {
	v, err := s.getVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return uuid.Nil, "", err
	}
	kind := model.BlockMaintenance
	res, err := s.CreateBlock(ctx, orgID, vehicleUUID, model.BlockInput{Kind: &kind, Reason: &reason, SyncStatus: syncStatus})
	if err != nil {
		return uuid.Nil, "", err
	}
	prev := ""
	if res.StatusChanged {
		prev = v.Status
	}
	return res.Block.UUID, prev, nil
}

// UndoMaintenance reverts StartMaintenance (bulk rollback): the block is
// deleted and the previous status restored when it was changed.
func (s *Service) UndoMaintenance(ctx context.Context, orgID int64, vehicleUUID, blockUUID uuid.UUID, previousStatus string) error {
	v, err := s.getVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return err
	}
	return s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if err := tx.DeleteBlock(ctx, orgID, v.ID, blockUUID); err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		if previousStatus == "" {
			return nil
		}
		_, err := s.vehicleTx(tx).ApplyOperationalChange(unscoped(ctx), orgID, v.Uuid, vehicleusecase.OperationalChange{
			Status: previousStatus, Note: "rollback",
		})
		return err
	})
}

// ItemError reports whether err refuses one vehicle of a bulk job.
func (s *Service) ItemError(err error) bool {
	var ve *ValidationError
	_, isConflict := IsConflict(err)
	return isConflict || errors.As(err, &ve) || errors.Is(err, ErrVehicleNotFound)
}

func (s *Service) applyBlockInput(ve *ValidationError, f *repository.BlockFields, in model.BlockInput) {
	if in.Kind != nil {
		k := strings.TrimSpace(*in.Kind)
		if !model.Contains(model.BlockKinds, k) {
			ve.add("kind", "invalid", "kind must be one of "+strings.Join(model.BlockKinds, ", "))
		}
		f.Kind = k
	}
	if in.StartsAt != nil {
		f.StartsAt = in.StartsAt.UTC().Truncate(time.Microsecond)
	}
	if in.EndsAt.Set {
		if in.EndsAt.Value == nil {
			f.EndsAt = nil
		} else {
			end := in.EndsAt.Value.UTC().Truncate(time.Microsecond)
			f.EndsAt = &end
		}
	}
	if f.EndsAt != nil && !f.EndsAt.After(f.StartsAt) {
		ve.add("ends_at", "before_start", "ends_at must be after starts_at")
	}
	if in.Reason != nil {
		f.Reason = checkNote(ve, "reason", *in.Reason)
	}
}

// syncStatusFor returns the vehicle status of a block that covers now.
func (s *Service) syncStatusFor(ve *ValidationError, f repository.BlockFields, now time.Time) string {
	status := model.BlockStatus[f.Kind]
	switch {
	case ve.has("kind"):
		return ""
	case status == "":
		ve.add("sync_status", "no_status", "blocks of kind other have no vehicle status")
	case f.StartsAt.After(now) || endsBefore(f.EndsAt, now):
		ve.add("sync_status", "not_current", "the vehicle status follows a block only when the block covers now")
	}
	return status
}

func (s *Service) checkBlockOverlap(ctx context.Context, orgID, vehicleID int64, f repository.BlockFields, excludeID int64) error {
	rows, err := s.repo.OverlappingBlocks(ctx, orgID, vehicleID, f.StartsAt, f.EndsAt, excludeID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	items := make([]Conflict, 0, len(rows))
	for _, row := range rows {
		items = append(items, Conflict{
			Kind: "block", UUID: row.Uuid, StartsAt: row.StartsAt.Time.UTC(), EndsAt: timePtr(row.EndsAt),
		})
	}
	return conflict(ConflictBlockOverlap, "the vehicle already has a block in this period", items...)
}

// blockWriteErr turns the EXCLUDE violation of a concurrent writer into
// the same 409 as the pre-check.
func (s *Service) blockWriteErr(ctx context.Context, orgID, vehicleID int64, f repository.BlockFields, excludeID int64, err error) error {
	if errors.Is(err, repository.ErrBlockOverlap) {
		if cerr := s.checkBlockOverlap(ctx, orgID, vehicleID, f, excludeID); cerr != nil {
			return cerr
		}
		return conflict(ConflictBlockOverlap, "the vehicle already has a block in this period")
	}
	return mapNotFound(err, ErrVehicleNotFound)
}

func (s *Service) blockResult(ctx context.Context, orgID, vehicleID int64, vehicleUUID, blockUUID uuid.UUID, changed bool) (model.BlockResult, error) {
	row, err := s.repo.GetBlock(ctx, orgID, vehicleID, blockUUID)
	if err != nil {
		return model.BlockResult{}, mapNotFound(err, ErrBlockNotFound)
	}
	now := s.clock()
	block := s.toBlock(row, vehicleUUID, now)
	affected, err := s.affectedBy(ctx, orgID, vehicleID, block)
	if err != nil {
		return model.BlockResult{}, err
	}
	v, err := s.repo.GetVehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return model.BlockResult{}, mapNotFound(err, ErrVehicleNotFound)
	}
	return model.BlockResult{Block: block, Affected: affected, VehicleStatus: v.Status, StatusChanged: changed}, nil
}

// affectedBy lists the open transfers and the active allocations (holds /
// reservations, kind = allocation kind, label = product code) a block
// overlaps. Blocks always win: the availability engine never plans on a
// blocked vehicle, and the overlapped allocations are for a person (or a
// later re-plan) to move.
func (s *Service) affectedBy(ctx context.Context, orgID, vehicleID int64, b model.Block) ([]model.Affected, error) {
	out := []model.Affected{}
	allocs, err := s.repo.AffectedAllocations(ctx, orgID, vehicleID, b.StartsAt, b.EndsAt, s.clock())
	if err != nil {
		return nil, err
	}
	for _, a := range allocs {
		out = append(out, model.Affected{
			Kind: a.Kind, UUID: a.Uuid, Label: a.ProductCode, Status: "active",
			StartsAt: a.StartsAt.Time.UTC(), EndsAt: timePtr(a.EndsAt),
		})
	}
	rows, err := s.repo.OverlappingTransfers(ctx, orgID, vehicleID, b.StartsAt, b.EndsAt, 0)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		label := ""
		if t, err := s.repo.GetTransfer(ctx, orgID, row.Uuid); err == nil {
			label = t.FromBranchName + " → " + t.ToBranchName
		}
		out = append(out, model.Affected{
			Kind: model.AffectedTransfer, UUID: row.Uuid, Label: label, Status: row.Status,
			StartsAt: row.StartsAt.Time.UTC(), EndsAt: timePtr(row.EndsAt),
		})
	}
	return out, nil
}

func blockNote(f repository.BlockFields) string {
	if f.Reason != "" {
		return "block (" + f.Kind + "): " + f.Reason
	}
	return "block (" + f.Kind + ")"
}
