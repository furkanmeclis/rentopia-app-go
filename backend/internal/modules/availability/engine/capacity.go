package engine

// Breakdown explains the capacity of one request: what happens to every
// vehicle that satisfies the guarantee (each vehicle is counted once, in the
// first matching bucket), and how many more units fit.
type Breakdown struct {
	// Eligible vehicles of the guarantee (and pool, when requested).
	Eligible int `json:"eligible"`
	// OutOfService: outside the vehicle's service window.
	OutOfService int `json:"out_of_service"`
	// Blocked: a block or an open transfer overlaps the range.
	Blocked int `json:"blocked"`
	// Reserved: a confirmed reservation / rental overlaps the range.
	Reserved int `json:"reserved"`
	// Held: only options (holds) overlap the range.
	Held int `json:"held"`
	// Elsewhere: free in time but not at the pick-up branch at the start
	// (or its return would break a later commitment / transfer).
	Elsewhere int `json:"elsewhere"`
	// Free: the request fits directly.
	Free int `json:"free"`
	// Reassignable: extra units found by moving flexible commitments to
	// other vehicles.
	Reassignable int `json:"reassignable"`
	// Physical = Free + Reassignable: units that can be sold at the same
	// time for this exact request, before channel rules.
	Physical int `json:"physical"`
}

// Classify sorts the eligible vehicles of req into the breakdown buckets
// (without Physical / Reassignable).
func (s *State) Classify(req Request) Breakdown {
	s.index()
	b := Breakdown{}
	probe := &Commitment{
		ID: tempID, Key: req.Key, Pool: req.Pool, Period: req.Period, End: req.End,
		Pickup: req.Pickup, Return: req.Return,
	}
	for id, ok := range s.Eligible[req.Key] {
		if !ok {
			continue
		}
		v := s.Vehicles[id]
		if v == nil || (req.Pool != "" && v.Pool != req.Pool) {
			continue
		}
		b.Eligible++
		if !v.Service.Covers(probe.customer()) {
			b.OutOfService++
			continue
		}
		blocked := false
		for _, seg := range v.Segments {
			if seg.Period.Overlaps(req.Period) {
				blocked = true
				break
			}
		}
		if blocked {
			b.Blocked++
			continue
		}
		reserved, held := false, false
		for _, k := range s.byVehicle[id] {
			if k.ID != req.Self && k.Period.Overlaps(req.Period) {
				if k.Hold {
					held = true
				} else {
					reserved = true
				}
			}
		}
		switch {
		case reserved:
			b.Reserved++
			continue
		case held:
			b.Held++
			continue
		}
		p := newPlanner(s, DefaultOptions)
		s.Commitments[tempID] = probe
		p.assign(probe, id)
		fits := p.ok(id)
		p.rollback(0)
		delete(s.Commitments, tempID)
		if fits {
			b.Free++
		} else {
			b.Elsewhere++
		}
	}
	return b
}

// Capacity is Classify plus the number of identical requests that can be
// accepted together (Physical), counted by placing copies one by one with
// the same planner Reserve uses, so search and reserve agree. limit stops
// the count early (0 = no limit).
func (s *State) Capacity(req Request, limit int, opts Options) Breakdown {
	b := s.Classify(req)
	work := s.Clone()
	n := 0
	for limit <= 0 || n < limit {
		plan, ok := work.Place(req, opts)
		if !ok {
			break
		}
		r := req
		r.Self = 0
		work.Apply(r, plan, int64(-1000-n))
		n++
	}
	b.Physical = n
	if n > b.Free {
		b.Reassignable = n - b.Free
	}
	return b
}
