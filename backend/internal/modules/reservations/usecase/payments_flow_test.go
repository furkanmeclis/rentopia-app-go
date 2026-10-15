package usecase_test

// Acceptance tests of the M3 payment and approval flows (TEC-707 /
// TEC-708, docs/design/payments.md §8). They span reservations, payments
// and the iyzico adapter (against iyzicofake, no network), so they live
// next to the reservation fixture:
//
//   RT-A16  sandbox checkout + the same webhook delivered 3× → exactly one
//           payment, one confirmation, one job (deliveries = 3).
//   RT-A14  staff-approved online request: no stock → accept at current
//           price (hold + payment window, payment link) → paid → confirmed.
//   RT-A15  payment after the hold expired and the capacity was taken →
//           no confirmation, a late_payment resolution task; with the
//           capacity still free the reservation is reinstated (recorded).

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// payFx adds the payment stack to a reservation fixture.
type payFx struct {
	*fx
	fake     *iyzicofake.Fake
	accounts *accountsusecase.Service
	pay      *paymentsusecase.Service
	pctx     context.Context // caller with the payment / approval permissions
}

func newPayFx(t *testing.T) *payFx {
	t.Helper()
	f := newFx(t)
	fake := iyzicofake.New()
	t.Cleanup(fake.Server.Close)
	fake.AddAccount("sandbox-api-key-0001", "sandbox-secret-key-0001")
	fc, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatal(err)
	}
	vault, err := integrations.NewVault(fc)
	if err != nil {
		t.Fatal(err)
	}
	adapter := iyzico.New(iyzico.Config{SandboxBaseURL: fake.URL(), LiveBaseURL: "http://127.0.0.1:1"})
	accounts := accountsusecase.New(f.q, vault, integrations.NewRegistry(adapter))
	pay := paymentsusecase.New(f.pool, f.q, accounts, f.svc, paymentsusecase.Config{PublicBaseURL: "https://app.test"}, adapter)
	pay.SetClock(f.now)
	f.svc.SetPaymentLinks(pay)
	perms := append([]string{}, allPerms...)
	perms = append(perms, rbac.PermTenantReservationsApprove, rbac.PermTenantPaymentsRead, rbac.PermTenantPaymentsCreateLink,
		rbac.PermTenantPaymentRefund, rbac.PermTenantPaymentCollect, rbac.PermTenantIntegrationsManage)
	p := &payFx{fx: f, fake: fake, accounts: accounts, pay: pay, pctx: f.scoped(nil, perms...)}
	acc, err := accounts.Connect(p.pctx, f.org, accountsusecase.ConnectInput{
		Provider: iyzico.ProviderCode, DisplayName: "iyzico sandbox", Mode: integrations.ModeSandbox,
		Credentials: map[string]string{iyzico.FieldAPIKey: "sandbox-api-key-0001", iyzico.FieldSecretKey: "sandbox-secret-key-0001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if acc.CredentialsHint[iyzico.FieldSecretKey] == "sandbox-secret-key-0001" || acc.CredentialsHint[iyzico.FieldSecretKey] == "" {
		t.Fatalf("secret not masked: %v", acc.CredentialsHint)
	}
	if _, err := accounts.Test(p.pctx, f.org, acc.UUID); err != nil {
		t.Fatalf("connection test: %v", err)
	}
	return p
}

// checkout opens the hosted checkout of a link and returns the fake token.
func (p *payFx) checkout(link paymodel.Link) string {
	p.t.Helper()
	token := link.URL[strings.LastIndex(link.URL, "/")+1:]
	before := map[string]bool{}
	for _, tk := range p.fake.Tokens() {
		before[tk] = true
	}
	start, err := p.pay.StartCheckout(context.Background(), token, "10.0.0.7", "tr")
	if err != nil {
		p.t.Fatalf("checkout: %v", err)
	}
	if !strings.HasPrefix(start.PaymentPageURL, p.fake.URL()) {
		p.t.Fatalf("payment page %s", start.PaymentPageURL)
	}
	for _, tk := range p.fake.Tokens() {
		if !before[tk] {
			return tk
		}
	}
	p.t.Fatal("no checkout opened on the provider")
	return ""
}

func (p *payFx) deliver(body []byte, sig string) error {
	h := http.Header{}
	h.Set(iyzico.SignatureHeader, sig)
	_, err := p.pay.Webhook(context.Background(), iyzico.ProviderCode, h, body)
	return err
}

func (p *payFx) count(sql string, args ...any) int {
	p.t.Helper()
	var n int
	if err := p.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		p.t.Fatal(err)
	}
	return n
}

func (p *payFx) prepaymentSettings(mutate func(s *osmodel.Settings)) {
	p.configure(func(s *osmodel.Settings) {
		s.Reservation.PaymentHoldMinutes = intp(30)
		s.Reservation.StaffReviewWindowMinutes = intp(120)
		s.Reservation.QuoteValidityMinutes = intp(30)
		s.Payment.PrepaymentRule = osmodel.PrepaymentFixed
		s.Payment.PrepaymentValue = "500"
		if mutate != nil {
			mutate(s)
		}
	})
}

// RT-A16: the success / return page is not proof; the same notification
// delivered three times (twice concurrently) → one payment, one booking.
func TestRTA16WebhookReplayedThreeTimes(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(nil)
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserve(prod, c, d, p.at(0), p.at(48))
	if r.Status != model.StatusAwaitingPayment || r.ApprovalProtocol != model.ProtocolAuto {
		t.Fatalf("created %s / %s", r.Status, r.ApprovalProtocol)
	}
	link, err := p.pay.CreateLink(p.pctx, p.org, r.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !link.Amount.Equal(money.MustParse("500")) || link.Purpose != paymodel.PurposePrepayment || r.PaymentDeadline == nil ||
		!link.ExpiresAt.Equal(*r.PaymentDeadline) {
		t.Fatalf("link %+v (deadline %v)", link, r.PaymentDeadline)
	}
	token := link.URL[strings.LastIndex(link.URL, "/")+1:]
	fakeToken := p.checkout(link)
	// The customer returns from the provider without paying: the page
	// shows "processing", nothing is confirmed (the return is not proof).
	if pub, err := p.pay.PublicLink(context.Background(), token, true); err != nil || pub.State != paymodel.PublicProcessing {
		t.Fatalf("pay page before payment: %+v %v", pub, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.Status != model.StatusAwaitingPayment {
		t.Fatalf("confirmed without a verified payment: %s", got.Status)
	}
	body, sig, err := p.fake.Complete(fakeToken, iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	// A forged notification (wrong signature) changes nothing.
	if err := p.deliver(body, strings.Repeat("0", 64)); !errors.Is(err, paymentsusecase.ErrInvalidSignature) {
		t.Fatalf("forged webhook: %v", err)
	}
	retrievesBefore := p.fake.Retrieves.Load()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- p.deliver(body, sig)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("webhook: %v", err)
		}
	}
	if err := p.deliver(body, sig); err != nil {
		t.Fatalf("third webhook: %v", err)
	}
	if n := p.fake.Retrieves.Load() - retrievesBefore; n != 1 {
		t.Errorf("%d provider retrieves for one notification, want 1", n)
	}
	if n := p.count(`SELECT count(*) FROM payments WHERE organization_id = $1 AND status = 'succeeded' AND provider_payment_id IS NOT NULL`, p.org); n != 1 {
		t.Fatalf("%d succeeded payments, want 1", n)
	}
	if n := p.count(`SELECT count(*) FROM reservation_events e JOIN reservations r ON r.organization_id = e.organization_id AND r.id = e.reservation_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.action = 'confirm'`, p.org, r.UUID); n != 1 {
		t.Fatalf("%d confirmations, want 1", n)
	}
	if n := p.count(`SELECT deliveries FROM integration_jobs WHERE organization_id = $1 AND kind = 'payment.webhook'`, p.org); n != 3 {
		t.Fatalf("webhook job deliveries = %d, want 3", n)
	}
	got, _ := p.svc.Get(p.ctx, p.org, r.UUID)
	if got.Status != model.StatusConfirmed || got.CapacityCommitment != model.CommitmentFirm || got.PaymentStatus != model.PaymentPartially ||
		!got.Price.Paid.Equal(money.MustParse("500")) || !got.Price.PrepaymentDue.IsZero() || !got.Price.Balance.Equal(money.MustParse("2500")) {
		t.Fatalf("after payment: %s / %s / %s paid %s due %s balance %s", got.Status, got.CapacityCommitment, got.PaymentStatus,
			got.Price.Paid, got.Price.PrepaymentDue, got.Price.Balance)
	}
	if kind, status, _ := p.allocation(r.UUID); kind != "reservation" || status != "active" || p.activeAllocations() != 1 {
		t.Fatalf("allocation %s/%s, %d active", kind, status, p.activeAllocations())
	}
	items, _, err := p.pay.List(p.pctx, p.org, paymodel.ListFilter{ReservationUUIDs: []uuid.UUID{r.UUID}, Limit: 10})
	if err != nil || len(items) != 1 {
		t.Fatalf("payments of the reservation: %d %v", len(items), err)
	}
	pm := items[0]
	if pm.VerifiedVia != "webhook" || pm.Outcome != model.OutcomeConfirmed || !pm.GrossAmount.Equal(money.MustParse("500")) ||
		!pm.CommissionAmount.IsPositive() || !pm.NetAmount.Equal(pm.GrossAmount.Sub(pm.CommissionAmount)) {
		t.Fatalf("payment record %+v", pm)
	}
	if pub, _ := p.pay.PublicLink(context.Background(), token, true); pub.State != paymodel.PublicPaid || pub.Result != model.OutcomeConfirmed {
		t.Fatalf("pay page after payment: %+v", pub)
	}
	// Partial refund through the provider; the reservation recounts.
	refunded, err := p.pay.Refund(p.pctx, p.org, pm.UUID, paymodel.RefundInput{Amount: moneyPtr("200"), Reason: "müşteri talebi"}, "10.0.0.1")
	if err != nil || refunded.Status != paymodel.StatusPartiallyRefunded || !refunded.RefundedAmount.Equal(money.MustParse("200")) || len(refunded.Refunds) != 1 {
		t.Fatalf("refund: %+v %v", refunded, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); !got.Price.Paid.Equal(money.MustParse("300")) {
		t.Fatalf("paid after refund %s, want 300", got.Price.Paid)
	}
	if _, err := p.pay.Refund(p.pctx, p.org, pm.UUID, paymodel.RefundInput{Amount: moneyPtr("400"), Reason: "fazla"}, ""); err == nil {
		t.Fatal("refund above the refundable amount accepted")
	}
	// Branch collection of the remaining rent.
	col, err := p.pay.Collect(p.pctx, p.org, r.UUID, paymodel.CollectInput{Method: paymodel.MethodCash})
	if err != nil || !col.Amount.Equal(money.MustParse("2700")) {
		t.Fatalf("collect: %+v %v", col, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.PaymentStatus != model.PaymentPaid || !got.Price.Balance.IsZero() {
		t.Fatalf("after collection: %s balance %s", got.PaymentStatus, got.Price.Balance)
	}
}

func moneyPtr(s string) *money.Money {
	v := money.MustParse(s)
	return &v
}

// A lost webhook: the reconciliation sweep retrieves the checkout.
func TestReconcilePendingRecoversLostWebhook(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(nil)
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserve(prod, c, d, p.at(0), p.at(24))
	link, err := p.pay.CreateLink(p.pctx, p.org, r.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	fakeToken := p.checkout(link)
	if _, _, err := p.fake.Complete(fakeToken, iyzicofake.StatePaid); err != nil {
		t.Fatal(err)
	}
	if n, err := p.pay.ReconcilePending(context.Background()); err != nil || n != 0 {
		t.Fatalf("sweep inside the grace period: %d %v", n, err)
	}
	p.advance(3 * time.Minute)
	if n, err := p.pay.ReconcilePending(context.Background()); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.Status != model.StatusConfirmed {
		t.Fatalf("after the sweep: %s", got.Status)
	}
}

// A provider answer that does not match the payment (amount) is never
// recorded as a success.
func TestVerificationMismatchIsRefused(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(nil)
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserve(prod, c, d, p.at(0), p.at(24))
	link, err := p.pay.CreateLink(p.pctx, p.org, r.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	fakeToken := p.checkout(link)
	// The amount of the payment row is tampered with after the checkout.
	if _, err := p.pool.Exec(context.Background(), `UPDATE payments SET amount = 900, gross_amount = 900 WHERE organization_id = $1`, p.org); err != nil {
		t.Fatal(err)
	}
	body, sig, _ := p.fake.Complete(fakeToken, iyzicofake.StatePaid)
	if err := p.deliver(body, sig); err != nil {
		t.Fatalf("webhook: %v", err)
	}
	if n := p.count(`SELECT count(*) FROM payments WHERE organization_id = $1 AND status = 'failed' AND failure_reason LIKE 'verification_mismatch%'`, p.org); n != 1 {
		t.Fatal("mismatching provider answer was not refused")
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.Status != model.StatusAwaitingPayment {
		t.Fatalf("confirmed on a mismatching payment: %s", got.Status)
	}
}

func (p *payFx) onlineSettings() {
	p.prepaymentSettings(func(s *osmodel.Settings) {
		s.Reservation.ApprovalMode = osmodel.ApprovalStaff
		s.Reservation.PaymentHoldMinutes = intp(45)
		s.Payment.PrepaymentRule = osmodel.PrepaymentPercent
		s.Payment.PrepaymentValue = "20"
		for i := range s.SalesChannels {
			s.SalesChannels[i].IsActive = true
		}
	})
}

func (p *payFx) reserveOnline(prod, c, d ref, from, to time.Time) model.Reservation {
	p.t.Helper()
	q, err := p.svc.CreateQuote(p.ctx, p.org, model.QuoteInput{Where: p.where(from, to), Channel: "web", ProductUUID: &prod.uuid})
	if err != nil {
		p.t.Fatalf("quote: %v", err)
	}
	r, err := p.svc.Create(p.ctx, p.org, model.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &c.uuid, Drivers: []model.DriverInput{{DriverUUID: d.uuid}}})
	if err != nil {
		p.t.Fatalf("create: %v", err)
	}
	return r
}

// RT-A14: staff-approved flow end to end.
func TestRTA14StaffApprovedFlow(t *testing.T) {
	p := newPayFx(t)
	p.onlineSettings()
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserveOnline(prod, c, d, p.at(0), p.at(24))
	// Request: no stock, SLA from the review window ("değerlendirme bekliyor").
	if r.Status != model.StatusRequest || r.CapacityCommitment != model.CommitmentNone || r.ApprovalProtocol != model.ProtocolStaff ||
		r.ReviewDeadline == nil || p.activeAllocations() != 0 || !r.Actions.Approve || !r.Actions.Reject {
		t.Fatalf("request: %s / %s / %s deadline %v, %d allocations", r.Status, r.CapacityCommitment, r.ApprovalProtocol,
			r.ReviewDeadline, p.activeAllocations())
	}
	if want := p.now().Add(120 * time.Minute); r.ReviewDeadline.Sub(want) > time.Minute || want.Sub(*r.ReviewDeadline) > time.Minute {
		t.Fatalf("review deadline %v, want ~%v", r.ReviewDeadline, want)
	}
	queue, _, err := p.svc.List(p.ctx, p.org, model.ListFilter{Statuses: []string{model.StatusRequest, model.StatusAwaitingReview}, SortField: "review_deadline", Limit: 10})
	if err != nil || len(queue) != 1 || queue[0].Deadline == nil || queue[0].ReviewDeadline == nil {
		t.Fatalf("approval queue: %+v %v", queue, err)
	}
	// Payment link creation needs the permission.
	noLink := p.scoped(nil, rbac.PermTenantReservationsRead, rbac.PermTenantReservationsApprove)
	if _, err := p.svc.Accept(noLink, p.org, r.UUID, model.AcceptInput{SendPaymentLink: true}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("accept with link without create_link: %v", err)
	}
	// The price moved since the request: accept answers with a fresh quote.
	p.plan(prod, "branch", p.mrk.id, "1600", "1400", "1100") // a branch price list now
	_, err = p.svc.Accept(p.pctx, p.org, r.UUID, model.AcceptInput{SendPaymentLink: true})
	var qe *usecase.QuoteError
	if !errors.As(err, &qe) || qe.Code != "QUOTE_PRICE_CHANGED" || qe.Replacement == nil || !qe.Replacement.Total.Equal(money.MustParse("1600")) {
		t.Fatalf("accept after a price change: %v", err)
	}
	if p.activeAllocations() != 0 {
		t.Fatal("a refused accept held capacity")
	}
	out, err := p.svc.Accept(p.pctx, p.org, r.UUID, model.AcceptInput{QuoteUUID: &qe.Replacement.UUID, SendPaymentLink: true, Note: "fiyat güncel"})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	a := out.Reservation
	if a.Status != model.StatusAwaitingPayment || a.CapacityCommitment != model.CommitmentHold || a.PaymentDeadline == nil ||
		a.Plan.HoldExpiresAt == nil || !a.PaymentDeadline.Equal(*a.Plan.HoldExpiresAt) || !a.Price.Total.Equal(money.MustParse("1600")) ||
		!a.Price.Prepayment.Equal(money.MustParse("320")) || a.ReviewDeadline != nil {
		t.Fatalf("accepted: %s / %s deadline %v hold %v total %s prepayment %s", a.Status, a.CapacityCommitment, a.PaymentDeadline,
			a.Plan.HoldExpiresAt, a.Price.Total, a.Price.Prepayment)
	}
	if out.PaymentLink == nil || !out.PaymentLink.Amount.Equal(money.MustParse("320")) || !out.PaymentLink.ExpiresAt.Equal(*a.PaymentDeadline) {
		t.Fatalf("payment link %+v (%s)", out.PaymentLink, out.PaymentLinkError)
	}
	link, err := p.pay.GetLink(p.pctx, p.org, out.PaymentLink.UUID)
	if err != nil {
		t.Fatal(err)
	}
	body, sig, err := p.fake.Complete(p.checkout(link), iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.deliver(body, sig); err != nil {
		t.Fatal(err)
	}
	got, _ := p.svc.Get(p.ctx, p.org, r.UUID)
	if got.Status != model.StatusConfirmed || got.CapacityCommitment != model.CommitmentFirm || got.PaymentStatus != model.PaymentPartially ||
		!got.Price.Balance.Equal(money.MustParse("1280")) {
		t.Fatalf("after payment: %s / %s / %s balance %s", got.Status, got.CapacityCommitment, got.PaymentStatus, got.Price.Balance)
	}
	ev := p.events(r.UUID)
	if !containsAll(ev, "created", "approve", "payment_link_created", "payment_recorded", "confirm") {
		t.Fatalf("timeline %v", ev)
	}
	// A second request is rejected with a reason.
	r2 := p.reserveOnline(prod, c, d, p.at(100), p.at(120))
	if _, err := p.svc.Reject(p.pctx, p.org, r2.UUID, ""); err == nil {
		t.Fatal("reject without a reason accepted")
	}
	rj, err := p.svc.Reject(p.pctx, p.org, r2.UUID, "araç uygun değil")
	if err != nil || rj.Status != model.StatusCancelled || rj.CancelReason != "araç uygun değil" {
		t.Fatalf("reject: %+v %v", rj.Status, err)
	}
	// The branch counter keeps reviewing on an option (awaiting_review).
	r3 := p.reserve(prod, c, d, p.at(200), p.at(220))
	if r3.Status != model.StatusAwaitingReview || r3.CapacityCommitment != model.CommitmentHold {
		t.Fatalf("branch sale in staff mode: %s / %s", r3.Status, r3.CapacityCommitment)
	}
}

// Channel reservations follow the channel's protocol, not the online
// approval mode.
func TestChannelProtocolOverridesApprovalMode(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(func(s *osmodel.Settings) {
		s.Reservation.ApprovalMode = osmodel.ApprovalStaff
		for i := range s.SalesChannels {
			s.SalesChannels[i].IsActive = true
			if s.SalesChannels[i].Code == "web" {
				s.SalesChannels[i].ApprovalProtocol = osmodel.ProtocolChannel
			}
		}
	})
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserveOnline(prod, c, d, p.at(0), p.at(24))
	if r.Status != model.StatusConfirmed || r.ApprovalProtocol != model.ProtocolChannel || r.CapacityCommitment != model.CommitmentFirm {
		t.Fatalf("channel-confirmed reservation: %s / %s / %s", r.Status, r.ApprovalProtocol, r.CapacityCommitment)
	}
}

func containsAll(list []string, want ...string) bool {
	set := map[string]bool{}
	for _, v := range list {
		set[v] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// RT-A15: a successful payment after the hold expired, with the capacity
// taken by another sale → no confirmation, a late_payment task.
func TestRTA15LatePaymentCapacityGone(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(nil)
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk) // the only car
	c, d := p.customer(30, 5)
	r := p.reserve(prod, c, d, p.at(0), p.at(24))
	link, err := p.pay.CreateLink(p.pctx, p.org, r.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	fakeToken := p.checkout(link) // the customer is on the provider page …
	p.advance(31 * time.Minute)   // … while the payment window elapses
	if n, err := p.svc.ExpireDue(context.Background()); err != nil || n < 1 {
		t.Fatalf("expiry sweep: %d %v", n, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.Status != model.StatusExpired {
		t.Fatalf("after the sweep: %s", got.Status)
	}
	if p.activeAllocations() != 0 {
		t.Fatal("expired hold still holds capacity")
	}
	other := p.reserve(prod, c, d, p.at(0), p.at(24)) // the car is sold again
	if other.Status != model.StatusAwaitingPayment || p.activeAllocations() != 1 {
		t.Fatalf("second sale: %s", other.Status)
	}
	body, sig, _ := p.fake.Complete(fakeToken, iyzicofake.StatePaid)
	if err := p.deliver(body, sig); err != nil {
		t.Fatalf("webhook: %v", err)
	}
	got, _ := p.svc.Get(p.ctx, p.org, r.UUID)
	if got.Status != model.StatusExpired || got.CapacityCommitment != model.CommitmentNone || !got.Price.Paid.Equal(money.MustParse("500")) {
		t.Fatalf("late payment: %s / %s paid %s (must not be confirmed)", got.Status, got.CapacityCommitment, got.Price.Paid)
	}
	if p.activeAllocations() != 1 {
		t.Fatal("the other sale lost its capacity")
	}
	tasks, _, err := p.svc.ListTasks(p.ctx, p.org, model.TaskFilter{ReservationUUIDs: []uuid.UUID{r.UUID}, TaskTypes: []string{model.TaskLatePayment}, Limit: 10})
	if err != nil || len(tasks) != 1 || tasks[0].Status != model.TaskOpen {
		t.Fatalf("late payment task: %+v %v", tasks, err)
	}
	items, _, _ := p.pay.List(p.pctx, p.org, paymodel.ListFilter{ReservationUUIDs: []uuid.UUID{r.UUID}, Limit: 10})
	if len(items) != 1 || items[0].Status != paymodel.StatusSucceeded || items[0].Outcome != model.OutcomeLateTask {
		t.Fatalf("payment %+v", items)
	}
	// The notification again: still one task, one payment.
	if err := p.deliver(body, sig); err != nil {
		t.Fatal(err)
	}
	if n := p.count(`SELECT count(*) FROM resolution_tasks WHERE organization_id = $1 AND task_type = 'late_payment'`, p.org); n != 1 {
		t.Fatalf("%d late payment tasks, want 1", n)
	}
	// The refund closes the money side.
	if _, err := p.pay.Refund(p.pctx, p.org, items[0].UUID, paymodel.RefundInput{Reason: "geç ödeme iadesi"}, ""); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.PaymentStatus != model.PaymentRefunded {
		t.Fatalf("after refund: %s", got.PaymentStatus)
	}
}

// A late payment with the capacity still free reinstates the reservation
// (recorded on the timeline), never silently.
func TestLatePaymentReinstatesWhenCapacityFree(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(nil)
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk)
	c, d := p.customer(30, 5)
	r := p.reserve(prod, c, d, p.at(0), p.at(24))
	link, err := p.pay.CreateLink(p.pctx, p.org, r.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	fakeToken := p.checkout(link)
	p.advance(31 * time.Minute)
	// No sweep yet: the option lapsed but the reservation still waits;
	// the payment finds it expired-in-place and re-validates.
	body, sig, _ := p.fake.Complete(fakeToken, iyzicofake.StatePaid)
	if err := p.deliver(body, sig); err != nil {
		t.Fatalf("webhook: %v", err)
	}
	got, _ := p.svc.Get(p.ctx, p.org, r.UUID)
	if got.Status != model.StatusConfirmed || got.CapacityCommitment != model.CommitmentFirm || p.activeAllocations() != 1 {
		t.Fatalf("reinstated: %s / %s, %d active", got.Status, got.CapacityCommitment, p.activeAllocations())
	}
	if ev := p.events(r.UUID); !containsAll(ev, "expire", "reinstate") {
		t.Fatalf("timeline %v", ev)
	}
	items, _, _ := p.pay.List(p.pctx, p.org, paymodel.ListFilter{ReservationUUIDs: []uuid.UUID{r.UUID}, Limit: 10})
	if len(items) != 1 || items[0].Outcome != model.OutcomeReinstated {
		t.Fatalf("payment %+v", items)
	}
}
