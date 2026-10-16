package adapters

import (
	"context"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
)

// SupplierInvoiceFilter maps list query params to an invoice filter.
func SupplierInvoiceFilter(q map[string]string) model.SupplierInvoiceFilter {
	window, _ := strconv.Atoi(strings.TrimSpace(q["window"]))
	return model.SupplierInvoiceFilter{
		Q: strings.TrimSpace(q["q"]), SupplierUUIDs: uuids(q["supplier"]), Statuses: csv(q["status"]),
		DueStates: csv(q["due_state"]), CategoryUUIDs: uuids(q["category"]), BranchUUIDs: uuids(q["branch"]),
		From: datePtr(q["from"]), To: datePtr(q["to"]), DueTo: datePtr(q["due_to"]), WindowDays: window,
	}
}

// SupplierPaymentFilter maps list query params to a payment filter.
func SupplierPaymentFilter(q map[string]string) model.SupplierPaymentFilter {
	return model.SupplierPaymentFilter{
		Q: strings.TrimSpace(q["q"]), SupplierUUIDs: uuids(q["supplier"]), CashAccountUUIDs: uuids(q["cash_account"]),
		Statuses: csv(q["status"]), Methods: csv(q["method"]), InvoiceUUID: uuidPtr(q["invoice"]),
		From: datePtr(q["from"]), To: datePtr(q["to"]),
	}
}

// ------------------------------------------------------- supplier invoices

// SupplierInvoicesIO exports supplier invoices (the payables list and the
// "due soon" view; imports go through the dialog, one posting per invoice).
type SupplierInvoicesIO struct{ svc *usecase.Service }

// NewSupplierInvoicesIO builds the adapter.
func NewSupplierInvoicesIO(svc *usecase.Service) *SupplierInvoicesIO {
	return &SupplierInvoicesIO{svc: svc}
}

func (a *SupplierInvoicesIO) Resource() string { return model.ResourceSupplierInvoices }

func (a *SupplierInvoicesIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "supplier_invoices." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("invoice_no"), col("supplier_code"), col("supplier"), col("invoice_date"), col("due_date"), col("category"),
		col("branch_code"), col("plate"), col("net_amount"), col("vat_amount"), col("total_amount"), col("paid_amount"),
		col("open_amount"), col("currency"), col("due_state"), col("status"), col("description"),
		{Key: "uuid", LabelKey: "supplier_invoices.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *SupplierInvoicesIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := SupplierInvoiceFilter(query)
	f.Limit = usecase.MaxListTargets
	items, _, err := a.svc.ListSupplierInvoices(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		branch, plate := "", ""
		if it.Branch != nil {
			branch = it.Branch.Code
		}
		if it.Vehicle != nil {
			plate = it.Vehicle.Code
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "invoice_no": it.InvoiceNo, "supplier_code": it.Supplier.Code, "supplier": it.Supplier.Name,
			"invoice_date": it.InvoiceDate, "due_date": it.DueDate, "category": it.Category.Name, "branch_code": branch,
			"plate": plate, "net_amount": it.NetAmount.String(), "vat_amount": it.VATAmount.String(),
			"total_amount": it.TotalAmount.String(), "paid_amount": it.PaidAmount.String(), "open_amount": it.OpenAmount.String(),
			"currency": it.Currency, "due_state": label(locale, "finance.due_state.", it.DueState),
			"status": label(locale, "finance.supplier_invoice_status.", it.Status), "description": it.Description,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceSupplierInvoices, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *SupplierInvoicesIO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *SupplierInvoicesIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *SupplierInvoicesIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// ------------------------------------------------------- supplier payments

// SupplierPaymentsIO exports supplier payments.
type SupplierPaymentsIO struct{ svc *usecase.Service }

// NewSupplierPaymentsIO builds the adapter.
func NewSupplierPaymentsIO(svc *usecase.Service) *SupplierPaymentsIO {
	return &SupplierPaymentsIO{svc: svc}
}

func (a *SupplierPaymentsIO) Resource() string { return model.ResourceSupplierPayments }

func (a *SupplierPaymentsIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "supplier_payments." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("payment_date"), col("supplier_code"), col("supplier"), col("cash_account_code"), col("method"), col("reference"),
		col("amount"), col("allocated_amount"), col("unallocated_amount"), col("currency"), col("invoices"), col("status"),
		col("description"),
		{Key: "uuid", LabelKey: "supplier_payments.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *SupplierPaymentsIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := SupplierPaymentFilter(query)
	f.Limit = usecase.MaxListTargets
	items, _, err := a.svc.ListSupplierPayments(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		invoices := make([]string, 0, len(it.Allocations))
		for _, al := range it.Allocations {
			invoices = append(invoices, al.InvoiceNo+" ("+al.Amount.String()+")")
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "payment_date": it.PaymentDate, "supplier_code": it.Supplier.Code, "supplier": it.Supplier.Name,
			"cash_account_code": it.CashAccount.Code, "method": label(locale, "finance.supplier_payment_method.", it.Method),
			"reference": it.Reference, "amount": it.Amount.String(), "allocated_amount": it.AllocatedAmount.String(),
			"unallocated_amount": it.UnallocatedAmt.String(), "currency": it.Currency, "invoices": strings.Join(invoices, ", "),
			"status": label(locale, "finance.supplier_payment_status.", it.Status), "description": it.Description,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceSupplierPayments, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *SupplierPaymentsIO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *SupplierPaymentsIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *SupplierPaymentsIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

var (
	_ ioengine.ResourceAdapter = (*SupplierInvoicesIO)(nil)
	_ ioengine.ResourceAdapter = (*SupplierPaymentsIO)(nil)
)
