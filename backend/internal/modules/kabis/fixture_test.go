package kabis_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	rentaladapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/adapters"
	rentalmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis/kabisfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with two branches (with a city), a product,
// settings, a customer and a driver with encrypted identity / licence
// numbers, the rental chain (reservations → rentals on the availability
// engine) and the KABİS service with the fake adapter on the integration
// runner (no queue: the first step runs inline).
type fx struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	cipher   *crypto.FieldCipher
	res      *resusecase.Service
	rentals  *rentalusecase.Service
	kabis    *usecase.Service
	fake     *kabisfake.Provider
	accounts *accountsusecase.Service
	runner   *jobsusecase.Runner
	store    *storage.Memory
	org      int64
	orgUUID  uuid.UUID
	suffix   string
	segment  int64
	branch   ref
	other    ref
	product  ref
	customer ref
	driver   ref
	tckn     string
	licence  string
	plate    int
	clockMu  sync.Mutex
	shift    time.Duration
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var perms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose,
	rbac.PermTenantKabisRead, rbac.PermTenantKabisRetry, rbac.PermTenantKabisReconcile,
	rbac.PermTenantVehiclesRead, rbac.PermTenantVehiclesWrite,
}

// newFx builds the fixture; connect = a kabis_fake account is connected.
func newFx(t *testing.T, connect bool) *fx {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(pool)
	f := &fx{t: t, pool: pool, q: q, suffix: strings.ReplaceAll(uuid.NewString()[:8], "-", "")}
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "kbs-" + f.suffix, Name: "Kbs " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM integration_jobs WHERE organization_id = $1", f.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = orgctx.WithScope(ctx, orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "kbs-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms,
	})
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment))

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	avail.SetClock(f.now)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	f.res = resusecase.New(pool, q, avail, pricing, settings)
	f.res.SetClock(f.now)
	f.rentals = rentalusecase.New(pool, q, avail, rentaladapters.NewReservations(f.res), vehicles)
	f.rentals.SetClock(f.now)
	f.res.SetLifecycleListener(f.rentals)

	f.cipher, err = crypto.NewFieldCipher(strings.Repeat("t", 32))
	f.must(err)
	vault, err := integrations.NewVault(f.cipher)
	f.must(err)
	f.fake = kabisfake.New()
	providers := []kabis.Provider{kabis.Official{}, f.fake}
	f.accounts = accountsusecase.New(q, vault, integrations.NewRegistry(kabis.Official{}, f.fake))
	ops := integrations.NewOperationRegistry()
	f.runner = jobsusecase.NewRunner(q, f.accounts, ops)
	f.runner.SetClock(f.now)
	f.kabis, err = usecase.New(pool, q, f.cipher, f.runner, ops, providers...)
	f.must(err)
	f.kabis.SetClock(f.now)
	f.store = storage.NewMemory()
	f.kabis.SetStore(f.store)
	f.rentals.SetKabis(f.kabis)

	f.branch = f.newBranch("MRK", "İstanbul")
	f.other = f.newBranch("HVL", "Ankara")
	f.product = f.newProduct("EKO")
	f.tckn = "1" + strconv.Itoa(1000000000+int(time.Now().UnixNano()%899999999))
	f.licence = "B" + f.suffix
	f.customer, f.driver = f.newCustomer(true)
	f.configure(func(s *osmodel.Settings) {
		s.Reservation.ApprovalMode = osmodel.ApprovalAuto
		s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = intp(60), intp(60), intp(30)
		s.Company.LegalName = "Test Oto A.Ş."
	})
	if connect {
		f.connect(kabisfake.ProviderCode)
	}
	return f
}

func intp(v int) *int    { return &v }
func i32(v int32) *int32 { return &v }
func i16(v int16) *int16 { return &v }

func (f *fx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fx) now() time.Time {
	f.clockMu.Lock()
	defer f.clockMu.Unlock()
	return time.Now().Add(f.shift)
}

// connect adds an active KABİS account of the provider.
func (f *fx) connect(provider string) uuid.UUID {
	f.t.Helper()
	a, err := f.accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
		Kind: kabis.Kind, Provider: provider, DisplayName: "KABİS " + provider,
		Credentials: map[string]string{"username": "u-" + f.suffix, "password": "p-" + f.suffix},
	})
	f.must(err)
	return a.UUID
}

func (f *fx) newBranch(code, city string) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", City: city, Address: code + " Cad. No:1",
		WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"), PrepMinutes: 60, IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

// vehicle adds a vehicle at branch b; it returns the ref and the plate.
func (f *fx) vehicle(b ref) (ref, string) {
	f.plate++
	plate := fmt.Sprintf("34 KB %d%s", f.plate, strings.ToUpper(f.suffix[:3]))
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Fiat', 'Egea', 2025, $3, $3, 1000) RETURNING id, uuid`,
		f.org, plate, b.id).Scan(&r.id, &r.uuid))
	_, err := f.pool.Exec(context.Background(), `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, g.id FROM rental_groups g WHERE g.organization_id = $1 ON CONFLICT DO NOTHING`, f.org, r.id)
	f.must(err)
	return r, plate
}

func (f *fx) newProduct(code string) ref {
	ctx := context.Background()
	var p ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, $2, $3, $4, 'published') RETURNING id, uuid`, f.org, code, code+" ürün", f.segment).Scan(&p.id, &p.uuid))
	var group int64
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, $2, $2, $3) RETURNING id`,
		f.org, "G"+code, f.segment).Scan(&group))
	_, err := f.pool.Exec(ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years)
		VALUES ($1, $2, 1, 'GROUP', $3, 18, 1)`, f.org, p.id, group)
	f.must(err)
	var planID, versionID int64
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rate_plans (organization_id, rental_product_id, scope, name) VALUES ($1, $2, 'common', 'Tarife') RETURNING id`,
		f.org, p.id).Scan(&planID))
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rate_plan_versions (organization_id, rate_plan_id, version) VALUES ($1, $2, 1) RETURNING id`,
		f.org, planID).Scan(&versionID))
	_, err = f.pool.Exec(ctx, `INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, 1, NULL, '1000')`,
		f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, planID)
	f.must(err)
	return p
}

func (f *fx) configure(mutate func(s *osmodel.Settings)) {
	s := osmodel.Defaults()
	mutate(&s)
	data, _ := json.Marshal(s)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	f.must(err)
}

func (f *fx) enc(column, v string) string {
	out, err := f.cipher.Encrypt(v, column+"|org:"+strconv.FormatInt(f.org, 10))
	f.must(err)
	return out
}

// newCustomer adds an individual customer with a TCKN and a driver (the
// customer); withDriverIdentity = the driver has an identity number.
func (f *fx) newCustomer(withDriverIdentity bool) (ref, ref) {
	ctx := context.Background()
	var c, d ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email,
		identity_type, identity_enc, identity_bidx, identity_hint, nationality)
		VALUES ($1, 'individual', 'Deniz', 'Kaya', '+905321112244', 'deniz@example.test', 'tckn', $2, $3, '123******45', 'TR') RETURNING id, uuid`,
		f.org, f.enc("customers.identity", f.tckn), uuid.NewString()).Scan(&c.id, &c.uuid))
	now := time.Now().UTC()
	idType, idEnc, idBidx := any(nil), "", ""
	if withDriverIdentity {
		idType, idEnc, idBidx = "tckn", f.enc("drivers.identity", f.tckn), uuid.NewString()
	}
	f.must(f.pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, is_customer_self, first_name, last_name, birth_date,
		identity_type, identity_enc, identity_bidx, identity_hint, nationality,
		licence_enc, licence_bidx, licence_hint, licence_class, licence_issued_on, licence_country)
		VALUES ($1, $2, TRUE, 'Deniz', 'Kaya', $3, $4, $5, $6, '123******45', 'TR', $7, $8, '****789', 'B', $9, 'TR') RETURNING id, uuid`,
		f.org, c.id, now.AddDate(-30, 0, 0), idType, idEnc, idBidx, f.enc("drivers.licence", f.licence), uuid.NewString(),
		now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return c, d
}

// reserve books vehicle v at branch b from now+1h for two days and
// creates the rental (preparation).
func (f *fx) reserve(v ref, b ref, customer, driver ref) (resmodel.Reservation, rentalmodel.Rental) {
	f.t.Helper()
	start := f.now().Add(time.Hour).Truncate(time.Minute)
	end := start.Add(48 * time.Hour)
	bu := b.uuid
	quote, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{
		Where: resmodel.Where{PickupBranchUUID: &bu, StartsAt: &start, EndsAt: &end}, Channel: "branch", ProductUUID: &f.product.uuid,
	})
	f.must(err)
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &quote.UUID, CustomerUUID: &customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: driver.uuid}},
	})
	f.must(err)
	created, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &r.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	return r, created
}

// handover hands the rental over now at its branch.
func (f *fx) handover(rental uuid.UUID) rentalmodel.Rental {
	f.t.Helper()
	out, err := f.rentals.Handover(f.ctx, f.org, rental, rentalmodel.HandoverInput{Km: i32(1100), Fuel: i16(90)})
	f.must(err)
	return out
}

// notifications lists the rental's notifications (oldest first).
func (f *fx) notifications(rental uuid.UUID) []model.Notification {
	f.t.Helper()
	items, _, err := f.kabis.List(f.ctx, f.org, model.ListFilter{RentalUUID: &rental, SortField: "created_at", Limit: 50})
	f.must(err)
	return items
}

func (f *fx) only(rental uuid.UUID, event string) model.Notification {
	f.t.Helper()
	var out []model.Notification
	for _, n := range f.notifications(rental) {
		if n.Event == event {
			out = append(out, n)
		}
	}
	if len(out) != 1 {
		f.t.Fatalf("want exactly one %s notification, got %d: %+v", event, len(out), out)
	}
	return out[0]
}

func (f *fx) get(id uuid.UUID) model.Detail {
	f.t.Helper()
	d, err := f.kabis.Get(f.ctx, f.org, id)
	f.must(err)
	return d
}

// step runs one runner step of the notification's job (the sweep).
func (f *fx) step(n model.Notification) {
	f.t.Helper()
	var jobID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM integration_jobs WHERE organization_id = $1 AND uuid = $2`, f.org, n.JobUUID).Scan(&jobID))
	_, err := f.runner.Process(context.Background(), f.org, jobID, jobsusecase.TriggerSweep)
	f.must(err)
}

func (f *fx) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	f.must(f.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}
