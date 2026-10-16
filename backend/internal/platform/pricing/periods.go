package pricing

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

// Periodic billing (TEC-723, docs/design/corporate-agreements.md).
//
// A periodically billed rental (a corporate agreement with a monthly
// invoice period) is not accrued in one piece at the handover; its rent is
// split into monthly periods. Everything here is pure, like Calculate.
//
// Periods are built from the billing days of the rental (rentaltime: day i
// starts at start + i·24h, its Istanbul calendar date decides the month):
// consecutive billing days of the same calendar month form one period. The
// first and the last period are partial when the rental does not start on
// the 1st or does not end at the end of a month.
//
// Amounts (pro rata by days). With R the amount to spread, D the billing
// days it covers and C_i the cumulative days up to and including period i:
//
//	amount_i = round(R · C_i / D) − round(R · C_(i−1) / D)
//
// Every period gets its share by days, the rounding never drifts, and the
// amounts add up to R to the kuruş. A period cut short (early termination
// on day k of a period of d days) keeps round(amount · k / d).

// BillingPeriod is one period of a billing plan. FromDay / ToDay are
// billing day indices [from, to) counted from the rental start; StartsOn /
// EndsOn are the Istanbul dates of those days (EndsOn exclusive).
type BillingPeriod struct {
	No       int             `json:"no"`
	FromDay  int             `json:"from_day"`
	ToDay    int             `json:"to_day"`
	StartsOn rentaltime.Date `json:"starts_on"`
	EndsOn   rentaltime.Date `json:"ends_on"`
	Days     int             `json:"days"`
	// Partial is true when the period covers less than its calendar month.
	Partial bool        `json:"partial"`
	Amount  money.Money `json:"amount"`
}

// dayDate is the Istanbul date billing day i of a rental starting at start
// begins on.
func dayDate(start time.Time, i int) rentaltime.Date {
	return rentaltime.LocalDate(start.Add(time.Duration(i) * rentaltime.Day))
}

// daysIn is the number of days of the calendar month of d.
func daysIn(d rentaltime.Date) int {
	return time.Date(d.Year, d.Month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// MonthlySpans groups the billing days [fromDay, toDay) of a rental
// starting at start into calendar-month periods, numbered from firstNo.
// Amounts are left zero (see Distribute).
func MonthlySpans(start time.Time, fromDay, toDay, firstNo int) []BillingPeriod {
	out := []BillingPeriod{}
	if fromDay < 0 {
		fromDay = 0
	}
	for i := fromDay; i < toDay; {
		first := dayDate(start, i)
		j := i + 1
		for j < toDay {
			d := dayDate(start, j)
			if d.Month != first.Month || d.Year != first.Year {
				break
			}
			j++
		}
		days := j - i
		out = append(out, BillingPeriod{
			No: firstNo + len(out), FromDay: i, ToDay: j, StartsOn: first, EndsOn: dayDate(start, j-1).AddDays(1),
			Days: days, Partial: days < daysIn(first), Amount: money.Zero(),
		})
		i = j
	}
	return out
}

// Distribute spreads total over the periods by days (the formula above)
// and returns them with their amounts. A negative total spreads nothing
// (every amount zero): money already billed is never taken back here.
func Distribute(total money.Money, periods []BillingPeriod) []BillingPeriod {
	out := make([]BillingPeriod, len(periods))
	copy(out, periods)
	days := 0
	for _, p := range out {
		days += p.Days
	}
	zero := money.NewIn(0, total.Currency())
	if days == 0 || !total.IsPositive() {
		for i := range out {
			out[i].Amount = zero
		}
		return out
	}
	cum, prev := 0, zero
	for i := range out {
		cum += out[i].Days
		next := total.Ratio(int64(cum), int64(days))
		out[i].Amount = next.Sub(prev)
		prev = next
	}
	return out
}

// Prorate is the part of a period amount kept when only kept of its days
// are billed: round(amount · kept / days).
func Prorate(amount money.Money, kept, days int) money.Money {
	if days <= 0 || kept >= days {
		return amount
	}
	if kept <= 0 {
		return money.NewIn(0, amount.Currency())
	}
	return amount.Ratio(int64(kept), int64(days))
}

// Truncate cuts a plan at billing day keptDays (early termination): periods
// starting at or after it are dropped, the period containing it is cut
// short and prorated.
func Truncate(periods []BillingPeriod, start time.Time, keptDays int) []BillingPeriod {
	out := []BillingPeriod{}
	for _, p := range periods {
		switch {
		case p.FromDay >= keptDays:
			continue
		case p.ToDay > keptDays:
			kept := keptDays - p.FromDay
			p.Amount = Prorate(p.Amount, kept, p.Days)
			p.ToDay, p.Days = keptDays, kept
			p.EndsOn = dayDate(start, keptDays-1).AddDays(1)
			p.Partial = true
		}
		out = append(out, p)
	}
	return out
}

// PeriodPlan is the full monthly plan of a rental [start, end) whose rent
// total is spread by days (quotes show it before anything is posted).
func PeriodPlan(start, end time.Time, total money.Money) []BillingPeriod {
	rng, err := rentaltime.NewRange(start, end)
	if err != nil {
		return []BillingPeriod{}
	}
	return Distribute(total, MonthlySpans(rng.Start, 0, rng.BillingDays(), 1))
}

// ProrateKm is the km allowance of a period: the monthly allowance times
// the period's days over the days of its calendar month (rounded half up).
func ProrateKm(monthly int, p BillingPeriod) int {
	if monthly <= 0 || p.Days <= 0 {
		return 0
	}
	n := daysIn(p.StartsOn)
	return (monthly*p.Days*2 + n) / (2 * n)
}
