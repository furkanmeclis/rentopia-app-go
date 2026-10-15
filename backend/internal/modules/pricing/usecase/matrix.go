package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// matrixTarget validates a matrix target.
func (s *Service) matrixTarget(ctx context.Context, q *db.Queries, orgID int64, ve *ValidationError, in model.MatrixTarget) planTarget {
	t := s.resolveTarget(ctx, q, orgID, ve, "", in.Scope, in.BranchUUID, in.Channel)
	p := in.Priority
	checkPriority(ve, "priority", &p)
	return t
}

// targetPlans returns, per product id, the plan of the target the matrix
// edits (the active one, else the newest).
func targetPlans(ctx context.Context, q *db.Queries, orgID int64, t planTarget, priority int, productID pgtype.Int8) (map[int64]db.ListRatePlansForTargetRow, error) {
	rows, err := q.ListRatePlansForTarget(ctx, db.ListRatePlansForTargetParams{
		OrganizationID: orgID, Scope: t.Scope, BranchID: t.BranchID, Channel: t.Channel, Priority: int32(priority),
		RentalProductID: productID,
	})
	if err != nil {
		return nil, err
	}
	out := map[int64]db.ListRatePlansForTargetRow{}
	for _, r := range rows {
		if _, ok := out[r.RentalProductID]; !ok {
			out[r.RentalProductID] = r
		}
	}
	return out, nil
}

func effectiveVersion(r db.ListRatePlansForTargetRow) (int64, string) {
	if r.DraftVersionID.Valid {
		return r.DraftVersionID.Int64, model.VersionDraft
	}
	if r.ActiveVersionID.Valid {
		return r.ActiveVersionID.Int64, model.VersionActive
	}
	return 0, ""
}

func bandKey(min int, max *int) string {
	if max == nil {
		return fmt.Sprintf("%d-", min)
	}
	return fmt.Sprintf("%d-%d", min, *max)
}

// Matrix returns the product × band grid of one scope target.
func (s *Service) Matrix(ctx context.Context, orgID int64, in model.MatrixTarget) (model.Matrix, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	t := s.matrixTarget(ctx, q, orgID, ve, in)
	if err := ve.orNil(); err != nil {
		return model.Matrix{}, err
	}
	return s.matrix(ctx, q, orgID, t, in.Priority)
}

func (s *Service) matrix(ctx context.Context, q *db.Queries, orgID int64, t planTarget, priority int) (model.Matrix, error) {
	products, err := q.ListPricingProducts(ctx, orgID)
	if err != nil {
		return model.Matrix{}, err
	}
	plans, err := targetPlans(ctx, q, orgID, t, priority, pgtype.Int8{})
	if err != nil {
		return model.Matrix{}, err
	}
	ids := []int64{}
	for _, p := range plans {
		if id, _ := effectiveVersion(p); id != 0 {
			ids = append(ids, id)
		}
	}
	bands, err := bandsByVersion(ctx, q, orgID, ids)
	if err != nil {
		return model.Matrix{}, err
	}
	// Columns: the union of the band ranges in use.
	cols := map[string]model.MatrixBand{}
	for _, list := range bands {
		for _, b := range list {
			cols[bandKey(b.MinDays, b.MaxDays)] = model.MatrixBand{MinDays: b.MinDays, MaxDays: b.MaxDays}
		}
	}
	out := model.Matrix{Scope: t.Scope, Branch: t.Branch, Priority: priority, Bands: []model.MatrixBand{}, Rows: []model.MatrixRow{}}
	if t.Channel.Valid {
		ch := t.Channel.String
		out.Channel = &ch
	}
	for _, c := range cols {
		out.Bands = append(out.Bands, c)
	}
	sort.Slice(out.Bands, func(i, j int) bool {
		a, b := out.Bands[i], out.Bands[j]
		if a.MinDays != b.MinDays {
			return a.MinDays < b.MinDays
		}
		return a.MaxDays != nil && (b.MaxDays == nil || *a.MaxDays < *b.MaxDays)
	})
	index := map[string]int{}
	for i, b := range out.Bands {
		index[bandKey(b.MinDays, b.MaxDays)] = i
	}
	for _, p := range products {
		row := model.MatrixRow{
			Product: model.ProductRef{UUID: p.Uuid, Code: p.Code, Title: p.Title},
			Segment: model.SegmentRef{UUID: p.SegmentUuid, Code: p.SegmentCode, Name: p.SegmentName},
			Prices:  make([]*money.Money, len(out.Bands)),
		}
		if plan, ok := plans[p.ID]; ok {
			id := plan.Uuid
			status := plan.Status
			row.PlanUUID, row.Status, row.HasDraft = &id, &status, plan.DraftVersionID.Valid
			if row.HasDraft {
				out.Drafts++
			}
			if vid, src := effectiveVersion(plan); vid != 0 {
				row.Source = &src
				for _, b := range bands[vid] {
					price := b.DailyPrice
					row.Prices[index[bandKey(b.MinDays, b.MaxDays)]] = &price
				}
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// SaveMatrix writes the changed rows as drafts (creating plans as needed).
// It returns the plans it created / changed (live events, activity).
func (s *Service) SaveMatrix(ctx context.Context, orgID int64, in model.MatrixSaveInput) (model.MatrixSaveResult, []uuid.UUID, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	t := s.matrixTarget(ctx, q, orgID, ve, in.MatrixTarget)
	note := checkText(ve, "change_note", in.ChangeNote, 2000)
	cols := make([]pricing.Band, 0, len(in.Bands))
	for _, b := range in.Bands {
		cols = append(cols, pricing.Band{MinDays: b.MinDays, MaxDays: b.MaxDays})
	}
	for _, issue := range pricing.ValidateBands(cols, 0) {
		if issue.Code != pricing.IssueNoBands {
			ve.add(issue.Field, issue.Code, issue.Message)
		}
	}
	products, err := q.ListPricingProducts(ctx, orgID)
	if err != nil {
		return model.MatrixSaveResult{}, nil, err
	}
	byUUID := map[uuid.UUID]db.ListPricingProductsRow{}
	for _, p := range products {
		byUUID[p.Uuid] = p
	}
	type rowPlan struct {
		product db.ListPricingProductsRow
		bands   []model.Band
	}
	work := []rowPlan{}
	seen := map[uuid.UUID]bool{}
	for i, r := range in.Rows {
		p, ok := byUUID[r.RentalProductUUID]
		if !ok || seen[r.RentalProductUUID] {
			ve.add(fmt.Sprintf("rows.%d.rental_product_uuid", i), "not_found", "rental product was not found")
			continue
		}
		seen[r.RentalProductUUID] = true
		if len(r.Prices) != len(in.Bands) {
			ve.add(fmt.Sprintf("rows.%d.prices", i), "invalid", "prices must align with bands")
			continue
		}
		bands := []model.Band{}
		for j, raw := range r.Prices {
			if raw == nil || *raw == "" {
				continue
			}
			price, ok := parsePrice(ve, fmt.Sprintf("rows.%d.prices.%d", i, j), *raw)
			if ok {
				bands = append(bands, model.Band{MinDays: in.Bands[j].MinDays, MaxDays: in.Bands[j].MaxDays, DailyPrice: price})
			}
		}
		if len(bands) > 0 {
			work = append(work, rowPlan{product: p, bands: bands})
		}
	}
	if err := ve.orNil(); err != nil {
		return model.MatrixSaveResult{}, nil, err
	}
	res := model.MatrixSaveResult{}
	changed := []uuid.UUID{}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		tq := tx.Q()
		plans, err := targetPlans(ctx, tq, orgID, t, in.Priority, pgtype.Int8{})
		if err != nil {
			return err
		}
		for _, w := range work {
			plan, ok := plans[w.product.ID]
			if !ok {
				head, err := tq.CreateRatePlan(ctx, db.CreateRatePlanParams{
					OrganizationID: orgID, RentalProductID: w.product.ID, Scope: t.Scope, BranchID: t.BranchID, Channel: t.Channel,
				})
				if err != nil {
					return repository.MapErr(err)
				}
				v, err := tq.CreateRatePlanVersion(ctx, db.CreateRatePlanVersionParams{
					OrganizationID: orgID, RatePlanID: head.ID, Version: 1, Priority: int32(in.Priority), ChangeNote: note,
					CreatedBy: actorArg(ctx),
				})
				if err != nil {
					return repository.MapErr(err)
				}
				if err := writeBands(ctx, tq, orgID, v.ID, w.bands); err != nil {
					return err
				}
				res.Created++
				changed = append(changed, head.Uuid)
				continue
			}
			vid, _ := effectiveVersion(plan)
			current, err := bandsByVersion(ctx, tq, orgID, []int64{vid})
			if err != nil {
				return err
			}
			if sameBands(current[vid], w.bands) {
				res.Unchanged++
				continue
			}
			row, err := s.lockPlan(ctx, tq, orgID, plan.Uuid)
			if err != nil {
				return err
			}
			draftID, _, err := ensurePlanDraft(ctx, tq, orgID, row)
			if err != nil {
				return err
			}
			if note != "" {
				if err := repository.Rows(tq.UpdateRatePlanDraft(ctx, db.UpdateRatePlanDraftParams{
					OrganizationID: orgID, ID: draftID, Priority: int32(in.Priority), ChangeNote: note,
				})); err != nil {
					return err
				}
			}
			if err := writeBands(ctx, tq, orgID, draftID, w.bands); err != nil {
				return err
			}
			if err := tq.TouchRatePlan(ctx, db.TouchRatePlanParams{OrganizationID: orgID, ID: row.ID}); err != nil {
				return err
			}
			res.Updated++
			changed = append(changed, plan.Uuid)
		}
		return nil
	})
	if err != nil {
		return model.MatrixSaveResult{}, nil, mapBandErr(err)
	}
	m, err := s.matrix(ctx, q, orgID, t, in.Priority)
	if err != nil {
		return model.MatrixSaveResult{}, nil, err
	}
	res.Matrix = m
	return res, changed, nil
}

// ActivateMatrix activates every draft of the target in one transaction;
// any issue (prefixed with the product code) refuses all of them.
func (s *Service) ActivateMatrix(ctx context.Context, orgID int64, in model.MatrixTarget) (model.MatrixActivateResult, []uuid.UUID, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	t := s.matrixTarget(ctx, q, orgID, ve, in)
	if err := ve.orNil(); err != nil {
		return model.MatrixActivateResult{}, nil, err
	}
	cfg, _, err := s.orgPricing(ctx, orgID)
	if err != nil {
		return model.MatrixActivateResult{}, nil, err
	}
	activated := []uuid.UUID{}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		tq := tx.Q()
		rows, err := tq.ListRatePlansForTarget(ctx, db.ListRatePlansForTargetParams{
			OrganizationID: orgID, Scope: t.Scope, BranchID: t.BranchID, Channel: t.Channel, Priority: int32(in.Priority),
		})
		if err != nil {
			return err
		}
		issues := []model.Issue{}
		for _, r := range rows {
			if !r.DraftVersionID.Valid {
				continue
			}
			row, err := s.lockPlan(ctx, tq, orgID, r.Uuid)
			if err != nil {
				return err
			}
			found, err := s.activatePlanTx(ctx, tq, orgID, row, cfg, nil, row.ProductCode)
			if err != nil {
				return err
			}
			if len(found) > 0 {
				for i := range found {
					if found[i].Ref == nil {
						found[i].Ref = &model.IssueRef{UUID: row.Uuid, Name: row.ProductCode}
					}
				}
				issues = append(issues, found...)
				continue
			}
			activated = append(activated, row.Uuid)
		}
		if len(issues) > 0 {
			return &ActivationError{Issues: issues}
		}
		return nil
	})
	if err != nil {
		return model.MatrixActivateResult{}, nil, mapNotFound(err, ErrPlanNotFound)
	}
	return model.MatrixActivateResult{Activated: len(activated)}, activated, nil
}
