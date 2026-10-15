package usecase_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with its own branches, catalog models, vehicles and
// products, created with SQL and removed when the test ends (the shared
// TEST_DATABASE_URL database is never truncated).
type fx struct {
	t       testing.TB
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	svc     *usecase.Service
	org     int64
	segment int64
	suffix  string
	base    time.Time
	brands  []int64
	display []int64
	plate   int
}

type ref struct {
	id   int64
	uuid uuid.UUID
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
	cfg.MaxConns = 16
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(pool)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	f := &fx{t: t, ctx: ctx, pool: pool, q: q, suffix: suffix}
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "avail-" + suffix, Name: "Avail " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org = org.ID
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", f.org)
		_, _ = pool.Exec(bg, "DELETE FROM catalog_brands WHERE id = ANY($1)", f.brands)
		_, _ = pool.Exec(bg, "DELETE FROM catalog_display_brands WHERE id = ANY($1)", f.display)
		pool.Close()
	})
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment); err != nil {
		t.Fatal(err)
	}
	f.svc = f.newService()
	// Requests start two days ahead on a whole hour.
	f.base = time.Now().UTC().Add(48 * time.Hour).Truncate(time.Hour)
	return f
}

func (f *fx) newService() *usecase.Service {
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(f.pool, f.q), nil)
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(f.pool, f.q))
	return usecase.New(f.pool, f.q, vehicles, settings)
}

// at is base + hours.
func (f *fx) at(hours int) time.Time { return f.base.Add(time.Duration(hours) * time.Hour) }

func (f *fx) branch(code string, prep int) ref {
	f.t.Helper()
	b, err := f.q.CreateBranch(f.ctx, db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: int32(prep), IsActive: true,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return ref{b.ID, b.Uuid}
}

// model creates a catalog model with one type (own display brand) and
// returns (model id, type id).
func (f *fx) model(name string) (int64, int64) {
	f.t.Helper()
	code := strings.ToUpper(name[:2]) + f.suffix
	var display, brand, mdl, typ int64
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO catalog_display_brands (name, slug) VALUES ($1, $2) RETURNING id`,
		name+" "+f.suffix, strings.ToLower(code)).Scan(&display); err != nil {
		f.t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO catalog_brands (tsb_code, tsb_name, display_name, display_brand_id) VALUES ($1, $2, $2, $3) RETURNING id`,
		code, name, display).Scan(&brand); err != nil {
		f.t.Fatal(err)
	}
	f.display, f.brands = append(f.display, display), append(f.brands, brand)
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO catalog_models (brand_id, name, source) VALUES ($1, $2, 'manual') RETURNING id`, brand, name).Scan(&mdl); err != nil {
		f.t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO catalog_types (brand_id, model_id, tsb_type_code, raw_name, first_seen_period, last_seen_period)
		VALUES ($1, $2, $3, $4, '2026-01', '2026-09') RETURNING id`, brand, mdl, "T"+code, strings.ToUpper(name)+" 1.4").Scan(&typ); err != nil {
		f.t.Fatal(err)
	}
	return mdl, typ
}

// vehicle creates a live vehicle at branch (catalog type, or manual when
// typeID is 0).
func (f *fx) vehicle(typeID int64, at ref, pool string) ref {
	f.t.Helper()
	f.plate++
	plate := "34 AV " + itoa(f.plate)
	var r ref
	var err error
	if typeID == 0 {
		err = f.pool.QueryRow(f.ctx, `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
			home_branch_id, current_branch_id, fleet_pool) VALUES ($1, $2, TRUE, 'Fiat', 'Manual', 2024, $3, $3, $4) RETURNING id, uuid`,
			f.org, plate, at.id, pool).Scan(&r.id, &r.uuid)
	} else {
		err = f.pool.QueryRow(f.ctx, `INSERT INTO vehicles (organization_id, plate, catalog_type_id, model_year,
			home_branch_id, current_branch_id, fleet_pool) VALUES ($1, $2, $3, 2024, $4, $4, $5) RETURNING id, uuid`,
			f.org, plate, typeID, at.id, pool).Scan(&r.id, &r.uuid)
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}

// product creates a published product with one version. kind GROUP takes
// catalog model ids as the group's criteria (none = every vehicle);
// EXACT_VARIANT takes one model id.
func (f *fx) product(code, kind string, models ...int64) ref {
	f.t.Helper()
	var p ref
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, $2, $3, $4, 'published') RETURNING id, uuid`, f.org, code, code+" ürün", f.segment).Scan(&p.id, &p.uuid); err != nil {
		f.t.Fatal(err)
	}
	switch kind {
	case "GROUP":
		var group int64
		if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, $2, $2, $3) RETURNING id`,
			f.org, "G"+code, f.segment).Scan(&group); err != nil {
			f.t.Fatal(err)
		}
		for _, m := range models {
			if _, err := f.pool.Exec(f.ctx, `INSERT INTO rental_group_catalog_models (organization_id, rental_group_id, catalog_model_id) VALUES ($1, $2, $3)`,
				f.org, group, m); err != nil {
				f.t.Fatal(err)
			}
		}
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id)
			VALUES ($1, $2, 1, 'GROUP', $3)`, f.org, p.id, group); err != nil {
			f.t.Fatal(err)
		}
	case "EXACT_VARIANT":
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, variant_level, catalog_model_id)
			VALUES ($1, $2, 1, 'EXACT_VARIANT', 'model', $3)`, f.org, p.id, models[0]); err != nil {
			f.t.Fatal(err)
		}
	default:
		f.t.Fatalf("unknown guarantee %s", kind)
	}
	return p
}

func (f *fx) block(v ref, from, to time.Time) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO vehicle_blocks (organization_id, vehicle_id, kind, during)
		VALUES ($1, $2, 'maintenance', tstzrange($3, $4, '[)')) RETURNING uuid`, f.org, v.id, from, to).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fx) allowOneWay(from, to ref) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO one_way_rules (organization_id, from_branch_id, to_branch_id, allowed, fee) VALUES ($1, $2, $3, TRUE, 0)`,
		f.org, from.id, to.id); err != nil {
		f.t.Fatal(err)
	}
}

func where(pickup, ret ref, from, to time.Time) model.Where {
	p, r := pickup.uuid, ret.uuid
	return model.Where{PickupBranchUUID: &p, ReturnBranchUUID: &r, StartsAt: from, EndsAt: to}
}

func (f *fx) search(w model.Where, products ...ref) map[uuid.UUID]model.ProductAvailability {
	f.t.Helper()
	ids := []uuid.UUID{}
	for _, p := range products {
		ids = append(ids, p.uuid)
	}
	res, err := f.svc.Search(f.ctx, f.org, model.SearchInput{Where: w, ProductUUIDs: ids})
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[uuid.UUID]model.ProductAvailability{}
	for _, item := range res.Items {
		out[item.Product.UUID] = item
	}
	return out
}

func (f *fx) hold(p ref, w model.Where) (model.PlanResult, error) {
	return f.svc.Reserve(f.ctx, f.org, model.ReserveInput{Where: w, ProductUUID: p.uuid, Kind: model.KindHold})
}

func (f *fx) mustHold(p ref, w model.Where) model.PlanResult {
	f.t.Helper()
	res, err := f.hold(p, w)
	if err != nil {
		f.t.Fatalf("hold %v: %v", w.StartsAt, err)
	}
	return res
}

func isCapacity(err error) bool {
	_, ok := usecase.IsCapacity(err)
	return ok
}

// vehicleOf reads the current plate plan of an allocation.
func (f *fx) vehicleOf(a uuid.UUID) int64 {
	f.t.Helper()
	var id int64
	if err := f.pool.QueryRow(f.ctx, `SELECT vehicle_id FROM vehicle_allocations WHERE organization_id = $1 AND uuid = $2`, f.org, a).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// invariants checks the database state independently of the engine: no
// two active allocations of a vehicle overlap, every active allocation's
// vehicle keeps its product's guarantee (eligible[product] = vehicles), and
// location continuity holds per vehicle (each pick-up starts where the
// previous active allocation returned, or the vehicle's current branch).
func (f *fx) invariants(eligible map[int64]map[int64]bool) {
	f.t.Helper()
	f.invariantsExcept(eligible, nil)
}

// invariantsExcept skips the continuity check of allocations a release
// reported as affected (by design they are left for a person to re-plan).
func (f *fx) invariantsExcept(eligible map[int64]map[int64]bool, affected map[uuid.UUID]bool) {
	f.t.Helper()
	var overlaps int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM vehicle_allocations a JOIN vehicle_allocations b
		ON a.organization_id = b.organization_id AND a.vehicle_id = b.vehicle_id AND a.id < b.id AND a.period && b.period
		WHERE a.organization_id = $1 AND a.status = 'active' AND b.status = 'active'`, f.org).Scan(&overlaps); err != nil {
		f.t.Fatal(err)
	}
	if overlaps != 0 {
		f.t.Fatalf("%d double bookings", overlaps)
	}
	rows, err := f.pool.Query(f.ctx, `SELECT a.uuid, a.vehicle_id, a.rental_product_id, a.pickup_branch_id, a.return_branch_id, v.current_branch_id
		FROM vehicle_allocations a JOIN vehicles v ON v.organization_id = a.organization_id AND v.id = a.vehicle_id
		WHERE a.organization_id = $1 AND a.status = 'active' ORDER BY a.vehicle_id, lower(a.period)`, f.org)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	loc := map[int64]int64{}
	for rows.Next() {
		var id uuid.UUID
		var vehicle, product, pickup, ret, current int64
		if err := rows.Scan(&id, &vehicle, &product, &pickup, &ret, &current); err != nil {
			f.t.Fatal(err)
		}
		if eligible != nil && !eligible[product][vehicle] {
			f.t.Fatalf("vehicle %d does not keep product %d's guarantee", vehicle, product)
		}
		if _, ok := loc[vehicle]; !ok {
			loc[vehicle] = current
		}
		if loc[vehicle] != pickup && !affected[id] {
			f.t.Fatalf("vehicle %d: pick-up at %d but it is at %d", vehicle, pickup, loc[vehicle])
		}
		loc[vehicle] = ret
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

var errAny = errors.New("any")
