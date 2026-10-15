// Package repository is the Postgres access path of the fleet operations
// resources (vehicle blocks, transfers, one-way rules). Every method takes
// the organization id explicitly and every query filters by it: a uuid of
// another organization reads as not found.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned for rows missing in the organization.
	ErrNotFound = errors.New("not found")
	// ErrBlockOverlap is returned when the EXCLUDE constraint of
	// vehicle_blocks rejects a write (a concurrent overlapping block).
	ErrBlockOverlap = errors.New("block overlaps another block")
	// ErrTransferOverlap is returned when the EXCLUDE constraint of
	// vehicle_transfers rejects a write.
	ErrTransferOverlap = errors.New("transfer overlaps another open transfer")
	// ErrRouteTaken is returned when a live rule already covers the route
	// and product.
	ErrRouteTaken = errors.New("route taken")
)

// Row aliases (sqlc rows used by the use case).
type (
	VehicleRow  = db.GetFleetOpsVehicleRow
	BlockRow    = db.GetVehicleBlockRow
	TransferRow = db.GetVehicleTransferRow
	RuleRow     = db.GetOneWayRuleRow
)

// BlockFields are the writable columns of a block.
type BlockFields struct {
	Kind     string
	StartsAt time.Time
	EndsAt   *time.Time
	Reason   string
}

// TransferFields are the writable columns of a transfer.
type TransferFields struct {
	VehicleID         int64
	FromBranchID      int64
	ToBranchID        int64
	ResponsibleUserID pgtype.Int8
	PlannedDeparture  time.Time
	PlannedArrival    time.Time
	Cost              pgtype.Numeric
	Note              string
}

// RuleFields are the writable columns of a one-way rule.
type RuleFields struct {
	FromBranchID    int64
	ToBranchID      int64
	RentalProductID pgtype.Int8
	Allowed         bool
	Fee             pgtype.Numeric
	Note            string
	IsActive        bool
}

// Postgres implements the repository with sqlc queries.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository. pool may be nil when the caller never
// opens transactions (InTx then runs fn without one).
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// Queries is the sqlc handle of this repository (bound to the transaction
// inside InTx); other modules' services run on it to share the transaction.
func (r *Postgres) Queries() *db.Queries {
	return r.q
}

// InTx runs fn with a repository bound to one transaction.
func (r *Postgres) InTx(ctx context.Context, fn func(tx *Postgres) error) error {
	if r.pool == nil {
		return fn(r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("fleetops: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&Postgres{q: r.q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- lookups ---------------------------------------------------------------------

func (r *Postgres) GetVehicle(ctx context.Context, orgID int64, id uuid.UUID) (VehicleRow, error) {
	row, err := r.q.GetFleetOpsVehicle(ctx, db.GetFleetOpsVehicleParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetBranch(ctx context.Context, orgID int64, id uuid.UUID) (db.Branch, error) {
	row, err := r.q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetProduct(ctx context.Context, orgID int64, id uuid.UUID) (db.RentalProduct, error) {
	row, err := r.q.GetRentalProductByUUID(ctx, db.GetRentalProductByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetMember(ctx context.Context, orgID int64, userUUID uuid.UUID) (db.GetTenantMemberByUserUUIDRow, error) {
	row, err := r.q.GetTenantMemberByUserUUID(ctx, db.GetTenantMemberByUserUUIDParams{OrganizationID: orgID, UserUuid: userUUID})
	return row, mapErr(err)
}

func (r *Postgres) ListMembers(ctx context.Context, orgID int64, q string, limit int32) ([]db.ListTenantMembersRow, error) {
	return r.q.ListTenantMembers(ctx, db.ListTenantMembersParams{
		OrganizationID: orgID, Q: textArg(q), SortField: "name", LimitCount: limit,
	})
}

// --- blocks --------------------------------------------------------------------

func (r *Postgres) CreateBlock(ctx context.Context, orgID, vehicleID int64, f BlockFields, actor pgtype.Int8) (uuid.UUID, error) {
	id, err := r.q.CreateVehicleBlock(ctx, db.CreateVehicleBlockParams{
		OrganizationID: orgID, VehicleID: vehicleID, Kind: f.Kind,
		StartsAt: ts(f.StartsAt), EndsAt: tsPtr(f.EndsAt), Reason: f.Reason, CreatedBy: actor,
	})
	return id, mapErr(err)
}

func (r *Postgres) UpdateBlock(ctx context.Context, orgID, vehicleID int64, id uuid.UUID, f BlockFields) error {
	n, err := r.q.UpdateVehicleBlock(ctx, db.UpdateVehicleBlockParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id, Kind: f.Kind,
		StartsAt: ts(f.StartsAt), EndsAt: tsPtr(f.EndsAt), Reason: f.Reason,
	})
	return rowsErr(n, err)
}

func (r *Postgres) ReleaseBlock(ctx context.Context, orgID, vehicleID int64, id uuid.UUID, at time.Time, actor pgtype.Int8) error {
	n, err := r.q.ReleaseVehicleBlock(ctx, db.ReleaseVehicleBlockParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id, ReleasedAt: ts(at), ReleasedBy: actor,
	})
	return rowsErr(n, err)
}

func (r *Postgres) DeleteBlock(ctx context.Context, orgID, vehicleID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteVehicleBlock(ctx, db.SoftDeleteVehicleBlockParams{
		OrganizationID: orgID, VehicleID: vehicleID, Uuid: id,
	})
	return rowsErr(n, err)
}

func (r *Postgres) GetBlock(ctx context.Context, orgID, vehicleID int64, id uuid.UUID) (BlockRow, error) {
	row, err := r.q.GetVehicleBlock(ctx, db.GetVehicleBlockParams{OrganizationID: orgID, VehicleID: vehicleID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) ListBlocks(ctx context.Context, orgID, vehicleID int64, f model.BlockFilter, now time.Time) ([]BlockRow, int64, error) {
	rows, err := r.q.ListVehicleBlocks(ctx, db.ListVehicleBlocksParams{
		OrganizationID: orgID, VehicleID: vehicleID, Kinds: stringsArg(f.Kinds), States: stringsArg(f.States),
		Now: ts(now), LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountVehicleBlocks(ctx, db.CountVehicleBlocksParams{
		OrganizationID: orgID, VehicleID: vehicleID, Kinds: stringsArg(f.Kinds), States: stringsArg(f.States), Now: ts(now),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]BlockRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, BlockRow(row))
	}
	return out, total, nil
}

func (r *Postgres) OverlappingBlocks(ctx context.Context, orgID, vehicleID int64, start time.Time, end *time.Time, excludeID int64) ([]db.ListOverlappingVehicleBlocksRow, error) {
	return r.q.ListOverlappingVehicleBlocks(ctx, db.ListOverlappingVehicleBlocksParams{
		OrganizationID: orgID, VehicleID: vehicleID, StartsAt: ts(start), EndsAt: tsPtr(end), ExcludeID: excludeID,
	})
}

// BusyIntervals reads vehicle_busy_intervals for one vehicle in [from, to).
func (r *Postgres) BusyIntervals(ctx context.Context, orgID, vehicleID int64, from time.Time, to *time.Time, kinds []string) ([]db.ListVehicleBusyIntervalsRow, error) {
	return r.q.ListVehicleBusyIntervals(ctx, db.ListVehicleBusyIntervalsParams{
		OrganizationID: orgID, VehicleID: vehicleID, FromAt: ts(from), ToAt: tsPtr(to), Kinds: stringsArg(kinds),
	})
}

// --- transfers -----------------------------------------------------------------

func (r *Postgres) CreateTransfer(ctx context.Context, orgID int64, f TransferFields, actor pgtype.Int8) (uuid.UUID, error) {
	id, err := r.q.CreateVehicleTransfer(ctx, db.CreateVehicleTransferParams{
		OrganizationID: orgID, VehicleID: f.VehicleID, FromBranchID: f.FromBranchID, ToBranchID: f.ToBranchID,
		ResponsibleUserID: f.ResponsibleUserID, PlannedDeparture: ts(f.PlannedDeparture),
		PlannedArrival: ts(f.PlannedArrival), Cost: f.Cost, Note: f.Note, CreatedBy: actor,
	})
	return id, mapErr(err)
}

func (r *Postgres) UpdateTransferPlan(ctx context.Context, orgID int64, id uuid.UUID, f TransferFields) error {
	n, err := r.q.UpdateVehicleTransferPlan(ctx, db.UpdateVehicleTransferPlanParams{
		OrganizationID: orgID, Uuid: id, FromBranchID: f.FromBranchID, ToBranchID: f.ToBranchID,
		ResponsibleUserID: f.ResponsibleUserID, PlannedDeparture: ts(f.PlannedDeparture),
		PlannedArrival: ts(f.PlannedArrival), Cost: f.Cost, Note: f.Note,
	})
	return rowsErr(n, err)
}

func (r *Postgres) SetTransferCost(ctx context.Context, orgID int64, id uuid.UUID, cost pgtype.Numeric, note string) error {
	n, err := r.q.SetVehicleTransferCost(ctx, db.SetVehicleTransferCostParams{
		OrganizationID: orgID, Uuid: id, Cost: cost, Note: note,
	})
	return rowsErr(n, err)
}

func (r *Postgres) StartTransfer(ctx context.Context, orgID int64, id uuid.UUID, at time.Time) error {
	n, err := r.q.StartVehicleTransfer(ctx, db.StartVehicleTransferParams{OrganizationID: orgID, Uuid: id, ActualDeparture: ts(at)})
	return rowsErr(n, err)
}

func (r *Postgres) CompleteTransfer(ctx context.Context, orgID int64, id uuid.UUID, at time.Time) error {
	n, err := r.q.CompleteVehicleTransfer(ctx, db.CompleteVehicleTransferParams{OrganizationID: orgID, Uuid: id, ActualArrival: ts(at)})
	return rowsErr(n, err)
}

func (r *Postgres) CancelTransfer(ctx context.Context, orgID int64, id uuid.UUID, note string) error {
	n, err := r.q.CancelVehicleTransfer(ctx, db.CancelVehicleTransferParams{OrganizationID: orgID, Uuid: id, Note: note})
	return rowsErr(n, err)
}

func (r *Postgres) GetTransfer(ctx context.Context, orgID int64, id uuid.UUID) (TransferRow, error) {
	row, err := r.q.GetVehicleTransfer(ctx, db.GetVehicleTransferParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) ListTransfers(ctx context.Context, orgID int64, f model.TransferFilter) ([]TransferRow, int64, error) {
	var vehicle pgtype.UUID
	if f.VehicleUUID != nil {
		vehicle = pgtype.UUID{Bytes: *f.VehicleUUID, Valid: true}
	}
	rows, err := r.q.ListVehicleTransfers(ctx, db.ListVehicleTransfersParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, Statuses: stringsArg(f.Statuses),
		FromBranchUuids: uuidsArg(f.FromBranchUUIDs), ToBranchUuids: uuidsArg(f.ToBranchUUIDs),
		VehicleUuid: vehicle, Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountVehicleTransfers(ctx, db.CountVehicleTransfersParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, Statuses: stringsArg(f.Statuses),
		FromBranchUuids: uuidsArg(f.FromBranchUUIDs), ToBranchUuids: uuidsArg(f.ToBranchUUIDs),
		VehicleUuid: vehicle, Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]TransferRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, TransferRow(row))
	}
	return out, total, nil
}

func (r *Postgres) TransferFacets(ctx context.Context, orgID int64, branchIDs []int64) ([]db.ListVehicleTransferFacetsRow, error) {
	return r.q.ListVehicleTransferFacets(ctx, db.ListVehicleTransferFacetsParams{OrganizationID: orgID, BranchIds: branchIDs})
}

func (r *Postgres) OverlappingTransfers(ctx context.Context, orgID, vehicleID int64, start time.Time, end *time.Time, excludeID int64) ([]db.ListOverlappingVehicleTransfersRow, error) {
	return r.q.ListOverlappingVehicleTransfers(ctx, db.ListOverlappingVehicleTransfersParams{
		OrganizationID: orgID, VehicleID: vehicleID, StartsAt: ts(start), EndsAt: tsPtr(end), ExcludeID: excludeID,
	})
}

// AffectedAllocations lists the active holds / reservations of a vehicle
// that overlap [start, end) (end nil = unbounded).
func (r *Postgres) AffectedAllocations(ctx context.Context, orgID, vehicleID int64, start time.Time, end *time.Time, now time.Time) ([]db.ListBlockAffectedAllocationsRow, error) {
	return r.q.ListBlockAffectedAllocations(ctx, db.ListBlockAffectedAllocationsParams{
		OrganizationID: orgID, VehicleID: vehicleID, StartsAt: ts(start), EndsAt: tsPtr(end), NowAt: ts(now),
	})
}

func (r *Postgres) Movements(ctx context.Context, orgID, vehicleID int64) ([]db.ListVehicleMovementsRow, error) {
	return r.q.ListVehicleMovements(ctx, db.ListVehicleMovementsParams{OrganizationID: orgID, VehicleID: vehicleID})
}

// --- one-way rules -------------------------------------------------------------

func (r *Postgres) CreateRule(ctx context.Context, orgID int64, f RuleFields) (uuid.UUID, error) {
	id, err := r.q.CreateOneWayRule(ctx, db.CreateOneWayRuleParams{
		OrganizationID: orgID, FromBranchID: f.FromBranchID, ToBranchID: f.ToBranchID,
		RentalProductID: f.RentalProductID, Allowed: f.Allowed, Fee: f.Fee, Note: f.Note, IsActive: f.IsActive,
	})
	return id, mapErr(err)
}

func (r *Postgres) UpdateRule(ctx context.Context, orgID int64, id uuid.UUID, f RuleFields) error {
	n, err := r.q.UpdateOneWayRule(ctx, db.UpdateOneWayRuleParams{
		OrganizationID: orgID, Uuid: id, FromBranchID: f.FromBranchID, ToBranchID: f.ToBranchID,
		RentalProductID: f.RentalProductID, Allowed: f.Allowed, Fee: f.Fee, Note: f.Note, IsActive: f.IsActive,
	})
	return rowsErr(n, err)
}

func (r *Postgres) SetRuleActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) error {
	n, err := r.q.SetOneWayRuleActive(ctx, db.SetOneWayRuleActiveParams{OrganizationID: orgID, Uuid: id, IsActive: active})
	return rowsErr(n, err)
}

func (r *Postgres) DeleteRule(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteOneWayRule(ctx, db.SoftDeleteOneWayRuleParams{OrganizationID: orgID, Uuid: id})
	return rowsErr(n, err)
}

func (r *Postgres) RouteTaken(ctx context.Context, orgID int64, f RuleFields, exclude *uuid.UUID) (bool, error) {
	var ex pgtype.UUID
	if exclude != nil {
		ex = pgtype.UUID{Bytes: *exclude, Valid: true}
	}
	return r.q.OneWayRouteTaken(ctx, db.OneWayRouteTakenParams{
		OrganizationID: orgID, FromBranchID: f.FromBranchID, ToBranchID: f.ToBranchID,
		RentalProductID: f.RentalProductID, ExcludeUuid: ex,
	})
}

func (r *Postgres) GetRule(ctx context.Context, orgID int64, id uuid.UUID) (RuleRow, error) {
	row, err := r.q.GetOneWayRule(ctx, db.GetOneWayRuleParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) ListRules(ctx context.Context, orgID int64, f model.OneWayFilter) ([]RuleRow, int64, error) {
	rows, err := r.q.ListOneWayRules(ctx, db.ListOneWayRulesParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, IsActive: boolArg(f.IsActive), Allowed: boolArg(f.Allowed),
		FromBranchUuids: uuidsArg(f.FromBranchUUIDs), ToBranchUuids: uuidsArg(f.ToBranchUUIDs), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOneWayRules(ctx, db.CountOneWayRulesParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, IsActive: boolArg(f.IsActive), Allowed: boolArg(f.Allowed),
		FromBranchUuids: uuidsArg(f.FromBranchUUIDs), ToBranchUuids: uuidsArg(f.ToBranchUUIDs), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]RuleRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, RuleRow(row))
	}
	return out, total, nil
}

func (r *Postgres) RuleUUIDs(ctx context.Context, orgID int64, f model.OneWayFilter, limit int32) ([]uuid.UUID, error) {
	return r.q.ListOneWayRuleUUIDs(ctx, db.ListOneWayRuleUUIDsParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, IsActive: boolArg(f.IsActive), Allowed: boolArg(f.Allowed),
		FromBranchUuids: uuidsArg(f.FromBranchUUIDs), ToBranchUuids: uuidsArg(f.ToBranchUUIDs), Q: textArg(f.Q),
		LimitCount: limit,
	})
}

func (r *Postgres) RuleFacets(ctx context.Context, orgID int64, branchIDs []int64) ([]db.ListOneWayRuleFacetsRow, error) {
	return r.q.ListOneWayRuleFacets(ctx, db.ListOneWayRuleFacetsParams{OrganizationID: orgID, BranchIds: branchIDs})
}

func (r *Postgres) RulesForRoute(ctx context.Context, orgID, fromID, toID int64, productID pgtype.Int8) ([]db.ListOneWayRulesForRouteRow, error) {
	return r.q.ListOneWayRulesForRoute(ctx, db.ListOneWayRulesForRouteParams{
		OrganizationID: orgID, FromBranchID: fromID, ToBranchID: toID, RentalProductID: productID,
	})
}

// --- activity -------------------------------------------------------------------

func (r *Postgres) ListActivity(ctx context.Context, orgID int64, resource string, id uuid.UUID, limit, offset int32) ([]db.ListOrganizationResourceActivityRow, int64, error) {
	org := pgtype.Int8{Int64: orgID, Valid: true}
	ru := pgtype.UUID{Bytes: id, Valid: true}
	rows, err := r.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: resource, ResourceUuid: ru, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: org, Resource: resource, ResourceUuid: ru,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// --- helpers ----------------------------------------------------------------------

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == "23P01" && pgErr.ConstraintName == "ex_vehicle_blocks_overlap":
			return ErrBlockOverlap
		case pgErr.Code == "23P01" && pgErr.ConstraintName == "ex_vehicle_transfers_overlap":
			return ErrTransferOverlap
		case pgErr.Code == "23505" && pgErr.ConstraintName == "uq_one_way_rules_organization_route":
			return ErrRouteTaken
		}
	}
	return err
}

func rowsErr(n int64, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func boolArg(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

func stringsArg(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func uuidsArg(v []uuid.UUID) []uuid.UUID {
	if len(v) == 0 {
		return nil
	}
	return v
}
