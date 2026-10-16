package usecase

import (
	"context"
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/jackc/pgx/v5/pgtype"
)

// Aging bucket keys (chart and export).
var AgingBucketKeys = []string{"current", "days_1_30", "days_31_60", "days_61_90", "days_90_plus"}

func addAging(a, b model.AgingFigures) model.AgingFigures {
	return model.AgingFigures{
		Current: a.Current.Add(b.Current), Days30: a.Days30.Add(b.Days30), Days60: a.Days60.Add(b.Days60),
		Days90: a.Days90.Add(b.Days90), Over90: a.Over90.Add(b.Over90), Overdue: a.Overdue.Add(b.Overdue),
		Open: a.Open.Add(b.Open), Credit: a.Credit.Add(b.Credit),
	}
}

func zeroAging() model.AgingFigures {
	z := money.Zero()
	return model.AgingFigures{Current: z, Days30: z, Days60: z, Days90: z, Over90: z, Overdue: z, Open: z, Credit: z}
}

// agingOf buckets one account: settlements pay the oldest charges first
// (FIFO), so the open part of the charges due before a threshold is
// max(0, charges before it − settlements).
func agingOf(r db.ReportAgingAccountsRow) model.AgingFigures {
	settled := money.FromNumericOrZero(r.Settlements)
	open := func(c pgtype.Numeric) money.Money { return money.FromNumericOrZero(c).Sub(settled).ClampZero() }
	o90, o60, o30, oOver, oAll := open(r.Charges90), open(r.Charges60), open(r.Charges30), open(r.ChargesOverdue), open(r.ChargesAll)
	return model.AgingFigures{
		Over90: o90, Days90: o60.Sub(o90), Days60: o30.Sub(o60), Days30: oOver.Sub(o30), Current: oAll.Sub(oOver),
		Overdue: oOver, Open: oAll, Credit: settled.Sub(money.FromNumericOrZero(r.ChargesAll)).ClampZero(),
	}
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := rentaltime.DateOf(d.Time).String()
	return &s
}

// Aging is the receivable (customer accounts) or payable (supplier
// accounts) aging as of a date: open charges by days past their due date
// (not due, 1–30, 31–60, 61–90, 90+), FIFO against the settlements.
// Accounts with nothing open and no credit are left out.
func (s *Service) Aging(ctx context.Context, orgID int64, f model.Filter) (model.AgingReport, error) {
	party := f.Party
	if party == "" {
		party = model.PartyCustomer
	}
	if !contains(model.Parties, party) {
		return model.AgingReport{}, invalid("party", "invalid", "party must be customer or supplier")
	}
	all, err := s.branches(ctx, orgID)
	if err != nil {
		return model.AgingReport{}, err
	}
	filter, err := branchFilter(ctx, all, f.BranchUUIDs)
	if err != nil {
		return model.AgingReport{}, err
	}
	asOf := rentaltime.LocalDate(s.clock())
	if f.AsOf != nil {
		asOf = rentaltime.DateOf(*f.AsOf)
	}
	rows, err := s.q.ReportAgingAccounts(ctx, db.ReportAgingAccountsParams{
		OrganizationID: orgID, PartyKind: party, AsOf: pgDate(asOf),
		D30: pgDate(asOf.AddDays(-30)), D60: pgDate(asOf.AddDays(-60)), D90: pgDate(asOf.AddDays(-90)),
		BranchIds: effectiveBranches(ctx, filter),
	})
	if err != nil {
		return model.AgingReport{}, err
	}
	out := model.AgingReport{AsOf: asOf.String(), Party: party, Method: method(ctx), Rows: []model.AgingRow{}, Totals: zeroAging()}
	for _, r := range rows {
		a := agingOf(r)
		if a.Open.IsZero() && a.Credit.IsZero() {
			continue
		}
		out.Totals = addAging(out.Totals, a)
		bal := money.FromNumericOrZero(r.Balance)
		if party == model.PartySupplier {
			// A supplier's balance reads as what we owe.
			bal = bal.Neg()
		}
		out.Rows = append(out.Rows, model.AgingRow{
			UUID: r.Uuid, Code: r.Code, Name: r.Name, PartyKind: r.PartyKind, PaymentTermDays: r.PaymentTermDays,
			Balance: bal, AgingFigures: a, LastEntry: dateStr(r.LastEntry),
		})
	}
	sort.SliceStable(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i], out.Rows[j]
		if a.Overdue.Minor() != b.Overdue.Minor() {
			return a.Overdue.Minor() > b.Overdue.Minor()
		}
		if a.Open.Minor() != b.Open.Minor() {
			return a.Open.Minor() > b.Open.Minor()
		}
		return a.Code < b.Code
	})
	t := out.Totals
	out.Buckets = []model.AgingBucket{
		{Key: "current", Amount: t.Current}, {Key: "days_1_30", Amount: t.Days30}, {Key: "days_31_60", Amount: t.Days60},
		{Key: "days_61_90", Amount: t.Days90}, {Key: "days_90_plus", Amount: t.Over90},
	}
	return out, nil
}
