package usecase

import (
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

func TestSplitLargestRemainder(t *testing.T) {
	for _, c := range []struct {
		amount  int64
		weights []int64
		want    []int64
	}{
		{1000, []int64{4, 2}, []int64{667, 333}},
		{4750, []int64{4, 2}, []int64{3167, 1583}},
		{50000, []int64{4, 2}, []int64{33333, 16667}},
		{-50000, []int64{4, 2}, []int64{-33333, -16667}},
		{100, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{60000, []int64{7, 2, 2}, []int64{38182, 10909, 10909}},
		{5, []int64{0, 3}, []int64{0, 5}},
	} {
		keys := make([]int64, len(c.weights))
		for i := range keys {
			keys[i] = int64(i + 1)
		}
		got := split(c.amount, keys, c.weights)
		var sum int64
		for i, k := range keys {
			sum += got[k]
			if got[k] != c.want[i] {
				t.Errorf("split(%d, %v)[%d] = %d, want %d", c.amount, c.weights, i, got[k], c.want[i])
			}
		}
		if sum != c.amount {
			t.Errorf("split(%d, %v) sums to %d", c.amount, c.weights, sum)
		}
	}
	if split(10, []int64{1, 2}, []int64{0, 0}) != nil {
		t.Error("no weight must answer nil")
	}
}

func TestRatioRounding(t *testing.T) {
	for _, c := range []struct{ v, num, den, want int64 }{
		{600000, 60000, 600000, 60000},
		{12000, 631000, 631000, 12000},
		{5, 1, 2, 3},
		{-5, 1, 2, -3},
		{7, 0, 3, 0},
		{7, 1, 0, 0},
	} {
		if got := ratio(c.v, c.num, c.den); got != c.want {
			t.Errorf("ratio(%d, %d, %d) = %d, want %d", c.v, c.num, c.den, got, c.want)
		}
	}
}

func TestIntervals(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 3, 1, h, 0, 0, 0, time.UTC) }
	m := merge([]interval{{at(5), at(8)}, {at(1), at(3)}, {at(2), at(4)}, {at(8), at(9)}, {at(6), at(6)}})
	if len(m) != 2 || !m[0].start.Equal(at(1)) || !m[0].end.Equal(at(4)) || !m[1].end.Equal(at(9)) {
		t.Fatalf("merge = %+v", m)
	}
	if got := overlap(m, []interval{{at(3), at(6)}}); got != 2*3600 {
		t.Errorf("overlap = %d", got)
	}
}

func TestMonthBuckets(t *testing.T) {
	b := monthBuckets(rentaltime.MustDate("2025-12-15"), rentaltime.MustDate("2026-02-10"))
	if len(b) != 3 || b[0].key != "2025-12" || b[0].from.String() != "2025-12-15" || b[0].to.String() != "2025-12-31" ||
		b[2].key != "2026-02" || b[2].to.String() != "2026-02-10" {
		t.Fatalf("buckets %+v", b)
	}
	// Istanbul midnight is 21:00 UTC the day before.
	if !b[1].start.Equal(time.Date(2025, 12, 31, 21, 0, 0, 0, time.UTC)) {
		t.Errorf("January starts at %s", b[1].start)
	}
}

// TestSwapSplitAndOpenLeg: a swap splits by leg time; an open leg runs to
// the later of now and the planned return.
func TestSwapSplitAndOpenLeg(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 3, d, 7, 0, 0, 0, time.UTC) }
	r := rentalFact{id: 1, vehicle: 2, endsAt: day(10)}
	in := day(5)
	keys, w := legShares(r, []legFact{{rental: 1, vehicle: 1, out: day(1), in: &in}, {rental: 1, vehicle: 2, out: day(5)}}, day(7))
	if len(keys) != 2 || w[0] != 4*86400 || w[1] != 5*86400 {
		t.Fatalf("open leg shares %v %v", keys, w)
	}
	keys, w = legShares(r, nil, day(7))
	if len(keys) != 1 || keys[0] != 2 || w[0] != 1 {
		t.Fatalf("no legs: %v %v", keys, w)
	}
}

// TestAllocationFallback: with no rental days in a pool, the overhead is
// shared by vehicle count and the method says so; a pool without vehicles
// stays unallocated.
func TestAllocationFallback(t *testing.T) {
	from, to := rentaltime.MustDate("2026-03-01"), rentaltime.MustDate("2026-03-31")
	svc := interval{from.StartIn().UTC(), farFuture}
	in := input{
		basis: model.BasisAccrual, allocation: model.AllocationRentalDays, from: from, to: to,
		now: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), scopeAll: true,
		vehicles: []vehicleFact{{id: 1, home: 10, current: 10, service: svc}, {id: 2, home: 10, current: 10, service: svc}},
		expenses: []costFact{
			{branch: 10, amount: 1001, day: rentaltime.MustDate("2026-03-05"), code: "operations"},
			{branch: 20, amount: 500, day: rentaltime.MustDate("2026-03-05"), code: "operations"},
		},
	}
	res := compute(in)
	if !res.fallback || res.unallocated != 500 {
		t.Fatalf("fallback %v unallocated %d", res.fallback, res.unallocated)
	}
	if a, b := res.vehicles[1].buckets[0].money[fOverhead], res.vehicles[2].buckets[0].money[fOverhead]; a != 501 || b != 500 {
		t.Errorf("overhead %d / %d", a, b)
	}
}
