// Package adapters connects corporate agreements to the shared export
// engine. The organization of an engine call always comes from the
// context (orgctx); the branch scope is the job's.
package adapters

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

// Resource is the engine slug of agreements.
const Resource = model.Resource

var (
	errNoOrganization = errors.New("agreements: organization scope is required")
	errNoImport       = errors.New("agreements: import is not supported")
)

// IO exports agreements (list filters, branch scope of the job).
type IO struct {
	svc *usecase.Service
}

// NewIO builds the export adapter.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

func (a *IO) Resource() string { return Resource }

func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "agreements." + key, Type: t}
	}
	s := ioengine.ColumnTypeString
	return []ioengine.Column{
		col("code", s), col("name", s), col("customer", s), col("status", s), col("valid_from", s), col("valid_to", s),
		col("invoice_period", s), col("payment_term_days", s), col("open_account", s), col("credit_limit", s),
		col("coupon_eligible", s), col("branch", s), col("notes", s), col("uuid", ioengine.ColumnTypeUUID),
		col("created_at", ioengine.ColumnTypeDatetime),
	}
}

func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, _ i18n.Locale) (ioengine.Dataset, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return ioengine.Dataset{}, errNoOrganization
	}
	rows, err := a.svc.ExportRows(ctx, orgID, model.FilterFromQuery(query))
	if err != nil {
		return ioengine.Dataset{}, err
	}
	yesNo := func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		row := map[string]any{
			"uuid": r.Uuid.String(), "code": r.Code, "name": r.Name, "customer": r.CustomerCompanyTitle, "status": r.Status,
			"valid_from": r.ValidFrom.Time.Format("2006-01-02"), "valid_to": "", "invoice_period": r.InvoicePeriod,
			"payment_term_days": r.PaymentTermDays, "open_account": yesNo(r.OpenAccount), "credit_limit": "",
			"coupon_eligible": yesNo(r.CouponEligible), "branch": r.BranchCode.String, "notes": r.Notes, "created_at": r.CreatedAt.Time.UTC(),
		}
		if r.ValidTo.Valid {
			row["valid_to"] = r.ValidTo.Time.Format("2006-01-02")
		}
		if r.CreditLimit.Valid {
			row["credit_limit"] = money.FormatNumeric(r.CreditLimit)
		}
		out = append(out, row)
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: out}, nil
}

func (a *IO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *IO) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }
