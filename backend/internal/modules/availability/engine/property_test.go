package engine

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"
)

// Property test: random sequences of reserve / cancel / date change /
// plate assignment over overlapping products, pools, blocks, transfers and
// one-way returns. After every operation the state must be valid (Verify:
// no double booking, every commitment on an eligible vehicle in its pool,
// no block / transfer collision, location continuity) — the plate plan is
// the witness that all commitments are satisfiable together. Every answer
// is compared with a brute-force search over all assignments: an accepted
// operation must be feasible (trivially, it produced a witness) and a
// refused one is a "miss" when brute force finds an assignment. Misses are
// allowed by design (the reassignment search is bounded) but must stay rare;
// the test reports the rate and fails above 1.5 % (typical: 0.4-0.7 %; the
// known gap is relocation through another commitment, e.g. moving a one-way
// sale onto a vehicle only to bring it to the requested pick-up branch).

type world struct {
	s        *State
	keys     []string
	branches []int64
	nextID   int64
}

func newWorld(rng *rand.Rand) *world {
	nVeh := 2 + rng.Intn(3) // 2..4
	s := NewState(h(0))
	w := &world{s: s, branches: []int64{1, 2}, nextID: 1}
	for i := 1; i <= nVeh; i++ {
		pool := "MAIN"
		if rng.Intn(5) == 0 {
			pool = "P2"
		}
		v := &Vehicle{ID: int64(i), Pool: pool, Branch: w.branches[rng.Intn(2)], Service: always()}
		if rng.Intn(6) == 0 {
			v.Service = Interval{Start: time.Time{}, End: h(30 + rng.Intn(40))}
		}
		// Up to one block and one transfer, placed at random.
		if rng.Intn(3) == 0 {
			a := 1 + rng.Intn(60)
			v.Segments = append(v.Segments, Segment{Kind: SegmentBlock, Ref: fmt.Sprintf("b%d", i), Period: iv(a, a+1+rng.Intn(8))})
		}
		if rng.Intn(3) == 0 {
			a := 1 + rng.Intn(60)
			p := iv(a, a+1+rng.Intn(4))
			overlaps := false
			for _, seg := range v.Segments {
				overlaps = overlaps || seg.Period.Overlaps(p)
			}
			if !overlaps {
				to := int64(3) - v.Branch
				// The transfer starts where the vehicle is (no earlier movement).
				v.Segments = append(v.Segments, Segment{Kind: SegmentTransfer, Ref: fmt.Sprintf("t%d", i), Period: p, From: v.Branch, To: to})
			}
		}
		s.Vehicles[v.ID] = v
	}
	// 2..3 products with random, overlapping eligible sets (never empty).
	nKeys := 2 + rng.Intn(2)
	for k := 0; k < nKeys; k++ {
		key := "K" + strconv.Itoa(k)
		set := map[int64]bool{}
		for id := range s.Vehicles {
			if rng.Intn(3) != 0 {
				set[id] = true
			}
		}
		if len(set) == 0 {
			set[int64(1+rng.Intn(nVeh))] = true
		}
		s.Eligible[key] = set
		w.keys = append(w.keys, key)
	}
	s.Invalidate()
	return w
}

func (w *world) randomRequest(rng *rand.Rand) Request {
	a := 1 + rng.Intn(60)
	b := a + 1 + rng.Intn(12)
	prep := rng.Intn(3)
	pickup := w.branches[rng.Intn(2)]
	ret := pickup
	if rng.Intn(4) == 0 {
		ret = 3 - pickup
	}
	r := Request{Key: w.keys[rng.Intn(len(w.keys))], Period: iv(a, b+prep), End: h(b), Pickup: pickup, Return: ret}
	if rng.Intn(5) == 0 {
		r.Hold = true
	}
	return r
}

// feasible reports whether every commitment (plus extra, when not nil) can
// be placed together, by exhaustive search. Fixed commitments keep their
// vehicle; pools of existing commitments are pinned.
func feasible(s *State, extra *Request, allowed map[string]bool) bool {
	type item struct {
		c      Commitment
		domain []int64
	}
	items := []item{}
	add := func(c Commitment, fixedVehicle int64, only []int64) {
		dom := []int64{}
		for id := range s.Eligible[c.Key] {
			v := s.Vehicles[id]
			if fixedVehicle != 0 && id != fixedVehicle {
				continue
			}
			if only != nil && !containsID(only, id) {
				continue
			}
			if !s.fits(v, &c) {
				continue
			}
			dom = append(dom, id)
		}
		sort.Slice(dom, func(i, j int) bool { return dom[i] < dom[j] })
		items = append(items, item{c: c, domain: dom})
	}
	for _, c := range s.Commitments {
		if extra != nil && extra.Self == c.ID {
			cp := *c
			cp.Period, cp.End, cp.Pickup, cp.Return = extra.Period, extra.End, extra.Pickup, extra.Return
			fixed := int64(0)
			if c.Fixed {
				fixed = c.Vehicle
			}
			add(cp, fixed, extra.Vehicles)
			continue
		}
		fixed := int64(0)
		if c.Fixed || !c.Period.Start.After(s.Now) || allowed[eventID(c.ID)] {
			fixed = c.Vehicle
		}
		if allowed[eventID(c.ID)] {
			// Affected: stays where it is, whatever its problems.
			items = append(items, item{c: *c, domain: []int64{c.Vehicle}})
			continue
		}
		add(*c, fixed, nil)
	}
	if extra != nil && extra.Self == 0 {
		add(Commitment{ID: tempID, Key: extra.Key, Pool: extra.Pool, Period: extra.Period, End: extra.End,
			Pickup: extra.Pickup, Return: extra.Return}, 0, extra.Vehicles)
	}
	sort.Slice(items, func(i, j int) bool { return len(items[i].domain) < len(items[j].domain) })
	assign := make([]int64, len(items))
	var rec func(i int) bool
	rec = func(i int) bool {
		if i == len(items) {
			st := &State{Now: s.Now, Vehicles: s.Vehicles, Eligible: s.Eligible, Commitments: map[int64]*Commitment{}}
			for j, it := range items {
				c := it.c
				c.Vehicle = assign[j]
				st.Commitments[c.ID] = &c
			}
			st.index()
			for v := range st.Vehicles {
				for id := range st.violations(v) {
					if !allowed[id] {
						return false
					}
				}
			}
			return true
		}
		for _, v := range items[i].domain {
			clash := false
			for j := 0; j < i; j++ {
				if assign[j] == v && items[j].c.Period.Overlaps(items[i].c.Period) {
					clash = true
					break
				}
			}
			if clash {
				continue
			}
			assign[i] = v
			if rec(i + 1) {
				return true
			}
		}
		return false
	}
	return rec(0)
}

func containsID(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestRandomSequencesStaySatisfiable(t *testing.T) {
	seed := time.Now().UnixNano()
	if env := os.Getenv("AVAILABILITY_PROPERTY_SEED"); env != "" {
		seed, _ = strconv.ParseInt(env, 10, 64)
	}
	rounds := 400
	if testing.Short() {
		rounds = 60
	}
	rng := rand.New(rand.NewSource(seed))
	t.Logf("seed %d", seed)
	var accepted, refused, misses, ops, releases, repaired, affectedCount int
	for round := 0; round < rounds; round++ {
		w := newWorld(rng)
		allowed := map[string]bool{}
		for step := 0; step < 25; step++ {
			ops++
			s := w.s
			ids := make([]int64, 0, len(s.Commitments))
			for id := range s.Commitments {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			op := rng.Intn(10)
			var r Request
			switch {
			case op < 6 || len(ids) == 0: // reserve
				r = w.randomRequest(rng)
				if rng.Intn(8) == 0 {
					r.Fixed = true
					r.Vehicles = []int64{int64(1 + rng.Intn(len(s.Vehicles)))}
				}
			case op < 7: // cancel (always succeeds; dependents are re-planned)
				res := s.Release(ids[rng.Intn(len(ids))], Options{})
				releases++
				for _, m := range res.Moves {
					if m.From != m.To {
						repaired++
					}
				}
				for _, a := range res.Affected {
					allowed[a] = true
					affectedCount++
				}
				if p := s.Verify(allowed); len(p) > 0 {
					t.Fatalf("seed %d round %d: invalid after cancel: %v", seed, round, p)
				}
				continue
			case op < 9: // date change
				c := s.Commitments[ids[rng.Intn(len(ids))]]
				r = w.randomRequest(rng)
				r.Key, r.Self, r.Hold = c.Key, c.ID, false
				if c.Fixed {
					r.Vehicles, r.Fixed = []int64{c.Vehicle}, true
				}
			default: // plate assignment
				c := s.Commitments[ids[rng.Intn(len(ids))]]
				if c.Fixed {
					continue
				}
				r = Request{Key: c.Key, Period: c.Period, End: c.End, Pickup: c.Pickup, Return: c.Return, Self: c.ID,
					Vehicles: []int64{int64(1 + rng.Intn(len(s.Vehicles)))}, Fixed: true}
			}
			before := s.Clone()
			plan, ok := s.Place(r, Options{})
			if !ok {
				refused++
				if feasible(s, &r, allowed) {
					misses++
				}
				// Refused: nothing changed.
				for id, c := range before.Commitments {
					if *s.Commitments[id] != *c {
						t.Fatalf("seed %d round %d: refused plan changed commitment %d", seed, round, id)
					}
				}
				continue
			}
			accepted++
			id := int64(0)
			if r.Self == 0 {
				id = w.nextID
				w.nextID++
			}
			s.Apply(r, plan, id)
			if p := s.Verify(allowed); len(p) > 0 {
				t.Fatalf("seed %d round %d step %d: invalid after %+v plan %+v: %v", seed, round, step, r, plan, p)
			}
			// A re-planned affected commitment is valid again.
			if r.Self != 0 {
				delete(allowed, eventID(r.Self))
			}
			if !feasible(s, nil, allowed) {
				t.Fatalf("seed %d round %d: brute force finds the accepted state unsatisfiable", seed, round)
			}
			for _, m := range plan.Moves {
				if before.Commitments[m.Commitment].Fixed {
					t.Fatalf("seed %d: plan moved a fixed plate", seed)
				}
				if before.Commitments[m.Commitment].Pool != s.Commitments[m.Commitment].Pool {
					t.Fatalf("seed %d: plan moved a commitment across pools", seed)
				}
			}
		}
	}
	t.Logf("ops %d, accepted %d, refused %d, refused although feasible %d; releases %d (dependents moved %d, left affected %d)",
		ops, accepted, refused, misses, releases, repaired, affectedCount)
	if accepted == 0 || refused == 0 {
		t.Fatalf("degenerate run: accepted %d refused %d", accepted, refused)
	}
	if float64(misses) > 0.015*float64(refused) {
		t.Fatalf("seed %d: %d of %d refusals were feasible (reassignment search too weak)", seed, misses, refused)
	}
}
