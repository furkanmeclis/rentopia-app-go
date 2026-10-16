package finance_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Security deposits (TEC-719, docs/design/deposits.md).

var depositPerms = []string{
	rbac.PermTenantFinanceDepositsRead, rbac.PermTenantFinanceDepositsWrite, rbac.PermTenantFinanceDepositsDeduct,
	rbac.PermTenantFinanceDepositsExport,
}

func (f *fx) depositCtx(branches []int64, extra ...string) context.Context {
	perms := append(append([]string{}, allPerms...), depositPerms...)
	return f.scoped(branches, append(perms, extra...)...)
}

// depositFx is a finance fixture whose product asks for a 2 000 deposit,
// with a till of the main branch and the online pre-auth wiring.
func newDepositFx(t *testing.T) (*fx, context.Context, model.CashAccount) {
	t.Helper()
	f := newFx(t)
	ctx := f.depositCtx(nil)
	f.ctx = ctx
	till, err := f.fin.CreateCashAccount(ctx, f.org, model.CashAccountInput{Code: sp("KASA"), Name: sp("Merkez kasa"), Kind: sp("cash"),
		BranchUUID: &f.branch.uuid})
	f.must(err)
	adapter := iyzico.New(iyzico.Config{SandboxBaseURL: f.fake.URL(), LiveBaseURL: "http://127.0.0.1:1"})
	f.fin.SetPreAuth(f.accounts, "https://app.test", adapter)
	return f, ctx, till
}

func (f *fx) tillBalance(id uuid.UUID) money.Money {
	f.t.Helper()
	c, err := f.fin.GetCashAccount(f.ctx, f.org, id)
	f.must(err)
	return c.Balance
}

func (f *fx) depositEntries(dep uuid.UUID) map[string]int {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT e.kind || ':' || e.purpose FROM ledger_entries e
		JOIN security_deposits d ON d.organization_id = e.organization_id AND d.id = e.security_deposit_id
		WHERE e.organization_id = $1 AND d.uuid = $2`, f.org, dep)
	f.must(err)
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		f.must(rows.Scan(&k))
		out[k]++
	}
	return out
}

// TestDepositCashHeldAsLiability: a cash deposit is a liability: the till
// holds the money (deposit_in, not a collection), the customer still owes
// the full rent, and nothing reaches revenue or collections.
func TestDepositCashHeldAsLiability(t *testing.T) {
	f, ctx, till := newDepositFx(t)
	r, res := f.rental(f.branch)
	r = f.handover(r)
	rent := f.total(res.UUID)
	before := f.account()
	if !before.Balance.Balance.Equal(rent) {
		t.Fatalf("balance after the handover %s, want the rent %s", before.Balance.Balance, rent)
	}
	card, err := f.fin.DepositsForRental(ctx, f.org, r.UUID)
	f.must(err)
	if !card.DefaultAmount.Equal(money.MustParse("2000")) || !card.CanTake || !card.OnlineEnabled {
		t.Fatalf("deposit card before: %+v", card)
	}
	if _, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositCash}); err == nil {
		t.Fatal("a cash deposit without a till was accepted")
	}
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID})
	f.must(err)
	if dep.Status != model.DepositHeld || !dep.Amount.Equal(money.MustParse("2000")) || dep.HeldAt == nil || dep.Account == nil {
		t.Fatalf("deposit %+v", dep)
	}
	after := f.account()
	if !after.Balance.Balance.Equal(rent) {
		t.Fatalf("the deposit reduced the receivable: %s, want %s", after.Balance.Balance, rent)
	}
	if !after.Balance.DepositHeld.Equal(money.MustParse("2000")) {
		t.Fatalf("deposit held %s", after.Balance.DepositHeld)
	}
	if got := f.depositEntries(dep.UUID); got["deposit_hold:deposit"] != 1 || len(got) != 1 {
		t.Fatalf("deposit entries %v", got)
	}
	if n := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'collection'`, f.org); n != 0 {
		t.Fatalf("%d collections posted for a deposit", n)
	}
	if !f.tillBalance(till.UUID).Equal(money.MustParse("2000")) {
		t.Fatalf("till %s", f.tillBalance(till.UUID))
	}
	if n := f.count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'deposit_in'`, f.org); n != 1 {
		t.Fatalf("%d deposit_in movements", n)
	}
	sum, err := f.fin.Summary(ctx, f.org)
	f.must(err)
	if !sum.Receivable.Equal(rent) || !sum.DepositHeld.Equal(money.MustParse("2000")) {
		t.Fatalf("summary receivable %s deposit %s", sum.Receivable, sum.DepositHeld)
	}
	// One open deposit per rental; a deposit posting is not reversed by hand.
	var se *usecase.StateError
	if _, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositPOSPreAuth, POSReference: "123456"}); !errors.As(err, &se) || se.Code != "deposit_open" {
		t.Fatalf("second deposit: %v", err)
	}
	var hold uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `SELECT uuid FROM ledger_entries WHERE organization_id = $1 AND kind = 'deposit_hold'`, f.org).Scan(&hold))
	if _, err := f.fin.Reverse(ctx, f.org, hold, model.ReverseInput{Reason: "yanlış"}); !errors.As(err, &se) || se.Code != "deposit_managed" {
		t.Fatalf("manual reversal of a deposit hold: %v", err)
	}
	var depositIn uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `SELECT uuid FROM cash_movements WHERE organization_id = $1 AND kind = 'deposit_in'`, f.org).Scan(&depositIn))
	if _, err := f.fin.ReverseMovement(ctx, f.org, depositIn, model.ReverseInput{Reason: "yanlış"}); !errors.As(err, &se) || se.Code != "deposit_movement" {
		t.Fatalf("manual reversal of a deposit movement: %v", err)
	}
	// The statement's running balance ignores the deposit leg too.
	st, err := f.fin.Statement(ctx, f.org, after.UUID, model.EntryFilter{Limit: 50})
	f.must(err)
	last := st.Entries[len(st.Entries)-1]
	if !last.RunningBalance.Equal(rent) {
		t.Fatalf("running balance %s, want %s", last.RunningBalance, rent)
	}
	// Taking needs the write permission.
	r2, _ := f.rental(f.branch)
	readOnly := f.scoped(nil, rbac.PermTenantFinanceDepositsRead)
	if _, err := f.fin.TakeDeposit(readOnly, f.org, model.DepositInput{RentalUUID: &r2.UUID, Method: model.DepositPOSPreAuth, POSReference: "1"}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("take without write: %v", err)
	}
}

// TestDepositPreAuthPostsNothing: a POS pre-authorization moves no money:
// no ledger entry, no cash movement, no revenue; its release posts nothing
// either.
func TestDepositPreAuthPostsNothing(t *testing.T) {
	f, ctx, _ := newDepositFx(t)
	r, res := f.rental(f.branch)
	r = f.handover(r)
	if _, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositPOSPreAuth}); err == nil {
		t.Fatal("a POS pre-auth without a reference was accepted")
	}
	entries := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1`, f.org)
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositPOSPreAuth, POSReference: "PRV-778899",
		Amount: mp("3500")})
	f.must(err)
	if dep.Status != model.DepositHeld || dep.POSReference != "PRV-778899" || !dep.OpenAmount.Equal(money.MustParse("3500")) {
		t.Fatalf("deposit %+v", dep)
	}
	if n := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1`, f.org); n != entries {
		t.Fatalf("a pre-auth posted %d entries", n-entries)
	}
	if n := f.count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1`, f.org); n != 0 {
		t.Fatalf("a pre-auth moved money (%d movements)", n)
	}
	acc := f.account()
	if !acc.Balance.Balance.Equal(f.total(res.UUID)) || !acc.Balance.DepositHeld.IsZero() {
		t.Fatalf("balance %s deposit %s", acc.Balance.Balance, acc.Balance.DepositHeld)
	}
	totals, err := f.fin.DepositTotals(ctx, f.org, 0)
	f.must(err)
	if totals.HeldCount != 1 || !totals.ByMethod[model.DepositPOSPreAuth].Equal(money.MustParse("3500")) {
		t.Fatalf("totals %+v", totals)
	}
	out, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{IdempotencyKey: "rel-1"})
	f.must(err)
	if out.Status != model.DepositReleased || !out.ReturnedAmount.Equal(money.MustParse("3500")) || out.ResolvedAt == nil {
		t.Fatalf("released %+v", out)
	}
	if n := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1`, f.org); n != entries {
		t.Fatalf("a pre-auth release posted %d entries", n-entries)
	}
}

// TestDepositPartialCaptureAgainstCharge: a cash deposit of 2 000, an
// approved damage charge of 750: the deduction is captured as a collection
// applied to the charge (deposit_capture + collection), the remaining
// 1 250 is paid back from the till (deposit_return + deposit_out), the
// customer owes the rent only, the liability is zero.
func TestDepositPartialCaptureAgainstCharge(t *testing.T) {
	f, ctx, till := newDepositFx(t)
	r, res := f.rental(f.branch)
	r = f.handover(r)
	rid := rentalID(f, r.UUID)
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID})
	f.must(err)
	var proposed, damage uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason)
		VALUES ($1, $2, 'damage', 'manual', 750, 'tampon çiziği') RETURNING uuid`, f.org, rid).Scan(&damage))
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason)
		VALUES ($1, $2, 'fuel', 'manual', 120, 'yakıt eksik') RETURNING uuid`, f.org, rid).Scan(&proposed))
	if _, err := f.rentals.DecideCharge(ctx, f.org, r.UUID, damage, true, "fotoğraflarla doğrulandı"); err != nil {
		t.Fatal(err)
	}
	rent := f.total(res.UUID)
	if acc := f.account(); !acc.Balance.Balance.Equal(rent.Add(money.MustParse("750"))) {
		t.Fatalf("balance with the approved charge %s", acc.Balance.Balance)
	}
	line := func(amount string, charge *uuid.UUID, reason string) model.DeductionInput {
		return model.DeductionInput{Amount: mp(amount), RentalChargeUUID: charge, Reason: reason}
	}
	// Deductions need the deduct permission and a reason, an approved
	// charge, and stay within the charge and the deposit.
	noDeduct := f.scoped(nil, append(append([]string{}, allPerms...), rbac.PermTenantFinanceDepositsRead, rbac.PermTenantFinanceDepositsWrite)...)
	if _, err := f.fin.ResolveDeposit(noDeduct, f.org, dep.UUID, model.ResolveInput{Deductions: []model.DeductionInput{line("750", &damage, "hasar bedeli")}}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("deduction without permission: %v", err)
	}
	var ve *usecase.ValidationError
	for name, in := range map[string]model.DeductionInput{
		"no reason":      line("750", &damage, ""),
		"over charge":    line("800", &damage, "hasar bedeli"),
		"not approved":   line("120", &proposed, "yakıt"),
		"unknown charge": line("100", ptr(uuid.New()), "yok"),
		"over deposit":   {Amount: mp("2500"), AppliesTo: model.AppliesGeneral, Reason: "genel kesinti"},
	} {
		if _, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{Deductions: []model.DeductionInput{in}}); !errors.As(err, &ve) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if n := f.count(`SELECT count(*) FROM deposit_deductions WHERE organization_id = $1`, f.org); n != 0 {
		t.Fatalf("refused resolutions left %d deductions", n)
	}
	out, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{
		Deductions: []model.DeductionInput{line("750", &damage, "tampon onarımı")}, IdempotencyKey: "res-1", Note: "kalan iade edildi",
	})
	f.must(err)
	if out.Status != model.DepositPartiallyCaptured || !out.CapturedAmount.Equal(money.MustParse("750")) ||
		!out.ReturnedAmount.Equal(money.MustParse("1250")) || len(out.Deductions) != 1 || out.Deductions[0].Charge == nil {
		t.Fatalf("resolved %+v", out)
	}
	if out.CustomerNoticeStatus != "pending" || out.CustomerNotice["captured"] != "750.00" {
		t.Fatalf("customer notice %s %v", out.CustomerNoticeStatus, out.CustomerNotice)
	}
	got := f.depositEntries(dep.UUID)
	want := map[string]int{"deposit_hold:deposit": 1, "deposit_capture:deposit": 1, "collection:charge": 1, "deposit_return:deposit": 1}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("deposit entries %v, want %v", got, want)
		}
	}
	// The collection names the charge it settles.
	if n := f.count(`SELECT count(*) FROM ledger_entries e JOIN rental_charges c ON c.organization_id = e.organization_id AND c.id = e.rental_charge_id
		WHERE e.organization_id = $1 AND e.kind = 'collection' AND c.uuid = $2 AND e.amount = 750`, f.org, damage); n != 1 {
		t.Fatal("the capture is not applied to the charge")
	}
	acc := f.account()
	if !acc.Balance.Balance.Equal(rent) || !acc.Balance.DepositHeld.IsZero() {
		t.Fatalf("after the capture: balance %s (want the rent %s), deposit %s", acc.Balance.Balance, rent, acc.Balance.DepositHeld)
	}
	if !f.tillBalance(till.UUID).Equal(money.MustParse("750")) {
		t.Fatalf("till after paying back 1 250: %s", f.tillBalance(till.UUID))
	}
	var se *usecase.StateError
	// Collections (TEC-718): the deduction is a collection document
	// allocated to the charge, so the charge shows as paid; the deposit
	// hold / return have no document and are no customer credit.
	items, err := f.fin.OpenItems(ctx, f.org, acc.UUID, &r.UUID, false)
	f.must(err)
	paid := false
	for _, it := range items.Items {
		if it.Purpose == model.PurposeCharge && it.Amount.Equal(money.MustParse("750")) {
			paid = it.Status == model.ItemPaid
		}
	}
	if !paid || !items.Credit.IsZero() {
		t.Fatalf("open items after the capture: %+v", items)
	}
	if n := f.count(`SELECT count(*) FROM collections c JOIN ledger_entries e ON e.organization_id = c.organization_id AND e.id = c.ledger_entry_id
		WHERE c.organization_id = $1 AND e.kind IN ('deposit_hold', 'deposit_return', 'deposit_capture')`, f.org); n != 0 {
		t.Fatalf("%d deposit legs became collections", n)
	}
	var docUUID uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `SELECT c.uuid FROM collections c JOIN ledger_entries e ON e.organization_id = c.organization_id
		AND e.id = c.ledger_entry_id WHERE c.organization_id = $1 AND e.deposit_deduction_id IS NOT NULL`, f.org).Scan(&docUUID))
	if n := f.count(`SELECT count(*) FROM collection_allocations a JOIN collections c ON c.organization_id = a.organization_id AND c.id = a.collection_id
		WHERE a.organization_id = $1 AND c.uuid = $2 AND a.kind = 'allocate' AND a.amount = 750`, f.org, docUUID); n != 1 {
		t.Fatalf("deduction allocations %d", n)
	}
	if _, err := f.fin.VoidCollection(ctx, f.org, docUUID, model.CollectionVoidInput{Reason: "deneme"}); !errors.As(err, &se) || se.Code != "deposit_managed" {
		t.Fatalf("void of a deduction collection: %v", err)
	}
	if _, err := f.fin.RefundCollection(ctx, f.org, docUUID, model.CollectionRefundInput{Reason: "deneme"}, ""); !errors.As(err, &se) || se.Code != "deposit_managed" {
		t.Fatalf("refund of a deduction collection: %v", err)
	}
	if n := f.count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'deposit_out' AND amount = 1250`, f.org); n != 1 {
		t.Fatal("no deposit_out movement")
	}
	// The deduction's document (a repair invoice) is a private object under
	// the organization prefix, streamed back by the API.
	ded := out.Deductions[0].UUID
	pdf := []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")
	if _, err := f.fin.UploadDeductionDocument(ctx, f.org, dep.UUID, ded, usecase.FileUpload{Body: strings.NewReader("not a document"), Size: 14, Filename: "x.txt"}); err == nil {
		t.Fatal("a text file was accepted")
	}
	withDoc, err := f.fin.UploadDeductionDocument(ctx, f.org, dep.UUID, ded, usecase.FileUpload{Body: strings.NewReader(string(pdf)), Size: int64(len(pdf)), Filename: "fatura.pdf"})
	f.must(err)
	if d := withDoc.Deductions[0].Document; d == nil || d.ContentType != "application/pdf" || !strings.Contains(d.URL, "/v1/tenant/finance/deposits/") {
		t.Fatalf("document %+v", withDoc.Deductions[0].Document)
	}
	var key string
	f.must(f.pool.QueryRow(context.Background(), `SELECT document_object_key FROM deposit_deductions WHERE organization_id = $1 AND uuid = $2`, f.org, ded).Scan(&key))
	if !strings.HasPrefix(key, "organizations/"+f.orgUUID.String()+"/deposits/"+dep.UUID.String()+"/deductions/") {
		t.Fatalf("object key %s", key)
	}
	file, err := f.fin.OpenDeductionDocument(ctx, f.org, dep.UUID, ded)
	f.must(err)
	_ = file.Body.Close()
	if _, err := f.fin.OpenDeductionDocument(ctx, f.org, dep.UUID, uuid.New()); !errors.Is(err, usecase.ErrDeductionNotFound) {
		t.Fatalf("unknown deduction: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

// TestDepositReleaseIdempotent: a release is posted once. The same key
// answers the resolved deposit; another key, a second concurrent call or a
// capture after the release changes nothing; the database refuses a second
// return entry.
func TestDepositReleaseIdempotent(t *testing.T) {
	f, ctx, till := newDepositFx(t)
	r, _ := f.rental(f.branch)
	r = f.handover(r)
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID})
	f.must(err)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{IdempotencyKey: "release-1"})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := f.depositEntries(dep.UUID); got["deposit_return:deposit"] != 1 {
		t.Fatalf("deposit entries %v", got)
	}
	if n := f.count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1 AND kind = 'deposit_out'`, f.org); n != 1 {
		t.Fatalf("%d deposit_out movements", n)
	}
	out, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{IdempotencyKey: "release-1"})
	f.must(err)
	if out.Status != model.DepositRefunded || !out.ReturnedAmount.Equal(money.MustParse("2000")) {
		t.Fatalf("replay %+v", out)
	}
	var se *usecase.StateError
	if _, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{IdempotencyKey: "release-2"}); !errors.As(err, &se) || se.Code != "already_resolved" {
		t.Fatalf("second release: %v", err)
	}
	if _, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{
		Deductions: []model.DeductionInput{{Amount: mp("100"), AppliesTo: model.AppliesGeneral, Reason: "geç kesinti"}},
	}); !errors.As(err, &se) || se.Code != "already_resolved" {
		t.Fatalf("capture after the release: %v", err)
	}
	if !f.tillBalance(till.UUID).IsZero() || !f.account().Balance.DepositHeld.IsZero() {
		t.Fatal("money moved twice")
	}
	var accID, depID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT account_id, id FROM security_deposits WHERE uuid = $1`, dep.UUID).Scan(&accID, &depID))
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO ledger_entries (organization_id, account_id, kind, direction, purpose, amount,
		entry_date, due_date, source_type, security_deposit_id) VALUES ($1, $2, 'deposit_return', 'debit', 'deposit', 10, CURRENT_DATE,
		CURRENT_DATE, 'security_deposit', $3)`, f.org, accID, depID); err == nil || !strings.Contains(err.Error(), "uq_ledger_entries_deposit_return") {
		t.Fatalf("a second return entry: %v", err)
	}
}

// TestDepositCaptureIdempotent: a capture is posted once per deduction,
// also when the same resolution arrives twice at the same moment.
func TestDepositCaptureIdempotent(t *testing.T) {
	f, ctx, till := newDepositFx(t)
	r, _ := f.rental(f.branch)
	r = f.handover(r)
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID})
	f.must(err)
	in := model.ResolveInput{IdempotencyKey: "cap-1", Deductions: []model.DeductionInput{
		{Amount: mp("400"), AppliesTo: model.AppliesRent, Reason: "kira farkı"},
		{Amount: mp("100"), AppliesTo: model.AppliesGeneral, Reason: "temizlik"},
	}}
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.fin.ResolveDeposit(ctx, f.org, dep.UUID, in)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if n := f.count(`SELECT count(*) FROM deposit_deductions WHERE organization_id = $1`, f.org); n != 2 {
		t.Fatalf("%d deductions", n)
	}
	got := f.depositEntries(dep.UUID)
	if got["deposit_capture:deposit"] != 2 || got["collection:rent"] != 1 || got["collection:general"] != 1 || got["deposit_return:deposit"] != 1 {
		t.Fatalf("entries %v", got)
	}
	if !f.tillBalance(till.UUID).Equal(money.MustParse("500")) {
		t.Fatalf("till %s", f.tillBalance(till.UUID))
	}
	var accID, depID, dedID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT d.account_id, d.id, x.id FROM security_deposits d JOIN deposit_deductions x
		ON x.organization_id = d.organization_id AND x.deposit_id = d.id WHERE d.uuid = $1 LIMIT 1`, dep.UUID).Scan(&accID, &depID, &dedID))
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO ledger_entries (organization_id, account_id, kind, direction, purpose, amount,
		entry_date, due_date, source_type, security_deposit_id, deposit_deduction_id) VALUES ($1, $2, 'deposit_capture', 'debit', 'deposit', 10,
		CURRENT_DATE, CURRENT_DATE, 'security_deposit', $3, $4)`, f.org, accID, depID, dedID); err == nil {
		t.Fatal("a second capture leg of one deduction was accepted")
	}
}

// TestDepositOnlinePreAuth: iyzico pre-auth through the fake: pending until
// the provider confirms it, a post-auth of the deduction total (a
// collection applied to the charge, no deposit leg: no money was held as
// a liability), and a cancel for a full release.
func TestDepositOnlinePreAuth(t *testing.T) {
	f, ctx, _ := newDepositFx(t)
	r, _ := f.rental(f.branch)
	r = f.handover(r)
	rid := rentalID(f, r.UUID)
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositOnlinePreAuth})
	f.must(err)
	if dep.Status != model.DepositPending || dep.PaymentPageURL == "" || !dep.Capabilities.Verify {
		t.Fatalf("online deposit %+v", dep)
	}
	// Not confirmed yet: verify keeps it pending, a resolution is refused.
	if got, err := f.fin.VerifyDeposit(ctx, f.org, dep.UUID); err != nil || got.Status != model.DepositPending {
		t.Fatalf("verify before the customer: %+v %v", got, err)
	}
	var se *usecase.StateError
	if _, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{}); !errors.As(err, &se) || se.Code != "not_held" {
		t.Fatalf("resolve while pending: %v", err)
	}
	token := f.tokenOf(dep.UUID)
	if c, _ := f.fake.Checkout(token); !c.PreAuth || !c.Price.Equal(money.MustParse("2000")) {
		t.Fatalf("provider checkout %+v", c)
	}
	if _, _, err := f.fake.Complete(token, iyzicofake.StatePaid); err != nil {
		t.Fatal(err)
	}
	held, err := f.fin.VerifyDeposit(ctx, f.org, dep.UUID)
	f.must(err)
	if held.Status != model.DepositHeld || held.ProviderPaymentID == "" || held.PaymentPageURL != "" {
		t.Fatalf("verified %+v", held)
	}
	if n := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND security_deposit_id IS NOT NULL`, f.org); n != 0 {
		t.Fatalf("a pre-auth posted %d entries", n)
	}
	var charge uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason)
		VALUES ($1, $2, 'fuel', 'manual', 500, 'yakıt eksik') RETURNING uuid`, f.org, rid).Scan(&charge))
	if _, err := f.rentals.DecideCharge(ctx, f.org, r.UUID, charge, true, "onay"); err != nil {
		t.Fatal(err)
	}
	balance := f.account().Balance.Balance
	entries := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1`, f.org)
	out, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{IdempotencyKey: "on-1", ClientIP: "10.0.0.5",
		Deductions: []model.DeductionInput{{Amount: mp("500"), RentalChargeUUID: &charge, Reason: "yakıt bedeli"}}})
	f.must(err)
	if out.Status != model.DepositPartiallyCaptured || !out.CapturedAmount.Equal(money.MustParse("500")) {
		t.Fatalf("resolved %+v", out)
	}
	p, _ := f.fake.Payment(held.ProviderPaymentID)
	if !p.Captured.Equal(money.MustParse("500")) || f.fake.PostAuths.Load() != 1 {
		t.Fatalf("post-auth captured %s (%d calls)", p.Captured, f.fake.PostAuths.Load())
	}
	if got := f.depositEntries(dep.UUID); got["collection:charge"] != 1 || len(got) != 1 {
		t.Fatalf("online capture entries %v", got)
	}
	if n := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1`, f.org); n != entries+1 {
		t.Fatalf("%d entries posted, want 1 collection", n-entries)
	}
	if got := f.account().Balance.Balance; !got.Equal(balance.Sub(money.MustParse("500"))) {
		t.Fatalf("balance after the capture %s (was %s)", got, balance)
	}
	// A replay does not call the provider again.
	if _, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{IdempotencyKey: "on-1",
		Deductions: []model.DeductionInput{{Amount: mp("500"), RentalChargeUUID: &charge, Reason: "yakıt bedeli"}}}); err != nil {
		t.Fatal(err)
	}
	if f.fake.PostAuths.Load() != 1 {
		t.Fatalf("the replay captured again (%d)", f.fake.PostAuths.Load())
	}
	// A second online deposit released in full: iyzico cancel.
	r2, _ := f.rental(f.branch)
	r2 = f.handover(r2)
	dep2, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r2.UUID, Method: model.DepositOnlinePreAuth, Amount: mp("1500")})
	f.must(err)
	if _, _, err := f.fake.Complete(f.tokenOf(dep2.UUID), iyzicofake.StatePaid); err != nil {
		t.Fatal(err)
	}
	held2, err := f.fin.VerifyDeposit(ctx, f.org, dep2.UUID)
	f.must(err)
	rel, err := f.fin.ResolveDeposit(ctx, f.org, dep2.UUID, model.ResolveInput{IdempotencyKey: "on-2"})
	f.must(err)
	if rel.Status != model.DepositReleased || f.fake.Cancels.Load() != 1 {
		t.Fatalf("release %+v (cancels %d)", rel, f.fake.Cancels.Load())
	}
	if p2, _ := f.fake.Payment(held2.ProviderPaymentID); !p2.Cancelled {
		t.Fatal("the provider hold was not released")
	}
	// A declined pre-auth is cancelled on verify.
	r3, _ := f.rental(f.branch)
	dep3, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r3.UUID, Method: model.DepositOnlinePreAuth})
	f.must(err)
	if _, _, err := f.fake.Complete(f.tokenOf(dep3.UUID), iyzicofake.StateFailed); err != nil {
		t.Fatal(err)
	}
	if got, err := f.fin.VerifyDeposit(ctx, f.org, dep3.UUID); err != nil || got.Status != model.DepositCancelled {
		t.Fatalf("declined: %+v %v", got, err)
	}
	// Without a payment account the online path is refused (manual path).
	_, err = f.pool.Exec(context.Background(), `UPDATE integration_accounts SET status = 'disabled' WHERE organization_id = $1`, f.org)
	f.must(err)
	var ve *usecase.ValidationError
	if _, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r3.UUID, Method: model.DepositOnlinePreAuth}); !errors.As(err, &ve) ||
		ve.Details[0].Code != "online_unavailable" {
		t.Fatalf("online without an account: %v", err)
	}
}

func (f *fx) tokenOf(dep uuid.UUID) string {
	f.t.Helper()
	var token string
	f.must(f.pool.QueryRow(context.Background(), `SELECT provider_token FROM security_deposits WHERE organization_id = $1 AND uuid = $2`,
		f.org, dep).Scan(&token))
	return token
}

// TestDepositTaskCompletes: the return opens the deposit task; resolving
// the deposit completes it.
func TestDepositTaskCompletes(t *testing.T) {
	f, ctx, till := newDepositFx(t)
	r, _ := f.rental(f.branch)
	r = f.handover(r)
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID})
	f.must(err)
	km, fuel := int32(1300), int16(80)
	if _, err := f.rentals.Return(ctx, f.org, r.UUID, rentalmodel.ReturnInput{Km: &km, Fuel: &fuel}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.rentals.CompleteReturn(ctx, f.org, r.UUID, rentalmodel.CompleteReturnInput{VehicleStatus: "available", DepositTask: true}); err != nil {
		t.Fatal(err)
	}
	card, err := f.fin.DepositsForRental(ctx, f.org, r.UUID)
	f.must(err)
	if card.DepositTask == nil || card.Open == nil || *card.Open != dep.UUID || card.CanTake {
		t.Fatalf("deposit card after the return %+v", card)
	}
	if _, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{IdempotencyKey: "task-1"}); err != nil {
		t.Fatal(err)
	}
	var status, note string
	f.must(f.pool.QueryRow(context.Background(), `SELECT status, completion_note FROM operation_tasks WHERE organization_id = $1 AND uuid = $2`,
		f.org, *card.DepositTask).Scan(&status, &note))
	if status != "completed" || note != "deposit:refunded" {
		t.Fatalf("deposit task %s %q", status, note)
	}
	card, err = f.fin.DepositsForRental(ctx, f.org, r.UUID)
	f.must(err)
	if card.DepositTask != nil || card.Open != nil {
		t.Fatalf("deposit card after the resolution %+v", card)
	}
}

// TestDepositListFilters: status / method / branch filters and "held over
// N days".
func TestDepositListFilters(t *testing.T) {
	f, ctx, till := newDepositFx(t)
	r1, _ := f.rental(f.branch)
	r1 = f.handover(r1)
	r2, _ := f.rental(f.branch)
	r2 = f.handover(r2)
	d1, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r1.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID})
	f.must(err)
	_, err = f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r2.UUID, Method: model.DepositPOSPreAuth, POSReference: "A1"})
	f.must(err)
	list := func(f2 model.DepositFilter) []model.Deposit {
		t.Helper()
		f2.Limit = 50
		items, _, err := f.fin.ListDeposits(ctx, f.org, f2)
		f.must(err)
		return items
	}
	if got := list(model.DepositFilter{Methods: []string{model.DepositCash}}); len(got) != 1 || got[0].UUID != d1.UUID {
		t.Fatalf("method filter %d", len(got))
	}
	if got := list(model.DepositFilter{BranchUUIDs: []uuid.UUID{f.other.uuid}}); len(got) != 0 {
		t.Fatalf("branch filter %d", len(got))
	}
	seven := 7
	if got := list(model.DepositFilter{HeldOverDays: &seven}); len(got) != 0 {
		t.Fatalf("held over 7 days now: %d", len(got))
	}
	f.fin.SetClock(func() time.Time { return time.Now().Add(10 * 24 * time.Hour) })
	defer f.fin.SetClock(time.Now)
	if got := list(model.DepositFilter{HeldOverDays: &seven}); len(got) != 2 || got[0].HeldDays < 9 {
		t.Fatalf("held over 7 days ten days later: %d", len(got))
	}
	totals, err := f.fin.DepositTotals(ctx, f.org, 7)
	f.must(err)
	if totals.HeldOver != 2 || totals.HeldCount != 2 {
		t.Fatalf("totals %+v", totals)
	}
	if _, _, err := f.fin.ListDeposits(ctx, f.org, model.DepositFilter{Statuses: []string{"bogus"}}); err == nil {
		t.Fatal("an unknown status filter was accepted")
	}
}

// TestDepositBranchScopeAndIsolation: a member of another branch does not
// see the deposit (not found everywhere); another organization's uuids
// answer not found, its charges are unknown to a deduction.
func TestDepositBranchScopeAndIsolation(t *testing.T) {
	f, ctx, till := newDepositFx(t)
	r, _ := f.rental(f.branch)
	r = f.handover(r)
	dep, err := f.fin.TakeDeposit(ctx, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID})
	f.must(err)
	hvl := f.depositCtx([]int64{f.other.id})
	if _, err := f.fin.GetDeposit(hvl, f.org, dep.UUID); !errors.Is(err, usecase.ErrDepositNotFound) {
		t.Fatalf("other branch get: %v", err)
	}
	if items, _, err := f.fin.ListDeposits(hvl, f.org, model.DepositFilter{Limit: 50}); err != nil || len(items) != 0 {
		t.Fatalf("other branch list: %d %v", len(items), err)
	}
	if _, err := f.fin.ResolveDeposit(hvl, f.org, dep.UUID, model.ResolveInput{}); !errors.Is(err, usecase.ErrDepositNotFound) {
		t.Fatalf("other branch resolve: %v", err)
	}
	if _, err := f.fin.DepositsForRental(hvl, f.org, r.UUID); !errors.Is(err, usecase.ErrRentalNotFound) {
		t.Fatalf("other branch rental card: %v", err)
	}
	var ve *usecase.ValidationError
	if _, err := f.fin.TakeDeposit(hvl, f.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositPOSPreAuth, POSReference: "X"}); !errors.As(err, &ve) {
		t.Fatalf("other branch take: %v", err)
	}
	// Another organization.
	g := newFx(t)
	gctx := g.depositCtx(nil)
	gr, _ := g.rental(g.branch)
	gr = g.handover(gr)
	var gCharge uuid.UUID
	g.must(g.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason, status, decided_at)
		VALUES ($1, $2, 'fuel', 'manual', 100, 'yakıt', 'approved', NOW()) RETURNING uuid`, g.org, rentalID(g, gr.UUID)).Scan(&gCharge))
	if _, err := f.fin.GetDeposit(gctx, g.org, dep.UUID); !errors.Is(err, usecase.ErrDepositNotFound) {
		t.Fatalf("cross-organization get: %v", err)
	}
	if _, err := f.fin.ResolveDeposit(gctx, g.org, dep.UUID, model.ResolveInput{}); !errors.Is(err, usecase.ErrDepositNotFound) {
		t.Fatalf("cross-organization resolve: %v", err)
	}
	if _, err := f.fin.TakeDeposit(gctx, g.org, model.DepositInput{RentalUUID: &r.UUID, Method: model.DepositPOSPreAuth, POSReference: "X"}); !errors.As(err, &ve) {
		t.Fatalf("cross-organization rental: %v", err)
	}
	if _, err := f.fin.TakeDeposit(gctx, g.org, model.DepositInput{RentalUUID: &gr.UUID, Method: model.DepositCash, CashAccountUUID: &till.UUID}); !errors.As(err, &ve) {
		t.Fatalf("cross-organization till: %v", err)
	}
	if _, err := f.fin.ResolveDeposit(ctx, f.org, dep.UUID, model.ResolveInput{
		Deductions: []model.DeductionInput{{Amount: mp("100"), RentalChargeUUID: &gCharge, Reason: "başka şirket"}},
	}); !errors.As(err, &ve) {
		t.Fatalf("cross-organization charge: %v", err)
	}
	if items, _, err := f.fin.ListDeposits(gctx, g.org, model.DepositFilter{Limit: 50}); err != nil || len(items) != 0 {
		t.Fatalf("cross-organization list: %d %v", len(items), err)
	}
}
