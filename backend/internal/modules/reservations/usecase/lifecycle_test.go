package usecase_test

import (
	"errors"
	"testing"
	"time"

	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func stateCode(err error) string {
	var se *usecase.StateError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

// Auto approval without prepayment: created confirmed with a firm
// commitment; cancel releases it exactly once; terminal states refuse
// every further transition.
func TestLifecycleAutoConfirmAndCancel(t *testing.T) {
	f := newFx(t)
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(48))
	if r.Status != model.StatusConfirmed || r.CapacityCommitment != model.CommitmentFirm {
		t.Fatalf("created %s / %s, want confirmed / firm", r.Status, r.CapacityCommitment)
	}
	if !r.Price.Total.Equal(money.MustParse("3000")) {
		t.Fatalf("total %s, want 3000 (2 days x 1500)", r.Price.Total)
	}
	if kind, status, _ := f.allocation(r.UUID); kind != "reservation" || status != "active" {
		t.Fatalf("allocation %s/%s, want reservation/active", kind, status)
	}
	if r.Plan.PlannedVehicle == nil || r.Plan.PlateFixed {
		t.Fatalf("plan: %+v", r.Plan)
	}
	if _, err := f.svc.Cancel(f.ctx, f.org, r.UUID, ""); err == nil {
		t.Fatal("cancel without a reason accepted")
	}
	got, err := f.svc.Cancel(f.ctx, f.org, r.UUID, "müşteri vazgeçti")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusCancelled || got.CapacityCommitment != model.CommitmentNone {
		t.Fatalf("after cancel %s / %s", got.Status, got.CapacityCommitment)
	}
	if _, status, _ := f.allocation(r.UUID); status != "released" {
		t.Fatalf("allocation %s, want released", status)
	}
	if f.activeAllocations() != 0 {
		t.Fatal("capacity still held after cancel")
	}
	for name, fn := range map[string]func() error{
		"cancel":  func() error { _, err := f.svc.Cancel(f.ctx, f.org, r.UUID, "tekrar iptal"); return err },
		"confirm": func() error { _, err := f.svc.Confirm(f.ctx, f.org, r.UUID); return err },
		"approve": func() error { _, err := f.svc.Approve(f.ctx, f.org, r.UUID); return err },
		"submit":  func() error { _, err := f.svc.Submit(f.ctx, f.org, r.UUID); return err },
		"no_show": func() error { _, err := f.svc.NoShow(f.ctx, f.org, r.UUID, "gelmedi"); return err },
	} {
		if err := fn(); stateCode(err) != "invalid_transition" {
			t.Errorf("%s on a cancelled reservation: %v", name, err)
		}
	}
	want := []string{"created", "cancel"}
	if ev := f.events(r.UUID); len(ev) != 2 || ev[0] != want[0] || ev[1] != want[1] {
		t.Fatalf("events %v, want %v", ev, want)
	}
}

// Staff approval with prepayment: awaiting_review on an option that
// expires with the review window → approve → awaiting_payment (option
// moved to the payment window) → confirm (same allocation, hold →
// reservation; capacity consumed once).
func TestLifecycleStaffReviewPaymentConfirm(t *testing.T) {
	f := newFx(t)
	f.configure(func(s *osmodel.Settings) {
		s.Reservation.ApprovalMode = osmodel.ApprovalStaff
		s.Reservation.StaffReviewWindowMinutes = intp(120)
		s.Reservation.PaymentHoldMinutes = intp(45)
		s.Payment.PrepaymentRule = osmodel.PrepaymentPercent
		s.Payment.PrepaymentValue = "20"
	})
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(24))
	if r.Status != model.StatusAwaitingReview || r.CapacityCommitment != model.CommitmentHold {
		t.Fatalf("created %s / %s", r.Status, r.CapacityCommitment)
	}
	if r.ReviewDeadline == nil || r.Plan.HoldExpiresAt == nil || !r.ReviewDeadline.Equal(*r.Plan.HoldExpiresAt) {
		t.Fatalf("review deadline %v / hold expiry %v", r.ReviewDeadline, r.Plan.HoldExpiresAt)
	}
	if !r.Price.Prepayment.Equal(money.MustParse("300")) {
		t.Fatalf("prepayment %s, want 300", r.Price.Prepayment)
	}
	kind, _, vehicle := f.allocation(r.UUID)
	if kind != "hold" {
		t.Fatalf("allocation kind %s, want hold", kind)
	}
	a, err := f.svc.Approve(f.ctx, f.org, r.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != model.StatusAwaitingPayment || a.PaymentDeadline == nil || !a.PaymentDeadline.Equal(*a.Plan.HoldExpiresAt) {
		t.Fatalf("after approve %s, deadline %v, hold %v", a.Status, a.PaymentDeadline, a.Plan.HoldExpiresAt)
	}
	cf, err := f.svc.Confirm(f.ctx, f.org, r.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if cf.Status != model.StatusConfirmed || cf.CapacityCommitment != model.CommitmentFirm || cf.ConfirmedAt == nil {
		t.Fatalf("after confirm %+v", cf.Status)
	}
	kind2, status2, vehicle2 := f.allocation(r.UUID)
	if kind2 != "reservation" || status2 != "active" || vehicle2 != vehicle {
		t.Fatalf("allocation after confirm %s/%s on %d (was %d)", kind2, status2, vehicle2, vehicle)
	}
	if n := f.activeAllocations(); n != 1 {
		t.Fatalf("%d active allocations, want 1 (hold → firm consumes once)", n)
	}
	if _, err := f.svc.NoShow(f.ctx, f.org, r.UUID, "gelmedi"); stateCode(err) != "not_started" {
		t.Fatalf("no-show before the pick-up time: %v", err)
	}
	f.advance(73 * time.Hour)
	ns, err := f.svc.NoShow(f.ctx, f.org, r.UUID, "müşteri gelmedi")
	if err != nil {
		t.Fatal(err)
	}
	if ns.Status != model.StatusNoShow || f.activeAllocations() != 0 {
		t.Fatalf("no-show %s, %d active", ns.Status, f.activeAllocations())
	}
}

// The payment window elapses: the sweep expires the reservation and
// releases its option exactly once; a second sweep does nothing.
func TestExpirySweepReleasesOnce(t *testing.T) {
	f := newFx(t)
	f.configure(func(s *osmodel.Settings) {
		s.Reservation.ApprovalMode = osmodel.ApprovalAuto
		s.Reservation.PaymentHoldMinutes = intp(15)
		s.Payment.PrepaymentRule = osmodel.PrepaymentFixed
		s.Payment.PrepaymentValue = "500"
	})
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(24))
	other := f.reserve(p, c, d, f.at(100), f.at(124))
	if r.Status != model.StatusAwaitingPayment {
		t.Fatalf("status %s, want awaiting_payment", r.Status)
	}
	if n, err := f.svc.ExpireDue(f.ctx); err != nil || n != 0 {
		t.Fatalf("early sweep expired %d: %v", n, err)
	}
	f.advance(16 * time.Minute)
	n, err := f.svc.ExpireDue(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expired %d, want 2", n)
	}
	got, err := f.svc.Get(f.ctx, f.org, r.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.StatusExpired || got.CapacityCommitment != model.CommitmentNone {
		t.Fatalf("after sweep %s / %s", got.Status, got.CapacityCommitment)
	}
	if _, status, _ := f.allocation(r.UUID); status == "active" {
		t.Fatalf("option still active: %+v", got.Plan)
	}
	if f.activeAllocations() != 0 {
		t.Fatal("capacity still held")
	}
	if n, err := f.svc.ExpireDue(f.ctx); err != nil || n != 0 {
		t.Fatalf("second sweep expired %d: %v", n, err)
	}
	// Confirming a lapsed option expires instead of confirming.
	if _, err := f.svc.Confirm(f.ctx, f.org, other.UUID); err == nil {
		t.Fatal("confirmed an expired reservation")
	}
}

// A lapsed option found by the engine first (any writer expires due holds)
// is still released only once: confirm answers hold_expired and the
// reservation expires.
func TestConfirmAfterHoldLapsed(t *testing.T) {
	f := newFx(t)
	f.configure(func(s *osmodel.Settings) {
		s.Reservation.PaymentHoldMinutes = intp(10)
		s.Payment.PrepaymentRule = osmodel.PrepaymentFixed
		s.Payment.PrepaymentValue = "100"
	})
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(24))
	f.advance(11 * time.Minute)
	if _, err := f.avail.ExpireDue(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Confirm(f.ctx, f.org, r.UUID); stateCode(err) != "hold_expired" {
		t.Fatalf("confirm a lapsed option: %v", err)
	}
	got, _ := f.svc.Get(f.ctx, f.org, r.UUID)
	if got.Status != model.StatusExpired {
		t.Fatalf("status %s, want expired", got.Status)
	}
	var released int
	_ = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM vehicle_allocations WHERE organization_id = $1 AND status <> 'active'`, f.org).Scan(&released)
	if released != 1 {
		t.Fatalf("%d allocations left active, want exactly one transition", released)
	}
}

// A request holds no capacity; submit places it on the approval flow.
func TestRequestSubmit(t *testing.T) {
	f := newFx(t)
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	q := f.quote(f.ctx, f.svc, p, f.at(0), f.at(24))
	r, err := f.svc.Create(f.ctx, f.org, model.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &c.uuid, Drivers: []model.DriverInput{{DriverUUID: d.uuid}}, RequestOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != model.StatusRequest || r.CapacityCommitment != model.CommitmentNone || f.activeAllocations() != 0 {
		t.Fatalf("request %s / %s, %d active", r.Status, r.CapacityCommitment, f.activeAllocations())
	}
	s, err := f.svc.Submit(f.ctx, f.org, r.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != model.StatusConfirmed || f.activeAllocations() != 1 {
		t.Fatalf("submitted %s, %d active", s.Status, f.activeAllocations())
	}
	if r.Reference == "" || r.Reference[:4] != "RSV-" {
		t.Fatalf("reference %q, want slug prefix RSV-", r.Reference)
	}
}

// Payments (M3 port): covering the prepayment confirms in auto mode.
func TestRecordPaymentConfirms(t *testing.T) {
	f := newFx(t)
	f.configure(func(s *osmodel.Settings) {
		s.Reservation.PaymentHoldMinutes = intp(30)
		s.Payment.PrepaymentRule = osmodel.PrepaymentFixed
		s.Payment.PrepaymentValue = "500"
	})
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	r := f.reserve(p, c, d, f.at(0), f.at(24))
	if err := f.svc.RecordPayment(f.ctx, f.org, r.UUID, money.MustParse("200"), false); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.Get(f.ctx, f.org, r.UUID)
	if got.Status != model.StatusAwaitingPayment || got.PaymentStatus != model.PaymentPartially {
		t.Fatalf("partial: %s / %s", got.Status, got.PaymentStatus)
	}
	if err := f.svc.RecordPayment(f.ctx, f.org, r.UUID, money.MustParse("500"), false); err != nil {
		t.Fatal(err)
	}
	got, _ = f.svc.Get(f.ctx, f.org, r.UUID)
	if got.Status != model.StatusConfirmed || got.PaymentStatus != model.PaymentPartially {
		t.Fatalf("prepaid: %s / %s", got.Status, got.PaymentStatus)
	}
}
