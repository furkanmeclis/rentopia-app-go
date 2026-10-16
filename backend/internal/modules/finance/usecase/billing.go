package usecase

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/jackc/pgx/v5/pgtype"
)

// Periodic billing (TEC-723, RT-A24, docs/design/corporate-agreements.md).
//
// A rental whose rent_billing is "periodic" (sold under a corporate
// agreement with a monthly invoice period) never accrues its rent at the
// handover. Its rent (the reservation total) is split into calendar-month
// periods (pricing.MonthlySpans / Distribute: partial first and last
// periods pro rata by days) kept in rental_billing_periods, and each period
// is accrued once, under the key agreement:<a>:r<rental>:p<n>, when it
// starts (and the rental is under way). SyncRental drives it, so the job,
// the handover, an extension and the return all go through the same
// reconciliation:
//
//   - posted periods are history: an extension re-plans only what follows
//     the last posted period (the remaining total spread over the new
//     remaining days);
//   - an early return cuts the plan at the return: periods after it are
//     reversed, the period containing it is superseded by a prorated
//     revision (reversal + new accrual under the same key);
//   - the database refuses a whole-rent accrual on a periodic rental and a
//     period accrual on a handover rental, and the single-accrual rule
//     (RT-A23) refuses a second live accrual of the same period key.

const (
	periodPlanned    = "planned"
	periodPosted     = "posted"
	periodSuperseded = "superseded"
	periodReversed   = "reversed"
)

// PeriodKey is the accrual key of a billing period.
func PeriodKey(agreementID, rentalID int64, no int) string {
	if agreementID > 0 {
		return fmt.Sprintf("agreement:%d:r%d:p%d", agreementID, rentalID, no)
	}
	return fmt.Sprintf("rental:%d:p%d", rentalID, no)
}

// periodWant is a period accrual SyncRental keeps or posts.
type periodWant struct {
	periodID  int64
	amount    money.Money
	entryDate time.Time
	no        int
}

// plannedRow is a period row to (re)write as planned.
type plannedRow struct {
	no, revision     int
	dayFrom, dayTo   int
	startsOn, endsOn rentaltime.Date
	days             int
	partial          bool
	amount           money.Money
	km               *int
}

// syncBillingPlan brings the billing plan of a periodic rental in line with
// its reservation (range, total) and its return, on q (the caller's
// transaction), and returns the period accruals that must be live: every
// posted period, plus the planned periods that are due (started by today,
// rental under way). Called by SyncRental only.
func (s *Service) syncBillingPlan(ctx context.Context, q *db.Queries, orgID int64, f db.GetRentalLedgerFactsRow) (map[string]periodWant, error) {
	if _, err := q.LockRentalForBilling(ctx, db.LockRentalForBillingParams{OrganizationID: orgID, ID: f.ID}); err != nil {
		return nil, err
	}
	rows, err := q.ListRentalBillingPeriods(ctx, db.ListRentalBillingPeriodsParams{OrganizationID: orgID, RentalID: f.ID})
	if err != nil {
		return nil, err
	}
	want := map[string]periodWant{}
	now := s.clock()
	if f.Status == "cancelled" || f.CollectingParty != "us" {
		// Nothing is (or stays) due: planned periods go, posted ones are
		// reversed with their accruals.
		if _, err := q.DeletePlannedBillingPeriods(ctx, db.DeletePlannedBillingPeriodsParams{OrganizationID: orgID, RentalID: f.ID}); err != nil {
			return nil, err
		}
		for _, r := range rows {
			if r.Status == periodPosted {
				if _, err := q.CloseBillingPeriod(ctx, db.CloseBillingPeriodParams{OrganizationID: orgID, ID: r.ID, Status: periodReversed, ClosedAt: tsOf(now)}); err != nil {
					return nil, err
				}
			}
		}
		return want, nil
	}

	start, plannedEnd := f.StartsAt.Time.UTC(), f.EndsAt.Time.UTC()
	rng, err := rentaltime.NewRange(start, plannedEnd)
	if err != nil {
		return nil, fmt.Errorf("finance: rental %d range: %w", f.ID, err)
	}
	totalDays := rng.BillingDays()
	keptDays := totalDays
	if f.ActualReturnAt.Valid && f.ActualReturnAt.Time.Before(plannedEnd) {
		// Early return: billing stops at the return (at least one day).
		keptDays = 1
		if r, err := rentaltime.NewRange(start, f.ActualReturnAt.Time.UTC()); err == nil {
			keptDays = r.BillingDays()
		}
	}
	total := m(f.TotalAmount)
	currency := f.Currency
	agreementID := f.AgreementID.Int64

	// Posted (live) periods are fixed; the rest is re-planned after them.
	posted := []db.ListRentalBillingPeriodsRow{}
	maxRevision := map[int]int{}
	for _, r := range rows {
		if int(r.Revision) > maxRevision[int(r.PeriodNo)] {
			maxRevision[int(r.PeriodNo)] = int(r.Revision)
		}
		if r.Status == periodPosted {
			posted = append(posted, r)
		}
	}
	sort.Slice(posted, func(i, j int) bool { return posted[i].PeriodNo < posted[j].PeriodNo })
	postedSum, cursor, nextNo := money.NewIn(0, total.Currency()), 0, 1
	for _, r := range posted {
		postedSum = postedSum.Add(m(r.Amount))
		cursor = max(cursor, int(r.DayTo))
		nextNo = max(nextNo, int(r.PeriodNo)+1)
	}
	future := pricing.Distribute(total.Sub(postedSum), pricing.MonthlySpans(start, cursor, totalDays, nextNo))

	desired := []plannedRow{}
	km := func(p pricing.BillingPeriod) *int {
		if !f.MonthlyKmAllowance.Valid {
			return nil
		}
		v := pricing.ProrateKm(int(f.MonthlyKmAllowance.Int32), p)
		return &v
	}
	if keptDays < totalDays || keptDays < cursor {
		future = pricing.Truncate(future, start, keptDays)
		for _, r := range posted {
			switch {
			case int(r.DayFrom) >= keptDays:
				// Not due any more: the period and its accrual are reversed.
				if _, err := q.CloseBillingPeriod(ctx, db.CloseBillingPeriodParams{OrganizationID: orgID, ID: r.ID, Status: periodReversed, ClosedAt: tsOf(now)}); err != nil {
					return nil, err
				}
			case int(r.DayTo) > keptDays:
				// Cut short: a prorated revision replaces it (same key).
				if _, err := q.CloseBillingPeriod(ctx, db.CloseBillingPeriodParams{OrganizationID: orgID, ID: r.ID, Status: periodSuperseded, ClosedAt: tsOf(now)}); err != nil {
					return nil, err
				}
				kept := keptDays - int(r.DayFrom)
				p := pricing.BillingPeriod{
					No: int(r.PeriodNo), FromDay: int(r.DayFrom), ToDay: keptDays, StartsOn: rentaltime.DateOf(r.StartsOn.Time),
					EndsOn: rentaltime.LocalDate(start.Add(time.Duration(keptDays-1) * rentaltime.Day)).AddDays(1),
					Days:   kept, Partial: true, Amount: pricing.Prorate(m(r.Amount), kept, int(r.Days)),
				}
				desired = append(desired, plannedRow{
					no: p.No, revision: maxRevision[p.No] + 1, dayFrom: p.FromDay, dayTo: p.ToDay, startsOn: p.StartsOn,
					endsOn: p.EndsOn, days: p.Days, partial: true, amount: p.Amount, km: km(p),
				})
				maxRevision[p.No]++
			}
		}
	}
	for _, p := range future {
		if !p.Amount.IsPositive() {
			continue
		}
		desired = append(desired, plannedRow{
			no: p.No, revision: maxRevision[p.No] + 1, dayFrom: p.FromDay, dayTo: p.ToDay, startsOn: p.StartsOn,
			endsOn: p.EndsOn, days: p.Days, partial: p.Partial, amount: p.Amount, km: km(p),
		})
	}

	// Planned rows are replaced only when the plan changed (idempotent).
	planned := []db.ListRentalBillingPeriodsRow{}
	for _, r := range rows {
		if r.Status == periodPlanned {
			planned = append(planned, r)
		}
	}
	if !samePlan(planned, desired) {
		if _, err := q.DeletePlannedBillingPeriods(ctx, db.DeletePlannedBillingPeriodsParams{OrganizationID: orgID, RentalID: f.ID}); err != nil {
			return nil, err
		}
		planned = planned[:0]
		for _, d := range desired {
			rev := d.revision
			row, err := q.InsertRentalBillingPeriod(ctx, db.InsertRentalBillingPeriodParams{
				OrganizationID: orgID, RentalID: f.ID, AgreementID: f.AgreementID, PeriodNo: int32(d.no), Revision: int32(rev),
				DayFrom: int32(d.dayFrom), DayTo: int32(d.dayTo), StartsOn: pgtype.Date{Time: d.startsOn.Time(), Valid: true},
				EndsOn: pgtype.Date{Time: d.endsOn.Time(), Valid: true}, Days: int32(d.days), Partial: d.partial,
				Amount: d.amount.Numeric(), Currency: currency, KmAllowance: int4Of(d.km), AccrualKey: PeriodKey(agreementID, f.ID, d.no),
			})
			if err != nil {
				return nil, fmt.Errorf("finance: billing period %d of rental %d: %w", d.no, f.ID, err)
			}
			planned = append(planned, db.ListRentalBillingPeriodsRow{
				ID: row.ID, PeriodNo: int32(d.no), StartsOn: pgtype.Date{Time: d.startsOn.Time(), Valid: true},
				Amount: d.amount.Numeric(), AccrualKey: PeriodKey(agreementID, f.ID, d.no),
			})
		}
	}

	// Live accruals: every posted period that is still posted, and the
	// planned periods that are due.
	refreshed, err := q.ListRentalBillingPeriods(ctx, db.ListRentalBillingPeriodsParams{OrganizationID: orgID, RentalID: f.ID})
	if err != nil {
		return nil, err
	}
	today := s.today()
	underway := rentAccrues[f.Status]
	for _, r := range refreshed {
		switch {
		case r.Status == periodPosted:
			want[r.AccrualKey] = periodWant{periodID: r.ID, amount: m(r.Amount), entryDate: r.StartsOn.Time, no: int(r.PeriodNo)}
		case r.Status == periodPlanned && underway && !r.StartsOn.Time.After(today) && m(r.Amount).IsPositive():
			want[r.AccrualKey] = periodWant{periodID: r.ID, amount: m(r.Amount), entryDate: r.StartsOn.Time, no: int(r.PeriodNo)}
		}
	}
	return want, nil
}

// samePlan reports whether the planned rows already match the desired plan.
func samePlan(rows []db.ListRentalBillingPeriodsRow, desired []plannedRow) bool {
	if len(rows) != len(desired) {
		return false
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].PeriodNo < rows[j].PeriodNo })
	sorted := append([]plannedRow(nil), desired...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].no < sorted[j].no })
	for i, r := range rows {
		d := sorted[i]
		if int(r.PeriodNo) != d.no || int(r.DayFrom) != d.dayFrom || int(r.DayTo) != d.dayTo || !m(r.Amount).Equal(d.amount) ||
			(r.KmAllowance.Valid != (d.km != nil)) || (d.km != nil && int(r.KmAllowance.Int32) != *d.km) {
			return false
		}
	}
	return true
}

// AccruePeriods is the periodic billing job (worker, hourly): every
// periodically billed rental whose plan is missing or has a period due is
// reconciled (SyncRental, one transaction per rental). Running it again,
// or concurrently, posts nothing twice: the rental row lock serialises
// runs, posted periods are skipped and the ledger refuses a second live
// accrual of a period key. Returns how many rentals were reconciled.
func (s *Service) AccruePeriods(ctx context.Context) (int, error) {
	rows, err := s.q.ListDuePeriodicRentals(ctx, db.ListDuePeriodicRentalsParams{Today: dateOf(s.today()), LimitCount: 500})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		octx := orgctx.WithOrganizationID(ctx, r.OrganizationID)
		if err := s.tx(octx, func(q *db.Queries) error { return s.SyncRental(octx, q, r.OrganizationID, r.ID) }); err != nil {
			s.log.Warn("agreement_period_accrual_failed", "organization_id", r.OrganizationID, "rental_id", r.ID, "error", err)
			continue
		}
		n++
	}
	return n, nil
}

func tsOf(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func int4Of(v *int) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*v), Valid: true}
}
