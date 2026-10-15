package usecase_test

import (
	"testing"

	campaignsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	campaignsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	extrasmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	extrasusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	pricingadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/adapters"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
)

func ptrOf[T any](v T) *T { return &v }

// wired rebuilds the reservation service with the real extras (TEC-702)
// and campaigns (TEC-703) modules behind the ports, as server.go does.
func (f *fx) wired() (*extrasusecase.Service, *campaignsusecase.Service) {
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(f.pool, f.q))
	extras := extrasusecase.New(f.pool, f.q)
	campaigns := campaignsusecase.New(f.pool, f.q)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(f.pool, f.q), settings)
	pricing.SetExtrasResolver(pricingadapters.NewExtras(extras))
	pricing.SetCouponEvaluator(pricingadapters.NewCoupons(campaigns))
	svc := usecase.New(f.pool, f.q, f.avail, pricing, settings)
	svc.SetClock(f.now)
	svc.SetEquipment(adapters.NewEquipment(extras))
	svc.SetCoupons(adapters.NewCoupons(campaigns))
	f.svc = svc
	return extras, campaigns
}

// Extras are priced into the quote, physical ones held with the vehicle
// allocation (cancel frees them through it); a coupon is redeemed on the
// atomic counter and given back on cancellation.
func TestExtrasAndCouponThroughTheModules(t *testing.T) {
	f := newFx(t)
	extras, campaigns := f.wired()
	p := f.product("EKO", 21)
	f.vehicle(f.mrk)
	c, d := f.customer(30, 5)
	seat, err := extras.Create(f.ctx, f.org, extrasmodel.ExtraInput{
		Code: ptrOf("CS"), Name: ptrOf("Çocuk koltuğu"), ExtraType: ptrOf(extrasmodel.TypeChildSeat), ChargeType: ptrOf(extrasmodel.ChargePerDay),
		MinQuantity: ptrOf(1), MaxQuantity: ptrOf(2), Status: ptrOf(extrasmodel.StatusPublished), IsPhysical: ptrOf(true),
		DefaultPrice: ptrOf(extrasmodel.Decimal("100")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extras.SetStock(f.ctx, f.org, seat.UUID, extrasmodel.StockInput{Items: []extrasmodel.StockItemInput{{BranchUUID: &f.mrk.uuid, Quantity: ptrOf(1)}}}); err != nil {
		t.Fatal(err)
	}
	value := campaignsmodel.Decimal("10")
	limit := 1
	camp, err := campaigns.Create(f.ctx, f.org, campaignsmodel.CampaignInput{
		Name: ptrOf("Ekim"), CampaignType: ptrOf(campaignsmodel.TypeCoupon), Code: campaignsmodel.Optional[string]{Set: true, Value: ptrOf("EKIM10")},
		DiscountKind: ptrOf(campaignsmodel.KindPercent), DiscountValue: &value, ReleaseOnCancel: ptrOf(true),
		UsageLimit: campaignsmodel.Optional[int]{Set: true, Value: &limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	from, to := f.at(0), f.at(48)
	q, err := f.svc.CreateQuote(f.ctx, f.org, model.QuoteInput{
		Where: f.where(from, to), Channel: "branch", ProductUUID: &p.uuid, CustomerUUID: &c.uuid,
		Extras: []model.ExtraRequest{{ExtraUUID: seat.UUID, Quantity: 1}}, CouponCode: "ekim10",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 2 days x 1500 = 3000, -10% = 2700, child seat 2 x 100 = 200.
	if !q.Total.Equal(money.MustParse("2900")) || !q.ExtrasTotal.Equal(money.MustParse("200")) || !q.DiscountTotal.Equal(money.MustParse("300")) {
		t.Fatalf("quote total %s extras %s discount %s", q.Total, q.ExtrasTotal, q.DiscountTotal)
	}
	r, err := f.svc.Create(f.ctx, f.org, model.CreateInput{QuoteUUID: &q.UUID, CustomerUUID: &c.uuid, Drivers: []model.DriverInput{{DriverUUID: d.uuid}}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := campaigns.Get(f.ctx, f.org, camp.UUID)
	if got.UsedCount != 1 {
		t.Fatalf("used %d, want 1", got.UsedCount)
	}
	avail, err := extras.Availability(f.ctx, f.org, extrasmodel.AvailabilityInput{BranchUUID: &f.mrk.uuid, StartsAt: &from, EndsAt: &to})
	if err != nil || len(avail.Items) != 1 || avail.Items[0].Available != 0 {
		t.Fatalf("seat availability %+v %v", avail, err)
	}
	// The coupon is exhausted: a second sale with it is refused before any
	// capacity is taken.
	f.vehicle(f.mrk)
	q2, err := f.svc.CreateQuote(f.ctx, f.org, model.QuoteInput{Where: f.where(from, to), Channel: "branch", ProductUUID: &p.uuid, CouponCode: "EKIM10"})
	if err == nil {
		t.Fatalf("exhausted coupon quoted: %+v", q2.Breakdown.Pricing.Discount)
	}
	if _, err := f.svc.Cancel(f.ctx, f.org, r.UUID, "vazgeçti"); err != nil {
		t.Fatal(err)
	}
	got, _ = campaigns.Get(f.ctx, f.org, camp.UUID)
	if got.UsedCount != 0 {
		t.Fatalf("used %d after cancel, want 0 (release_on_cancel)", got.UsedCount)
	}
	avail, _ = extras.Availability(f.ctx, f.org, extrasmodel.AvailabilityInput{BranchUUID: &f.mrk.uuid, StartsAt: &from, EndsAt: &to})
	if avail.Items[0].Available != 1 {
		t.Fatalf("seat not freed by the cancellation: %+v", avail.Items[0])
	}
}
