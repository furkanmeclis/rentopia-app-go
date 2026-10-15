package usecase

import (
	"testing"
	"time"
)

func TestPeakSweepLine(t *testing.T) {
	t0 := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Hour) }
	list := []interval{
		{start: h(0), end: h(10), qty: 1},
		{start: h(5), end: h(15), qty: 2},
		// Starts exactly when the first ends: half-open, no overlap with it.
		{start: h(10), end: h(12), qty: 1},
		{start: h(20), end: h(30), qty: 4},
	}
	cases := []struct {
		name     string
		from, to time.Time
		want     int
	}{
		{"whole window", h(0), h(40), 4},
		{"first two overlap", h(0), h(10), 3},
		{"handover at 10 is not double", h(9), h(12), 3},
		{"after the overlap", h(12), h(20), 2},
		{"empty gap", h(15), h(20), 0},
		{"clipped big one", h(29), h(31), 4},
		{"window ends where the big one starts", h(15), h(20), 0},
	}
	for _, c := range cases {
		if got := peak(list, c.from, c.to); got != c.want {
			t.Errorf("%s: peak = %d, want %d", c.name, got, c.want)
		}
	}
	if peak(nil, h(0), h(1)) != 0 {
		t.Fatal("empty list")
	}
}
