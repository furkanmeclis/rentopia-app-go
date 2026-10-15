// Package rentaltime holds the time rules of a rental (TEC-675).
//
//   - Instants are stored and compared in UTC (timestamptz columns).
//   - Calendar decisions (which local day a billing day starts on, date rule
//     ranges, "today") use Europe/Istanbul: Location().
//   - Ranges are half-open: [start, end). A rule "1 June – 1 September" is
//     June 1 .. August 31; September 1 is outside ("bitiş günü hariç").
//   - Billing days = ceil(duration / 24h), at least 1. Exactly 24h is one
//     day, 24h and one minute is two days; 3 hours is one day.
package rentaltime

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	// The Istanbul zone must resolve in minimal containers too.
	_ "time/tzdata"
)

// Day is the billing unit.
const Day = 24 * time.Hour

// ErrInvalidRange is returned when end is not after start.
var ErrInvalidRange = errors.New("rentaltime: end must be after start")

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		// Turkey is UTC+3 all year since 2016.
		return time.FixedZone("Europe/Istanbul", 3*60*60)
	}
	return loc
}()

// Location is Europe/Istanbul, the display / local-date zone.
func Location() *time.Location { return istanbul }

// UTC normalises an instant for storage (UTC, microsecond precision like
// timestamptz).
func UTC(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// Local returns t in Europe/Istanbul (display).
func Local(t time.Time) time.Time { return t.In(istanbul) }

// Range is a half-open instant range [Start, End).
type Range struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NewRange validates end > start and normalises both to UTC.
func NewRange(start, end time.Time) (Range, error) {
	if !end.After(start) {
		return Range{}, ErrInvalidRange
	}
	return Range{Start: UTC(start), End: UTC(end)}, nil
}

// Duration returns End - Start.
func (r Range) Duration() time.Duration { return r.End.Sub(r.Start) }

// Contains reports whether t is in [Start, End).
func (r Range) Contains(t time.Time) bool { return !t.Before(r.Start) && t.Before(r.End) }

// Overlaps reports whether two half-open ranges share an instant
// (touching ranges, a.End == b.Start, do not overlap).
func (r Range) Overlaps(o Range) bool { return r.Start.Before(o.End) && o.Start.Before(r.End) }

// BillingDays returns ceil(duration / 24h), at least 1.
func (r Range) BillingDays() int { return BillingDays(r.Duration()) }

// BillingDays returns ceil(d / 24h), at least 1 (d <= 0 counts as 1).
func BillingDays(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	days := int(d / Day)
	if d%Day != 0 {
		days++
	}
	return days
}

// BillingDayStarts returns the start instant of every billing day:
// Start, Start+24h, … (BillingDays entries).
func (r Range) BillingDayStarts() []time.Time {
	n := r.BillingDays()
	out := make([]time.Time, n)
	for i := 0; i < n; i++ {
		out[i] = r.Start.Add(time.Duration(i) * Day)
	}
	return out
}

// BillingDayDates returns the local (Istanbul) calendar date each billing
// day starts on. Date rules are matched against these dates.
func (r Range) BillingDayDates() []Date {
	starts := r.BillingDayStarts()
	out := make([]Date, len(starts))
	for i, s := range starts {
		out[i] = LocalDate(s)
	}
	return out
}

// ------------------------------------------------------------------ dates

// Date is a calendar date without a zone ("2026-06-01").
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar date of t in t's own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// LocalDate returns the Istanbul calendar date of an instant.
func LocalDate(t time.Time) Date { return DateOf(t.In(istanbul)) }

// NewDate builds a date (normalised: NewDate(2026, 2, 30) = 2026-03-02).
func NewDate(y int, m time.Month, d int) Date {
	return DateOf(time.Date(y, m, d, 0, 0, 0, 0, time.UTC))
}

// ParseDate reads "2006-01-02".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, fmt.Errorf("rentaltime: invalid date %q", s)
	}
	return DateOf(t), nil
}

// MustDate is ParseDate that panics (tests).
func MustDate(s string) Date {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

// IsZero reports the zero date.
func (d Date) IsZero() bool { return d.Year == 0 && d.Month == 0 && d.Day == 0 }

// Time returns midnight UTC of the date (pgtype.Date values).
func (d Date) Time() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// StartIn returns local midnight of the date in Istanbul as an instant.
func (d Date) StartIn() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, istanbul) }

// AddDays returns the date n days later.
func (d Date) AddDays(n int) Date { return DateOf(d.Time().AddDate(0, 0, n)) }

// Before, After and Equal compare dates.
func (d Date) Before(o Date) bool { return d.Time().Before(o.Time()) }
func (d Date) After(o Date) bool  { return d.Time().After(o.Time()) }
func (d Date) Equal(o Date) bool  { return d == o }

// String renders "2006-01-02".
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

// MarshalJSON renders "2006-01-02".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON reads "2006-01-02".
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// DateRange is a half-open date range [Start, End): End is excluded.
type DateRange struct {
	Start Date `json:"start"`
	End   Date `json:"end"`
}

// NewDateRange validates end > start.
func NewDateRange(start, end Date) (DateRange, error) {
	if !end.After(start) {
		return DateRange{}, ErrInvalidRange
	}
	return DateRange{Start: start, End: end}, nil
}

// Contains reports whether d is in [Start, End).
func (r DateRange) Contains(d Date) bool { return !d.Before(r.Start) && d.Before(r.End) }

// Overlaps reports whether two half-open date ranges share a day.
func (r DateRange) Overlaps(o DateRange) bool { return r.Start.Before(o.End) && o.Start.Before(r.End) }

// Days returns the number of days in the range.
func (r DateRange) Days() int { return int(r.End.Time().Sub(r.Start.Time()) / Day) }
