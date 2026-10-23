package usecase_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fx struct {
	t        testing.TB
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	svc      *usecase.Service
	org      int64
	segment  int64
	segUUID  uuid.UUID
	product  int64
	prodUUID uuid.UUID
}

func newFx(t testing.TB) *fx {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 24
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(pool)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "camp-" + suffix, Name: "Campaigns " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{t: t, ctx: ctx, pool: pool, q: q, svc: usecase.New(pool, q), org: org.ID}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id, uuid`, f.org).Scan(&f.segment, &f.segUUID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id) VALUES ($1, 'EGEA', 'Egea', $2) RETURNING id, uuid`,
		f.org, f.segment).Scan(&f.product, &f.prodUUID); err != nil {
		t.Fatal(err)
	}
	return f
}

func ptr[T any](v T) *T { return &v }

func some[T any](v T) model.Optional[T] { return model.Optional[T]{Set: true, Value: &v} }

func (f *fx) coupon(code string, mutate func(in *model.CampaignInput)) model.Campaign {
	f.t.Helper()
	value := model.Decimal("20")
	in := model.CampaignInput{
		Name: ptr("Kampanya " + code), CampaignType: ptr(model.TypeCoupon), Code: some(code),
		DiscountKind: ptr(model.KindPercent), DiscountValue: &value,
	}
	if mutate != nil {
		mutate(&in)
	}
	c, err := f.svc.Create(f.ctx, f.org, in)
	if err != nil {
		f.t.Fatal(err)
	}
	return c
}

func (f *fx) quote(code string, subtotal string) usecase.Quote {
	return usecase.Quote{Code: code, ProductID: f.product, SegmentID: f.segment, Channel: "web", Subtotal: money.MustParse(subtotal)}
}

func isRejection(err error, reason string) bool {
	var re *usecase.RejectionError
	return errors.As(err, &re) && re.Reason == reason
}

// RT-A12: concurrent uses of the last remaining redemptions never exceed
// the limit (the atomic counter), for limit 1 and for limit 3 under ten
// concurrent uses.
func TestRT_A12_LastRedemptionConcurrency(t *testing.T) {
	for _, c := range []struct{ limit, workers int }{{1, 2}, {1, 8}, {3, 10}} {
		f := newFx(t)
		camp := f.coupon("SON"+string(rune('A'+c.limit))+string(rune('A'+c.workers)), func(in *model.CampaignInput) {
			in.UsageLimit = some(c.limit)
		})
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, limited := 0, 0
		start := make(chan struct{})
		for i := 0; i < c.workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: f.quote(*camp.Code, "3000"), ReservationUUID: uuid.New()})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case isRejection(err, model.ReasonLimit):
					limited++
				default:
					t.Errorf("redeem: %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if ok != c.limit || limited != c.workers-c.limit {
			t.Fatalf("limit %d, %d workers: %d succeeded, %d limited", c.limit, c.workers, ok, limited)
		}
		got, err := f.svc.Get(f.ctx, f.org, camp.UUID)
		if err != nil || got.UsedCount != c.limit || got.State != model.StateExhausted || *got.Remaining != 0 {
			t.Fatalf("after the race = %+v %v", got, err)
		}
		// The database refuses to pass the limit even without the WHERE.
		if _, err := f.pool.Exec(f.ctx, `UPDATE campaigns SET used_count = used_count + 1 WHERE organization_id = $1 AND uuid = $2`, f.org, camp.UUID); err == nil {
			t.Fatal("used_count passed usage_limit")
		}
	}
}

// One customer racing for a per-customer limit of 1: exactly one use.
func TestPerCustomerLimitConcurrency(t *testing.T) {
	f := newFx(t)
	camp := f.coupon("MUSTERI1", func(in *model.CampaignInput) { in.PerCustomerLimit = some(1) })
	customer := uuid.New()
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := f.quote(*camp.Code, "2000")
			q.CustomerUUID = &customer
			_, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: q, ReservationUUID: uuid.New()})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			} else if !isRejection(err, model.ReasonCustomerLimit) {
				t.Errorf("redeem: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d uses for one customer, want 1", ok)
	}
	// Without a customer the per-customer limit cannot be checked: refused.
	if _, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: f.quote(*camp.Code, "2000"), ReservationUUID: uuid.New()}); !isRejection(err, model.ReasonCustomerRequired) {
		t.Fatalf("no customer: %v", err)
	}
}

// Evaluate explains every rejection; the discount follows percent, cap
// and amount rules.
func TestEvaluateReasons(t *testing.T) {
	f := newFx(t)
	today := rentaltime.LocalDate(time.Now())
	eval := func(q usecase.Quote) *model.CouponResult {
		t.Helper()
		ev, err := f.svc.Evaluate(f.ctx, f.org, q)
		if err != nil {
			t.Fatal(err)
		}
		if ev.Result.Status == "applied" && ev.Discount == nil {
			t.Fatal("applied without a discount")
		}
		return ev.Result
	}
	reason := func(q usecase.Quote, want string) {
		t.Helper()
		if r := eval(q); r.Status != "rejected" || r.Reason != want {
			t.Fatalf("%s: got %+v", want, r)
		}
	}
	reason(f.quote("YOKBOYLE", "1000"), model.ReasonNotFound)
	f.coupon("PASIF", func(in *model.CampaignInput) { in.IsActive = ptr(false) })
	reason(f.quote("pasif", "1000"), model.ReasonInactive)
	f.coupon("ESKI", func(in *model.CampaignInput) {
		in.StartDate, in.EndDate = some(today.AddDays(-10).String()), some(today.AddDays(-1).String())
	})
	reason(f.quote("ESKI", "1000"), model.ReasonExpired)
	f.coupon("GELECEK", func(in *model.CampaignInput) { in.StartDate = some(today.AddDays(2).String()) })
	reason(f.quote("GELECEK", "1000"), model.ReasonNotStarted)
	f.coupon("SUBE", func(in *model.CampaignInput) { in.Channels = some([]string{"branch"}) })
	reason(f.quote("SUBE", "1000"), model.ReasonChannel)
	min := model.Decimal("5000")
	f.coupon("ENAZ", func(in *model.CampaignInput) { in.MinSubtotal = model.Optional[model.Decimal]{Set: true, Value: &min} })
	if r := eval(f.quote("ENAZ", "4999.99")); r.Reason != model.ReasonMinAmount || r.Params["min_subtotal"] != "5000.00" {
		t.Fatalf("min amount = %+v", r)
	}
	f.coupon("SEGMENT", func(in *model.CampaignInput) {
		in.ProductScope = ptr(model.ScopeSegments)
		in.SegmentUUIDs = &[]uuid.UUID{f.segUUID}
	})
	other := f.quote("SEGMENT", "1000")
	other.SegmentID = f.segment + 999999
	reason(other, model.ReasonScope)
	if r := eval(f.quote("SEGMENT", "1000")); r.Status != "applied" {
		t.Fatalf("segment scope = %+v", r)
	}
	vip := uuid.New()
	f.coupon("VIP", func(in *model.CampaignInput) { in.CustomerUUIDs = some([]uuid.UUID{vip}) })
	reason(f.quote("VIP", "1000"), model.ReasonCustomerRequired)
	stranger := uuid.New()
	q := f.quote("VIP", "1000")
	q.CustomerUUID = &stranger
	reason(q, model.ReasonCustomerScope)
	f.coupon("KURUMSAL", nil)
	corp := f.quote("KURUMSAL", "1000")
	corp.Corporate = true
	reason(corp, model.ReasonCorporate)
	f.coupon("TEK", func(in *model.CampaignInput) { in.UsageLimit = some(1) })
	if _, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: f.quote("TEK", "1000"), ReservationUUID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	reason(f.quote("TEK", "1000"), model.ReasonLimit)

	// Percent with a cap, case-insensitive code.
	maxD := model.Decimal("300")
	camp := f.coupon("YAZ20", func(in *model.CampaignInput) { in.MaxDiscount = model.Optional[model.Decimal]{Set: true, Value: &maxD} })
	ev, err := f.svc.Evaluate(f.ctx, f.org, f.quote("yaz20", "5000"))
	if err != nil || ev.Discount == nil || ev.Discount.Percent != money.Percent(20) || ev.Discount.MaxAmount.String() != "300.00" || ev.Discount.CampaignID != camp.UUID.String() {
		t.Fatalf("discount = %+v %v", ev.Discount, err)
	}
	// Repeat-customer campaigns never apply with the zero rental counter.
	value := model.Decimal("100")
	if _, err := f.svc.Create(f.ctx, f.org, model.CampaignInput{
		Name: ptr("Sadık müşteri"), CampaignType: ptr(model.TypeRepeatCustomer), DiscountKind: ptr(model.KindAmount), DiscountValue: &value,
	}); err != nil {
		t.Fatal(err)
	}
	loyal := uuid.New()
	auto := f.quote("", "1000")
	auto.CustomerUUID = &loyal
	if ev, err := f.svc.Evaluate(f.ctx, f.org, auto); err != nil || ev.Discount != nil {
		t.Fatalf("repeat customer with zero rentals = %+v %v", ev, err)
	}
	f.svc.SetRentalCounter(fixedCounter(3))
	if ev, err := f.svc.Evaluate(f.ctx, f.org, auto); err != nil || ev.Discount == nil || ev.Discount.Amount.String() != "100.00" {
		t.Fatalf("repeat customer with 3 rentals = %+v %v", ev, err)
	}
}

type fixedCounter int

func (c fixedCounter) CompletedRentals(context.Context, int64, uuid.UUID) (int, error) {
	return int(c), nil
}

// Repeat-customer campaigns apply from min_completed_rentals completed
// rentals on: two required, one completed is refused (not_eligible with the
// numbers), two completed redeem.
func TestRepeatCustomerMinCompletedRentals(t *testing.T) {
	f := newFx(t)
	value := model.Decimal("150")
	camp, err := f.svc.Create(f.ctx, f.org, model.CampaignInput{
		Name: ptr("İkinci kiralama"), CampaignType: ptr(model.TypeRepeatCustomer), DiscountKind: ptr(model.KindAmount),
		DiscountValue: &value, MinCompletedRentals: ptr(2),
	})
	if err != nil || camp.MinCompletedRentals != 2 {
		t.Fatalf("create = %+v %v", camp, err)
	}
	customer := uuid.New()
	// Automatic (no code) for Evaluate; by campaign uuid for Redeem.
	quote := f.quote("", "1000")
	quote.CustomerUUID = &customer
	byUUID := quote
	byUUID.CampaignUUID = &camp.UUID

	f.svc.SetRentalCounter(fixedCounter(1))
	if ev, err := f.svc.Evaluate(f.ctx, f.org, quote); err != nil || ev.Discount != nil {
		t.Fatalf("one completed rental must not apply: %+v %v", ev, err)
	}
	_, err = f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: byUUID, ReservationUUID: uuid.New()})
	var re *usecase.RejectionError
	if !errors.As(err, &re) || re.Reason != model.ReasonNotEligible ||
		re.Params["min_completed_rentals"] != "2" || re.Params["completed_rentals"] != "1" {
		t.Fatalf("one completed rental: %v %+v", err, re)
	}

	f.svc.SetRentalCounter(fixedCounter(2))
	if ev, err := f.svc.Evaluate(f.ctx, f.org, quote); err != nil || ev.Discount == nil || ev.Discount.Amount.String() != "150.00" {
		t.Fatalf("two completed rentals must apply: %+v %v", ev, err)
	}
	red, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: byUUID, ReservationUUID: uuid.New()})
	if err != nil || red.Status != model.RedemptionReserved || red.DiscountAmount.String() != "150.00" {
		t.Fatalf("two completed rentals redeem = %+v %v", red, err)
	}
}

// One discount per reservation; release gives the usage back only when the
// campaign says so; codes are unique per organization (case-insensitive).
func TestRedemptionLifecycle(t *testing.T) {
	f := newFx(t)
	back := f.coupon("IADE", func(in *model.CampaignInput) { in.UsageLimit = some(1) })
	keep := f.coupon("KALIR", func(in *model.CampaignInput) {
		in.UsageLimit = some(5)
		in.ReleaseOnCancel = ptr(false)
	})
	res := uuid.New()
	red, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: f.quote("IADE", "1000"), ReservationUUID: res})
	if err != nil || red.Status != model.RedemptionReserved || red.DiscountAmount.String() != "200.00" {
		t.Fatalf("redeem = %+v %v", red, err)
	}
	if _, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: f.quote("KALIR", "1000"), ReservationUUID: res}); !errors.Is(err, usecase.ErrAlreadyRedeemed) {
		t.Fatalf("second discount on one reservation: %v", err)
	}
	if got, _ := f.svc.Get(f.ctx, f.org, keep.UUID); got.UsedCount != 0 {
		t.Fatalf("refused redemption kept its usage: %d", got.UsedCount)
	}
	rel, err := f.svc.Release(f.ctx, f.org, res, "cancelled")
	if err != nil || rel.Status != model.RedemptionReleased || !rel.UsageReturned {
		t.Fatalf("release = %+v %v", rel, err)
	}
	if got, _ := f.svc.Get(f.ctx, f.org, back.UUID); got.UsedCount != 0 {
		t.Fatalf("usage not returned: %d", got.UsedCount)
	}
	// The released reservation may take another campaign; this one keeps
	// its usage on cancel.
	if _, err := f.svc.Redeem(f.ctx, f.org, usecase.RedeemInput{Quote: f.quote("KALIR", "1000"), ReservationUUID: res}); err != nil {
		t.Fatal(err)
	}
	if c, err := f.svc.Consume(f.ctx, f.org, res); err != nil || c.Status != model.RedemptionConsumed {
		t.Fatalf("consume = %+v %v", c, err)
	}
	if rel, err := f.svc.Release(f.ctx, f.org, res, "refunded"); err != nil || rel.UsageReturned {
		t.Fatalf("refund = %+v %v", rel, err)
	}
	if got, _ := f.svc.Get(f.ctx, f.org, keep.UUID); got.UsedCount != 1 {
		t.Fatalf("usage returned against the setting: %d", got.UsedCount)
	}
	list, total, err := f.svc.Redemptions(f.ctx, f.org, keep.UUID, nil, 10, 0)
	if err != nil || total != 1 || list[0].ReleaseReason != "refunded" {
		t.Fatalf("redemptions = %+v %d %v", list, total, err)
	}
	if _, err := f.svc.Release(f.ctx, f.org, res, "again"); !errors.Is(err, usecase.ErrRedemptionNotFound) {
		t.Fatalf("release twice: %v", err)
	}
	// Code uniqueness, usage_limit below the uses so far.
	value := model.Decimal("10")
	_, err = f.svc.Create(f.ctx, f.org, model.CampaignInput{
		Name: ptr("x"), CampaignType: ptr(model.TypeCoupon), Code: some("iade"), DiscountKind: ptr(model.KindAmount), DiscountValue: &value,
	})
	if !errors.Is(err, usecase.ErrCodeTaken) {
		t.Fatalf("duplicate code: %v", err)
	}
	if _, err := f.svc.Patch(f.ctx, f.org, keep.UUID, model.CampaignInput{UsageLimit: some(0)}); err == nil {
		t.Fatal("usage_limit 0 accepted")
	}
	var ve *usecase.ValidationError
	if _, err := f.svc.Create(f.ctx, f.org, model.CampaignInput{
		Name: ptr("y"), CampaignType: ptr(model.TypeCoupon), DiscountKind: ptr(model.KindPercent), DiscountValue: ptr(model.Decimal("120")),
	}); !errors.As(err, &ve) || len(ve.Details) != 2 {
		t.Fatalf("missing code + 120 %%: %v", err)
	}
}
