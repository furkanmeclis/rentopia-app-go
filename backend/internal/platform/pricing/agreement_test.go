package pricing

import (
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

func agreementPlan(agreement, price string) Plan {
	return Plan{
		ID: "plan-" + agreement, VersionID: "pv-" + agreement, Version: 2, Name: "Kurumsal", Scope: ScopeAgreement,
		AgreementID: agreement, Bands: []Band{{MinDays: 1, DailyPrice: m(price)}},
	}
}

func TestAgreementTariffBeatsPublicPlans(t *testing.T) {
	t.Parallel()
	start := istanbul(2026, 4, 10, 10, 0)
	branchChannel := Plan{
		ID: "plan-bc", VersionID: "pv-bc", Name: "Şube web", Scope: ScopeBranchChannel, BranchID: "br-ist", Channel: "web",
		Priority: 1000, Bands: []Band{{MinDays: 1, DailyPrice: m("2000")}},
	}
	plans := []Plan{commonPlan(), branchChannel, agreementPlan("ag-1", "900"), agreementPlan("ag-2", "800")}

	// Under ag-1: its tariff wins over the most specific public plan.
	b := calc(t, Input{Start: start, End: start.Add(5 * rentaltime.Day), Plans: plans, AgreementID: "ag-1"})
	if b.Plan.ID != "plan-ag-1" || b.Plan.Scope != ScopeAgreement || b.Plan.AgreementID != "ag-1" || b.Total.String() != "4500.00" {
		t.Fatalf("agreement sale = %+v total %s", b.Plan, b.Total)
	}
	// A public sale never sees agreement plans.
	b = calc(t, Input{Start: start, End: start.Add(5 * rentaltime.Day), Plans: plans})
	if b.Plan.ID != "plan-bc" {
		t.Fatalf("public sale = %+v", b.Plan)
	}
	// An agreement without its own tariff falls back to the public plans.
	b = calc(t, Input{Start: start, End: start.Add(5 * rentaltime.Day), Plans: plans, AgreementID: "ag-3"})
	if b.Plan.ID != "plan-bc" {
		t.Fatalf("agreement without tariff = %+v", b.Plan)
	}
	// Only agreement plans: a public sale has no plan.
	_, err := Calculate(Input{Start: start, End: start.Add(4*time.Hour), Product: product, BranchID: "br-ist", Channel: "web", Plans: []Plan{agreementPlan("ag-1", "900")}})
	if !errors.Is(err, ErrNoPlan) {
		t.Fatalf("public sale with only agreement plans = %v", err)
	}
	if ScopeRank(ScopeAgreement) <= ScopeRank(ScopeBranchChannel) {
		t.Fatal("agreement scope must outrank every public scope")
	}
}

func TestBillingPeriodsPartialFirstAndLast(t *testing.T) {
	t.Parallel()
	// 15 January 10:00 → 10 April 10:00 = 85 billing days:
	// January 17 (partial), February 28, March 31, April 9 (partial).
	start := istanbul(2026, 1, 15, 10, 0)
	end := istanbul(2026, 4, 10, 10, 0)
	total := m("8500.00")
	plan := PeriodPlan(start, end, total)
	if len(plan) != 4 {
		t.Fatalf("periods = %d", len(plan))
	}
	want := []struct {
		days    int
		partial bool
		from    string
		to      string
		amount  string
	}{
		{17, true, "2026-01-15", "2026-02-01", "1700.00"},
		{28, false, "2026-02-01", "2026-03-01", "2800.00"},
		{31, false, "2026-03-01", "2026-04-01", "3100.00"},
		{9, true, "2026-04-01", "2026-04-10", "900.00"},
	}
	sum := money.Zero()
	for i, w := range want {
		p := plan[i]
		if p.No != i+1 || p.Days != w.days || p.Partial != w.partial || p.StartsOn.String() != w.from || p.EndsOn.String() != w.to || p.Amount.String() != w.amount {
			t.Fatalf("period %d = %+v, want %+v", i+1, p, w)
		}
		sum = sum.Add(p.Amount)
	}
	if !sum.Equal(total) {
		t.Fatalf("sum %s != total %s", sum, total)
	}
}

func TestDistributeNeverDrifts(t *testing.T) {
	t.Parallel()
	start := istanbul(2026, 1, 15, 10, 0)
	// 100.00 over 85 days does not divide: the cumulative rounding still
	// adds up exactly, and every period is within a kuruş of its share.
	for _, raw := range []string{"100.00", "0.01", "99999.99", "12345.67"} {
		total := m(raw)
		plan := PeriodPlan(start, start.Add(85*rentaltime.Day), total)
		sum := money.Zero()
		for _, p := range plan {
			sum = sum.Add(p.Amount)
		}
		if !sum.Equal(total) {
			t.Fatalf("%s: sum %s", raw, sum)
		}
	}
	// A negative remainder spreads nothing.
	for _, p := range Distribute(m("-5.00"), MonthlySpans(start, 0, 40, 1)) {
		if !p.Amount.IsZero() {
			t.Fatalf("negative total spread %s", p.Amount)
		}
	}
}

func TestTruncateProratesTheCutPeriod(t *testing.T) {
	t.Parallel()
	start := istanbul(2026, 1, 15, 10, 0)
	plan := PeriodPlan(start, istanbul(2026, 4, 10, 10, 0), m("8500.00"))
	// Terminated after 30 billing days: January (17) stays, February keeps
	// 13 of its 28 days, March and April go.
	cut := Truncate(plan, start, 30)
	if len(cut) != 2 || cut[1].Days != 13 || !cut[1].Partial || cut[1].EndsOn.String() != "2026-02-14" {
		t.Fatalf("cut = %+v", cut)
	}
	if got := cut[1].Amount.String(); got != "1300.00" {
		t.Fatalf("prorated February = %s", got)
	}
	if Prorate(m("10.00"), 1, 3).String() != "3.33" || Prorate(m("10.00"), 3, 3).String() != "10.00" || !Prorate(m("10.00"), 0, 3).IsZero() {
		t.Fatal("prorate rounding")
	}
	if ProrateKm(3000, cut[1]) != 1393 || ProrateKm(3000, plan[2]) != 3000 {
		t.Fatalf("km allowance = %d / %d", ProrateKm(3000, cut[1]), ProrateKm(3000, plan[2]))
	}
}

func TestMonthlySpansFromACursor(t *testing.T) {
	t.Parallel()
	// An extension re-plans from the end of the last posted period (day 17
	// = 1 February) to the new end: numbering continues.
	start := istanbul(2026, 1, 15, 10, 0)
	spans := MonthlySpans(start, 17, 17+28+5, 2)
	if len(spans) != 2 || spans[0].No != 2 || spans[0].StartsOn.String() != "2026-02-01" || spans[1].Days != 5 || !spans[1].Partial {
		t.Fatalf("spans = %+v", spans)
	}
}
