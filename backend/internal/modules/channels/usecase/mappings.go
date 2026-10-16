package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func refOf(id pgtype.UUID, code, name pgtype.Text) *model.Ref {
	if !id.Valid {
		return nil
	}
	return &model.Ref{UUID: id.Bytes, Code: textOr(code), Name: textOr(name)}
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	x := v.Int32
	return &x
}

func mappingView(r db.GetChannelMappingRow) model.Mapping {
	return model.Mapping{
		UUID: r.Uuid, Account: model.Ref{UUID: r.AccountUuid, Name: r.AccountName, Provider: r.AccountProvider},
		EntityType: r.EntityType, Branch: refOf(r.BranchUuid, r.BranchCode, r.BranchName),
		Product: refOf(r.ProductUuid, r.ProductCode, r.ProductTitle), Extra: refOf(r.ExtraUuid, r.ExtraCode, r.ExtraName),
		ExternalCode: r.ExternalCode, ExternalName: r.ExternalName, Priority: r.Priority, StopSale: r.StopSale,
		MaxQuota: int4Ptr(r.MaxQuota), IsActive: r.IsActive, Note: r.Note,
		CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

// ListMappings lists a page of mappings.
func (s *Service) ListMappings(ctx context.Context, orgID int64, f model.MappingFilter) ([]model.Mapping, int64, error) {
	p := db.ListChannelMappingsParams{
		OrganizationID: orgID, EntityTypes: f.EntityTypes, SortField: f.SortField, SortDesc: f.SortDesc,
		LimitCount: f.Limit, OffsetCount: f.Offset,
	}
	if f.AccountUUID != nil {
		p.AccountUuid = pgtype.UUID{Bytes: *f.AccountUUID, Valid: true}
	}
	if f.Active != nil {
		p.Active = pgtype.Bool{Bool: *f.Active, Valid: true}
	}
	if f.StopSale != nil {
		p.StopSale = pgtype.Bool{Bool: *f.StopSale, Valid: true}
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Q = pgtype.Text{String: q, Valid: true}
	}
	rows, err := s.q.ListChannelMappings(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Mapping, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, mappingView(db.GetChannelMappingRow{
			Uuid: r.Uuid, EntityType: r.EntityType, ExternalCode: r.ExternalCode, ExternalName: r.ExternalName,
			Priority: r.Priority, StopSale: r.StopSale, MaxQuota: r.MaxQuota, IsActive: r.IsActive, Note: r.Note,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, AccountUuid: r.AccountUuid, AccountName: r.AccountName,
			AccountProvider: r.AccountProvider, BranchUuid: r.BranchUuid, BranchCode: r.BranchCode, BranchName: r.BranchName,
			ProductUuid: r.ProductUuid, ProductCode: r.ProductCode, ProductTitle: r.ProductTitle,
			ExtraUuid: r.ExtraUuid, ExtraCode: r.ExtraCode, ExtraName: r.ExtraName,
		}))
	}
	return out, total, nil
}

// MappingFacets counts mappings per entity type.
func (s *Service) MappingFacets(ctx context.Context, orgID int64) (model.MappingFacets, error) {
	rows, err := s.q.ChannelMappingFacets(ctx, orgID)
	if err != nil {
		return model.MappingFacets{}, err
	}
	out := model.MappingFacets{EntityTypes: map[string]int64{}}
	for _, t := range model.EntityTypes {
		out.EntityTypes[t] = 0
	}
	for _, r := range rows {
		out.EntityTypes[r.EntityType] = r.Total
		out.StopSale += r.StopSale
		out.Inactive += r.Inactive
	}
	return out, nil
}

// GetMapping returns one mapping.
func (s *Service) GetMapping(ctx context.Context, orgID int64, id uuid.UUID) (model.Mapping, error) {
	r, err := s.q.GetChannelMapping(ctx, db.GetChannelMappingParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Mapping{}, ErrMappingNotFound
	}
	if err != nil {
		return model.Mapping{}, err
	}
	return mappingView(r), nil
}

func uniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func cleanCode(field string, v *string, ve *ValidationError) string {
	if v == nil {
		ve.add(field, "required", field+" is required")
		return ""
	}
	c := strings.TrimSpace(*v)
	if c == "" || len(c) > 64 {
		ve.add(field, "invalid", field+" is required (at most 64 characters)")
	}
	return c
}

// CreateMapping maps an internal record to an external code of an account.
// The branch, product and extra must belong to the organization (another
// organization's uuid answers not_found).
func (s *Service) CreateMapping(ctx context.Context, orgID int64, in model.MappingInput) (model.Mapping, error) {
	if err := requireAllBranches(ctx); err != nil {
		return model.Mapping{}, err
	}
	ve := &ValidationError{}
	p := db.InsertChannelMappingParams{OrganizationID: orgID, EntityType: in.EntityType, IsActive: true, ActorID: actorID(ctx)}
	if in.AccountUUID == nil {
		ve.add("account_uuid", "required", "account_uuid is required")
	} else {
		r, err := s.accountRow(ctx, orgID, *in.AccountUUID)
		switch {
		case errors.Is(err, ErrAccountNotFound):
			ve.add("account_uuid", "not_found", "channel account was not found")
		case err != nil:
			return model.Mapping{}, err
		default:
			p.IntegrationAccountID = r.ID
		}
	}
	if !model.Contains(model.EntityTypes, in.EntityType) {
		ve.add("entity_type", "invalid", "entity_type must be branch, product or extra")
	}
	p.ExternalCode = cleanCode("external_code", in.ExternalCode, ve)
	if in.ExternalName != nil {
		p.ExternalName = strings.TrimSpace(*in.ExternalName)
	}
	if len(p.ExternalName) > 255 {
		ve.add("external_name", "too_long", "external_name is at most 255 characters")
	}
	if in.Priority != nil {
		p.Priority = *in.Priority
	}
	if in.IsActive != nil {
		p.IsActive = *in.IsActive
	}
	if in.Note != nil {
		p.Note = strings.TrimSpace(*in.Note)
	}
	if len(p.Note) > 1000 {
		ve.add("note", "too_long", "note is at most 1000 characters")
	}
	if in.EntityType == model.EntityProduct {
		if in.StopSale != nil {
			p.StopSale = *in.StopSale
		}
		if in.MaxQuota.Set && in.MaxQuota.Value != nil {
			if *in.MaxQuota.Value < 0 {
				ve.add("max_quota", "invalid", "max_quota must not be negative")
			}
			p.MaxQuota = pgtype.Int4{Int32: *in.MaxQuota.Value, Valid: true}
		}
	} else if (in.StopSale != nil && *in.StopSale) || (in.MaxQuota.Set && in.MaxQuota.Value != nil) {
		ve.add("stop_sale", "product_only", "stop-sale and quota apply to product mappings only")
	}
	switch in.EntityType {
	case model.EntityBranch:
		if !in.BranchUUID.Set || in.BranchUUID.Value == nil {
			ve.add("branch_uuid", "required", "branch_uuid is required")
		} else if id, err := s.branchID(ctx, orgID, *in.BranchUUID.Value); err != nil {
			ve.add("branch_uuid", "not_found", "branch was not found")
		} else {
			p.BranchID = int8Of(id)
		}
	case model.EntityProduct:
		if in.ProductUUID == nil {
			ve.add("product_uuid", "required", "product_uuid is required")
		} else if r, err := s.q.ChannelProductByUUID(ctx, db.ChannelProductByUUIDParams{OrganizationID: orgID, Uuid: *in.ProductUUID}); err != nil {
			ve.add("product_uuid", "not_found", "product was not found")
		} else {
			p.RentalProductID = int8Of(r.ID)
		}
		if in.BranchUUID.Set && in.BranchUUID.Value != nil {
			if id, err := s.branchID(ctx, orgID, *in.BranchUUID.Value); err != nil {
				ve.add("branch_uuid", "not_found", "branch was not found")
			} else {
				p.BranchID = int8Of(id)
			}
		}
	case model.EntityExtra:
		if in.ExtraUUID == nil {
			ve.add("extra_uuid", "required", "extra_uuid is required")
		} else if r, err := s.q.ChannelExtraByUUID(ctx, db.ChannelExtraByUUIDParams{OrganizationID: orgID, Uuid: *in.ExtraUUID}); err != nil {
			ve.add("extra_uuid", "not_found", "extra was not found")
		} else {
			p.ExtraID = int8Of(r.ID)
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Mapping{}, err
	}
	id, err := s.q.InsertChannelMapping(ctx, p)
	if uniqueViolation(err) {
		return model.Mapping{}, ErrMappingTaken
	}
	if err != nil {
		return model.Mapping{}, err
	}
	s.requestPush(ctx, orgID)
	s.publish(ctx, model.ResourceMappings, "created", id)
	return s.GetMapping(ctx, orgID, id)
}

func (s *Service) branchID(ctx context.Context, orgID int64, id uuid.UUID) (int64, error) {
	r, err := s.q.ChannelBranchByUUID(ctx, db.ChannelBranchByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return 0, err
	}
	return r.ID, nil
}

// UpdateMapping changes the external code / name, priority, stop-sale,
// quota, activity, note and (products) the location scope.
func (s *Service) UpdateMapping(ctx context.Context, orgID int64, id uuid.UUID, in model.MappingInput) (model.Mapping, error) {
	if err := requireAllBranches(ctx); err != nil {
		return model.Mapping{}, err
	}
	cur, err := s.q.GetChannelMapping(ctx, db.GetChannelMappingParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Mapping{}, ErrMappingNotFound
	}
	if err != nil {
		return model.Mapping{}, err
	}
	ve := &ValidationError{}
	p := db.UpdateChannelMappingParams{
		OrganizationID: orgID, ID: cur.ID, ExternalCode: cur.ExternalCode, ExternalName: cur.ExternalName,
		BranchID: cur.BranchID, Priority: cur.Priority, StopSale: cur.StopSale, MaxQuota: cur.MaxQuota,
		IsActive: cur.IsActive, Note: cur.Note, ActorID: actorID(ctx),
	}
	if in.EntityType != "" && in.EntityType != cur.EntityType {
		ve.add("entity_type", "immutable", "the entity type of a mapping cannot change")
	}
	if in.AccountUUID != nil && *in.AccountUUID != cur.AccountUuid {
		ve.add("account_uuid", "immutable", "the account of a mapping cannot change")
	}
	if (in.ProductUUID != nil && (!cur.ProductUuid.Valid || *in.ProductUUID != uuid.UUID(cur.ProductUuid.Bytes))) ||
		(in.ExtraUUID != nil && (!cur.ExtraUuid.Valid || *in.ExtraUUID != uuid.UUID(cur.ExtraUuid.Bytes))) {
		ve.add("product_uuid", "immutable", "the mapped record cannot change; create another mapping")
	}
	if in.ExternalCode != nil {
		p.ExternalCode = cleanCode("external_code", in.ExternalCode, ve)
	}
	if in.ExternalName != nil {
		p.ExternalName = strings.TrimSpace(*in.ExternalName)
		if len(p.ExternalName) > 255 {
			ve.add("external_name", "too_long", "external_name is at most 255 characters")
		}
	}
	if in.Priority != nil {
		p.Priority = *in.Priority
	}
	if in.IsActive != nil {
		p.IsActive = *in.IsActive
	}
	if in.Note != nil {
		p.Note = strings.TrimSpace(*in.Note)
		if len(p.Note) > 1000 {
			ve.add("note", "too_long", "note is at most 1000 characters")
		}
	}
	product := cur.EntityType == model.EntityProduct
	if in.StopSale != nil {
		if !product && *in.StopSale {
			ve.add("stop_sale", "product_only", "stop-sale applies to product mappings only")
		}
		p.StopSale = *in.StopSale && product
	}
	if in.MaxQuota.Set {
		switch {
		case in.MaxQuota.Value == nil:
			p.MaxQuota = pgtype.Int4{}
		case !product:
			ve.add("max_quota", "product_only", "quota applies to product mappings only")
		case *in.MaxQuota.Value < 0:
			ve.add("max_quota", "invalid", "max_quota must not be negative")
		default:
			p.MaxQuota = pgtype.Int4{Int32: *in.MaxQuota.Value, Valid: true}
		}
	}
	if in.BranchUUID.Set {
		switch {
		case cur.EntityType == model.EntityExtra:
			if in.BranchUUID.Value != nil {
				ve.add("branch_uuid", "not_allowed", "an extra mapping has no branch")
			}
		case in.BranchUUID.Value == nil:
			if cur.EntityType == model.EntityBranch {
				ve.add("branch_uuid", "required", "a branch mapping needs its branch")
			} else {
				p.BranchID = pgtype.Int8{}
			}
		default:
			if bid, err := s.branchID(ctx, orgID, *in.BranchUUID.Value); err != nil {
				ve.add("branch_uuid", "not_found", "branch was not found")
			} else {
				p.BranchID = int8Of(bid)
			}
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Mapping{}, err
	}
	if _, err := s.q.UpdateChannelMapping(ctx, p); err != nil {
		if uniqueViolation(err) {
			return model.Mapping{}, ErrMappingTaken
		}
		return model.Mapping{}, err
	}
	s.requestPush(ctx, orgID)
	s.publish(ctx, model.ResourceMappings, "updated", id)
	return s.GetMapping(ctx, orgID, id)
}

// DeleteMapping removes a mapping (its offers go with it; channel bookings
// keep their stored codes).
func (s *Service) DeleteMapping(ctx context.Context, orgID int64, id uuid.UUID) error {
	if err := requireAllBranches(ctx); err != nil {
		return err
	}
	cur, err := s.q.GetChannelMapping(ctx, db.GetChannelMappingParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return ErrMappingNotFound
	}
	if err != nil {
		return err
	}
	if _, err := s.q.DeleteChannelMapping(ctx, db.DeleteChannelMappingParams{OrganizationID: orgID, ID: cur.ID}); err != nil {
		return err
	}
	s.requestPush(ctx, orgID)
	s.publish(ctx, model.ResourceMappings, "deleted", id)
	return nil
}
