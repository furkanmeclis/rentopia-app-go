package usecase

import (
	"context"
	"math"
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// ChannelSource names the reservation field the channel report groups by.
// The channel manager (TEC-736) writes the same field for the channels it
// imports, so new channels appear here without a change.
const ChannelSource = "reservations.channel"

type chAgg struct {
	reservations, confirmed, pending, cancelled, expired, noShow int
	revenue, commission, channelCollected                        int64
	seconds                                                      float64
}

func (a *chAgg) add(r db.ReportChannelFactsRow) {
	n := int(r.Reservations)
	a.reservations += n
	switch r.Status {
	case "confirmed":
		a.confirmed += n
		total := kurus(r.TotalAmount)
		a.revenue += total
		a.commission += kurus(r.CommissionAmount)
		if r.CollectingParty == "channel" {
			a.channelCollected += total
		}
		a.seconds += r.RentalSeconds
	case "cancelled":
		a.cancelled += n
	case "expired":
		a.expired += n
	case "no_show":
		a.noShow += n
	default:
		a.pending += n
	}
}

func (a chAgg) figures(totalRevenue int64) model.ChannelFigures {
	f := model.ChannelFigures{
		Reservations: a.reservations, Confirmed: a.confirmed, Pending: a.pending, Cancelled: a.cancelled,
		Expired: a.expired, NoShow: a.noShow,
		CancellationRate: pct(float64(a.cancelled), float64(a.reservations)),
		Revenue:          money.New(a.revenue), Commission: money.New(a.commission), NetRevenue: money.New(a.revenue - a.commission),
		CommissionRate:   pct(float64(a.commission), float64(a.revenue)),
		ChannelCollected: money.New(a.channelCollected),
		RevenueShare:     pct(float64(a.revenue), float64(totalRevenue)),
	}
	if a.confirmed > 0 {
		v := money.New(int64(math.Round(float64(a.revenue) / float64(a.confirmed))))
		f.AverageRevenue = &v
		f.AverageDays = round(a.seconds/86400/float64(a.confirmed), 2)
	}
	return f
}

// Channels is the performance of each sales channel: reservations by
// status, the cancellation rate, revenue (confirmed reservations' sold
// price), channel commission and net revenue, by booking or pick-up date.
func (s *Service) Channels(ctx context.Context, orgID int64, f model.Filter) (model.ChannelReport, error) {
	basis := f.DateBasis
	if basis == "" {
		basis = model.DateBooking
	}
	if !contains(model.DateBases, basis) {
		return model.ChannelReport{}, invalid("date_basis", "invalid", "date_basis must be booking or pickup")
	}
	all, err := s.branches(ctx, orgID)
	if err != nil {
		return model.ChannelReport{}, err
	}
	filter, err := branchFilter(ctx, all, f.BranchUUIDs)
	if err != nil {
		return model.ChannelReport{}, err
	}
	from, to := s.Period(f.From, f.To)
	rows, err := s.q.ReportChannelFacts(ctx, db.ReportChannelFactsParams{
		OrganizationID: orgID, ByPickup: basis == model.DatePickup,
		FromTs: ts(from.StartIn().UTC()), ToTs: ts(to.AddDays(1).StartIn().UTC()),
		ScopeBranchIds: orgctx.BranchScope(ctx), FilterBranchIds: filter,
	})
	if err != nil {
		return model.ChannelReport{}, err
	}
	out := model.ChannelReport{
		From: from.String(), To: to.String(), DateBasis: basis, Method: method(ctx), Source: ChannelSource,
		Rows: []model.ChannelRow{}, Trend: []model.ChannelPoint{},
	}
	var total chAgg
	byChannel := map[string]*chAgg{}
	type mk struct{ month, channel string }
	byMonth := map[mk]*chAgg{}
	for _, r := range rows {
		total.add(r)
		a, ok := byChannel[r.Channel]
		if !ok {
			a = &chAgg{}
			byChannel[r.Channel] = a
		}
		a.add(r)
		k := mk{r.Month, r.Channel}
		m, ok := byMonth[k]
		if !ok {
			m = &chAgg{}
			byMonth[k] = m
		}
		m.add(r)
	}
	out.Totals = total.figures(total.revenue)
	for ch, a := range byChannel {
		out.Rows = append(out.Rows, model.ChannelRow{Channel: ch, ChannelFigures: a.figures(total.revenue)})
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		if a.Revenue.Minor() != b.Revenue.Minor() {
			return a.Revenue.Minor() > b.Revenue.Minor()
		}
		if a.Reservations != b.Reservations {
			return a.Reservations > b.Reservations
		}
		return a.Channel < b.Channel
	})
	for k, a := range byMonth {
		out.Trend = append(out.Trend, model.ChannelPoint{
			Month: k.month, Channel: k.channel, Reservations: a.reservations, Cancelled: a.cancelled, Revenue: money.New(a.revenue),
		})
	}
	sort.Slice(out.Trend, func(i, j int) bool {
		if out.Trend[i].Month != out.Trend[j].Month {
			return out.Trend[i].Month < out.Trend[j].Month
		}
		return out.Trend[i].Channel < out.Trend[j].Channel
	})
	return out, nil
}
