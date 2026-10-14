package usecase

import (
	"context"
	"errors"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/eligibility"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxGroupCriteria caps the catalog models / types of one group.
const MaxGroupCriteria = 200

// ListGroups returns one page of the organization's rental groups.
func (s *Service) ListGroups(ctx context.Context, orgID int64, f model.RentalGroupFilter) ([]model.RentalGroup, int64, error) {
	if _, ok := GroupSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	rows, total, err := s.repo.ListGroups(ctx, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	out, err := s.hydrateGroups(ctx, orgID, rows)
	return out, total, err
}

// GroupFacets returns the dynamic filter options of the groups list.
func (s *Service) GroupFacets(ctx context.Context, orgID int64) (model.RentalGroupFacets, error) {
	return s.repo.GroupFacets(ctx, orgID)
}

// GetGroup returns one rental group of the organization.
func (s *Service) GetGroup(ctx context.Context, orgID int64, id uuid.UUID) (model.RentalGroup, error) {
	row, err := s.repo.GetGroup(ctx, orgID, id)
	if err != nil {
		return model.RentalGroup{}, mapNotFound(err, ErrGroupNotFound)
	}
	out, err := s.hydrateGroups(ctx, orgID, []db.RentalGroup{row})
	if err != nil {
		return model.RentalGroup{}, err
	}
	return out[0], nil
}

// groupDraft is a validated group write.
type groupDraft struct {
	fields    repository.GroupFields
	modelIDs  []int64
	typeIDs   []int64
	setModels bool
	setTypes  bool
}

// CreateGroup validates and stores a new rental group with its criteria.
func (s *Service) CreateGroup(ctx context.Context, orgID int64, in model.RentalGroupInput) (model.RentalGroup, error) {
	d := groupDraft{fields: repository.GroupFields{IsActive: true}, setModels: true, setTypes: true}
	if err := s.applyGroupInput(ctx, orgID, &d, in, true); err != nil {
		return model.RentalGroup{}, err
	}
	if taken, err := s.repo.GroupCodeTaken(ctx, orgID, d.fields.Code, nil); err != nil {
		return model.RentalGroup{}, err
	} else if taken {
		return model.RentalGroup{}, ErrCodeTaken
	}
	var created db.RentalGroup
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		row, err := tx.CreateGroup(ctx, orgID, d.fields)
		if err != nil {
			return mapCodeTaken(err)
		}
		created = row
		return tx.ReplaceGroupCriteria(ctx, orgID, row.ID, d.modelIDs, d.typeIDs, true, true)
	})
	if err != nil {
		return model.RentalGroup{}, err
	}
	return s.GetGroup(ctx, orgID, created.Uuid)
}

// PatchGroup updates the given fields of a group. Criteria lists that are
// sent replace the stored ones. Products guaranteeing the group follow its
// new attributes (the guarantee is "the group", not a copy of it).
func (s *Service) PatchGroup(ctx context.Context, orgID int64, id uuid.UUID, in model.RentalGroupInput) (model.RentalGroup, error) {
	current, err := s.repo.GetGroup(ctx, orgID, id)
	if err != nil {
		return model.RentalGroup{}, mapNotFound(err, ErrGroupNotFound)
	}
	d := groupDraft{fields: groupFields(current)}
	if err := s.applyGroupInput(ctx, orgID, &d, in, false); err != nil {
		return model.RentalGroup{}, err
	}
	if d.fields.Code != current.Code {
		if taken, err := s.repo.GroupCodeTaken(ctx, orgID, d.fields.Code, &id); err != nil {
			return model.RentalGroup{}, err
		} else if taken {
			return model.RentalGroup{}, ErrCodeTaken
		}
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		if _, err := tx.UpdateGroup(ctx, orgID, id, d.fields); err != nil {
			return mapNotFound(mapCodeTaken(err), ErrGroupNotFound)
		}
		return tx.ReplaceGroupCriteria(ctx, orgID, current.ID, d.modelIDs, d.typeIDs, d.setModels, d.setTypes)
	})
	if err != nil {
		return model.RentalGroup{}, err
	}
	return s.GetGroup(ctx, orgID, id)
}

// SetGroupActive switches is_active and returns the previous value (bulk).
func (s *Service) SetGroupActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	current, err := s.repo.GetGroup(ctx, orgID, id)
	if err != nil {
		return false, mapNotFound(err, ErrGroupNotFound)
	}
	if current.IsActive != active {
		if _, err := s.repo.SetGroupActive(ctx, orgID, id, active); err != nil {
			return current.IsActive, mapNotFound(err, ErrGroupNotFound)
		}
	}
	return current.IsActive, nil
}

// DeleteGroup soft-deletes a group no live product currently guarantees.
// Past product versions keep pointing at the (soft-deleted) row.
func (s *Service) DeleteGroup(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.repo.GetGroup(ctx, orgID, id)
	if err != nil {
		return mapNotFound(err, ErrGroupNotFound)
	}
	counts, err := s.repo.GroupProductCounts(ctx, orgID, []int64{row.ID})
	if err != nil {
		return err
	}
	if counts[row.ID] > 0 {
		return ErrGroupInUse
	}
	return mapNotFound(s.repo.DeleteGroup(ctx, orgID, id), ErrGroupNotFound)
}

// GroupTargetUUIDs resolves the groups matching a filter (bulk "all matching").
func (s *Service) GroupTargetUUIDs(ctx context.Context, orgID int64, f model.RentalGroupFilter) ([]uuid.UUID, error) {
	f.Limit, f.Offset, f.SortField = MaxListTargets, 0, "created_at"
	rows, _, err := s.repo.ListGroups(ctx, orgID, f)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Uuid)
	}
	return out, nil
}

// GroupExportRows returns every group matching a filter (export engine).
func (s *Service) GroupExportRows(ctx context.Context, orgID int64, f model.RentalGroupFilter) ([]model.RentalGroup, error) {
	f.Limit, f.Offset = MaxListTargets, 0
	f.SortField, f.SortDesc = "code", false
	rows, _, err := s.ListGroups(ctx, orgID, f)
	return rows, err
}

// GroupCriteria returns the eligibility criteria of a group, for the
// vehicles module and the capacity engine.
func (s *Service) GroupCriteria(ctx context.Context, orgID int64, id uuid.UUID) (eligibility.GroupCriteria, error) {
	row, err := s.repo.GetGroup(ctx, orgID, id)
	if err != nil {
		return eligibility.GroupCriteria{}, mapNotFound(err, ErrGroupNotFound)
	}
	models, types, err := s.repo.GroupCriteria(ctx, orgID, []int64{row.ID})
	if err != nil {
		return eligibility.GroupCriteria{}, err
	}
	return CriteriaFromGroup(row, models[row.ID], types[row.ID]), nil
}

// CriteriaFromGroup maps a stored group to eligibility criteria.
func CriteriaFromGroup(row db.RentalGroup, modelIDs, typeIDs []int64) eligibility.GroupCriteria {
	return eligibility.GroupCriteria{
		Transmission: row.Transmission.String, Fuel: row.Fuel.String, BodyType: row.BodyType.String,
		MinSeats: int(row.MinSeats.Int16), MinLuggage: int(row.MinLuggage.Int16),
		AirConditioning: row.AirConditioning, CatalogModelIDs: modelIDs, CatalogTypeIDs: typeIDs,
	}
}

func (s *Service) hydrateGroups(ctx context.Context, orgID int64, rows []db.RentalGroup) ([]model.RentalGroup, error) {
	ids := make([]int64, 0, len(rows))
	segIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		segIDs = append(segIDs, row.SegmentID)
	}
	segments, err := s.repo.SegmentsByIDs(ctx, orgID, uniqueIDs(segIDs))
	if err != nil {
		return nil, err
	}
	modelIDs, typeIDs, err := s.repo.GroupCriteria(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	allModels, allTypes := []int64{}, []int64{}
	for _, v := range modelIDs {
		allModels = append(allModels, v...)
	}
	for _, v := range typeIDs {
		allTypes = append(allTypes, v...)
	}
	models, err := s.repo.CatalogModelsByIDs(ctx, uniqueIDs(allModels))
	if err != nil {
		return nil, err
	}
	types, err := s.repo.CatalogTypesByIDs(ctx, uniqueIDs(allTypes))
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.GroupProductCounts(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]model.RentalGroup, 0, len(rows))
	for _, row := range rows {
		g := model.RentalGroup{
			UUID: row.Uuid, Code: row.Code, Name: row.Name, Description: row.Description,
			Segment:      segmentRef(segments[row.SegmentID]),
			Transmission: textPtr(row.Transmission), Fuel: textPtr(row.Fuel), BodyType: textPtr(row.BodyType),
			MinSeats: int2Ptr(row.MinSeats), MinLuggage: int2Ptr(row.MinLuggage),
			AirConditioning: row.AirConditioning, AcrissCode: textPtr(row.AcrissCode),
			CatalogModels: []model.CatalogModelRef{}, CatalogTypes: []model.CatalogTypeRef{},
			IsActive: row.IsActive, ProductsCount: counts[row.ID],
			CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		}
		for _, id := range modelIDs[row.ID] {
			if ref, ok := models[id]; ok {
				g.CatalogModels = append(g.CatalogModels, modelRef(ref))
			}
		}
		for _, id := range typeIDs[row.ID] {
			if ref, ok := types[id]; ok {
				g.CatalogTypes = append(g.CatalogTypes, typeRef(ref))
			}
		}
		out = append(out, g)
	}
	return out, nil
}

func groupFields(row db.RentalGroup) repository.GroupFields {
	return repository.GroupFields{
		Code: row.Code, Name: row.Name, Description: row.Description, SegmentID: row.SegmentID,
		Transmission: row.Transmission, Fuel: row.Fuel, BodyType: row.BodyType,
		MinSeats: row.MinSeats, MinLuggage: row.MinLuggage, AirConditioning: row.AirConditioning,
		AcrissCode: row.AcrissCode, IsActive: row.IsActive,
	}
}

func (s *Service) applyGroupInput(ctx context.Context, orgID int64, d *groupDraft, in model.RentalGroupInput, create bool) error {
	ve := &ValidationError{}
	f := &d.fields
	applyCode(ve, in.Code, create, &f.Code)
	applyName(ve, "name", in.Name, create, &f.Name)
	applyText(ve, "description", in.Description, 2000, &f.Description)
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
	applyEnum(ve, "transmission", in.Transmission, model.Transmissions, &f.Transmission)
	applyEnum(ve, "fuel", in.Fuel, model.Fuels, &f.Fuel)
	applyEnum(ve, "body_type", in.BodyType, model.BodyTypes, &f.BodyType)
	applyInt2(ve, "min_seats", in.MinSeats, 1, 60, &f.MinSeats)
	applyInt2(ve, "min_luggage", in.MinLuggage, 0, 30, &f.MinLuggage)
	if in.AirConditioning != nil {
		f.AirConditioning = *in.AirConditioning
	}
	if in.AcrissCode.Set {
		if in.AcrissCode.Value == nil || model.NormalizeACRISS(*in.AcrissCode.Value) == "" {
			f.AcrissCode = pgtype.Text{}
		} else {
			code := model.NormalizeACRISS(*in.AcrissCode.Value)
			if err := model.ValidateACRISS(code); err != nil {
				ve.add("acriss_code", "invalid", err.Error())
			} else {
				f.AcrissCode = pgtype.Text{String: code, Valid: true}
			}
		}
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	if in.CatalogModelUUIDs != nil {
		d.setModels = true
		ids := uniqueUUIDs(*in.CatalogModelUUIDs)
		if len(ids) > MaxGroupCriteria {
			ve.add("catalog_model_uuids", "too_many", "at most 200 catalog models are allowed")
		} else {
			rows, err := s.repo.CatalogModelsByUUIDs(ctx, ids)
			if err != nil {
				return err
			}
			found := map[uuid.UUID]int64{}
			for _, row := range rows {
				found[row.Uuid] = row.ID
			}
			d.modelIDs = d.modelIDs[:0]
			for i, id := range ids {
				if dbID, ok := found[id]; ok {
					d.modelIDs = append(d.modelIDs, dbID)
				} else {
					ve.add("catalog_model_uuids."+strconv.Itoa(i), "not_found", "catalog model was not found")
				}
			}
		}
	}
	if in.CatalogTypeUUIDs != nil {
		d.setTypes = true
		ids := uniqueUUIDs(*in.CatalogTypeUUIDs)
		if len(ids) > MaxGroupCriteria {
			ve.add("catalog_type_uuids", "too_many", "at most 200 catalog types are allowed")
		} else {
			rows, err := s.repo.CatalogTypesByUUIDs(ctx, ids)
			if err != nil {
				return err
			}
			found := map[uuid.UUID]int64{}
			for _, row := range rows {
				found[row.Uuid] = row.ID
			}
			d.typeIDs = d.typeIDs[:0]
			for i, id := range ids {
				if dbID, ok := found[id]; ok {
					d.typeIDs = append(d.typeIDs, dbID)
				} else {
					ve.add("catalog_type_uuids."+strconv.Itoa(i), "not_found", "catalog type was not found")
				}
			}
		}
	}
	return ve.orNil()
}

func applyInt2(ve *ValidationError, field string, in model.OptionalInt, lo, hi int64, dst *pgtype.Int2) {
	if !in.Set {
		return
	}
	if in.Value == nil {
		*dst = pgtype.Int2{}
		return
	}
	if *in.Value < lo || *in.Value > hi {
		ve.add(field, "out_of_range", field+" must be between "+strconv.FormatInt(lo, 10)+" and "+strconv.FormatInt(hi, 10))
		return
	}
	*dst = pgtype.Int2{Int16: int16(*in.Value), Valid: true}
}
