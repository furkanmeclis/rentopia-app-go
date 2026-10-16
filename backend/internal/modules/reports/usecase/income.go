package usecase

import (
	"context"
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/jackc/pgx/v5/pgtype"
)

// HeadOfficeKey is the branch row of records without a branch.
const HeadOfficeKey = "head_office"

// Income category codes (the client and the export translate them).
const (
	IncomeRent  = "rent"
	IncomeOther = "other"
	// IncomeChargePrefix + rental charge kind (fuel, damage, toll_fee …).
	IncomeChargePrefix = "charge."
	// passThroughKind: tolls re-billed to the customer.
	passThroughKind = "toll"
	// capexCategory: vehicle procurement is an investment.
	capexCategory = "vehicle_procurement"
)

// ieAgg accumulates kuruş by class.
type ieAgg struct {
	income, expense, pass, capex int64
}

func (a *ieAgg) add(kind string, amount int64) {
	switch kind {
	case model.KindIncome:
		a.income += amount
	case model.KindExpense:
		a.expense += amount
	case model.KindPassThrough:
		a.pass += amount
	case model.KindCapex:
		a.capex += amount
	}
}

func (a ieAgg) figures() model.IncomeExpenseFigures {
	return model.IncomeExpenseFigures{
		Income: money.New(a.income), Expense: money.New(a.expense), Net: money.New(a.income - a.expense),
		PassThrough: money.New(a.pass), Capex: money.New(a.capex), Margin: pct(float64(a.income-a.expense), float64(a.income)),
	}
}

// ieFact is one grouped fact of either side.
type ieFact struct {
	kind, code, name, month string
	branch                  pgtype.Int8
	amount                  int64
}

func incomeCode(purpose, chargeKind string) (kind, code string) {
	switch purpose {
	case "rent":
		return model.KindIncome, IncomeRent
	case "charge":
		if chargeKind == passThroughKind {
			return model.KindPassThrough, IncomeChargePrefix + chargeKind
		}
		if chargeKind == "" {
			return model.KindIncome, IncomeOther
		}
		return model.KindIncome, IncomeChargePrefix + chargeKind
	default:
		return model.KindIncome, IncomeOther
	}
}

func (s *Service) incomeExpenseFacts(ctx context.Context, orgID int64, from, to pgtype.Date, branchIDs []int64) ([]ieFact, error) {
	income, err := s.q.ReportIncomeFacts(ctx, db.ReportIncomeFactsParams{OrganizationID: orgID, DateFrom: from, DateTo: to, BranchIds: branchIDs})
	if err != nil {
		return nil, err
	}
	expenses, err := s.q.ReportExpenseFacts(ctx, db.ReportExpenseFactsParams{OrganizationID: orgID, DateFrom: from, DateTo: to, BranchIds: branchIDs})
	if err != nil {
		return nil, err
	}
	out := make([]ieFact, 0, len(income)+len(expenses))
	for _, r := range income {
		kind, code := incomeCode(r.Purpose, r.ChargeKind)
		out = append(out, ieFact{kind: kind, code: code, month: r.Month, branch: r.BranchID, amount: kurus(r.Amount)})
	}
	for _, r := range expenses {
		kind := model.KindExpense
		if r.CategoryCode == capexCategory {
			kind = model.KindCapex
		}
		out = append(out, ieFact{kind: kind, code: r.CategoryCode, name: r.CategoryName, month: r.Month, branch: r.BranchID, amount: kurus(r.Amount)})
	}
	return out, nil
}

// kindOrder sorts category rows: income, pass-through, expense, capex.
var kindOrder = map[string]int{model.KindIncome: 0, model.KindPassThrough: 1, model.KindExpense: 2, model.KindCapex: 3}

// IncomeExpense is income (ledger accruals of customer accounts, net of
// reversals, by entry date) against expenses (recorded expenses by expense
// date), by category, branch or month, with the monthly trend. Tolls
// passed through to the customer and vehicle procurement are kept apart.
func (s *Service) IncomeExpense(ctx context.Context, orgID int64, f model.Filter) (model.IncomeExpenseReport, error) {
	g, err := groupBy(f.GroupBy, model.IncomeExpenseGroups)
	if err != nil {
		return model.IncomeExpenseReport{}, err
	}
	all, err := s.branches(ctx, orgID)
	if err != nil {
		return model.IncomeExpenseReport{}, err
	}
	filter, err := branchFilter(ctx, all, f.BranchUUIDs)
	if err != nil {
		return model.IncomeExpenseReport{}, err
	}
	from, to := s.Period(f.From, f.To)
	facts, err := s.incomeExpenseFacts(ctx, orgID, pgDate(from), pgDate(to), effectiveBranches(ctx, filter))
	if err != nil {
		return model.IncomeExpenseReport{}, err
	}
	monthKeys := months(from, to)
	out := model.IncomeExpenseReport{From: from.String(), To: to.String(), GroupBy: g, Method: method(ctx), Rows: []model.IncomeExpenseRow{}}

	var total ieAgg
	byMonth := map[string]*ieAgg{}
	for _, m := range monthKeys {
		byMonth[m] = &ieAgg{}
	}
	for _, fc := range facts {
		total.add(fc.kind, fc.amount)
		if a, ok := byMonth[fc.month]; ok {
			a.add(fc.kind, fc.amount)
		}
	}
	out.Totals = total.figures()
	for _, m := range monthKeys {
		out.Trend = append(out.Trend, model.IncomeExpensePoint{Month: m, IncomeExpenseFigures: byMonth[m].figures()})
	}

	switch g {
	case model.GroupCategory:
		type cat struct {
			kind, code, name string
			amount           int64
		}
		cats := map[string]*cat{}
		for _, fc := range facts {
			key := fc.kind + ":" + fc.code
			c, ok := cats[key]
			if !ok {
				c = &cat{kind: fc.kind, code: fc.code, name: fc.name}
				cats[key] = c
			}
			c.amount += fc.amount
		}
		list := make([]*cat, 0, len(cats))
		for _, c := range cats {
			if c.amount != 0 {
				list = append(list, c)
			}
		}
		sort.Slice(list, func(i, j int) bool {
			a, b := list[i], list[j]
			if kindOrder[a.kind] != kindOrder[b.kind] {
				return kindOrder[a.kind] < kindOrder[b.kind]
			}
			if a.amount != b.amount {
				return a.amount > b.amount
			}
			return a.code < b.code
		})
		kindTotal := map[string]int64{
			model.KindIncome: total.income, model.KindExpense: total.expense, model.KindPassThrough: total.pass, model.KindCapex: total.capex,
		}
		for _, c := range list {
			var a ieAgg
			a.add(c.kind, c.amount)
			fig := a.figures()
			fig.Margin = nil
			out.Rows = append(out.Rows, model.IncomeExpenseRow{
				Key: c.kind + ":" + c.code, Kind: c.kind, Code: c.code, Name: c.name, IncomeExpenseFigures: fig,
				Share: pct(float64(c.amount), float64(kindTotal[c.kind])),
			})
		}
	case model.GroupBranch:
		type br struct {
			key, code, name string
			agg             ieAgg
		}
		rows := map[string]*br{}
		for _, fc := range facts {
			key, code, name := HeadOfficeKey, "", ""
			if fc.branch.Valid {
				b := all[fc.branch.Int64]
				key, code, name = b.uuid.String(), b.code, b.name
			}
			r, ok := rows[key]
			if !ok {
				r = &br{key: key, code: code, name: name}
				rows[key] = r
			}
			r.agg.add(fc.kind, fc.amount)
		}
		list := make([]*br, 0, len(rows))
		for _, r := range rows {
			list = append(list, r)
		}
		sort.Slice(list, func(i, j int) bool {
			if (list[i].key == HeadOfficeKey) != (list[j].key == HeadOfficeKey) {
				return list[j].key == HeadOfficeKey
			}
			return list[i].code < list[j].code
		})
		for _, r := range list {
			out.Rows = append(out.Rows, model.IncomeExpenseRow{
				Key: r.key, Kind: model.KindGroup, Code: r.code, Name: r.name, IncomeExpenseFigures: r.agg.figures(),
				Share: pct(float64(r.agg.income), float64(total.income)),
			})
		}
	case model.GroupMonth:
		for _, m := range monthKeys {
			a := byMonth[m]
			out.Rows = append(out.Rows, model.IncomeExpenseRow{
				Key: m, Kind: model.KindGroup, Code: m, Name: m, IncomeExpenseFigures: a.figures(),
				Share: pct(float64(a.income), float64(total.income)),
			})
		}
	}
	return out, nil
}
