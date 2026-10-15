// Package adapters connects the pricing resources to the shared engines:
// ioengine (rate plan price list export + İdes XLSX import with a dry-run
// diff; date rule export) and the one-way fee lookup of fleetops for the
// preview. The organization and the branch scope always come from the
// context (orgctx): the request scope on tenant routes, the job scope in
// the worker. Without an organization every call fails.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
)

var (
	errNoOrganization = errors.New("pricing: organization scope is required")
	errNoImport       = errors.New("pricing: import is not supported for this resource")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

func cell(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// ---------------------------------------------------------------- rate plans

// RatePlansIO exports the price list (one row per band of the effective
// version) and imports the İdes price list into drafts.
type RatePlansIO struct {
	svc *usecase.Service
}

// NewRatePlansIO builds the rate plans io adapter. svc may be nil for
// metadata only.
func NewRatePlansIO(svc *usecase.Service) *RatePlansIO { return &RatePlansIO{svc: svc} }

func (a *RatePlansIO) Resource() string { return model.ResourceRatePlans }

func (a *RatePlansIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "rate_plans." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("product_code"), col("product_title"), col("band"), col("daily_price"), col("branch_code"), col("channel"),
		col("priority"), col("scope"), col("name"), col("status"), col("version"),
		{Key: "uuid", LabelKey: "rate_plans.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *RatePlansIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.PlanExportRows(ctx, orgID, model.RatePlanFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var max *int
		if r.MaxDays.Valid {
			v := int(r.MaxDays.Int32)
			max = &v
		}
		out = append(out, map[string]any{
			"product_code": r.ProductCode, "product_title": r.ProductTitle,
			"band": usecase.FormatBand(int(r.MinDays), max), "daily_price": money.FormatNumeric(r.DailyPrice),
			"branch_code": r.BranchCode.String, "channel": r.Channel.String, "priority": fmt.Sprint(r.Priority),
			"scope": r.Scope, "name": r.Name, "status": r.Status,
			"version": fmt.Sprintf("%d (%s)", r.Version, r.VersionStatus), "uuid": r.Uuid.String(),
		})
	}
	return ioengine.Dataset{Resource: model.ResourceRatePlans, Columns: a.ExportColumns(), Rows: out}, nil
}

// ImportSchema follows the İdes price list (ürün kodu, bant, günlük fiyat);
// branch / channel / priority target an override plan.
func (a *RatePlansIO) ImportSchema() []ioengine.ImportField {
	f := func(key string, required bool, hint string) ioengine.ImportField {
		return ioengine.ImportField{Key: key, LabelKey: "rate_plans." + key, Type: ioengine.ColumnTypeString, Required: required, DefaultHint: hint}
	}
	return []ioengine.ImportField{
		f("product_code", true, ""), f("band", true, "1-3, 4-7, 15+"), f("daily_price", true, "1300.00"),
		f("branch_code", false, ""), f("channel", false, "web, branch …"), f("priority", false, "0"),
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *RatePlansIO) SampleRows() []map[string]any {
	return []map[string]any{
		{"product_code": "EKO-M", "band": "1-3", "daily_price": "1500.00", "branch_code": "", "channel": "", "priority": "0"},
		{"product_code": "EKO-M", "band": "4-7", "daily_price": "1400.00", "branch_code": "", "channel": "", "priority": "0"},
		{"product_code": "EKO-M", "band": "8-14", "daily_price": "1300.00", "branch_code": "", "channel": "", "priority": "0"},
		{"product_code": "EKO-M", "band": "15+", "daily_price": "1200.00", "branch_code": "", "channel": "", "priority": "0"},
		{"product_code": "EKO-M", "band": "1-3", "daily_price": "1450.00", "branch_code": "MRK", "channel": "web", "priority": "0"},
	}
}

func priceRow(row, defaults map[string]any) usecase.PriceRow {
	get := func(key string) string {
		if v := cell(row, key); v != "" {
			return v
		}
		return cell(defaults, key)
	}
	return usecase.PriceRow{
		ProductCode: get("product_code"), Band: get("band"), DailyPrice: get("daily_price"),
		BranchCode: get("branch_code"), Channel: get("channel"), Priority: get("priority"),
	}
}

func rowError(err error) bool {
	var re *usecase.RowError
	return errors.As(err, &re)
}

func (a *RatePlansIO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowResult{}, err
	}
	res, err := a.svc.ApplyPriceRow(ctx, orgID, priceRow(row, defaults))
	if err != nil {
		if rowError(err) {
			return ioengine.RowResult{OK: false, Error: err.Error()}, nil
		}
		return ioengine.RowResult{}, err
	}
	return ioengine.RowResult{
		OK: true, EntityType: "rate_plan_band", EntityUUID: res.PlanUUID.String(), Op: res.Op, Previous: res.Previous,
	}, nil
}

// PreviewRow implements ioengine.RowPreviewer (dry-run diff).
func (a *RatePlansIO) PreviewRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowDiff, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.RowDiff{}, err
	}
	res, err := a.svc.PreviewPriceRow(ctx, orgID, priceRow(row, defaults))
	if err != nil {
		if rowError(err) {
			return ioengine.RowDiff{Error: err.Error()}, nil
		}
		return ioengine.RowDiff{}, err
	}
	out := ioengine.RowDiff{Op: res.Op, Changes: make([]ioengine.FieldChange, 0, len(res.Changes))}
	for _, c := range res.Changes {
		out.Changes = append(out.Changes, ioengine.FieldChange{Field: c.Field, From: c.From, To: c.To})
	}
	return out, nil
}

func (a *RatePlansIO) RevertRow(ctx context.Context, entityType, _ string, previous map[string]any) error {
	if entityType != "rate_plan_band" {
		return fmt.Errorf("unsupported entity %q", entityType)
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	return a.svc.RevertPriceRow(ctx, orgID, previous)
}

// ---------------------------------------------------------------- date rules

// DateRulesIO exports date rules (no import).
type DateRulesIO struct {
	svc *usecase.Service
}

// NewDateRulesIO builds the date rules io adapter.
func NewDateRulesIO(svc *usecase.Service) *DateRulesIO { return &DateRulesIO{svc: svc} }

func (a *DateRulesIO) Resource() string { return model.ResourceDateRules }

func (a *DateRulesIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "date_rules." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("name"), col("start_date"), col("end_date"), col("kind"), col("value"), col("priority"),
		col("product_scope"), col("branches"), col("channels"), col("status"), col("version"),
		{Key: "uuid", LabelKey: "date_rules.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *DateRulesIO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows, err := a.svc.RuleExportRows(ctx, orgID, model.DateRuleFilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		branches, channels := "all", "all"
		if !r.AllBranches {
			branches = "selected"
		}
		if r.Channels != nil {
			channels = strings.Join(r.Channels, ", ")
		}
		out = append(out, map[string]any{
			"name": r.Name, "start_date": rentaltime.DateOf(r.StartDate.Time).String(),
			"end_date": rentaltime.DateOf(r.EndDate.Time).String(), "kind": r.Kind, "value": money.FormatNumeric(r.Value),
			"priority": fmt.Sprint(r.Priority), "product_scope": r.ProductScope, "branches": branches, "channels": channels,
			"status": r.Status, "version": fmt.Sprintf("%d (%s)", r.Version, r.VersionStatus), "uuid": r.Uuid.String(),
		})
	}
	return ioengine.Dataset{Resource: model.ResourceDateRules, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *DateRulesIO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *DateRulesIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *DateRulesIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// ------------------------------------------------------------------ one-way

// OneWay resolves one-way fees through the fleetops use case (preview).
type OneWay struct {
	svc *fleetopsusecase.Service
}

// NewOneWay builds the one-way fee resolver.
func NewOneWay(svc *fleetopsusecase.Service) *OneWay { return &OneWay{svc: svc} }

// ResolveOneWayFee implements usecase.OneWayResolver.
func (o *OneWay) ResolveOneWayFee(ctx context.Context, orgID int64, from, to, product uuid.UUID) (bool, money.Money, error) {
	q, err := o.svc.ResolveOneWay(ctx, orgID, from, to, &product)
	if err != nil {
		return false, money.Money{}, err
	}
	fee, err := money.Parse(q.Fee)
	if err != nil {
		return false, money.Money{}, err
	}
	return q.Allowed, fee, nil
}

var (
	_ ioengine.ResourceAdapter = (*RatePlansIO)(nil)
	_ ioengine.RowPreviewer    = (*RatePlansIO)(nil)
	_ ioengine.SampleProvider  = (*RatePlansIO)(nil)
	_ ioengine.ResourceAdapter = (*DateRulesIO)(nil)
	_ usecase.OneWayResolver   = (*OneWay)(nil)
)
