package finance_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// ------------------------------------------------------------- helpers

// till / bank open a cash account of the fixture.
func (f *fx) till(code string, b ref, opening string) model.CashAccount {
	f.t.Helper()
	a, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp(code), Name: sp(code), Kind: sp(model.CashKindCash),
		BranchUUID: &b.uuid, OpeningBalance: mp(opening)})
	f.must(err)
	return a
}

func (f *fx) bank(code string, provider string) model.CashAccount {
	f.t.Helper()
	in := model.CashAccountInput{Code: sp(code), Name: sp(code), Kind: sp(model.CashKindBank)}
	if provider != "" {
		in.Kind = sp(model.CashKindClearing)
		in.Provider = sp(provider)
	}
	a, err := f.fin.CreateCashAccount(f.ctx, f.org, in)
	f.must(err)
	return a
}

// charge adds an approved rental charge (accrued through the rental).
func (f *fx) charge(r rentalmodel.Rental, kind, amount string) {
	f.t.Helper()
	var id uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason)
		VALUES ($1, $2, $3, 'manual', $4, 'test') RETURNING uuid`, f.org, rentalID(f, r.UUID), kind, amount).Scan(&id))
	if _, err := f.rentals.DecideCharge(f.ctx, f.org, r.UUID, id, true, "onaylandı"); err != nil {
		f.t.Fatalf("approve charge: %v", err)
	}
}

func (f *fx) openItems(acc uuid.UUID, rental *uuid.UUID) model.OpenItems {
	f.t.Helper()
	out, err := f.fin.OpenItems(f.ctx, f.org, acc, rental, false)
	f.must(err)
	return out
}

func (f *fx) collect(in model.CollectionCreateInput) model.Collection {
	f.t.Helper()
	c, _, err := f.fin.CreateCollection(f.ctx, f.org, in)
	f.must(err)
	return c
}

func (f *fx) cashBalance(id uuid.UUID) money.Money {
	f.t.Helper()
	a, err := f.fin.GetCashAccount(f.ctx, f.org, id)
	f.must(err)
	return a.Balance
}

// payOnline pays a link of the reservation on the iyzico fake (webhook) and
// returns the payment uuid.
func (f *fx) payOnline(reservation uuid.UUID, amount string) uuid.UUID {
	f.t.Helper()
	link, err := f.pay.CreateLink(f.ctx, f.org, reservation, paymodel.CreateLinkInput{Amount: mp(amount)})
	f.must(err)
	token := link.URL[strings.LastIndex(link.URL, "/")+1:]
	before := map[string]bool{}
	for _, tk := range f.fake.Tokens() {
		before[tk] = true
	}
	if _, err := f.pay.StartCheckout(context.Background(), token, "10.0.0.7", "tr"); err != nil {
		f.t.Fatal(err)
	}
	fakeToken := ""
	for _, tk := range f.fake.Tokens() {
		if !before[tk] {
			fakeToken = tk
		}
	}
	body, sig, err := f.fake.Complete(fakeToken, iyzicofake.StatePaid)
	f.must(err)
	h := http.Header{}
	h.Set(iyzico.SignatureHeader, sig)
	if _, err := f.pay.Webhook(context.Background(), iyzico.ProviderCode, h, body); err != nil {
		f.t.Fatalf("webhook: %v", err)
	}
	var id uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `SELECT p.uuid FROM payments p JOIN payment_links l ON l.organization_id = p.organization_id
		AND l.id = p.payment_link_id WHERE p.organization_id = $1 AND l.uuid = $2 AND p.status = 'succeeded'`, f.org, link.UUID).Scan(&id))
	return id
}

func (f *fx) collectionOfPayment(payment uuid.UUID) model.Collection {
	f.t.Helper()
	var id uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `SELECT c.uuid FROM collections c JOIN payments p ON p.organization_id = c.organization_id
		AND p.id = c.payment_id WHERE c.organization_id = $1 AND p.uuid = $2`, f.org, payment).Scan(&id))
	c, err := f.fin.GetCollection(f.ctx, f.org, id)
	f.must(err)
	return c
}

func item(items model.OpenItems, purpose string) model.OpenItem {
	for _, it := range items.Items {
		if it.Purpose == purpose {
			return it
		}
	}
	return model.OpenItem{}
}

func eq(t *testing.T, what string, got money.Money, want string) {
	t.Helper()
	if !got.Equal(money.MustParse(want)) {
		t.Fatalf("%s = %s, want %s", what, got, want)
	}
}

// --------------------------------------------------------------- tests

// TestCollectionAllocation: a cash collection pays the oldest open item
// first (rent partially paid, the charge still open); a second one is
// split by hand over the rest of the rent and part of the charge; a manual
// amount above an item's open amount or above the collection is refused;
// the till and the cash movement follow.
func TestCollectionAllocation(t *testing.T) {
	f := newFx(t)
	r, res := f.rental(f.branch)
	f.handover(r)
	f.charge(r, "fuel", "300")
	acc := f.account()
	total := f.total(res.UUID)
	till := f.till("KASA", f.branch, "0")

	if _, _, err := f.fin.CreateCollection(f.ctx, f.org, model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: mp("100"),
		Method: model.MethodCash}); err == nil {
		t.Fatal("a cash collection without a till was accepted")
	}
	bank := f.bank("BANKA", "")
	var ve *usecase.ValidationError
	if _, _, err := f.fin.CreateCollection(f.ctx, f.org, model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: mp("100"),
		Method: model.MethodCash, CashAccountUUID: &bank.UUID}); !errors.As(err, &ve) || ve.Details[0].Code != "kind_mismatch" {
		t.Fatalf("cash into a bank account: %v", err)
	}
	noCollect := f.scoped(nil, rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite)
	if _, _, err := f.fin.CreateCollection(noCollect, f.org, model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: mp("1"),
		Method: model.MethodCash, CashAccountUUID: &till.UUID}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("collection without payment.collect: %v", err)
	}

	part := total.Sub(money.MustParse("500"))
	c1 := f.collect(model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: &part, Method: model.MethodCash,
		CashAccountUUID: &till.UUID, ReferenceNo: "MAKBUZ-1", PayerName: "Ayşe Yılmaz", Note: "teslimde"})
	if c1.Account.UUID != acc.UUID || c1.Code == "" || c1.AllocationStatus != model.AllocAllocated || c1.MovementUUID == nil {
		t.Fatalf("first collection %+v", c1)
	}
	if len(c1.Allocations) != 1 || c1.Allocations[0].Purpose != model.PurposeRent || c1.Allocations[0].ItemStatus != model.ItemPartiallyPaid {
		t.Fatalf("first allocation %+v", c1.Allocations)
	}
	eq(t, "till", f.cashBalance(till.UUID), part.String())
	items := f.openItems(acc.UUID, &r.UUID)
	rent, fuel := item(items, model.PurposeRent), item(items, model.PurposeCharge)
	if rent.Status != model.ItemPartiallyPaid || fuel.Status != model.ItemOpen {
		t.Fatalf("after the first collection: rent %s fuel %s", rent.Status, fuel.Status)
	}
	eq(t, "rent open", rent.Open, "500")

	// Manual split: the rest of the rent and 100 of the fuel.
	overOpen := model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: mp("700"), Method: model.MethodBankTransfer,
		CashAccountUUID: &bank.UUID, AllocationMode: model.AllocateManual, Allocations: []model.AllocationItem{
			{EntryUUID: rent.EntryUUID, Amount: mp("600")},
		}}
	if _, _, err := f.fin.CreateCollection(f.ctx, f.org, overOpen); !errors.As(err, &ve) || ve.Details[0].Code != "exceeds_open" {
		t.Fatalf("allocation above the open amount: %v", err)
	}
	if got := f.count(`SELECT count(*) FROM collections WHERE organization_id = $1`, f.org); got != 1 {
		t.Fatalf("a refused collection left %d rows", got)
	}
	c2 := f.collect(model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: mp("700"), Method: model.MethodBankTransfer,
		CashAccountUUID: &bank.UUID, ReferenceNo: "EFT-77", AllocationMode: model.AllocateManual, Allocations: []model.AllocationItem{
			{EntryUUID: rent.EntryUUID}, {EntryUUID: fuel.EntryUUID, Amount: mp("100")},
		}})
	if len(c2.Allocations) != 2 || c2.AllocationStatus != model.AllocPartiallyAllocated {
		t.Fatalf("second collection %+v", c2)
	}
	eq(t, "second credit", c2.UnallocatedAmount, "100")
	items = f.openItems(acc.UUID, &r.UUID)
	if item(items, model.PurposeRent).Status != model.ItemPaid || item(items, model.PurposeCharge).Status != model.ItemPartiallyPaid {
		t.Fatalf("after the split: %+v", items.Items)
	}
	eq(t, "fuel open", item(items, model.PurposeCharge).Open, "200")
	eq(t, "credit", items.Credit, "100")
	// The ledger agrees: debits (rent + fuel) - credits (both collections).
	got, err := f.fin.GetAccount(f.ctx, f.org, acc.UUID)
	f.must(err)
	eq(t, "balance", got.Balance.Balance, total.Add(money.MustParse("300")).Sub(part).Sub(money.MustParse("700")).String())
	// Lists and filters.
	list, n, err := f.fin.ListCollections(f.ctx, f.org, model.CollectionFilter{Methods: []string{model.MethodCash}, Limit: 10})
	f.must(err)
	if n != 1 || list[0].UUID != c1.UUID {
		t.Fatalf("cash filter: %d", n)
	}
	list, _, err = f.fin.ListCollections(f.ctx, f.org, model.CollectionFilter{CashAccountUUIDs: []uuid.UUID{bank.UUID}, Limit: 10})
	f.must(err)
	if len(list) != 1 || list[0].UUID != c2.UUID {
		t.Fatalf("cash account filter: %d", len(list))
	}
	list, _, err = f.fin.ListCollections(f.ctx, f.org, model.CollectionFilter{Q: "EFT-77", Limit: 10})
	f.must(err)
	if len(list) != 1 {
		t.Fatalf("search by reference: %d", len(list))
	}
}

// TestOverpaymentCredit: what a collection pays above the open items is
// the customer's credit. A later charge of the rental takes the credit of
// the rental's own collections by itself; the credit of an account-level
// collection is allocated by hand (idempotent per request key). A retry
// with the create key returns the first collection.
func TestOverpaymentCredit(t *testing.T) {
	f := newFx(t)
	r, res := f.rental(f.branch)
	f.handover(r)
	acc := f.account()
	total := f.total(res.UUID)
	till := f.till("KASA", f.branch, "0")
	over := total.Add(money.MustParse("400"))
	key := uuid.New()
	in := model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: &over, Method: model.MethodCash, CashAccountUUID: &till.UUID,
		IdempotencyKey: &key}
	c, created, err := f.fin.CreateCollection(f.ctx, f.org, in)
	f.must(err)
	if !created || c.AllocationStatus != model.AllocPartiallyAllocated {
		t.Fatalf("overpayment %+v", c)
	}
	eq(t, "credit", c.UnallocatedAmount, "400")
	again, created, err := f.fin.CreateCollection(f.ctx, f.org, in)
	f.must(err)
	if created || again.UUID != c.UUID {
		t.Fatal("a retry with the same key recorded a second collection")
	}
	eq(t, "till after the retry", f.cashBalance(till.UUID), over.String())
	if got, _ := f.fin.GetAccount(f.ctx, f.org, acc.UUID); !got.Balance.Balance.Equal(money.MustParse("-400")) {
		t.Fatalf("balance %s", got.Balance.Balance)
	}
	// A rental-linked overpayment pays a later charge by itself.
	linked := f.collect(model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: mp("50"), Method: model.MethodCash,
		CashAccountUUID: &till.UUID})
	eq(t, "linked credit", linked.UnallocatedAmount, "50")
	f.charge(r, "fuel", "30")
	linked, err = f.fin.GetCollection(f.ctx, f.org, linked.UUID)
	f.must(err)
	eq(t, "linked credit after the charge", linked.UnallocatedAmount, "20")
	if item(f.openItems(acc.UUID, nil), model.PurposeCharge).Status != model.ItemPaid {
		t.Fatal("the rental's credit did not pay its new charge")
	}
	// A damage charge later: the account-level credit is allocated by hand.
	f.charge(r, "damage", "250")
	damage := func(items model.OpenItems) model.OpenItem {
		for _, it := range items.Items {
			if it.ChargeKind == "damage" {
				return it
			}
		}
		return model.OpenItem{}
	}
	items := f.openItems(acc.UUID, nil)
	if got := damage(items); got.Status != model.ItemPartiallyPaid || !got.Allocated.Equal(money.MustParse("20")) {
		t.Fatalf("the damage charge %+v (the rest of the rental's credit pays part of it)", got)
	}
	akey := uuid.New()
	out, err := f.fin.Allocate(f.ctx, f.org, c.UUID, model.AllocateInput{Mode: model.AllocateAuto, IdempotencyKey: &akey})
	f.must(err)
	eq(t, "credit after allocation", out.UnallocatedAmount, "170")
	out, err = f.fin.Allocate(f.ctx, f.org, c.UUID, model.AllocateInput{Mode: model.AllocateAuto, IdempotencyKey: &akey})
	f.must(err)
	eq(t, "credit after the retry", out.UnallocatedAmount, "170")
	items = f.openItems(acc.UUID, nil)
	if damage(items).Status != model.ItemPaid {
		t.Fatalf("charge after the credit %+v", items.Items)
	}
	eq(t, "account credit", items.Credit, "170")
	// Nothing left to pay: a manual allocation is refused.
	var se *usecase.StateError
	var ve *usecase.ValidationError
	if _, err := f.fin.Allocate(f.ctx, f.org, c.UUID, model.AllocateInput{Mode: model.AllocateManual,
		Items: []model.AllocationItem{{EntryUUID: damage(items).EntryUUID}}}); !errors.As(err, &ve) && !errors.As(err, &se) {
		t.Fatalf("allocating to a paid item: %v", err)
	}
	// Release a wrong allocation: the money is credit again.
	chargeID := damage(items).EntryUUID
	out, err = f.fin.Release(f.ctx, f.org, c.UUID, model.ReleaseInput{EntryUUID: &chargeID, Reason: "yanlış kalem"})
	f.must(err)
	eq(t, "credit after release", out.UnallocatedAmount, "400")
	if damage(f.openItems(acc.UUID, nil)).Status != model.ItemPartiallyPaid {
		t.Fatal("the released charge is not open again")
	}
}

// TestConcurrentAllocation: two collections race to pay the same open item
// in full; the account lock and the database guard let exactly one win.
// The same allocate request sent twice at once applies once.
func TestConcurrentAllocation(t *testing.T) {
	f := newFx(t)
	r, res := f.rental(f.branch)
	f.handover(r)
	acc := f.account()
	total := f.total(res.UUID)
	bank := f.bank("BANKA", "")
	mk := func() model.Collection {
		return f.collect(model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: &total, Method: model.MethodBankTransfer,
			CashAccountUUID: &bank.UUID, AllocationMode: model.AllocateNone})
	}
	a, b := mk(), mk()
	rent := item(f.openItems(acc.UUID, nil), model.PurposeRent)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, c := range []model.Collection{a, b} {
		wg.Add(1)
		go func(i int, c model.Collection) {
			defer wg.Done()
			_, errs[i] = f.fin.Allocate(f.ctx, f.org, c.UUID, model.AllocateInput{Mode: model.AllocateManual,
				Items: []model.AllocationItem{{EntryUUID: rent.EntryUUID, Amount: &total}}})
		}(i, c)
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("both or neither allocation won: %v / %v", errs[0], errs[1])
	}
	if got := item(f.openItems(acc.UUID, nil), model.PurposeRent); got.Status != model.ItemPaid || !got.Allocated.Equal(total) {
		t.Fatalf("rent after the race %+v", got)
	}
	// The database refuses over-allocation even without the use case.
	var cid, eid int64
	loser := b
	if errs[0] != nil {
		loser = a
	}
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM collections WHERE uuid = $1`, loser.UUID).Scan(&cid))
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM ledger_entries WHERE uuid = $1`, rent.EntryUUID).Scan(&eid))
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO collection_allocations (organization_id, collection_id, entry_id, kind, amount, source)
		VALUES ($1, $2, $3, 'allocate', 1, 'manual')`, f.org, cid, eid); err == nil {
		t.Fatal("the database accepted an allocation above the open item")
	}
	// Same request twice at once: one allocation.
	f.charge(r, "fuel", "120")
	key := uuid.New()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = f.fin.Allocate(f.ctx, f.org, loser.UUID, model.AllocateInput{Mode: model.AllocateAuto, IdempotencyKey: &key})
		}()
	}
	wg.Wait()
	if got := f.count(`SELECT count(*) FROM collection_allocations WHERE organization_id = $1 AND request_key = $2`, f.org, key); got != 1 {
		t.Fatalf("%d allocations for one request", got)
	}
	eq(t, "fuel allocated", item(f.openItems(acc.UUID, nil), model.PurposeCharge).Allocated, "120")
}

// TestOnlineCollectionCommission: an iyzico payment becomes an online
// collection with gross, commission and net apart; without a settlement
// account it waits (pending), the sweep settles it once one exists: the
// gross enters the clearing account and the commission leaves it as an
// expense, leaving the net. The prepayment pays the rent at the handover.
func TestOnlineCollectionCommission(t *testing.T) {
	f := newFx(t)
	r, res := f.rental(f.branch)
	pay := f.payOnline(res.UUID, "1000")
	c := f.collectionOfPayment(pay)
	if c.Method != model.MethodOnline || c.Provider != model.ProviderIyzico || c.SettlementStatus != model.SettlementPending {
		t.Fatalf("online collection %+v", c)
	}
	eq(t, "amount", c.Amount, "1000")
	eq(t, "gross", c.GrossAmount, "1000")
	// Fake: 2.75 % + 0.25.
	eq(t, "commission", c.CommissionAmount, "27.75")
	eq(t, "net", c.NetAmount, "972.25")
	if c.ReferenceNo == "" || c.Reservation == nil {
		t.Fatalf("reference / reservation missing: %+v", c)
	}
	clearing := f.bank("IYZICO", model.ProviderIyzico)
	if _, err := f.fin.PostPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	c = f.collectionOfPayment(pay)
	if c.SettlementStatus != model.SettlementSettled || c.CashAccount == nil || c.CashAccount.UUID != clearing.UUID ||
		c.CommissionExpense == nil {
		t.Fatalf("after the sweep %+v", c)
	}
	eq(t, "clearing balance", f.cashBalance(clearing.UUID), "972.25")
	var cat string
	f.must(f.pool.QueryRow(context.Background(), `SELECT ec.code FROM expenses x JOIN expense_categories ec ON ec.organization_id = x.organization_id
		AND ec.id = x.category_id WHERE x.organization_id = $1 AND x.uuid = $2`, f.org, *c.CommissionExpense).Scan(&cat))
	if cat != model.CommissionCategory {
		t.Fatalf("commission category %s", cat)
	}
	// The sweep again changes nothing.
	if _, err := f.fin.PostPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, "clearing balance after a second sweep", f.cashBalance(clearing.UUID), "972.25")
	if got := f.count(`SELECT count(*) FROM expenses WHERE organization_id = $1`, f.org); got != 1 {
		t.Fatalf("%d commission expenses", got)
	}
	// A second payment settles at once.
	pay2 := f.payOnline(res.UUID, "200")
	c2 := f.collectionOfPayment(pay2)
	if c2.SettlementStatus != model.SettlementSettled {
		t.Fatalf("second payment %+v", c2)
	}
	eq(t, "clearing after two payments", f.cashBalance(clearing.UUID), money.MustParse("972.25").Add(c2.NetAmount).String())
	// The handover accrues the rent and the prepayments pay it.
	f.handover(r)
	acc := f.account()
	rent := item(f.openItems(acc.UUID, &r.UUID), model.PurposeRent)
	eq(t, "rent allocated from prepayments", rent.Allocated, "1200")
	if c = f.collectionOfPayment(pay); c.AllocationStatus != model.AllocAllocated {
		t.Fatalf("prepayment allocation %s", c.AllocationStatus)
	}
}

// TestOnlineRefund: a refund of an online collection goes through the
// provider. Refused by the provider: a failed refund row, no entry, the
// collection unchanged. Succeeded: one refund entry, the collection's
// refunded amount, its allocation reduced and the money leaving the
// clearing account.
func TestOnlineRefund(t *testing.T) {
	f := newFx(t)
	r, res := f.rental(f.branch)
	clearing := f.bank("IYZICO", model.ProviderIyzico)
	f.handover(r)
	total := f.total(res.UUID)
	pay := f.payOnline(res.UUID, total.String())
	c := f.collectionOfPayment(pay)
	if c.AllocationStatus != model.AllocAllocated {
		t.Fatalf("the online payment did not pay the rent: %+v", c)
	}
	acc := f.account()
	balance := f.cashBalance(clearing.UUID)
	noRefund := f.scoped(nil, rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite, rbac.PermTenantPaymentCollect)
	if _, err := f.fin.RefundCollection(noRefund, f.org, c.UUID, model.CollectionRefundInput{Amount: mp("100"), Reason: "müşteri talebi"}, ""); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("refund without the permission: %v", err)
	}
	if _, err := f.fin.RefundCollection(f.ctx, f.org, c.UUID, model.CollectionRefundInput{Amount: mp("100")}, ""); err == nil {
		t.Fatal("a refund without a reason was accepted")
	}

	f.fake.FailRefunds.Store(true)
	_, err := f.fin.RefundCollection(f.ctx, f.org, c.UUID, model.CollectionRefundInput{Amount: mp("300"), Reason: "fazla çekim"}, "10.0.0.7")
	var pf *paymentsusecase.ProviderFailure
	if !errors.As(err, &pf) {
		t.Fatalf("refused refund: %v", err)
	}
	f.fake.FailRefunds.Store(false)
	c = f.collectionOfPayment(pay)
	if !c.RefundedAmount.IsZero() || c.Status != model.CollectionRecorded || len(c.Refunds) != 1 || c.Refunds[0].Status != "failed" ||
		c.Refunds[0].EntryUUID != nil {
		t.Fatalf("after the refused refund %+v", c)
	}
	if got := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'refund'`, f.org); got != 0 {
		t.Fatalf("a refused refund posted %d entries", got)
	}

	c, err = f.fin.RefundCollection(f.ctx, f.org, c.UUID, model.CollectionRefundInput{Amount: mp("300"), Reason: "fazla çekim"}, "10.0.0.7")
	f.must(err)
	eq(t, "refunded", c.RefundedAmount, "300")
	if c.Status != model.CollectionPartiallyRefunded || len(c.Refunds) != 2 {
		t.Fatalf("after the refund %+v", c)
	}
	var ok model.CollectionRefund
	for _, rf := range c.Refunds {
		if rf.Status == "succeeded" {
			ok = rf
		}
	}
	if ok.EntryUUID == nil || ok.MovementUUID == nil {
		t.Fatalf("succeeded refund %+v", ok)
	}
	eq(t, "allocated after the refund", c.AllocatedAmount, total.Sub(money.MustParse("300")).String())
	eq(t, "clearing after the refund", f.cashBalance(clearing.UUID), balance.Sub(money.MustParse("300")).String())
	rent := item(f.openItems(acc.UUID, &r.UUID), model.PurposeRent)
	if rent.Status != model.ItemPartiallyPaid {
		t.Fatalf("rent after the refund %s", rent.Status)
	}
	eq(t, "rent open", rent.Open, "300")
	// Posting the refund again (sweep) changes nothing.
	if _, err := f.fin.PostPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'refund'`, f.org); got != 1 {
		t.Fatalf("%d refund entries", got)
	}
	// An online collection is refunded, never voided.
	var se *usecase.StateError
	if _, err := f.fin.VoidCollection(f.ctx, f.org, c.UUID, model.CollectionVoidInput{Reason: "yanlış"}); !errors.As(err, &se) {
		t.Fatalf("void of an online collection: %v", err)
	}
}

// TestCashRefundAndVoid: a cash refund leaves the till and releases the
// newest allocation first; it needs the permission, a reason and the money
// in the till. A void releases everything and reverses the entry and the
// deposit; reversing a collection's entry is the same void.
func TestCashRefundAndVoid(t *testing.T) {
	f := newFx(t)
	r, res := f.rental(f.branch)
	f.handover(r)
	f.charge(r, "mileage", "200")
	acc := f.account()
	total := f.total(res.UUID)
	till := f.till("KASA", f.branch, "0")
	amount := total.Add(money.MustParse("200"))
	c := f.collect(model.CollectionCreateInput{RentalUUID: &r.UUID, Amount: &amount, Method: model.MethodCash, CashAccountUUID: &till.UUID})
	if c.AllocationStatus != model.AllocAllocated {
		t.Fatalf("collection %+v", c)
	}
	// Spend most of the till so a large refund cannot be paid out.
	other := f.bank("BANKA", "")
	_, err := f.fin.Transfer(f.ctx, f.org, model.TransferInput{FromAccountUUID: &till.UUID, ToAccountUUID: &other.UUID, Amount: &total})
	f.must(err)
	var ve *usecase.ValidationError
	if _, err := f.fin.RefundCollection(f.ctx, f.org, c.UUID, model.CollectionRefundInput{Amount: mp("250"), Reason: "fazla"}, ""); !errors.As(err, &ve) ||
		ve.Details[0].Code != "insufficient_funds" {
		t.Fatalf("refund above the till: %v", err)
	}
	if got, _ := f.fin.GetCollection(f.ctx, f.org, c.UUID); !got.RefundedAmount.IsZero() || !got.AllocatedAmount.Equal(amount) {
		t.Fatalf("a refused refund changed the collection %+v", got)
	}
	out, err := f.fin.RefundCollection(f.ctx, f.org, c.UUID, model.CollectionRefundInput{Amount: mp("150"), Reason: "km düzeltmesi"}, "")
	f.must(err)
	eq(t, "refunded", out.RefundedAmount, "150")
	eq(t, "till", f.cashBalance(till.UUID), "50")
	items := f.openItems(acc.UUID, &r.UUID)
	if item(items, model.PurposeRent).Status != model.ItemPaid || item(items, model.PurposeCharge).Status != model.ItemPartiallyPaid {
		t.Fatalf("after the refund %+v", items.Items)
	}
	if len(out.Refunds) != 1 || out.Refunds[0].Status != "succeeded" || out.Refunds[0].CashAccount == nil {
		t.Fatalf("refund row %+v", out.Refunds)
	}
	var se *usecase.StateError
	if _, err := f.fin.VoidCollection(f.ctx, f.org, c.UUID, model.CollectionVoidInput{Reason: "yanlış"}); !errors.As(err, &se) || se.Code != "not_voidable" {
		t.Fatalf("void after a refund: %v", err)
	}

	// Void a second collection; reverse a third through its entry.
	c2 := f.collect(model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: mp("90"), Method: model.MethodCash, CashAccountUUID: &till.UUID})
	noAdjust := f.scoped(nil, rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite)
	if _, err := f.fin.VoidCollection(noAdjust, f.org, c2.UUID, model.CollectionVoidInput{Reason: "yanlış kayıt"}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("void without ledger.adjust: %v", err)
	}
	c2, err = f.fin.VoidCollection(f.ctx, f.org, c2.UUID, model.CollectionVoidInput{Reason: "yanlış kayıt"})
	f.must(err)
	if c2.Status != model.CollectionVoid || !c2.AllocatedAmount.IsZero() {
		t.Fatalf("void %+v", c2)
	}
	eq(t, "till after the void", f.cashBalance(till.UUID), "50")
	c3 := f.collect(model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: mp("40"), Method: model.MethodCash, CashAccountUUID: &till.UUID})
	if _, err := f.fin.Reverse(f.ctx, f.org, c3.EntryUUID, model.ReverseInput{Reason: "mükerrer"}); err != nil {
		t.Fatalf("reverse the entry: %v", err)
	}
	if got, _ := f.fin.GetCollection(f.ctx, f.org, c3.UUID); got.Status != model.CollectionVoid {
		t.Fatalf("collection after its entry's reversal: %s", got.Status)
	}
	eq(t, "till after the reversal", f.cashBalance(till.UUID), "50")
	if got := item(f.openItems(acc.UUID, &r.UUID), model.PurposeCharge); !got.Open.Equal(money.MustParse("150")) {
		t.Fatalf("charge open %s", got.Open)
	}
	// An extension re-prices the rent: the allocation follows the new
	// accrual (released from the reversed one, allocated again).
	ends := res.EndsAt.Add(24 * 3600e9)
	if _, err := f.rentals.Extend(f.ctx, f.org, r.UUID, rentalmodel.ExtendInput{EndsAt: &ends, Reason: "uzatma"}); err != nil {
		t.Fatalf("extend: %v", err)
	}
	rent := item(f.openItems(acc.UUID, &r.UUID), model.PurposeRent)
	if !rent.Allocated.Equal(total) || rent.Status != model.ItemPartiallyPaid {
		t.Fatalf("rent after the extension %+v (total %s)", rent, total)
	}
}

// TestCollectionScope: a member of one branch sees and allocates that
// branch's collections only; another organization's collection, items and
// cash accounts are unknown.
func TestCollectionScope(t *testing.T) {
	f := newFx(t)
	r, _ := f.rental(f.branch)
	f.handover(r)
	acc := f.account()
	tillA := f.till("KASA-MRK", f.branch, "0")
	tillB := f.till("KASA-HVL", f.other, "0")
	cA := f.collect(model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: mp("10"), Method: model.MethodCash, CashAccountUUID: &tillA.UUID,
		BranchUUID: &f.branch.uuid, AllocationMode: model.AllocateNone})
	cB := f.collect(model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: mp("20"), Method: model.MethodCash, CashAccountUUID: &tillB.UUID,
		BranchUUID: &f.other.uuid, AllocationMode: model.AllocateNone})
	mrk := f.scoped([]int64{f.branch.id})
	items, _, err := f.fin.ListCollections(mrk, f.org, model.CollectionFilter{Limit: 50})
	f.must(err)
	if len(items) != 1 || items[0].UUID != cA.UUID {
		t.Fatalf("MRK member sees %d collections", len(items))
	}
	if _, err := f.fin.GetCollection(mrk, f.org, cB.UUID); !errors.Is(err, usecase.ErrCollectionNotFound) {
		t.Fatalf("HVL collection for MRK: %v", err)
	}
	if _, err := f.fin.Allocate(mrk, f.org, cB.UUID, model.AllocateInput{Mode: model.AllocateAuto}); !errors.Is(err, usecase.ErrCollectionNotFound) {
		t.Fatalf("allocate HVL collection as MRK: %v", err)
	}
	if _, err := f.fin.RefundCollection(mrk, f.org, cB.UUID, model.CollectionRefundInput{Reason: "deneme"}, ""); !errors.Is(err, usecase.ErrCollectionNotFound) {
		t.Fatalf("refund HVL collection as MRK: %v", err)
	}
	// A MRK member cannot book into the HVL till.
	if _, _, err := f.fin.CreateCollection(mrk, f.org, model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: mp("5"),
		Method: model.MethodCash, CashAccountUUID: &tillB.UUID, BranchUUID: &f.branch.uuid}); err == nil {
		t.Fatal("a MRK member booked into the HVL till")
	}

	// Another organization.
	g := newFx(t)
	gacc, _, err := g.fin.CreateAccount(g.ctx, g.org, model.AccountCreateInput{CustomerUUID: &g.customer.uuid})
	g.must(err)
	gtill := g.till("KASA", g.branch, "0")
	gc := g.collect(model.CollectionCreateInput{AccountUUID: &gacc.UUID, Amount: mp("30"), Method: model.MethodCash, CashAccountUUID: &gtill.UUID})
	if _, err := f.fin.GetCollection(f.ctx, f.org, gc.UUID); !errors.Is(err, usecase.ErrCollectionNotFound) {
		t.Fatalf("other organization's collection: %v", err)
	}
	if _, err := f.fin.VoidCollection(f.ctx, f.org, gc.UUID, model.CollectionVoidInput{Reason: "deneme"}); !errors.Is(err, usecase.ErrCollectionNotFound) {
		t.Fatalf("void another organization's collection: %v", err)
	}
	if _, err := f.fin.OpenItems(f.ctx, f.org, gacc.UUID, nil, false); !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Fatalf("open items of another organization's account: %v", err)
	}
	if _, _, err := f.fin.CreateCollection(f.ctx, f.org, model.CollectionCreateInput{AccountUUID: &acc.UUID, Amount: mp("5"),
		Method: model.MethodCash, CashAccountUUID: &gtill.UUID}); err == nil {
		t.Fatal("a collection into another organization's till")
	}
	if _, _, err := f.fin.CreateCollection(f.ctx, f.org, model.CollectionCreateInput{AccountUUID: &gacc.UUID, Amount: mp("5"),
		Method: model.MethodCash, CashAccountUUID: &tillA.UUID}); err == nil {
		t.Fatal("a collection on another organization's account")
	}
	rentItem := item(f.openItems(acc.UUID, nil), model.PurposeRent)
	if _, err := g.fin.Allocate(g.ctx, g.org, gc.UUID, model.AllocateInput{Mode: model.AllocateManual,
		Items: []model.AllocationItem{{EntryUUID: rentItem.EntryUUID}}}); err == nil {
		t.Fatal("allocated to another organization's open item")
	}
	list, _, err := g.fin.ListCollections(g.ctx, g.org, model.CollectionFilter{Limit: 100})
	g.must(err)
	for _, it := range list {
		if it.UUID == cA.UUID || it.UUID == cB.UUID {
			t.Fatal("another organization's collection listed")
		}
	}
}
