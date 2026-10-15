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
	avmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	avusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with a branch, a segment and two products, made
// with SQL and removed when the test ends (the shared database is never
// truncated).
type fx struct {
	t       testing.TB
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	svc     *usecase.Service
	org     int64
	segment int64
	branch  ref
	base    time.Time
}

type ref struct {
	id   int64
	uuid uuid.UUID
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
	cfg.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(pool)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "extras-" + suffix, Name: "Extras " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	f := &fx{t: t, ctx: ctx, pool: pool, q: q, svc: usecase.New(pool, q), org: org.ID}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment); err != nil {
		t.Fatal(err)
	}
	f.branch = f.newBranch("IST")
	f.base = time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	return f
}

func (f *fx) at(hours int) time.Time { return f.base.Add(time.Duration(hours) * time.Hour) }

func (f *fx) newBranch(code string) ref {
	f.t.Helper()
	b, err := f.q.CreateBranch(f.ctx, db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: 0, IsActive: true,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return ref{b.ID, b.Uuid}
}

// product creates a published product with a GROUP version accepting every
// vehicle (availability needs a version).
func (f *fx) product(code string) ref {
	f.t.Helper()
	var p ref
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, $2, $3, $4, 'published') RETURNING id, uuid`, f.org, code, code+" ürün", f.segment).Scan(&p.id, &p.uuid); err != nil {
		f.t.Fatal(err)
	}
	var group int64
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, $2, $2, $3) RETURNING id`,
		f.org, "G"+code, f.segment).Scan(&group); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id)
		VALUES ($1, $2, 1, 'GROUP', $3)`, f.org, p.id, group); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func ptr[T any](v T) *T { return &v }

func dec(s string) *model.Decimal { d := model.Decimal(s); return &d }

// childSeat creates a published, physical per-day child seat.
func (f *fx) childSeat(code, defaultPrice string) model.Extra {
	f.t.Helper()
	e, err := f.svc.Create(f.ctx, f.org, model.ExtraInput{
		Code: ptr(code), Name: ptr("Çocuk koltuğu"), ExtraType: ptr(model.TypeChildSeat), ChargeType: ptr(model.ChargePerDay),
		MinQuantity: ptr(1), MaxQuantity: ptr(3), Status: ptr(model.StatusPublished), IsPhysical: ptr(true),
		DefaultPrice: dec(defaultPrice),
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

func (f *fx) setStock(e model.Extra, b ref, n int) {
	f.t.Helper()
	if _, err := f.svc.SetStock(f.ctx, f.org, e.UUID, model.StockInput{Items: []model.StockItemInput{{BranchUUID: &b.uuid, Quantity: &n}}}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fx) reserve(e model.Extra, b ref, from, to time.Time, qty int, kind string) (model.Allocation, error) {
	return f.svc.Reserve(f.ctx, f.org, model.ReserveInput{
		ExtraUUID: &e.UUID, BranchUUID: &b.uuid, Quantity: &qty, StartsAt: &from, EndsAt: &to, Kind: kind,
	}, nil)
}

func isStock(err error) bool {
	var se *usecase.StockError
	return errors.As(err, &se)
}

// RT-A13 (1/2): the price of the rental product is applied; a product
// without its own price falls back to the default; price changes are new
// rows, so the row a quote used stays readable.
func TestRT_A13_PerProductPrice(t *testing.T) {
	f := newFx(t)
	egea, clio := f.product("EGEA"), f.product("CLIO")
	seat := f.childSeat("SEAT", "100")
	if _, _, err := f.svc.SetPrices(f.ctx, f.org, seat.UUID, model.PricesInput{
		Items: []model.PriceItemInput{{RentalProductUUID: &egea.uuid, Price: dec("150")}},
	}); err != nil {
		t.Fatal(err)
	}
	resolve := func(p ref, qty int) []string {
		t.Helper()
		lines, err := f.svc.Resolve(f.ctx, f.org, usecase.ResolveInput{ProductID: p.id, Channel: "web", Items: []model.Selection{{ExtraUUID: &seat.UUID, Quantity: &qty}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != 1 || !lines[0].PerDay || lines[0].Quantity != qty || lines[0].ExtraID != seat.UUID.String() || lines[0].PriceID == "" {
			t.Fatalf("lines = %+v", lines)
		}
		return []string{lines[0].UnitPrice.String(), lines[0].PriceID}
	}
	egeaPrice := resolve(egea, 2)
	if egeaPrice[0] != "150.00" {
		t.Fatalf("egea price = %s, want the product price 150.00", egeaPrice[0])
	}
	if p := resolve(clio, 1); p[0] != "100.00" {
		t.Fatalf("clio price = %s, want the default 100.00", p[0])
	}

	// A new price closes the old row; the quoted row is still in the history.
	m, _, err := f.svc.SetPrices(f.ctx, f.org, seat.UUID, model.PricesInput{
		Items: []model.PriceItemInput{{RentalProductUUID: &egea.uuid, Price: dec("175")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if again := resolve(egea, 1); again[0] != "175.00" || again[1] == egeaPrice[1] {
		t.Fatalf("after change = %v (old row %s)", again, egeaPrice[1])
	}
	for _, it := range m.Items {
		if it.Product.UUID == clio.uuid && (it.Source != "default" || it.Effective.String() != "100.00") {
			t.Fatalf("clio matrix row = %+v", it)
		}
	}
	hist, total, err := f.svc.PriceHistory(f.ctx, f.org, seat.UUID, 50, 0)
	if err != nil || total != 3 {
		t.Fatalf("history = %d %v", total, err)
	}
	closed := false
	for _, h := range hist {
		if h.UUID.String() == egeaPrice[1] && h.ValidTo != nil && h.Price.String() == "150.00" {
			closed = true
		}
	}
	if !closed {
		t.Fatalf("old egea row not closed in history: %+v", hist)
	}
	// Direct changes of a price row are refused by the trigger.
	if _, err := f.pool.Exec(f.ctx, `UPDATE extra_prices SET price = 1 WHERE organization_id = $1 AND uuid = $2`, f.org, egeaPrice[1]); err == nil {
		t.Fatal("a price row was changed in place")
	}
	// A scheduled price does not apply before it starts.
	future := time.Now().UTC().Add(72 * time.Hour)
	m, _, err = f.svc.SetPrices(f.ctx, f.org, seat.UUID, model.PricesInput{
		EffectiveFrom: &future, Items: []model.PriceItemInput{{RentalProductUUID: &clio.uuid, Price: dec("90")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Scheduled) != 1 || resolve(clio, 1)[0] != "100.00" {
		t.Fatalf("scheduled = %+v", m.Scheduled)
	}

	// Resolver rules: quantity bounds, channel, publish status, cap.
	bad := func(items []model.Selection, channel, code string) {
		t.Helper()
		_, err := f.svc.Resolve(f.ctx, f.org, usecase.ResolveInput{ProductID: egea.id, Channel: channel, Items: items})
		var ve *usecase.ValidationError
		if !errors.As(err, &ve) || ve.Details[0].Code != code {
			t.Fatalf("want %s, got %v", code, err)
		}
	}
	bad([]model.Selection{{ExtraUUID: &seat.UUID, Quantity: ptr(4)}}, "web", "out_of_range")
	bad([]model.Selection{{ExtraUUID: &seat.UUID}, {ExtraUUID: &seat.UUID}}, "web", "duplicate")
	bad([]model.Selection{{ExtraUUID: ptr(uuid.New())}}, "web", "not_found")
	if _, err := f.svc.Patch(f.ctx, f.org, seat.UUID, model.ExtraInput{Channels: model.OptionalStrings{Set: true, Value: []string{"branch"}}}); err != nil {
		t.Fatal(err)
	}
	bad([]model.Selection{{ExtraUUID: &seat.UUID}}, "web", "channel_not_allowed")
	draft, err := f.svc.Create(f.ctx, f.org, model.ExtraInput{
		Code: ptr("GPS"), Name: ptr("GPS"), ExtraType: ptr(model.TypeOther), ChargeType: ptr(model.ChargePerRental), DefaultPrice: dec("50"),
	})
	if err != nil {
		t.Fatal(err)
	}
	bad([]model.Selection{{ExtraUUID: &draft.UUID}}, "web", "not_available")
	cap := model.Decimal("250")
	delivery, err := f.svc.Create(f.ctx, f.org, model.ExtraInput{
		Code: ptr("DLV"), Name: ptr("Teslimat"), ExtraType: ptr(model.TypeDelivery), ChargeType: ptr(model.ChargePerRental),
		Status: ptr(model.StatusPublished), MaxTotal: model.OptionalDecimal{Set: true, Value: cap},
	})
	if err != nil {
		t.Fatal(err)
	}
	bad([]model.Selection{{ExtraUUID: &delivery.UUID}}, "web", "no_price")
	if _, _, err := f.svc.SetPrices(f.ctx, f.org, delivery.UUID, model.PricesInput{DefaultPrice: model.OptionalDecimal{Set: true, Value: "300"}}); err != nil {
		t.Fatal(err)
	}
	lines, err := f.svc.Resolve(f.ctx, f.org, usecase.ResolveInput{ProductID: egea.id, Channel: "web", Items: []model.Selection{{ExtraUUID: &delivery.UUID, Quantity: ptr(5)}}})
	if err != nil || len(lines) != 1 || lines[0].Quantity != 1 || lines[0].PerDay || lines[0].MaxTotal == nil || lines[0].MaxTotal.String() != "250.00" {
		t.Fatalf("per-rental delivery = %+v %v", lines, err)
	}
}

// RT-A13 (2/2): concurrent requests for the last child seat at a branch —
// exactly one succeeds, the others are refused (no oversell).
func TestRT_A13_LastChildSeatConcurrency(t *testing.T) {
	for _, workers := range []int{2, 8} {
		f := newFx(t)
		seat := f.childSeat("SEAT", "100")
		f.setStock(seat, f.branch, 2)
		// One seat is already taken in the range: one left.
		if _, err := f.reserve(seat, f.branch, f.at(0), f.at(48), 1, model.KindReservation); err != nil {
			t.Fatal(err)
		}
		// Every worker has its own service; the winner pauses between the
		// check and the insert, so the others are sure to arrive while it
		// holds the stock row lock.
		svc := usecase.New(f.pool, f.q)
		svc.SetAfterCheckHook(func() { time.Sleep(20 * time.Millisecond) })
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, refused := 0, 0
		start := make(chan struct{})
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				// Overlapping ranges, each covering hour 24.
				from, to, qty := f.at(12+i), f.at(30+i), 1
				_, err := svc.Reserve(f.ctx, f.org, model.ReserveInput{
					ExtraUUID: &seat.UUID, BranchUUID: &f.branch.uuid, Quantity: &qty, StartsAt: &from, EndsAt: &to, Kind: model.KindHold,
				}, nil)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case isStock(err):
					refused++
				default:
					t.Errorf("worker %d: %v", i, err)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		if ok != 1 || refused != workers-1 {
			t.Fatalf("%d workers: %d succeeded, %d refused; want exactly one success", workers, ok, refused)
		}
		// The database agrees: the peak never passes the stock.
		av, err := f.svc.Availability(f.ctx, f.org, model.AvailabilityInput{BranchUUID: &f.branch.uuid, StartsAt: ptr(f.at(0)), EndsAt: ptr(f.at(60)), ExtraUUIDs: []uuid.UUID{seat.UUID}})
		if err != nil {
			t.Fatal(err)
		}
		if it := av.Items[0]; it.Stock != 2 || it.Available != 0 || it.Reserved != 1 || it.Held != 1 {
			t.Fatalf("availability = %+v", it)
		}
		// A range after the busy period is still sellable.
		if _, err := f.reserve(seat, f.branch, f.at(100), f.at(120), 2, model.KindHold); err != nil {
			t.Fatalf("later range: %v", err)
		}
	}
}

// The control experiment: without the stock row lock, the same race (every
// worker checks before anyone inserts) oversells. This is what the lock
// prevents in TestRT_A13_LastChildSeatConcurrency.
func TestRT_A13_WithoutLockOversells(t *testing.T) {
	const workers = 4
	f := newFx(t)
	seat := f.childSeat("SEAT", "100")
	f.setStock(seat, f.branch, 1)
	svc := usecase.New(f.pool, f.q)
	svc.DisableStockLock()
	var arrived sync.WaitGroup
	arrived.Add(workers)
	svc.SetAfterCheckHook(func() {
		arrived.Done()
		arrived.Wait() // every worker has passed the check
	})
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			from, to, qty := f.at(0), f.at(10), 1
			_, err := svc.Reserve(f.ctx, f.org, model.ReserveInput{
				ExtraUUID: &seat.UUID, BranchUUID: &f.branch.uuid, Quantity: &qty, StartsAt: &from, EndsAt: &to, Kind: model.KindHold,
			}, nil)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ok++
			}
		}()
	}
	wg.Wait()
	if ok != workers {
		t.Fatalf("without the lock %d of %d succeeded; the control experiment expects an oversell", ok, workers)
	}
}

// Holds lapse lazily, confirm keeps the same row, release frees once,
// stock below commitments is recorded with a warning.
func TestEquipmentHoldLifecycle(t *testing.T) {
	f := newFx(t)
	seat := f.childSeat("SEAT", "100")
	other := f.newBranch("ANK")
	f.setStock(seat, f.branch, 1)
	if _, err := f.reserve(seat, other, f.at(0), f.at(10), 1, model.KindHold); !isStock(err) {
		t.Fatalf("no stock row at ANK: want StockError, got %v", err)
	}
	minutes := 5
	h, err := f.svc.Reserve(f.ctx, f.org, model.ReserveInput{
		ExtraUUID: &seat.UUID, BranchUUID: &f.branch.uuid, StartsAt: ptr(f.at(0)), EndsAt: ptr(f.at(10)), HoldMinutes: &minutes,
	}, nil)
	if err != nil || h.Kind != model.KindHold || h.ExpiresAt == nil {
		t.Fatalf("hold = %+v %v", h, err)
	}
	if _, err := f.reserve(seat, f.branch, f.at(5), f.at(6), 1, model.KindHold); !isStock(err) {
		t.Fatalf("seat is held: want StockError, got %v", err)
	}
	// Ten minutes later the hold no longer counts (even before the sweep).
	later := usecase.New(f.pool, f.q)
	later.SetClock(func() time.Time { return time.Now().Add(10 * time.Minute) })
	if _, err := later.Confirm(f.ctx, f.org, h.UUID, model.ConfirmInput{}); !errors.Is(err, usecase.ErrHoldExpired) {
		t.Fatalf("confirm lapsed hold: %v", err)
	}
	r, err := later.Reserve(f.ctx, f.org, model.ReserveInput{
		ExtraUUID: &seat.UUID, BranchUUID: &f.branch.uuid, StartsAt: ptr(f.at(5)), EndsAt: ptr(f.at(6)), Kind: model.KindReservation,
	}, nil)
	if err != nil {
		t.Fatalf("after lapse: %v", err)
	}
	if n, err := later.ExpireDue(f.ctx); err != nil || n < 1 {
		t.Fatalf("sweep = %d %v", n, err)
	}
	if a, _ := f.svc.GetAllocation(f.ctx, f.org, h.UUID); a.Status != model.AllocExpired {
		t.Fatalf("swept hold = %+v", a)
	}
	// Confirm and release exactly once.
	h2, err := f.reserve(seat, f.branch, f.at(20), f.at(30), 1, model.KindHold)
	if err != nil {
		t.Fatal(err)
	}
	ref := uuid.New()
	c, err := f.svc.Confirm(f.ctx, f.org, h2.UUID, model.ConfirmInput{RefType: "reservation", RefUUID: &ref})
	if err != nil || c.Kind != model.KindReservation || c.ExpiresAt != nil || c.RefUUID == nil || *c.RefUUID != ref {
		t.Fatalf("confirm = %+v %v", c, err)
	}
	if _, err := f.svc.Confirm(f.ctx, f.org, h2.UUID, model.ConfirmInput{}); !errors.Is(err, usecase.ErrNotHold) {
		t.Fatalf("confirm twice: %v", err)
	}
	if _, err := f.svc.Release(f.ctx, f.org, r.UUID, "cancel", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Release(f.ctx, f.org, r.UUID, "cancel", nil); !errors.Is(err, usecase.ErrNotActive) {
		t.Fatalf("release twice: %v", err)
	}
	// Stock 0 while a reservation exists: recorded, with a warning.
	zero := 0
	res, err := f.svc.SetStock(f.ctx, f.org, seat.UUID, model.StockInput{Items: []model.StockItemInput{{BranchUUID: &f.branch.uuid, Quantity: &zero}}})
	if err != nil || len(res.Warnings) != 1 || res.Warnings[0].PeakCommitted != 1 {
		t.Fatalf("stock below commitments = %+v %v", res, err)
	}
	// A non-physical extra has no stock.
	gps, err := f.svc.Create(f.ctx, f.org, model.ExtraInput{Code: ptr("KM"), Name: ptr("Km"), ExtraType: ptr(model.TypeKmPackage), ChargeType: ptr(model.ChargePerUnit)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.reserve(gps, f.branch, f.at(0), f.at(1), 1, model.KindHold); err == nil {
		t.Fatal("reserved a non-physical extra")
	}
}

// Equipment tied to an availability hold follows it: extended, confirmed
// and released together (trigger on vehicle_allocations).
func TestEquipmentFollowsVehicleAllocation(t *testing.T) {
	f := newFx(t)
	seat := f.childSeat("SEAT", "100")
	f.setStock(seat, f.branch, 1)
	product := f.product("EGEA")
	var vehicle int64
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id) VALUES ($1, '34 EX 1', TRUE, 'Fiat', 'Egea', 2024, $2, $2) RETURNING id`, f.org, f.branch.id).Scan(&vehicle); err != nil {
		t.Fatal(err)
	}
	// Rental group membership is explicit: the vehicle joins the product's group.
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, id FROM rental_groups WHERE organization_id = $1`, f.org, vehicle); err != nil {
		t.Fatal(err)
	}
	av := avusecase.New(f.pool, f.q, vehicleusecase.New(vehiclerepository.NewPostgres(f.pool, f.q), nil),
		orgsettingsusecase.New(orgsettingsrepository.NewPostgres(f.pool, f.q)))
	hold := func() avmodel.PlanResult {
		t.Helper()
		res, err := av.Reserve(f.ctx, f.org, avmodel.ReserveInput{
			Kind: avmodel.KindHold, ProductUUID: product.uuid,
			Where: avmodel.Where{PickupBranchUUID: &f.branch.uuid, ReturnBranchUUID: &f.branch.uuid, StartsAt: f.at(0), EndsAt: f.at(24)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	vh := hold()
	link := func(va uuid.UUID) (model.Allocation, error) {
		return f.svc.Reserve(f.ctx, f.org, model.ReserveInput{ExtraUUID: &seat.UUID, VehicleAllocationUUID: &va}, nil)
	}
	eq, err := link(vh.Allocation.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if eq.Kind != model.KindHold || eq.VehicleAllocationUUID == nil || !eq.StartsAt.Equal(f.at(0)) || eq.Branch.UUID != f.branch.uuid ||
		eq.ExpiresAt == nil || vh.Allocation.ExpiresAt == nil || !eq.ExpiresAt.Equal(*vh.Allocation.ExpiresAt) {
		t.Fatalf("linked hold = %+v (vehicle %+v)", eq, vh.Allocation)
	}
	// Extend the vehicle hold -> same expiry.
	ext, err := av.ExtendHold(f.ctx, f.org, vh.Allocation.UUID, 120)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.svc.GetAllocation(f.ctx, f.org, eq.UUID); got.ExpiresAt == nil || !got.ExpiresAt.Equal(*ext.ExpiresAt) {
		t.Fatalf("extended = %+v, vehicle %v", got.ExpiresAt, ext.ExpiresAt)
	}
	// Confirm the vehicle hold -> the equipment becomes a reservation.
	res := uuid.New()
	if _, err := av.Confirm(f.ctx, f.org, vh.Allocation.UUID, "reservation", &res); err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.GetAllocation(f.ctx, f.org, eq.UUID)
	if got.Kind != model.KindReservation || got.ExpiresAt != nil || got.RefUUID == nil || *got.RefUUID != res {
		t.Fatalf("confirmed = %+v", got)
	}
	// Release the vehicle allocation -> the equipment is released too.
	if _, err := av.Release(f.ctx, f.org, vh.Allocation.UUID, "cancel", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = f.svc.GetAllocation(f.ctx, f.org, eq.UUID)
	if got.Status != model.AllocReleased || got.ReleaseReason != "vehicle_allocation_released" {
		t.Fatalf("released = %+v", got)
	}
	// A released vehicle allocation cannot take equipment.
	if _, err := link(vh.Allocation.UUID); err == nil {
		t.Fatal("linked to a released allocation")
	}
	// The seat is free again for a new hold.
	vh2 := hold()
	if _, err := link(vh2.Allocation.UUID); err != nil {
		t.Fatalf("seat after release: %v", err)
	}
}
