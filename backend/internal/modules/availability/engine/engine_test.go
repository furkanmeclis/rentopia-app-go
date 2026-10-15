package engine

import (
	"testing"
	"time"
)

var t0 = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

func h(n int) time.Time { return t0.Add(time.Duration(n) * time.Hour) }

func iv(a, b int) Interval { return Interval{Start: h(a), End: h(b)} }

func always() Interval { return Interval{Start: time.Time{}, End: Forever} }

// fleet builds n vehicles at branch 1 in pool MAIN with ids 1..n.
func fleet(n int) *State {
	s := NewState(h(-24))
	for i := 1; i <= n; i++ {
		s.Vehicles[int64(i)] = &Vehicle{ID: int64(i), Pool: "MAIN", Branch: 1, Service: always()}
	}
	return s
}

func eligible(s *State, key string, ids ...int64) {
	set := map[int64]bool{}
	for _, id := range ids {
		set[id] = true
	}
	s.Eligible[key] = set
	s.Invalidate()
}

func req(key string, a, b int) Request {
	return Request{Key: key, Period: iv(a, b+1), End: h(b), Pickup: 1, Return: 1}
}

func reserve(t *testing.T, s *State, r Request, id int64) bool {
	t.Helper()
	plan, ok := s.Place(r, Options{})
	if !ok {
		return false
	}
	s.Apply(r, plan, id)
	if problems := s.Verify(nil); len(problems) > 0 {
		t.Fatalf("invalid state after reserve %d: %v", id, problems)
	}
	return true
}

func TestCapacityOfOneProduct(t *testing.T) {
	s := fleet(30)
	ids := []int64{}
	for i := int64(1); i <= 30; i++ {
		ids = append(ids, i)
	}
	eligible(s, "P", ids...)
	b := s.Capacity(req("P", 10, 20), 0, Options{})
	if b.Eligible != 30 || b.Free != 30 || b.Physical != 30 {
		t.Fatalf("breakdown = %+v, want 30 eligible / free / physical", b)
	}
}

func TestContinuousCoverage(t *testing.T) {
	// Two vehicles, each free for half the range: no single vehicle covers
	// the whole range, so nothing is sellable ("a free car each day" is not
	// availability).
	s := fleet(2)
	eligible(s, "P", 1, 2)
	s.Vehicles[1].Segments = []Segment{{Kind: SegmentBlock, Ref: "b1", Period: iv(15, 40)}}
	s.Vehicles[2].Segments = []Segment{{Kind: SegmentBlock, Ref: "b2", Period: iv(0, 14)}}
	if _, ok := s.Place(req("P", 10, 20), Options{}); ok {
		t.Fatal("a range covered only by two half-free vehicles was accepted")
	}
	b := s.Capacity(req("P", 10, 20), 0, Options{})
	if b.Blocked != 2 || b.Physical != 0 {
		t.Fatalf("breakdown = %+v", b)
	}
}

func TestOneWayReturnMovesTheVehicle(t *testing.T) {
	// RT-A05 at engine level: picked up at 1, returned at 2 → after the
	// preparation buffer it is sellable at 2, not at 1.
	s := fleet(1)
	eligible(s, "P", 1)
	oneWay := Request{Key: "P", Period: Interval{h(10), h(20).Add(time.Hour)}, End: h(20), Pickup: 1, Return: 2}
	if !reserve(t, s, oneWay, 1) {
		t.Fatal("one-way reserve refused")
	}
	atB := Request{Key: "P", Period: iv(22, 31), End: h(30), Pickup: 2, Return: 2}
	atA := Request{Key: "P", Period: iv(22, 31), End: h(30), Pickup: 1, Return: 1}
	duringPrep := Request{Key: "P", Period: Interval{h(20).Add(30 * time.Minute), h(31)}, End: h(30), Pickup: 2, Return: 2}
	if b := s.Capacity(atB, 0, Options{}); b.Physical != 1 || b.Free != 1 {
		t.Fatalf("at the return branch after preparation: %+v", b)
	}
	if b := s.Capacity(atA, 0, Options{}); b.Physical != 0 || b.Elsewhere != 1 {
		t.Fatalf("at the pick-up branch after a one-way return: %+v", b)
	}
	if b := s.Capacity(duringPrep, 0, Options{}); b.Physical != 0 || b.Held+b.Reserved != 1 {
		t.Fatalf("inside the preparation buffer: %+v", b)
	}
	// A request before the one-way commitment that would return elsewhere
	// breaks it (continuity of the later commitment).
	before := Request{Key: "P", Period: iv(0, 6), End: h(5), Pickup: 1, Return: 2}
	if _, ok := s.Place(before, Options{}); ok {
		t.Fatal("an earlier one-way return that strands a later pick-up was accepted")
	}
	roundTrip := Request{Key: "P", Period: iv(0, 6), End: h(5), Pickup: 1, Return: 1}
	if _, ok := s.Place(roundTrip, Options{}); !ok {
		t.Fatal("an earlier round trip was refused")
	}
}

func TestBlockInTheMiddle(t *testing.T) {
	// RT-A06: a block in the middle of the range makes the vehicle unsellable.
	s := fleet(1)
	eligible(s, "P", 1)
	s.Vehicles[1].Segments = []Segment{{Kind: SegmentBlock, Ref: "b", Period: iv(14, 16)}}
	if b := s.Capacity(req("P", 10, 20), 0, Options{}); b.Physical != 0 || b.Blocked != 1 {
		t.Fatalf("breakdown = %+v", b)
	}
	if b := s.Capacity(req("P", 17, 20), 0, Options{}); b.Physical != 1 {
		t.Fatalf("after the block: %+v", b)
	}
}

func TestTransferMovesTheVehicle(t *testing.T) {
	s := fleet(1)
	eligible(s, "P", 1)
	s.Vehicles[1].Segments = []Segment{{Kind: SegmentTransfer, Ref: "t", Period: iv(5, 8), From: 1, To: 2}}
	if _, ok := s.Place(Request{Key: "P", Period: iv(10, 12), End: h(11), Pickup: 2, Return: 2}, Options{}); !ok {
		t.Fatal("refused at the transfer destination")
	}
	if _, ok := s.Place(Request{Key: "P", Period: iv(10, 12), End: h(11), Pickup: 1, Return: 1}, Options{}); ok {
		t.Fatal("accepted at the origin after the transfer left")
	}
	// A one-way sale before the transfer that leaves the vehicle elsewhere
	// would strand the planned transfer.
	if _, ok := s.Place(Request{Key: "P", Period: iv(1, 3), End: h(2), Pickup: 1, Return: 3}, Options{}); ok {
		t.Fatal("a one-way return before a planned transfer from the origin was accepted")
	}
}

func TestExactModelNotSoldOnOtherModel(t *testing.T) {
	// Report §15: a model-guaranteed product is not sold when only a
	// different model is free.
	s := fleet(2) // 1 = Egea, 2 = Clio
	eligible(s, "EGEA", 1)
	eligible(s, "COMPACT", 1, 2)
	if !reserve(t, s, req("COMPACT", 10, 20), 1) {
		t.Fatal("compact refused")
	}
	// The compact took the Clio (least flexible vehicle first), so the
	// Egea product is still sellable.
	if s.Commitments[1].Vehicle != 2 {
		t.Fatalf("broad product took vehicle %d, want the Clio", s.Commitments[1].Vehicle)
	}
	if !reserve(t, s, req("EGEA", 10, 20), 2) {
		t.Fatal("egea refused while the Egea was free")
	}
	if reserve(t, s, req("EGEA", 12, 14), 3) {
		t.Fatal("egea sold twice with one Egea")
	}
	// Only the Clio is free: the Egea product is not sold on it.
	s2 := fleet(2)
	eligible(s2, "EGEA", 1)
	eligible(s2, "COMPACT", 1, 2)
	s2.Vehicles[1].Segments = []Segment{{Kind: SegmentBlock, Ref: "b", Period: iv(0, 40)}}
	if b := s2.Capacity(req("EGEA", 10, 20), 0, Options{}); b.Physical != 0 || b.Eligible != 1 {
		t.Fatalf("egea with only a Clio free: %+v", b)
	}
	if b := s2.Capacity(req("COMPACT", 10, 20), 0, Options{}); b.Physical != 1 {
		t.Fatalf("compact with the Clio free: %+v", b)
	}
}

func TestReassignmentMakesRoom(t *testing.T) {
	// The general group and the exact model share the Egea. The compact
	// sale sits on the Egea (forced: the Clio is blocked then); once the
	// Clio is free again the Egea sale is accepted by moving the compact.
	s := fleet(2)
	eligible(s, "EGEA", 1)
	eligible(s, "COMPACT", 1, 2)
	s.Vehicles[2].Segments = []Segment{{Kind: SegmentBlock, Ref: "b", Period: iv(0, 5)}}
	if !reserve(t, s, req("COMPACT", 10, 20), 1) {
		t.Fatal("compact refused")
	}
	// Force the compact onto the Egea (as if sold when the Clio was busy).
	s.Commitments[1].Vehicle = 1
	s.Invalidate()
	plan, ok := s.Place(req("EGEA", 12, 18), Options{})
	if !ok {
		t.Fatal("egea refused although moving the compact to the Clio makes room")
	}
	if plan.Vehicle != 1 || len(plan.Moves) != 1 || plan.Moves[0] != (Move{Commitment: 1, From: 1, To: 2}) {
		t.Fatalf("plan = %+v", plan)
	}
	s.Apply(req("EGEA", 12, 18), plan, 2)
	if problems := s.Verify(nil); len(problems) > 0 {
		t.Fatal(problems)
	}
	// Now both are committed; a second egea sale cannot be satisfied
	// together with everything else.
	if _, ok := s.Place(req("EGEA", 11, 13), Options{}); ok {
		t.Fatal("accepted a sale that cannot be satisfied with the existing commitments")
	}
	// A fixed plate never moves.
	s2 := fleet(2)
	eligible(s2, "EGEA", 1)
	eligible(s2, "COMPACT", 1, 2)
	s2.Commitments[1] = &Commitment{ID: 1, Key: "COMPACT", Pool: "MAIN", Period: iv(10, 21), End: h(20), Pickup: 1, Return: 1, Vehicle: 1, Fixed: true}
	s2.Invalidate()
	if _, ok := s2.Place(req("EGEA", 12, 18), Options{}); ok {
		t.Fatal("moved a fixed plate")
	}
}

func TestChangeDatesKeepsOldWhenFull(t *testing.T) {
	s := fleet(1)
	eligible(s, "P", 1)
	if !reserve(t, s, req("P", 10, 20), 1) || !reserve(t, s, req("P", 30, 40), 2) {
		t.Fatal("setup refused")
	}
	change := req("P", 25, 35)
	change.Self = 1
	if _, ok := s.Place(change, Options{}); ok {
		t.Fatal("date change into a full period accepted")
	}
	if c := s.Commitments[1]; c.Period != iv(10, 21) || c.Vehicle != 1 {
		t.Fatalf("old commitment changed: %+v", c)
	}
	// Moving inside its own old range (overlapping itself) is fine.
	change = req("P", 12, 26)
	change.Self = 1
	plan, ok := s.Place(change, Options{})
	if !ok {
		t.Fatal("date change overlapping only itself refused")
	}
	s.Apply(change, plan, 0)
	if problems := s.Verify(nil); len(problems) > 0 || s.Commitments[1].Period != iv(12, 27) {
		t.Fatalf("after change: %v %+v", problems, s.Commitments[1])
	}
}

func TestAssignPlateMovesOthers(t *testing.T) {
	s := fleet(2)
	eligible(s, "P", 1, 2)
	if !reserve(t, s, req("P", 10, 20), 1) || !reserve(t, s, req("P", 10, 20), 2) {
		t.Fatal("setup refused")
	}
	target := s.Commitments[2].Vehicle
	if target == s.Commitments[1].Vehicle {
		t.Fatal("double booking")
	}
	assign := req("P", 10, 20)
	assign.Self = 1
	assign.Vehicles = []int64{target}
	assign.Fixed = true
	plan, ok := s.Place(assign, Options{})
	if !ok || plan.Vehicle != target || len(plan.Moves) != 1 {
		t.Fatalf("plate assignment plan = %+v ok=%v", plan, ok)
	}
	s.Apply(assign, plan, 0)
	if !s.Commitments[1].Fixed || s.Commitments[1].Vehicle != target || len(s.Commitments) != 2 {
		t.Fatalf("after assignment: %+v (count %d)", s.Commitments[1], len(s.Commitments))
	}
	if problems := s.Verify(nil); len(problems) > 0 {
		t.Fatal(problems)
	}
}

func TestPoolsNeverShare(t *testing.T) {
	s := fleet(2)
	s.Vehicles[2].Pool = "PARTNER"
	eligible(s, "P", 1, 2)
	if !reserve(t, s, req("P", 10, 20), 1) {
		t.Fatal("refused")
	}
	pool := s.Commitments[1].Pool
	r := req("P", 12, 14)
	r.Pool = pool
	if _, ok := s.Place(r, Options{}); ok {
		t.Fatalf("second sale in pool %s accepted with one vehicle in it", pool)
	}
	if b := s.Capacity(req("P", 12, 14), 0, Options{}); b.Physical != 1 {
		t.Fatalf("any pool: %+v", b)
	}
}
