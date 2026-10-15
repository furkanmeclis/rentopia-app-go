package pricing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	campaignmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	campaignusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	extrasmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	extrasusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	osrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	osusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing"
	pricingadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/adapters"
	pricinghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/handler"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	moneypkg "github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPreviewExtrasAndCoupons drives the price preview with extras
// (TEC-702) and coupon codes (TEC-703) over HTTP: the per-product extra
// price, the per-day multiplication and fee cap, the VAT split of the extra
// line, the coupon discount on the rental subtotal with its cap, and the
// rejection reasons (expired, channel, min amount, limit, scope).
func TestPreviewExtrasAndCoupons(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	owner, err := q.CreateUser(ctx, db.CreateUserParams{Email: "extras-" + suffix + "@example.test", PasswordHash: "x", Name: "Ex", Surname: "Tra", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Extras " + suffix, OwnerUserID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", owner.ID)
	})
	mrk, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: org.ID, Code: "MRK"})
	if err != nil {
		t.Fatal(err)
	}
	settingsSvc := osusecase.New(osrepository.NewPostgres(pool, q))
	cfg := osmodel.Defaults()
	cfg.Tax.DefaultVATRate = "20"
	for i := range cfg.SalesChannels {
		cfg.SalesChannels[i].IsActive = true
	}
	if _, err := settingsSvc.Save(ctx, org.ID, owner.ID, osmodel.SaveInput{Settings: cfg}); err != nil {
		t.Fatal(err)
	}
	var segment int64
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, org.ID).Scan(&segment); err != nil {
		t.Fatal(err)
	}
	product := func(code string) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id) VALUES ($1, $2, $2, $3) RETURNING uuid`,
			org.ID, code, segment).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	egea, clio := product("EGEA"), product("CLIO")

	svc := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settingsSvc)
	extras := extrasusecase.New(pool, q)
	campaigns := campaignusecase.New(pool, q)
	svc.SetExtrasResolver(pricingadapters.NewExtras(extras))
	svc.SetCouponEvaluator(pricingadapters.NewCoupons(campaigns))
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	pricing.RegisterRoutes(mux, pricinghandler.New(svc, nil), pricing.EngineHandlers{}, tokens, mapLoader{owner.Uuid: owner.ID}, orgSvc)
	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: owner.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(body)
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		if data, ok := env["data"].(map[string]any); ok {
			return rr.Code, data
		}
		e, _ := env["error"].(map[string]any)
		return rr.Code, e
	}
	// One common plan per product: 1000 TRY a day.
	for _, p := range []uuid.UUID{egea, clio} {
		code, res := call(http.MethodPost, "/v1/tenant/rate-plans", map[string]any{
			"rental_product_uuid": p, "scope": "common", "name": "Genel",
			"bands": []map[string]any{{"min_days": 1, "daily_price": "1000"}},
		})
		if code != http.StatusCreated {
			t.Fatalf("plan = %d %v", code, res)
		}
		if code, res := call(http.MethodPost, "/v1/tenant/rate-plans/"+res["uuid"].(string)+"/activate", nil); code != http.StatusOK {
			t.Fatalf("activate = %d %v", code, res)
		}
	}

	ptr := func(v string) *string { return &v }
	ip := func(v int) *int { return &v }
	dec := func(v string) *extrasmodel.Decimal { d := extrasmodel.Decimal(v); return &d }
	seat, err := extras.Create(ctx, org.ID, extrasmodel.ExtraInput{
		Code: ptr("SEAT"), Name: ptr("Çocuk koltuğu"), ExtraType: ptr(extrasmodel.TypeChildSeat), ChargeType: ptr(extrasmodel.ChargePerDay),
		MaxQuantity: ip(2), Status: ptr(extrasmodel.StatusPublished), DefaultPrice: dec("100"),
		MaxTotal: extrasmodel.OptionalDecimal{Set: true, Value: "900"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := extras.SetPrices(ctx, org.ID, seat.UUID, extrasmodel.PricesInput{
		Items: []extrasmodel.PriceItemInput{{RentalProductUUID: &egea, Price: dec("150")}},
	}); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	preview := func(p uuid.UUID, days int, extra map[string]any) (int, map[string]any) {
		t.Helper()
		body := map[string]any{
			"rental_product_uuid": p, "branch_uuid": mrk.Uuid, "channel": "web",
			"start_at": start.Format(time.RFC3339), "end_at": start.Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339),
		}
		for k, v := range extra {
			body[k] = v
		}
		return call(http.MethodPost, "/v1/tenant/pricing/preview", body)
	}
	current := func(res map[string]any) map[string]any {
		t.Helper()
		side := res["current"].(map[string]any)
		b, _ := side["breakdown"].(map[string]any)
		if b == nil {
			t.Fatalf("no breakdown: %v", side)
		}
		return side
	}
	extraLine := func(side map[string]any) map[string]any {
		for _, l := range side["breakdown"].(map[string]any)["lines"].([]any) {
			if line := l.(map[string]any); line["kind"] == "extra" {
				return line
			}
		}
		t.Fatalf("no extra line: %v", side)
		return nil
	}

	// RT-A13: per-product price (EGEA 150, CLIO default 100), per day x 2.
	_, res := preview(egea, 2, map[string]any{"extras": []map[string]any{{"extra_uuid": seat.UUID, "quantity": 2}}})
	line := extraLine(current(res))
	if line["gross"] != "600.00" || line["unit_price"] != "150.00" || line["price_id"] == "" || line["base"] != "500.00" || line["vat"] != "100.00" {
		t.Fatalf("egea seat line = %v", line)
	}
	_, res = preview(clio, 2, map[string]any{"extras": []map[string]any{{"extra_uuid": seat.UUID, "quantity": 2}}})
	if line := extraLine(current(res)); line["gross"] != "400.00" || line["unit_price"] != "100.00" {
		t.Fatalf("clio seat line = %v", line)
	}
	// Cap: 150 x 2 x 5 = 1500 -> 900.
	_, res = preview(egea, 5, map[string]any{"extras": []map[string]any{{"extra_uuid": seat.UUID, "quantity": 2}}})
	if line := extraLine(current(res)); line["gross"] != "900.00" || line["capped"] != true {
		t.Fatalf("capped seat line = %v", line)
	}
	if b := current(res)["breakdown"].(map[string]any); b["total"] != "5900.00" {
		t.Fatalf("total with extras = %v", b["total"])
	}
	// Invalid selection: 400 with a field detail.
	code, res := preview(egea, 2, map[string]any{"extras": []map[string]any{{"extra_uuid": seat.UUID, "quantity": 3}}})
	if code != http.StatusBadRequest || res["details"].([]any)[0].(map[string]any)["field"] != "extras.0.quantity" {
		t.Fatalf("quantity out of range = %d %v", code, res)
	}

	// RT-A12 / coupons: 20 % on the rental subtotal, capped at 500.
	value := campaignmodel.Decimal("20")
	maxD := campaignmodel.Decimal("500")
	some := func(v string) campaignmodel.Optional[string] {
		return campaignmodel.Optional[string]{Set: true, Value: &v}
	}
	newCoupon := func(code string, mutate func(*campaignmodel.CampaignInput)) {
		t.Helper()
		in := campaignmodel.CampaignInput{
			Name: ptr(code), CampaignType: ptr(campaignmodel.TypeCoupon), Code: some(code),
			DiscountKind: ptr(campaignmodel.KindPercent), DiscountValue: &value,
			MaxDiscount: campaignmodel.Optional[campaignmodel.Decimal]{Set: true, Value: &maxD},
		}
		if mutate != nil {
			mutate(&in)
		}
		if _, err := campaigns.Create(ctx, org.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	newCoupon("YAZ20", nil)
	_, res = preview(egea, 2, map[string]any{"coupon_code": "yaz20"})
	side := current(res)
	b := side["breakdown"].(map[string]any)
	coupon := side["coupon"].(map[string]any)
	if coupon["status"] != "applied" || b["discount"].(map[string]any)["amount"] != "400.00" || b["rental_total"] != "1600.00" {
		t.Fatalf("applied coupon = %v %v", coupon, b["discount"])
	}
	_, res = preview(egea, 5, map[string]any{"coupon_code": "YAZ20"})
	if d := current(res)["breakdown"].(map[string]any)["discount"].(map[string]any); d["amount"] != "500.00" || d["capped"] != true {
		t.Fatalf("capped coupon = %v", d)
	}
	// Rejections keep the price without a discount and explain why.
	rejected := func(code string, days int, reason string) {
		t.Helper()
		_, res := preview(egea, days, map[string]any{"coupon_code": code})
		side := current(res)
		c := side["coupon"].(map[string]any)
		if c["status"] != "rejected" || c["reason"] != reason || side["breakdown"].(map[string]any)["discount"] != nil {
			t.Fatalf("%s: coupon = %v", code, c)
		}
	}
	rejected("NOPE", 2, campaignmodel.ReasonNotFound)
	newCoupon("ESKI", func(in *campaignmodel.CampaignInput) {
		in.StartDate, in.EndDate = some("2025-01-01"), some("2025-02-01")
	})
	rejected("ESKI", 2, campaignmodel.ReasonExpired)
	newCoupon("SUBE", func(in *campaignmodel.CampaignInput) {
		branch := []string{"branch"}
		in.Channels = campaignmodel.Optional[[]string]{Set: true, Value: &branch}
	})
	rejected("SUBE", 2, campaignmodel.ReasonChannel)
	newCoupon("UZUN", func(in *campaignmodel.CampaignInput) {
		min := campaignmodel.Decimal("3000")
		in.MinSubtotal = campaignmodel.Optional[campaignmodel.Decimal]{Set: true, Value: &min}
	})
	rejected("UZUN", 2, campaignmodel.ReasonMinAmount)
	_, res = preview(egea, 3, map[string]any{"coupon_code": "UZUN"})
	if current(res)["coupon"].(map[string]any)["status"] != "applied" {
		t.Fatalf("3 days reach the minimum: %v", current(res)["coupon"])
	}
	newCoupon("CLIO", func(in *campaignmodel.CampaignInput) {
		in.ProductScope = ptr(campaignmodel.ScopeProducts)
		in.RentalProductUUIDs = &[]uuid.UUID{clio}
	})
	rejected("CLIO", 2, campaignmodel.ReasonScope)
	newCoupon("TEK", func(in *campaignmodel.CampaignInput) {
		one := 1
		in.UsageLimit = campaignmodel.Optional[int]{Set: true, Value: &one}
	})
	if _, err := campaigns.Redeem(ctx, org.ID, campaignusecase.RedeemInput{
		Quote:           campaignusecase.Quote{Code: "TEK", Channel: "web", Subtotal: moneypkg.MustParse("2000")},
		ReservationUUID: uuid.New(),
	}); err != nil {
		t.Fatal(err)
	}
	rejected("TEK", 2, campaignmodel.ReasonLimit)
	// A coupon and a manual discount together: 400.
	if code, _ := preview(egea, 2, map[string]any{"coupon_code": "YAZ20", "discount": map[string]any{"kind": "amount", "value": "10"}}); code != http.StatusBadRequest {
		t.Fatalf("coupon + manual discount = %d", code)
	}
}
