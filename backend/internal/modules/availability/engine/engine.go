// Package engine is the pure planning core of the availability engine
// (TEC-697, docs/design/availability-engine.md). It knows nothing about the
// database: the service loads a State (vehicles with their blocks and
// transfers, the eligibility of every guarantee, the active commitments with
// their provisional plate plan) inside the component lock, asks the engine
// for a plan, and writes the plan back in the same transaction.
//
// Invariants of a valid State (Verify):
//   - every commitment sits on a vehicle that satisfies its guarantee, is in
//     its pool and in service for its whole range;
//   - two commitments of a vehicle never overlap (period includes the
//     preparation buffer); no commitment overlaps a block or transfer;
//   - location continuity: a commitment (or planned transfer) that starts in
//     the future starts where the vehicle is at that time (current branch,
//     then the branch after each earlier movement / commitment).
//
// Commitments that already violate an invariant (a block recorded later, a
// vehicle edited so it no longer matches) are "affected": a plan never
// creates a new violation and never needs to repair an old one.
package engine

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// Forever is the upper bound of an unbounded interval.
var Forever = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// Interval is a half-open time range [Start, End).
type Interval struct {
	Start, End time.Time
}

// Overlaps reports whether two half-open intervals share an instant.
func (a Interval) Overlaps(b Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

// Covers reports whether a contains b.
func (a Interval) Covers(b Interval) bool {
	return !b.Start.Before(a.Start) && !b.End.After(a.End)
}

// Segment kinds.
const (
	SegmentBlock    = "block"
	SegmentTransfer = "transfer"
)

// Segment is something on a vehicle's timeline that is not a commitment: a
// block or an open transfer. Segments are hard constraints.
type Segment struct {
	Kind   string
	Ref    string
	Period Interval
	// From is the origin a planned transfer requires (0 = none); To is
	// where the vehicle is after the segment (0 = unchanged).
	From int64
	To   int64
}

// Vehicle is a physical vehicle as the engine sees it.
type Vehicle struct {
	ID   int64
	Pool string
	// Branch is the current branch at State.Now (it already reflects every
	// completed movement).
	Branch int64
	// Service is the window in which the vehicle counts
	// ([in_service_date, out_of_service_date]); {zero, Forever} = always.
	Service  Interval
	Segments []Segment
}

// Commitment is an active allocation: a sold unit with its provisional
// plate plan (Vehicle).
type Commitment struct {
	ID int64
	// Key names the guarantee (State.Eligible) the commitment keeps.
	Key  string
	Pool string
	// Period is [start, end + preparation); End is the customer end.
	Period Interval
	End    time.Time
	Pickup int64
	Return int64
	// Vehicle is the provisional (or fixed) plate; 0 while floating inside
	// a plan.
	Vehicle int64
	// Fixed: the plate is promised (never moved by a plan).
	Fixed bool
	// Hold: a timed option (breakdown only).
	Hold bool
}

// customer is the range the vehicle must be in service for.
func (c *Commitment) customer() Interval {
	return Interval{Start: c.Period.Start, End: c.End}
}

// State is everything the engine needs to plan inside one component.
type State struct {
	Now      time.Time
	Vehicles map[int64]*Vehicle
	// Eligible maps a guarantee key to the vehicles that satisfy it.
	Eligible    map[string]map[int64]bool
	Commitments map[int64]*Commitment
	// FlexKeys, when set, are the guarantees that count for a vehicle's
	// flexibility (the products on sale); nil = every key.
	FlexKeys map[string]bool

	byVehicle map[int64][]*Commitment
	flex      map[int64]int
	indexed   bool
}

// NewState builds an empty state.
func NewState(now time.Time) *State {
	return &State{
		Now: now, Vehicles: map[int64]*Vehicle{}, Eligible: map[string]map[int64]bool{},
		Commitments: map[int64]*Commitment{},
	}
}

// index (re)builds the per-vehicle commitment lists and the flexibility of
// every vehicle (how many guarantees accept it).
func (s *State) index() {
	if s.indexed {
		return
	}
	s.byVehicle = make(map[int64][]*Commitment, len(s.Vehicles))
	for _, c := range s.Commitments {
		if c.Vehicle != 0 {
			s.byVehicle[c.Vehicle] = append(s.byVehicle[c.Vehicle], c)
		}
	}
	s.flex = make(map[int64]int, len(s.Vehicles))
	for key, set := range s.Eligible {
		if s.FlexKeys != nil && !s.FlexKeys[key] {
			continue
		}
		for id, ok := range set {
			if ok {
				s.flex[id]++
			}
		}
	}
	s.indexed = true
}

// Invalidate must be called after the caller changed Commitments or
// Eligible directly.
func (s *State) Invalidate() { s.indexed = false }

// Clone copies the commitments (vehicles and eligibility are shared; the
// engine never changes them).
func (s *State) Clone() *State {
	out := &State{
		Now: s.Now, Vehicles: s.Vehicles, Eligible: s.Eligible, FlexKeys: s.FlexKeys,
		Commitments: make(map[int64]*Commitment, len(s.Commitments)),
	}
	for id, c := range s.Commitments {
		cp := *c
		out.Commitments[id] = &cp
	}
	return out
}

// Request asks for one unit (a new commitment, or an existing one with a
// new range / plate).
type Request struct {
	Key  string
	Pool string // "" = any pool
	// Period is [start, end + preparation); End is the customer end.
	Period Interval
	End    time.Time
	Pickup int64
	Return int64
	// Vehicles restricts the candidates (plate assignment); nil = every
	// eligible vehicle.
	Vehicles []int64
	// Self is the existing commitment being re-planned (date change, plate
	// assignment): its old placement does not conflict and it is moved by
	// the plan. 0 = a new commitment.
	Self int64
	// Fixed: the plan's vehicle becomes the promised plate.
	Fixed bool
	Hold  bool
}

// Move is a reassignment of an existing commitment.
type Move struct {
	Commitment int64
	From, To   int64
}

// Plan is the answer of Place: the vehicle of the request and the
// reassignments of other (flexible) commitments that make room for it.
type Plan struct {
	Vehicle  int64
	Moves    []Move
	Expanded int
}

// Options bound the reassignment search.
type Options struct {
	// MaxDepth is the length of the longest move chain (1 = only direct
	// placement on a free vehicle; 2 = move the conflicting commitments of
	// the chosen vehicle to free vehicles; ...).
	MaxDepth int
	// MaxExpansions caps the vehicles tried in one Place.
	MaxExpansions int
}

// DefaultOptions are used when the zero Options is passed.
var DefaultOptions = Options{MaxDepth: 4, MaxExpansions: 4000}

func (o Options) normalize() Options {
	if o.MaxDepth <= 0 {
		o.MaxDepth = DefaultOptions.MaxDepth
	}
	if o.MaxExpansions <= 0 {
		o.MaxExpansions = DefaultOptions.MaxExpansions
	}
	return o
}

// tempID is the id of a new commitment while it is planned.
const tempID int64 = -1

// Place looks for a plan for req without changing the state.
func (s *State) Place(req Request, opts Options) (Plan, bool) {
	s.index()
	p := newPlanner(s, opts.normalize())
	before := snapshot(s)
	var old int64
	if req.Self != 0 {
		old = s.Commitments[req.Self].Vehicle
	}
	c, restore := p.requestCommitment(req)
	defer restore()
	// Re-planning Self may strand later commitments of its old vehicle
	// that relied on its return branch: they are placed again too.
	roots := []*Commitment{c}
	ok := true
	if old != 0 {
		for {
			stranded, fine := p.stranded(old, map[int64]bool{c.ID: true}, c)
			if !fine {
				ok = false
				break
			}
			if len(stranded) == 0 {
				break
			}
			for _, k := range stranded {
				p.assign(k, 0)
				roots = append(roots, k)
			}
		}
	}
	for _, r := range roots {
		if !ok {
			break
		}
		ok = p.place(r, 1, map[int64]bool{})
	}
	if ok {
		ok = p.touchedOK()
	}
	plan := Plan{Expanded: p.expanded}
	if ok {
		plan.Vehicle = c.Vehicle
		for id, v := range before {
			if id == c.ID {
				continue
			}
			if now := s.Commitments[id].Vehicle; now != v {
				plan.Moves = append(plan.Moves, Move{Commitment: id, From: v, To: now})
			}
		}
		sort.Slice(plan.Moves, func(i, j int) bool { return plan.Moves[i].Commitment < plan.Moves[j].Commitment })
	}
	p.rollback(0)
	return plan, ok
}

// Apply writes a plan into the state: the moves, then the request (a new
// commitment with id newID, or Self with its new range and vehicle).
func (s *State) Apply(req Request, plan Plan, newID int64) *Commitment {
	for _, m := range plan.Moves {
		s.Commitments[m.Commitment].Vehicle = m.To
	}
	var c *Commitment
	if req.Self != 0 {
		c = s.Commitments[req.Self]
	} else {
		c = &Commitment{ID: newID, Key: req.Key}
		s.Commitments[newID] = c
	}
	c.Period, c.End, c.Pickup, c.Return = req.Period, req.End, req.Pickup, req.Return
	c.Vehicle = plan.Vehicle
	c.Pool = s.Vehicles[plan.Vehicle].Pool
	if req.Fixed {
		c.Fixed = true
	}
	if req.Self == 0 {
		c.Hold = req.Hold
	}
	s.indexed = false
	return c
}

// Remove releases a commitment.
func (s *State) Remove(id int64) {
	delete(s.Commitments, id)
	s.indexed = false
}

func snapshot(s *State) map[int64]int64 {
	out := make(map[int64]int64, len(s.Commitments))
	for id, c := range s.Commitments {
		out[id] = c.Vehicle
	}
	return out
}

// ---------------------------------------------------------------- planner

type undo struct {
	c    *Commitment
	prev int64
}

type planner struct {
	s        *State
	opts     Options
	expanded int
	log      []undo
	baseline map[int64]map[string]bool
	touched  map[int64]bool
	// only restricts the candidates of commitment onlyFor (plate
	// assignment).
	only    map[int64]bool
	onlyFor int64
}

func newPlanner(s *State, opts Options) *planner {
	return &planner{s: s, opts: opts, baseline: map[int64]map[string]bool{}, touched: map[int64]bool{}}
}

// requestCommitment returns the commitment to place (a temporary one for a
// new request, Self with the requested range otherwise) floating, and a
// function that restores the state.
func (p *planner) requestCommitment(req Request) (*Commitment, func()) {
	s := p.s
	if req.Self != 0 {
		c := s.Commitments[req.Self]
		saved := *c
		// Baselines of every vehicle are taken before anything moves. The
		// re-planned commitment itself must end up valid (an affected
		// commitment is repaired, never kept broken).
		p.base(c.Vehicle)
		delete(p.baseline[c.Vehicle], eventID(c.ID))
		p.assign(c, 0)
		c.Key = keyOr(req.Key, c.Key)
		c.Period, c.End, c.Pickup, c.Return = req.Period, req.End, req.Pickup, req.Return
		c.Fixed = false
		return p.restrictTo(c, req), func() {
			p.rollback(0)
			*c = saved
		}
	}
	c := &Commitment{
		ID: tempID, Key: req.Key, Pool: req.Pool, Period: req.Period, End: req.End,
		Pickup: req.Pickup, Return: req.Return, Hold: req.Hold,
	}
	s.Commitments[tempID] = c
	return p.restrictTo(c, req), func() {
		p.rollback(0)
		delete(s.Commitments, tempID)
	}
}

func keyOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// restrictTo remembers the candidate restriction of the request.
func (p *planner) restrictTo(c *Commitment, req Request) *Commitment {
	if req.Pool != "" {
		c.Pool = req.Pool
	} else if req.Self == 0 {
		c.Pool = ""
	}
	p.only = nil
	if req.Vehicles != nil {
		p.only = map[int64]bool{}
		for _, v := range req.Vehicles {
			p.only[v] = true
		}
		p.onlyFor = c.ID
	}
	return c
}

func (p *planner) attach(c *Commitment) {
	if c.Vehicle == 0 {
		return
	}
	p.s.byVehicle[c.Vehicle] = append(p.s.byVehicle[c.Vehicle], c)
}

func (p *planner) detach(c *Commitment) {
	if c.Vehicle == 0 {
		return
	}
	list := p.s.byVehicle[c.Vehicle]
	for i, x := range list {
		if x == c {
			list[i] = list[len(list)-1]
			p.s.byVehicle[c.Vehicle] = list[:len(list)-1]
			return
		}
	}
}

// assign moves c to vehicle v (0 = floating), logging the change.
func (p *planner) assign(c *Commitment, v int64) {
	if c.Vehicle != 0 {
		p.base(c.Vehicle)
		p.touched[c.Vehicle] = true
	}
	if v != 0 {
		p.base(v)
		p.touched[v] = true
	}
	p.log = append(p.log, undo{c: c, prev: c.Vehicle})
	p.detach(c)
	c.Vehicle = v
	p.attach(c)
}

func (p *planner) rollback(mark int) {
	for len(p.log) > mark {
		u := p.log[len(p.log)-1]
		p.log = p.log[:len(p.log)-1]
		p.detach(u.c)
		u.c.Vehicle = u.prev
		p.attach(u.c)
	}
}

// base records the continuity violations of a vehicle before the plan
// touches it.
func (p *planner) base(v int64) {
	if _, ok := p.baseline[v]; ok {
		return
	}
	p.baseline[v] = p.s.violations(v)
}

// ok: the vehicle has no continuity violation the plan introduced.
func (p *planner) ok(v int64) bool {
	if v == 0 {
		return true
	}
	base := p.baseline[v]
	for id := range p.s.violations(v) {
		if !base[id] {
			return false
		}
	}
	return true
}

func (p *planner) touchedOK() bool {
	for v := range p.touched {
		if !p.ok(v) {
			return false
		}
	}
	return true
}

// candidate is a vehicle a commitment may go to with the commitments it
// would displace.
type candidate struct {
	vehicle   int64
	conflicts []*Commitment
	flex      int
	stays     bool
}

// candidates lists the vehicles that can take c in time (eligible, pool,
// service window, no block / transfer), with their conflicts, best first:
// fewer conflicts, the vehicle c already uses, the least flexible vehicle
// (keep vehicles that many guarantees accept for the narrow ones), id.
func (p *planner) candidates(c *Commitment) []candidate {
	s := p.s
	out := []candidate{}
	for id, ok := range s.Eligible[c.Key] {
		if !ok {
			continue
		}
		if p.only != nil && c.ID == p.onlyFor && !p.only[id] {
			continue
		}
		v := s.Vehicles[id]
		if v == nil || !s.fits(v, c) {
			continue
		}
		cand := candidate{vehicle: id, flex: s.flex[id]}
		for _, k := range s.byVehicle[id] {
			if k != c && k.Period.Overlaps(c.Period) {
				cand.conflicts = append(cand.conflicts, k)
			}
		}
		out = append(out, cand)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if len(a.conflicts) != len(b.conflicts) {
			return len(a.conflicts) < len(b.conflicts)
		}
		if a.flex != b.flex {
			return a.flex < b.flex
		}
		return a.vehicle < b.vehicle
	})
	return out
}

// fits: pool, service window and no block / transfer during c.
func (s *State) fits(v *Vehicle, c *Commitment) bool {
	if c.Pool != "" && v.Pool != c.Pool {
		return false
	}
	if !v.Service.Covers(c.customer()) {
		return false
	}
	for _, seg := range v.Segments {
		if seg.Period.Overlaps(c.Period) {
			return false
		}
	}
	return true
}

// place puts c on a vehicle, recursively moving the flexible commitments it
// displaces: an augmenting path whose nodes are commitments. stack holds
// the commitments of the current chain; they are never displaced again
// inside it (no cycles), while a vehicle may be used several times at
// different times (swaps). depth counts the commitments in the chain.
func (p *planner) place(c *Commitment, depth int, stack map[int64]bool) bool {
	stack[c.ID] = true
	defer delete(stack, c.ID)
	for _, cand := range p.candidates(c) {
		if p.expanded >= p.opts.MaxExpansions {
			return false
		}
		p.expanded++
		if len(cand.conflicts) > 0 && depth >= p.opts.MaxDepth {
			continue
		}
		movable := true
		for _, k := range cand.conflicts {
			// Promised plates, commitments already under way and the chain
			// itself never move.
			if k.Fixed || !k.Period.Start.After(p.s.Now) || stack[k.ID] {
				movable = false
				break
			}
		}
		if !movable {
			continue
		}
		mark := len(p.log)
		displaced := append([]*Commitment(nil), cand.conflicts...)
		for _, k := range cand.conflicts {
			p.assign(k, 0)
		}
		p.assign(c, cand.vehicle)
		// A later commitment stranded by c's return branch (or by a
		// displaced commitment's return) is displaced too.
		stranded, ok := p.stranded(cand.vehicle, stack, c)
		for ok && len(stranded) > 0 {
			if depth >= p.opts.MaxDepth {
				ok = false
				break
			}
			for _, k := range stranded {
				p.assign(k, 0)
				displaced = append(displaced, k)
			}
			stranded, ok = p.stranded(cand.vehicle, stack, c)
		}
		if !ok {
			p.rollback(mark)
			continue
		}
		good := true
		for _, k := range displaced {
			if !p.place(k, depth+1, stack) {
				good = false
				break
			}
		}
		if good {
			return true
		}
		p.rollback(mark)
	}
	return false
}

// stranded returns the commitments of vehicle v that newly break location
// continuity and may be moved. ok is false when the vehicle cannot be
// repaired by moving commitments (a segment, c itself, a fixed / started /
// chain commitment broke).
func (p *planner) stranded(v int64, stack map[int64]bool, c *Commitment) ([]*Commitment, bool) {
	base := p.baseline[v]
	viol := p.s.violations(v)
	out := []*Commitment{}
	for id := range viol {
		if base[id] {
			continue
		}
		if id[0] != 'c' {
			return nil, false
		}
		var k *Commitment
		for _, x := range p.s.byVehicle[v] {
			if eventID(x.ID) == id {
				k = x
				break
			}
		}
		if k == c {
			// c itself does not start where the vehicle is: an earlier
			// one-way commitment moved it away; displace the latest one.
			k = nil
			for _, x := range p.s.byVehicle[v] {
				if x != c && x.Period.Start.Before(c.Period.Start) && (k == nil || x.Period.Start.After(k.Period.Start)) {
					k = x
				}
			}
		}
		if k == nil || k == c || k.Fixed || !k.Period.Start.After(p.s.Now) || stack[k.ID] {
			return nil, false
		}
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, true
}

// ------------------------------------------------------------- continuity

type event struct {
	id       string
	start    time.Time
	end      time.Time
	requires int64
	after    int64
}

// violations returns the ids of the timeline items of vehicle v that do
// not start where the vehicle is: future commitments whose pick-up branch
// is not the vehicle's location at their start, and planned transfers whose
// origin is not.
func (s *State) violations(v int64) map[string]bool {
	veh := s.Vehicles[v]
	out := map[string]bool{}
	if veh == nil {
		return out
	}
	events := make([]event, 0, len(veh.Segments)+len(s.byVehicle[v]))
	for _, seg := range veh.Segments {
		events = append(events, event{
			id: "s:" + seg.Ref, start: seg.Period.Start, end: seg.Period.End, requires: seg.From, after: seg.To,
		})
	}
	for _, c := range s.byVehicle[v] {
		events = append(events, event{
			id: "c:" + strconv.FormatInt(c.ID, 10), start: c.Period.Start, end: c.Period.End,
			requires: c.Pickup, after: c.Return,
		})
	}
	sort.Slice(events, func(i, j int) bool {
		if !events[i].start.Equal(events[j].start) {
			return events[i].start.Before(events[j].start)
		}
		if !events[i].end.Equal(events[j].end) {
			return events[i].end.Before(events[j].end)
		}
		return events[i].id < events[j].id
	})
	loc := veh.Branch
	for _, e := range events {
		if e.requires != 0 && !e.start.Before(s.Now) && e.requires != loc {
			out[e.id] = true
		}
		if e.after != 0 {
			loc = e.after
		}
	}
	return out
}

// LocationAt is where vehicle v is at t according to the state (current
// branch moved by every segment / commitment that starts before t).
func (s *State) LocationAt(v int64, t time.Time) int64 {
	s.index()
	veh := s.Vehicles[v]
	if veh == nil {
		return 0
	}
	type mv struct {
		start time.Time
		to    int64
	}
	moves := []mv{}
	for _, seg := range veh.Segments {
		if seg.To != 0 {
			moves = append(moves, mv{seg.Period.Start, seg.To})
		}
	}
	for _, c := range s.byVehicle[v] {
		moves = append(moves, mv{c.Period.Start, c.Return})
	}
	sort.Slice(moves, func(i, j int) bool { return moves[i].start.Before(moves[j].start) })
	loc := veh.Branch
	for _, m := range moves {
		if !m.start.Before(t) {
			break
		}
		loc = m.to
	}
	return loc
}

// ----------------------------------------------------------------- verify

func eventID(commitment int64) string { return "c:" + strconv.FormatInt(commitment, 10) }

// Violations returns the timeline items that break location continuity,
// over every vehicle: "c:<commitment id>" or "s:<segment ref>".
func (s *State) Violations() map[string]bool {
	s.index()
	out := map[string]bool{}
	for v := range s.Vehicles {
		for id := range s.violations(v) {
			out[id] = true
		}
	}
	return out
}

// Verify checks the invariants of the state and returns every violation
// (empty = valid). Tests use it as the oracle after each operation.
// allowed lists known affected items ("c:<id>" skips every check of that
// commitment, "s:<ref>" the continuity of that segment).
func (s *State) Verify(allowed map[string]bool) []string {
	s.index()
	problems := []string{}
	for _, c := range s.Commitments {
		if allowed[eventID(c.ID)] {
			continue
		}
		v := s.Vehicles[c.Vehicle]
		if v == nil {
			problems = append(problems, fmt.Sprintf("commitment %d has no vehicle", c.ID))
			continue
		}
		if !s.Eligible[c.Key][c.Vehicle] {
			problems = append(problems, fmt.Sprintf("commitment %d on ineligible vehicle %d", c.ID, c.Vehicle))
		}
		if c.Pool != "" && v.Pool != c.Pool {
			problems = append(problems, fmt.Sprintf("commitment %d left pool %s", c.ID, c.Pool))
		}
		if !s.fits(v, c) {
			problems = append(problems, fmt.Sprintf("commitment %d collides with a block / transfer or the service window", c.ID))
		}
	}
	for v, list := range s.byVehicle {
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				if list[i].Period.Overlaps(list[j].Period) {
					problems = append(problems, fmt.Sprintf("double booking on vehicle %d: %d and %d", v, list[i].ID, list[j].ID))
				}
			}
		}
		for id := range s.violations(v) {
			if !allowed[id] {
				problems = append(problems, fmt.Sprintf("continuity broken on vehicle %d at %s", v, id))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

// ---------------------------------------------------------------- release

// ReleaseResult is what releasing a commitment changed: the commitments
// that had to move because they relied on the released one (a later pick-up
// at its one-way return branch), and the items that could not be repaired.
type ReleaseResult struct {
	Moves []Move
	// Affected are timeline items left without a valid plan
	// ("c:<commitment id>" or "s:<transfer ref>"); the caller reports them.
	Affected []string
}

// Release removes a commitment. Releasing always succeeds; when a later
// commitment on the same vehicle depended on its return branch, that
// commitment is re-planned (moved to another vehicle, possibly moving
// others); if that is impossible it is reported as affected.
func (s *State) Release(id int64, opts Options) ReleaseResult {
	s.index()
	c := s.Commitments[id]
	out := ReleaseResult{}
	if c == nil {
		return out
	}
	v := c.Vehicle
	before := s.violations(v)
	s.Remove(id)
	s.index()
	after := s.violations(v)
	broken := []*Commitment{}
	for _, k := range s.byVehicle[v] {
		if after[eventID(k.ID)] && !before[eventID(k.ID)] {
			broken = append(broken, k)
		}
	}
	sort.Slice(broken, func(i, j int) bool { return broken[i].Period.Start.Before(broken[j].Period.Start) })
	for _, k := range broken {
		s.index()
		if !s.violations(k.Vehicle)[eventID(k.ID)] {
			continue // repaired by an earlier move
		}
		if k.Fixed || !k.Period.Start.After(s.Now) {
			out.Affected = append(out.Affected, eventID(k.ID))
			continue
		}
		r := Request{Key: k.Key, Period: k.Period, End: k.End, Pickup: k.Pickup, Return: k.Return, Self: k.ID}
		plan, ok := s.Place(r, opts)
		if !ok {
			out.Affected = append(out.Affected, eventID(k.ID))
			continue
		}
		from := k.Vehicle
		s.Apply(r, plan, 0)
		out.Moves = append(out.Moves, plan.Moves...)
		if from != plan.Vehicle {
			out.Moves = append(out.Moves, Move{Commitment: k.ID, From: from, To: plan.Vehicle})
		}
	}
	s.index()
	for sid := range s.violations(v) {
		if sid[0] == 's' && !before[sid] {
			out.Affected = append(out.Affected, sid)
		}
	}
	sort.Strings(out.Affected)
	return out
}
