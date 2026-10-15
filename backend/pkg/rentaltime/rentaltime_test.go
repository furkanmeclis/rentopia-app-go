package rentaltime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBillingDays(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		d    time.Duration
		want int
	}{
		{"zero counts as one", 0, 1},
		{"negative counts as one", -time.Hour, 1},
		{"one minute", time.Minute, 1},
		{"sub-day", 3 * time.Hour, 1},
		{"exact 24h", 24 * time.Hour, 1},
		{"24h + 1 minute", 24*time.Hour + time.Minute, 2},
		{"24h + 1 microsecond", 24*time.Hour + time.Microsecond, 2},
		{"exact 48h", 48 * time.Hour, 2},
		{"47h59m", 47*time.Hour + 59*time.Minute, 2},
		{"exact 8 days", 8 * 24 * time.Hour, 8},
		{"8 days + 1 minute", 8*24*time.Hour + time.Minute, 9},
		{"7 days 23h", 7*24*time.Hour + 23*time.Hour, 8},
	}
	for _, c := range cases {
		if got := BillingDays(c.d); got != c.want {
			t.Errorf("%s: BillingDays(%s) = %d, want %d", c.name, c.d, got, c.want)
		}
		if c.d > 0 {
			r, err := NewRange(start, start.Add(c.d))
			if err != nil {
				t.Fatal(err)
			}
			if got := r.BillingDays(); got != c.want {
				t.Errorf("%s: Range.BillingDays = %d, want %d", c.name, got, c.want)
			}
		}
	}
}

func TestNewRangeRejectsEmpty(t *testing.T) {
	t.Parallel()
	now := time.Now()
	if _, err := NewRange(now, now); err == nil {
		t.Fatal("empty range must fail")
	}
	if _, err := NewRange(now, now.Add(-time.Minute)); err == nil {
		t.Fatal("reversed range must fail")
	}
}

func TestRangeHalfOpen(t *testing.T) {
	t.Parallel()
	a := Range{Start: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)}
	b := Range{Start: a.End, End: a.End.Add(Day)}
	if a.Overlaps(b) || b.Overlaps(a) {
		t.Fatal("touching ranges must not overlap")
	}
	if !a.Contains(a.Start) || a.Contains(a.End) {
		t.Fatal("[start, end)")
	}
	c := Range{Start: a.End.Add(-time.Minute), End: b.End}
	if !a.Overlaps(c) {
		t.Fatal("one shared minute overlaps")
	}
}

func TestBillingDayDatesUseIstanbul(t *testing.T) {
	t.Parallel()
	// 22:30 UTC = 01:30 Istanbul the next day: the first billing day starts
	// on the local date.
	start := time.Date(2026, 5, 31, 22, 30, 0, 0, time.UTC)
	r, _ := NewRange(start, start.Add(2*Day))
	got := r.BillingDayDates()
	if len(got) != 2 || got[0].String() != "2026-06-01" || got[1].String() != "2026-06-02" {
		t.Fatalf("dates = %v", got)
	}
	if LocalDate(time.Date(2026, 8, 31, 20, 59, 0, 0, time.UTC)).String() != "2026-08-31" {
		t.Fatal("23:59 Istanbul is still Aug 31")
	}
	if LocalDate(time.Date(2026, 8, 31, 21, 0, 0, 0, time.UTC)).String() != "2026-09-01" {
		t.Fatal("00:00 Istanbul is Sep 1")
	}
	if Location().String() != "Europe/Istanbul" {
		t.Fatalf("location = %s", Location())
	}
}

func TestDateRangeEndExcluded(t *testing.T) {
	t.Parallel()
	// "YAZ" 1 June – 1 September: September 1 is off-season.
	yaz, err := NewDateRange(MustDate("2026-06-01"), MustDate("2026-09-01"))
	if err != nil {
		t.Fatal(err)
	}
	for d, want := range map[string]bool{
		"2026-05-31": false, "2026-06-01": true, "2026-08-31": true, "2026-09-01": false,
	} {
		if got := yaz.Contains(MustDate(d)); got != want {
			t.Errorf("YAZ contains %s = %v, want %v", d, got, want)
		}
	}
	if yaz.Days() != 92 {
		t.Fatalf("days = %d", yaz.Days())
	}
	autumn, _ := NewDateRange(MustDate("2026-09-01"), MustDate("2026-10-01"))
	if yaz.Overlaps(autumn) {
		t.Fatal("touching date ranges must not overlap")
	}
	if _, err := NewDateRange(MustDate("2026-06-01"), MustDate("2026-06-01")); err == nil {
		t.Fatal("empty date range must fail")
	}
}

func TestDateJSONAndHelpers(t *testing.T) {
	t.Parallel()
	var d Date
	if err := json.Unmarshal([]byte(`"2026-02-28"`), &d); err != nil {
		t.Fatal(err)
	}
	if d.AddDays(1).String() != "2026-03-01" || !d.Before(d.AddDays(1)) || d.After(d) {
		t.Fatalf("helpers: %s", d)
	}
	b, _ := json.Marshal(d)
	if string(b) != `"2026-02-28"` {
		t.Fatalf("json = %s", b)
	}
	if err := json.Unmarshal([]byte(`"28.02.2026"`), &d); err == nil {
		t.Fatal("non-ISO date must fail")
	}
	if NewDate(2026, 2, 30).String() != "2026-03-02" {
		t.Fatal("normalisation")
	}
	if got := MustDate("2026-06-01").StartIn().UTC(); !got.Equal(time.Date(2026, 5, 31, 21, 0, 0, 0, time.UTC)) {
		t.Fatalf("local midnight = %s", got)
	}
}
