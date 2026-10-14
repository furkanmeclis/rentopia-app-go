package usecase

import (
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestResolveLocation(t *testing.T) {
	t.Parallel()
	const a, b, c = 1, 2, 3
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return now.Add(time.Duration(n) * time.Hour) }

	cases := []struct {
		name    string
		current int64
		moves   []Movement
		at      time.Time
		want    Location
	}{
		{"no movement: at the current branch now", a, nil, now, Location{BranchID: a}},
		{"no movement: never somewhere else by itself", a, nil, h(500), Location{BranchID: a}},
		{"no movement in the past", a, nil, h(-500), Location{BranchID: a}},
		{
			"RT-A05: picked at A, returned at B -> at B after the return", b,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-5), Arrival: h(-2), State: MoveDone}},
			h(-1), Location{BranchID: b},
		},
		{
			"RT-A05: at A before the pick-up", b,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-5), Arrival: h(-2), State: MoveDone}},
			h(-6), Location{BranchID: a},
		},
		{
			"RT-A05: on the road between pick-up and return", b,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-5), Arrival: h(-2), State: MoveDone}},
			h(-3), Location{BranchID: b, InTransit: true, FromBranchID: a},
		},
		{
			"arrival instant is at the destination (half-open)", b,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-5), Arrival: h(-2), State: MoveDone}},
			h(-2), Location{BranchID: b},
		},
		{
			"planned transfer: still at A before departure", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(2), Arrival: h(4), State: MovePlanned}},
			h(1), Location{BranchID: a},
		},
		{
			"planned transfer: in transit during the window", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(2), Arrival: h(4), State: MovePlanned}},
			h(3), Location{BranchID: b, InTransit: true, FromBranchID: a},
		},
		{
			"planned transfer: at B after arrival", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(2), Arrival: h(4), State: MovePlanned}},
			h(5), Location{BranchID: b},
		},
		{
			"two planned transfers chain A -> B -> C", a,
			[]Movement{
				{FromBranchID: b, ToBranchID: c, Departure: h(10), Arrival: h(12), State: MovePlanned},
				{FromBranchID: a, ToBranchID: b, Departure: h(2), Arrival: h(4), State: MovePlanned},
			},
			h(13), Location{BranchID: c},
		},
		{
			"in transit now: still on the road before planned arrival", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-1), Arrival: h(3), State: MoveInTransit}},
			h(1), Location{BranchID: b, InTransit: true, FromBranchID: a},
		},
		{
			"in transit now: expected at B after planned arrival", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-1), Arrival: h(3), State: MoveInTransit}},
			h(4), Location{BranchID: b},
		},
		{
			"in transit now: was in transit just after departure", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-2), Arrival: h(3), State: MoveInTransit}},
			h(-1), Location{BranchID: b, InTransit: true, FromBranchID: a},
		},
		{
			"in transit now: at A before departure", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-2), Arrival: h(3), State: MoveInTransit}},
			h(-3), Location{BranchID: a},
		},
		{
			"done movements unwound newest first", c,
			[]Movement{
				{FromBranchID: a, ToBranchID: b, Departure: h(-10), Arrival: h(-9), State: MoveDone},
				{FromBranchID: b, ToBranchID: c, Departure: h(-5), Arrival: h(-4), State: MoveDone},
			},
			h(-7), Location{BranchID: b},
		},
		{
			"done movements: before both", c,
			[]Movement{
				{FromBranchID: a, ToBranchID: b, Departure: h(-10), Arrival: h(-9), State: MoveDone},
				{FromBranchID: b, ToBranchID: c, Departure: h(-5), Arrival: h(-4), State: MoveDone},
			},
			h(-11), Location{BranchID: a},
		},
		{
			"planned movements never happened in the past", a,
			[]Movement{{FromBranchID: a, ToBranchID: b, Departure: h(-5), Arrival: h(-4), State: MovePlanned}},
			h(-3), Location{BranchID: a},
		},
	}
	for _, tc := range cases {
		if got := ResolveLocation(tc.current, now, tc.moves, tc.at); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestMovementsFromTransfers(t *testing.T) {
	t.Parallel()
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	rows := []db.ListVehicleMovementsRow{
		{FromBranchID: 1, ToBranchID: 2, PlannedDeparture: ts(base), PlannedArrival: ts(base.Add(time.Hour))},
		// Started late: the actual departure is after the planned arrival.
		{FromBranchID: 2, ToBranchID: 3, PlannedDeparture: ts(base), PlannedArrival: ts(base.Add(time.Hour)),
			ActualDeparture: ts(base.Add(2 * time.Hour))},
		{FromBranchID: 3, ToBranchID: 1, PlannedDeparture: ts(base), PlannedArrival: ts(base.Add(time.Hour)),
			ActualDeparture: ts(base.Add(time.Minute)), ActualArrival: ts(base.Add(30 * time.Minute))},
	}
	got := MovementsFromTransfers(rows)
	if got[0].State != MovePlanned || !got[0].Arrival.Equal(base.Add(time.Hour)) {
		t.Errorf("planned = %+v", got[0])
	}
	if got[1].State != MoveInTransit || got[1].Arrival.Before(got[1].Departure) {
		t.Errorf("in transit = %+v", got[1])
	}
	if got[2].State != MoveDone || !got[2].Arrival.Equal(base.Add(30*time.Minute)) {
		t.Errorf("done = %+v", got[2])
	}
}
