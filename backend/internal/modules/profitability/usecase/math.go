package usecase

import (
	"math/big"
	"sort"
	"time"
)

// interval is a half-open time range [start, end).
type interval struct{ start, end time.Time }

func (iv interval) empty() bool { return !iv.start.Before(iv.end) }

func (iv interval) seconds() int64 {
	if iv.empty() {
		return 0
	}
	return int64(iv.end.Sub(iv.start) / time.Second)
}

func clip(iv, to interval) interval {
	if iv.start.Before(to.start) {
		iv.start = to.start
	}
	if iv.end.After(to.end) {
		iv.end = to.end
	}
	return iv
}

// merge sorts and unions intervals (empty ones dropped).
func merge(list []interval) []interval {
	out := make([]interval, 0, len(list))
	for _, iv := range list {
		if !iv.empty() {
			out = append(out, iv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].start.Before(out[j].start) })
	merged := out[:0]
	for _, iv := range out {
		if n := len(merged); n > 0 && !iv.start.After(merged[n-1].end) {
			if iv.end.After(merged[n-1].end) {
				merged[n-1].end = iv.end
			}
			continue
		}
		merged = append(merged, iv)
	}
	return merged
}

func total(list []interval) int64 {
	var s int64
	for _, iv := range list {
		s += iv.seconds()
	}
	return s
}

// overlap is the total length of the intersection of two merged lists.
func overlap(a, b []interval) int64 {
	var s int64
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		s += clip(a[i], b[j]).seconds()
		if a[i].end.Before(b[j].end) {
			i++
		} else {
			j++
		}
	}
	return s
}

// ratio returns round(v × num / den), half away from zero; 0 when den = 0.
func ratio(v, num, den int64) int64 {
	if den == 0 || v == 0 || num == 0 {
		return 0
	}
	p := new(big.Int).Mul(big.NewInt(v), big.NewInt(num))
	d := big.NewInt(den)
	if d.Sign() < 0 {
		d.Neg(d)
		p.Neg(p)
	}
	neg := p.Sign() < 0
	if neg {
		p.Neg(p)
	}
	q, r := new(big.Int).QuoRem(p, d, new(big.Int))
	if new(big.Int).Mul(r, big.NewInt(2)).Cmp(d) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return q.Int64()
}

// split shares amount (kuruş) across keys by non-negative weights with the
// largest remainder method: the parts always add up to amount exactly. Ties
// on the remainder go to the larger weight, then to the smaller key. With
// no positive weight it returns nil.
func split(amount int64, keys []int64, weights []int64) map[int64]int64 {
	var sum int64
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	if sum == 0 {
		return nil
	}
	out := make(map[int64]int64, len(keys))
	if amount == 0 {
		return out
	}
	neg := amount < 0
	a := amount
	if neg {
		a = -a
	}
	type part struct {
		key    int64
		weight int64
		rem    *big.Int
	}
	parts := make([]part, 0, len(keys))
	bigSum := big.NewInt(sum)
	var given int64
	for i, k := range keys {
		w := weights[i]
		if w <= 0 {
			continue
		}
		p := new(big.Int).Mul(big.NewInt(a), big.NewInt(w))
		q, r := new(big.Int).QuoRem(p, bigSum, new(big.Int))
		out[k] += q.Int64()
		given += q.Int64()
		parts = append(parts, part{key: k, weight: w, rem: r})
	}
	sort.SliceStable(parts, func(i, j int) bool {
		if c := parts[i].rem.Cmp(parts[j].rem); c != 0 {
			return c > 0
		}
		if parts[i].weight != parts[j].weight {
			return parts[i].weight > parts[j].weight
		}
		return parts[i].key < parts[j].key
	})
	for i := 0; given < a; i++ {
		out[parts[i%len(parts)].key]++
		given++
	}
	if neg {
		for k, v := range out {
			out[k] = -v
		}
	}
	return out
}
