package usecase_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization (branches, vehicles, products, rate plans,
// customers) created with SQL and removed when the test ends; the shared
// TEST_DATABASE_URL database is never truncated.
type fx struct {
	t        testing.TB
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	avail    *availusecase.Service
	vehicles *vehicleusecase.Service
	svc      *usecase.Service
	org      int64
	orgUUID  uuid.UUID
	segment  int64
	suffix   string
	base     time.Time
	plate    int
	mrk      ref
	clockMu  sync.Mutex
	shift    time.Duration
	settings int
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

// allPerms are the tenant permissions of the test caller.
var allPerms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantReservationsExport, rbac.PermTenantReservationsOverrideEligibility,
	rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite, rbac.PermTenantResolutionTasksRead, rbac.PermTenantResolutionTasksWrite,
	rbac.PermTenantDiscountApply, rbac.PermTenantPriceOverride,
}

func dsn(t testing.TB) string {
	v := os.Getenv("TEST_DATABASE_URL")
	if v == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	return v
}

func newFx(t testing.TB) *fx {
	t.Helper()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn(t))
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
	f := &fx{t: t, pool: pool, q: q, suffix: suffix}
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "rsv-" + suffix, Name: "Rsv " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = f.scoped(nil, allPerms...)
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment); err != nil {
		t.Fatal(err)
	}
	f.vehicles = vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	f.avail, f.svc = f.services()
	f.base = time.Now().UTC().Add(72 * time.Hour).Truncate(time.Hour)
	f.mrk = f.branch("MRK", 60)
	return f
}

// services builds an engine + reservation service pair on the fixture
// clock (concurrency tests build several).
func (f *fx) services() (*availusecase.Service, *usecase.Service) {
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(f.pool, f.q))
	avail := availusecase.New(f.pool, f.q, f.vehicles, settings)
	avail.SetClock(f.now)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(f.pool, f.q), settings)
	svc := usecase.New(f.pool, f.q, avail, pricing, settings)
	svc.SetClock(f.now)
	return avail, svc
}

func (f *fx) now() time.Time {
	f.clockMu.Lock()
	defer f.clockMu.Unlock()
	return time.Now().Add(f.shift)
}

// advance moves the clock of both services forward.
func (f *fx) advance(d time.Duration) {
	f.clockMu.Lock()
	f.shift += d
	f.clockMu.Unlock()
}

// scoped is a request context of the organization with the given branch
// scope (nil = every branch) and permissions.
func (f *fx) scoped(branches []int64, perms ...string) context.Context {
	return orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "rsv-" + f.suffix, Status: orgctx.StatusActive, BranchIDs: branches, Permissions: perms,
	})
}

func (f *fx) at(hours int) time.Time { return f.base.Add(time.Duration(hours) * time.Hour) }

func (f *fx) branch(code string, prep int) ref {
	f.t.Helper()
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: int32(prep), IsActive: true,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return ref{b.ID, b.Uuid}
}

func (f *fx) vehicle(at ref) ref {
	f.t.Helper()
	f.plate++
	var r ref
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id) VALUES ($1, $2, TRUE, 'Fiat', 'Egea', 2024, $3, $3) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 RS %d", f.plate), at.id).Scan(&r.id, &r.uuid); err != nil {
		f.t.Fatal(err)
	}
	return r
}

// product creates a published GROUP product whose group has no criteria
// (every vehicle keeps it) with the given minimum driver age, and an
// active common rate plan with bands 1-3 / 4-7 / 8+ days.
func (f *fx) product(code string, minAge int, prices ...string) ref {
	f.t.Helper()
	ctx := context.Background()
	var p ref
	if err := f.pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, $2, $3, $4, 'published') RETURNING id, uuid`, f.org, code, code+" ürün", f.segment).Scan(&p.id, &p.uuid); err != nil {
		f.t.Fatal(err)
	}
	var group int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, $2, $2, $3) RETURNING id`,
		f.org, "G"+code, f.segment).Scan(&group); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years)
		VALUES ($1, $2, 1, 'GROUP', $3, $4, 1)`, f.org, p.id, group, minAge); err != nil {
		f.t.Fatal(err)
	}
	if len(prices) == 0 {
		prices = []string{"1500", "1300", "1000"}
	}
	f.plan(p, "common", 0, prices...)
	return p
}

// plan inserts an active rate plan (scope common, or branch with branchID).
func (f *fx) plan(p ref, scope string, branchID int64, prices ...string) {
	f.t.Helper()
	ctx := context.Background()
	var planID, versionID int64
	var branch any
	if branchID != 0 {
		branch = branchID
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO rate_plans (organization_id, rental_product_id, scope, branch_id, name)
		VALUES ($1, $2, $3, $4, 'Tarife') RETURNING id`, f.org, p.id, scope, branch).Scan(&planID); err != nil {
		f.t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO rate_plan_versions (organization_id, rate_plan_id, version) VALUES ($1, $2, 1) RETURNING id`,
		f.org, planID).Scan(&versionID); err != nil {
		f.t.Fatal(err)
	}
	bands := [][2]any{{1, 3}, {4, 7}, {8, nil}}
	for i, price := range prices {
		if _, err := f.pool.Exec(ctx, `INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, $3, $4, $5)`,
			f.org, versionID, bands[i][0], bands[i][1], price); err != nil {
			f.t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, versionID); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, planID); err != nil {
		f.t.Fatal(err)
	}
}

// configure stores a settings version (approval mode, windows, prepayment).
func (f *fx) configure(mutate func(s *osmodel.Settings)) {
	f.t.Helper()
	s := osmodel.Defaults()
	mutate(&s)
	data, _ := json.Marshal(s)
	f.settings++
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, $2, $3)`,
		f.org, f.settings, data); err != nil {
		f.t.Fatal(err)
	}
}

func intp(v int) *int { return &v }

// customer creates an individual customer with one driver born `age` years
// ago holding a licence for `licence` years.
func (f *fx) customer(age, licence int) (ref, ref) {
	f.t.Helper()
	ctx := context.Background()
	var c, d ref
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', 'Ayşe', 'Yılmaz', '+905321112233', 'ayse@example.test') RETURNING id, uuid`, f.org).Scan(&c.id, &c.uuid); err != nil {
		f.t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := f.pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, 'Ayşe', 'Yılmaz', $3, 'v1:x', $4, '****456', $5) RETURNING id, uuid`,
		f.org, c.id, now.AddDate(-age, 0, -10), uuid.NewString(), now.AddDate(-licence, 0, -10)).Scan(&d.id, &d.uuid); err != nil {
		f.t.Fatal(err)
	}
	return c, d
}

func (f *fx) where(from, to time.Time) model.Where {
	b := f.mrk.uuid
	return model.Where{PickupBranchUUID: &b, StartsAt: &from, EndsAt: &to}
}

func (f *fx) quote(ctx context.Context, svc *usecase.Service, p ref, from, to time.Time) model.Quote {
	f.t.Helper()
	q, err := svc.CreateQuote(ctx, f.org, model.QuoteInput{Where: f.where(from, to), Channel: "branch", ProductUUID: &p.uuid})
	if err != nil {
		f.t.Fatalf("quote: %v", err)
	}
	return q
}

// reserve quotes and creates a reservation for the customer.
func (f *fx) reserve(p ref, c, d ref, from, to time.Time) model.Reservation {
	f.t.Helper()
	q := f.quote(f.ctx, f.svc, p, from, to)
	r, err := f.svc.Create(f.ctx, f.org, model.CreateInput{
		QuoteUUID: &q.UUID, CustomerUUID: &c.uuid, Drivers: []model.DriverInput{{DriverUUID: d.uuid}},
	})
	if err != nil {
		f.t.Fatalf("create: %v", err)
	}
	return r
}

// allocation reads the allocation of a reservation.
func (f *fx) allocation(id uuid.UUID) (string, string, int64) {
	f.t.Helper()
	var kind, status string
	var vehicle int64
	if err := f.pool.QueryRow(context.Background(), `SELECT a.kind, a.status, a.vehicle_id FROM reservations r
		JOIN vehicle_allocations a ON a.organization_id = r.organization_id AND a.id = r.allocation_id
		WHERE r.organization_id = $1 AND r.uuid = $2`, f.org, id).Scan(&kind, &status, &vehicle); err != nil {
		f.t.Fatal(err)
	}
	return kind, status, vehicle
}

// activeAllocations counts the organization's active allocations.
func (f *fx) activeAllocations() int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM vehicle_allocations WHERE organization_id = $1 AND status = 'active'`, f.org).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *fx) events(id uuid.UUID) []string {
	f.t.Helper()
	items, _, err := f.svc.Events(f.ctx, f.org, id, 100, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	out := []string{}
	for i := len(items) - 1; i >= 0; i-- {
		out = append(out, items[i].Action)
	}
	return out
}
