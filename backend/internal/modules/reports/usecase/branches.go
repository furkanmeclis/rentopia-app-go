package usecase

import (
	"context"
	"sort"

	profitmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

type branchAgg struct {
	ref                                    model.Ref
	vehicles, rentals                      int
	rented, avail                          float64
	gross, comm, direct, overhead, contrib money.Money
}

func newBranchAgg(ref model.Ref) *branchAgg {
	z := money.Zero()
	return &branchAgg{ref: ref, gross: z, comm: z, direct: z, overhead: z, contrib: z}
}

func (a *branchAgg) add(v profitmodel.VehicleMonths) {
	t := v.Totals
	if t.AvailableDays > 0 || t.RentalDays > 0 {
		a.vehicles++
	}
	a.rentals += t.RentalCount
	a.rented += t.RentalDays
	a.avail += t.AvailableDays
	a.gross = a.gross.Add(t.GrossIncome)
	a.comm = a.comm.Add(t.Commissions)
	a.direct = a.direct.Add(t.DirectCosts)
	a.overhead = a.overhead.Add(t.Overhead)
	a.contrib = a.contrib.Add(t.Contribution)
}

func (a *branchAgg) figures(totalContribution money.Money) model.BranchFigures {
	f := model.BranchFigures{
		Vehicles: a.vehicles, RentalCount: a.rentals, RentalDays: round(a.rented, 2), AvailableDays: round(a.avail, 2),
		Occupancy: pct(a.rented, a.avail), GrossIncome: a.gross, Commissions: a.comm, DirectCosts: a.direct,
		Overhead: a.overhead, Contribution: a.contrib, Margin: pct(float64(a.contrib.Minor()), float64(a.gross.Minor())),
	}
	if a.vehicles > 0 {
		v := a.gross.Ratio(1, int64(a.vehicles))
		f.IncomePerVehicle = &v
	}
	if totalContribution.IsPositive() {
		f.ContributionShare = pct(float64(a.contrib.Minor()), float64(totalContribution.Minor()))
	}
	return f
}

// Branches compares the branches by their vehicles' figures (home branch):
// occupancy, income, commissions, direct costs, allocated overhead and the
// operating contribution of the vehicle profitability engine.
func (s *Service) Branches(ctx context.Context, orgID int64, f model.Filter) (model.BranchReport, error) {
	bd, err := s.profit.Breakdown(ctx, orgID, profitmodel.Filter{
		From: f.From, To: f.To, BranchUUIDs: f.BranchUUIDs, Basis: f.Basis, Allocation: f.Allocation,
	})
	if err != nil {
		return model.BranchReport{}, profitError(err)
	}
	out := model.BranchReport{
		From: bd.From, To: bd.To, Rows: []model.BranchRow{},
		Method: model.BranchMethod{
			Method: method(ctx), Basis: bd.Method.Basis, Allocation: bd.Method.Allocation, OverheadScope: bd.Method.OverheadScope,
			AllocationFallback: bd.Method.AllocationFallback, UnallocatedOverhead: bd.Method.UnallocatedOverhead,
		},
	}
	total := newBranchAgg(model.Ref{})
	groups := map[string]*branchAgg{}
	for _, v := range bd.Vehicles {
		total.add(v)
		hb := v.Vehicle.HomeBranch
		a, ok := groups[hb.UUID.String()]
		if !ok {
			id := hb.UUID
			a = newBranchAgg(model.Ref{UUID: &id, Code: hb.Code, Name: hb.Name})
			groups[hb.UUID.String()] = a
		}
		a.add(v)
	}
	list := make([]*branchAgg, 0, len(groups))
	for _, a := range groups {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ref.Code < list[j].ref.Code })
	for _, a := range list {
		out.Rows = append(out.Rows, model.BranchRow{Branch: a.ref, BranchFigures: a.figures(total.contrib)})
	}
	out.Totals = total.figures(total.contrib)
	return out, nil
}
