package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// RatePlanSortColumns are the sortable plan list fields.
var RatePlanSortColumns = map[string]string{
	"name": "name", "rental_product": "rental_product", "scope": "scope", "status": "status",
	"priority": "priority", "created_at": "created_at", "updated_at": "updated_at",
}

// planRow is the plan head the use cases work with.
type planRow = db.GetRatePlanRow

func toPlan(row planRow) model.RatePlan {
	out := model.RatePlan{
		UUID: row.Uuid, Name: row.Name, Scope: row.Scope, Status: row.Status,
		Product:        model.ProductRef{UUID: row.ProductUuid, Code: row.ProductCode, Title: row.ProductTitle},
		Channel:        textPtr(row.Channel),
		CurrentVersion: int(row.CurrentVersion), ActiveVersion: int4Ptr(row.ActiveVersion),
		ActivatedAt: timePtr(row.ActivatedAt), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
	if row.BranchUuid.Valid {
		out.Branch = &model.BranchRef{UUID: row.BranchUuid.Bytes, Code: row.BranchCode.String, Name: row.BranchName.String}
	}
	return out
}

func toPlanListRow(row db.ListRatePlansRow) model.RatePlan {
	out := toPlan(planRow{
		Uuid: row.Uuid, Scope: row.Scope, BranchID: row.BranchID, Channel: row.Channel, Name: row.Name,
		Status: row.Status, CurrentVersion: row.CurrentVersion, ActiveVersion: row.ActiveVersion,
		ActivatedAt: row.ActivatedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		ProductUuid: row.ProductUuid, ProductCode: row.ProductCode, ProductTitle: row.ProductTitle,
		BranchUuid: row.BranchUuid, BranchCode: row.BranchCode, BranchName: row.BranchName,
	})
	out.HasDraft, out.Priority, out.BandCount = row.HasDraft, int(row.Priority), row.BandCount
	if row.PriceFrom.Valid {
		m := money.FromNumericOrZero(row.PriceFrom)
		out.PriceFrom = &m
	}
	return out
}

func toPlanVersion(row db.GetRatePlanVersionByStatusRow, bands []model.Band) *model.PlanVersion {
	if bands == nil {
		bands = []model.Band{}
	}
	return &model.PlanVersion{
		UUID: row.Uuid, Version: int(row.Version), Status: row.Status, Priority: int(row.Priority),
		ChangeNote: row.ChangeNote, Bands: bands, CreatedAt: row.CreatedAt.Time.UTC(),
		CreatedBy:    actorFrom(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		ActivatedAt:  timePtr(row.ActivatedAt),
		ActivatedBy:  actorFrom(row.ActivatedByUuid, row.ActivatedByName, row.ActivatedBySurname, row.ActivatedByEmail),
		SupersededAt: timePtr(row.SupersededAt),
	}
}

// bandsByVersion loads the bands of versions, keyed by version id.
func bandsByVersion(ctx context.Context, q *db.Queries, orgID int64, ids []int64) (map[int64][]model.Band, error) {
	out := map[int64][]model.Band{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListRateBandsByVersions(ctx, db.ListRateBandsByVersionsParams{OrganizationID: orgID, VersionIds: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.RatePlanVersionID] = append(out[r.RatePlanVersionID], model.Band{
			MinDays: int(r.MinDays), MaxDays: int4Ptr(r.MaxDays), DailyPrice: money.FromNumericOrZero(r.DailyPrice),
		})
	}
	return out, nil
}

// planVersion loads the version of a plan with the given status (nil when
// there is none) and its bands.
func planVersion(ctx context.Context, q *db.Queries, orgID, planID int64, status string) (*model.PlanVersion, int64, error) {
	row, err := q.GetRatePlanVersionByStatus(ctx, db.GetRatePlanVersionByStatusParams{
		OrganizationID: orgID, RatePlanID: planID, Status: status,
	})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	bands, err := bandsByVersion(ctx, q, orgID, []int64{row.ID})
	if err != nil {
		return nil, 0, err
	}
	return toPlanVersion(row, bands[row.ID]), row.ID, nil
}

// ListPlans returns one page of rate plans in the caller's branch scope.
func (s *Service) ListPlans(ctx context.Context, orgID int64, f model.RatePlanFilter) ([]model.RatePlan, int64, error) {
	if _, ok := RatePlanSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "rental_product", false
	}
	f.BranchIDs = orgctx.BranchScope(ctx)
	q := s.repo.Q()
	rows, err := q.ListRatePlans(ctx, db.ListRatePlansParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, ProductUuids: uuids(f.ProductUUIDs), BranchUuids: uuids(f.BranchUUIDs),
		Scopes: strs(f.Scopes), Channels: strs(f.Channels), Statuses: strs(f.Statuses), HasDraft: boolArg(f.HasDraft),
		Q: textArg(f.Q), SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountRatePlans(ctx, db.CountRatePlansParams{
		OrganizationID: orgID, BranchIds: f.BranchIDs, ProductUuids: uuids(f.ProductUUIDs), BranchUuids: uuids(f.BranchUUIDs),
		Scopes: strs(f.Scopes), Channels: strs(f.Channels), Statuses: strs(f.Statuses), HasDraft: boolArg(f.HasDraft),
		Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.RatePlan, 0, len(rows))
	for _, row := range rows {
		out = append(out, toPlanListRow(row))
	}
	return out, total, nil
}

// PlanFacets returns the dynamic filter options in the caller's scope.
func (s *Service) PlanFacets(ctx context.Context, orgID int64) (model.RatePlanFacets, error) {
	rows, err := s.repo.Q().ListRatePlanFacets(ctx, db.ListRatePlanFacetsParams{OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx)})
	if err != nil {
		return model.RatePlanFacets{}, err
	}
	out := model.RatePlanFacets{
		RentalProduct: []model.Facet{}, Branch: []model.Facet{}, Scope: []model.Facet{}, Channel: []model.Facet{}, Status: []model.Facet{},
	}
	for _, row := range rows {
		f := model.Facet{Value: row.Value, Label: row.Label, Count: row.Count}
		switch row.Facet {
		case "rental_product":
			out.RentalProduct = append(out.RentalProduct, f)
		case "branch":
			out.Branch = append(out.Branch, f)
		case "scope":
			out.Scope = append(out.Scope, f)
		case "channel":
			out.Channel = append(out.Channel, f)
		case "status":
			out.Status = append(out.Status, f)
		}
	}
	return out, nil
}

// getPlan loads a live plan of the organization in the caller's branch
// scope (a branch plan outside it reads as not found).
func (s *Service) getPlan(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (planRow, error) {
	row, err := q.GetRatePlan(ctx, db.GetRatePlanParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return planRow{}, mapNotFound(repository.MapErr(err), ErrPlanNotFound)
	}
	if row.BranchID.Valid && orgctx.CheckBranch(ctx, row.BranchID.Int64) != nil {
		return planRow{}, ErrPlanNotFound
	}
	return row, nil
}

// GetPlan returns one plan with its active and draft versions.
func (s *Service) GetPlan(ctx context.Context, orgID int64, id uuid.UUID) (model.RatePlan, error) {
	return s.getPlanDetail(ctx, s.repo.Q(), orgID, id)
}

func (s *Service) getPlanDetail(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (model.RatePlan, error) {
	row, err := s.getPlan(ctx, q, orgID, id)
	if err != nil {
		return model.RatePlan{}, err
	}
	out := toPlan(row)
	active, _, err := planVersion(ctx, q, orgID, row.ID, model.VersionActive)
	if err != nil {
		return model.RatePlan{}, err
	}
	draft, _, err := planVersion(ctx, q, orgID, row.ID, model.VersionDraft)
	if err != nil {
		return model.RatePlan{}, err
	}
	out.Active, out.Draft = active, draft
	eff := draft
	if eff == nil {
		eff = active
	}
	out.HasDraft = draft != nil
	if eff != nil {
		out.Priority, out.BandCount = eff.Priority, int64(len(eff.Bands))
		for i, b := range eff.Bands {
			if i == 0 || b.DailyPrice.Cmp(*out.PriceFrom) < 0 {
				p := b.DailyPrice
				out.PriceFrom = &p
			}
		}
	}
	return out, nil
}

// PlanVersions lists every version of a plan (newest first) with bands.
func (s *Service) PlanVersions(ctx context.Context, orgID int64, id uuid.UUID) ([]model.PlanVersion, error) {
	q := s.repo.Q()
	row, err := s.getPlan(ctx, q, orgID, id)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRatePlanVersions(ctx, db.ListRatePlanVersionsParams{OrganizationID: orgID, RatePlanID: row.ID})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, v := range rows {
		ids = append(ids, v.ID)
	}
	bands, err := bandsByVersion(ctx, q, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]model.PlanVersion, 0, len(rows))
	for _, v := range rows {
		out = append(out, *toPlanVersion(db.GetRatePlanVersionByStatusRow(v), bands[v.ID]))
	}
	return out, nil
}

// PlanActivity lists the audit events of a plan.
func (s *Service) PlanActivity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getPlan(ctx, s.repo.Q(), orgID, id); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, model.ResourceRatePlans, id, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return activityEntries(rows), total, nil
}

// planTarget is a validated scope target.
type planTarget struct {
	Scope    string
	BranchID pgtype.Int8
	Branch   *model.BranchRef
	Channel  pgtype.Text
}

// resolveTarget validates scope + branch + channel. The branch must belong
// to the organization and lie in the caller's scope (validation
// "not_found" otherwise, like an unknown branch).
func (s *Service) resolveTarget(ctx context.Context, q *db.Queries, orgID int64, ve *ValidationError, prefix, scope string, branchUUID *uuid.UUID, channel *string) planTarget {
	t := planTarget{Scope: scope}
	if pricing.ScopeRank(scope) == 0 {
		ve.add(prefix+"scope", "invalid", "scope must be common, branch, channel or branch_channel")
		return t
	}
	needBranch := scope == pricing.ScopeBranch || scope == pricing.ScopeBranchChannel
	needChannel := scope == pricing.ScopeChannel || scope == pricing.ScopeBranchChannel
	switch {
	case needBranch && branchUUID == nil:
		ve.add(prefix+"branch_uuid", "required", "branch_uuid is required for this scope")
	case !needBranch && branchUUID != nil:
		ve.add(prefix+"branch_uuid", "not_allowed", "branch_uuid is only allowed for branch scopes")
	case needBranch:
		b, err := q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: *branchUUID})
		if err != nil || orgctx.CheckBranch(ctx, b.ID) != nil {
			ve.add(prefix+"branch_uuid", "not_found", "branch was not found")
		} else {
			t.BranchID = int8Arg(b.ID, true)
			t.Branch = &model.BranchRef{UUID: b.Uuid, Code: b.Code, Name: b.Name}
		}
	}
	ch := ""
	if channel != nil {
		ch = *channel
	}
	switch {
	case needChannel && ch == "":
		ve.add(prefix+"channel", "required", "channel is required for this scope")
	case !needChannel && ch != "":
		ve.add(prefix+"channel", "not_allowed", "channel is only allowed for channel scopes")
	case needChannel && !validChannel(ch):
		ve.add(prefix+"channel", "invalid", "channel is not a sales channel")
	case needChannel:
		t.Channel = textArg(ch)
	}
	return t
}

// parseBands validates band inputs (structure, prices, overlap) into
// engine bands.
func parseBands(ve *ValidationError, prefix string, in []model.BandInput) []model.Band {
	out := make([]model.Band, 0, len(in))
	ok := true
	for i, b := range in {
		field := fmt.Sprintf("%sbands.%d", prefix, i)
		price, good := parsePrice(ve, field+".daily_price", b.DailyPrice)
		if !good {
			ok = false
			continue
		}
		out = append(out, model.Band{MinDays: b.MinDays, MaxDays: b.MaxDays, DailyPrice: price})
	}
	if !ok {
		return out
	}
	for _, issue := range pricing.ValidateBands(engineBands(out), 0) {
		if issue.Code == pricing.IssueNoBands {
			continue // an empty draft is allowed; activation requires bands
		}
		ve.add(prefix+issue.Field, issue.Code, issue.Message)
	}
	return out
}

func engineBands(in []model.Band) []pricing.Band {
	out := make([]pricing.Band, 0, len(in))
	for _, b := range in {
		out = append(out, pricing.Band{MinDays: b.MinDays, MaxDays: b.MaxDays, DailyPrice: b.DailyPrice})
	}
	return out
}

func writeBands(ctx context.Context, q *db.Queries, orgID, versionID int64, bands []model.Band) error {
	if err := q.DeleteRateBands(ctx, db.DeleteRateBandsParams{OrganizationID: orgID, RatePlanVersionID: versionID}); err != nil {
		return repository.MapErr(err)
	}
	for _, b := range bands {
		if err := q.InsertRateBand(ctx, db.InsertRateBandParams{
			OrganizationID: orgID, RatePlanVersionID: versionID, MinDays: int32(b.MinDays), MaxDays: int4Arg(b.MaxDays),
			DailyPrice: b.DailyPrice.Numeric(),
		}); err != nil {
			return repository.MapErr(err)
		}
	}
	return nil
}

// CreatePlan stores a new plan as draft version 1.
func (s *Service) CreatePlan(ctx context.Context, orgID int64, in model.RatePlanCreateInput) (model.RatePlan, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	var productID int64
	if in.RentalProductUUID == nil {
		ve.add("rental_product_uuid", "required", "rental_product_uuid is required")
	} else if p, err := q.GetPricingProductByUUID(ctx, db.GetPricingProductByUUIDParams{OrganizationID: orgID, Uuid: *in.RentalProductUUID}); err != nil {
		ve.add("rental_product_uuid", "not_found", "rental product was not found")
	} else {
		productID = p.ID
	}
	target := s.resolveTarget(ctx, q, orgID, ve, "", in.Scope, in.BranchUUID, in.Channel)
	name := checkText(ve, "name", in.Name, 255)
	priority := checkPriority(ve, "priority", in.Priority)
	note := checkText(ve, "change_note", in.ChangeNote, 2000)
	bands := parseBands(ve, "", in.Bands)
	if err := ve.orNil(); err != nil {
		return model.RatePlan{}, err
	}
	var id uuid.UUID
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		tq := tx.Q()
		head, err := tq.CreateRatePlan(ctx, db.CreateRatePlanParams{
			OrganizationID: orgID, RentalProductID: productID, Scope: target.Scope, BranchID: target.BranchID,
			Channel: target.Channel, Name: name,
		})
		if err != nil {
			return repository.MapErr(err)
		}
		v, err := tq.CreateRatePlanVersion(ctx, db.CreateRatePlanVersionParams{
			OrganizationID: orgID, RatePlanID: head.ID, Version: 1, Priority: int32(priority), ChangeNote: note,
			CreatedBy: actorArg(ctx),
		})
		if err != nil {
			return repository.MapErr(err)
		}
		id = head.Uuid
		return writeBands(ctx, tq, orgID, v.ID, bands)
	})
	if err != nil {
		return model.RatePlan{}, mapBandErr(err)
	}
	return s.GetPlan(ctx, orgID, id)
}

func mapBandErr(err error) error {
	if errors.Is(err, repository.ErrBandOverlap) {
		return &ValidationError{Details: []Detail{{Field: "bands", Code: pricing.IssueBandOverlap, Message: "duration bands overlap"}}}
	}
	return mapNotFound(err, ErrPlanNotFound)
}

// ensurePlanDraft returns the draft version id of a plan, creating version
// N+1 from the active version (priority + bands) when there is none. The
// plan row must be locked by the caller.
func ensurePlanDraft(ctx context.Context, q *db.Queries, orgID int64, row planRow) (int64, bool, error) {
	draft, err := q.GetRatePlanVersionByStatus(ctx, db.GetRatePlanVersionByStatusParams{
		OrganizationID: orgID, RatePlanID: row.ID, Status: model.VersionDraft,
	})
	if err == nil {
		return draft.ID, false, nil
	}
	if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
		return 0, false, err
	}
	active, err := q.GetRatePlanVersionByStatus(ctx, db.GetRatePlanVersionByStatusParams{
		OrganizationID: orgID, RatePlanID: row.ID, Status: model.VersionActive,
	})
	if err != nil {
		return 0, false, repository.MapErr(err)
	}
	next := row.CurrentVersion + 1
	v, err := q.CreateRatePlanVersion(ctx, db.CreateRatePlanVersionParams{
		OrganizationID: orgID, RatePlanID: row.ID, Version: next, Priority: active.Priority, CreatedBy: actorArg(ctx),
	})
	if err != nil {
		return 0, false, repository.MapErr(err)
	}
	if err := q.CopyRateBands(ctx, db.CopyRateBandsParams{OrganizationID: orgID, FromVersionID: active.ID, ToVersionID: v.ID}); err != nil {
		return 0, false, repository.MapErr(err)
	}
	if err := repository.Rows(q.SetRatePlanState(ctx, db.SetRatePlanStateParams{
		OrganizationID: orgID, ID: row.ID, Status: row.Status, CurrentVersion: next,
		ActiveVersion: row.ActiveVersion, ActivatedAt: row.ActivatedAt,
	})); err != nil {
		return 0, false, err
	}
	return v.ID, true, nil
}

// lockPlan loads and locks a plan inside a transaction.
func (s *Service) lockPlan(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (planRow, error) {
	row, err := s.getPlan(ctx, q, orgID, id)
	if err != nil {
		return planRow{}, err
	}
	if _, err := q.LockRatePlan(ctx, db.LockRatePlanParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return planRow{}, mapNotFound(repository.MapErr(err), ErrPlanNotFound)
	}
	// Re-read after the lock (versions may have moved).
	return s.getPlan(ctx, q, orgID, id)
}

// PatchPlan renames a plan and / or edits its draft (priority, bands).
func (s *Service) PatchPlan(ctx context.Context, orgID int64, id uuid.UUID, in model.RatePlanPatchInput) (model.RatePlan, error) {
	ve := &ValidationError{}
	var name string
	if in.Name != nil {
		name = checkText(ve, "name", *in.Name, 255)
	}
	var priority int
	if in.Priority != nil {
		priority = checkPriority(ve, "priority", in.Priority)
	}
	var note string
	if in.ChangeNote != nil {
		note = checkText(ve, "change_note", *in.ChangeNote, 2000)
	}
	var bands []model.Band
	if in.Bands != nil {
		bands = parseBands(ve, "", *in.Bands)
	}
	if err := ve.orNil(); err != nil {
		return model.RatePlan{}, err
	}
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockPlan(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if in.Name != nil {
			if err := repository.Rows(q.UpdateRatePlanName(ctx, db.UpdateRatePlanNameParams{OrganizationID: orgID, ID: row.ID, Name: name})); err != nil {
				return err
			}
		}
		if in.Priority == nil && in.Bands == nil && in.ChangeNote == nil {
			return nil
		}
		draftID, _, err := ensurePlanDraft(ctx, q, orgID, row)
		if err != nil {
			return err
		}
		if in.Priority != nil || in.ChangeNote != nil {
			cur, err := q.GetRatePlanVersionByStatus(ctx, db.GetRatePlanVersionByStatusParams{
				OrganizationID: orgID, RatePlanID: row.ID, Status: model.VersionDraft,
			})
			if err != nil {
				return repository.MapErr(err)
			}
			p, n := cur.Priority, cur.ChangeNote
			if in.Priority != nil {
				p = int32(priority)
			}
			if in.ChangeNote != nil {
				n = note
			}
			if err := repository.Rows(q.UpdateRatePlanDraft(ctx, db.UpdateRatePlanDraftParams{
				OrganizationID: orgID, ID: draftID, Priority: p, ChangeNote: n,
			})); err != nil {
				return err
			}
		}
		if in.Bands != nil {
			if err := writeBands(ctx, q, orgID, draftID, bands); err != nil {
				return err
			}
		}
		return q.TouchRatePlan(ctx, db.TouchRatePlanParams{OrganizationID: orgID, ID: row.ID})
	})
	if err != nil {
		return model.RatePlan{}, mapBandErr(err)
	}
	return s.GetPlan(ctx, orgID, id)
}

// DiscardPlanDraft deletes the draft of a plan that has an active version.
func (s *Service) DiscardPlanDraft(ctx context.Context, orgID int64, id uuid.UUID) (model.RatePlan, error) {
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockPlan(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if !row.ActiveVersion.Valid {
			return stateErr("never_activated", "the plan was never activated; delete it instead")
		}
		if err := repository.Rows(q.DeleteRatePlanDraft(ctx, db.DeleteRatePlanDraftParams{OrganizationID: orgID, RatePlanID: row.ID})); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return stateErr("no_draft", "the plan has no draft")
			}
			return err
		}
		return repository.Rows(q.SetRatePlanState(ctx, db.SetRatePlanStateParams{
			OrganizationID: orgID, ID: row.ID, Status: row.Status, CurrentVersion: row.ActiveVersion.Int32,
			ActiveVersion: row.ActiveVersion, ActivatedAt: row.ActivatedAt,
		}))
	})
	if err != nil {
		return model.RatePlan{}, mapNotFound(err, ErrPlanNotFound)
	}
	return s.GetPlan(ctx, orgID, id)
}

// DeletePlan soft-deletes a plan (the engine stops using it; versions stay
// for reservations that reference them).
func (s *Service) DeletePlan(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.getPlan(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return err
	}
	return mapNotFound(repository.Rows(s.repo.Q().SoftDeleteRatePlan(ctx, db.SoftDeleteRatePlanParams{OrganizationID: orgID, ID: row.ID})), ErrPlanNotFound)
}

// PlanActivationCheck reports what activation would refuse.
func (s *Service) PlanActivationCheck(ctx context.Context, orgID int64, id uuid.UUID) (model.ActivationCheck, error) {
	q := s.repo.Q()
	row, err := s.getPlan(ctx, q, orgID, id)
	if err != nil {
		return model.ActivationCheck{}, err
	}
	cfg, _, err := s.orgPricing(ctx, orgID)
	if err != nil {
		return model.ActivationCheck{}, err
	}
	issues, err := s.planIssues(ctx, q, orgID, row, cfg, "")
	if err != nil {
		return model.ActivationCheck{}, err
	}
	return model.ActivationCheck{Ready: len(issues) == 0, Issues: issues}, nil
}

// reachesOnline reports whether a plan can price an online sale: its
// channel is online, or it has no channel and an online channel is on.
func reachesOnline(row planRow, cfg orgPricing) bool {
	if row.Channel.Valid {
		return osmodel.IsOnlineChannel(row.Channel.String)
	}
	return cfg.anyOnline()
}

// planIssues validates the version activation would put live: the draft,
// or (re-enabling an inactive plan without a draft) the active version.
func (s *Service) planIssues(ctx context.Context, q *db.Queries, orgID int64, row planRow, cfg orgPricing, prefix string) ([]model.Issue, error) {
	field := func(f string) string {
		if prefix == "" {
			return f
		}
		return prefix + "." + f
	}
	v, _, err := planVersion(ctx, q, orgID, row.ID, model.VersionDraft)
	if err != nil {
		return nil, err
	}
	if v == nil {
		if row.Status != model.StatusInactive {
			return []model.Issue{{Field: field("draft"), Code: "no_draft", Message: "there is no draft to activate"}}, nil
		}
		if v, _, err = planVersion(ctx, q, orgID, row.ID, model.VersionActive); err != nil || v == nil {
			return []model.Issue{{Field: field("draft"), Code: "no_draft", Message: "there is no version to activate"}}, err
		}
	}
	gapless := 0
	if reachesOnline(row, cfg) {
		gapless = cfg.MaxOnline
	}
	issues := issuesFrom(pricing.ValidateBands(engineBands(v.Bands), gapless), prefix)
	// Equal-priority overlap with another active plan of the same target.
	active, err := q.ListActiveRatePlans(ctx, db.ListActiveRatePlansParams{
		OrganizationID: orgID, RentalProductID: int8Arg(row.RentalProductID, true),
	})
	if err != nil {
		return nil, err
	}
	for _, other := range active {
		if other.ID == row.ID || other.Scope != row.Scope || other.BranchID != row.BranchID || other.Channel != row.Channel {
			continue
		}
		if int(other.Priority) == v.Priority {
			issues = append(issues, model.Issue{
				Field: field("priority"), Code: "priority_conflict",
				Message: fmt.Sprintf("active plan %q has the same scope and priority %d", other.Name, v.Priority),
				Params:  map[string]int{"priority": v.Priority},
				Ref:     &model.IssueRef{UUID: other.Uuid, Name: other.Name},
			})
		}
	}
	return issues, nil
}

// activatePlanTx freezes the draft (or re-enables an inactive plan) inside
// a transaction; issues refuse it.
func (s *Service) activatePlanTx(ctx context.Context, q *db.Queries, orgID int64, row planRow, cfg orgPricing, note *string, prefix string) ([]model.Issue, error) {
	issues, err := s.planIssues(ctx, q, orgID, row, cfg, prefix)
	if err != nil || len(issues) > 0 {
		return issues, err
	}
	now := s.clock()
	draft, err := q.GetRatePlanVersionByStatus(ctx, db.GetRatePlanVersionByStatusParams{
		OrganizationID: orgID, RatePlanID: row.ID, Status: model.VersionDraft,
	})
	if err != nil {
		if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return nil, err
		}
		// Re-enable the active version of an inactive plan.
		return nil, repository.Rows(q.SetRatePlanState(ctx, db.SetRatePlanStateParams{
			OrganizationID: orgID, ID: row.ID, Status: model.StatusActive, CurrentVersion: row.CurrentVersion,
			ActiveVersion: row.ActiveVersion, ActivatedAt: ts(now),
		}))
	}
	if note != nil {
		if err := repository.Rows(q.UpdateRatePlanDraft(ctx, db.UpdateRatePlanDraftParams{
			OrganizationID: orgID, ID: draft.ID, Priority: draft.Priority, ChangeNote: *note,
		})); err != nil {
			return nil, err
		}
	}
	if _, err := q.SupersedeRatePlanVersion(ctx, db.SupersedeRatePlanVersionParams{
		OrganizationID: orgID, RatePlanID: row.ID, SupersededAt: ts(now),
	}); err != nil {
		return nil, repository.MapErr(err)
	}
	if err := repository.Rows(q.ActivateRatePlanVersion(ctx, db.ActivateRatePlanVersionParams{
		OrganizationID: orgID, ID: draft.ID, ActivatedAt: ts(now), ActivatedBy: actorArg(ctx),
	})); err != nil {
		return nil, err
	}
	return nil, repository.Rows(q.SetRatePlanState(ctx, db.SetRatePlanStateParams{
		OrganizationID: orgID, ID: row.ID, Status: model.StatusActive, CurrentVersion: draft.Version,
		ActiveVersion: pgtype.Int4{Int32: draft.Version, Valid: true}, ActivatedAt: ts(now),
	}))
}

// ActivatePlan validates and freezes the draft (or re-enables an inactive
// plan). Issues answer ActivationError (422).
func (s *Service) ActivatePlan(ctx context.Context, orgID int64, id uuid.UUID, in model.ActivateInput) (model.RatePlan, error) {
	cfg, _, err := s.orgPricing(ctx, orgID)
	if err != nil {
		return model.RatePlan{}, err
	}
	ve := &ValidationError{}
	var note *string
	if in.ChangeNote != nil {
		n := checkText(ve, "change_note", *in.ChangeNote, 2000)
		note = &n
	}
	if err := ve.orNil(); err != nil {
		return model.RatePlan{}, err
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockPlan(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		issues, err := s.activatePlanTx(ctx, q, orgID, row, cfg, note, "")
		if err != nil {
			return err
		}
		if len(issues) > 0 {
			return &ActivationError{Issues: issues}
		}
		return nil
	})
	if err != nil {
		return model.RatePlan{}, mapNotFound(err, ErrPlanNotFound)
	}
	return s.GetPlan(ctx, orgID, id)
}

// DeactivatePlan switches an active plan off (versions are kept).
func (s *Service) DeactivatePlan(ctx context.Context, orgID int64, id uuid.UUID) (model.RatePlan, error) {
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockPlan(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if row.Status != model.StatusActive {
			return stateErr("not_active", "only an active plan can be deactivated")
		}
		return repository.Rows(q.SetRatePlanState(ctx, db.SetRatePlanStateParams{
			OrganizationID: orgID, ID: row.ID, Status: model.StatusInactive, CurrentVersion: row.CurrentVersion,
			ActiveVersion: row.ActiveVersion, ActivatedAt: row.ActivatedAt,
		}))
	})
	if err != nil {
		return model.RatePlan{}, mapNotFound(err, ErrPlanNotFound)
	}
	return s.GetPlan(ctx, orgID, id)
}

// PlanExportRow is one band row of the price list export.
type PlanExportRow = db.ListRatePlansForExportRow

// PlanExportRows returns the bands of the plans matching a filter (export;
// one row per band of the effective version), capped.
func (s *Service) PlanExportRows(ctx context.Context, orgID int64, f model.RatePlanFilter) ([]PlanExportRow, error) {
	return s.repo.Q().ListRatePlansForExport(ctx, db.ListRatePlansForExportParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), ProductUuids: uuids(f.ProductUUIDs),
		BranchUuids: uuids(f.BranchUUIDs), Scopes: strs(f.Scopes), Channels: strs(f.Channels),
		Statuses: strs(f.Statuses), Q: textArg(f.Q), LimitCount: MaxListTargets,
	})
}

// sameBands reports whether two band lists are equal (order-insensitive).
func sameBands(a, b []model.Band) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(x model.Band) string {
		max := -1
		if x.MaxDays != nil {
			max = *x.MaxDays
		}
		return fmt.Sprintf("%d|%d|%d", x.MinDays, max, x.DailyPrice.Minor())
	}
	ka, kb := make([]string, len(a)), make([]string, len(b))
	for i := range a {
		ka[i], kb[i] = key(a[i]), key(b[i])
	}
	sort.Strings(ka)
	sort.Strings(kb)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}
