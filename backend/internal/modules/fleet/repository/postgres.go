// Package repository is the Postgres access path of the fleet resources.
// Every method takes the organization id explicitly and every query on a
// fleet table filters by it: a uuid from another organization reads as not
// found. Platform catalog lookups are not organization scoped (platform data).
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned for rows missing in the organization.
	ErrNotFound = errors.New("not found")
	// ErrCodeTaken is returned when a code is already used in the organization.
	ErrCodeTaken = errors.New("code taken")
	// ErrVersionConflict is returned when two writers create the same version.
	ErrVersionConflict = errors.New("version conflict")
)

// SegmentFields are the writable columns of a segment.
type SegmentFields struct {
	Code        string
	Name        string
	Description string
	SortOrder   int32
	IsActive    bool
}

// GroupFields are the writable columns of a rental group.
type GroupFields struct {
	Code            string
	Name            string
	Description     string
	SegmentID       int64
	Transmission    pgtype.Text
	Fuel            pgtype.Text
	BodyType        pgtype.Text
	MinSeats        pgtype.Int2
	MinLuggage      pgtype.Int2
	AirConditioning bool
	AcrissCode      pgtype.Text
	IsActive        bool
}

// ProductFields are the writable columns of a product.
type ProductFields struct {
	Code                  string
	Title                 string
	Description           string
	SegmentID             int64
	RepresentativeModelID pgtype.Int8
	RepresentativeTypeID  pgtype.Int8
	Status                string
	IsActive              bool
	CurrentVersion        int32
}

// VersionFields are the columns of a product version.
type VersionFields struct {
	GuaranteeType    string
	VehicleID        pgtype.UUID
	VariantLevel     pgtype.Text
	CatalogModelID   pgtype.Int8
	CatalogTypeID    pgtype.Int8
	RentalGroupID    pgtype.Int8
	GuaranteeLabel   string
	DepositAmount    pgtype.Numeric
	MinDriverAge     int16
	MinLicenseYears  int16
	IncludedKmPerDay pgtype.Int4
	ExtraKmFee       pgtype.Numeric
	FuelPolicy       string
	Notes            string
	ChangeNote       string
	CreatedBy        pgtype.Int8
}

// SegmentUsage counts the live records of a segment.
type SegmentUsage struct {
	Groups   int64
	Products int64
}

// ImageFields are the columns of a product image.
type ImageFields struct {
	UUID        uuid.UUID
	ObjectKey   string
	Filename    string
	ContentType string
	SizeBytes   int64
}

// Postgres implements the fleet repository with sqlc queries.
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
		return fmt.Errorf("fleet: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&Postgres{pool: nil, q: r.q.WithTx(tx)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- segments ---------------------------------------------------------------

func (r *Postgres) ListSegments(ctx context.Context, orgID int64, f model.SegmentFilter) ([]db.VehicleSegment, int64, error) {
	rows, err := r.q.ListVehicleSegments(ctx, db.ListVehicleSegmentsParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountVehicleSegments(ctx, db.CountVehicleSegmentsParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) SegmentsByIDs(ctx context.Context, orgID int64, ids []int64) (map[int64]db.VehicleSegment, error) {
	out := map[int64]db.VehicleSegment{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.ListVehicleSegmentsByIDs(ctx, db.ListVehicleSegmentsByIDsParams{OrganizationID: orgID, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func (r *Postgres) GetSegment(ctx context.Context, orgID int64, id uuid.UUID) (db.VehicleSegment, error) {
	row, err := r.q.GetVehicleSegmentByUUID(ctx, db.GetVehicleSegmentByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) SegmentCodeTaken(ctx context.Context, orgID int64, code string, exclude *uuid.UUID) (bool, error) {
	return r.q.VehicleSegmentCodeTaken(ctx, db.VehicleSegmentCodeTakenParams{
		OrganizationID: orgID, Code: code, ExcludeUuid: uuidArg(exclude),
	})
}

func (r *Postgres) CreateSegment(ctx context.Context, orgID int64, f SegmentFields) (db.VehicleSegment, error) {
	row, err := r.q.CreateVehicleSegment(ctx, db.CreateVehicleSegmentParams{
		OrganizationID: orgID, Code: f.Code, Name: f.Name, Description: f.Description,
		SortOrder: f.SortOrder, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateSegment(ctx context.Context, orgID int64, id uuid.UUID, f SegmentFields) (db.VehicleSegment, error) {
	row, err := r.q.UpdateVehicleSegment(ctx, db.UpdateVehicleSegmentParams{
		OrganizationID: orgID, Uuid: id, Code: f.Code, Name: f.Name, Description: f.Description,
		SortOrder: f.SortOrder, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) DeleteSegment(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteVehicleSegment(ctx, db.SoftDeleteVehicleSegmentParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) SegmentUsage(ctx context.Context, orgID int64, ids []int64) (map[int64]SegmentUsage, error) {
	out := map[int64]SegmentUsage{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.CountVehicleSegmentUsage(ctx, db.CountVehicleSegmentUsageParams{OrganizationID: orgID, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.SegmentID] = SegmentUsage{Groups: row.GroupsCount, Products: row.ProductsCount}
	}
	return out, nil
}

func (r *Postgres) SegmentFacets(ctx context.Context, orgID int64) (model.SegmentFacets, error) {
	rows, err := r.q.ListVehicleSegmentActiveFacets(ctx, orgID)
	if err != nil {
		return model.SegmentFacets{}, err
	}
	out := model.SegmentFacets{IsActive: []model.Facet{}}
	for _, row := range rows {
		value := "false"
		if row.IsActive {
			value = "true"
		}
		out.IsActive = append(out.IsActive, model.Facet{Value: value, Count: row.Count})
	}
	return out, nil
}

// --- rental groups ----------------------------------------------------------

func (r *Postgres) ListGroups(ctx context.Context, orgID int64, f model.RentalGroupFilter) ([]db.RentalGroup, int64, error) {
	rows, err := r.q.ListRentalGroups(ctx, db.ListRentalGroupsParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), SegmentUuids: uuidsArg(f.SegmentUUIDs),
		Transmissions: stringsArg(f.Transmissions), Fuels: stringsArg(f.Fuels), BodyTypes: stringsArg(f.BodyTypes),
		Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountRentalGroups(ctx, db.CountRentalGroupsParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), SegmentUuids: uuidsArg(f.SegmentUUIDs),
		Transmissions: stringsArg(f.Transmissions), Fuels: stringsArg(f.Fuels), BodyTypes: stringsArg(f.BodyTypes),
		Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

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

func (r *Postgres) GetGroup(ctx context.Context, orgID int64, id uuid.UUID) (db.RentalGroup, error) {
	row, err := r.q.GetRentalGroupByUUID(ctx, db.GetRentalGroupByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) GroupCodeTaken(ctx context.Context, orgID int64, code string, exclude *uuid.UUID) (bool, error) {
	return r.q.RentalGroupCodeTaken(ctx, db.RentalGroupCodeTakenParams{
		OrganizationID: orgID, Code: code, ExcludeUuid: uuidArg(exclude),
	})
}

func (r *Postgres) CreateGroup(ctx context.Context, orgID int64, f GroupFields) (db.RentalGroup, error) {
	row, err := r.q.CreateRentalGroup(ctx, db.CreateRentalGroupParams{
		OrganizationID: orgID, Code: f.Code, Name: f.Name, Description: f.Description, SegmentID: f.SegmentID,
		Transmission: f.Transmission, Fuel: f.Fuel, BodyType: f.BodyType, MinSeats: f.MinSeats,
		MinLuggage: f.MinLuggage, AirConditioning: f.AirConditioning, AcrissCode: f.AcrissCode, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateGroup(ctx context.Context, orgID int64, id uuid.UUID, f GroupFields) (db.RentalGroup, error) {
	row, err := r.q.UpdateRentalGroup(ctx, db.UpdateRentalGroupParams{
		OrganizationID: orgID, Uuid: id, Code: f.Code, Name: f.Name, Description: f.Description, SegmentID: f.SegmentID,
		Transmission: f.Transmission, Fuel: f.Fuel, BodyType: f.BodyType, MinSeats: f.MinSeats,
		MinLuggage: f.MinLuggage, AirConditioning: f.AirConditioning, AcrissCode: f.AcrissCode, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

// ReplaceGroupCriteria replaces the catalog criteria of a group. nil lists
// are left unchanged.
func (r *Postgres) ReplaceGroupCriteria(ctx context.Context, orgID, groupID int64, modelIDs, typeIDs []int64, models, types bool) error {
	if models {
		if err := r.q.DeleteRentalGroupCatalogModels(ctx, db.DeleteRentalGroupCatalogModelsParams{
			OrganizationID: orgID, RentalGroupID: groupID,
		}); err != nil {
			return err
		}
		if len(modelIDs) > 0 {
			if err := r.q.InsertRentalGroupCatalogModels(ctx, db.InsertRentalGroupCatalogModelsParams{
				OrganizationID: orgID, RentalGroupID: groupID, CatalogModelIds: modelIDs,
			}); err != nil {
				return err
			}
		}
	}
	if types {
		if err := r.q.DeleteRentalGroupCatalogTypes(ctx, db.DeleteRentalGroupCatalogTypesParams{
			OrganizationID: orgID, RentalGroupID: groupID,
		}); err != nil {
			return err
		}
		if len(typeIDs) > 0 {
			if err := r.q.InsertRentalGroupCatalogTypes(ctx, db.InsertRentalGroupCatalogTypesParams{
				OrganizationID: orgID, RentalGroupID: groupID, CatalogTypeIds: typeIDs,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Postgres) SetGroupActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (db.RentalGroup, error) {
	row, err := r.q.SetRentalGroupActive(ctx, db.SetRentalGroupActiveParams{OrganizationID: orgID, Uuid: id, IsActive: active})
	return row, mapErr(err)
}

func (r *Postgres) DeleteGroup(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteRentalGroup(ctx, db.SoftDeleteRentalGroupParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
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

// GroupProductCounts counts the live products whose current version
// guarantees each group.
func (r *Postgres) GroupProductCounts(ctx context.Context, orgID int64, groupIDs []int64) (map[int64]int64, error) {
	out := map[int64]int64{}
	if len(groupIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.CountRentalGroupProducts(ctx, db.CountRentalGroupProductsParams{OrganizationID: orgID, Ids: groupIDs})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.RentalGroupID] = row.ProductsCount
	}
	return out, nil
}

func (r *Postgres) GroupFacets(ctx context.Context, orgID int64) (model.RentalGroupFacets, error) {
	out := model.RentalGroupFacets{
		Segment: []model.Facet{}, Transmission: []model.Facet{}, Fuel: []model.Facet{}, BodyType: []model.Facet{},
	}
	segs, err := r.q.ListRentalGroupSegmentFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range segs {
		out.Segment = append(out.Segment, model.Facet{Value: row.Uuid.String(), Label: row.Name, Count: row.Count})
	}
	attrs, err := r.q.ListRentalGroupAttributeFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range attrs {
		facet := model.Facet{Value: row.Value, Count: row.Count}
		switch row.Attribute {
		case "transmission":
			out.Transmission = append(out.Transmission, facet)
		case "fuel":
			out.Fuel = append(out.Fuel, facet)
		case "body_type":
			out.BodyType = append(out.BodyType, facet)
		}
	}
	return out, nil
}

// --- platform catalog references ---------------------------------------------

func (r *Postgres) CatalogModelsByUUIDs(ctx context.Context, ids []uuid.UUID) ([]db.FleetCatalogModelRefsByIDsRow, error) {
	if len(ids) == 0 {
		return []db.FleetCatalogModelRefsByIDsRow{}, nil
	}
	rows, err := r.q.FleetCatalogModelRefsByUUIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]db.FleetCatalogModelRefsByIDsRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, db.FleetCatalogModelRefsByIDsRow(row))
	}
	return out, nil
}

func (r *Postgres) CatalogModelsByIDs(ctx context.Context, ids []int64) (map[int64]db.FleetCatalogModelRefsByIDsRow, error) {
	out := map[int64]db.FleetCatalogModelRefsByIDsRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.FleetCatalogModelRefsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

func (r *Postgres) CatalogTypesByUUIDs(ctx context.Context, ids []uuid.UUID) ([]db.FleetCatalogTypeRefsByIDsRow, error) {
	if len(ids) == 0 {
		return []db.FleetCatalogTypeRefsByIDsRow{}, nil
	}
	rows, err := r.q.FleetCatalogTypeRefsByUUIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]db.FleetCatalogTypeRefsByIDsRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, db.FleetCatalogTypeRefsByIDsRow(row))
	}
	return out, nil
}

func (r *Postgres) CatalogTypesByIDs(ctx context.Context, ids []int64) (map[int64]db.FleetCatalogTypeRefsByIDsRow, error) {
	out := map[int64]db.FleetCatalogTypeRefsByIDsRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.FleetCatalogTypeRefsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

// --- rental products --------------------------------------------------------

func (r *Postgres) ListProducts(ctx context.Context, orgID int64, f model.RentalProductFilter) ([]db.RentalProduct, int64, error) {
	var group pgtype.UUID
	if f.RentalGroupUUID != nil {
		group = pgtype.UUID{Bytes: *f.RentalGroupUUID, Valid: true}
	}
	rows, err := r.q.ListRentalProducts(ctx, db.ListRentalProductsParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Statuses: stringsArg(f.Statuses),
		SegmentUuids: uuidsArg(f.SegmentUUIDs), GuaranteeTypes: stringsArg(f.GuaranteeTypes), RentalGroupUuid: group,
		Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountRentalProducts(ctx, db.CountRentalProductsParams{
		OrganizationID: orgID, IsActive: boolArg(f.IsActive), Statuses: stringsArg(f.Statuses),
		SegmentUuids: uuidsArg(f.SegmentUUIDs), GuaranteeTypes: stringsArg(f.GuaranteeTypes), RentalGroupUuid: group,
		Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) GetProduct(ctx context.Context, orgID int64, id uuid.UUID) (db.RentalProduct, error) {
	row, err := r.q.GetRentalProductByUUID(ctx, db.GetRentalProductByUUIDParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

// LockProduct reads a product with FOR UPDATE (inside InTx).
func (r *Postgres) LockProduct(ctx context.Context, orgID int64, id uuid.UUID) (db.RentalProduct, error) {
	row, err := r.q.GetRentalProductByUUIDForUpdate(ctx, db.GetRentalProductByUUIDForUpdateParams{OrganizationID: orgID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) ProductCodeTaken(ctx context.Context, orgID int64, code string, exclude *uuid.UUID) (bool, error) {
	return r.q.RentalProductCodeTaken(ctx, db.RentalProductCodeTakenParams{
		OrganizationID: orgID, Code: code, ExcludeUuid: uuidArg(exclude),
	})
}

func (r *Postgres) CreateProduct(ctx context.Context, orgID int64, f ProductFields) (db.RentalProduct, error) {
	row, err := r.q.CreateRentalProduct(ctx, db.CreateRentalProductParams{
		OrganizationID: orgID, Code: f.Code, Title: f.Title, Description: f.Description, SegmentID: f.SegmentID,
		RepresentativeModelID: f.RepresentativeModelID, RepresentativeTypeID: f.RepresentativeTypeID,
		Status: f.Status, IsActive: f.IsActive,
	})
	return row, mapErr(err)
}

func (r *Postgres) UpdateProduct(ctx context.Context, orgID int64, id uuid.UUID, f ProductFields) (db.RentalProduct, error) {
	row, err := r.q.UpdateRentalProduct(ctx, db.UpdateRentalProductParams{
		OrganizationID: orgID, Uuid: id, Code: f.Code, Title: f.Title, Description: f.Description, SegmentID: f.SegmentID,
		RepresentativeModelID: f.RepresentativeModelID, RepresentativeTypeID: f.RepresentativeTypeID,
		Status: f.Status, IsActive: f.IsActive, CurrentVersion: f.CurrentVersion,
	})
	return row, mapErr(err)
}

func (r *Postgres) SetProductActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (db.RentalProduct, error) {
	row, err := r.q.SetRentalProductActive(ctx, db.SetRentalProductActiveParams{OrganizationID: orgID, Uuid: id, IsActive: active})
	return row, mapErr(err)
}

func (r *Postgres) TouchProduct(ctx context.Context, orgID, productID int64) error {
	return r.q.TouchRentalProduct(ctx, db.TouchRentalProductParams{OrganizationID: orgID, ID: productID})
}

func (r *Postgres) DeleteProduct(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := r.q.SoftDeleteRentalProduct(ctx, db.SoftDeleteRentalProductParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) ProductFacets(ctx context.Context, orgID int64) (model.RentalProductFacets, error) {
	out := model.RentalProductFacets{Segment: []model.Facet{}, Status: []model.Facet{}, GuaranteeType: []model.Facet{}}
	segs, err := r.q.ListRentalProductSegmentFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range segs {
		out.Segment = append(out.Segment, model.Facet{Value: row.Uuid.String(), Label: row.Name, Count: row.Count})
	}
	statuses, err := r.q.ListRentalProductStatusFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range statuses {
		out.Status = append(out.Status, model.Facet{Value: row.Status, Count: row.Count})
	}
	guarantees, err := r.q.ListRentalProductGuaranteeFacets(ctx, orgID)
	if err != nil {
		return out, err
	}
	for _, row := range guarantees {
		out.GuaranteeType = append(out.GuaranteeType, model.Facet{Value: row.GuaranteeType, Count: row.Count})
	}
	return out, nil
}

// --- product versions -------------------------------------------------------

func (r *Postgres) CreateVersion(ctx context.Context, orgID, productID int64, version int32, f VersionFields) (db.RentalProductVersion, error) {
	row, err := r.q.CreateRentalProductVersion(ctx, db.CreateRentalProductVersionParams{
		OrganizationID: orgID, ProductID: productID, Version: version, GuaranteeType: f.GuaranteeType,
		VehicleID: f.VehicleID, VariantLevel: f.VariantLevel, CatalogModelID: f.CatalogModelID,
		CatalogTypeID: f.CatalogTypeID, RentalGroupID: f.RentalGroupID, GuaranteeLabel: f.GuaranteeLabel,
		DepositAmount: f.DepositAmount, MinDriverAge: f.MinDriverAge, MinLicenseYears: f.MinLicenseYears,
		IncludedKmPerDay: f.IncludedKmPerDay, ExtraKmFee: f.ExtraKmFee, FuelPolicy: f.FuelPolicy,
		Notes: f.Notes, ChangeNote: f.ChangeNote, CreatedBy: f.CreatedBy,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_rental_product_versions_product_version" {
			return row, ErrVersionConflict
		}
		return row, err
	}
	return row, nil
}

// CurrentVersions returns the current version of each product.
func (r *Postgres) CurrentVersions(ctx context.Context, orgID int64, productIDs []int64) (map[int64]db.RentalProductVersion, error) {
	out := map[int64]db.RentalProductVersion{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListCurrentRentalProductVersions(ctx, db.ListCurrentRentalProductVersionsParams{
		OrganizationID: orgID, ProductIds: productIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ProductID] = row
	}
	return out, nil
}

func (r *Postgres) ListVersions(ctx context.Context, orgID, productID int64, limit, offset int32) ([]db.RentalProductVersion, int64, error) {
	rows, err := r.q.ListRentalProductVersions(ctx, db.ListRentalProductVersionsParams{
		OrganizationID: orgID, ProductID: productID, LimitCount: limit, OffsetCount: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := r.q.CountRentalProductVersions(ctx, db.CountRentalProductVersionsParams{OrganizationID: orgID, ProductID: productID})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Postgres) GetVersion(ctx context.Context, orgID, productID int64, id uuid.UUID) (db.RentalProductVersion, error) {
	row, err := r.q.GetRentalProductVersionByUUID(ctx, db.GetRentalProductVersionByUUIDParams{
		OrganizationID: orgID, ProductID: productID, Uuid: id,
	})
	return row, mapErr(err)
}

// UsersByIDs returns the version authors.
func (r *Postgres) UsersByIDs(ctx context.Context, ids []int64) (map[int64]model.Actor, error) {
	out := map[int64]model.Actor{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.ListUsersByIDsForFleet(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = model.Actor{UUID: row.Uuid, Name: row.Name, Surname: row.Surname, Email: row.Email}
	}
	return out, nil
}

// --- product images ---------------------------------------------------------

func (r *Postgres) CreateImage(ctx context.Context, orgID, productID int64, f ImageFields) (db.RentalProductImage, error) {
	return r.q.CreateRentalProductImage(ctx, db.CreateRentalProductImageParams{
		Uuid: f.UUID, OrganizationID: orgID, ProductID: productID, ObjectKey: f.ObjectKey,
		Filename: f.Filename, ContentType: f.ContentType, SizeBytes: f.SizeBytes,
	})
}

// ImagesByProducts returns the ordered images of each product.
func (r *Postgres) ImagesByProducts(ctx context.Context, orgID int64, productIDs []int64) (map[int64][]db.RentalProductImage, error) {
	out := map[int64][]db.RentalProductImage{}
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.ListRentalProductImages(ctx, db.ListRentalProductImagesParams{OrganizationID: orgID, ProductIds: productIDs})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], row)
	}
	return out, nil
}

func (r *Postgres) CountImages(ctx context.Context, orgID, productID int64) (int64, error) {
	return r.q.CountRentalProductImages(ctx, db.CountRentalProductImagesParams{OrganizationID: orgID, ProductID: productID})
}

func (r *Postgres) GetImage(ctx context.Context, orgID, productID int64, id uuid.UUID) (db.RentalProductImage, error) {
	row, err := r.q.GetRentalProductImage(ctx, db.GetRentalProductImageParams{OrganizationID: orgID, ProductID: productID, Uuid: id})
	return row, mapErr(err)
}

func (r *Postgres) DeleteImage(ctx context.Context, orgID, productID int64, id uuid.UUID) error {
	n, err := r.q.DeleteRentalProductImage(ctx, db.DeleteRentalProductImageParams{OrganizationID: orgID, ProductID: productID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Postgres) SetImageSort(ctx context.Context, orgID, productID int64, id uuid.UUID, sort int32) error {
	n, err := r.q.SetRentalProductImageSort(ctx, db.SetRentalProductImageSortParams{
		OrganizationID: orgID, ProductID: productID, Uuid: id, SortOrder: sort,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// PublicImage resolves a public image stream (organization uuid + image uuid).
func (r *Postgres) PublicImage(ctx context.Context, orgUUID, imageUUID uuid.UUID) (db.GetPublicRentalProductImageRow, error) {
	row, err := r.q.GetPublicRentalProductImage(ctx, db.GetPublicRentalProductImageParams{OrganizationUuid: orgUUID, Uuid: imageUUID})
	return row, mapErr(err)
}

// OrganizationUUID returns the public uuid of an organization (image URLs).
func (r *Postgres) OrganizationUUID(ctx context.Context, orgID int64) (uuid.UUID, error) {
	row, err := r.q.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return uuid.Nil, mapErr(err)
	}
	return row.Uuid, nil
}

// --- activity -----------------------------------------------------------------

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

// --- helpers ------------------------------------------------------------------

var codeConstraints = map[string]bool{
	"uq_vehicle_segments_organization_code": true,
	"uq_rental_groups_organization_code":    true,
	"uq_rental_products_organization_code":  true,
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && codeConstraints[pgErr.ConstraintName] {
		return ErrCodeTaken
	}
	return err
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

func uuidArg(v *uuid.UUID) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *v, Valid: true}
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

// VehiclePlate returns the plate of a live vehicle of the organization.
func (r *Postgres) VehiclePlate(ctx context.Context, orgID int64, id uuid.UUID) (string, error) {
	return r.q.FleetVehicleRef(ctx, db.FleetVehicleRefParams{OrganizationID: orgID, Uuid: id})
}
