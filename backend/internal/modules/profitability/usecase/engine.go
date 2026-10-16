package usecase

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

// The engine turns the facts of one organization and period into figures
// per vehicle and calendar month. It is pure (no database, no clock): the
// service loads the facts, the tests feed them directly.
//
// docs/design/vehicle-profitability.md describes every rule.

// Money slots of an accumulator (kuruş).
const (
	fRent = iota
	fExtras
	fCharges
	fTollFees
	fPayComm
	fChanComm
	fExpenses
	fMaint
	fTransfers
	fOverhead
	fRefunded
	fHeld
	fReturned
	fPass
	fCapex
	nMoney
)

// Rental-level classes split across the rental's vehicles.
var rentalSlots = []int{fRent, fExtras, fCharges, fTollFees, fPass, fPayComm, fChanComm, fRefunded, fHeld, fReturned}

type vehicleFact struct {
	id      int64
	ref     model.VehicleRef
	home    int64
	current int64
	// [start, end) of the service life (end far in the future when open).
	service interval
}

type legFact struct {
	rental, vehicle int64
	out             time.Time
	in              *time.Time
}

type rentalFact struct {
	id, vehicle int64
	endsAt      time.Time
	channelComm int64
	total       int64
	extras      int64
}

type ledgerFact struct {
	id                                                     int64
	kind, direction, purpose, origKind, origPurpose, ckind string
	amount                                                 int64
	day                                                    rentaltime.Date
	rental                                                 int64
}

// costFact is an expense (vehicle or branch / head office), a maintenance
// cost or a transfer cost, already dated by the basis.
type costFact struct {
	vehicle, branch int64 // 0 = none
	amount          int64
	day             rentaltime.Date
	code, name      string
}

// Billed classes of a rental (live accruals, all dates).
type billed struct {
	rentTotal, charges, tollFee, toll int64
}

type input struct {
	basis, allocation string
	from, to          rentaltime.Date
	now               time.Time
	// Overhead visibility: every branch and the head office, or these
	// branches.
	scopeAll      bool
	scopeBranches map[int64]bool

	vehicles    []vehicleFact
	rentals     map[int64]rentalFact
	legs        map[int64][]legFact // by rental
	ledger      []ledgerFact
	billed      map[int64]billed
	paymentComm map[int64]int64
	expenses    []costFact
	maintenance []costFact
	transfers   []costFact
	blocks      map[int64][]interval // by vehicle
}

type bucket struct {
	key        string
	from, to   rentaltime.Date
	start, end time.Time // [start, end) in UTC
}

type acc struct {
	money             [nMoney]int64
	rented, available int64
}

type vehicleResult struct {
	buckets    []acc
	rentals    map[int64]bool
	categories map[string]*category
}

type category struct {
	code, name string
	amount     int64
}

type result struct {
	buckets     []bucket
	vehicles    map[int64]*vehicleResult
	unallocated int64
	fallback    bool
}

// monthBuckets cuts [from, to] into calendar months (Istanbul).
func monthBuckets(from, to rentaltime.Date) []bucket {
	var out []bucket
	cur := from
	for !cur.After(to) {
		last := rentaltime.NewDate(cur.Year, cur.Month+1, 1).AddDays(-1)
		if last.After(to) {
			last = to
		}
		out = append(out, bucket{
			key: cur.String()[:7], from: cur, to: last,
			start: cur.StartIn().UTC(), end: last.AddDays(1).StartIn().UTC(),
		})
		cur = last.AddDays(1)
	}
	return out
}

func bucketOf(buckets []bucket, d rentaltime.Date) int {
	for i, b := range buckets {
		if !d.Before(b.from) && !d.After(b.to) {
			return i
		}
	}
	return -1
}

func (r rentalFact) splitRent(totalRent int64) (rent, extras int64) {
	if r.total <= 0 || r.extras <= 0 {
		return totalRent, 0
	}
	ex := r.extras
	if ex > r.total {
		ex = r.total
	}
	extras = ratio(totalRent, ex, r.total)
	return totalRent - extras, extras
}

// rentalMoney is what one rental did in one month.
type rentalMoney struct {
	rentTotal, charges, tollFee, toll int64 // accrual basis
	cash, refunded, held, returned    int64
}

func compute(in input) result {
	buckets := monthBuckets(in.from, in.to)
	res := result{buckets: buckets, vehicles: map[int64]*vehicleResult{}}
	vr := func(id int64) *vehicleResult {
		v := res.vehicles[id]
		if v == nil {
			v = &vehicleResult{buckets: make([]acc, len(buckets)), rentals: map[int64]bool{}, categories: map[string]*category{}}
			res.vehicles[id] = v
		}
		return v
	}
	for _, v := range in.vehicles {
		vr(v.id)
	}

	// 1. Rental money per month.
	money := map[int64]map[int]*rentalMoney{}
	seen := map[int64]bool{}
	for _, e := range in.ledger {
		if seen[e.id] {
			continue
		}
		seen[e.id] = true
		b := bucketOf(buckets, e.day)
		if b < 0 {
			continue
		}
		byB := money[e.rental]
		if byB == nil {
			byB = map[int]*rentalMoney{}
			money[e.rental] = byB
		}
		m := byB[b]
		if m == nil {
			m = &rentalMoney{}
			byB[b] = m
		}
		applyLedger(m, e)
	}

	// 2. Rental money → vehicles (by leg time), per basis.
	for rid, byB := range money {
		r, ok := in.rentals[rid]
		if !ok {
			continue
		}
		keys, weights := legShares(r, in.legs[rid], in.now)
		bl := in.billed[rid]
		billedIncome := bl.rentTotal + bl.charges + bl.tollFee
		for b, m := range byB {
			var slots [nMoney]int64
			if in.basis == model.BasisCash {
				cashClasses(r, bl, m.cash, &slots)
			} else {
				slots[fRent], slots[fExtras] = r.splitRent(m.rentTotal)
				slots[fCharges], slots[fTollFees], slots[fPass] = m.charges, m.tollFee, m.toll
			}
			income := slots[fRent] + slots[fExtras] + slots[fCharges] + slots[fTollFees]
			slots[fPayComm] = ratio(in.paymentComm[rid], income, billedIncome)
			slots[fChanComm] = ratio(r.channelComm, income, billedIncome)
			slots[fRefunded], slots[fHeld], slots[fReturned] = m.refunded, m.held, m.returned
			for _, slot := range rentalSlots {
				if slots[slot] == 0 {
					continue
				}
				for vid, part := range split(slots[slot], keys, weights) {
					vr(vid).buckets[b].money[slot] += part
				}
			}
		}
	}

	// 3. Direct costs.
	for _, c := range in.expenses {
		if c.vehicle == 0 {
			continue
		}
		b := bucketOf(buckets, c.day)
		if b < 0 {
			continue
		}
		v := vr(c.vehicle)
		if c.code == capexCategory {
			v.buckets[b].money[fCapex] += c.amount
			continue
		}
		v.buckets[b].money[fExpenses] += c.amount
		cat := v.categories[c.code]
		if cat == nil {
			cat = &category{code: c.code, name: c.name}
			v.categories[c.code] = cat
		}
		cat.amount += c.amount
	}
	for _, list := range []struct {
		facts []costFact
		slot  int
	}{{in.maintenance, fMaint}, {in.transfers, fTransfers}} {
		for _, c := range list.facts {
			if b := bucketOf(buckets, c.day); b >= 0 && c.vehicle != 0 {
				vr(c.vehicle).buckets[b].money[list.slot] += c.amount
			}
		}
	}

	// 4. Occupancy and rental counts.
	legsByVehicle := map[int64][]legFact{}
	for _, list := range in.legs {
		for _, l := range list {
			legsByVehicle[l.vehicle] = append(legsByVehicle[l.vehicle], l)
		}
	}
	periodIv := interval{buckets[0].start, buckets[len(buckets)-1].end}
	if in.now.Before(periodIv.end) {
		periodIv.end = in.now
	}
	for _, v := range in.vehicles {
		out := vr(v.id)
		blocks := merge(in.blocks[v.id])
		var legIvs []interval
		for _, l := range legsByVehicle[v.id] {
			end := in.now
			if l.in != nil {
				end = *l.in
			}
			iv := interval{l.out, end}
			legIvs = append(legIvs, iv)
			if !clip(iv, periodIv).empty() {
				out.rentals[l.rental] = true
			}
		}
		legIvs = merge(legIvs)
		for b, bk := range buckets {
			s := clip(clip(v.service, interval{bk.start, bk.end}), interval{bk.start, in.now})
			if s.empty() {
				continue
			}
			blocked := merge(clipAll(blocks, s))
			rented := merge(clipAll(legIvs, s))
			out.buckets[b].available = s.seconds() - total(blocked)
			out.buckets[b].rented = total(rented) - overlap(rented, blocked)
		}
	}

	// 5. Shared overhead, per month and pool.
	type poolKey struct {
		bucket int
		branch int64 // 0 = head office
	}
	pools := map[poolKey]int64{}
	var poolOrder []poolKey
	for _, c := range in.expenses {
		if c.vehicle != 0 || c.code == capexCategory {
			continue
		}
		b := bucketOf(buckets, c.day)
		if b < 0 {
			continue
		}
		if c.branch == 0 && !in.scopeAll {
			continue
		}
		if c.branch != 0 && !in.scopeAll && !in.scopeBranches[c.branch] {
			continue
		}
		k := poolKey{b, c.branch}
		if _, ok := pools[k]; !ok {
			poolOrder = append(poolOrder, k)
		}
		pools[k] += c.amount
	}
	for _, k := range poolOrder {
		amount := pools[k]
		bk := buckets[k.bucket]
		var keys, weights, counts []int64
		for _, v := range in.vehicles {
			if k.branch != 0 && v.home != k.branch {
				continue
			}
			if clip(v.service, interval{bk.start, bk.end}).empty() {
				continue
			}
			a := res.vehicles[v.id].buckets[k.bucket]
			var w int64
			switch in.allocation {
			case model.AllocationRentalDays:
				w = a.rented
			case model.AllocationRevenue:
				w = a.money[fRent] + a.money[fExtras] + a.money[fCharges] + a.money[fTollFees]
			default:
				w = 1
			}
			keys = append(keys, v.id)
			weights = append(weights, w)
			counts = append(counts, 1)
		}
		if len(keys) == 0 {
			res.unallocated += amount
			continue
		}
		parts := split(amount, keys, weights)
		if parts == nil {
			parts = split(amount, keys, counts)
			res.fallback = true
		}
		for vid, part := range parts {
			res.vehicles[vid].buckets[k.bucket].money[fOverhead] += part
		}
	}
	return res
}

// capexCategory: vehicle procurement is an investment, kept apart from the
// operating costs.
const capexCategory = "vehicle_procurement"

func clipAll(list []interval, to interval) []interval {
	out := make([]interval, 0, len(list))
	for _, iv := range list {
		if c := clip(iv, to); !c.empty() {
			out = append(out, c)
		}
	}
	return out
}

// applyLedger books one ledger entry on the rental's month.
func applyLedger(m *rentalMoney, e ledgerFact) {
	switch e.kind {
	case "accrual":
		if e.direction == "debit" {
			accrue(m, e.purpose, e.ckind, e.amount)
		}
	case "reversal":
		switch e.origKind {
		case "accrual":
			accrue(m, e.origPurpose, e.ckind, -e.amount)
		case "collection":
			if e.origPurpose == "deposit" {
				m.held -= e.amount
			} else {
				m.cash -= e.amount
			}
		case "refund":
			if e.origPurpose == "deposit" {
				m.returned -= e.amount
			} else {
				m.cash += e.amount
				m.refunded -= e.amount
			}
		}
	case "collection":
		if e.purpose == "deposit" {
			m.held += e.amount
		} else {
			m.cash += e.amount
		}
	case "refund":
		if e.purpose == "deposit" {
			m.returned += e.amount
		} else {
			m.cash -= e.amount
			m.refunded += e.amount
		}
	case "deposit_hold":
		m.held += e.amount
	case "deposit_return":
		m.returned += e.amount
	}
}

func accrue(m *rentalMoney, purpose, ckind string, amount int64) {
	switch {
	case purpose == "rent":
		m.rentTotal += amount
	case ckind == "toll":
		m.toll += amount
	case ckind == "toll_fee":
		m.tollFee += amount
	case purpose == "charge":
		m.charges += amount
	}
}

// cashClasses splits a rental's net cash of a month by what the rental
// billed (live accruals): rent / extras / charges / toll fees / tolls.
func cashClasses(r rentalFact, bl billed, cash int64, slots *[nMoney]int64) {
	if cash == 0 {
		return
	}
	rent, extras := r.splitRent(bl.rentTotal)
	keys := []int64{fRent, fExtras, fCharges, fTollFees, fPass}
	parts := split(cash, keys, []int64{rent, extras, bl.charges, bl.tollFee, bl.toll})
	if parts == nil {
		slots[fRent], slots[fExtras] = r.splitRent(cash)
		return
	}
	for k, v := range parts {
		slots[k] += v
	}
}

// legShares: the rental's vehicles weighted by their leg time. A returned
// leg counts to its return, an open leg to the later of now and the
// planned return. A rental not handed over belongs to its vehicle.
func legShares(r rentalFact, legs []legFact, now time.Time) ([]int64, []int64) {
	idx := map[int64]int{}
	var keys, weights []int64
	for _, l := range legs {
		end := r.endsAt
		if now.After(end) {
			end = now
		}
		if l.in != nil {
			end = *l.in
		}
		secs := interval{l.out, end}.seconds()
		i, ok := idx[l.vehicle]
		if !ok {
			i = len(keys)
			idx[l.vehicle] = i
			keys = append(keys, l.vehicle)
			weights = append(weights, 0)
		}
		weights[i] += secs
	}
	var sum int64
	for _, w := range weights {
		sum += w
	}
	if sum == 0 {
		return []int64{r.vehicle}, []int64{1}
	}
	return keys, weights
}
