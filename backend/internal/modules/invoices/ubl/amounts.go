package ubl

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Split is the VAT split of one VAT-inclusive amount: Net + VAT is what the
// invoice line says. The ledger holds VAT-inclusive amounts (reservations
// and charges are priced gross), so an invoice line starts from the gross
// amount of the ledger item it covers.
type Split struct {
	Gross money.Money // the amount the line is meant to carry (ledger item)
	Net   money.Money // the line extension (tax base)
	VAT   money.Money // round(Net × rate), as go-ubltr's builder computes it
}

// Total is Net + VAT: the line's tax-inclusive amount on the invoice.
func (s Split) Total() money.Money { return s.Net.Add(s.VAT) }

// vatOf is the tax the builder writes for a net amount: net × rate rounded
// half away from zero to the minor unit (shopspring Round and money.Ratio
// use the same rule).
func vatOf(net money.Money, rate money.Rate) money.Money { return net.MulRate(rate) }

// netFor returns the net amount whose net + vat(net) is closest to gross,
// preferring an exact match and, between two equally close ones, the lower
// total (an invoice never asks for more than the ledger item).
func netFor(gross money.Money, rate money.Rate) money.Money {
	if rate <= 0 {
		return gross
	}
	guess := gross.Ratio(money.RateScale, money.RateScale+int64(rate))
	best := guess
	bestDiff := int64(-1)
	for d := int64(-2); d <= 2; d++ {
		n := guess.Add(money.NewIn(d, gross.Currency()))
		if n.IsNegative() {
			continue
		}
		total := n.Add(vatOf(n, rate))
		diff := total.Minor() - gross.Minor()
		abs := diff
		if abs < 0 {
			abs = -abs
		}
		switch {
		case bestDiff < 0 || abs < bestDiff:
			best, bestDiff = n, abs
		case abs == bestDiff && diff < 0:
			// Prefer the total below the gross amount.
			best = n
		}
	}
	return best
}

// SplitLines computes the VAT split of every gross amount so that the sum
// of the line totals equals the sum of the gross amounts whenever the
// rounding allows it: each line takes the net closest to its gross amount,
// then the remaining kuruş difference is moved onto lines whose total can
// absorb it exactly. The returned difference is Σ totals − Σ gross (0 in
// almost every case; ±0.01 when no combination of nets reaches the gross
// total, e.g. a single 20 % line of an unreachable amount).
func SplitLines(gross []money.Money, rates []money.Rate) ([]Split, money.Money) {
	out := make([]Split, len(gross))
	if len(gross) == 0 {
		return out, money.Zero()
	}
	cur := gross[0].Currency()
	diff := int64(0)
	for i, g := range gross {
		n := netFor(g, rates[i])
		out[i] = Split{Gross: g, Net: n, VAT: vatOf(n, rates[i])}
		diff += out[i].Total().Minor() - g.Minor()
	}
	// Move the difference: try one kuruş of net on a line at a time, keeping
	// a step only when it reduces |diff| without overshooting.
	for guard := 0; diff != 0 && guard < 4*len(out)+4; guard++ {
		step := int64(-1)
		if diff < 0 {
			step = 1
		}
		moved := false
		for i := range out {
			n := out[i].Net.Add(money.NewIn(step, cur))
			if n.IsNegative() {
				continue
			}
			v := vatOf(n, rates[i])
			delta := n.Add(v).Minor() - out[i].Total().Minor()
			next := diff + delta
			if abs64(next) < abs64(diff) {
				out[i].Net, out[i].VAT = n, v
				diff = next
				moved = true
				break
			}
		}
		if !moved {
			break
		}
	}
	return out, money.NewIn(diff, cur)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
