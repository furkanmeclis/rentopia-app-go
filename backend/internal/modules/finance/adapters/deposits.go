package adapters

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

// DepositFilter maps list query params to a deposit filter
// (held_over_days: only deposits held for more than N days).
func DepositFilter(q map[string]string) model.DepositFilter {
	f := model.DepositFilter{
		Q: strings.TrimSpace(q["q"]), Statuses: csv(q["status"]), Methods: csv(q["method"]), BranchUUIDs: uuids(q["branch"]),
		RentalUUID: uuidPtr(q["rental"]), From: datePtr(q["from"]), To: datePtr(q["to"]),
	}
	if raw := strings.TrimSpace(q["held_over_days"]); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			n = -1 // refused by the use case
		}
		f.HeldOverDays = &n
	}
	return f
}

// DepositsIO exports security deposits (no import: a deposit is money
// taken at the counter, never a spreadsheet row).
type DepositsIO struct{ svc *usecase.Service }

// NewDepositsIO builds the adapter.
func NewDepositsIO(svc *usecase.Service) *DepositsIO { return &DepositsIO{svc: svc} }

func (a *DepositsIO) Resource() string { return model.ResourceDeposits }

func (a *DepositsIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "security_deposits." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("created_at"), col("rental"), col("customer"), col("branch"), col("method"), col("status"), col("amount"),
		col("captured_amount"), col("returned_amount"), col("currency"), col("cash_account"), col("pos_reference"),
		col("held_at"), col("held_days"), col("resolved_at"), col("note"),
		{Key: "uuid", LabelKey: "security_deposits.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func istanbul(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(rentaltime.Location()).Format("2006-01-02 15:04")
}

func (a *DepositsIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := DepositFilter(query)
	f.Limit, f.SortField, f.SortDesc = usecase.MaxListTargets, "created_at", true
	items, _, err := a.svc.ListDeposits(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		created := it.CreatedAt
		heldDays := ""
		if it.Status == model.DepositHeld {
			heldDays = strconv.Itoa(it.HeldDays)
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "created_at": istanbul(&created), "rental": it.Rental.Code, "customer": it.Customer.Name,
			"branch": refText(&it.Branch), "method": label(locale, "finance.deposit_method.", it.Method),
			"status": label(locale, "finance.deposit_status.", it.Status), "amount": it.Amount.String(),
			"captured_amount": it.CapturedAmount.String(), "returned_amount": it.ReturnedAmount.String(), "currency": it.Currency,
			"cash_account": refText(it.CashAccount), "pos_reference": it.POSReference, "held_at": istanbul(it.HeldAt),
			"held_days": heldDays, "resolved_at": istanbul(it.ResolvedAt), "note": it.Note,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceDeposits, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *DepositsIO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *DepositsIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *DepositsIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}
