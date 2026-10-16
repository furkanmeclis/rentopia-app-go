package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

// MaxPreviewDays bounds the preview rental length.
const MaxPreviewDays = 366

// draftPlanID / draftRuleID name unsaved drafts in breakdowns.
const (
	draftPlanID = "draft"
	draftRuleID = "draft"
)

// activeEnginePlans loads the active plans of a product with their bands.
func activeEnginePlans(ctx context.Context, q *db.Queries, orgID, productID int64) ([]pricing.Plan, error) {
	rows, err := q.ListActiveRatePlans(ctx, db.ListActiveRatePlansParams{OrganizationID: orgID, RentalProductID: int8Arg(productID, true)})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.VersionID)
	}
	bands, err := bandsByVersion(ctx, q, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]pricing.Plan, 0, len(rows))
	for _, r := range rows {
		p := pricing.Plan{
			ID: r.Uuid.String(), VersionID: r.VersionUuid.String(), Version: int(r.Version), Name: r.Name,
			Scope: r.Scope, Priority: int(r.Priority), Bands: engineBands(bands[r.VersionID]),
		}
		if r.BranchUuid.Valid {
			p.BranchID = uuid.UUID(r.BranchUuid.Bytes).String()
		}
		if r.Channel.Valid {
			p.Channel = r.Channel.String
		}
		if r.AgreementUuid.Valid {
			p.AgreementID = uuid.UUID(r.AgreementUuid.Bytes).String()
		}
		out = append(out, p)
	}
	return out, nil
}

func samePlanTarget(a, b pricing.Plan) bool {
	return a.Scope == b.Scope && a.BranchID == b.BranchID && a.Channel == b.Channel && a.Priority == b.Priority
}

// Preview prices one rental twice: with the active plans and rules
// (current) and with the given draft applied (proposed).
func (s *Service) Preview(ctx context.Context, orgID int64, in model.PreviewInput) (model.Preview, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	var product db.GetPricingProductByUUIDRow
	if in.RentalProductUUID == nil {
		ve.add("rental_product_uuid", "required", "rental_product_uuid is required")
	} else if p, err := q.GetPricingProductByUUID(ctx, db.GetPricingProductByUUIDParams{OrganizationID: orgID, Uuid: *in.RentalProductUUID}); err != nil {
		ve.add("rental_product_uuid", "not_found", "rental product was not found")
	} else {
		product = p
	}
	var branch db.Branch
	if in.BranchUUID == nil {
		ve.add("branch_uuid", "required", "branch_uuid is required")
	} else if b, err := q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: *in.BranchUUID}); err != nil || orgctx.CheckBranch(ctx, b.ID) != nil {
		ve.add("branch_uuid", "not_found", "branch was not found")
	} else {
		branch = b
	}
	var returnBranch *db.Branch
	if in.ReturnBranchUUID != nil {
		if b, err := q.GetBranchByUUID(ctx, db.GetBranchByUUIDParams{OrganizationID: orgID, Uuid: *in.ReturnBranchUUID}); err != nil {
			ve.add("return_branch_uuid", "not_found", "branch was not found")
		} else {
			returnBranch = &b
		}
	}
	if !validChannel(in.Channel) {
		ve.add("channel", "invalid", "channel is not a sales channel")
	}
	var agreement *db.GetAgreementForSaleRow
	if in.AgreementUUID != nil {
		a, err := q.GetAgreementForSale(ctx, db.GetAgreementForSaleParams{OrganizationID: orgID, Uuid: *in.AgreementUUID})
		if err != nil {
			ve.add("agreement_uuid", "not_found", "agreement was not found")
		} else {
			agreement = &a
		}
	}
	var rng rentaltime.Range
	switch {
	case in.StartAt == nil:
		ve.add("start_at", "required", "start_at is required")
	case in.EndAt == nil:
		ve.add("end_at", "required", "end_at is required")
	default:
		r, err := rentaltime.NewRange(*in.StartAt, *in.EndAt)
		if err != nil {
			ve.add("end_at", "before_start", "end_at must be after start_at")
		} else if r.BillingDays() > MaxPreviewDays {
			ve.add("end_at", "too_long", "the preview covers at most 366 days")
		} else {
			rng = r
		}
	}
	var delivery *money.Money
	if in.DeliveryFee != nil && *in.DeliveryFee != "" {
		if m, ok := parsePrice(ve, "delivery_fee", *in.DeliveryFee); ok {
			delivery = &m
		}
	}
	var discount *pricing.Discount
	if d := in.Discount; d != nil {
		discount = &pricing.Discount{Code: "preview", Name: d.Name, Kind: d.Kind}
		switch d.Kind {
		case pricing.KindPercent:
			if r, err := money.ParseRate(string(d.Value)); err != nil || r < 0 || r > money.Percent(100) {
				ve.add("discount.value", "invalid", "a discount percentage must be between 0 and 100")
			} else {
				discount.Percent = r
			}
		case pricing.KindAmount:
			if m, ok := parsePrice(ve, "discount.value", d.Value); ok {
				discount.Amount = m
			}
		default:
			ve.add("discount.kind", "invalid", "kind must be percent or amount")
		}
	}
	if strings.TrimSpace(in.CouponCode) != "" && in.Discount != nil {
		ve.add("coupon_code", "not_allowed", "send a coupon code or a manual discount, not both")
	}
	if len([]rune(in.CouponCode)) > 64 {
		ve.add("coupon_code", "too_long", "coupon_code is too long")
	}
	var extras []pricing.Extra
	if len(in.Extras) > 0 {
		switch {
		case len(in.Extras) > 50:
			ve.add("extras", "too_many", "at most 50 extras")
		case s.extras == nil:
			ve.add("extras", "unavailable", "extras are not available")
		case product.ID != 0:
			lines, details, err := s.extras.ResolveExtras(ctx, orgID, product.ID, in.Channel, in.Extras, s.clock())
			if err != nil {
				return model.Preview{}, err
			}
			ve.Details = append(ve.Details, details...)
			extras = lines
		}
	}
	if in.RatePlanUUID != nil && in.RatePlanDraft != nil {
		ve.add("rate_plan_draft", "not_allowed", "send rate_plan_uuid or rate_plan_draft, not both")
	}
	if in.DateRuleUUID != nil && in.DateRuleDraft != nil {
		ve.add("date_rule_draft", "not_allowed", "send date_rule_uuid or date_rule_draft, not both")
	}

	// Proposed changes (validated like a save).
	var planOverride *pricing.Plan
	replacePlanID := ""
	if in.RatePlanUUID != nil {
		row, err := s.getPlan(ctx, q, orgID, *in.RatePlanUUID)
		switch {
		case errors.Is(err, ErrPlanNotFound):
			ve.add("rate_plan_uuid", "not_found", "rate plan was not found")
		case err != nil:
			return model.Preview{}, err
		case row.RentalProductID != product.ID && product.ID != 0:
			ve.add("rate_plan_uuid", "other_product", "the rate plan belongs to another rental product")
		default:
			v, _, err := planVersion(ctx, q, orgID, row.ID, model.VersionDraft)
			if err != nil {
				return model.Preview{}, err
			}
			if v == nil {
				v, _, err = planVersion(ctx, q, orgID, row.ID, model.VersionActive)
				if err != nil {
					return model.Preview{}, err
				}
			}
			if v != nil {
				p := pricing.Plan{
					ID: row.Uuid.String(), VersionID: v.UUID.String(), Version: v.Version, Name: row.Name, Scope: row.Scope,
					Priority: v.Priority, Bands: engineBands(v.Bands),
				}
				if row.BranchUuid.Valid {
					p.BranchID = uuid.UUID(row.BranchUuid.Bytes).String()
				}
				if row.Channel.Valid {
					p.Channel = row.Channel.String
				}
				planOverride, replacePlanID = &p, p.ID
			}
		}
	}
	if d := in.RatePlanDraft; d != nil {
		t := s.resolveTarget(ctx, q, orgID, ve, "rate_plan_draft.", d.Scope, d.BranchUUID, d.Channel)
		priority := checkPriority(ve, "rate_plan_draft.priority", d.Priority)
		bands := parseBands(ve, "rate_plan_draft.", d.Bands)
		p := pricing.Plan{ID: draftPlanID, VersionID: draftPlanID, Name: "draft", Scope: t.Scope, Priority: priority, Bands: engineBands(bands)}
		if t.Branch != nil {
			p.BranchID = t.Branch.UUID.String()
		}
		if t.Channel.Valid {
			p.Channel = t.Channel.String
		}
		planOverride = &p
	}
	var ruleOverride *pricing.DateRule
	replaceRuleID := ""
	if in.DateRuleUUID != nil {
		row, err := s.getRule(ctx, q, orgID, *in.DateRuleUUID)
		switch {
		case errors.Is(err, ErrRuleNotFound):
			ve.add("date_rule_uuid", "not_found", "date rule was not found")
		case err != nil:
			return model.Preview{}, err
		default:
			_, v, err := ruleVersion(ctx, q, orgID, row.ID, model.VersionDraft)
			if err != nil {
				return model.Preview{}, err
			}
			if v == nil {
				if _, v, err = ruleVersion(ctx, q, orgID, row.ID, model.VersionActive); err != nil {
					return model.Preview{}, err
				}
			}
			if v != nil {
				targets, err := ruleTargets(ctx, q, orgID, []int64{v.ID})
				if err != nil {
					return model.Preview{}, err
				}
				r := engineRule(row.Uuid, row.Name, v.Uuid, v.Version, stateFromVersion(*v, targets[v.ID]))
				ruleOverride, replaceRuleID = &r, r.ID
			}
		}
	}
	if d := in.DateRuleDraft; d != nil {
		st := defaultRuleState()
		s.applyRuleFields(ctx, q, orgID, ve, "date_rule_draft.", &st, d.DateRuleFields, true)
		name := d.Name
		if name == "" {
			name = "draft"
		}
		r := engineRule(uuid.Nil, name, uuid.Nil, 0, st)
		r.ID, r.VersionID = draftRuleID, draftRuleID
		ruleOverride = &r
	}
	if err := ve.orNil(); err != nil {
		return model.Preview{}, err
	}

	cfg, warnings, err := s.orgPricing(ctx, orgID)
	if err != nil {
		return model.Preview{}, err
	}
	plans, err := activeEnginePlans(ctx, q, orgID, product.ID)
	if err != nil {
		return model.Preview{}, err
	}
	days := rng.BillingDayDates()
	from, to := days[0], days[len(days)-1].AddDays(1)
	rules, err := activeEngineRules(ctx, q, orgID, &from, &to)
	if err != nil {
		return model.Preview{}, err
	}
	base := pricing.Input{
		Start: rng.Start, End: rng.End, AgreementID: agreementID(agreement),
		Product:  pricing.Product{ID: product.Uuid.String(), SegmentID: product.SegmentUuid.String()},
		BranchID: branch.Uuid.String(), Channel: in.Channel, Discount: discount, DeliveryFee: delivery,
		Deliveries: in.Deliveries, Extras: extras, VATRate: cfg.VAT, Prepayment: cfg.Prepayment,
	}
	var coupon *CouponQuery
	// Coupon eligibility of a corporate agreement (TEC-723): without it no
	// campaign applies on top of the agreement; with it only campaigns
	// marked corporate_eligible do (campaigns.Evaluate, reason corporate).
	couponsBlocked := agreement != nil && !agreement.CouponEligible
	if s.coupons != nil && in.Discount == nil && !couponsBlocked && (strings.TrimSpace(in.CouponCode) != "" || in.CustomerUUID != nil) {
		coupon = &CouponQuery{
			Code: strings.TrimSpace(in.CouponCode), ProductID: product.ID, SegmentID: product.SegmentID,
			Channel: in.Channel, CustomerUUID: in.CustomerUUID, At: s.clock(), Corporate: agreement != nil,
		}
	} else if s.coupons == nil && strings.TrimSpace(in.CouponCode) != "" {
		warnings = append(warnings, "coupons_unavailable")
	}
	if returnBranch != nil && returnBranch.ID != branch.ID {
		if s.oneWay == nil {
			warnings = append(warnings, "one_way_unavailable")
		} else {
			allowed, fee, err := s.oneWay.ResolveOneWayFee(ctx, orgID, branch.Uuid, returnBranch.Uuid, product.Uuid)
			if err != nil {
				return model.Preview{}, err
			}
			if !allowed {
				warnings = append(warnings, "one_way_not_allowed")
			} else {
				base.OneWayFee = &fee
			}
		}
	}

	current := base
	current.Plans, current.Rules = plans, rules
	proposed := base
	proposed.Plans, proposed.Rules = plans, rules
	if planOverride != nil {
		next := []pricing.Plan{}
		for _, p := range plans {
			if p.ID == replacePlanID || (replacePlanID == "" && samePlanTarget(p, *planOverride)) {
				continue
			}
			next = append(next, p)
		}
		proposed.Plans = append(next, *planOverride)
	}
	if ruleOverride != nil {
		next := []pricing.DateRule{}
		for _, r := range rules {
			if r.ID != replaceRuleID {
				next = append(next, r)
			}
		}
		proposed.Rules = append(next, *ruleOverride)
	}
	if planOverride == nil && ruleOverride == nil {
		warnings = append(warnings, "no_changes")
	}
	cur, err := s.side(ctx, orgID, current, coupon)
	if err != nil {
		return model.Preview{}, err
	}
	prop, err := s.side(ctx, orgID, proposed, coupon)
	if err != nil {
		return model.Preview{}, err
	}
	if couponsBlocked && strings.TrimSpace(in.CouponCode) != "" {
		blocked := &model.CouponResult{
			Code: strings.ToUpper(strings.TrimSpace(in.CouponCode)), Status: "rejected", Reason: "corporate", Params: map[string]string{},
		}
		cur.Coupon, prop.Coupon = blocked, blocked
	}
	cur.Agreement, prop.Agreement = agreementPricing(agreement, cur.Breakdown), agreementPricing(agreement, prop.Breakdown)
	out := model.Preview{Current: cur, Proposed: prop, Warnings: warnings}
	if out.Current.Breakdown != nil && out.Proposed.Breakdown != nil {
		d := out.Proposed.Breakdown.Total.Sub(out.Current.Breakdown.Total)
		out.Difference = &d
	}
	return out, nil
}

// side prices one variant. With a coupon query the rental subtotal of a
// first pass decides the campaign (minimum amount, percent base); the
// applied campaign is the single discount of the second pass.
func (s *Service) side(ctx context.Context, orgID int64, in pricing.Input, coupon *CouponQuery) (model.PreviewSide, error) {
	if coupon == nil {
		return calculateSide(in), nil
	}
	first := calculateSide(in)
	if first.Breakdown == nil {
		return first, nil
	}
	q := *coupon
	q.Subtotal = first.Breakdown.RentalSubtotal
	discount, result, err := s.coupons.EvaluateCoupon(ctx, orgID, q)
	if err != nil {
		return model.PreviewSide{}, err
	}
	if discount == nil {
		first.Coupon = result
		return first, nil
	}
	in.Discount = discount
	out := calculateSide(in)
	out.Coupon = result
	return out, nil
}

func calculateSide(in pricing.Input) model.PreviewSide {
	b, err := pricing.Calculate(in)
	if err != nil {
		pe := &model.PreviewError{Code: pricing.Code(err), Message: err.Error(), Refs: []string{}}
		var e *pricing.Error
		if errors.As(err, &e) {
			pe.Refs = append(pe.Refs, e.IDs...)
		}
		if pe.Code == "" {
			pe.Code = "error"
		}
		return model.PreviewSide{Error: pe}
	}
	return model.PreviewSide{Breakdown: &b}
}

func agreementID(a *db.GetAgreementForSaleRow) string {
	if a == nil {
		return ""
	}
	return a.Uuid.String()
}

func agreementPricing(a *db.GetAgreementForSaleRow, b *pricing.Breakdown) *model.AgreementPricing {
	if a == nil {
		return nil
	}
	return &model.AgreementPricing{
		UUID: a.Uuid, Code: a.Code, Name: a.Name, CouponEligible: a.CouponEligible, InvoicePeriod: a.InvoicePeriod,
		TariffApplied: b != nil && b.Plan.Scope == pricing.ScopeAgreement && b.Plan.AgreementID == a.Uuid.String(),
	}
}

// Quote prices a rental with the active plans and rules only (M3
// reservations call it; the preview's current side is the same path).
func (s *Service) Quote(ctx context.Context, orgID int64, in model.PreviewInput) (model.PreviewSide, error) {
	in.RatePlanUUID, in.RatePlanDraft, in.DateRuleUUID, in.DateRuleDraft = nil, nil, nil, nil
	p, err := s.Preview(ctx, orgID, in)
	if err != nil {
		return model.PreviewSide{}, err
	}
	return p.Current, nil
}
