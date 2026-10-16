package usecase

import (
	"context"
	"sort"

	profitmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// UngroupedKey is the occupancy row of vehicles in no rental group.
const UngroupedKey = "ungrouped"

// occAgg accumulates the time figures of one group.
type occAgg struct {
	key      string
	ref      model.Ref
	order    string
	vehicles map[int64]bool
	rented   float64
	avail    float64
	rentals  int
	gross    money.Money
	months   []struct{ rented, avail float64 }
}

func newOccAgg(key string, ref model.Ref, order string, n int) *occAgg {
	return &occAgg{key: key, ref: ref, order: order, vehicles: map[int64]bool{}, gross: money.Zero(),
		months: make([]struct{ rented, avail float64 }, n)}
}

// addVehicle adds a vehicle's whole period.
func (a *occAgg) addVehicle(v profitmodel.VehicleMonths) {
	if v.Totals.AvailableDays > 0 || v.Totals.RentalDays > 0 {
		a.vehicles[v.VehicleID] = true
	}
	a.rented += v.Totals.RentalDays
	a.avail += v.Totals.AvailableDays
	a.rentals += v.Totals.RentalCount
	a.gross = a.gross.Add(v.Totals.GrossIncome)
	for i, m := range v.Months {
		a.months[i].rented += m.RentalDays
		a.months[i].avail += m.AvailableDays
	}
}

func (a *occAgg) figures() model.OccupancyFigures {
	return model.OccupancyFigures{
		Vehicles: len(a.vehicles), RentalDays: round(a.rented, 2), AvailableDays: round(a.avail, 2),
		Occupancy: pct(a.rented, a.avail), RentalCount: a.rentals, GrossIncome: a.gross,
		IncomePerAvailableDay: perDay(a.gross, a.avail),
	}
}

func (a *occAgg) trend(monthKeys []string) []model.OccupancyPoint {
	out := make([]model.OccupancyPoint, 0, len(monthKeys))
	for i, m := range a.months {
		out = append(out, model.OccupancyPoint{
			Month: monthKeys[i], RentalDays: round(m.rented, 2), AvailableDays: round(m.avail, 2), Occupancy: pct(m.rented, m.avail),
		})
	}
	return out
}

// Occupancy is rented time over available time (the profitability
// engine's: service life minus usage blocks) by rental group, home branch
// or month, with the monthly trend of each group.
func (s *Service) Occupancy(ctx context.Context, orgID int64, f model.Filter) (model.OccupancyReport, error) {
	g, err := groupBy(f.GroupBy, model.OccupancyGroups)
	if err != nil {
		return model.OccupancyReport{}, err
	}
	bd, err := s.profit.Breakdown(ctx, orgID, profitmodel.Filter{
		From: f.From, To: f.To, BranchUUIDs: f.BranchUUIDs, RentalGroupUUIDs: f.RentalGroupUUIDs,
	})
	if err != nil {
		return model.OccupancyReport{}, profitError(err)
	}
	monthKeys := make([]string, len(bd.Months))
	for i, m := range bd.Months {
		monthKeys[i] = m.Month
	}
	out := model.OccupancyReport{From: bd.From, To: bd.To, GroupBy: g, Method: method(ctx), Rows: []model.OccupancyRow{}}

	total := newOccAgg("", model.Ref{}, "", len(monthKeys))
	for _, v := range bd.Vehicles {
		total.addVehicle(v)
	}
	out.Totals = total.figures()
	out.Trend = total.trend(monthKeys)

	groups := map[string]*occAgg{}
	get := func(key string, ref model.Ref, order string) *occAgg {
		a, ok := groups[key]
		if !ok {
			a = newOccAgg(key, ref, order, len(monthKeys))
			groups[key] = a
		}
		return a
	}
	switch g {
	case model.GroupRentalGroup:
		rows, err := s.q.ReportVehicleGroups(ctx, orgID)
		if err != nil {
			return out, err
		}
		type grp struct {
			uuid       uuid.UUID
			code, name string
		}
		byVehicle := map[int64][]grp{}
		for _, r := range rows {
			byVehicle[r.VehicleID] = append(byVehicle[r.VehicleID], grp{r.GroupUuid, r.GroupCode, r.GroupName})
		}
		for _, v := range bd.Vehicles {
			gs := byVehicle[v.VehicleID]
			if len(gs) > 1 {
				out.MultiGroupVehicles++
			}
			if len(gs) == 0 {
				get(UngroupedKey, model.Ref{Code: "", Name: ""}, "￿").addVehicle(v)
				continue
			}
			for _, gr := range gs {
				id := gr.uuid
				get(gr.uuid.String(), model.Ref{UUID: &id, Code: gr.code, Name: gr.name}, gr.code).addVehicle(v)
			}
		}
	case model.GroupBranch:
		for _, v := range bd.Vehicles {
			hb := v.Vehicle.HomeBranch
			id := hb.UUID
			get(hb.UUID.String(), model.Ref{UUID: &id, Code: hb.Code, Name: hb.Name}, hb.Code).addVehicle(v)
		}
	case model.GroupMonth:
		for i, key := range monthKeys {
			a := get(key, model.Ref{Code: key, Name: key}, key)
			for _, v := range bd.Vehicles {
				m := v.Months[i]
				if m.AvailableDays > 0 || m.RentalDays > 0 {
					a.vehicles[v.VehicleID] = true
				}
				a.rented += m.RentalDays
				a.avail += m.AvailableDays
				a.gross = a.gross.Add(m.GrossIncome)
				a.months[i].rented += m.RentalDays
				a.months[i].avail += m.AvailableDays
			}
		}
	}
	list := make([]*occAgg, 0, len(groups))
	for _, a := range groups {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].order != list[j].order {
			return list[i].order < list[j].order
		}
		return list[i].key < list[j].key
	})
	for _, a := range list {
		out.Rows = append(out.Rows, model.OccupancyRow{Key: a.key, Group: a.ref, OccupancyFigures: a.figures(), Trend: a.trend(monthKeys)})
	}
	return out, nil
}
