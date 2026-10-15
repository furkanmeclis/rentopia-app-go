package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// GetAllocation returns one allocation in the caller's branch scope.
func (s *Service) GetAllocation(ctx context.Context, orgID int64, id uuid.UUID) (model.Allocation, error) {
	row, err := s.q.GetAllocation(ctx, db.GetAllocationParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.Allocation{}, ErrAllocationNotFound
	}
	if err != nil {
		return model.Allocation{}, err
	}
	if err := s.checkScope(ctx, row.PickupBranchID, row.ReturnBranchID); err != nil {
		return model.Allocation{}, err
	}
	return allocationView(row), nil
}

// ListAllocations returns a page of allocations (newest first) in the
// caller's branch scope. Active holds past their expiry are left out of an
// "active" listing (they no longer hold anything).
func (s *Service) ListAllocations(ctx context.Context, orgID int64, f model.AllocationFilter) ([]model.Allocation, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	live := len(f.Statuses) == 1 && f.Statuses[0] == model.StatusActive
	rows, err := s.q.ListAllocationUUIDs(ctx, db.ListAllocationUUIDsParams{
		OrganizationID: orgID, Kinds: nilIfEmpty(f.Kinds), Statuses: nilIfEmpty(f.Statuses),
		BranchIds: orgctx.BranchScope(ctx), LiveOnly: live, NowAt: ts(s.clock()),
		LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Allocation, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		a, err := s.GetAllocation(ctx, orgID, r.Uuid)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, nil
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func allocationView(row db.GetAllocationRow) model.Allocation {
	a := model.Allocation{
		UUID: row.Uuid, Kind: row.Kind, Status: row.Status,
		Product:      model.ProductRef{UUID: row.ProductUuid, Code: row.ProductCode, Title: row.ProductTitle, Version: row.ProductVersion},
		Vehicle:      model.VehicleRef{UUID: row.VehicleUuid, Plate: row.VehiclePlate, Label: row.VehicleLabel},
		PlateFixed:   row.PlateFixed,
		PickupBranch: model.BranchRef{UUID: row.PickupBranchUuid, Code: row.PickupBranchCode, Name: row.PickupBranchName},
		ReturnBranch: model.BranchRef{UUID: row.ReturnBranchUuid, Code: row.ReturnBranchCode, Name: row.ReturnBranchName},
		StartsAt:     row.StartsAt.Time.UTC(), EndsAt: row.EndsAt.Time.UTC(), ReadyAt: row.ReadyAt.Time.UTC(),
		PrepMinutes: row.PrepMinutes, ExpiresAt: timePtr(row.ExpiresAt), Channel: row.Channel, FleetPool: row.FleetPool,
		Note: row.Note, RefType: row.RefType.String, ReleasedAt: timePtr(row.ReleasedAt), ReleaseReason: row.ReleaseReason,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.RefUuid.Valid {
		id := uuid.UUID(row.RefUuid.Bytes)
		a.RefUUID = &id
	}
	if row.PickupPointUuid.Valid {
		a.PickupPoint = &model.PointRef{UUID: row.PickupPointUuid.Bytes, Name: row.PickupPointName.String}
	}
	if row.ReturnPointUuid.Valid {
		a.ReturnPoint = &model.PointRef{UUID: row.ReturnPointUuid.Bytes, Name: row.ReturnPointName.String}
	}
	if row.CreatedByUuid.Valid {
		a.CreatedBy = &model.Actor{
			UUID: row.CreatedByUuid.Bytes, Name: row.CreatedByName.String,
			Surname: row.CreatedBySurname.String, Email: row.CreatedByEmail.String,
		}
	}
	return a
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// ------------------------------------------------- channel capacity rules

// ListRules returns a page of capacity rules.
func (s *Service) ListRules(ctx context.Context, orgID int64, f model.RuleFilter) ([]model.CapacityRule, int64, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.SortField == "" {
		f.SortField, f.SortDesc = "created_at", true
	}
	var q pgtype.Text
	if v := strings.TrimSpace(f.Q); v != "" {
		q = pgtype.Text{String: v, Valid: true}
	}
	rows, err := s.q.ListCapacityRules(ctx, db.ListCapacityRulesParams{
		OrganizationID: orgID, RuleTypes: nilIfEmpty(f.RuleTypes), Channels: nilIfEmpty(f.Channels), Q: q,
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.CapacityRule, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, ruleView(db.GetCapacityRuleRow{
			ID: r.ID, Uuid: r.Uuid, RuleType: r.RuleType, Channel: r.Channel, Quantity: r.Quantity, Note: r.Note,
			IsActive: r.IsActive, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, ProductUuid: r.ProductUuid,
			ProductCode: r.ProductCode, ProductTitle: r.ProductTitle, BranchUuid: r.BranchUuid, BranchCode: r.BranchCode,
			BranchName: r.BranchName,
		}))
	}
	return out, total, nil
}

// GetRule returns one capacity rule.
func (s *Service) GetRule(ctx context.Context, orgID int64, id uuid.UUID) (model.CapacityRule, error) {
	row, err := s.q.GetCapacityRule(ctx, db.GetCapacityRuleParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.CapacityRule{}, ErrRuleNotFound
	}
	if err != nil {
		return model.CapacityRule{}, err
	}
	return ruleView(row), nil
}

func ruleView(row db.GetCapacityRuleRow) model.CapacityRule {
	r := model.CapacityRule{
		UUID: row.Uuid, RuleType: row.RuleType, Quantity: row.Quantity, Note: row.Note, IsActive: row.IsActive,
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.Channel.Valid {
		c := row.Channel.String
		r.Channel = &c
	}
	if row.ProductUuid.Valid {
		r.Product = &model.ProductRef{UUID: row.ProductUuid.Bytes, Code: row.ProductCode.String, Title: row.ProductTitle.String}
	}
	if row.BranchUuid.Valid {
		r.Branch = &model.BranchRef{UUID: row.BranchUuid.Bytes, Code: row.BranchCode.String, Name: row.BranchName.String}
	}
	return r
}

type ruleFields struct {
	ruleType  string
	channel   pgtype.Text
	productID pgtype.Int8
	branchID  pgtype.Int8
	quantity  int32
	note      string
	active    bool
}

// validateRule checks a rule body; exclude is the rule being updated.
func (s *Service) validateRule(ctx context.Context, orgID int64, in model.CapacityRuleInput, exclude *uuid.UUID) (ruleFields, error) {
	ve := &ValidationError{}
	f := ruleFields{ruleType: in.RuleType, note: strings.TrimSpace(in.Note), active: true}
	if in.IsActive != nil {
		f.active = *in.IsActive
	}
	known := false
	for _, t := range model.RuleTypes {
		known = known || t == in.RuleType
	}
	if !known {
		ve.add("rule_type", "invalid", "rule_type must be sales_limit, guaranteed_quota or safety_margin")
	}
	if in.Channel != nil && strings.TrimSpace(*in.Channel) != "" {
		ch := strings.TrimSpace(*in.Channel)
		settings, err := s.settings.Current(ctx, orgID)
		if err != nil {
			return f, err
		}
		found := false
		for _, c := range settings.Settings.SalesChannels {
			found = found || c.Code == ch
		}
		if !found {
			ve.add("channel", "invalid", "unknown sales channel")
		}
		f.channel = pgtype.Text{String: ch, Valid: true}
	} else if known && in.RuleType != model.RuleSafetyMargin {
		ve.add("channel", "required", "sales limits and quotas belong to a channel")
	}
	if in.ProductUUID != nil {
		p, err := s.q.GetAvailabilityProduct(ctx, db.GetAvailabilityProductParams{OrganizationID: orgID, Uuid: *in.ProductUUID})
		if err != nil {
			ve.add("product_uuid", "not_found", "rental product was not found")
		} else {
			f.productID = pgtype.Int8{Int64: p.ID, Valid: true}
		}
	}
	if in.BranchUUID != nil {
		b, err := s.q.GetAvailabilityBranch(ctx, db.GetAvailabilityBranchParams{OrganizationID: orgID, Uuid: *in.BranchUUID})
		if err != nil || orgctx.CheckBranch(ctx, b.ID) != nil {
			ve.add("branch_uuid", "not_found", "branch was not found")
		} else {
			f.branchID = pgtype.Int8{Int64: b.ID, Valid: true}
		}
	} else if err := orgctx.RequireAllBranches(ctx); err != nil {
		return f, err
	}
	if in.Quantity < 0 || in.Quantity > 10000 {
		ve.add("quantity", "out_of_range", "quantity must be between 0 and 10000")
	}
	f.quantity = int32(in.Quantity)
	if err := ve.orNil(); err != nil {
		return f, err
	}
	if f.ruleType == model.RuleGuaranteedQuota && f.active {
		if err := s.checkQuotaSum(ctx, orgID, f, exclude); err != nil {
			return f, err
		}
	}
	return f, nil
}

// checkQuotaSum: the guaranteed quotas of a product (its own and the rules
// for every product) never exceed its eligible vehicles.
func (s *Service) checkQuotaSum(ctx context.Context, orgID int64, f ruleFields, exclude *uuid.UUID) error {
	snap, err := s.snapshot(ctx, orgID)
	if err != nil {
		return err
	}
	rules, err := s.q.ListActiveCapacityRules(ctx, orgID)
	if err != nil {
		return err
	}
	for _, p := range snap.products {
		if f.productID.Valid && f.productID.Int64 != p.ProductID {
			continue
		}
		sum := int(f.quantity)
		for _, r := range rules {
			if r.RuleType != model.RuleGuaranteedQuota || (exclude != nil && r.Uuid == *exclude) {
				continue
			}
			if !r.RentalProductID.Valid || r.RentalProductID.Int64 == p.ProductID {
				sum += int(r.Quantity)
			}
		}
		if capacity := len(snap.eligible[versionKey(p.ProductID, p.Version)]); sum > capacity {
			return validation("quantity", "exceeds_capacity",
				"guaranteed quotas of "+p.Code+" would exceed its "+itoa(capacity)+" eligible vehicles")
		}
	}
	return nil
}

// CreateRule adds a capacity rule.
func (s *Service) CreateRule(ctx context.Context, orgID int64, in model.CapacityRuleInput, actorID *int64) (model.CapacityRule, error) {
	f, err := s.validateRule(ctx, orgID, in, nil)
	if err != nil {
		return model.CapacityRule{}, err
	}
	id, err := s.q.CreateCapacityRule(ctx, db.CreateCapacityRuleParams{
		OrganizationID: orgID, RuleType: f.ruleType, Channel: f.channel, RentalProductID: f.productID,
		BranchID: f.branchID, Quantity: f.quantity, Note: f.note, IsActive: f.active, CreatedBy: int8Arg(actorID),
	})
	if err != nil {
		return model.CapacityRule{}, ruleWriteErr(err)
	}
	return s.GetRule(ctx, orgID, id)
}

// UpdateRule replaces a capacity rule.
func (s *Service) UpdateRule(ctx context.Context, orgID int64, id uuid.UUID, in model.CapacityRuleInput) (model.CapacityRule, error) {
	if _, err := s.GetRule(ctx, orgID, id); err != nil {
		return model.CapacityRule{}, err
	}
	f, err := s.validateRule(ctx, orgID, in, &id)
	if err != nil {
		return model.CapacityRule{}, err
	}
	n, err := s.q.UpdateCapacityRule(ctx, db.UpdateCapacityRuleParams{
		OrganizationID: orgID, Uuid: id, RuleType: f.ruleType, Channel: f.channel, RentalProductID: f.productID,
		BranchID: f.branchID, Quantity: f.quantity, Note: f.note, IsActive: f.active,
	})
	if err != nil {
		return model.CapacityRule{}, ruleWriteErr(err)
	}
	if n == 0 {
		return model.CapacityRule{}, ErrRuleNotFound
	}
	return s.GetRule(ctx, orgID, id)
}

// DeleteRule soft-deletes a capacity rule.
func (s *Service) DeleteRule(ctx context.Context, orgID int64, id uuid.UUID) error {
	rule, err := s.GetRule(ctx, orgID, id)
	if err != nil {
		return err
	}
	if rule.Branch == nil {
		if err := orgctx.RequireAllBranches(ctx); err != nil {
			return err
		}
	}
	n, err := s.q.SoftDeleteCapacityRule(ctx, db.SoftDeleteCapacityRuleParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrRuleNotFound
	}
	return nil
}

func ruleWriteErr(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ErrRuleTaken
	}
	return err
}
