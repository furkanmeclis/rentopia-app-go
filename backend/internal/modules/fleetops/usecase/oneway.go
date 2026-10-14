package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// OneWaySortColumns are the sortable rule list fields.
var OneWaySortColumns = map[string]string{
	"fee": "fee", "from_branch": "from_branch", "to_branch": "to_branch", "is_active": "is_active",
	"created_at": "created_at", "updated_at": "updated_at",
}

func toRule(row repository.RuleRow) model.OneWayRule {
	out := model.OneWayRule{
		UUID:       row.Uuid,
		FromBranch: model.BranchRef{UUID: row.FromBranchUuid, Code: row.FromBranchCode, Name: row.FromBranchName},
		ToBranch:   model.BranchRef{UUID: row.ToBranchUuid, Code: row.ToBranchCode, Name: row.ToBranchName},
		Allowed:    row.Allowed, Fee: FormatMoney(row.Fee), Note: row.Note, IsActive: row.IsActive,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.ProductUuid.Valid {
		out.Product = &model.ProductRef{UUID: row.ProductUuid.Bytes, Code: row.ProductCode.String, Title: row.ProductTitle.String}
	}
	return out
}

// ListRules returns one page of one-way rules in the caller's branch scope.
func (s *Service) ListRules(ctx context.Context, orgID int64, f model.OneWayFilter) ([]model.OneWayRule, int64, error) {
	if _, ok := OneWaySortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "created_at", true
	}
	f.BranchIDs = orgctx.BranchScope(ctx)
	rows, total, err := s.repo.ListRules(ctx, orgID, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.OneWayRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRule(row))
	}
	return out, total, nil
}

// RuleExportRows returns the rules matching a filter (export), capped.
func (s *Service) RuleExportRows(ctx context.Context, orgID int64, f model.OneWayFilter) ([]model.OneWayRule, error) {
	f.Limit, f.Offset, f.SortField, f.SortDesc = MaxListTargets, 0, "from_branch", false
	out, _, err := s.ListRules(ctx, orgID, f)
	return out, err
}

// RuleTargetUUIDs resolves the rules matching a filter (bulk "all matching").
func (s *Service) RuleTargetUUIDs(ctx context.Context, orgID int64, f model.OneWayFilter) ([]uuid.UUID, error) {
	f.BranchIDs = orgctx.BranchScope(ctx)
	return s.repo.RuleUUIDs(ctx, orgID, f, MaxListTargets)
}

// RuleFacets returns the dynamic filter options in the caller's scope.
func (s *Service) RuleFacets(ctx context.Context, orgID int64) (model.OneWayFacets, error) {
	rows, err := s.repo.RuleFacets(ctx, orgID, orgctx.BranchScope(ctx))
	if err != nil {
		return model.OneWayFacets{}, err
	}
	out := model.OneWayFacets{
		FromBranch: []model.Facet{}, ToBranch: []model.Facet{}, IsActive: []model.Facet{}, Allowed: []model.Facet{},
	}
	for _, row := range rows {
		f := model.Facet{Value: row.Value, Label: row.Label, Count: row.Count}
		switch row.Facet {
		case "from_branch":
			out.FromBranch = append(out.FromBranch, f)
		case "to_branch":
			out.ToBranch = append(out.ToBranch, f)
		case "is_active":
			out.IsActive = append(out.IsActive, f)
		case "allowed":
			out.Allowed = append(out.Allowed, f)
		}
	}
	return out, nil
}

// GetRule returns one rule of the organization.
func (s *Service) GetRule(ctx context.Context, orgID int64, id uuid.UUID) (model.OneWayRule, error) {
	row, err := s.getRule(ctx, orgID, id)
	if err != nil {
		return model.OneWayRule{}, err
	}
	return toRule(row), nil
}

func (s *Service) getRule(ctx context.Context, orgID int64, id uuid.UUID) (repository.RuleRow, error) {
	row, err := s.repo.GetRule(ctx, orgID, id)
	if err != nil {
		return repository.RuleRow{}, mapNotFound(err, ErrRuleNotFound)
	}
	if !eitherInScope(ctx, row.FromBranchID, row.ToBranchID) {
		return repository.RuleRow{}, ErrRuleNotFound
	}
	return row, nil
}

// CreateRule stores a rule. The from branch must be in the caller's scope;
// a route + product that already has a live rule answers ErrRouteTaken.
func (s *Service) CreateRule(ctx context.Context, orgID int64, in model.OneWayInput) (model.OneWayRule, error) {
	ve := &ValidationError{}
	if in.FromBranchUUID == nil {
		ve.add("from_branch_uuid", "required", "from_branch_uuid is required")
	}
	if in.ToBranchUUID == nil {
		ve.add("to_branch_uuid", "required", "to_branch_uuid is required")
	}
	fields := repository.RuleFields{Allowed: true, IsActive: true}
	_ = fields.Fee.Scan("0")
	s.applyRuleInput(ctx, orgID, ve, &fields, in)
	if err := ve.orNil(); err != nil {
		return model.OneWayRule{}, err
	}
	if taken, err := s.repo.RouteTaken(ctx, orgID, fields, nil); err != nil {
		return model.OneWayRule{}, err
	} else if taken {
		return model.OneWayRule{}, ErrRouteTaken
	}
	id, err := s.repo.CreateRule(ctx, orgID, fields)
	if err != nil {
		return model.OneWayRule{}, mapRouteTaken(err)
	}
	return s.GetRule(ctx, orgID, id)
}

// PatchRule updates the given fields of a rule.
func (s *Service) PatchRule(ctx context.Context, orgID int64, id uuid.UUID, in model.OneWayInput) (model.OneWayRule, error) {
	row, err := s.getRule(ctx, orgID, id)
	if err != nil {
		return model.OneWayRule{}, err
	}
	fields := repository.RuleFields{
		FromBranchID: row.FromBranchID, ToBranchID: row.ToBranchID, RentalProductID: row.RentalProductID,
		Allowed: row.Allowed, Fee: row.Fee, Note: row.Note, IsActive: row.IsActive,
	}
	ve := &ValidationError{}
	s.applyRuleInput(ctx, orgID, ve, &fields, in)
	if err := ve.orNil(); err != nil {
		return model.OneWayRule{}, err
	}
	if taken, err := s.repo.RouteTaken(ctx, orgID, fields, &id); err != nil {
		return model.OneWayRule{}, err
	} else if taken {
		return model.OneWayRule{}, ErrRouteTaken
	}
	if err := s.repo.UpdateRule(ctx, orgID, id, fields); err != nil {
		return model.OneWayRule{}, mapNotFound(mapRouteTaken(err), ErrRuleNotFound)
	}
	return s.GetRule(ctx, orgID, id)
}

// SetRuleActive switches is_active and returns the previous value (bulk).
func (s *Service) SetRuleActive(ctx context.Context, orgID int64, id uuid.UUID, active bool) (bool, error) {
	row, err := s.getRule(ctx, orgID, id)
	if err != nil {
		return false, err
	}
	if row.IsActive == active {
		return row.IsActive, nil
	}
	if err := s.repo.SetRuleActive(ctx, orgID, id, active); err != nil {
		return row.IsActive, mapNotFound(err, ErrRuleNotFound)
	}
	return row.IsActive, nil
}

// DeleteRule soft-deletes a rule.
func (s *Service) DeleteRule(ctx context.Context, orgID int64, id uuid.UUID) error {
	if _, err := s.getRule(ctx, orgID, id); err != nil {
		return err
	}
	return mapNotFound(s.repo.DeleteRule(ctx, orgID, id), ErrRuleNotFound)
}

// RuleActivity lists the audit events of a rule.
func (s *Service) RuleActivity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getRule(ctx, orgID, id); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, model.ResourceOneWayRules, id, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return activityEntries(rows), total, nil
}

// ResolveOneWay answers whether a rental of product (optional) picked up
// at from may be returned at to, and for which fee: the active rule of the
// product overrides the generic rule of the route; no active rule = not
// allowed. The same branch is always allowed for free (not one-way). M2 /
// M3 pricing calls it.
func (s *Service) ResolveOneWay(ctx context.Context, orgID int64, from, to uuid.UUID, product *uuid.UUID) (model.OneWayQuote, error) {
	ve := &ValidationError{}
	fb, err := s.repo.GetBranch(ctx, orgID, from)
	if err != nil {
		ve.add("from_branch", "not_found", "branch was not found")
	}
	tb, err := s.repo.GetBranch(ctx, orgID, to)
	if err != nil {
		ve.add("to_branch", "not_found", "branch was not found")
	}
	var productID pgtype.Int8
	if product != nil {
		p, err := s.repo.GetProduct(ctx, orgID, *product)
		if err != nil {
			ve.add("rental_product", "not_found", "rental product was not found")
		} else {
			productID = pgtype.Int8{Int64: p.ID, Valid: true}
		}
	}
	if err := ve.orNil(); err != nil {
		return model.OneWayQuote{}, err
	}
	if fb.ID == tb.ID {
		return model.OneWayQuote{Allowed: true, Fee: "0.00"}, nil
	}
	rows, err := s.repo.RulesForRoute(ctx, orgID, fb.ID, tb.ID, productID)
	if err != nil {
		return model.OneWayQuote{}, err
	}
	return QuoteFromRules(rows), nil
}

// QuoteFromRules picks the effective rule: rows come product rule first,
// generic rule second (ListOneWayRulesForRoute order).
func QuoteFromRules(rows []db.ListOneWayRulesForRouteRow) model.OneWayQuote {
	if len(rows) == 0 {
		return model.OneWayQuote{Allowed: false, Fee: "0.00"}
	}
	r := rows[0]
	id := r.Uuid
	fee := FormatMoney(r.Fee)
	if !r.Allowed {
		fee = "0.00"
	}
	return model.OneWayQuote{Allowed: r.Allowed, Fee: fee, Rule: &id, ProductSpecific: r.RentalProductID.Valid}
}

func (s *Service) applyRuleInput(ctx context.Context, orgID int64, ve *ValidationError, f *repository.RuleFields, in model.OneWayInput) {
	if in.FromBranchUUID != nil {
		b, err := s.repo.GetBranch(ctx, orgID, *in.FromBranchUUID)
		if err != nil || orgctx.CheckBranch(ctx, b.ID) != nil {
			// Outside the scope reads like an unknown branch.
			ve.add("from_branch_uuid", "not_found", "branch was not found")
		} else {
			f.FromBranchID = b.ID
		}
	}
	if in.ToBranchUUID != nil {
		b, err := s.repo.GetBranch(ctx, orgID, *in.ToBranchUUID)
		if err != nil {
			ve.add("to_branch_uuid", "not_found", "branch was not found")
		} else {
			f.ToBranchID = b.ID
		}
	}
	if in.RentalProductUUID.Set {
		f.RentalProductID = pgtype.Int8{}
		if in.RentalProductUUID.Value != nil {
			p, err := s.repo.GetProduct(ctx, orgID, *in.RentalProductUUID.Value)
			if err != nil {
				ve.add("rental_product_uuid", "not_found", "rental product was not found")
			} else {
				f.RentalProductID = pgtype.Int8{Int64: p.ID, Valid: true}
			}
		}
	}
	if !ve.has("from_branch_uuid") && !ve.has("to_branch_uuid") && f.FromBranchID != 0 && f.FromBranchID == f.ToBranchID {
		ve.add("to_branch_uuid", "same_branch", "to_branch_uuid must differ from from_branch_uuid")
	}
	if in.Allowed != nil {
		f.Allowed = *in.Allowed
	}
	if in.Fee != nil {
		if n := parseMoney(ve, "fee", in.Fee); n.Valid {
			f.Fee = n
		}
	}
	if in.Note != nil {
		f.Note = checkNote(ve, "note", *in.Note)
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
}

func mapRouteTaken(err error) error {
	if errors.Is(err, repository.ErrRouteTaken) {
		return ErrRouteTaken
	}
	return err
}
