// Package pricing is the pure rental pricing engine (TEC-699). Calculate
// turns a fully resolved Input (candidate rate plans and date rules already
// loaded by the caller) into a Breakdown. It never touches the database, the
// clock or the request context, so the same input always yields the same
// breakdown (reservations store it; the preview compares two of them).
//
// Rules (beta doc §6, §19.2):
//
//  1. Billing days = ceil(duration / 24h), min 1 (rentaltime).
//  2. Rate plan: among the candidates that match the branch / channel, the
//     most specific scope wins (branch+channel → branch → channel →
//     common); inside that scope the highest priority wins; two plans with
//     the same scope and priority are ErrAmbiguousPlan (activation prevents
//     it; the engine still refuses to guess).
//  3. Duration band: chosen once from the WHOLE rental length; every day
//     uses that band's daily price.
//  4. Date rules: for each billing day, by the Istanbul date the day starts
//     on, the single highest-priority matching rule adjusts that day's unit
//     price (± percent or ± amount); rule ranges are [start, end). Two
//     matching rules with the same top priority are ErrAmbiguousRule. A day
//     price never goes below zero.
//  5. One campaign / coupon discount on the rental subtotal (input hook;
//     coupons arrive with TEC-703); the rental total never goes negative.
//  6. Extras, one-way fee and delivery fee are lines from the input.
//  7. VAT: every price is VAT-inclusive. Each line (rental, each extra, each
//     fee) is split into base (matrah) and VAT with the organization's rate:
//     base = round(gross / (1 + rate)), VAT = gross - base; the totals are
//     the sums of the lines, so base + VAT = total to the kuruş.
//  8. Prepayment from the settings rule: none, fixed (capped at the total),
//     percent of the total, or full.
//
// All rounding is money's single rule: half away from zero to the kuruş.
package pricing

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

// Plan scopes, from the least to the most specific.
const (
	ScopeCommon        = "common"
	ScopeChannel       = "channel"
	ScopeBranch        = "branch"
	ScopeBranchChannel = "branch_channel"
)

// Scopes lists the plan scopes (API enum order).
var Scopes = []string{ScopeCommon, ScopeBranch, ScopeChannel, ScopeBranchChannel}

// ScopeRank orders the scopes by precedence (higher wins).
func ScopeRank(scope string) int {
	switch scope {
	case ScopeBranchChannel:
		return 4
	case ScopeBranch:
		return 3
	case ScopeChannel:
		return 2
	case ScopeCommon:
		return 1
	}
	return 0
}

// Date rule kinds.
const (
	KindPercent = "percent"
	KindAmount  = "amount"
)

// Date rule product scopes.
const (
	ProductsAll      = "all"
	ProductsSegments = "segments"
	ProductsSelected = "products"
)

// Prepayment rules.
const (
	PrepaymentNone    = "none"
	PrepaymentFixed   = "fixed"
	PrepaymentPercent = "percent"
	PrepaymentFull    = "full"
)

// Line kinds of the VAT split.
const (
	LineRental   = "rental"
	LineExtra    = "extra"
	LineOneWay   = "one_way"
	LineDelivery = "delivery"
)

// Typed errors (the API maps them to error codes).
var (
	ErrInvalidInput  = errors.New("pricing: invalid input")
	ErrNoPlan        = errors.New("pricing: no rate plan matches")
	ErrAmbiguousPlan = errors.New("pricing: two rate plans with the same scope and priority match")
	ErrNoBand        = errors.New("pricing: no duration band covers the rental length")
	ErrAmbiguousBand = errors.New("pricing: two duration bands cover the rental length")
	ErrAmbiguousRule = errors.New("pricing: two date rules with the same priority match a day")
)

// Error wraps a typed error with the records involved.
type Error struct {
	Err     error
	Message string
	// IDs are the plan / rule ids involved (ambiguity).
	IDs []string
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

func fail(err error, msg string, ids ...string) error {
	return &Error{Err: err, Message: msg, IDs: ids}
}

// Code returns the stable error code of a pricing error ("" for others).
func Code(err error) string {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, ErrNoPlan):
		return "no_plan"
	case errors.Is(err, ErrAmbiguousPlan):
		return "ambiguous_plan"
	case errors.Is(err, ErrNoBand):
		return "no_band"
	case errors.Is(err, ErrAmbiguousBand):
		return "ambiguous_band"
	case errors.Is(err, ErrAmbiguousRule):
		return "ambiguous_rule"
	}
	return ""
}

// Band is a duration band: MinDays..MaxDays inclusive (MaxDays nil = open).
type Band struct {
	MinDays    int         `json:"min_days"`
	MaxDays    *int        `json:"max_days"`
	DailyPrice money.Money `json:"daily_price"`
}

// Covers reports whether days falls in the band.
func (b Band) Covers(days int) bool {
	return days >= b.MinDays && (b.MaxDays == nil || days <= *b.MaxDays)
}

// Plan is one candidate rate plan version of the product.
type Plan struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id"`
	Version   int    `json:"version"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	BranchID  string `json:"branch_id,omitempty"`
	Channel   string `json:"channel,omitempty"`
	Priority  int    `json:"priority"`
	Bands     []Band `json:"bands"`
}

// Matches reports whether the plan applies to a sale at branch through
// channel.
func (p Plan) Matches(branchID, channel string) bool {
	switch p.Scope {
	case ScopeCommon:
		return true
	case ScopeBranch:
		return p.BranchID == branchID
	case ScopeChannel:
		return p.Channel == channel
	case ScopeBranchChannel:
		return p.BranchID == branchID && p.Channel == channel
	}
	return false
}

// DateRule is one candidate date rule version.
type DateRule struct {
	ID        string               `json:"id"`
	VersionID string               `json:"version_id"`
	Version   int                  `json:"version"`
	Name      string               `json:"name"`
	Range     rentaltime.DateRange `json:"range"`
	// ProductScope is all, segments (SegmentIDs) or products (ProductIDs).
	ProductScope string   `json:"product_scope"`
	ProductIDs   []string `json:"product_ids,omitempty"`
	SegmentIDs   []string `json:"segment_ids,omitempty"`
	// BranchIDs / Channels nil = every branch / channel.
	BranchIDs []string `json:"branch_ids,omitempty"`
	Channels  []string `json:"channels,omitempty"`
	Kind      string   `json:"kind"`
	// Percent is the signed percentage of a percent rule, Amount the signed
	// amount of an amount rule.
	Percent  money.Rate  `json:"percent"`
	Amount   money.Money `json:"amount"`
	Priority int         `json:"priority"`
}

// MatchesTarget reports whether the rule applies to the product / branch /
// channel (dates aside).
func (r DateRule) MatchesTarget(p Product, branchID, channel string) bool {
	switch r.ProductScope {
	case ProductsAll:
	case ProductsSegments:
		if !contains(r.SegmentIDs, p.SegmentID) {
			return false
		}
	case ProductsSelected:
		if !contains(r.ProductIDs, p.ID) {
			return false
		}
	default:
		return false
	}
	if r.BranchIDs != nil && !contains(r.BranchIDs, branchID) {
		return false
	}
	if r.Channels != nil && !contains(r.Channels, channel) {
		return false
	}
	return true
}

// Apply returns the adjusted unit price (not clamped).
func (r DateRule) Apply(base money.Money) money.Money {
	if r.Kind == KindPercent {
		return base.Add(base.MulRate(r.Percent))
	}
	return base.Add(r.Amount)
}

// Product is the rental product (alt grup) being priced.
type Product struct {
	ID        string `json:"id"`
	SegmentID string `json:"segment_id"`
}

// Discount is the single campaign / coupon on the rental subtotal.
type Discount struct {
	// CampaignID is the campaign uuid (empty for a manual discount).
	CampaignID string      `json:"campaign_id,omitempty"`
	Code       string      `json:"code"`
	Name       string      `json:"name"`
	Kind       string      `json:"kind"`
	Percent    money.Rate  `json:"percent"`
	Amount     money.Money `json:"amount"`
	// MaxAmount caps the discount (nil = no cap besides the subtotal).
	MaxAmount *money.Money `json:"max_amount,omitempty"`
}

// Extra is an extra service line (child seat, additional driver …).
type Extra struct {
	Code      string      `json:"code"`
	Name      string      `json:"name"`
	UnitPrice money.Money `json:"unit_price"`
	Quantity  int         `json:"quantity"`
	// PerDay multiplies by the billing days.
	PerDay bool `json:"per_day"`
	// MaxTotal caps the line total (nil = no cap).
	MaxTotal *money.Money `json:"max_total,omitempty"`
	// ExtraID / PriceID name the catalog extra and the effective-dated
	// price row used (a quote snapshots them).
	ExtraID string `json:"extra_id,omitempty"`
	PriceID string `json:"price_id,omitempty"`
}

// Prepayment is the organization's prepayment rule.
type Prepayment struct {
	Rule    string      `json:"rule"`
	Percent money.Rate  `json:"percent"`
	Amount  money.Money `json:"amount"`
}

// Input is everything one calculation needs.
type Input struct {
	Start    time.Time
	End      time.Time
	Product  Product
	BranchID string
	Channel  string
	// Plans are the candidate (active) plan versions of the product; the
	// engine picks one.
	Plans []Plan
	// Rules are the candidate (active) date rule versions; the engine
	// matches them per day.
	Rules    []DateRule
	Discount *Discount
	Extras   []Extra
	// OneWayFee is the resolved one-way rule fee (nil = same branch / none).
	OneWayFee *money.Money
	// DeliveryFee is the delivery / collection fee (nil = none).
	DeliveryFee *money.Money
	// Deliveries are named delivery / collection fees of service points
	// (TEC-724), one line each, after DeliveryFee.
	Deliveries []DeliveryFee
	// VATRate is the organization's VAT rate; nil = not configured (the
	// breakdown then carries no VAT split: base = gross, VAT = 0).
	VATRate    *money.Rate
	Prepayment Prepayment
}

// DeliveryFee is one delivery or collection fee: the service point's
// delivery extra price (ExtraID / PriceID set) or its flat fee.
type DeliveryFee struct {
	Code    string
	Name    string
	Fee     money.Money
	ExtraID string
	PriceID string
}

// PlanRef names the plan version a breakdown used.
type PlanRef struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id"`
	Version   int    `json:"version"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	Priority  int    `json:"priority"`
}

// RuleRef names the date rule version applied to a day.
type RuleRef struct {
	ID        string       `json:"id"`
	VersionID string       `json:"version_id"`
	Version   int          `json:"version"`
	Name      string       `json:"name"`
	Kind      string       `json:"kind"`
	Percent   *money.Rate  `json:"percent,omitempty"`
	Amount    *money.Money `json:"amount,omitempty"`
	Priority  int          `json:"priority"`
}

// DayLine is one billing day.
type DayLine struct {
	Index int `json:"index"`
	// Date is the Istanbul date the day starts on.
	Date       rentaltime.Date `json:"date"`
	StartsAt   time.Time       `json:"starts_at"`
	BasePrice  money.Money     `json:"base_price"`
	Rule       *RuleRef        `json:"rule"`
	Adjustment money.Money     `json:"adjustment"`
	UnitPrice  money.Money     `json:"unit_price"`
}

// DiscountLine is the applied discount.
type DiscountLine struct {
	CampaignID string `json:"campaign_id,omitempty"`
	// Capped is true when MaxAmount limited the discount.
	Capped  bool        `json:"capped,omitempty"`
	Code    string      `json:"code"`
	Name    string      `json:"name"`
	Kind    string      `json:"kind"`
	Percent *money.Rate `json:"percent,omitempty"`
	Amount  money.Money `json:"amount"`
}

// Line is one VAT-split line of the breakdown.
type Line struct {
	Kind     string `json:"kind"`
	Code     string `json:"code,omitempty"`
	Name     string `json:"name,omitempty"`
	Quantity int    `json:"quantity"`
	// UnitPrice is the unit price of an extra line.
	UnitPrice *money.Money `json:"unit_price,omitempty"`
	// Capped is true when the extra's MaxTotal limited the line.
	Capped  bool        `json:"capped,omitempty"`
	ExtraID string      `json:"extra_id,omitempty"`
	PriceID string      `json:"price_id,omitempty"`
	Gross   money.Money `json:"gross"`
	Base    money.Money `json:"base"`
	VAT     money.Money `json:"vat"`
}

// Breakdown is the full, explainable result.
type Breakdown struct {
	Currency    money.Currency `json:"currency"`
	StartsAt    time.Time      `json:"starts_at"`
	EndsAt      time.Time      `json:"ends_at"`
	BillingDays int            `json:"billing_days"`
	Plan        PlanRef        `json:"plan"`
	Band        Band           `json:"band"`
	Days        []DayLine      `json:"days"`
	// Rules lists the distinct rule versions applied, in first-use order.
	Rules          []RuleRef     `json:"rules"`
	RentalSubtotal money.Money   `json:"rental_subtotal"`
	Discount       *DiscountLine `json:"discount"`
	RentalTotal    money.Money   `json:"rental_total"`
	Lines          []Line        `json:"lines"`
	VATRate        *money.Rate   `json:"vat_rate"`
	BaseTotal      money.Money   `json:"base_total"`
	VATTotal       money.Money   `json:"vat_total"`
	Total          money.Money   `json:"total"`
	Prepayment     money.Money   `json:"prepayment"`
	PrepaymentRule string        `json:"prepayment_rule"`
}

// Calculate prices one rental.
func Calculate(in Input) (Breakdown, error) {
	rng, err := rentaltime.NewRange(in.Start, in.End)
	if err != nil {
		return Breakdown{}, fail(ErrInvalidInput, "end must be after start")
	}
	if in.Product.ID == "" {
		return Breakdown{}, fail(ErrInvalidInput, "product is required")
	}
	days := rng.BillingDays()

	plan, err := SelectPlan(in.Plans, in.BranchID, in.Channel)
	if err != nil {
		return Breakdown{}, err
	}
	band, err := SelectBand(plan.Bands, days)
	if err != nil {
		return Breakdown{}, err
	}

	out := Breakdown{
		Currency: band.DailyPrice.Currency(), StartsAt: rng.Start, EndsAt: rng.End, BillingDays: days,
		Plan: PlanRef{
			ID: plan.ID, VersionID: plan.VersionID, Version: plan.Version, Name: plan.Name,
			Scope: plan.Scope, Priority: plan.Priority,
		},
		Band:  band,
		Days:  make([]DayLine, 0, days),
		Rules: []RuleRef{},
		Lines: []Line{},
	}

	// Candidate rules for this product / branch / channel.
	candidates := make([]DateRule, 0, len(in.Rules))
	for _, r := range in.Rules {
		if r.MatchesTarget(in.Product, in.BranchID, in.Channel) {
			candidates = append(candidates, r)
		}
	}

	subtotal := money.Zero()
	seen := map[string]bool{}
	starts := rng.BillingDayStarts()
	for i, s := range starts {
		date := rentaltime.LocalDate(s)
		line := DayLine{Index: i + 1, Date: date, StartsAt: s, BasePrice: band.DailyPrice, UnitPrice: band.DailyPrice, Adjustment: money.Zero()}
		rule, err := ruleForDay(candidates, date)
		if err != nil {
			return Breakdown{}, err
		}
		if rule != nil {
			ref := ruleRef(*rule)
			line.Rule = &ref
			line.UnitPrice = rule.Apply(band.DailyPrice).ClampZero()
			line.Adjustment = line.UnitPrice.Sub(band.DailyPrice)
			if !seen[rule.VersionID+"|"+rule.ID] {
				seen[rule.VersionID+"|"+rule.ID] = true
				out.Rules = append(out.Rules, ref)
			}
		}
		subtotal = subtotal.Add(line.UnitPrice)
		out.Days = append(out.Days, line)
	}
	out.RentalSubtotal = subtotal

	rental := subtotal
	if d := in.Discount; d != nil {
		var amount money.Money
		dl := &DiscountLine{CampaignID: d.CampaignID, Code: d.Code, Name: d.Name, Kind: d.Kind}
		switch d.Kind {
		case KindPercent:
			pct := d.Percent
			dl.Percent = &pct
			amount = subtotal.MulRate(d.Percent)
		case KindAmount:
			amount = d.Amount
		default:
			return Breakdown{}, fail(ErrInvalidInput, "discount kind must be percent or amount")
		}
		if amount.IsNegative() {
			return Breakdown{}, fail(ErrInvalidInput, "discount must not be negative")
		}
		if d.MaxAmount != nil {
			if d.MaxAmount.IsNegative() {
				return Breakdown{}, fail(ErrInvalidInput, "discount cap must not be negative")
			}
			if amount.Cmp(*d.MaxAmount) > 0 {
				amount, dl.Capped = *d.MaxAmount, true
			}
		}
		amount = amount.Min(subtotal)
		dl.Amount = amount
		out.Discount = dl
		rental = subtotal.Sub(amount).ClampZero()
	}
	out.RentalTotal = rental

	out.VATRate = in.VATRate
	addLine := func(l Line) {
		l.Base, l.VAT = SplitVAT(l.Gross, in.VATRate)
		out.Lines = append(out.Lines, l)
	}
	addLine(Line{Kind: LineRental, Quantity: days, Gross: rental})
	for _, e := range in.Extras {
		if e.Quantity <= 0 || e.UnitPrice.IsNegative() {
			return Breakdown{}, fail(ErrInvalidInput, fmt.Sprintf("extra %q needs a positive quantity and a non-negative price", e.Code))
		}
		gross := e.UnitPrice.Mul(int64(e.Quantity))
		if e.PerDay {
			gross = gross.Mul(int64(days))
		}
		unit := e.UnitPrice
		line := Line{Kind: LineExtra, Code: e.Code, Name: e.Name, Quantity: e.Quantity, UnitPrice: &unit, ExtraID: e.ExtraID, PriceID: e.PriceID}
		if e.MaxTotal != nil {
			if e.MaxTotal.IsNegative() {
				return Breakdown{}, fail(ErrInvalidInput, fmt.Sprintf("extra %q needs a non-negative cap", e.Code))
			}
			if gross.Cmp(*e.MaxTotal) > 0 {
				gross, line.Capped = *e.MaxTotal, true
			}
		}
		line.Gross = gross
		addLine(line)
	}
	if in.OneWayFee != nil && !in.OneWayFee.IsZero() {
		if in.OneWayFee.IsNegative() {
			return Breakdown{}, fail(ErrInvalidInput, "one-way fee must not be negative")
		}
		addLine(Line{Kind: LineOneWay, Quantity: 1, Gross: *in.OneWayFee})
	}
	if in.DeliveryFee != nil && !in.DeliveryFee.IsZero() {
		if in.DeliveryFee.IsNegative() {
			return Breakdown{}, fail(ErrInvalidInput, "delivery fee must not be negative")
		}
		addLine(Line{Kind: LineDelivery, Quantity: 1, Gross: *in.DeliveryFee})
	}
	for _, d := range in.Deliveries {
		if d.Fee.IsZero() {
			continue
		}
		if d.Fee.IsNegative() {
			return Breakdown{}, fail(ErrInvalidInput, "delivery fee must not be negative")
		}
		unit := d.Fee
		addLine(Line{Kind: LineDelivery, Code: d.Code, Name: d.Name, Quantity: 1, UnitPrice: &unit, ExtraID: d.ExtraID, PriceID: d.PriceID, Gross: d.Fee})
	}
	out.BaseTotal, out.VATTotal, out.Total = money.Zero(), money.Zero(), money.Zero()
	for _, l := range out.Lines {
		out.BaseTotal = out.BaseTotal.Add(l.Base)
		out.VATTotal = out.VATTotal.Add(l.VAT)
		out.Total = out.Total.Add(l.Gross)
	}

	pre, err := PrepaymentAmount(in.Prepayment, out.Total)
	if err != nil {
		return Breakdown{}, err
	}
	out.Prepayment = pre
	out.PrepaymentRule = in.Prepayment.Rule
	if out.PrepaymentRule == "" {
		out.PrepaymentRule = PrepaymentNone
	}
	return out, nil
}

// SelectPlan picks the plan for a sale at branch through channel.
func SelectPlan(plans []Plan, branchID, channel string) (Plan, error) {
	bestRank := 0
	var best []Plan
	for _, p := range plans {
		if !p.Matches(branchID, channel) {
			continue
		}
		rank := ScopeRank(p.Scope)
		switch {
		case rank > bestRank:
			bestRank, best = rank, []Plan{p}
		case rank == bestRank:
			best = append(best, p)
		}
	}
	if len(best) == 0 {
		return Plan{}, fail(ErrNoPlan, "no active rate plan matches the product, branch and channel")
	}
	sort.SliceStable(best, func(i, j int) bool { return best[i].Priority > best[j].Priority })
	if len(best) > 1 && best[0].Priority == best[1].Priority {
		ids := []string{}
		for _, p := range best {
			if p.Priority == best[0].Priority {
				ids = append(ids, p.ID)
			}
		}
		return Plan{}, fail(ErrAmbiguousPlan, "two rate plans with the same scope and priority match", ids...)
	}
	return best[0], nil
}

// SelectBand picks the band covering days.
func SelectBand(bands []Band, days int) (Band, error) {
	var found []Band
	for _, b := range bands {
		if b.Covers(days) {
			found = append(found, b)
		}
	}
	switch len(found) {
	case 0:
		return Band{}, fail(ErrNoBand, fmt.Sprintf("no duration band covers %d days", days))
	case 1:
		return found[0], nil
	}
	return Band{}, fail(ErrAmbiguousBand, fmt.Sprintf("%d duration bands cover %d days", len(found), days))
}

func ruleForDay(rules []DateRule, d rentaltime.Date) (*DateRule, error) {
	var best *DateRule
	tie := false
	for i := range rules {
		r := &rules[i]
		if !r.Range.Contains(d) {
			continue
		}
		switch {
		case best == nil || r.Priority > best.Priority:
			best, tie = r, false
		case r.Priority == best.Priority:
			tie = true
		}
	}
	if tie {
		ids := []string{}
		for _, r := range rules {
			if r.Range.Contains(d) && r.Priority == best.Priority {
				ids = append(ids, r.ID)
			}
		}
		return nil, fail(ErrAmbiguousRule, fmt.Sprintf("two date rules with priority %d match %s", best.Priority, d), ids...)
	}
	return best, nil
}

func ruleRef(r DateRule) RuleRef {
	ref := RuleRef{ID: r.ID, VersionID: r.VersionID, Version: r.Version, Name: r.Name, Kind: r.Kind, Priority: r.Priority}
	if r.Kind == KindPercent {
		p := r.Percent
		ref.Percent = &p
	} else {
		a := r.Amount
		ref.Amount = &a
	}
	return ref
}

// SplitVAT splits a VAT-inclusive gross amount into base and VAT. A nil
// rate means "not configured": base = gross, VAT = 0.
func SplitVAT(gross money.Money, rate *money.Rate) (base, vat money.Money) {
	if rate == nil || *rate == 0 {
		return gross, money.NewIn(0, gross.Currency())
	}
	base = gross.Ratio(money.RateScale, money.RateScale+int64(*rate))
	return base, gross.Sub(base)
}

// PrepaymentAmount applies the prepayment rule to the total.
func PrepaymentAmount(p Prepayment, total money.Money) (money.Money, error) {
	switch p.Rule {
	case "", PrepaymentNone:
		return money.NewIn(0, total.Currency()), nil
	case PrepaymentFixed:
		if p.Amount.IsNegative() {
			return money.Money{}, fail(ErrInvalidInput, "prepayment amount must not be negative")
		}
		return p.Amount.Min(total), nil
	case PrepaymentPercent:
		if p.Percent < 0 || p.Percent > money.Percent(100) {
			return money.Money{}, fail(ErrInvalidInput, "prepayment percent must be between 0 and 100")
		}
		return total.MulRate(p.Percent), nil
	case PrepaymentFull:
		return total, nil
	}
	return money.Money{}, fail(ErrInvalidInput, "unknown prepayment rule "+p.Rule)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
