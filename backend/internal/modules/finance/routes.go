// Package finance is the M4 finance foundation (/v1/tenant/finance): the
// customer / supplier ledger ("cari", TEC-717) and the cash / bank book
// with expenses (TEC-720). docs/design/ledger.md, docs/design/cash-accounts.md.
package finance

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	financehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Engine slugs.
const (
	ResourceAccounts     = model.ResourceAccounts
	ResourceEntries      = model.ResourceEntries
	ResourceCashAccounts = model.ResourceCashAccounts
	ResourceMovements    = model.ResourceMovements
	ResourceExpenses     = model.ResourceExpenses
	// Supplier invoices and payments (TEC-721).
	ResourceSupplierInvoices = model.ResourceSupplierInvoices
	ResourceSupplierPayments = model.ResourceSupplierPayments
	// ResourceCollections: collections (TEC-718).
	ResourceCollections = model.ResourceCollections
)

// ResourceEngines are the engine handlers of one resource (nil members
// skip the route).
type ResourceEngines struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
	Bulk   http.HandlerFunc
}

// EngineHandlers are the shared engine handlers per finance resource.
type EngineHandlers struct {
	Accounts, Entries, CashAccounts, Movements, Expenses ResourceEngines
	SupplierInvoices, SupplierPayments                   ResourceEngines
	Collections                                          ResourceEngines
}

// RegisterRoutes mounts /v1/tenant/finance. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *financehandler.Handler,
	engines EngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	accRead, accWrite := perm(rbac.PermTenantFinanceAccountsRead), perm(rbac.PermTenantFinanceAccountsWrite)
	accExport := perm(rbac.PermTenantFinanceAccountsExport)
	cashRead, cashWrite := perm(rbac.PermTenantFinanceCashRead), perm(rbac.PermTenantFinanceCashWrite)
	cashExport, cashImport := perm(rbac.PermTenantFinanceCashExport), perm(rbac.PermTenantFinanceCashImport)
	expRead, expWrite := perm(rbac.PermTenantFinanceExpensesRead), perm(rbac.PermTenantFinanceExpensesWrite)
	expExport, expImport := perm(rbac.PermTenantFinanceExpensesExport), perm(rbac.PermTenantFinanceExpensesImport)
	reports := perm(rbac.PermTenantFinanceReportsRead)

	// Ledger.
	route("GET /v1/tenant/finance/summary", h.Summary, accRead)
	route("GET /v1/tenant/finance/rental-balances", h.RentalBalances, accRead)
	route("GET /v1/tenant/finance/accounts/meta", h.AccountsMeta, accRead)
	route("GET /v1/tenant/finance/accounts", h.ListAccounts, accRead)
	route("POST /v1/tenant/finance/accounts", h.CreateAccount, accWrite)
	if engines.Accounts.Export != nil {
		route("POST /v1/tenant/finance/accounts/export", engines.Accounts.Export, accRead, accExport)
	}
	route("GET /v1/tenant/finance/accounts/{uuid}", h.GetAccount, accRead)
	route("PATCH /v1/tenant/finance/accounts/{uuid}", h.PatchAccount, accWrite)
	route("GET /v1/tenant/finance/accounts/{uuid}/statement", h.Statement, accRead)
	route("POST /v1/tenant/finance/accounts/{uuid}/collections", h.Collect, accWrite)
	route("POST /v1/tenant/finance/accounts/{uuid}/refunds", h.Refund, accWrite)
	route("POST /v1/tenant/finance/accounts/{uuid}/adjustments", h.Adjust, accWrite)
	route("POST /v1/tenant/finance/accounts/{uuid}/offsets", h.Offset, accWrite)
	route("POST /v1/tenant/finance/accounts/{uuid}/send-statement", h.SendStatement, accWrite)
	route("GET /v1/tenant/finance/entries/meta", h.EntriesMeta, accRead)
	route("GET /v1/tenant/finance/entries", h.ListEntries, accRead)
	if engines.Entries.Export != nil {
		route("POST /v1/tenant/finance/entries/export", engines.Entries.Export, accRead, accExport)
	}
	route("GET /v1/tenant/finance/entries/{uuid}", h.GetEntry, accRead)
	route("POST /v1/tenant/finance/entries/{uuid}/reverse", h.ReverseEntry, accWrite)
	route("GET /v1/tenant/finance/accounts/{uuid}/open-items", h.OpenItems, accRead)
	route("GET /v1/tenant/finance/rentals/{uuid}", h.RentalLedger, accRead)
	route("GET /v1/tenant/finance/customers/{uuid}", h.CustomerLedger, accRead)

	// Collections, allocation to open items, refunds (TEC-718).
	collect, refund := perm(rbac.PermTenantPaymentCollect), perm(rbac.PermTenantPaymentRefund)
	adjust := perm(rbac.PermTenantFinanceLedgerAdjust)
	route("GET /v1/tenant/finance/collections/meta", h.CollectionsMeta, accRead)
	route("GET /v1/tenant/finance/collections", h.ListCollections, accRead)
	route("POST /v1/tenant/finance/collections", h.CreateCollection, accRead, collect)
	if engines.Collections.Export != nil {
		route("POST /v1/tenant/finance/collections/export", engines.Collections.Export, accRead, accExport)
	}
	if engines.Collections.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/finance/collections/bulk", engines.Collections.Bulk, accRead)
	}
	route("GET /v1/tenant/finance/collections/{uuid}", h.GetCollection, accRead)
	route("POST /v1/tenant/finance/collections/{uuid}/allocations", h.AllocateCollection, accWrite)
	route("POST /v1/tenant/finance/collections/{uuid}/allocations/release", h.ReleaseAllocation, accWrite)
	route("POST /v1/tenant/finance/collections/{uuid}/refund", h.RefundCollection, accRead, refund)
	route("POST /v1/tenant/finance/collections/{uuid}/void", h.VoidCollection, accRead, adjust)

	// Cash / bank accounts and movements.
	route("GET /v1/tenant/finance/cash-accounts/meta", h.CashAccountsMeta, cashRead)
	route("GET /v1/tenant/finance/cash-accounts", h.ListCashAccounts, cashRead)
	route("POST /v1/tenant/finance/cash-accounts", h.CreateCashAccount, cashWrite)
	if engines.CashAccounts.Export != nil {
		route("POST /v1/tenant/finance/cash-accounts/export", engines.CashAccounts.Export, cashRead, cashExport)
	}
	if engines.CashAccounts.Import != nil {
		route("POST /v1/tenant/finance/cash-accounts/import", engines.CashAccounts.Import, cashWrite, cashImport)
	}
	if engines.CashAccounts.Sample != nil {
		route("GET /v1/tenant/finance/cash-accounts/import/sample", engines.CashAccounts.Sample, cashImport)
	}
	route("GET /v1/tenant/finance/cash-accounts/{uuid}", h.GetCashAccount, cashRead)
	route("PATCH /v1/tenant/finance/cash-accounts/{uuid}", h.PatchCashAccount, cashWrite)
	route("POST /v1/tenant/finance/cash-accounts/{uuid}/movements", h.RecordMovement, cashWrite)
	route("POST /v1/tenant/finance/transfers", h.Transfer, cashWrite)
	route("GET /v1/tenant/finance/movements/meta", h.MovementsMeta, cashRead)
	route("GET /v1/tenant/finance/movements", h.ListMovements, cashRead)
	if engines.Movements.Export != nil {
		route("POST /v1/tenant/finance/movements/export", engines.Movements.Export, cashRead, cashExport)
	}
	if engines.Movements.Import != nil {
		route("POST /v1/tenant/finance/movements/import", engines.Movements.Import, cashWrite, cashImport)
	}
	if engines.Movements.Sample != nil {
		route("GET /v1/tenant/finance/movements/import/sample", engines.Movements.Sample, cashImport)
	}
	if engines.Movements.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/finance/movements/bulk", engines.Movements.Bulk, cashRead)
	}
	route("GET /v1/tenant/finance/movements/{uuid}", h.GetMovement, cashRead)
	route("POST /v1/tenant/finance/movements/{uuid}/reverse", h.ReverseMovement, cashWrite)

	// Expense categories and expenses.
	route("GET /v1/tenant/finance/expense-categories", h.ListCategories, expRead)
	route("POST /v1/tenant/finance/expense-categories", h.CreateCategory, expWrite)
	route("PATCH /v1/tenant/finance/expense-categories/{uuid}", h.PatchCategory, expWrite)
	route("GET /v1/tenant/finance/expenses/meta", h.ExpensesMeta, expRead)
	route("GET /v1/tenant/finance/expenses", h.ListExpenses, expRead)
	route("POST /v1/tenant/finance/expenses", h.CreateExpense, expWrite)
	if engines.Expenses.Export != nil {
		route("POST /v1/tenant/finance/expenses/export", engines.Expenses.Export, expRead, expExport)
	}
	if engines.Expenses.Import != nil {
		route("POST /v1/tenant/finance/expenses/import", engines.Expenses.Import, expWrite, expImport)
	}
	if engines.Expenses.Sample != nil {
		route("GET /v1/tenant/finance/expenses/import/sample", engines.Expenses.Sample, expImport)
	}
	if engines.Expenses.Bulk != nil {
		route("POST /v1/tenant/finance/expenses/bulk", engines.Expenses.Bulk, expRead)
	}
	route("GET /v1/tenant/finance/expenses/{uuid}", h.GetExpense, expRead)
	route("PATCH /v1/tenant/finance/expenses/{uuid}", h.PatchExpense, expWrite)
	route("POST /v1/tenant/finance/expenses/{uuid}/void", h.VoidExpense, expWrite)
	route("PUT /v1/tenant/finance/expenses/{uuid}/file", h.UploadAttachment, expWrite)
	route("GET /v1/tenant/finance/expenses/{uuid}/file", h.DownloadAttachment, expRead)
	route("DELETE /v1/tenant/finance/expenses/{uuid}/file", h.DeleteAttachment, expWrite)

	// Supplier invoices (expense documents: tenant.finance.expenses.*) and
	// supplier payments (money out: tenant.finance.cash.*), TEC-721.
	route("GET /v1/tenant/finance/payables/summary", h.PayablesSummary, expRead)
	route("GET /v1/tenant/finance/supplier-invoices/meta", h.SupplierInvoicesMeta, expRead)
	route("GET /v1/tenant/finance/supplier-invoices", h.ListSupplierInvoices, expRead)
	route("POST /v1/tenant/finance/supplier-invoices", h.CreateSupplierInvoice, expWrite)
	if engines.SupplierInvoices.Export != nil {
		route("POST /v1/tenant/finance/supplier-invoices/export", engines.SupplierInvoices.Export, expRead, expExport)
	}
	route("GET /v1/tenant/finance/supplier-invoices/{uuid}", h.GetSupplierInvoice, expRead)
	route("PATCH /v1/tenant/finance/supplier-invoices/{uuid}", h.PatchSupplierInvoice, expWrite)
	route("POST /v1/tenant/finance/supplier-invoices/{uuid}/void", h.VoidSupplierInvoice, expWrite)
	route("PUT /v1/tenant/finance/supplier-invoices/{uuid}/file", h.UploadInvoiceAttachment, expWrite)
	route("GET /v1/tenant/finance/supplier-invoices/{uuid}/file", h.DownloadInvoiceAttachment, expRead)
	route("DELETE /v1/tenant/finance/supplier-invoices/{uuid}/file", h.DeleteInvoiceAttachment, expWrite)
	route("GET /v1/tenant/finance/supplier-payments/meta", h.SupplierPaymentsMeta, cashRead)
	route("GET /v1/tenant/finance/supplier-payments", h.ListSupplierPayments, cashRead)
	route("POST /v1/tenant/finance/supplier-payments", h.CreateSupplierPayment, cashWrite)
	if engines.SupplierPayments.Export != nil {
		route("POST /v1/tenant/finance/supplier-payments/export", engines.SupplierPayments.Export, cashRead, cashExport)
	}
	route("GET /v1/tenant/finance/supplier-payments/{uuid}", h.GetSupplierPayment, cashRead)
	route("POST /v1/tenant/finance/supplier-payments/{uuid}/void", h.VoidSupplierPayment, cashWrite)

	// Reports.
	route("GET /v1/tenant/finance/reports/expenses", h.ExpenseReport, reports)
	route("GET /v1/tenant/finance/reports/cash-flow", h.CashFlowReport, reports)
}

// RegisterLiveResources declares the finance list channels
// (org:{org_uuid}:resource:<name>) and their read permissions.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceAccounts, rbac.PermTenantFinanceAccountsRead)
	reg.Register(model.ResourceEntries, rbac.PermTenantFinanceAccountsRead)
	reg.Register(model.ResourceCashAccounts, rbac.PermTenantFinanceCashRead)
	reg.Register(model.ResourceMovements, rbac.PermTenantFinanceCashRead)
	reg.Register(model.ResourceExpenses, rbac.PermTenantFinanceExpensesRead)
	reg.Register(model.ResourceCategories, rbac.PermTenantFinanceExpensesRead)
	reg.Register(model.ResourceSupplierInvoices, rbac.PermTenantFinanceExpensesRead)
	reg.Register(model.ResourceSupplierPayments, rbac.PermTenantFinanceCashRead)
	reg.Register(model.ResourceCollections, rbac.PermTenantFinanceAccountsRead)
}

// Export / bulk query keys per resource.
var (
	// Supplier invoices / payments (TEC-721).
	SupplierInvoiceQueryKeys = []string{"q", "supplier", "status", "due_state", "category", "branch", "from", "to", "due_to", "window"}
	SupplierPaymentQueryKeys = []string{"q", "supplier", "cash_account", "status", "method", "invoice", "from", "to"}
	AccountQueryKeys         = []string{"q", "balance_status", "status", "party_kind", "branch", "due_before", "min_balance", "max_balance"}
	EntryQueryKeys           = []string{"q", "account", "rental", "kind", "purpose", "direction", "branch", "from", "to"}
	CashQueryKeys            = []string{"q", "kind", "status", "branch", "head_office"}
	MovementQueryKeys        = []string{"q", "account", "kind", "direction", "category", "branch", "from", "to"}
	ExpenseQueryKeys         = []string{"q", "category", "branch", "vehicle", "supplier", "status", "from", "to"}
	CollectionQueryKeys      = []string{
		"q", "method", "status", "allocation_status", "branch", "cash_account", "account", "rental", "from", "to",
	}
)
