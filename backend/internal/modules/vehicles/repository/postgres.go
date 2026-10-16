// Package repository is the Postgres access path of the vehicles resource.
// Every method takes the organization id explicitly and every query on a
// tenant table filters by it: a uuid from another organization reads as not
// found. Platform catalog lookups are not organization scoped.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned for rows missing in the organization.
	ErrNotFound = errors.New("not found")
	// ErrPlateTaken is returned when an active vehicle already has the plate.
	ErrPlateTaken = errors.New("plate taken")
)

// VehicleFields are the writable columns of a vehicle.
type VehicleFields struct {
	Plate              string
	VIN                string
	CatalogTypeID      pgtype.Int8
	ManualEntry        bool
	ManualBrand        string
	ManualModel        string
	ManualType         string
	ModelYear          int16
	Transmission       pgtype.Text
	Fuel               pgtype.Text
	BodyType           pgtype.Text
	Seats              pgtype.Int2
	Luggage            pgtype.Int2
	AirConditioning    bool
	AttributesVerified bool
	Color              string
	HomeBranchID       int64
	CurrentBranchID    int64
	FleetPool          string
	Ownership          string
	SupplierName       string
	SupplierID         pgtype.Int8
	InServiceDate      pgtype.Date
	OutOfServiceDate   pgtype.Date
	OdometerKm         int32
	FuelLevel          pgtype.Int2
	Status             string
	IsActive           bool
}

// EventFields are the columns of a status event.
type EventFields struct {
	EventType  string
	FromStatus pgtype.Text
	ToStatus   string
	OdometerKm pgtype.Int4
	FuelLevel  pgtype.Int2
	Note       string
	ActorID    pgtype.Int8
}

// VehicleRow is a vehicle with its catalog, branch and kasko joins.
type VehicleRow = db.GetVehicleDetailRow

// GroupRow is a live rental group with its segment.
type GroupRow = db.ListLiveRentalGroupsForVehiclesRow

// Postgres implements the vehicles repository with sqlc queries.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository. pool may be nil when the caller never
// opens transactions (InTx then runs fn without one).
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// InTx runs fn with a repository bound to one transaction.
func (r *Postgres) InTx(ctx context.Context, fn func(tx *Postgres) error) error {
	if r.pool == nil {
		return fn(r)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("vehicles: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&Postgres{q: r.q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- vehicles -----------------------------------------------------------------

func (r *Postgres) ListVehicles(ctx context.Context, orgID int64, f model.ListFilter) ([]VehicleRow, int64, error) {
	rows, err := r.q.ListVehicles(ctx, db.ListVehiclesParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, IsActive: boolArg(f.IsActive),
		Statuses: stringsArg(f.Statuses), Ownerships: stringsArg(f.Ownerships), Pools: stringsArg(f.Pools),
		DocumentStatuses: stringsArg(f.DocumentStatuses), Today: dateArg(f.Today),
		DocumentHorizonDays: model.DocumentReminderDays,
		HomeBranchUuids:     uuidsArg(f.HomeBranchUUIDs), CurrentBranchUuids: uuidsArg(f.CurrentBranchUUIDs),
		GroupUuids: uuidsArg(f.GroupUUIDs), SegmentUuids: uuidsArg(f.SegmentUUIDs), SupplierUuids: uuidsArg(f.SupplierUUIDs), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountVehicles(ctx, db.CountVehiclesParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, IsActive: boolArg(f.IsActive),
		Statuses: stringsArg(f.Statuses), Ownerships: stringsArg(f.Ownerships), Pools: stringsArg(f.Pools),
		DocumentStatuses: stringsArg(f.DocumentStatuses), Today: dateArg(f.Today),
		DocumentHorizonDays: model.DocumentReminderDays,
		HomeBranchUuids:     uuidsArg(f.HomeBranchUUIDs), CurrentBranchUuids: uuidsArg(f.CurrentBranchUUIDs),
		GroupUuids: uuidsArg(f.GroupUUIDs), SegmentUuids: uuidsArg(f.SegmentUUIDs), SupplierUuids: uuidsArg(f.SupplierUUIDs), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]VehicleRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, VehicleRow(row))
	}
	return out, total, nil
}

func (r *Postgres) ListVehicleUUIDs(ctx context.Context, orgID int64, f model.ListFilter, limit int32) ([]uuid.UUID, error) {
	return r.q.ListVehicleUUIDs(ctx, db.ListVehicleUUIDsParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, IsActive: boolArg(f.IsActive),
		Statuses: stringsArg(f.Statuses), Ownerships: stringsArg(f.Ownerships), Pools: stringsArg(f.Pools),
		DocumentStatuses: stringsArg(f.DocumentStatuses), Today: dateArg(f.Today),
		DocumentHorizonDays: model.DocumentReminderDays,
		HomeBranchUuids:     uuidsArg(f.HomeBranchUUIDs), CurrentBranchUuids: uuidsArg(f.CurrentBranchUUIDs),
		GroupUuids: uuidsArg(f.GroupUUIDs), SegmentUuids: uuidsArg(f.SegmentUUIDs), SupplierUuids: uuidsArg(f.SupplierUUIDs), Q: textArg(f.Q),
		LimitCount: limit,
	})
}

func (r *Postgres) Facets(ctx context.Context, orgID int64, branchIDs []int64, today time.Time) ([]db.ListVehicleFacetsRow, error) {
	return r.q.ListVehicleFacets(ctx, db.ListVehicleFacetsParams{
		OrganizationID: orgID, BranchIds: branchIDs, Today: dateArg(today),
		DocumentHorizonDays: model.DocumentReminderDays,
	})
}

func (r *Postgres) GetVehicle(ctx context.Context, orgID int64, id uuid.UUID) (VehicleRow, error) {
	row, err := r.q.GetVehicleDetail(ctx, db.GetVehicleDetailParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GetVehicleByPlate(ctx context.Context, orgID int64, plate string) (db.Vehicle, error) {
	row, err := r.q.GetVehicleByPlate(ctx, db.GetVehicleByPlateParams{OrganizationID: orgID, Plate: plate})
	return row, mapErr(err)
}

func (r *Postgres) PlateTaken(ctx context.Context, orgID int64, plate string, exclude *uuid.UUID) (bool, error) {
	var ex pgtype.UUID
	if exclude != nil {
		ex = pgtype.UUID{Bytes: *exclude, Valid: true}
	}
	return r.q.VehiclePlateTaken(ctx, db.VehiclePlateTakenParams{OrganizationID: orgID, Plate: plate, ExcludeUuid: ex})
}

func (r *Postgres) CreateVehicle(ctx context.Context, orgID int64, f VehicleFields) (db.Vehicle, error) {
	row, err := r.q.CreateVehicle(ctx, db.CreateVehicleParams{
		OrganizationID: orgID, Plate: f.Plate, Vin: f.VIN, CatalogTypeID: f.CatalogTypeID,
		ManualEntry: f.ManualEntry, ManualBrand: f.ManualBrand, ManualModel: f.ManualModel, ManualType: f.ManualType,
		ModelYear: f.ModelYear, Transmission: f.Transmission, Fuel: f.Fuel, BodyType: f.BodyType,
		Seats: f.Seats, Luggage: f.Luggage, AirConditioning: f.AirConditioning, AttributesVerified: f.AttributesVerified,
		Color: f.Color, HomeBranchID: f.HomeBranchID, CurrentBranchID: f.CurrentBranchID, FleetPool: f.FleetPool,
		Ownership: f.Ownership, SupplierName: f.SupplierName, SupplierID: f.SupplierID, InServiceDate: f.InServiceDate,
		OutOfServiceDate: f.OutOfServiceDate, OdometerKm: f.OdometerKm, FuelLevel: f.FuelLevel,
		Status: f.Status, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateVehicle(ctx context.Context, orgID int64, id uuid.UUID, f VehicleFields) (db.Vehicle, error) {
	row, err := r.q.UpdateVehicle(ctx, db.UpdateVehicleParams{
		OrganizationID: orgID, Uuid: id, Plate: f.Plate, Vin: f.VIN, CatalogTypeID: f.CatalogTypeID,
		ManualEntry: f.ManualEntry, ManualBrand: f.ManualBrand, ManualModel: f.ManualModel, ManualType: f.ManualType,
		ModelYear: f.ModelYear, Transmission: f.Transmission, Fuel: f.Fuel, BodyType: f.BodyType,
		Seats: f.Seats, Luggage: f.Luggage, AirConditioning: f.AirConditioning, AttributesVerified: f.AttributesVerified,
		Color: f.Color, HomeBranchID: f.HomeBranchID, CurrentBranchID: f.CurrentBranchID, FleetPool: f.FleetPool,
		Ownership: f.Ownership, SupplierName: f.SupplierName, SupplierID: f.SupplierID, InServiceDate: f.InServiceDate,
		OutOfServiceDate: f.OutOfServiceDate, OdometerKm: f.OdometerKm, FuelLevel: f.FuelLevel,
		IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) SetStatus(ctx context.Context, orgID int64, id uuid.UUID, status string, km int32, fuel pgtype.Int2) (db.Vehicle, error) {
	row, err := r.q.SetVehicleStatus(ctx, db.SetVehicleStatusParams{
		OrganizationID: orgID, Uuid: id, Status: status, OdometerKm: km, FuelLevel: fuel,
	})
	return row, mapErr(err)
}

func (r *Postgres) SetCurrentBranch(ctx context.Context, orgID int64, id uuid.UUID, branchID int64) (db.Vehicle, error) {
	row, err := r.q.SetVehicleCurrentBranch(ctx, db.SetVehicleCurrentBranchParams{
		OrganizationID: orgID, Uuid: id, CurrentBranchID: branchID,
	})
	return row, mapErr(err)
}

func (r *Postgres) SetActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (db.Vehicle, error) {
	row, err := r.q.SetVehicleActive(ctx, db.SetVehicleActiveParams{OrganizationID: orgID, Uuid: id, IsActive: active})
	return row, mapErr(err)
}

func (r *Postgres) DeleteVehicle(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteVehicle(ctx, db.SoftDeleteVehicleParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- events -------------------------------------------------------------------

func (r *Postgres) InsertEvent(ctx context.Context, orgID, vehicleID int64, f EventFields) (db.VehicleEvent, error) {
	return r.q.InsertVehicleEvent(ctx, db.InsertVehicleEventParams{
		OrganizationID: orgID, VehicleID: vehicleID, EventType: f.EventType, FromStatus: f.FromStatus,
		ToStatus: f.ToStatus, OdometerKm: f.OdometerKm, FuelLevel: f.FuelLevel, Note: f.Note, ActorID: f.ActorID,
	})
}

func (r *Postgres) ListEvents(ctx context.Context, orgID, vehicleID int64, limit, offset int32) ([]db.ListVehicleEventsRow, int64, error) {
	rows, err := r.q.ListVehicleEvents(ctx, db.ListVehicleEventsParams{
		OrganizationID: orgID, VehicleID: vehicleID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountVehicleEvents(ctx, db.CountVehicleEventsParams{OrganizationID: orgID, VehicleID: vehicleID})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// --- rental groups --------------------------------------------------------------

// MembershipsByVehicle returns the live group memberships of each vehicle.
func (r *Postgres) MembershipsByVehicle(ctx context.Context, orgID int64, vehicleIDs []int64) (map[int64][]db.ListVehicleGroupsByVehicleIDsRow, error) {
	out := map[int64][]db.ListVehicleGroupsByVehicleIDsRow{}
	if len(vehicleIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListVehicleGroupsByVehicleIDs(ctx, db.ListVehicleGroupsByVehicleIDsParams{
		OrganizationID: orgID, VehicleIds: vehicleIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.VehicleID] = append(out[row.VehicleID], row)
	}
	return out, nil
}

func (r *Postgres) AddMembership(ctx context.Context, orgID, vehicleID, groupID int64) error {
	return r.q.AddVehicleRentalGroup(ctx, db.AddVehicleRentalGroupParams{
		OrganizationID: orgID, VehicleID: vehicleID, RentalGroupID: groupID,
	})
}

func (r *Postgres) RemoveMembership(ctx context.Context, orgID, vehicleID, groupID int64) (bool, error) {
	n, err := r.q.RemoveVehicleRentalGroup(ctx, db.RemoveVehicleRentalGroupParams{
		OrganizationID: orgID, VehicleID: vehicleID, RentalGroupID: groupID,
	})
	return n > 0, err
}

func (r *Postgres) ClearMemberships(ctx context.Context, orgID, vehicleID int64) error {
	return r.q.ClearVehicleRentalGroups(ctx, db.ClearVehicleRentalGroupsParams{OrganizationID: orgID, VehicleID: vehicleID})
}

// GroupMembers returns the live member vehicles (by uuid) of each group.
func (r *Postgres) GroupMembers(ctx context.Context, orgID int64, groupIDs []int64) (map[int64]map[uuid.UUID]bool, error) {
	out := map[int64]map[uuid.UUID]bool{}
	if len(groupIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListRentalGroupMemberVehicles(ctx, db.ListRentalGroupMemberVehiclesParams{OrganizationID: orgID, GroupIds: groupIDs})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if out[row.RentalGroupID] == nil {
			out[row.RentalGroupID] = map[uuid.UUID]bool{}
		}
		out[row.RentalGroupID][row.VehicleUuid] = true
	}
	return out, nil
}

// LiveGroups lists every live rental group of the organization.
func (r *Postgres) LiveGroups(ctx context.Context, orgID int64) ([]GroupRow, error) {
	return r.q.ListLiveRentalGroupsForVehicles(ctx, orgID)
}

// GetGroup loads a live rental group of the organization.
func (r *Postgres) GetGroup(ctx context.Context, orgID int64, id uuid.UUID) (GroupRow, error) {
	row, err := r.q.GetLiveRentalGroupForVehicles(ctx, db.GetLiveRentalGroupForVehiclesParams{OrganizationID: orgID, Uuid: id})
	return GroupRow(row), mapErr(err)
}

// GroupsByIDs loads rental groups of the organization by id.
func (r *Postgres) GroupsByIDs(ctx context.Context, orgID int64, ids []int64) (map[int64]db.RentalGroup, error) {
	out := map[int64]db.RentalGroup{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.ListRentalGroupsByIDs(ctx, db.ListRentalGroupsByIDsParams{OrganizationID: orgID, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

// GroupCriteria returns the catalog model / type ids of each group.
func (r *Postgres) GroupCriteria(ctx context.Context, orgID int64, groupIDs []int64) (map[int64][]int64, map[int64][]int64, error) {
	models, types := map[int64][]int64{}, map[int64][]int64{}
	if len(groupIDs) == 0 {
		return models, types, nil
	}
	mrows, err := r.q.ListRentalGroupCatalogModelIDs(ctx, db.ListRentalGroupCatalogModelIDsParams{OrganizationID: orgID, GroupIds: groupIDs})
	if err != nil {
		return nil, nil, err
	}
	for _, row := range mrows {
		models[row.RentalGroupID] = append(models[row.RentalGroupID], row.CatalogModelID)
	}
	trows, err := r.q.ListRentalGroupCatalogTypeIDs(ctx, db.ListRentalGroupCatalogTypeIDsParams{OrganizationID: orgID, GroupIds: groupIDs})
	if err != nil {
		return nil, nil, err
	}
	for _, row := range trows {
		types[row.RentalGroupID] = append(types[row.RentalGroupID], row.CatalogTypeID)
	}
	return models, types, nil
}

// --- products -------------------------------------------------------------------

// ProductCurrentVersion loads a live product of the organization and its
// current version.
func (r *Postgres) ProductCurrentVersion(ctx context.Context, orgID int64, id uuid.UUID) (db.RentalProductVersion, error) {
	p, err := r.q.GetRentalProductByUUID(ctx, db.GetRentalProductByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return db.RentalProductVersion{}, mapErr(err)
	}
	rows, err := r.q.ListCurrentRentalProductVersions(ctx, db.ListCurrentRentalProductVersionsParams{
		OrganizationID: orgID, ProductIds: []int64{p.ID},
	})
	if err != nil {
		return db.RentalProductVersion{}, err
	}
	if len(rows) == 0 {
		return db.RentalProductVersion{}, ErrNotFound
	}
	return rows[0], nil
}

// --- branches -------------------------------------------------------------------

func (r *Postgres) GetBranch(ctx context.Context, orgID int64, id uuid.UUID) (db.Branch, error) {
	row, err := r.q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

// FindBranch loads a live branch of the organization by code or name.
func (r *Postgres) FindBranch(ctx context.Context, orgID int64, ref string) (db.Branch, error) {
	row, err := r.q.FindVehicleImportBranch(ctx, db.FindVehicleImportBranchParams{OrganizationID: orgID, Ref: ref})
	return row, mapErr(err)
}

// --- platform catalog (not organization scoped) ----------------------------------

func (r *Postgres) GetCatalogType(ctx context.Context, id uuid.UUID) (db.GetVehicleCatalogTypeRow, error) {
	row, err := r.q.GetVehicleCatalogType(ctx, id)
	return row, mapErr(err)
}

func (r *Postgres) CatalogTypesByCode(ctx context.Context, code string) ([]db.FindVehicleCatalogTypeByCodeRow, error) {
	return r.q.FindVehicleCatalogTypeByCode(ctx, code)
}

func (r *Postgres) CatalogTypesByNames(ctx context.Context, brand, mdl, typeName string) ([]db.FindVehicleCatalogTypesByNamesRow, error) {
	return r.q.FindVehicleCatalogTypesByNames(ctx, db.FindVehicleCatalogTypesByNamesParams{
		Brand: brand, Model: mdl, TypeName: textArg(typeName),
	})
}

// CanonicalModelIDs maps catalog model ids to their grouped model id.
func (r *Postgres) CanonicalModelIDs(ctx context.Context, ids []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.VehicleCanonicalModelIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.CanonicalID
	}
	return out, nil
}

func (r *Postgres) Valuations(ctx context.Context, typeID int64, year int16) ([]db.ListCatalogValuationsForVehicleRow, error) {
	return r.q.ListCatalogValuationsForVehicle(ctx, db.ListCatalogValuationsForVehicleParams{TypeID: typeID, ModelYear: int32(year)})
}

// --- activity -------------------------------------------------------------------

func (r *Postgres) ListActivity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]db.ListOrganizationResourceActivityRow, int64, error) {
	org := pgtype.Int8{Int64: orgID, Valid: true}
	ru := pgtype.UUID{Bytes: id, Valid: true}
	rows, err := r.q.ListOrganizationResourceActivity(ctx, db.ListOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountOrganizationResourceActivity(ctx, db.CountOrganizationResourceActivityParams{
		OrganizationID: org, Resource: model.Resource, ResourceUuid: ru,
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// --- helpers ----------------------------------------------------------------------

// GetSupplier resolves a supplier card of the organization.
func (r *Postgres) GetSupplier(ctx context.Context, orgID int64, id uuid.UUID) (db.GetSupplierForFinanceRow, error) {
	row, err := r.q.GetSupplierForFinance(ctx, db.GetSupplierForFinanceParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

// SupplierByTitle returns the one active supplier whose title matches a
// free-text name (case-insensitive); none or several → invalid.
func (r *Postgres) SupplierByTitle(ctx context.Context, orgID int64, title string) pgtype.Int8 {
	if title == "" {
		return pgtype.Int8{}
	}
	rows, err := r.q.FindSupplierIDsByTitle(ctx, db.FindSupplierIDsByTitleParams{OrganizationID: orgID, Title: title})
	if err != nil || len(rows) != 1 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: rows[0].ID, Valid: true}
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_vehicles_organization_plate" {
		return ErrPlateTaken
	}
	return err
}

func textArg(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// dateArg is the DATE of t (a zero t is sent as NULL).
func dateArg(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
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

// ListMovements returns the non-cancelled transfers of a vehicle (LocationAt).
func (r *Postgres) ListMovements(ctx context.Context, orgID, vehicleID int64) ([]db.ListVehicleMovementsRow, error) {
	return r.q.ListVehicleMovements(ctx, db.ListVehicleMovementsParams{OrganizationID: orgID, VehicleID: vehicleID})
}
