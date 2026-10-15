package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// PriceRow is one row of the İdes price list import: a product code, a
// band ("1-3", "15+") and a daily price, optionally for a branch / channel
// override and a priority. Imports only write drafts.
type PriceRow struct {
	ProductCode string
	Band        string
	DailyPrice  string
	BranchCode  string
	Channel     string
	Priority    string
}

// RowError is a row-level import problem (the row is skipped).
type RowError struct{ Message string }

func (e *RowError) Error() string { return e.Message }

func rowErr(format string, args ...any) error {
	return &RowError{Message: fmt.Sprintf(format, args...)}
}

// PriceRowResult is what applying a row did.
type PriceRowResult struct {
	Op       string
	PlanUUID uuid.UUID
	Previous map[string]any
	Changes  []Change
}

// Change is one field a row changes (dry-run diff).
type Change struct {
	Field, From, To string
}

var bandRE = regexp.MustCompile(`^(\d{1,4})\s*(?:(\+)|[-–—]\s*(\d{1,4})?)?$`)

// ParseBand reads "1-3", "1 – 3", "15+", "15-" (open) or "7" (one length);
// a trailing "gün" / "days" is ignored.
func ParseBand(raw string) (int, *int, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	for _, suffix := range []string{"gün", "gun", "days", "day"} {
		s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
	}
	m := bandRE.FindStringSubmatch(s)
	if m == nil {
		return 0, nil, fmt.Errorf("band %q must look like 1-3 or 15+", raw)
	}
	min, _ := strconv.Atoi(m[1])
	if min < 1 {
		return 0, nil, fmt.Errorf("band %q must start at 1 day or more", raw)
	}
	switch {
	case m[2] == "+" || (m[3] == "" && strings.ContainsAny(s, "-–—")):
		return min, nil, nil
	case m[3] == "":
		return min, &min, nil
	}
	max, _ := strconv.Atoi(m[3])
	if max < min {
		return 0, nil, fmt.Errorf("band %q ends before it starts", raw)
	}
	return min, &max, nil
}

// FormatBand renders a band as the import reads it ("1-3", "15+").
func FormatBand(min int, max *int) string {
	if max == nil {
		return fmt.Sprintf("%d+", min)
	}
	return fmt.Sprintf("%d-%d", min, *max)
}

type resolvedRow struct {
	product  db.RentalProduct
	target   planTarget
	priority int
	min      int
	max      *int
	price    money.Money
}

func (s *Service) resolvePriceRow(ctx context.Context, q *db.Queries, orgID int64, in PriceRow) (resolvedRow, error) {
	out := resolvedRow{}
	code := strings.TrimSpace(in.ProductCode)
	if code == "" {
		return out, rowErr("product_code is required")
	}
	p, err := q.GetPricingProductByCode(ctx, db.GetPricingProductByCodeParams{OrganizationID: orgID, Code: code})
	if err != nil {
		return out, rowErr("rental product %q was not found", code)
	}
	out.product = p
	min, max, err := ParseBand(in.Band)
	if err != nil {
		return out, rowErr("%s", err.Error())
	}
	out.min, out.max = min, max
	price, err := money.Parse(strings.TrimSpace(in.DailyPrice))
	if err != nil || price.IsNegative() {
		return out, rowErr("daily_price %q must be a non-negative amount with at most 2 decimals", in.DailyPrice)
	}
	out.price = price
	if raw := strings.TrimSpace(in.Priority); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > model.MaxPriority {
			return out, rowErr("priority %q must be between 0 and 1000", raw)
		}
		out.priority = n
	}
	branchCode, channel := strings.TrimSpace(in.BranchCode), strings.TrimSpace(strings.ToLower(in.Channel))
	t := planTarget{Scope: pricing.ScopeCommon}
	if branchCode != "" {
		b, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: orgID, Code: branchCode})
		if err != nil || orgctx.CheckBranch(ctx, b.ID) != nil {
			return out, rowErr("branch %q was not found", branchCode)
		}
		t.Scope, t.BranchID = pricing.ScopeBranch, int8Arg(b.ID, true)
		t.Branch = &model.BranchRef{UUID: b.Uuid, Code: b.Code, Name: b.Name}
	}
	if channel != "" {
		if !validChannel(channel) {
			return out, rowErr("channel %q is not a sales channel", channel)
		}
		t.Channel = textArg(channel)
		if t.Scope == pricing.ScopeBranch {
			t.Scope = pricing.ScopeBranchChannel
		} else {
			t.Scope = pricing.ScopeChannel
		}
	}
	out.target = t
	return out, nil
}

// PreviewPriceRow dry-runs a row: create (new band / plan) or update (price
// change) without writing.
func (s *Service) PreviewPriceRow(ctx context.Context, orgID int64, in PriceRow) (PriceRowResult, error) {
	q := s.repo.Q()
	r, err := s.resolvePriceRow(ctx, q, orgID, in)
	if err != nil {
		return PriceRowResult{}, err
	}
	plans, err := targetPlans(ctx, q, orgID, r.target, r.priority, int8Arg(r.product.ID, true))
	if err != nil {
		return PriceRowResult{}, err
	}
	res := PriceRowResult{Op: "create"}
	to := r.price.String()
	plan, ok := plans[r.product.ID]
	if !ok {
		res.Changes = []Change{{Field: "daily_price", From: "", To: to}}
		return res, nil
	}
	res.PlanUUID = plan.Uuid
	vid, _ := effectiveVersion(plan)
	bands, err := bandsByVersion(ctx, q, orgID, []int64{vid})
	if err != nil {
		return PriceRowResult{}, err
	}
	for _, b := range bands[vid] {
		if b.MinDays == r.min && sameMax(b.MaxDays, r.max) {
			res.Op = "update"
			res.Changes = []Change{}
			if b.DailyPrice.Minor() != r.price.Minor() {
				res.Changes = append(res.Changes, Change{Field: "daily_price", From: b.DailyPrice.String(), To: to})
			}
			return res, nil
		}
		if overlaps(b.MinDays, b.MaxDays, r.min, r.max) {
			return PriceRowResult{}, rowErr("band %s overlaps the existing band %s", FormatBand(r.min, r.max), FormatBand(b.MinDays, b.MaxDays))
		}
	}
	res.Changes = []Change{{Field: "daily_price", From: "", To: to}}
	return res, nil
}

func sameMax(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func overlaps(minA int, maxA *int, minB int, maxB *int) bool {
	aEnd, bEnd := 1<<30, 1<<30
	if maxA != nil {
		aEnd = *maxA
	}
	if maxB != nil {
		bEnd = *maxB
	}
	return minA <= bEnd && minB <= aEnd
}

// ApplyPriceRow writes a row into the draft of the target plan (creating
// the plan / draft as needed). Previous holds what RevertPriceRow needs.
func (s *Service) ApplyPriceRow(ctx context.Context, orgID int64, in PriceRow) (PriceRowResult, error) {
	res := PriceRowResult{}
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		r, err := s.resolvePriceRow(ctx, q, orgID, in)
		if err != nil {
			return err
		}
		plans, err := targetPlans(ctx, q, orgID, r.target, r.priority, int8Arg(r.product.ID, true))
		if err != nil {
			return err
		}
		planCreated := false
		var draftID int64
		var planUUID uuid.UUID
		if plan, ok := plans[r.product.ID]; ok {
			row, err := s.lockPlan(ctx, q, orgID, plan.Uuid)
			if err != nil {
				return err
			}
			if draftID, _, err = ensurePlanDraft(ctx, q, orgID, row); err != nil {
				return err
			}
			planUUID = row.Uuid
		} else {
			head, err := q.CreateRatePlan(ctx, db.CreateRatePlanParams{
				OrganizationID: orgID, RentalProductID: r.product.ID, Scope: r.target.Scope, BranchID: r.target.BranchID,
				Channel: r.target.Channel,
			})
			if err != nil {
				return repository.MapErr(err)
			}
			v, err := q.CreateRatePlanVersion(ctx, db.CreateRatePlanVersionParams{
				OrganizationID: orgID, RatePlanID: head.ID, Version: 1, Priority: int32(r.priority), CreatedBy: actorArg(ctx),
				ChangeNote: "import",
			})
			if err != nil {
				return repository.MapErr(err)
			}
			draftID, planUUID, planCreated = v.ID, head.Uuid, true
		}
		prev := map[string]any{"plan_uuid": planUUID.String(), "min_days": r.min, "max_days": nil, "price": nil, "plan_created": planCreated}
		if r.max != nil {
			prev["max_days"] = *r.max
		}
		band, err := q.GetRateBand(ctx, db.GetRateBandParams{OrganizationID: orgID, RatePlanVersionID: draftID, MinDays: int32(r.min), MaxDays: int4Arg(r.max)})
		switch {
		case err == nil:
			prev["price"] = money.FormatNumeric(band.DailyPrice)
			if err := repository.Rows(q.UpdateRateBandPrice(ctx, db.UpdateRateBandPriceParams{OrganizationID: orgID, ID: band.ID, DailyPrice: r.price.Numeric()})); err != nil {
				return err
			}
			res.Op = "update"
		case errors.Is(repository.MapErr(err), repository.ErrNotFound):
			if err := q.InsertRateBand(ctx, db.InsertRateBandParams{
				OrganizationID: orgID, RatePlanVersionID: draftID, MinDays: int32(r.min), MaxDays: int4Arg(r.max), DailyPrice: r.price.Numeric(),
			}); err != nil {
				if errors.Is(repository.MapErr(err), repository.ErrBandOverlap) {
					return rowErr("band %s overlaps another band of the plan", FormatBand(r.min, r.max))
				}
				return repository.MapErr(err)
			}
			res.Op = "create"
		default:
			return err
		}
		res.PlanUUID, res.Previous = planUUID, prev
		return nil
	})
	return res, err
}

// RevertPriceRow undoes an applied row while the plan still has the draft:
// restores the old price, removes an inserted band, or deletes a plan the
// import created (when it was never activated).
func (s *Service) RevertPriceRow(ctx context.Context, orgID int64, previous map[string]any) error {
	planUUID, err := uuid.Parse(fmt.Sprint(previous["plan_uuid"]))
	if err != nil {
		return fmt.Errorf("pricing import revert: invalid plan uuid")
	}
	min, ok := toInt(previous["min_days"])
	if !ok {
		return fmt.Errorf("pricing import revert: invalid band")
	}
	var max *int
	if v, ok := toInt(previous["max_days"]); ok {
		max = &v
	}
	return s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockPlan(ctx, q, orgID, planUUID)
		if errors.Is(err, ErrPlanNotFound) {
			return nil // deleted since: nothing to undo
		}
		if err != nil {
			return err
		}
		if created, _ := previous["plan_created"].(bool); created && row.Status == model.StatusDraft {
			return repository.Rows(q.SoftDeleteRatePlan(ctx, db.SoftDeleteRatePlanParams{OrganizationID: orgID, ID: row.ID}))
		}
		draft, err := q.GetRatePlanVersionByStatus(ctx, db.GetRatePlanVersionByStatusParams{OrganizationID: orgID, RatePlanID: row.ID, Status: model.VersionDraft})
		if err != nil {
			if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
				return stateErr("activated", "the imported draft was already activated or discarded")
			}
			return err
		}
		if price, ok := previous["price"].(string); ok && price != "" {
			m, err := money.Parse(price)
			if err != nil {
				return err
			}
			band, err := q.GetRateBand(ctx, db.GetRateBandParams{OrganizationID: orgID, RatePlanVersionID: draft.ID, MinDays: int32(min), MaxDays: int4Arg(max)})
			if err != nil {
				return repository.MapErr(err)
			}
			return repository.Rows(q.UpdateRateBandPrice(ctx, db.UpdateRateBandPriceParams{OrganizationID: orgID, ID: band.ID, DailyPrice: m.Numeric()}))
		}
		_, err = q.DeleteRateBand(ctx, db.DeleteRateBandParams{OrganizationID: orgID, RatePlanVersionID: draft.ID, MinDays: int32(min), MaxDays: int4Arg(max)})
		return repository.MapErr(err)
	})
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}
