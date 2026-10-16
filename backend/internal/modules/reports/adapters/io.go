// Package adapters connects the reports (TEC-738) to the io engine
// (export only). Each report exports its grouped rows and a totals row;
// the period (or as-of date) and the method are printed in the notes
// above the table (PDF, XLSX, JSON) and in a column of every row (CSV).
package adapters

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

var errNoImport = errors.New("reports cannot be imported")

// base holds what every report adapter shares.
type base struct {
	svc      *usecase.Service
	resource string
	prefix   string
	columns  []string
}

func (b base) Resource() string { return b.resource }

func (b base) ExportColumns() []ioengine.Column {
	out := make([]ioengine.Column, 0, len(b.columns))
	for _, key := range b.columns {
		out = append(out, ioengine.Column{Key: key, LabelKey: b.prefix + "." + key, Type: ioengine.ColumnTypeString})
	}
	return out
}

func (b base) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (b base) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (b base) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }

// load parses the export query and resolves the organization.
func (b base) load(ctx context.Context, query ioengine.ExportQuery) (int64, model.Filter, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok || orgID == 0 {
		return 0, model.Filter{}, errors.New("report export: no organization in context")
	}
	f, err := usecase.ParseFilter(func(k string) string { return query[k] })
	return orgID, f, err
}

func (b base) dataset(rows []map[string]any, notes []string) ioengine.Dataset {
	return ioengine.Dataset{Resource: b.resource, Columns: b.ExportColumns(), Rows: rows, Notes: notes, Landscape: true}
}

func pct(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 1, 64)
}

func days(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

func optMoney(m *money.Money) string {
	if m == nil {
		return ""
	}
	return m.String()
}

// tl translates one key of the reports catalog (catalog_reports.go).
func tl(locale i18n.Locale, key string) string { return i18n.Translate(locale, key) }

// tr translates key, or returns fallback when the catalog has no entry.
func tr(locale i18n.Locale, key, fallback string) string {
	if v := tl(locale, key); v != key {
		return v
	}
	return fallback
}

func scopeNote(locale i18n.Locale, m model.Method) []string {
	notes := []string{tl(locale, "reports.note.amounts")}
	if m.BranchScope == model.ScopeBranches {
		notes = append(notes, tl(locale, "reports.note.scope_branches"))
	}
	return notes
}

func periodNote(locale i18n.Locale, from, to string, extra ...string) string {
	parts := append([]string{tl(locale, "reports.period") + ": " + from + " – " + to}, extra...)
	return strings.Join(parts, " · ")
}

// --- Occupancy -----------------------------------------------------------

// Occupancy exports the occupancy report.
type Occupancy struct{ base }

// NewOccupancyIO builds the adapter.
func NewOccupancyIO(svc *usecase.Service) *Occupancy {
	return &Occupancy{base{svc: svc, resource: model.ResourceOccupancy, prefix: "report_occupancy", columns: []string{
		"group", "vehicles", "rental_days", "available_days", "occupancy", "rental_count", "gross_income",
		"income_per_available_day", "period",
	}}}
}

func (a *Occupancy) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, f, err := a.load(ctx, query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.Occupancy(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	period := rep.From + " – " + rep.To
	row := func(label string, fig model.OccupancyFigures) map[string]any {
		return map[string]any{
			"group": label, "vehicles": strconv.Itoa(fig.Vehicles), "rental_days": days(fig.RentalDays),
			"available_days": days(fig.AvailableDays), "occupancy": pct(fig.Occupancy), "rental_count": strconv.Itoa(fig.RentalCount),
			"gross_income": fig.GrossIncome.String(), "income_per_available_day": optMoney(fig.IncomePerAvailableDay), "period": period,
		}
	}
	rows := make([]map[string]any, 0, len(rep.Rows)+1)
	for _, r := range rep.Rows {
		label := r.Group.Name
		switch {
		case r.Key == usecase.UngroupedKey:
			label = tl(locale, "reports.ungrouped")
		case rep.GroupBy == model.GroupRentalGroup && r.Group.Code != "":
			label = r.Group.Code + " · " + r.Group.Name
		}
		rows = append(rows, row(label, r.OccupancyFigures))
	}
	rows = append(rows, row(tl(locale, "reports.total"), rep.Totals))
	notes := []string{
		periodNote(locale, rep.From, rep.To, tl(locale, "reports.group_by")+": "+tl(locale, "reports.group."+rep.GroupBy)),
		tl(locale, "reports.note.occupancy"),
	}
	if rep.MultiGroupVehicles > 0 {
		notes = append(notes, tl(locale, "reports.note.multi_group"))
	}
	return a.dataset(rows, append(notes, scopeNote(locale, rep.Method)...)), nil
}

// --- Income / expense ----------------------------------------------------

// IncomeExpense exports the income / expense report.
type IncomeExpense struct{ base }

// NewIncomeExpenseIO builds the adapter.
func NewIncomeExpenseIO(svc *usecase.Service) *IncomeExpense {
	return &IncomeExpense{base{svc: svc, resource: model.ResourceIncomeExpense, prefix: "report_income_expense", columns: []string{
		"kind", "name", "income", "expense", "net", "pass_through", "capex", "margin", "share", "period",
	}}}
}

func (a *IncomeExpense) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, f, err := a.load(ctx, query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.IncomeExpense(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	period := rep.From + " – " + rep.To
	row := func(kind, name string, fig model.IncomeExpenseFigures, share *float64) map[string]any {
		return map[string]any{
			"kind": kind, "name": name, "income": fig.Income.String(), "expense": fig.Expense.String(), "net": fig.Net.String(),
			"pass_through": fig.PassThrough.String(), "capex": fig.Capex.String(), "margin": pct(fig.Margin), "share": pct(share),
			"period": period,
		}
	}
	rows := make([]map[string]any, 0, len(rep.Rows)+1)
	for _, r := range rep.Rows {
		kind := tl(locale, "reports.kind."+r.Kind)
		name := r.Name
		switch {
		case r.Kind == model.KindIncome || r.Kind == model.KindPassThrough:
			name = tr(locale, "reports.income_category."+r.Code, r.Code)
		case r.Key == usecase.HeadOfficeKey:
			name = tl(locale, "reports.head_office")
		}
		rows = append(rows, row(kind, name, r.IncomeExpenseFigures, r.Share))
	}
	rows = append(rows, row("", tl(locale, "reports.total"), rep.Totals, nil))
	notes := []string{
		periodNote(locale, rep.From, rep.To, tl(locale, "reports.group_by")+": "+tl(locale, "reports.group."+rep.GroupBy)),
		tl(locale, "reports.note.income_expense"),
		tl(locale, "reports.note.kept_apart"),
	}
	return a.dataset(rows, append(notes, scopeNote(locale, rep.Method)...)), nil
}

// --- Channels ------------------------------------------------------------

// Channels exports the channel performance report.
type Channels struct{ base }

// NewChannelsIO builds the adapter.
func NewChannelsIO(svc *usecase.Service) *Channels {
	return &Channels{base{svc: svc, resource: model.ResourceChannels, prefix: "report_channels", columns: []string{
		"channel", "reservations", "confirmed", "pending", "cancelled", "expired", "no_show", "cancellation_rate", "revenue",
		"commission", "net_revenue", "commission_rate", "average_revenue", "average_days", "revenue_share", "period",
	}}}
}

func (a *Channels) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, f, err := a.load(ctx, query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.Channels(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	period := rep.From + " – " + rep.To
	row := func(label string, fig model.ChannelFigures) map[string]any {
		return map[string]any{
			"channel": label, "reservations": strconv.Itoa(fig.Reservations), "confirmed": strconv.Itoa(fig.Confirmed),
			"pending": strconv.Itoa(fig.Pending), "cancelled": strconv.Itoa(fig.Cancelled), "expired": strconv.Itoa(fig.Expired),
			"no_show": strconv.Itoa(fig.NoShow), "cancellation_rate": pct(fig.CancellationRate), "revenue": fig.Revenue.String(),
			"commission": fig.Commission.String(), "net_revenue": fig.NetRevenue.String(), "commission_rate": pct(fig.CommissionRate),
			"average_revenue": optMoney(fig.AverageRevenue), "average_days": days(fig.AverageDays), "revenue_share": pct(fig.RevenueShare),
			"period": period,
		}
	}
	rows := make([]map[string]any, 0, len(rep.Rows)+1)
	for _, r := range rep.Rows {
		rows = append(rows, row(tr(locale, "reports.channel."+r.Channel, r.Channel), r.ChannelFigures))
	}
	rows = append(rows, row(tl(locale, "reports.total"), rep.Totals))
	notes := []string{
		periodNote(locale, rep.From, rep.To, tl(locale, "reports.date_basis")+": "+tl(locale, "reports.date_basis."+rep.DateBasis)),
		tl(locale, "reports.note.channels"),
	}
	return a.dataset(rows, append(notes, scopeNote(locale, rep.Method)...)), nil
}

// --- Aging ---------------------------------------------------------------

// Aging exports the receivable / payable aging.
type Aging struct{ base }

// NewAgingIO builds the adapter.
func NewAgingIO(svc *usecase.Service) *Aging {
	return &Aging{base{svc: svc, resource: model.ResourceAging, prefix: "report_aging", columns: []string{
		"code", "name", "payment_term_days", "current", "days_1_30", "days_31_60", "days_61_90", "days_90_plus", "overdue", "open",
		"credit", "balance", "last_entry", "as_of",
	}}}
}

func (a *Aging) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, f, err := a.load(ctx, query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.Aging(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	row := func(code, name, term string, fig model.AgingFigures, balance, last string) map[string]any {
		return map[string]any{
			"code": code, "name": name, "payment_term_days": term, "current": fig.Current.String(), "days_1_30": fig.Days30.String(),
			"days_31_60": fig.Days60.String(), "days_61_90": fig.Days90.String(), "days_90_plus": fig.Over90.String(),
			"overdue": fig.Overdue.String(), "open": fig.Open.String(), "credit": fig.Credit.String(), "balance": balance,
			"last_entry": last, "as_of": rep.AsOf,
		}
	}
	rows := make([]map[string]any, 0, len(rep.Rows)+1)
	for _, r := range rep.Rows {
		last := ""
		if r.LastEntry != nil {
			last = *r.LastEntry
		}
		rows = append(rows, row(r.Code, r.Name, strconv.Itoa(int(r.PaymentTermDays)), r.AgingFigures, r.Balance.String(), last))
	}
	rows = append(rows, row("", tl(locale, "reports.total"), "", rep.Totals, "", ""))
	notes := []string{
		tl(locale, "reports.as_of") + ": " + rep.AsOf + " · " + tl(locale, "reports.party."+rep.Party),
		tl(locale, "reports.note.aging"),
	}
	return a.dataset(rows, append(notes, scopeNote(locale, rep.Method)...)), nil
}

// --- Branch comparison ---------------------------------------------------

// Branches exports the branch comparison.
type Branches struct{ base }

// NewBranchesIO builds the adapter.
func NewBranchesIO(svc *usecase.Service) *Branches {
	return &Branches{base{svc: svc, resource: model.ResourceBranches, prefix: "report_branches", columns: []string{
		"branch", "vehicles", "rental_count", "rental_days", "occupancy", "gross_income", "commissions", "direct_costs", "overhead",
		"contribution", "margin", "income_per_vehicle", "contribution_share", "period",
	}}}
}

func (a *Branches) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, f, err := a.load(ctx, query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.Branches(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	period := rep.From + " – " + rep.To
	row := func(label string, fig model.BranchFigures) map[string]any {
		return map[string]any{
			"branch": label, "vehicles": strconv.Itoa(fig.Vehicles), "rental_count": strconv.Itoa(fig.RentalCount),
			"rental_days": days(fig.RentalDays), "occupancy": pct(fig.Occupancy), "gross_income": fig.GrossIncome.String(),
			"commissions": fig.Commissions.String(), "direct_costs": fig.DirectCosts.String(), "overhead": fig.Overhead.String(),
			"contribution": fig.Contribution.String(), "margin": pct(fig.Margin), "income_per_vehicle": optMoney(fig.IncomePerVehicle),
			"contribution_share": pct(fig.ContributionShare), "period": period,
		}
	}
	rows := make([]map[string]any, 0, len(rep.Rows)+1)
	for _, r := range rep.Rows {
		rows = append(rows, row(r.Branch.Name, r.BranchFigures))
	}
	rows = append(rows, row(tl(locale, "reports.total"), rep.Totals))
	notes := []string{
		periodNote(locale, rep.From, rep.To,
			tl(locale, "vehicle_profitability.basis_label")+": "+tl(locale, "vehicle_profitability.basis."+rep.Method.Basis),
			tl(locale, "vehicle_profitability.allocation_label")+": "+tl(locale, "vehicle_profitability.allocation."+rep.Method.Allocation)),
		tl(locale, "reports.note.branches"),
	}
	if rep.Method.UnallocatedOverhead.Minor() != 0 {
		notes = append(notes, tl(locale, "vehicle_profitability.note.unallocated")+" "+rep.Method.UnallocatedOverhead.String())
	}
	return a.dataset(rows, append(notes, scopeNote(locale, rep.Method.Method)...)), nil
}
