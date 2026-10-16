// Package adapters connects the vehicle profitability report to the io
// engine (export only).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

var errNoImport = errors.New("the profitability report cannot be imported")

// IO exports the fleet report: one row per vehicle and a totals row. The
// period, the basis and the allocation method are stated in every row
// (CSV) and in the notes above the table (PDF, XLSX, JSON).
type IO struct{ svc *usecase.Service }

// NewIO builds the adapter.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return model.Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "vehicle_profitability." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("plate"), col("home_branch"), col("rental_days"), col("occupancy"), col("gross_income"), col("commissions"),
		col("direct_costs"), col("overhead"), col("contribution"), col("margin"), col("refunded"), col("deposits_held"),
		col("pass_through"), col("basis"), col("allocation"), col("period"),
	}
}

func pct(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 1, 64)
}

func (a *IO) row(f model.Figures, plate, branch, basis, allocation, period string) map[string]any {
	return map[string]any{
		"plate": plate, "home_branch": branch,
		"rental_days": strconv.FormatFloat(f.RentalDays, 'f', 2, 64), "occupancy": pct(f.Occupancy),
		"gross_income": f.GrossIncome.String(), "commissions": f.Commissions.String(), "direct_costs": f.DirectCosts.String(),
		"overhead": f.Overhead.String(), "contribution": f.Contribution.String(), "margin": pct(f.Margin),
		"refunded": f.Refunded.String(), "deposits_held": f.DepositsHeld.String(), "pass_through": f.PassThrough.String(),
		"basis": basis, "allocation": allocation, "period": period,
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok || orgID == 0 {
		return ioengine.Dataset{}, errors.New("profitability export: no organization in context")
	}
	f, err := usecase.ParseFilter(func(k string) string { return query[k] })
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rep, err := a.svc.Report(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	t := func(key string) string { return i18n.Translate(locale, key) }
	basis := t("vehicle_profitability.basis." + rep.Method.Basis)
	allocation := t("vehicle_profitability.allocation." + rep.Method.Allocation)
	period := rep.From + " – " + rep.To
	rows := make([]map[string]any, 0, len(rep.Rows)+1)
	for _, r := range rep.Rows {
		rows = append(rows, a.row(r.Figures, r.Vehicle.Plate, r.Vehicle.HomeBranch.Name, basis, allocation, period))
	}
	rows = append(rows, a.row(rep.Totals, t("vehicle_profitability.total"), "", basis, allocation, period))
	notes := []string{
		fmt.Sprintf("%s: %s · %s: %s · %s: %s", t("vehicle_profitability.period"), period,
			t("vehicle_profitability.basis_label"), basis, t("vehicle_profitability.allocation_label"), allocation),
		t("vehicle_profitability.note.amounts"),
		t("vehicle_profitability.note.basis." + rep.Method.Basis),
		t("vehicle_profitability.note.allocation." + rep.Method.Allocation),
		t("vehicle_profitability.note.kept_apart"),
	}
	if rep.Method.OverheadScope == model.OverheadBranches {
		notes = append(notes, t("vehicle_profitability.note.overhead_branches"))
	}
	if rep.Method.AllocationFallback {
		notes = append(notes, t("vehicle_profitability.note.fallback"))
	}
	if rep.Method.UnallocatedOverhead.Minor() != 0 {
		notes = append(notes, t("vehicle_profitability.note.unallocated")+" "+rep.Method.UnallocatedOverhead.String())
	}
	return ioengine.Dataset{Resource: model.Resource, Columns: a.ExportColumns(), Rows: rows, Notes: notes, Landscape: true}, nil
}

func (a *IO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *IO) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }
