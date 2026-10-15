package pricing

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

func ip(v int) *int { return &v }

func m(s string) money.Money { return money.MustParse(s) }

// İdes style bands: 1-3, 4-7, 8-14, 15+.
func idesBands(p1, p4, p8, p15 string) []Band {
	return []Band{
		{MinDays: 1, MaxDays: ip(3), DailyPrice: m(p1)},
		{MinDays: 4, MaxDays: ip(7), DailyPrice: m(p4)},
		{MinDays: 8, MaxDays: ip(14), DailyPrice: m(p8)},
		{MinDays: 15, MaxDays: nil, DailyPrice: m(p15)},
	}
}

var product = Product{ID: "prd-egea", SegmentID: "seg-eko"}

func commonPlan() Plan {
	return Plan{ID: "plan-common", VersionID: "pv-1", Version: 1, Name: "Genel", Scope: ScopeCommon, Bands: idesBands("1500", "1400", "1300", "1200")}
}

// istanbul returns an Istanbul wall-clock instant.
func istanbul(y int, mo time.Month, d, h, min int) time.Time {
	return time.Date(y, mo, d, h, min, 0, 0, rentaltime.Location())
}

func yaz(kind string, pct int64, amount string) DateRule {
	r := DateRule{
		ID: "rule-yaz", VersionID: "rv-1", Version: 1, Name: "YAZ",
		Range:        rentaltime.DateRange{Start: rentaltime.MustDate("2026-06-01"), End: rentaltime.MustDate("2026-09-01")},
		ProductScope: ProductsAll, Kind: kind, Priority: 10,
	}
	if kind == KindPercent {
		r.Percent = money.Percent(pct)
	} else {
		r.Amount = m(amount)
	}
	return r
}

func calc(t *testing.T, in Input) Breakdown {
	t.Helper()
	if in.Product.ID == "" {
		in.Product = product
	}
	if in.BranchID == "" {
		in.BranchID = "br-ist"
	}
	if in.Channel == "" {
		in.Channel = "web"
	}
	b, err := Calculate(in)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	return b
}

func TestAcceptanceRTA07EightDaysBand(t *testing.T) {
	t.Parallel()
	// RT-A07: 8 days, band 8-14 at 1.300 → every day 1.300, 10.400.
	start := istanbul(2026, 4, 10, 10, 0)
	b := calc(t, Input{Start: start, End: start.Add(8 * rentaltime.Day), Plans: []Plan{commonPlan()}})
	if b.BillingDays != 8 || b.Band.MinDays != 8 || b.RentalTotal.String() != "10400.00" || b.Total.String() != "10400.00" {
		t.Fatalf("RT-A07 = days %d band %d total %s", b.BillingDays, b.Band.MinDays, b.Total)
	}
	for _, d := range b.Days {
		if d.UnitPrice.String() != "1300.00" || d.Rule != nil {
			t.Fatalf("day %+v", d)
		}
	}
}

func TestAcceptanceRTA08SummerPercent(t *testing.T) {
	t.Parallel()
	// RT-A08: YAZ +20 % → 1.560 × 8 = 12.480.
	start := istanbul(2026, 7, 1, 10, 0)
	b := calc(t, Input{Start: start, End: start.Add(8 * rentaltime.Day), Plans: []Plan{commonPlan()}, Rules: []DateRule{yaz(KindPercent, 20, "")}})
	if b.RentalTotal.String() != "12480.00" {
		t.Fatalf("RT-A08 total = %s", b.RentalTotal)
	}
	for _, d := range b.Days {
		if d.UnitPrice.String() != "1560.00" || d.Rule == nil || d.Rule.Name != "YAZ" || d.Adjustment.String() != "260.00" {
			t.Fatalf("day %+v", d)
		}
	}
	if len(b.Rules) != 1 || b.Rules[0].Version != 1 {
		t.Fatalf("rules = %+v", b.Rules)
	}
}

func TestAcceptanceRTA09RuleStartsMidRental(t *testing.T) {
	t.Parallel()
	// RT-A09: rental starts May 30, YAZ starts June 1 → 2 × 1.300 + 6 × 1.560 = 11.960.
	start := istanbul(2026, 5, 30, 10, 0)
	b := calc(t, Input{Start: start, End: start.Add(8 * rentaltime.Day), Plans: []Plan{commonPlan()}, Rules: []DateRule{yaz(KindPercent, 20, "")}})
	if b.RentalTotal.String() != "11960.00" {
		t.Fatalf("RT-A09 total = %s", b.RentalTotal)
	}
	if b.Days[0].Rule != nil || b.Days[1].Rule != nil || b.Days[2].Rule == nil || b.Days[2].Date.String() != "2026-06-01" {
		t.Fatalf("days = %+v", b.Days[:3])
	}
}

func TestAcceptanceRTA10AmountRule(t *testing.T) {
	t.Parallel()
	// RT-A10: +100 ₺ → 1.400 per day.
	start := istanbul(2026, 7, 1, 10, 0)
	b := calc(t, Input{Start: start, End: start.Add(8 * rentaltime.Day), Plans: []Plan{commonPlan()}, Rules: []DateRule{yaz(KindAmount, 0, "100")}})
	for _, d := range b.Days {
		if d.UnitPrice.String() != "1400.00" {
			t.Fatalf("day = %s", d.UnitPrice)
		}
	}
	if b.RentalTotal.String() != "11200.00" {
		t.Fatalf("total = %s", b.RentalTotal)
	}
}

func TestAcceptanceRTA11Precedence(t *testing.T) {
	t.Parallel()
	start := istanbul(2026, 4, 10, 10, 0)
	end := start.Add(8 * rentaltime.Day)
	common := commonPlan()
	channel := Plan{ID: "plan-web", Version: 1, Scope: ScopeChannel, Channel: "web", Bands: idesBands("1", "1", "1100", "1")}
	branch := Plan{ID: "plan-ist", Version: 1, Scope: ScopeBranch, BranchID: "br-ist", Bands: idesBands("1", "1", "1200", "1")}
	both := Plan{ID: "plan-ist-web", Version: 1, Scope: ScopeBranchChannel, BranchID: "br-ist", Channel: "web", Bands: idesBands("1", "1", "1000", "1")}
	otherBranch := Plan{ID: "plan-ank", Version: 1, Scope: ScopeBranch, BranchID: "br-ank", Bands: idesBands("1", "1", "1", "1")}

	cases := []struct {
		name    string
		plans   []Plan
		branch  string
		channel string
		want    string
	}{
		{"branch+channel beats all", []Plan{common, channel, branch, both, otherBranch}, "br-ist", "web", "plan-ist-web"},
		{"branch beats channel", []Plan{common, channel, branch}, "br-ist", "web", "plan-ist"},
		{"channel beats common", []Plan{common, channel, otherBranch}, "br-ist", "web", "plan-web"},
		{"common fallback", []Plan{common, channel, otherBranch}, "br-ist", "branch", "plan-common"},
	}
	for _, c := range cases {
		b := calc(t, Input{Start: start, End: end, Plans: c.plans, BranchID: c.branch, Channel: c.channel})
		if b.Plan.ID != c.want {
			t.Errorf("%s: plan = %s, want %s", c.name, b.Plan.ID, c.want)
		}
	}

	// Same scope, higher priority wins.
	hi := common
	hi.ID, hi.Priority, hi.Bands = "plan-common-hi", 5, idesBands("1", "1", "900", "1")
	if b := calc(t, Input{Start: start, End: end, Plans: []Plan{common, hi}}); b.Plan.ID != "plan-common-hi" || b.RentalTotal.String() != "7200.00" {
		t.Fatalf("priority: %s %s", b.Plan.ID, b.RentalTotal)
	}
	// Same scope, same priority: refused (typed error with both ids).
	twin := common
	twin.ID = "plan-common-2"
	_, err := Calculate(Input{Start: start, End: end, Product: product, BranchID: "br-ist", Channel: "web", Plans: []Plan{common, twin}})
	var pe *Error
	if !errors.Is(err, ErrAmbiguousPlan) || !errors.As(err, &pe) || len(pe.IDs) != 2 || Code(err) != "ambiguous_plan" {
		t.Fatalf("equal priority = %v", err)
	}

	// Date rules: highest priority wins; equal top priority is refused.
	summer := yaz(KindPercent, 20, "")
	bayram := DateRule{
		ID: "rule-bayram", Version: 1, Name: "BAYRAM", ProductScope: ProductsAll, Kind: KindPercent, Percent: money.Percent(50), Priority: 20,
		Range: rentaltime.DateRange{Start: rentaltime.MustDate("2026-07-02"), End: rentaltime.MustDate("2026-07-04")},
	}
	julStart := istanbul(2026, 7, 1, 10, 0)
	b := calc(t, Input{Start: julStart, End: julStart.Add(4 * rentaltime.Day), Plans: []Plan{common}, Rules: []DateRule{summer, bayram}})
	// 4 days → band 4-7 at 1.400: day1 YAZ 1.680, day2-3 BAYRAM 2.100, day4 YAZ 1.680.
	if b.Days[0].Rule.Name != "YAZ" || b.Days[1].Rule.Name != "BAYRAM" || b.Days[3].Rule.Name != "YAZ" || b.RentalTotal.String() != "7560.00" {
		t.Fatalf("rule priority: %+v total %s", b.Days, b.RentalTotal)
	}
	bayram.Priority = 10
	_, err = Calculate(Input{Start: julStart, End: julStart.Add(4 * rentaltime.Day), Product: product, BranchID: "br-ist", Channel: "web", Plans: []Plan{common}, Rules: []DateRule{summer, bayram}})
	if !errors.Is(err, ErrAmbiguousRule) || Code(err) != "ambiguous_rule" {
		t.Fatalf("equal rule priority = %v", err)
	}
}

func TestSummerEndsExclusive(t *testing.T) {
	t.Parallel()
	// "YAZ" 1 June – 1 Sept, +20 %: a day starting on 1 Sept is off-season.
	start := istanbul(2026, 8, 31, 10, 0)
	b := calc(t, Input{Start: start, End: start.Add(2 * rentaltime.Day), Plans: []Plan{commonPlan()}, Rules: []DateRule{yaz(KindPercent, 20, "")}})
	if b.Days[0].Rule == nil || b.Days[1].Rule != nil || b.Days[1].Date.String() != "2026-09-01" {
		t.Fatalf("Sept 1 must be off-season: %+v", b.Days)
	}
	// 1 day band 1-3 at 1.500: 1.800 + 1.500.
	if b.RentalTotal.String() != "3300.00" {
		t.Fatalf("total = %s", b.RentalTotal)
	}
}

func TestEdgeCases(t *testing.T) {
	t.Parallel()
	plan := commonPlan()
	start := istanbul(2026, 4, 10, 10, 0)

	// Exact 24h = 1 day; one minute over = 2 days.
	if b := calc(t, Input{Start: start, End: start.Add(24 * time.Hour), Plans: []Plan{plan}}); b.BillingDays != 1 || b.Total.String() != "1500.00" {
		t.Fatalf("24h = %d %s", b.BillingDays, b.Total)
	}
	if b := calc(t, Input{Start: start, End: start.Add(24*time.Hour + time.Minute), Plans: []Plan{plan}}); b.BillingDays != 2 || b.Total.String() != "3000.00" {
		t.Fatalf("24h+1m = %d %s", b.BillingDays, b.Total)
	}
	// Sub-day = 1 day.
	if b := calc(t, Input{Start: start, End: start.Add(2 * time.Hour), Plans: []Plan{plan}}); b.BillingDays != 1 {
		t.Fatalf("2h = %d", b.BillingDays)
	}
	// Band boundary: 7 days → 4-7 band; 7 days + 1 minute → 8 days → 8-14.
	if b := calc(t, Input{Start: start, End: start.Add(7*rentaltime.Day + time.Minute), Plans: []Plan{plan}}); b.Band.MinDays != 8 || b.Total.String() != "10400.00" {
		t.Fatalf("7d+1m = band %d %s", b.Band.MinDays, b.Total)
	}

	// Negative clamp: a -2.000 ₺ rule on a 1.500 day gives 0, not negative.
	neg := yaz(KindAmount, 0, "-2000")
	neg.Range = rentaltime.DateRange{Start: rentaltime.MustDate("2026-04-01"), End: rentaltime.MustDate("2026-05-01")}
	if b := calc(t, Input{Start: start, End: start.Add(rentaltime.Day), Plans: []Plan{plan}, Rules: []DateRule{neg}}); !b.Days[0].UnitPrice.IsZero() || b.Days[0].Adjustment.String() != "-1500.00" {
		t.Fatalf("clamp = %+v", b.Days[0])
	}
	// Discount larger than the subtotal: rental total 0, discount capped.
	big := &Discount{Code: "BIG", Kind: KindAmount, Amount: m("99999")}
	if b := calc(t, Input{Start: start, End: start.Add(rentaltime.Day), Plans: []Plan{plan}, Discount: big}); !b.RentalTotal.IsZero() || b.Discount.Amount.String() != "1500.00" {
		t.Fatalf("discount clamp = %s %+v", b.RentalTotal, b.Discount)
	}
	// Percent discount with rounding: 3 × 1.500 = 4.500, 12.5 % = 562.50.
	pct := &Discount{Code: "C125", Kind: KindPercent, Percent: 1250}
	if b := calc(t, Input{Start: start, End: start.Add(3 * rentaltime.Day), Plans: []Plan{plan}, Discount: pct}); b.Discount.Amount.String() != "562.50" || b.RentalTotal.String() != "3937.50" {
		t.Fatalf("percent discount = %+v %s", b.Discount, b.RentalTotal)
	}
	// Rounding of a percent rule: 1.333,33 × 7.5 % = 99.99975 → 100.00.
	odd := Plan{ID: "odd", Scope: ScopeCommon, Bands: []Band{{MinDays: 1, DailyPrice: m("1333.33")}}}
	r := yaz(KindPercent, 0, "")
	r.Percent = 750
	r.Range = rentaltime.DateRange{Start: rentaltime.MustDate("2026-04-01"), End: rentaltime.MustDate("2026-05-01")}
	if b := calc(t, Input{Start: start, End: start.Add(rentaltime.Day), Plans: []Plan{odd}, Rules: []DateRule{r}}); b.Days[0].UnitPrice.String() != "1433.33" {
		t.Fatalf("rounded rule = %s", b.Days[0].UnitPrice)
	}

	// Errors.
	if _, err := Calculate(Input{Start: start, End: start, Product: product, Plans: []Plan{plan}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty range = %v", err)
	}
	if _, err := Calculate(Input{Start: start, End: start.Add(time.Hour), Product: product, BranchID: "x", Channel: "web"}); !errors.Is(err, ErrNoPlan) {
		t.Fatalf("no plan = %v", err)
	}
	short := Plan{ID: "short", Scope: ScopeCommon, Bands: []Band{{MinDays: 1, MaxDays: ip(3), DailyPrice: m("1")}}}
	if _, err := Calculate(Input{Start: start, End: start.Add(5 * rentaltime.Day), Product: product, Plans: []Plan{short}}); !errors.Is(err, ErrNoBand) || Code(err) != "no_band" {
		t.Fatalf("no band = %v", err)
	}
	dup := Plan{ID: "dup", Scope: ScopeCommon, Bands: []Band{{MinDays: 1, MaxDays: ip(3), DailyPrice: m("1")}, {MinDays: 2, DailyPrice: m("1")}}}
	if _, err := Calculate(Input{Start: start, End: start.Add(2 * rentaltime.Day), Product: product, Plans: []Plan{dup}}); !errors.Is(err, ErrAmbiguousBand) {
		t.Fatalf("ambiguous band = %v", err)
	}
}

func TestRuleTargets(t *testing.T) {
	t.Parallel()
	start := istanbul(2026, 7, 1, 10, 0)
	end := start.Add(rentaltime.Day)
	base := yaz(KindPercent, 20, "")
	cases := []struct {
		name  string
		edit  func(r *DateRule)
		apply bool
	}{
		{"all", func(*DateRule) {}, true},
		{"segment hit", func(r *DateRule) { r.ProductScope, r.SegmentIDs = ProductsSegments, []string{"seg-eko"} }, true},
		{"segment miss", func(r *DateRule) { r.ProductScope, r.SegmentIDs = ProductsSegments, []string{"seg-suv"} }, false},
		{"product hit", func(r *DateRule) { r.ProductScope, r.ProductIDs = ProductsSelected, []string{"prd-egea"} }, true},
		{"product miss", func(r *DateRule) { r.ProductScope, r.ProductIDs = ProductsSelected, []string{"prd-x"} }, false},
		{"branch miss", func(r *DateRule) { r.BranchIDs = []string{"br-ank"} }, false},
		{"branch hit", func(r *DateRule) { r.BranchIDs = []string{"br-ank", "br-ist"} }, true},
		{"channel miss", func(r *DateRule) { r.Channels = []string{"branch"} }, false},
		{"channel hit", func(r *DateRule) { r.Channels = []string{"web"} }, true},
	}
	for _, c := range cases {
		r := base
		c.edit(&r)
		b := calc(t, Input{Start: start, End: end, Plans: []Plan{commonPlan()}, Rules: []DateRule{r}})
		if got := b.Days[0].Rule != nil; got != c.apply {
			t.Errorf("%s: applied = %v, want %v", c.name, got, c.apply)
		}
	}
}

func TestLinesVATAndPrepayment(t *testing.T) {
	t.Parallel()
	start := istanbul(2026, 4, 10, 10, 0)
	vat := money.Percent(20)
	oneWay, delivery := m("750"), m("250")
	b := calc(t, Input{
		Start: start, End: start.Add(3 * rentaltime.Day), Plans: []Plan{commonPlan()},
		Extras:    []Extra{{Code: "CHILD_SEAT", Name: "Çocuk koltuğu", UnitPrice: m("100"), Quantity: 1, PerDay: true}, {Code: "HGS", UnitPrice: m("50"), Quantity: 2}},
		OneWayFee: &oneWay, DeliveryFee: &delivery, VATRate: &vat,
		Prepayment: Prepayment{Rule: PrepaymentPercent, Percent: money.Percent(25)},
	})
	// rental 4.500 + seat 300 + HGS 100 + one-way 750 + delivery 250 = 5.900.
	if b.Total.String() != "5900.00" || len(b.Lines) != 5 {
		t.Fatalf("total = %s lines %d", b.Total, len(b.Lines))
	}
	// 4.500 / 1.2 = 3.750 base, 750 VAT.
	if b.Lines[0].Base.String() != "3750.00" || b.Lines[0].VAT.String() != "750.00" {
		t.Fatalf("rental line = %+v", b.Lines[0])
	}
	if b.BaseTotal.Add(b.VATTotal) != b.Total {
		t.Fatalf("base + vat != total: %s + %s", b.BaseTotal, b.VATTotal)
	}
	if b.Prepayment.String() != "1475.00" || b.PrepaymentRule != PrepaymentPercent {
		t.Fatalf("prepayment = %s", b.Prepayment)
	}

	// Per-line rounding: 100.00 at 18 % → base 84.75 (84.745…), VAT 15.25.
	base, v := SplitVAT(m("100"), func() *money.Rate { r := money.Percent(18); return &r }())
	if base.String() != "84.75" || v.String() != "15.25" {
		t.Fatalf("split = %s %s", base, v)
	}
	// No VAT rate configured: no split.
	if base, v := SplitVAT(m("100"), nil); base.String() != "100.00" || !v.IsZero() {
		t.Fatalf("no rate = %s %s", base, v)
	}

	for _, c := range []struct {
		p    Prepayment
		want string
	}{
		{Prepayment{}, "0.00"},
		{Prepayment{Rule: PrepaymentFixed, Amount: m("1000")}, "1000.00"},
		{Prepayment{Rule: PrepaymentFixed, Amount: m("99999")}, "5900.00"},
		{Prepayment{Rule: PrepaymentFull}, "5900.00"},
		{Prepayment{Rule: PrepaymentPercent, Percent: 3333}, "1966.47"},
	} {
		got, err := PrepaymentAmount(c.p, b.Total)
		if err != nil || got.String() != c.want {
			t.Errorf("prepayment %+v = %s %v, want %s", c.p, got, err, c.want)
		}
	}
	if _, err := PrepaymentAmount(Prepayment{Rule: "weird"}, b.Total); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("unknown rule must fail")
	}
}

func TestValidateBands(t *testing.T) {
	t.Parallel()
	codes := func(issues []Issue) []string {
		out := []string{}
		for _, i := range issues {
			out = append(out, i.Code)
		}
		return out
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	cases := []struct {
		name  string
		bands []Band
		upTo  int
		want  []string
	}{
		{"ides ok", idesBands("1", "1", "1", "1"), 30, []string{}},
		{"none", nil, 0, []string{IssueNoBands}},
		{"overlap", []Band{{MinDays: 1, MaxDays: ip(5)}, {MinDays: 4, MaxDays: ip(7)}}, 0, []string{IssueBandOverlap}},
		{"open band then another", []Band{{MinDays: 1}, {MinDays: 10}}, 0, []string{IssueBandOverlap}},
		{"bad range", []Band{{MinDays: 5, MaxDays: ip(3)}}, 0, []string{IssueBandRange}},
		{"negative price", []Band{{MinDays: 1, DailyPrice: m("-1")}}, 0, []string{IssueBandPrice}},
		{"gap ok offline", []Band{{MinDays: 1, MaxDays: ip(3)}, {MinDays: 8}}, 0, []string{}},
		{"gap online", []Band{{MinDays: 1, MaxDays: ip(3)}, {MinDays: 8}}, 30, []string{IssueCoverageGap}},
		{"start online", []Band{{MinDays: 2}}, 30, []string{IssueCoverageFrom}},
		{"short online", []Band{{MinDays: 1, MaxDays: ip(3)}, {MinDays: 4, MaxDays: ip(14)}}, 30, []string{IssueCoverageEnd}},
		{"exact online", []Band{{MinDays: 1, MaxDays: ip(3)}, {MinDays: 4, MaxDays: ip(30)}}, 30, []string{}},
		{"gap after max is fine", []Band{{MinDays: 1, MaxDays: ip(30)}, {MinDays: 40}}, 30, []string{}},
	}
	for _, c := range cases {
		if got := codes(ValidateBands(c.bands, c.upTo)); !eq(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRulesOverlap(t *testing.T) {
	t.Parallel()
	seg := map[string]string{"p1": "s1", "p2": "s2"}
	r := func(start, end string, edit func(*DateRule)) DateRule {
		x := DateRule{ProductScope: ProductsAll, Range: rentaltime.DateRange{Start: rentaltime.MustDate(start), End: rentaltime.MustDate(end)}}
		if edit != nil {
			edit(&x)
		}
		return x
	}
	a := r("2026-06-01", "2026-09-01", nil)
	cases := []struct {
		name string
		b    DateRule
		want bool
	}{
		{"touching", r("2026-09-01", "2026-10-01", nil), false},
		{"inside", r("2026-07-01", "2026-07-05", nil), true},
		{"other branch set vs all", r("2026-07-01", "2026-07-05", func(x *DateRule) { x.BranchIDs = []string{"b1"} }), true},
		{"channel disjoint", r("2026-07-01", "2026-07-05", func(x *DateRule) { x.Channels = []string{} }), false},
	}
	for _, c := range cases {
		if got := RulesOverlap(a, c.b, seg); got != c.want {
			t.Errorf("%s = %v, want %v", c.name, got, c.want)
		}
	}
	p1 := r("2026-07-01", "2026-07-05", func(x *DateRule) { x.ProductScope, x.ProductIDs = ProductsSelected, []string{"p1"} })
	s1 := r("2026-07-01", "2026-07-05", func(x *DateRule) { x.ProductScope, x.SegmentIDs = ProductsSegments, []string{"s1"} })
	s2 := r("2026-07-01", "2026-07-05", func(x *DateRule) { x.ProductScope, x.SegmentIDs = ProductsSegments, []string{"s2"} })
	if !RulesOverlap(p1, s1, seg) || RulesOverlap(p1, s2, seg) || RulesOverlap(s1, s2, seg) || !RulesOverlap(s2, s2, seg) {
		t.Fatal("product / segment intersection")
	}
}
