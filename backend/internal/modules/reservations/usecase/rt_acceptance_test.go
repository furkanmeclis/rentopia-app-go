package usecase_test

// RT-A14 and RT-A16 completions (TEC-708 / TEC-707). The staff-approved
// flow, late payments and the triple webhook are in payments_flow_test.go
// (TestRTA14StaffApprovedFlow, TestRTA15LatePaymentCapacityGone,
// TestRTA16WebhookReplayedThreeTimes); these add the automatic approval
// flow end to end and the per-field verification of a notification.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
)

// TestRT_A14_AutomaticApprovalFlow: automatic approval with a prepayment
// rule. Availability is validated before the hold (a second customer is
// refused while the only car is held), the hold carries the payment window,
// an expired hold gives the capacity back, and a verified payment of the
// next customer confirms the reservation (firm capacity).
func TestRT_A14_AutomaticApprovalFlow(t *testing.T) {
	p := newPayFx(t)
	p.prepaymentSettings(nil) // auto approval, 30 min payment hold, 500 TL prepayment
	prod := p.product("EKO", 21)
	p.vehicle(p.mrk) // the only car
	c1, d1 := p.customer(30, 5)
	c2, d2 := p.customer(35, 8)

	// 1. Customer 1: availability validated → timed hold.
	r1 := p.reserve(prod, c1, d1, p.at(0), p.at(48))
	if r1.Status != model.StatusAwaitingPayment || r1.ApprovalProtocol != model.ProtocolAuto ||
		r1.CapacityCommitment != model.CommitmentHold || r1.PaymentDeadline == nil {
		t.Fatalf("RT-A14 hold: %s / %s / %s deadline %v", r1.Status, r1.ApprovalProtocol, r1.CapacityCommitment, r1.PaymentDeadline)
	}
	if want := p.now().Add(30 * time.Minute); r1.PaymentDeadline.Sub(want).Abs() > time.Minute {
		t.Fatalf("RT-A14 payment deadline %v, want ~%v", r1.PaymentDeadline, want)
	}
	if kind, status, _ := p.allocation(r1.UUID); kind != "hold" || status != "active" {
		t.Fatalf("RT-A14 allocation %s/%s, want an active hold", kind, status)
	}

	// 2. Customer 2 for an overlapping range: refused on availability, no
	// second allocation.
	q2 := p.quote(p.ctx, p.svc, prod, p.at(10), p.at(30))
	_, err := p.svc.Create(p.ctx, p.org, model.CreateInput{QuoteUUID: &q2.UUID, CustomerUUID: &c2.uuid, Drivers: []model.DriverInput{{DriverUUID: d2.uuid}}})
	if _, ok := availusecase.IsCapacity(err); !ok {
		t.Fatalf("RT-A14: a sale over a held car = %v, want CAPACITY_UNAVAILABLE", err)
	}
	if p.activeAllocations() != 1 {
		t.Fatalf("RT-A14: %d active allocations after a refused sale", p.activeAllocations())
	}

	// 3. The payment window elapses: the sweep expires the hold and frees
	// the capacity.
	p.advance(31 * time.Minute)
	if n, err := p.svc.ExpireDue(context.Background()); err != nil || n < 1 {
		t.Fatalf("RT-A14 expiry sweep: %d %v", n, err)
	}
	if got, _ := p.svc.Get(p.ctx, p.org, r1.UUID); got.Status != model.StatusExpired || got.CapacityCommitment != model.CommitmentNone {
		t.Fatalf("RT-A14 after expiry: %s / %s", got.Status, got.CapacityCommitment)
	}
	if p.activeAllocations() != 0 {
		t.Fatal("RT-A14: an expired hold still holds capacity")
	}

	// 4. Customer 2 again: hold → verified payment → confirmed (firm).
	r2 := p.reserve(prod, c2, d2, p.at(10), p.at(30))
	if r2.Status != model.StatusAwaitingPayment || r2.CapacityCommitment != model.CommitmentHold {
		t.Fatalf("RT-A14 second hold: %s / %s", r2.Status, r2.CapacityCommitment)
	}
	link, err := p.pay.CreateLink(p.pctx, p.org, r2.UUID, paymodel.CreateLinkInput{})
	if err != nil {
		t.Fatal(err)
	}
	body, sig, err := p.fake.Complete(p.checkout(link), iyzicofake.StatePaid)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.deliver(body, sig); err != nil {
		t.Fatalf("RT-A14 webhook: %v", err)
	}
	got, _ := p.svc.Get(p.ctx, p.org, r2.UUID)
	if got.Status != model.StatusConfirmed || got.CapacityCommitment != model.CommitmentFirm {
		t.Fatalf("RT-A14 after payment: %s / %s", got.Status, got.CapacityCommitment)
	}
	if kind, status, _ := p.allocation(r2.UUID); kind != "reservation" || status != "active" || p.activeAllocations() != 1 {
		t.Fatalf("RT-A14 allocation after payment %s/%s (%d active)", kind, status, p.activeAllocations())
	}
	if ev := p.events(r2.UUID); !containsAll(ev, "created", "payment_recorded", "confirm") {
		t.Fatalf("RT-A14 timeline %v", ev)
	}
}

// TestRT_A16_WebhookVerifiesEveryField: a notification is matched to the
// payment by its order reference, amount, currency and the organization's
// own secret. Each mismatch leaves the reservation unconfirmed and no
// succeeded payment (amount: TestVerificationMismatchIsRefused).
func TestRT_A16_WebhookVerifiesEveryField(t *testing.T) {
	setup := func(t *testing.T) (*payFx, model.Reservation, string) {
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
		return p, r, p.checkout(link)
	}
	unconfirmed := func(t *testing.T, p *payFx, r model.Reservation) {
		t.Helper()
		if got, _ := p.svc.Get(p.ctx, p.org, r.UUID); got.Status != model.StatusAwaitingPayment {
			t.Fatalf("RT-A16: confirmed on a refused notification: %s", got.Status)
		}
		if n := p.count(`SELECT count(*) FROM payments WHERE organization_id = $1 AND status = 'succeeded'`, p.org); n != 0 {
			t.Fatalf("RT-A16: %d succeeded payments on a refused notification", n)
		}
	}
	for _, tc := range []struct {
		name, tamper, problem string
	}{
		{"currency", `UPDATE payments SET currency = 'USD' WHERE organization_id = $1`, "currency"},
		{"order_reference", `UPDATE payments SET uuid = gen_random_uuid() WHERE organization_id = $1`, "order reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r, token := setup(t)
			if _, err := p.pool.Exec(context.Background(), tc.tamper, p.org); err != nil {
				t.Fatal(err)
			}
			body, sig, err := p.fake.Complete(token, iyzicofake.StatePaid)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.deliver(body, sig); err != nil {
				t.Fatalf("webhook: %v", err)
			}
			if n := p.count(`SELECT count(*) FROM payments WHERE organization_id = $1 AND status = 'failed'
				AND failure_reason LIKE 'verification_mismatch%' AND failure_reason LIKE '%' || $2 || '%'`, p.org, tc.problem); n != 1 {
				t.Fatalf("RT-A16: %s mismatch not refused", tc.problem)
			}
			unconfirmed(t, p, r)
		})
	}
	t.Run("organization_secret", func(t *testing.T) {
		p, r, token := setup(t)
		body, _, err := p.fake.Complete(token, iyzicofake.StatePaid)
		if err != nil {
			t.Fatal(err)
		}
		// The same notification signed with another merchant's (another
		// organization's) secret key.
		var raw struct {
			EventType      string `json:"iyziEventType"`
			PaymentID      string `json:"iyziPaymentId"`
			Token          string `json:"token"`
			ConversationID string `json:"paymentConversationId"`
			Status         string `json:"status"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatal(err)
		}
		foreign := iyzico.WebhookSignature("sandbox-secret-key-OTHER", payments.Webhook{
			EventType: raw.EventType, PaymentID: raw.PaymentID, Token: raw.Token, ConversationID: raw.ConversationID, Status: raw.Status,
		})
		if err := p.deliver(body, foreign); !errors.Is(err, paymentsusecase.ErrInvalidSignature) {
			t.Fatalf("RT-A16: notification signed with another organization's secret = %v", err)
		}
		// An unknown order reference (conversation id of no payment).
		unknown := strings.Replace(string(body), raw.ConversationID, "00000000-0000-0000-0000-000000000000", 1)
		if err := p.deliver([]byte(unknown), foreign); err == nil {
			t.Fatal("RT-A16: a notification of an unknown order was accepted")
		}
		unconfirmed(t, p, r)
	})
}
