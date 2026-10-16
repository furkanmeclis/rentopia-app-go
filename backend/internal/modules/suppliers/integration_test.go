package suppliers_test

// Database tests of TEC-721 (TEST_DATABASE_URL, skipped otherwise): the
// payable accrual of an invoice (once; void reverses it), partial and
// multi-invoice payments, the due-soon figures, the supplier of a vehicle,
// VKN / TCKN validation, the free-text migration, branch scope and
// cross-organization isolation.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/dbtest"
	finmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	finusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/usecase"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixed clock: 2026-05-10 12:00 Istanbul (09:00 UTC).
var clock = time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)

type org struct {
	id       int64
	uuid     uuid.UUID
	slug     string
	branchA  ref
	branchB  ref
	bankUUID uuid.UUID // head office bank account
	tillA    uuid.UUID // till of branch A (opening 500)
	category uuid.UUID
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

type env struct {
	t        *testing.T
	pool     *pgxpool.Pool
	q        *db.Queries
	fin      *finusecase.Service
	sup      *usecase.Service
	vehicles *vehicleusecase.Service
}

var perms = []string{
	rbac.PermTenantSuppliersRead, rbac.PermTenantSuppliersWrite, rbac.PermTenantFinanceAccountsRead,
	rbac.PermTenantFinanceAccountsWrite, rbac.PermTenantFinanceLedgerAdjust, rbac.PermTenantFinanceCashRead,
	rbac.PermTenantFinanceCashWrite, rbac.PermTenantFinanceExpensesRead, rbac.PermTenantFinanceExpensesWrite,
	rbac.PermTenantVehiclesRead, rbac.PermTenantVehiclesWrite,
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	fin := finusecase.New(pool, q, nil)
	fin.SetClock(func() time.Time { return clock })
	sup := usecase.New(pool, q, fin, nil)
	sup.SetClock(func() time.Time { return clock })
	veh := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	return &env{t: t, pool: pool, q: q, fin: fin, sup: sup, vehicles: veh}
}

func (e *env) must(err error) {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) newOrg() *org {
	e.t.Helper()
	ctx := context.Background()
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	o, err := e.q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "sup-" + suffix, Name: "Sup " + suffix})
	e.must(err)
	e.t.Cleanup(func() {
		// Ledger rows are append-only: only the organization cascade
		// removes them.
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", o.ID)
	})
	out := &org{id: o.ID, uuid: o.Uuid, slug: o.Slug}
	branch := func(code string) ref {
		b, err := e.q.CreateBranch(ctx, db.CreateBranchParams{
			OrganizationID: o.ID, Code: code, Name: code, WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
			PrepMinutes: 30, IsActive: true,
		})
		e.must(err)
		return ref{b.ID, b.Uuid}
	}
	out.branchA, out.branchB = branch("A"), branch("B")
	all := e.ctx(out, nil)
	bank, err := e.fin.CreateCashAccount(all, o.ID, finmodel.CashAccountInput{
		Code: strp("BANKA"), Name: strp("Banka"), Kind: strp(finmodel.CashKindBank), OpeningBalance: mp("100000"),
		OpeningDate: strp("2026-01-01"),
	})
	e.must(err)
	out.bankUUID = bank.UUID
	till, err := e.fin.CreateCashAccount(all, o.ID, finmodel.CashAccountInput{
		Code: strp("KASA-A"), Name: strp("Kasa A"), Kind: strp(finmodel.CashKindCash), OpeningBalance: mp("500"),
		OpeningDate: strp("2026-01-01"), BranchUUID: &out.branchA.uuid,
	})
	e.must(err)
	out.tillA = till.UUID
	cats, err := e.fin.ListCategories(all, o.ID, "", true)
	e.must(err)
	for _, c := range cats {
		if c.Code == "repair" {
			out.category = c.UUID
		}
	}
	return out
}

func (e *env) ctx(o *org, branches []int64) context.Context {
	return orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: o.id, UUID: o.uuid, Slug: o.slug, Status: orgctx.StatusActive, Permissions: perms, BranchIDs: branches,
	})
}

func strp(v string) *string { return &v }

func mp(v string) *money.Money {
	m := money.MustParse(v)
	return &m
}

func (e *env) supplier(o *org, title string) model.Supplier {
	e.t.Helper()
	s, err := e.sup.Create(e.ctx(o, nil), o.id, model.Input{Title: strp(title), SupplierType: strp(model.TypeService), PaymentTermDays: i32p(30)})
	e.must(err)
	return s
}

func i32p(v int32) *int32 { return &v }

func (e *env) invoice(o *org, sup uuid.UUID, no, date, due, net string, branch *uuid.UUID) finmodel.SupplierInvoice {
	e.t.Helper()
	rate := money.Percent(20)
	in := finmodel.SupplierInvoiceInput{
		SupplierUUID: &sup, InvoiceNo: strp(no), InvoiceDate: strp(date), CategoryUUID: &o.category, NetAmount: mp(net),
		VATRate: &rate, BranchUUID: branch,
	}
	if due != "" {
		in.DueDate = strp(due)
	}
	inv, err := e.fin.CreateSupplierInvoice(e.ctx(o, nil), o.id, in)
	e.must(err)
	return inv
}

type entryRow struct {
	kind, direction, amount, reason string
	reverses                        bool
}

func (e *env) invoiceEntries(o *org, invoice uuid.UUID) []entryRow {
	e.t.Helper()
	rows, err := e.pool.Query(context.Background(), `
		SELECT le.kind, le.direction, le.amount::text, le.reason, le.reverses_entry_id IS NOT NULL
		FROM ledger_entries le JOIN supplier_invoices i ON i.organization_id = le.organization_id AND i.id = le.supplier_invoice_id
		WHERE le.organization_id = $1 AND i.uuid = $2 ORDER BY le.id`, o.id, invoice)
	e.must(err)
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (entryRow, error) {
		var x entryRow
		err := r.Scan(&x.kind, &x.direction, &x.amount, &x.reason, &x.reverses)
		return x, err
	})
	e.must(err)
	return out
}

func (e *env) account(o *org, s model.Supplier) finmodel.Account {
	e.t.Helper()
	got, err := e.sup.Get(e.ctx(o, nil), o.id, s.UUID)
	e.must(err)
	if got.Account == nil {
		e.t.Fatal("supplier has no ledger account")
	}
	acc, err := e.fin.GetAccount(e.ctx(o, nil), o.id, got.Account.UUID)
	e.must(err)
	return acc
}

func expectCode(t *testing.T, err error, field, code string) {
	t.Helper()
	var fve *finusecase.ValidationError
	var sve *usecase.ValidationError
	switch {
	case errors.As(err, &fve):
		for _, d := range fve.Details {
			if d.Field == field && d.Code == code {
				return
			}
		}
	case errors.As(err, &sve):
		for _, d := range sve.Details {
			if d.Field == field && d.Code == code {
				return
			}
		}
	}
	t.Fatalf("want %s/%s, got %v", field, code, err)
}

// ---------------------------------------------------------------- invoices

func TestInvoicePostsPayableOnceAndVoidReverses(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	ctx := e.ctx(o, nil)
	s := e.supplier(o, "Usta Servis Ltd.")
	inv := e.invoice(o, s.UUID, "FT-001", "2026-05-01", "", "1000", nil)
	if inv.TotalAmount.String() != "1200.00" || inv.VATAmount.String() != "200.00" || inv.Status != finmodel.InvoiceOpen {
		t.Fatalf("invoice totals: %+v", inv)
	}
	// Default due date: invoice date + the supplier's 30 days.
	if inv.DueDate != "2026-05-31" {
		t.Fatalf("due date %s", inv.DueDate)
	}
	if inv.ExpenseUUID == nil {
		t.Fatal("the invoice has no expense row")
	}

	// Posting is idempotent: re-running the reconciliation adds nothing.
	for i := 0; i < 3; i++ {
		e.must(e.fin.SyncSupplierInvoice(ctx, o.id, inv.UUID))
	}
	entries := e.invoiceEntries(o, inv.UUID)
	if len(entries) != 1 || entries[0].kind != "accrual" || entries[0].direction != "credit" || entries[0].amount != "1200.00" {
		t.Fatalf("entries after posting: %+v", entries)
	}
	// The database refuses a second live accrual of the same key (RT-A23).
	var accountID, invoiceID int64
	e.must(e.pool.QueryRow(context.Background(), `SELECT account_id, id FROM supplier_invoices WHERE organization_id = $1 AND uuid = $2`,
		o.id, inv.UUID).Scan(&accountID, &invoiceID))
	_, err := e.pool.Exec(context.Background(), `INSERT INTO ledger_entries (organization_id, account_id, kind, direction, amount,
		entry_date, due_date, source_type, accrual_key, supplier_invoice_id)
		VALUES ($1, $2, 'accrual', 'credit', 1200, '2026-05-01', '2026-05-31', 'supplier_invoice', $3, $4)`,
		o.id, accountID, finmodel.InvoiceKey(invoiceID), invoiceID)
	if err == nil || !strings.Contains(err.Error(), "already posted") {
		t.Fatalf("second accrual: %v", err)
	}
	acc := e.account(o, s)
	if acc.PartyKind != finmodel.PartySupplier || acc.Balance.Balance.String() != "-1200.00" || acc.Balance.Status != finmodel.BalancePayable {
		t.Fatalf("account after posting: %+v", acc.Balance)
	}

	// A moved due date posts the accrual again (reversal + new accrual).
	_, err = e.fin.PatchSupplierInvoice(ctx, o.id, inv.UUID, finmodel.SupplierInvoiceInput{DueDate: strp("2026-06-15")})
	e.must(err)
	entries = e.invoiceEntries(o, inv.UUID)
	if len(entries) != 3 || entries[1].reason != finmodel.ReasonDueChanged || entries[2].kind != "accrual" {
		t.Fatalf("entries after due change: %+v", entries)
	}
	// Amounts are fixed.
	_, err = e.fin.PatchSupplierInvoice(ctx, o.id, inv.UUID, finmodel.SupplierInvoiceInput{NetAmount: mp("10")})
	expectCode(t, err, "total_amount", "immutable")

	// Void reverses the live accrual and voids the expense row.
	voided, err := e.fin.VoidSupplierInvoice(ctx, o.id, inv.UUID, "yanlış fatura")
	e.must(err)
	if voided.Status != finmodel.InvoiceVoid || voided.OpenAmount.String() != "0.00" {
		t.Fatalf("voided invoice: %+v", voided)
	}
	entries = e.invoiceEntries(o, inv.UUID)
	last := entries[len(entries)-1]
	if len(entries) != 4 || last.kind != "reversal" || last.direction != "debit" || last.reason != finmodel.ReasonInvoiceVoided {
		t.Fatalf("entries after void: %+v", entries)
	}
	e.must(e.fin.SyncSupplierInvoice(ctx, o.id, inv.UUID))
	if n := len(e.invoiceEntries(o, inv.UUID)); n != 4 {
		t.Fatalf("void sync posted again: %d entries", n)
	}
	if acc := e.account(o, s); !acc.Balance.Balance.IsZero() {
		t.Fatalf("balance after void: %s", acc.Balance.Balance)
	}
	exp, err := e.fin.GetExpense(ctx, o.id, *inv.ExpenseUUID)
	e.must(err)
	if exp.Status != finmodel.ExpenseVoid || exp.SupplierInvoice == nil || exp.Supplier == nil {
		t.Fatalf("invoice expense: %+v", exp)
	}
	// The generated expense is managed by its invoice.
	if _, err := e.fin.VoidExpense(ctx, o.id, *inv.ExpenseUUID, "deneme"); err == nil {
		t.Fatal("voiding an invoice expense directly must fail")
	}
	// Ledger reversal of the accrual is refused (the invoice owns it).
	var accrualUUID uuid.UUID
	e.must(e.pool.QueryRow(context.Background(), `SELECT uuid FROM ledger_entries WHERE organization_id = $1 AND supplier_invoice_id = $2
		AND kind = 'accrual' ORDER BY id DESC LIMIT 1`, o.id, invoiceID).Scan(&accrualUUID))
	if _, err := e.fin.Reverse(ctx, o.id, accrualUUID, finmodel.ReverseInput{Reason: "deneme"}); err == nil {
		t.Fatal("reversing an invoice accrual directly must fail")
	}
	// The number is free again after a void; a live duplicate is refused.
	e.invoice(o, s.UUID, "FT-001", "2026-05-02", "", "100", nil)
	_, err = e.fin.CreateSupplierInvoice(ctx, o.id, finmodel.SupplierInvoiceInput{
		SupplierUUID: &s.UUID, InvoiceNo: strp("ft-001"), CategoryUUID: &o.category, TotalAmount: mp("50"),
	})
	if !errors.Is(err, finusecase.ErrInvoiceNoTaken) {
		t.Fatalf("duplicate number: %v", err)
	}
}

func TestInvoiceLinesVATBreakdown(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	s := e.supplier(o, "Parça A.Ş.")
	r20, r10 := money.Percent(20), money.Percent(10)
	inv, err := e.fin.CreateSupplierInvoice(e.ctx(o, nil), o.id, finmodel.SupplierInvoiceInput{
		SupplierUUID: &s.UUID, InvoiceNo: strp("P-1"), InvoiceDate: strp("2026-05-01"), DueDate: strp("2026-05-20"),
		CategoryUUID: &o.category, Lines: []finmodel.InvoiceLineInput{
			{Description: "Fren balatası", NetAmount: mp("1000"), VATRate: &r20},
			{Description: "İşçilik", NetAmount: mp("500"), VATRate: &r10},
		},
	})
	e.must(err)
	got, err := e.fin.GetSupplierInvoice(e.ctx(o, nil), o.id, inv.UUID)
	e.must(err)
	if got.NetAmount.String() != "1500.00" || got.VATAmount.String() != "250.00" || got.TotalAmount.String() != "1750.00" || len(got.Lines) != 2 {
		t.Fatalf("invoice: %+v", got)
	}
	if got.Lines[1].VATAmount.String() != "50.00" {
		t.Fatalf("line 2: %+v", got.Lines[1])
	}
	_, err = e.fin.CreateSupplierInvoice(e.ctx(o, nil), o.id, finmodel.SupplierInvoiceInput{
		SupplierUUID: &s.UUID, InvoiceNo: strp("P-2"), InvoiceDate: strp("2026-05-10"), DueDate: strp("2026-05-01"),
		CategoryUUID: &o.category, TotalAmount: mp("100"),
	})
	expectCode(t, err, "due_date", "before_invoice_date")
}

// ---------------------------------------------------------------- payments

func TestPartialAndMultiInvoicePayment(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	ctx := e.ctx(o, nil)
	s := e.supplier(o, "Filo Kiralama A.Ş.")
	a := e.invoice(o, s.UUID, "A-1", "2026-04-01", "2026-04-30", "1000", nil) // 1200, oldest
	b := e.invoice(o, s.UUID, "B-1", "2026-04-15", "2026-05-15", "500", nil)  // 600

	// Auto allocation: oldest due date first.
	p1, err := e.fin.CreateSupplierPayment(ctx, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.bankUUID, Amount: mp("1500"), PaymentDate: strp("2026-05-05"),
	})
	e.must(err)
	if len(p1.Allocations) != 2 || p1.AllocatedAmount.String() != "1500.00" || p1.MovementUUID == nil || p1.LedgerEntryUUID == nil {
		t.Fatalf("payment 1: %+v", p1)
	}
	ga, _ := e.fin.GetSupplierInvoice(ctx, o.id, a.UUID)
	gb, _ := e.fin.GetSupplierInvoice(ctx, o.id, b.UUID)
	if ga.Status != finmodel.InvoicePaid || gb.Status != finmodel.InvoicePartiallyPaid || gb.OpenAmount.String() != "300.00" {
		t.Fatalf("after payment 1: a=%s b=%s open=%s", ga.Status, gb.Status, gb.OpenAmount)
	}
	mv, err := e.fin.GetMovement(ctx, o.id, *p1.MovementUUID)
	e.must(err)
	if mv.Kind != finmodel.MoveSupplierPayment || mv.Direction != finmodel.DirOut || mv.Amount.String() != "1500.00" {
		t.Fatalf("movement: %+v", mv)
	}
	if acc := e.account(o, s); acc.Balance.Balance.String() != "-300.00" {
		t.Fatalf("balance after payment 1: %s", acc.Balance.Balance)
	}

	// Explicit allocation beyond the open amount is refused.
	_, err = e.fin.CreateSupplierPayment(ctx, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.bankUUID, Amount: mp("400"),
		Allocations: []finmodel.AllocationInput{{InvoiceUUID: &b.UUID, Amount: mp("400")}},
	})
	expectCode(t, err, "allocations.0.amount", "exceeds_open")
	// Allocations above the payment are refused.
	_, err = e.fin.CreateSupplierPayment(ctx, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.bankUUID, Amount: mp("100"),
		Allocations: []finmodel.AllocationInput{{InvoiceUUID: &b.UUID, Amount: mp("200")}},
	})
	expectCode(t, err, "allocations", "exceeds_amount")
	// A till cannot go negative.
	_, err = e.fin.CreateSupplierPayment(ctx, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.tillA, Amount: mp("600"),
	})
	expectCode(t, err, "amount", "insufficient_funds")

	// Explicit partial allocation, the rest is an advance.
	p2, err := e.fin.CreateSupplierPayment(ctx, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.bankUUID, Amount: mp("500"), Method: "cheque",
		Allocations: []finmodel.AllocationInput{{InvoiceUUID: &b.UUID, Amount: mp("300")}},
	})
	e.must(err)
	if p2.UnallocatedAmt.String() != "200.00" {
		t.Fatalf("payment 2 advance: %s", p2.UnallocatedAmt)
	}
	gb, _ = e.fin.GetSupplierInvoice(ctx, o.id, b.UUID)
	if gb.Status != finmodel.InvoicePaid || len(gb.Payments) != 2 {
		t.Fatalf("b after payment 2: %+v", gb)
	}
	if acc := e.account(o, s); acc.Balance.Balance.String() != "200.00" {
		t.Fatalf("balance after payment 2 (advance): %s", acc.Balance.Balance)
	}

	// An invoice with live payments cannot be voided.
	_, err = e.fin.VoidSupplierInvoice(ctx, o.id, a.UUID, "deneme")
	var se *finusecase.StateError
	if !errors.As(err, &se) || se.Code != "invoice_has_payments" {
		t.Fatalf("void paid invoice: %v", err)
	}
	// The payment's movement is reversed through the payment only.
	if _, err := e.fin.ReverseMovement(ctx, o.id, *p1.MovementUUID, finmodel.ReverseInput{Reason: "deneme"}); !errors.As(err, &se) {
		t.Fatalf("reverse payment movement: %v", err)
	}

	// Voiding payment 1 reverses entry + movement and reopens the invoices.
	v1, err := e.fin.VoidSupplierPayment(ctx, o.id, p1.UUID, "hatalı ödeme")
	e.must(err)
	if v1.Status != finmodel.SupplierPaymentVoid {
		t.Fatalf("void status %s", v1.Status)
	}
	ga, _ = e.fin.GetSupplierInvoice(ctx, o.id, a.UUID)
	gb, _ = e.fin.GetSupplierInvoice(ctx, o.id, b.UUID)
	if ga.Status != finmodel.InvoiceOpen || gb.Status != finmodel.InvoicePartiallyPaid || gb.PaidAmount.String() != "300.00" {
		t.Fatalf("after void: a=%s b=%s paid=%s", ga.Status, gb.Status, gb.PaidAmount)
	}
	if acc := e.account(o, s); acc.Balance.Balance.String() != "-1300.00" {
		t.Fatalf("balance after void: %s", acc.Balance.Balance)
	}
	bank, err := e.fin.GetCashAccount(ctx, o.id, o.bankUUID)
	e.must(err)
	if bank.Balance.String() != "99500.00" {
		t.Fatalf("bank balance: %s", bank.Balance)
	}
	if _, err := e.fin.VoidSupplierPayment(ctx, o.id, p1.UUID, "tekrar"); !errors.As(err, &se) {
		t.Fatalf("second void: %v", err)
	}

	// A manual supplier movement naming the supplier is a supplier payment.
	mv2, err := e.fin.RecordMovement(ctx, o.id, o.tillA, finmodel.MovementInput{
		Kind: finmodel.MoveSupplierPayment, Amount: mp("100"), SupplierUUID: &s.UUID,
	})
	e.must(err)
	if mv2.Kind != finmodel.MoveSupplierPayment {
		t.Fatalf("manual movement: %+v", mv2)
	}
	ga, _ = e.fin.GetSupplierInvoice(ctx, o.id, a.UUID)
	if ga.PaidAmount.String() != "100.00" {
		t.Fatalf("manual payment allocation: %s", ga.PaidAmount)
	}
}

// ------------------------------------------------------------- due soon

func TestDueSoonPayables(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	ctx := e.ctx(o, nil)
	s := e.supplier(o, "Sigorta A.Ş.")
	e.invoice(o, s.UUID, "OV-1", "2026-04-01", "2026-05-09", "100", nil) // overdue (120)
	e.invoice(o, s.UUID, "DS-1", "2026-05-01", "2026-05-13", "200", nil) // due in 3 days (240)
	e.invoice(o, s.UUID, "LT-1", "2026-05-01", "2026-06-30", "300", nil) // later (360)
	paid := e.invoice(o, s.UUID, "PD-1", "2026-04-01", "2026-05-01", "50", nil)
	_, err := e.fin.CreateSupplierPayment(ctx, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.bankUUID, Amount: mp("60"),
		Allocations: []finmodel.AllocationInput{{InvoiceUUID: &paid.UUID, Amount: mp("60")}},
	})
	e.must(err)

	sum, err := e.fin.PayablesSummary(ctx, o.id, 7)
	e.must(err)
	if sum.OverdueCount != 1 || sum.OverdueAmount.String() != "120.00" || sum.DueSoonCount != 1 ||
		sum.DueSoonAmount.String() != "240.00" || sum.OpenAmount.String() != "720.00" || sum.AttentionCount != 2 {
		t.Fatalf("summary: %+v", sum)
	}
	list, total, err := e.fin.ListSupplierInvoices(ctx, o.id, finmodel.SupplierInvoiceFilter{DueStates: []string{finmodel.DueOverdue, finmodel.DueSoon}, SortField: "due_date"})
	e.must(err)
	if total != 2 || list[0].InvoiceNo != "OV-1" || list[0].DueState != finmodel.DueOverdue || list[1].DueState != finmodel.DueSoon {
		t.Fatalf("due list: %d %+v", total, list)
	}
	// A wider window moves the later invoice in.
	wide, err := e.fin.PayablesSummary(ctx, o.id, 60)
	e.must(err)
	if wide.DueSoonCount != 2 {
		t.Fatalf("60-day window: %+v", wide)
	}
	got, err := e.sup.Get(ctx, o.id, s.UUID)
	e.must(err)
	if got.OverdueAmount.String() != "120.00" || got.DueSoonAmount.String() != "240.00" || got.DueState != "overdue" ||
		got.NextDueDate == nil || *got.NextDueDate != "2026-05-09" || got.Payable.String() != "720.00" {
		t.Fatalf("supplier figures: %+v", got)
	}
	// The ledger ages the supplier account's payables too.
	acc := e.account(o, s)
	if acc.Balance.Status != finmodel.BalanceOverdue || acc.Balance.Overdue.String() != "120.00" {
		t.Fatalf("account aging: %+v", acc.Balance)
	}
	fs, err := e.fin.Summary(ctx, o.id)
	e.must(err)
	if fs.PayableOverdue.String() != "120.00" || !fs.Overdue.IsZero() {
		t.Fatalf("finance summary: overdue=%s payable_overdue=%s", fs.Overdue, fs.PayableOverdue)
	}
	items, _, err := e.sup.List(ctx, o.id, model.Filter{DueStates: []string{"overdue"}})
	e.must(err)
	if len(items) != 1 || items[0].UUID != s.UUID {
		t.Fatalf("overdue supplier filter: %+v", items)
	}
}

// --------------------------------------------------------------- vehicles

// TestSupplierPaymentsStayOutOfCollections: supplier payments are not
// collection documents, and a supplier account takes no collections, so
// its invoices are never collection open items (TEC-718 / TEC-721).
func TestSupplierPaymentsStayOutOfCollections(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	ctx := orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: o.id, UUID: o.uuid, Slug: o.slug, Status: orgctx.StatusActive,
		Permissions: append([]string{rbac.PermTenantPaymentCollect}, perms...),
	})
	s := e.supplier(o, "Tahsilat Dışı Ltd.")
	e.invoice(o, s.UUID, "C-1", "2026-04-01", "2026-04-30", "1000", nil)
	p, err := e.fin.CreateSupplierPayment(ctx, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.bankUUID, Amount: mp("500"), PaymentDate: strp("2026-05-05"),
	})
	e.must(err)
	acc := e.account(o, s)

	list, total, err := e.fin.ListCollections(ctx, o.id, finmodel.CollectionFilter{Limit: 50})
	e.must(err)
	if total != 0 || len(list) != 0 {
		t.Fatalf("supplier payment listed as a collection: %d %+v", total, list)
	}
	byAccount, total, err := e.fin.ListCollections(ctx, o.id, finmodel.CollectionFilter{Limit: 50, AccountUUID: &acc.UUID})
	e.must(err)
	if total != 0 || len(byAccount) != 0 {
		t.Fatalf("supplier account has collections: %+v", byAccount)
	}
	_, err = e.fin.OpenItems(ctx, o.id, acc.UUID, nil, true)
	expectCode(t, err, "account_uuid", "supplier_account")
	_, _, err = e.fin.CreateCollection(ctx, o.id, finmodel.CollectionCreateInput{
		AccountUUID: &acc.UUID, Amount: mp("100"), Method: "bank_transfer", CashAccountUUID: &o.bankUUID,
	})
	expectCode(t, err, "account_uuid", "supplier_account")
	_, err = e.fin.Collect(ctx, o.id, acc.UUID, finmodel.CollectionInput{Amount: mp("100")})
	expectCode(t, err, "account_uuid", "supplier_account")

	// The payment entry stays a supplier payment entry.
	entry, err := e.fin.GetEntry(ctx, o.id, *p.LedgerEntryUUID)
	e.must(err)
	if entry.Kind != finmodel.KindPayment || entry.SourceType != finmodel.SourceSupplierPayment {
		t.Fatalf("payment entry: %s / %s", entry.Kind, entry.SourceType)
	}
}

func TestSupplierOnVehicle(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	other := e.newOrg()
	ctx := e.ctx(o, nil)
	s := e.supplier(o, "Araç Tedarik A.Ş.")
	foreign := e.supplier(other, "Başka Tedarik")
	supplied := vehiclemodel.OwnershipSupplied
	in := func(plate string, sup *uuid.UUID) vehiclemodel.VehicleInput {
		yes := true
		year := int16(2024)
		return vehiclemodel.VehicleInput{
			Plate: strp(plate), ManualEntry: &yes, ManualBrand: strp("Fiat"), ManualModel: strp("Egea"), ModelYear: &year,
			HomeBranchUUID: &o.branchA.uuid, Ownership: &supplied, SupplierUUID: vehiclemodel.OptionalUUID{Set: true, Value: sup},
		}
	}
	v, err := e.vehicles.Create(ctx, o.id, in("34 TED 1", &s.UUID))
	e.must(err)
	if v.Supplier == nil || v.Supplier.UUID != s.UUID || v.SupplierName != s.Title {
		t.Fatalf("vehicle supplier: %+v %q", v.Supplier, v.SupplierName)
	}
	// Another organization's supplier reads as unknown.
	_, err = e.vehicles.Create(ctx, o.id, in("34 TED 2", &foreign.UUID))
	if err == nil || !strings.Contains(err.Error(), "supplier_uuid") {
		t.Fatalf("foreign supplier: %v", err)
	}
	list, total, err := e.vehicles.List(ctx, o.id, vehiclemodel.ListFilter{SupplierUUIDs: []uuid.UUID{s.UUID}, Limit: 10})
	e.must(err)
	if total != 1 || list[0].UUID != v.UUID {
		t.Fatalf("vehicles of supplier: %d", total)
	}
	got, err := e.sup.Get(ctx, o.id, s.UUID)
	e.must(err)
	if got.VehicleCount != 1 {
		t.Fatalf("vehicle count: %d", got.VehicleCount)
	}
	// A renamed card renames the kept free-text name.
	_, err = e.sup.Patch(ctx, o.id, s.UUID, model.Input{Title: strp("Araç Tedarik Ltd.")})
	e.must(err)
	v2, err := e.vehicles.Get(ctx, o.id, v.UUID)
	e.must(err)
	if v2.SupplierName != "Araç Tedarik Ltd." || v2.Supplier == nil || v2.Supplier.Name != "Araç Tedarik Ltd." {
		t.Fatalf("renamed: %q %+v", v2.SupplierName, v2.Supplier)
	}
	// An owned vehicle has no supplier.
	owned := vehiclemodel.OwnershipOwned
	v3, err := e.vehicles.Patch(ctx, o.id, v.UUID, vehiclemodel.VehicleInput{Ownership: &owned})
	e.must(err)
	if v3.Supplier != nil || v3.SupplierName != "" {
		t.Fatalf("owned vehicle keeps supplier: %+v", v3.Supplier)
	}
	// A free-text name links the card with that title.
	v4, err := e.vehicles.Create(ctx, o.id, vehiclemodel.VehicleInput{
		Plate: strp("34 TED 3"), ManualEntry: boolp(true), ManualBrand: strp("Fiat"), ManualModel: strp("Egea"),
		ModelYear: i16p(2024), HomeBranchUUID: &o.branchA.uuid, Ownership: &supplied, SupplierName: strp("araç tedarik ltd. "),
	})
	e.must(err)
	if v4.Supplier == nil || v4.Supplier.UUID != s.UUID {
		t.Fatalf("free-text link: %+v", v4.Supplier)
	}
}

func boolp(v bool) *bool  { return &v }
func i16p(v int16) *int16 { return &v }

// ------------------------------------------------------------ validation

func TestTaxNumberAndCardValidation(t *testing.T) {
	cases := map[string]bool{
		"1234567890": true, "1234567891": false, // VKN checksum
		"10000000146": true, // valid TCKN
		"10000000147": false,
		"12345":       false,
	}
	for v, want := range cases {
		if got := usecase.ValidTaxNumber(v); got != want {
			t.Errorf("ValidTaxNumber(%s) = %v", v, got)
		}
	}
	if !usecase.ValidIBAN("TR330006100519786457841326") || usecase.ValidIBAN("TR330006100519786457841327") {
		t.Error("IBAN checksum")
	}

	e := newEnv(t)
	o := e.newOrg()
	ctx := e.ctx(o, nil)
	vkn := validVKN(t)
	_, err := e.sup.Create(ctx, o.id, model.Input{Title: strp("Hatalı"), TaxNumber: strp("1234567891")})
	expectCode(t, err, "tax_number", "invalid_checksum")
	_, err = e.sup.Create(ctx, o.id, model.Input{Title: strp("Hatalı"), TaxNumber: strp("12ab")})
	expectCode(t, err, "tax_number", "invalid")
	_, err = e.sup.Create(ctx, o.id, model.Input{Title: strp("Hatalı"), IBAN: strp("TR00 0000")})
	expectCode(t, err, "iban", "invalid")
	_, err = e.sup.Create(ctx, o.id, model.Input{Title: strp("Hatalı"), Contacts: &[]model.Contact{{Name: "", Email: "x"}}})
	expectCode(t, err, "contacts.0.name", "required")
	s, err := e.sup.Create(ctx, o.id, model.Input{
		Title: strp("  Doğru   Vergi Ltd. "), TaxNumber: strp(vkn), IBAN: strp("tr33 0006 1005 1978 6457 8413 26"),
		Phone: strp("0532 111 22 33"), Contacts: &[]model.Contact{{Name: "Ali Veli", Role: "Muhasebe", Email: "ALI@X.COM"}},
	})
	e.must(err)
	if s.Title != "Doğru Vergi Ltd." || s.TaxKind != model.TaxKindVKN || s.IBAN != "TR330006100519786457841326" ||
		s.Phone != "+905321112233" || s.Contacts[0].Email != "ali@x.com" || !strings.HasPrefix(s.Code, "TED-") {
		t.Fatalf("normalized: %+v", s)
	}
	// The tax number is unique among active suppliers.
	_, err = e.sup.Create(ctx, o.id, model.Input{Title: strp("Kopya"), TaxNumber: strp(vkn)})
	if !errors.Is(err, usecase.ErrTaxNumberTaken) {
		t.Fatalf("duplicate tax number: %v", err)
	}
	// Codes are unique per organization (custom codes upper-cased).
	_, err = e.sup.Create(ctx, o.id, model.Input{Title: strp("Kod"), Code: strp(strings.ToLower(s.Code))})
	if !errors.Is(err, usecase.ErrCodeTaken) {
		t.Fatalf("duplicate code: %v", err)
	}
	// Archive (bulk) and back.
	prev, err := e.sup.SetStatus(ctx, o.id, s.UUID, model.StatusArchived)
	e.must(err)
	if prev != model.StatusActive {
		t.Fatalf("previous status %s", prev)
	}
	// An archived supplier takes no new invoices.
	_, err = e.fin.CreateSupplierInvoice(ctx, o.id, finmodel.SupplierInvoiceInput{
		SupplierUUID: &s.UUID, InvoiceNo: strp("X"), CategoryUUID: &o.category, TotalAmount: mp("10"),
	})
	expectCode(t, err, "supplier_uuid", "archived")
}

// validVKN computes a 10-digit VKN with a valid GİB checksum.
func validVKN(t *testing.T) string {
	t.Helper()
	for n := 123456780; n < 123456900; n++ {
		base := []byte(strings.Repeat("0", 9))
		s := []byte(itoa9(n))
		copy(base, s)
		for d := byte('0'); d <= '9'; d++ {
			v := string(base) + string(d)
			if usecase.ValidTaxNumber(v) {
				return v
			}
		}
	}
	t.Fatal("no VKN found")
	return ""
}

func itoa9(n int) string {
	out := make([]byte, 9)
	for i := 8; i >= 0; i-- {
		out[i] = byte('0' + n%10)
		n /= 10
	}
	return string(out)
}

// -------------------------------------------------------- free-text links

func TestFreeTextSupplierMigration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	fresh := dbtest.FreshDatabase(t, dsn)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql[:min(len(sql), 60)], err)
		}
	}
	// Back to the state before 000105 (106 only seeds permissions).
	exec(migration(t, "000106_seed_suppliers_rbac.down.sql"))
	exec(migration(t, "000105_suppliers.down.sql"))
	var orgID, branchID, catID int64
	if err := conn.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ('mig-sup', 'Mig') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `INSERT INTO branches (organization_id, code, name, working_hours, holiday_exceptions)
		VALUES ($1, 'MRK', 'Merkez', '{}', '[]') RETURNING id`, orgID).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `INSERT INTO expense_categories (organization_id, code, name) VALUES ($1, 'other', 'Diğer') RETURNING id`,
		orgID).Scan(&catID); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"Acme Filo", " acme filo ", "", "Owned"} {
		ownership := "supplied"
		if name == "Owned" || name == "" {
			ownership = "owned"
		}
		sup := name
		if ownership == "owned" {
			sup = ""
		}
		exec(`INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year, home_branch_id,
			current_branch_id, ownership, supplier_name) VALUES ($1, $2, TRUE, 'Fiat', 'Egea', 2024, $3, $3, $4, $5)`,
			orgID, "34 MIG "+itoa9(i)[6:], branchID, ownership, sup)
	}
	exec(`INSERT INTO expenses (organization_id, category_id, expense_date, supplier_name, net_amount, total_amount)
		VALUES ($1, $2, '2026-01-01', 'ACME FILO', 100, 100), ($1, $2, '2026-01-02', 'Opet', 50, 50), ($1, $2, '2026-01-03', '', 10, 10)`,
		orgID, catID)
	exec(migration(t, "000105_suppliers.up.sql"))
	exec(migration(t, "000106_seed_suppliers_rbac.up.sql"))

	type sup struct {
		code, title, kind  string
		vehicles, expenses int
	}
	rows, err := conn.Query(ctx, `SELECT s.code, s.title, s.supplier_type,
		(SELECT count(*) FROM vehicles v WHERE v.organization_id = s.organization_id AND v.supplier_id = s.id)::int,
		(SELECT count(*) FROM expenses x WHERE x.organization_id = s.organization_id AND x.supplier_id = s.id)::int
		FROM suppliers s WHERE s.organization_id = $1 ORDER BY s.code`, orgID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (sup, error) {
		var x sup
		err := r.Scan(&x.code, &x.title, &x.kind, &x.vehicles, &x.expenses)
		return x, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("suppliers: %+v", got)
	}
	if got[0].code != "TED-000001" || !strings.EqualFold(got[0].title, "acme filo") || got[0].kind != "vehicle_supplier" ||
		got[0].vehicles != 2 || got[0].expenses != 1 {
		t.Fatalf("acme: %+v", got[0])
	}
	if got[1].title != "Opet" || got[1].kind != "other" || got[1].vehicles != 0 || got[1].expenses != 1 {
		t.Fatalf("opet: %+v", got[1])
	}
	var unlinked int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM vehicles WHERE organization_id = $1 AND supplier_id IS NULL`, orgID).Scan(&unlinked); err != nil {
		t.Fatal(err)
	}
	if unlinked != 2 {
		t.Fatalf("owned vehicles linked: %d unlinked", unlinked)
	}
}

func migration(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestNewCardAdoptsFreeTextRows(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	ctx := e.ctx(o, nil)
	exp, err := e.fin.CreateExpense(ctx, o.id, finmodel.ExpenseInput{
		CategoryUUID: &o.category, SupplierName: strp("Shell"), TotalAmount: mp("300"),
	})
	e.must(err)
	if exp.Supplier != nil {
		t.Fatal("no card yet")
	}
	s := e.supplier(o, "shell")
	got, err := e.fin.GetExpense(ctx, o.id, exp.UUID)
	e.must(err)
	if got.Supplier == nil || got.Supplier.UUID != s.UUID {
		t.Fatalf("adopted: %+v", got.Supplier)
	}
	// A later free-text expense links the card directly; supplier_uuid
	// writes the card's title.
	exp2, err := e.fin.CreateExpense(ctx, o.id, finmodel.ExpenseInput{CategoryUUID: &o.category, SupplierName: strp("SHELL"), TotalAmount: mp("10")})
	e.must(err)
	exp3, err := e.fin.CreateExpense(ctx, o.id, finmodel.ExpenseInput{CategoryUUID: &o.category, SupplierUUID: &s.UUID, TotalAmount: mp("10")})
	e.must(err)
	if exp2.Supplier == nil || exp3.Supplier == nil || exp3.SupplierName != "shell" {
		t.Fatalf("links: %+v %+v %q", exp2.Supplier, exp3.Supplier, exp3.SupplierName)
	}
	items, _, err := e.fin.ListExpenses(ctx, o.id, finmodel.ExpenseFilter{SupplierUUIDs: []uuid.UUID{s.UUID}})
	e.must(err)
	if len(items) != 3 {
		t.Fatalf("expenses of supplier: %d", len(items))
	}
}

// ------------------------------------------------------- scope / isolation

func TestBranchScopeOfPayables(t *testing.T) {
	e := newEnv(t)
	o := e.newOrg()
	all := e.ctx(o, nil)
	onlyA := e.ctx(o, []int64{o.branchA.id})
	s := e.supplier(o, "Şube Tedarik")
	invA := e.invoice(o, s.UUID, "A-1", "2026-05-01", "2026-05-12", "100", &o.branchA.uuid)
	invB := e.invoice(o, s.UUID, "B-1", "2026-05-01", "2026-05-08", "100", &o.branchB.uuid)

	list, total, err := e.fin.ListSupplierInvoices(onlyA, o.id, finmodel.SupplierInvoiceFilter{})
	e.must(err)
	if total != 1 || list[0].UUID != invA.UUID {
		t.Fatalf("branch A list: %d", total)
	}
	if _, err := e.fin.GetSupplierInvoice(onlyA, o.id, invB.UUID); !errors.Is(err, finusecase.ErrSupplierInvoiceNotFound) {
		t.Fatalf("branch B invoice for A member: %v", err)
	}
	sum, err := e.fin.PayablesSummary(onlyA, o.id, 7)
	e.must(err)
	if sum.OverdueCount != 0 || sum.DueSoonCount != 1 {
		t.Fatalf("branch A summary: %+v", sum)
	}
	// A branch-A member cannot allocate a branch-B invoice, nor pay from
	// the head office bank.
	_, err = e.fin.CreateSupplierPayment(onlyA, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.tillA, Amount: mp("50"),
		Allocations: []finmodel.AllocationInput{{InvoiceUUID: &invB.UUID, Amount: mp("50")}},
	})
	expectCode(t, err, "allocations.0.invoice_uuid", "not_found")
	_, err = e.fin.CreateSupplierPayment(onlyA, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.bankUUID, Amount: mp("50"),
	})
	expectCode(t, err, "cash_account_uuid", "not_found")
	// Auto allocation from branch A only touches branch A's invoices.
	p, err := e.fin.CreateSupplierPayment(onlyA, o.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &s.UUID, CashAccountUUID: &o.tillA, Amount: mp("200"),
	})
	e.must(err)
	if len(p.Allocations) != 1 || p.Allocations[0].InvoiceUUID != invA.UUID {
		t.Fatalf("scoped auto allocation: %+v", p.Allocations)
	}
	// A branch-A member cannot create a head office invoice.
	_, err = e.fin.CreateSupplierInvoice(onlyA, o.id, finmodel.SupplierInvoiceInput{
		SupplierUUID: &s.UUID, InvoiceNo: strp("HO-1"), CategoryUUID: &o.category, TotalAmount: mp("10"),
	})
	expectCode(t, err, "branch_uuid", "required")
	// Payments list of a member shows its branches' payments only.
	if _, total, err := e.fin.ListSupplierPayments(all, o.id, finmodel.SupplierPaymentFilter{}); err != nil || total != 1 {
		t.Fatalf("all payments: %d %v", total, err)
	}
	onlyB := e.ctx(o, []int64{o.branchB.id})
	if _, total, err := e.fin.ListSupplierPayments(onlyB, o.id, finmodel.SupplierPaymentFilter{}); err != nil || total != 0 {
		t.Fatalf("branch B payments: %d %v", total, err)
	}
}

func TestCrossOrganization(t *testing.T) {
	e := newEnv(t)
	a, b := e.newOrg(), e.newOrg()
	ctxA, ctxB := e.ctx(a, nil), e.ctx(b, nil)
	sa := e.supplier(a, "A Tedarik")
	inv := e.invoice(a, sa.UUID, "X-1", "2026-05-01", "", "100", nil)
	pay, err := e.fin.CreateSupplierPayment(ctxA, a.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &sa.UUID, CashAccountUUID: &a.bankUUID, Amount: mp("50"),
	})
	e.must(err)

	if _, err := e.sup.Get(ctxB, b.id, sa.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("supplier of A from B: %v", err)
	}
	if _, err := e.sup.Patch(ctxB, b.id, sa.UUID, model.Input{Title: strp("x")}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("patch A's supplier from B: %v", err)
	}
	if _, err := e.sup.SetStatus(ctxB, b.id, sa.UUID, model.StatusArchived); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("archive A's supplier from B: %v", err)
	}
	if _, err := e.fin.GetSupplierInvoice(ctxB, b.id, inv.UUID); !errors.Is(err, finusecase.ErrSupplierInvoiceNotFound) {
		t.Fatalf("invoice of A from B: %v", err)
	}
	if _, err := e.fin.VoidSupplierInvoice(ctxB, b.id, inv.UUID, "deneme"); !errors.Is(err, finusecase.ErrSupplierInvoiceNotFound) {
		t.Fatalf("void A's invoice from B: %v", err)
	}
	if _, err := e.fin.GetSupplierPayment(ctxB, b.id, pay.UUID); !errors.Is(err, finusecase.ErrSupplierPaymentNotFound) {
		t.Fatalf("payment of A from B: %v", err)
	}
	if _, err := e.fin.VoidSupplierPayment(ctxB, b.id, pay.UUID, "deneme"); !errors.Is(err, finusecase.ErrSupplierPaymentNotFound) {
		t.Fatalf("void A's payment from B: %v", err)
	}
	// A's uuids in B's bodies are unknown, never A's rows.
	_, err = e.fin.CreateSupplierInvoice(ctxB, b.id, finmodel.SupplierInvoiceInput{
		SupplierUUID: &sa.UUID, InvoiceNo: strp("Y"), CategoryUUID: &b.category, TotalAmount: mp("10"),
	})
	expectCode(t, err, "supplier_uuid", "not_found")
	sb := e.supplier(b, "B Tedarik")
	_, err = e.fin.CreateSupplierPayment(ctxB, b.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &sb.UUID, CashAccountUUID: &a.bankUUID, Amount: mp("10"),
	})
	expectCode(t, err, "cash_account_uuid", "not_found")
	_, err = e.fin.CreateSupplierPayment(ctxB, b.id, finmodel.SupplierPaymentInput{
		SupplierUUID: &sb.UUID, CashAccountUUID: &b.bankUUID, Amount: mp("10"),
		Allocations: []finmodel.AllocationInput{{InvoiceUUID: &inv.UUID, Amount: mp("10")}},
	})
	expectCode(t, err, "allocations.0.invoice_uuid", "not_found")
	if _, _, err := e.fin.ListSupplierInvoices(ctxB, b.id, finmodel.SupplierInvoiceFilter{SupplierUUIDs: []uuid.UUID{sa.UUID}}); err == nil {
		t.Fatal("filtering B's invoices by A's supplier must fail")
	}
	items, total, err := e.sup.List(ctxB, b.id, model.Filter{})
	e.must(err)
	if total != 1 || items[0].UUID != sb.UUID {
		t.Fatalf("B's suppliers: %d", total)
	}
	// The composite FK keeps a row from pointing at another organization's
	// supplier even below the API.
	var supA, catB int64
	e.must(e.pool.QueryRow(context.Background(), `SELECT id FROM suppliers WHERE organization_id = $1 AND uuid = $2`, a.id, sa.UUID).Scan(&supA))
	e.must(e.pool.QueryRow(context.Background(), `SELECT id FROM expense_categories WHERE organization_id = $1 AND uuid = $2`, b.id, b.category).Scan(&catB))
	_, err = e.pool.Exec(context.Background(), `INSERT INTO expenses (organization_id, category_id, expense_date, net_amount, total_amount, supplier_id)
		VALUES ($1, $2, '2026-05-01', 10, 10, $3)`, b.id, catB, supA)
	if err == nil || !strings.Contains(err.Error(), "fk_expenses_supplier") {
		t.Fatalf("composite FK: %v", err)
	}
}
