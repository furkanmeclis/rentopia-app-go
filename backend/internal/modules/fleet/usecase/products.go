package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListProducts returns one page of the organization's rental products.
func (s *Service) ListProducts(ctx context.Context, orgID int64, f model.RentalProductFilter) ([]model.RentalProduct, int64, error) {
	if _, ok := ProductSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, total, err := s.repo.ListProducts(ctx, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	out, err := s.hydrateProducts(ctx, orgID, rows)
	return out, total, err
}

// ProductFacets returns the dynamic filter options of the products list.
func (s *Service) ProductFacets(ctx context.Context, orgID int64) (model.RentalProductFacets, error) {
	return s.repo.ProductFacets(ctx, orgID)
}

// GetProduct returns one product with its current version and images.
func (s *Service) GetProduct(ctx context.Context, orgID int64, id uuid.UUID) (model.RentalProduct, error) {
	row, err := s.repo.GetProduct(ctx, orgID, id)
	if err != nil {
		return model.RentalProduct{}, mapNotFound(err, ErrProductNotFound)
	}
	out, err := s.hydrateProducts(ctx, orgID, []db.RentalProduct{row})
	if err != nil {
		return model.RentalProduct{}, err
	}
	return out[0], nil
}

// productDraft is a validated product write.
type productDraft struct {
	fields  repository.ProductFields
	version repository.VersionFields
	// versionInput: the body changed the guarantee or the conditions.
	versionInput bool
}

// DefaultVersionFields are the rental conditions of a new product.
func DefaultVersionFields() repository.VersionFields {
	return repository.VersionFields{
		DepositAmount: money("0"), MinDriverAge: 21, MinLicenseYears: 1,
		ExtraKmFee: money("0"), FuelPolicy: "same_to_same",
	}
}

// CreateProduct validates and stores a new product with version 1.
func (s *Service) CreateProduct(ctx context.Context, orgID, actorID int64, in model.RentalProductInput) (model.RentalProduct, error) {
	d := productDraft{
		fields:  repository.ProductFields{Status: model.StatusDraft, IsActive: true, CurrentVersion: 1},
		version: DefaultVersionFields(),
	}
	if err := s.applyProductInput(ctx, orgID, &d, in, true); err != nil {
		return model.RentalProduct{}, err
	}
	if taken, err := s.repo.ProductCodeTaken(ctx, orgID, d.fields.Code, nil); err != nil {
		return model.RentalProduct{}, err
	} else if taken {
		return model.RentalProduct{}, ErrCodeTaken
	}
	d.version.CreatedBy = actorArg(actorID)
	var created db.RentalProduct
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		row, err := tx.CreateProduct(ctx, orgID, d.fields)
		if err != nil {
			return mapCodeTaken(err)
		}
		created = row
		_, err = tx.CreateVersion(ctx, orgID, row.ID, 1, d.version)
		return err
	})
	if err != nil {
		return model.RentalProduct{}, err
	}
	s.indexProduct(ctx, orgID, created.Uuid)
	return s.GetProduct(ctx, orgID, created.Uuid)
}

// PatchResult is a product update and whether it created a new version.
type PatchResult struct {
	Product        model.RentalProduct
	VersionCreated bool
}

// PatchProduct updates a product. A changed guarantee or changed conditions
// create version N+1 (the current one); earlier versions are never edited.
// Title, description, segment, representative, status and is_active edits
// do not create a version.
func (s *Service) PatchProduct(ctx context.Context, orgID, actorID int64, id uuid.UUID, in model.RentalProductInput) (PatchResult, error) {
	var created bool
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		current, err := tx.LockProduct(ctx, orgID, id)
		if err != nil {
			return mapNotFound(err, ErrProductNotFound)
		}
		versions, err := tx.CurrentVersions(ctx, orgID, []int64{current.ID})
		if err != nil {
			return err
		}
		currentVersion, ok := versions[current.ID]
		if !ok {
			return ErrVersionNotFound
		}
		d := productDraft{fields: productFields(current), version: VersionFieldsFromRow(currentVersion)}
		if err := s.applyProductInput(ctx, orgID, &d, in, false); err != nil {
			return err
		}
		if d.fields.Code != current.Code {
			if taken, err := tx.ProductCodeTaken(ctx, orgID, d.fields.Code, &id); err != nil {
				return err
			} else if taken {
				return ErrCodeTaken
			}
		}
		if d.versionInput && VersionChanged(VersionFieldsFromRow(currentVersion), d.version) {
			next := current.CurrentVersion + 1
			d.version.CreatedBy = actorArg(actorID)
			if _, err := tx.CreateVersion(ctx, orgID, current.ID, next, d.version); err != nil {
				if errors.Is(err, repository.ErrVersionConflict) {
					return ErrVersionConflict
				}
				return err
			}
			d.fields.CurrentVersion = next
			created = true
		}
		if _, err := tx.UpdateProduct(ctx, orgID, id, d.fields); err != nil {
			return mapNotFound(mapCodeTaken(err), ErrProductNotFound)
		}
		return nil
	})
	if err != nil {
		return PatchResult{}, err
	}
	s.indexProduct(ctx, orgID, id)
	p, err := s.GetProduct(ctx, orgID, id)
	return PatchResult{Product: p, VersionCreated: created}, err
}

// SetProductActive switches is_active and returns the previous value (bulk).
func (s *Service) SetProductActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	current, err := s.repo.GetProduct(ctx, orgID, id)
	if err != nil {
		return false, mapNotFound(err, ErrProductNotFound)
	}
	if current.IsActive != active {
		if _, err := s.repo.SetProductActive(ctx, orgID, id, active); err != nil {
			return current.IsActive, mapNotFound(err, ErrProductNotFound)
		}
		s.indexProduct(ctx, orgID, id)
	}
	return current.IsActive, nil
}

// DeleteProduct soft-deletes a product (versions and images are kept).
func (s *Service) DeleteProduct(ctx context.Context, orgID int64, id uuid.UUID) error {
	if err := s.repo.DeleteProduct(ctx, orgID, id); err != nil {
		return mapNotFound(err, ErrProductNotFound)
	}
	if s.indexer != nil {
		s.indexer.EnqueueDelete(ctx, SearchSpecProducts, searchengine.TenantDocumentID(orgID, id.String()))
	}
	return nil
}

// ProductTargetUUIDs resolves the products matching a filter (bulk).
func (s *Service) ProductTargetUUIDs(ctx context.Context, orgID int64, f model.RentalProductFilter) ([]uuid.UUID, error) {
	f.Limit, f.Offset, f.SortField = MaxListTargets, 0, "created_at"
	rows, _, err := s.repo.ListProducts(ctx, orgID, f)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Uuid)
	}
	return out, nil
}

// ProductExportRows returns every product matching a filter (export engine).
func (s *Service) ProductExportRows(ctx context.Context, orgID int64, f model.RentalProductFilter) ([]model.RentalProduct, error) {
	f.Limit, f.Offset = MaxListTargets, 0
	f.SortField, f.SortDesc = "code", false
	rows, _, err := s.ListProducts(ctx, orgID, f)
	return rows, err
}

// ListVersions lists the versions of a product, newest first.
func (s *Service) ListVersions(ctx context.Context, orgID int64, productUUID uuid.UUID, limit, offset int32) ([]model.ProductVersion, int64, error) {
	product, err := s.repo.GetProduct(ctx, orgID, productUUID)
	if err != nil {
		return nil, 0, mapNotFound(err, ErrProductNotFound)
	}
	rows, total, err := s.repo.ListVersions(ctx, orgID, product.ID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	refs, err := s.loadVersionRefs(ctx, orgID, rows, nil)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ProductVersion, 0, len(rows))
	for _, row := range rows {
		out = append(out, refs.mapVersion(row, product.CurrentVersion))
	}
	return out, total, nil
}

// GetVersion returns one version of a product (read-only).
func (s *Service) GetVersion(ctx context.Context, orgID int64, productUUID, versionUUID uuid.UUID) (model.ProductVersion, error) {
	product, err := s.repo.GetProduct(ctx, orgID, productUUID)
	if err != nil {
		return model.ProductVersion{}, mapNotFound(err, ErrProductNotFound)
	}
	row, err := s.repo.GetVersion(ctx, orgID, product.ID, versionUUID)
	if err != nil {
		return model.ProductVersion{}, mapNotFound(err, ErrVersionNotFound)
	}
	refs, err := s.loadVersionRefs(ctx, orgID, []db.RentalProductVersion{row}, nil)
	if err != nil {
		return model.ProductVersion{}, err
	}
	return refs.mapVersion(row, product.CurrentVersion), nil
}

func (s *Service) indexProduct(ctx context.Context, orgID int64, id uuid.UUID) {
	if s.indexer != nil {
		s.indexer.EnqueueUpsert(ctx, SearchSpecProducts, searchengine.TenantDocumentID(orgID, id.String()))
	}
}

// --- versioning rules -------------------------------------------------------------

// VersionFieldsFromRow copies a stored version.
func VersionFieldsFromRow(v db.RentalProductVersion) repository.VersionFields {
	return repository.VersionFields{
		GuaranteeType: v.GuaranteeType, VehicleID: v.VehicleID, VariantLevel: v.VariantLevel,
		CatalogModelID: v.CatalogModelID, CatalogTypeID: v.CatalogTypeID, RentalGroupID: v.RentalGroupID,
		GuaranteeLabel: v.GuaranteeLabel, DepositAmount: v.DepositAmount, MinDriverAge: v.MinDriverAge,
		MinLicenseYears: v.MinLicenseYears, IncludedKmPerDay: v.IncludedKmPerDay, ExtraKmFee: v.ExtraKmFee,
		FuelPolicy: v.FuelPolicy, Notes: v.Notes,
	}
}

// VersionChanged reports whether b changes the guarantee or the rental
// conditions of a. Snapshot labels, change notes and authors are not part
// of the promise and are ignored.
func VersionChanged(a, b repository.VersionFields) bool {
	return a.GuaranteeType != b.GuaranteeType ||
		a.VehicleID != b.VehicleID ||
		a.VariantLevel != b.VariantLevel ||
		a.CatalogModelID != b.CatalogModelID ||
		a.CatalogTypeID != b.CatalogTypeID ||
		a.RentalGroupID != b.RentalGroupID ||
		FormatMoney(a.DepositAmount) != FormatMoney(b.DepositAmount) ||
		a.MinDriverAge != b.MinDriverAge ||
		a.MinLicenseYears != b.MinLicenseYears ||
		a.IncludedKmPerDay != b.IncludedKmPerDay ||
		FormatMoney(a.ExtraKmFee) != FormatMoney(b.ExtraKmFee) ||
		a.FuelPolicy != b.FuelPolicy ||
		a.Notes != b.Notes
}

// --- input ------------------------------------------------------------------------

func productFields(row db.RentalProduct) repository.ProductFields {
	return repository.ProductFields{
		Code: row.Code, Title: row.Title, Description: row.Description, SegmentID: row.SegmentID,
		RepresentativeModelID: row.RepresentativeModelID, RepresentativeTypeID: row.RepresentativeTypeID,
		Status: row.Status, IsActive: row.IsActive, CurrentVersion: row.CurrentVersion,
	}
}

func actorArg(id int64) pgtype.Int8 {
	if id <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}

func (s *Service) applyProductInput(ctx context.Context, orgID int64, d *productDraft, in model.RentalProductInput, create bool) error {
	ve := &ValidationError{}
	f := &d.fields
	applyCode(ve, in.Code, create, &f.Code)
	applyName(ve, "title", in.Title, create, &f.Title)
	applyText(ve, "description", in.Description, 10000, &f.Description)
	if in.SegmentUUID != nil || create {
		if in.SegmentUUID == nil {
			ve.add("segment_uuid", "required", "segment_uuid is required")
		} else if seg, err := s.repo.GetSegment(ctx, orgID, *in.SegmentUUID); err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			ve.add("segment_uuid", "not_found", "segment was not found")
		} else {
			f.SegmentID = seg.ID
		}
	}
	if in.RepresentativeModelUUID.Set {
		f.RepresentativeModelID = pgtype.Int8{}
		if in.RepresentativeModelUUID.Value != nil {
			rows, err := s.repo.CatalogModelsByUUIDs(ctx, []uuid.UUID{*in.RepresentativeModelUUID.Value})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				ve.add("representative_model_uuid", "not_found", "catalog model was not found")
			} else {
				f.RepresentativeModelID = pgtype.Int8{Int64: rows[0].ID, Valid: true}
			}
		}
	}
	if in.RepresentativeTypeUUID.Set {
		f.RepresentativeTypeID = pgtype.Int8{}
		if in.RepresentativeTypeUUID.Value != nil {
			rows, err := s.repo.CatalogTypesByUUIDs(ctx, []uuid.UUID{*in.RepresentativeTypeUUID.Value})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				ve.add("representative_type_uuid", "not_found", "catalog type was not found")
			} else {
				f.RepresentativeTypeID = pgtype.Int8{Int64: rows[0].ID, Valid: true}
			}
		}
	}
	if in.Status != nil {
		v := strings.TrimSpace(*in.Status)
		if !model.Contains(model.ProductStatuses, v) {
			ve.add("status", "invalid", "status must be one of draft, published, archived")
		} else {
			f.Status = v
		}
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	switch {
	case in.Guarantee != nil:
		d.versionInput = true
		if err := s.applyGuarantee(ctx, orgID, ve, &d.version, *in.Guarantee); err != nil {
			return err
		}
	case create:
		ve.add("guarantee.type", "required", "guarantee is required")
	}
	if in.Conditions != nil {
		d.versionInput = true
		applyConditions(ve, &d.version, *in.Conditions)
	}
	if in.ChangeNote != nil {
		applyText(ve, "change_note", in.ChangeNote, 1000, &d.version.ChangeNote)
	} else {
		d.version.ChangeNote = ""
	}
	return ve.orNil()
}

// applyGuarantee replaces the guarantee of v. References not used by the
// guarantee type are cleared.
func (s *Service) applyGuarantee(ctx context.Context, orgID int64, ve *ValidationError, v *repository.VersionFields, in model.GuaranteeInput) error {
	gType := strings.ToUpper(strings.TrimSpace(in.Type))
	if !model.Contains(model.GuaranteeTypes, gType) {
		ve.add("guarantee.type", "invalid", "guarantee type must be one of EXACT_VEHICLE, EXACT_VARIANT, GROUP")
		return nil
	}
	v.GuaranteeType = gType
	v.VehicleID, v.VariantLevel = pgtype.UUID{}, pgtype.Text{}
	v.CatalogModelID, v.CatalogTypeID, v.RentalGroupID = pgtype.Int8{}, pgtype.Int8{}, pgtype.Int8{}
	v.GuaranteeLabel = ""
	switch gType {
	case model.GuaranteeExactVehicle:
		// The id has no foreign key (vehicles came later); it may stay empty
		// for a draft, otherwise it must be a live vehicle of this
		// organization and its plate is the label.
		if in.VehicleID != nil && *in.VehicleID != uuid.Nil {
			plate, err := s.repo.VehiclePlate(ctx, orgID, *in.VehicleID)
			if errors.Is(err, pgx.ErrNoRows) {
				ve.add("guarantee.vehicle_id", "not_found", "vehicle was not found")
				break
			}
			if err != nil {
				return err
			}
			v.VehicleID = pgtype.UUID{Bytes: *in.VehicleID, Valid: true}
			v.GuaranteeLabel = plate
		}
	case model.GuaranteeExactVariant:
		level := ""
		if in.VariantLevel != nil {
			level = strings.TrimSpace(*in.VariantLevel)
		}
		switch level {
		case model.VariantLevelModel:
			if in.CatalogModelUUID == nil {
				ve.add("guarantee.catalog_model_uuid", "required", "catalog model is required for a model-level guarantee")
				break
			}
			rows, err := s.repo.CatalogModelsByUUIDs(ctx, []uuid.UUID{*in.CatalogModelUUID})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				ve.add("guarantee.catalog_model_uuid", "not_found", "catalog model was not found")
				break
			}
			v.VariantLevel = pgtype.Text{String: level, Valid: true}
			v.CatalogModelID = pgtype.Int8{Int64: rows[0].ID, Valid: true}
			v.GuaranteeLabel = model.CatalogModelLabel(rows[0].BrandDisplayName, rows[0].Name)
		case model.VariantLevelType:
			if in.CatalogTypeUUID == nil {
				ve.add("guarantee.catalog_type_uuid", "required", "catalog type is required for a type-level guarantee")
				break
			}
			rows, err := s.repo.CatalogTypesByUUIDs(ctx, []uuid.UUID{*in.CatalogTypeUUID})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				ve.add("guarantee.catalog_type_uuid", "not_found", "catalog type was not found")
				break
			}
			v.VariantLevel = pgtype.Text{String: level, Valid: true}
			v.CatalogTypeID = pgtype.Int8{Int64: rows[0].ID, Valid: true}
			v.CatalogModelID = rows[0].ModelID
			v.GuaranteeLabel = model.CatalogTypeLabel(rows[0].BrandDisplayName, rows[0].RawName)
		default:
			ve.add("guarantee.variant_level", "invalid", "variant level must be model or type")
		}
	case model.GuaranteeGroup:
		if in.RentalGroupUUID == nil {
			ve.add("guarantee.rental_group_uuid", "required", "rental group is required for a group guarantee")
			return nil
		}
		group, err := s.repo.GetGroup(ctx, orgID, *in.RentalGroupUUID)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			ve.add("guarantee.rental_group_uuid", "not_found", "rental group was not found")
			return nil
		}
		v.RentalGroupID = pgtype.Int8{Int64: group.ID, Valid: true}
		v.GuaranteeLabel = group.Name
	}
	return nil
}

func applyConditions(ve *ValidationError, v *repository.VersionFields, in model.ConditionsInput) {
	parseMoney(ve, "conditions.deposit_amount", in.DepositAmount, 12, &v.DepositAmount)
	parseMoney(ve, "conditions.extra_km_fee", in.ExtraKmFee, 8, &v.ExtraKmFee)
	if in.MinDriverAge != nil {
		if *in.MinDriverAge < 18 || *in.MinDriverAge > 99 {
			ve.add("conditions.min_driver_age", "out_of_range", "minimum driver age must be between 18 and 99")
		} else {
			v.MinDriverAge = *in.MinDriverAge
		}
	}
	if in.MinLicenseYears != nil {
		if *in.MinLicenseYears < 0 || *in.MinLicenseYears > 60 {
			ve.add("conditions.min_license_years", "out_of_range", "minimum license years must be between 0 and 60")
		} else {
			v.MinLicenseYears = *in.MinLicenseYears
		}
	}
	if in.IncludedKmPerDay.Set {
		switch {
		case in.IncludedKmPerDay.Value == nil:
			v.IncludedKmPerDay = pgtype.Int4{}
		case *in.IncludedKmPerDay.Value < 0 || *in.IncludedKmPerDay.Value > 100000:
			ve.add("conditions.included_km_per_day", "out_of_range", "included km per day must be between 0 and 100000")
		default:
			v.IncludedKmPerDay = pgtype.Int4{Int32: int32(*in.IncludedKmPerDay.Value), Valid: true}
		}
	}
	if in.FuelPolicy != nil {
		p := strings.TrimSpace(*in.FuelPolicy)
		if !model.Contains(model.FuelPolicies, p) {
			ve.add("conditions.fuel_policy", "invalid", "fuel policy must be one of "+strings.Join(model.FuelPolicies, ", "))
		} else {
			v.FuelPolicy = p
		}
	}
	applyText(ve, "conditions.notes", in.Notes, 4000, &v.Notes)
}

// --- hydration --------------------------------------------------------------------

// versionRefs are the references of a set of versions.
type versionRefs struct {
	models map[int64]db.FleetCatalogModelRefsByIDsRow
	types  map[int64]db.FleetCatalogTypeRefsByIDsRow
	groups map[int64]db.RentalGroup
	users  map[int64]model.Actor
}

// loadVersionRefs loads the catalog / group / author references of versions
// plus extra catalog ids (product representatives).
func (s *Service) loadVersionRefs(ctx context.Context, orgID int64, rows []db.RentalProductVersion, extra *[2][]int64) (versionRefs, error) {
	modelIDs, typeIDs, groupIDs, userIDs := []int64{}, []int64{}, []int64{}, []int64{}
	if extra != nil {
		modelIDs = append(modelIDs, extra[0]...)
		typeIDs = append(typeIDs, extra[1]...)
	}
	for _, v := range rows {
		if v.CatalogModelID.Valid {
			modelIDs = append(modelIDs, v.CatalogModelID.Int64)
		}
		if v.CatalogTypeID.Valid {
			typeIDs = append(typeIDs, v.CatalogTypeID.Int64)
		}
		if v.RentalGroupID.Valid {
			groupIDs = append(groupIDs, v.RentalGroupID.Int64)
		}
		if v.CreatedBy.Valid {
			userIDs = append(userIDs, v.CreatedBy.Int64)
		}
	}
	var refs versionRefs
	var err error
	if refs.models, err = s.repo.CatalogModelsByIDs(ctx, uniqueIDs(modelIDs)); err != nil {
		return refs, err
	}
	if refs.types, err = s.repo.CatalogTypesByIDs(ctx, uniqueIDs(typeIDs)); err != nil {
		return refs, err
	}
	if refs.groups, err = s.repo.GroupsByIDs(ctx, orgID, uniqueIDs(groupIDs)); err != nil {
		return refs, err
	}
	if refs.users, err = s.repo.UsersByIDs(ctx, uniqueIDs(userIDs)); err != nil {
		return refs, err
	}
	return refs, nil
}

func (r versionRefs) mapVersion(v db.RentalProductVersion, current int32) model.ProductVersion {
	g := model.Guarantee{Type: v.GuaranteeType, VariantLevel: textPtr(v.VariantLevel), Label: v.GuaranteeLabel}
	if v.VehicleID.Valid {
		id := uuid.UUID(v.VehicleID.Bytes)
		g.VehicleID = &id
	}
	if v.CatalogModelID.Valid {
		if row, ok := r.models[v.CatalogModelID.Int64]; ok {
			ref := modelRef(row)
			g.CatalogModel = &ref
		}
	}
	if v.CatalogTypeID.Valid {
		if row, ok := r.types[v.CatalogTypeID.Int64]; ok {
			ref := typeRef(row)
			g.CatalogType = &ref
		}
	}
	if v.RentalGroupID.Valid {
		if row, ok := r.groups[v.RentalGroupID.Int64]; ok {
			g.RentalGroup = &model.GroupRef{UUID: row.Uuid, Code: row.Code, Name: row.Name}
		}
	}
	c := model.Conditions{
		DepositAmount: FormatMoney(v.DepositAmount), MinDriverAge: v.MinDriverAge,
		MinLicenseYears: v.MinLicenseYears, ExtraKmFee: FormatMoney(v.ExtraKmFee),
		FuelPolicy: v.FuelPolicy, Notes: v.Notes,
	}
	if v.IncludedKmPerDay.Valid {
		km := v.IncludedKmPerDay.Int32
		c.IncludedKmPerDay = &km
	}
	out := model.ProductVersion{
		UUID: v.Uuid, Version: v.Version, IsCurrent: v.Version == current,
		Guarantee: g, Conditions: c, ChangeNote: v.ChangeNote, CreatedAt: v.CreatedAt.Time.UTC(),
	}
	if v.CreatedBy.Valid {
		if actor, ok := r.users[v.CreatedBy.Int64]; ok {
			out.CreatedBy = &actor
		}
	}
	return out
}

func (s *Service) hydrateProducts(ctx context.Context, orgID int64, rows []db.RentalProduct) ([]model.RentalProduct, error) {
	ids, segIDs, repModels, repTypes := []int64{}, []int64{}, []int64{}, []int64{}
	for _, row := range rows {
		ids = append(ids, row.ID)
		segIDs = append(segIDs, row.SegmentID)
		if row.RepresentativeModelID.Valid {
			repModels = append(repModels, row.RepresentativeModelID.Int64)
		}
		if row.RepresentativeTypeID.Valid {
			repTypes = append(repTypes, row.RepresentativeTypeID.Int64)
		}
	}
	segments, err := s.repo.SegmentsByIDs(ctx, orgID, uniqueIDs(segIDs))
	if err != nil {
		return nil, err
	}
	versions, err := s.repo.CurrentVersions(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	versionRows := make([]db.RentalProductVersion, 0, len(versions))
	for _, v := range versions {
		versionRows = append(versionRows, v)
	}
	refs, err := s.loadVersionRefs(ctx, orgID, versionRows, &[2][]int64{repModels, repTypes})
	if err != nil {
		return nil, err
	}
	images, err := s.repo.ImagesByProducts(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	var orgUUID uuid.UUID
	if len(images) > 0 {
		if orgUUID, err = s.repo.OrganizationUUID(ctx, orgID); err != nil {
			return nil, err
		}
	}
	out := make([]model.RentalProduct, 0, len(rows))
	for _, row := range rows {
		p := model.RentalProduct{
			UUID: row.Uuid, Code: row.Code, Title: row.Title, Description: row.Description,
			Segment: segmentRef(segments[row.SegmentID]), Status: row.Status, IsActive: row.IsActive,
			CurrentVersion: row.CurrentVersion, Images: []model.ProductImage{},
			CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		}
		representative := ""
		if row.RepresentativeModelID.Valid {
			if ref, ok := refs.models[row.RepresentativeModelID.Int64]; ok {
				m := modelRef(ref)
				p.RepresentativeModel = &m
				representative = m.Label
			}
		}
		if row.RepresentativeTypeID.Valid {
			if ref, ok := refs.types[row.RepresentativeTypeID.Int64]; ok {
				t := typeRef(ref)
				p.RepresentativeType = &t
				representative = t.Label
			}
		}
		if v, ok := versions[row.ID]; ok {
			p.Version = refs.mapVersion(v, row.CurrentVersion)
		}
		p.DisplayText = model.DisplayText(p.Version.Guarantee.Type, representative, p.Version.Guarantee.Label, p.Title)
		p.GuaranteeBadge = model.GuaranteeBadge(p.Version.Guarantee.Type)
		for _, img := range images[row.ID] {
			p.Images = append(p.Images, mapImage(img, orgUUID))
		}
		out = append(out, p)
	}
	return out, nil
}
