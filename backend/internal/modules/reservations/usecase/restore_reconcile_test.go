package usecase_test

// TEC-962: payments and refunds made after the backup a database was
// restored from (docs/operations/backup-restore.md RT-A32 (c)).

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// paidOnline pays a reservation's prepayment link on the fake and
// delivers the webhook; returns the reservation, the confirmed payment and
// the fake checkout token.
func (p *payFx) paidOnline(t *testing.T) (model.Reservation, paymodel.Payment, string) {
	t.Helper()
	p.prepaymentSettings(nil)
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserve(prod, c, d, p.at(0), p.at(48))
	link, err := p.pay.CreateLink(p.pctx, p.org, r.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	tk := p.checkout(link)
	body, sig, err := p.fake.Complete(tk, iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.deliver(body, sig); err != nil {
		t.Fatal(err)
	}
	items, _, err := p.pay.List(p.pctx, p.org, paymodel.ListFilter{ReservationUUIDs: []uuid.UUID{r.UUID}, Limit: 10})
	if err != nil || len(items) != 1 || items[0].Status != paymodel.StatusSucceeded {
		t.Fatalf("payment: %+v %v", items, err)
	}
	return r, items[0], tk
}

// A refund made on the provider after the backup is missing from the
// restored ledger: the guard refuses to refund again, opens a
// reconciliation item; recording the provider's refund brings the ledger
// in line and later refunds work again.
func TestRefundGuardAfterRestore(t *testing.T) {
	p := newPayFx(t)
	r, pm, _ := p.paidOnline(t)
	providerID := *pm.ProviderPaymentID
	if err := p.fake.RefundOutside(providerID, money.MustParse("200")); err != nil {
		t.Fatal(err)
	}
	before := p.fake.Refunds.Load()
	_, err := p.pay.Refund(p.pctx, p.org, pm.UUID, paymodel.RefundInput{Amount: moneyPtr("200"), Reason: "müşteri talebi"}, "10.0.0.1")
	var ge *paymentsusecase.RefundGuardError
	if !errors.As(err, &ge) || ge.Code != paymentsusecase.CodeRefundStateMismatch || ge.ItemUUID == nil ||
		!ge.ProviderRefunded.Equal(money.MustParse("200")) || !ge.OurRefunded.IsZero() {
		t.Fatalf("refund after a restore: %v", err)
	}
	if n := p.fake.Refunds.Load() - before; n != 0 {
		t.Fatalf("%d refund calls reached the provider", n)
	}
	if n := p.count(`SELECT count(*) FROM payment_refunds WHERE organization_id = $1`, p.org); n != 0 {
		t.Fatalf("%d refund rows recorded for a refused refund", n)
	}
	// A second attempt reuses the open item.
	if _, err := p.pay.Refund(p.pctx, p.org, pm.UUID, paymodel.RefundInput{Amount: moneyPtr("100"), Reason: "tekrar"}, ""); !errors.As(err, &ge) {
		t.Fatalf("second refund: %v", err)
	}
	if n := p.count(`SELECT count(*) FROM payment_reconciliation_items WHERE organization_id = $1 AND status = 'open'`, p.org); n != 1 {
		t.Fatalf("%d open items, want 1", n)
	}
	// The report shows the gap.
	rep, err := p.pay.Reconcile(p.pctx, p.org, p.now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Payments) != 1 || !containsAll(rep.Payments[0].Mismatches, "refund_missing_in_ledger") || len(rep.Items) != 1 ||
		rep.Payments[0].ProviderRefunded == nil || !rep.Payments[0].ProviderRefunded.Equal(money.MustParse("200")) {
		t.Fatalf("report: %+v", rep)
	}
	// Record the provider's refund: no provider call, ledger in line.
	got, err := p.pay.ResolveItem(p.pctx, p.org, *ge.ItemUUID, paymodel.ResolveItemInput{
		Resolution: paymodel.ResolutionRecordRefund, Note: "iyzico panelinde görüldü",
	})
	if err != nil || !got.RefundedAmount.Equal(money.MustParse("200")) || got.Status != paymodel.StatusPartiallyRefunded {
		t.Fatalf("record refund: %+v %v", got, err)
	}
	if n := p.fake.Refunds.Load() - before; n != 0 {
		t.Fatalf("recording called the provider %d times", n)
	}
	if res, _ := p.svc.Get(p.ctx, p.org, r.UUID); !res.Price.Paid.Equal(money.MustParse("300")) {
		t.Fatalf("reservation paid %s after recording, want 300", res.Price.Paid)
	}
	if _, err := p.pay.ResolveItem(p.pctx, p.org, *ge.ItemUUID, paymodel.ResolveItemInput{Resolution: paymodel.ResolutionDismiss, Note: "again"}); err == nil {
		t.Fatal("a resolved item resolved again")
	}
	// In line again: the next refund goes through.
	got, err = p.pay.Refund(p.pctx, p.org, pm.UUID, paymodel.RefundInput{Amount: moneyPtr("100"), Reason: "kalan iade"}, "")
	if err != nil || !got.RefundedAmount.Equal(money.MustParse("300")) {
		t.Fatalf("refund after reconciliation: %+v %v", got, err)
	}
	// The provider's refunds unreadable: the refund is not blocked (as
	// before the guard), an "unverified" item is kept for staff (one per
	// payment).
	p.fake.FailReports.Store(true)
	sent := p.fake.Refunds.Load()
	got, err = p.pay.Refund(p.pctx, p.org, pm.UUID, paymodel.RefundInput{Amount: moneyPtr("50"), Reason: "son"}, "")
	if err != nil || !got.RefundedAmount.Equal(money.MustParse("350")) || p.fake.Refunds.Load()-sent != 1 {
		t.Fatalf("refund with the provider's refunds unreadable: %+v %v", got, err)
	}
	if _, err := p.pay.Refund(p.pctx, p.org, pm.UUID, paymodel.RefundInput{Amount: moneyPtr("50"), Reason: "son 2"}, ""); err != nil {
		t.Fatalf("second unverified refund: %v", err)
	}
	if n := p.count(`SELECT count(*) FROM payment_reconciliation_items WHERE organization_id = $1 AND kind = 'unverified' AND status = 'open'`, p.org); n != 1 {
		t.Fatalf("%d open unverified items, want 1", n)
	}
}

// A customer paid after the backup: the restored database has no payment
// row, the webhook keeps the notification as an orphan (signature verified
// with the organization's account); staff match it to the reservation and
// the payment is confirmed once through the usual verification.
func TestOrphanWebhookKeptAndMatched(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(nil)
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserve(prod, c, d, p.at(0), p.at(48))
	link, err := p.pay.CreateLink(p.pctx, p.org, r.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	tk := p.checkout(link)
	body, sig, err := p.fake.Complete(tk, iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	// The restore: the checkout's payment row is gone.
	if _, err := p.pool.Exec(context.Background(), "DELETE FROM payments WHERE organization_id = $1", p.org); err != nil {
		t.Fatal(err)
	}
	h := http.Header{}
	h.Set(iyzico.SignatureHeader, sig)
	for i := 0; i < 2; i++ {
		res, err := p.pay.Webhook(context.Background(), iyzico.ProviderCode, h, body)
		if err != nil || !res.Orphan || res.Status != paymodel.OrphanUnmatched || res.Duplicate != (i == 1) {
			t.Fatalf("delivery %d: %+v %v", i, res, err)
		}
	}
	// A forged notification for an unknown checkout stays platform-level.
	forged := strings.ReplaceAll(string(body), tk, "fake-token-forged")
	t.Cleanup(func() {
		_, _ = p.pool.Exec(context.Background(), "DELETE FROM payment_webhook_orphans WHERE organization_id IS NULL AND provider_token = 'fake-token-forged'")
	})
	if _, err := p.pay.Webhook(context.Background(), iyzico.ProviderCode, h, []byte(forged)); !errors.Is(err, paymentsusecase.ErrUnknownPayment) {
		t.Fatalf("forged orphan: %v", err)
	}
	if n := p.count(`SELECT count(*) FROM payment_webhook_orphans WHERE organization_id IS NULL AND provider_token = 'fake-token-forged'`); n != 1 {
		t.Fatalf("%d platform-level orphans for the forged notification, want 1", n)
	}
	orphans, total, err := p.pay.ListOrphans(p.pctx, p.org, paymodel.OrphanFilter{Limit: 10, SortDesc: true})
	if err != nil || total != 1 || orphans[0].Deliveries != 2 || !orphans[0].SignatureVerified || orphans[0].Status != paymodel.OrphanUnmatched {
		t.Fatalf("orphans: %+v %d %v", orphans, total, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.Status != model.StatusAwaitingPayment {
		t.Fatalf("an orphan confirmed the reservation: %s", got.Status)
	}
	// Validation, then the match.
	if _, err := p.pay.MatchOrphan(p.pctx, p.org, orphans[0].UUID, paymodel.MatchOrphanInput{ReservationUUID: &r.UUID}); err == nil {
		t.Fatal("match without a note accepted")
	}
	pm, err := p.pay.MatchOrphan(p.pctx, p.org, orphans[0].UUID, paymodel.MatchOrphanInput{ReservationUUID: &r.UUID, Note: "geri yükleme sonrası"})
	if err != nil || pm.Status != paymodel.StatusSucceeded || !pm.Amount.Equal(money.MustParse("500")) || pm.Reservation.UUID != r.UUID {
		t.Fatalf("match: %+v %v", pm, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.Status != model.StatusConfirmed || !got.Price.Paid.Equal(money.MustParse("500")) {
		t.Fatalf("after match: %s paid %s", got.Status, got.Price.Paid)
	}
	if _, err := p.pay.MatchOrphan(p.pctx, p.org, orphans[0].UUID, paymodel.MatchOrphanInput{ReservationUUID: &r.UUID, Note: "tekrar"}); err == nil {
		t.Fatal("an orphan matched twice")
	}
	// The provider re-delivers: now a known payment, nothing doubles.
	if res, err := p.pay.Webhook(context.Background(), iyzico.ProviderCode, h, body); err != nil || res.Orphan {
		t.Fatalf("re-delivery after the match: %+v %v", res, err)
	}
	if n := p.count(`SELECT count(*) FROM payments WHERE organization_id = $1 AND status = 'succeeded'`, p.org); n != 1 {
		t.Fatalf("%d succeeded payments, want 1", n)
	}
	// Another organization cannot see or match it.
	other := newPayFx(t)
	if _, err := other.pay.MatchOrphan(other.pctx, other.org, orphans[0].UUID, paymodel.MatchOrphanInput{ReservationUUID: &r.UUID, Note: "nope"}); !errors.Is(err, paymentsusecase.ErrOrphanNotFound) {
		t.Fatalf("match from another organization: %v", err)
	}
}
