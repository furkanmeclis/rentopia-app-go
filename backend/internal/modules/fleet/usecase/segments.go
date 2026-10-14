package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	"github.com/google/uuid"
)

// ListSegments returns one page of the organization's segments.
func (s *Service) ListSegments(ctx context.Context, orgID int64, f model.SegmentFilter) ([]model.Segment, int64, error) {
	if _, ok := SegmentSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "sort_order", false
	}
	rows, total, err := s.repo.ListSegments(ctx, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	out, err := s.hydrateSegments(ctx, orgID, rows)
	return out, total, err
}

// SegmentFacets returns the dynamic filter options of the segments list.
func (s *Service) SegmentFacets(ctx context.Context, orgID int64) (model.SegmentFacets, error) {
	return s.repo.SegmentFacets(ctx, orgID)
}

// GetSegment returns one segment of the organization.
func (s *Service) GetSegment(ctx context.Context, orgID int64, id uuid.UUID) (model.Segment, error) {
	row, err := s.repo.GetSegment(ctx, orgID, id)
	if err != nil {
		return model.Segment{}, mapNotFound(err, ErrSegmentNotFound)
	}
	out, err := s.hydrateSegments(ctx, orgID, []db.VehicleSegment{row})
	if err != nil {
		return model.Segment{}, err
	}
	return out[0], nil
}

// CreateSegment validates and stores a new segment.
func (s *Service) CreateSegment(ctx context.Context, orgID int64, in model.SegmentInput) (model.Segment, error) {
	fields := repository.SegmentFields{IsActive: true}
	if err := applySegmentInput(&fields, in, true); err != nil {
		return model.Segment{}, err
	}
	if taken, err := s.repo.SegmentCodeTaken(ctx, orgID, fields.Code, nil); err != nil {
		return model.Segment{}, err
	} else if taken {
		return model.Segment{}, ErrCodeTaken
	}
	row, err := s.repo.CreateSegment(ctx, orgID, fields)
	if err != nil {
		return model.Segment{}, mapCodeTaken(err)
	}
	return MapSegment(row, repository.SegmentUsage{}), nil
}

// PatchSegment updates the given fields of a segment.
func (s *Service) PatchSegment(ctx context.Context, orgID int64, id uuid.UUID, in model.SegmentInput) (model.Segment, error) {
	current, err := s.repo.GetSegment(ctx, orgID, id)
	if err != nil {
		return model.Segment{}, mapNotFound(err, ErrSegmentNotFound)
	}
	fields := repository.SegmentFields{
		Code: current.Code, Name: current.Name, Description: current.Description,
		SortOrder: current.SortOrder, IsActive: current.IsActive,
	}
	if err := applySegmentInput(&fields, in, false); err != nil {
		return model.Segment{}, err
	}
	if fields.Code != current.Code {
		if taken, err := s.repo.SegmentCodeTaken(ctx, orgID, fields.Code, &id); err != nil {
			return model.Segment{}, err
		} else if taken {
			return model.Segment{}, ErrCodeTaken
		}
	}
	if _, err := s.repo.UpdateSegment(ctx, orgID, id, fields); err != nil {
		return model.Segment{}, mapNotFound(mapCodeTaken(err), ErrSegmentNotFound)
	}
	s.reindexSegmentProducts(ctx, orgID, current.ID)
	return s.GetSegment(ctx, orgID, id)
}

// DeleteSegment soft-deletes a segment that no live group or product uses.
func (s *Service) DeleteSegment(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.repo.GetSegment(ctx, orgID, id)
	if err != nil {
		return mapNotFound(err, ErrSegmentNotFound)
	}
	usage, err := s.repo.SegmentUsage(ctx, orgID, []int64{row.ID})
	if err != nil {
		return err
	}
	if u := usage[row.ID]; u.Groups > 0 || u.Products > 0 {
		return ErrSegmentInUse
	}
	return mapNotFound(s.repo.DeleteSegment(ctx, orgID, id), ErrSegmentNotFound)
}

// SegmentExportRows returns every segment matching a filter (export engine).
func (s *Service) SegmentExportRows(ctx context.Context, orgID int64, f model.SegmentFilter) ([]model.Segment, error) {
	f.Limit, f.Offset = MaxListTargets, 0
	f.SortField, f.SortDesc = "sort_order", false
	rows, _, err := s.ListSegments(ctx, orgID, f)
	return rows, err
}

func (s *Service) hydrateSegments(ctx context.Context, orgID int64, rows []db.VehicleSegment) ([]model.Segment, error) {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	usage, err := s.repo.SegmentUsage(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]model.Segment, 0, len(rows))
	for _, row := range rows {
		out = append(out, MapSegment(row, usage[row.ID]))
	}
	return out, nil
}

// reindexSegmentProducts refreshes the search documents of the segment's
// products (their subtitle shows the segment name). Fail-soft, capped.
func (s *Service) reindexSegmentProducts(ctx context.Context, orgID int64, segmentID int64) {
	if s.indexer == nil {
		return
	}
	seg, err := s.repo.SegmentsByIDs(ctx, orgID, []int64{segmentID})
	if err != nil || len(seg) == 0 {
		return
	}
	rows, _, err := s.repo.ListProducts(ctx, orgID, model.RentalProductFilter{
		Limit: 1000, SegmentUUIDs: []uuid.UUID{seg[segmentID].Uuid}, SortField: "created_at",
	})
	if err != nil {
		return
	}
	for _, row := range rows {
		s.indexProduct(ctx, orgID, row.Uuid)
	}
}

// MapSegment converts a row to the API shape.
func MapSegment(row db.VehicleSegment, usage repository.SegmentUsage) model.Segment {
	return model.Segment{
		UUID: row.Uuid, Code: row.Code, Name: row.Name, Description: row.Description,
		SortOrder: row.SortOrder, IsActive: row.IsActive,
		GroupsCount: usage.Groups, ProductsCount: usage.Products,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
}

func applySegmentInput(f *repository.SegmentFields, in model.SegmentInput, create bool) error {
	ve := &ValidationError{}
	applyCode(ve, in.Code, create, &f.Code)
	applyName(ve, "name", in.Name, create, &f.Name)
	applyText(ve, "description", in.Description, 2000, &f.Description)
	if in.SortOrder != nil {
		if *in.SortOrder < 0 || *in.SortOrder > 100000 {
			ve.add("sort_order", "out_of_range", "sort_order must be between 0 and 100000")
		} else {
			f.SortOrder = *in.SortOrder
		}
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	return ve.orNil()
}
