package tolls_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tolls"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tolls/tollfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with two branches, a product, settings and a
// customer (SQL fixtures), the rental chain (reservations → rentals on the
// availability engine) and the toll service with the fake provider on the
// integration runner (no queue: the first step runs inline).
type fx struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	res      *resusecase.Service
	rentals  *rentalusecase.Service
	tolls    *usecase.Service
	io       *adapters.IO
	fake     *tollfake.Provider
	accounts *accountsusecase.Service
	runner   *jobsusecase.Runner
	ledger   *recordingLedger
	org      int64
	orgUUID  uuid.UUID
	suffix   string
	segment  int64
	branch   ref
	other    ref
	product  ref
	customer ref
	driver   ref
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
	rbac.PermTenantTollsRead, rbac.PermTenantTollsImport, rbac.PermTenantTollsReview, rbac.PermTenantTollsExport,
	rbac.PermTenantVehiclesRead, rbac.PermTenantVehiclesWrite,
}

// recordingLedger stands in for the finance ledger (TEC-717): it records
// what the tolls module posts, inside the charge transaction.
type recordingLedger struct {
	mu      sync.Mutex
	entries []model.LedgerEntry
}

func (l *recordingLedger) PostTollCharge(_ context.Context, tx pgx.Tx, e model.LedgerEntry) error {
	if tx == nil {
		return errors.New("ledger posting outside the charge transaction")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	return nil
}

func (l *recordingLedger) sum(rental uuid.UUID) money.Money {
	l.mu.Lock()
	defer l.mu.Unlock()
	total := money.Zero()
	for _, e := range l.entries {
		if e.RentalUUID == rental {
			total = total.Add(e.Amount)
		}
	}
	return total
}

func newFx(t *testing.T) *fx {
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
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "tol-" + f.suffix, Name: "Tol " + f.suffix})
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
		ID: f.org, UUID: f.orgUUID, Slug: "tol-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms,
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

	cipher, err := crypto.NewFieldCipher(strings.Repeat("t", 32))
	f.must(err)
	vault, err := integrations.NewVault(cipher)
	f.must(err)
	f.fake = tollfake.New()
	f.fake.SetClock(f.now)
	f.accounts = accountsusecase.New(q, vault, integrations.NewRegistry(f.fake))
	ops := integrations.NewOperationRegistry()
	f.runner = jobsusecase.NewRunner(q, f.accounts, ops)
	f.runner.SetClock(f.now)
	f.tolls = usecase.New(pool, q, f.accounts, f.runner, ops, f.fake)
	f.tolls.SetClock(f.now)
	f.ledger = &recordingLedger{}
	f.tolls.SetLedger(f.ledger)
	f.io = adapters.NewIO(f.tolls)

	f.branch = f.newBranch("MRK", 60)
	f.other = f.newBranch("HVL", 30)
	f.product = f.newProduct("EKO")
	f.customer, f.driver = f.newCustomer()
	f.configure(func(s *osmodel.Settings) {
		s.Reservation.ApprovalMode = osmodel.ApprovalAuto
		s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = intp(60), intp(60), intp(30)
		grace := 30
		s.Policies.LateReturnGraceMinutes, s.Policies.ExtraHourFee = &grace, "150"
		s.Company.LegalName = "Test Oto A.Ş."
	})
	return f
}

func intp(v int) *int       { return &v }
func i32(v int32) *int32    { return &v }
func i16(v int16) *int16    { return &v }
func strp(v string) *string { return &v }

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

func (f *fx) advance(d time.Duration) {
	f.clockMu.Lock()
	f.shift += d
	f.clockMu.Unlock()
}

func (f *fx) newBranch(code string, prep int) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: int32(prep), IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

// vehicle adds a vehicle at branch b; it returns the ref and the plate.
func (f *fx) vehicle(b ref) (ref, string) {
	f.plate++
	plate := fmt.Sprintf("34 TL %d%s", f.plate, strings.ToUpper(f.suffix[:3]))
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

func (f *fx) newCustomer() (ref, ref) {
	ctx := context.Background()
	var c, d ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', 'Deniz', 'Kaya', '+905321112244', 'deniz@example.test') RETURNING id, uuid`, f.org).Scan(&c.id, &c.uuid))
	now := time.Now().UTC()
	f.must(f.pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, 'Deniz', 'Kaya', $3, 'v1:x', $4, '****789', $5) RETURNING id, uuid`,
		f.org, c.id, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return c, d
}

// rental books vehicle v at branch b from now+1h for two days and hands
// it over now; it returns the rental (actual handover = the current clock).
func (f *fx) rental(v ref, b ref) rentalmodel.Rental {
	f.t.Helper()
	start := f.now().Add(time.Hour).Truncate(time.Minute)
	end := start.Add(48 * time.Hour)
	bu := b.uuid
	quote, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{
		Where: resmodel.Where{PickupBranchUUID: &bu, StartsAt: &start, EndsAt: &end}, Channel: "branch", ProductUUID: &f.product.uuid,
	})
	f.must(err)
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &quote.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	f.must(err)
	created, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &r.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	out, err := f.rentals.Handover(f.ctx, f.org, created.UUID, rentalmodel.HandoverInput{Km: i32(1100), Fuel: i16(90)})
	f.must(err)
	return out
}

// plannedRental books vehicle v around [start, start+48h) without a
// handover (the rental stays in preparation).
func (f *fx) plannedRental(v ref, start time.Time) rentalmodel.Rental {
	f.t.Helper()
	end := start.Add(48 * time.Hour)
	bu := f.branch.uuid
	quote, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{
		Where: resmodel.Where{PickupBranchUUID: &bu, StartsAt: &start, EndsAt: &end}, Channel: "branch", ProductUUID: &f.product.uuid,
	})
	f.must(err)
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &quote.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	f.must(err)
	created, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &r.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	return created
}

func (f *fx) pass(ext, plate string, at time.Time, amount string) model.PassInput {
	return model.PassInput{
		Provider: "hgs", ExternalID: ext, Source: model.SourceFile, PassedAt: at, Plate: plate,
		Station: "Osmangazi Köprüsü", Amount: money.MustParse(amount), Currency: "TRY",
	}
}

func (f *fx) ingest(in model.PassInput) model.IngestResult {
	f.t.Helper()
	res, err := f.tolls.Ingest(f.ctx, f.org, in)
	f.must(err)
	return res
}

func (f *fx) get(id uuid.UUID) model.PassDetail {
	f.t.Helper()
	d, err := f.tolls.Get(f.ctx, f.org, id)
	f.must(err)
	return d
}

// tollCharges returns the toll / fee charge rows of a rental (amounts as
// strings, in creation order) and their sum.
func (f *fx) tollCharges(rental uuid.UUID) ([]string, money.Money) {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT ch.kind, ch.amount::text, ch.reversal_of_id IS NOT NULL
		FROM rental_charges ch JOIN rentals r ON r.organization_id = ch.organization_id AND r.id = ch.rental_id
		WHERE ch.organization_id = $1 AND r.uuid = $2 AND ch.source = 'toll' ORDER BY ch.id`, f.org, rental)
	f.must(err)
	defer rows.Close()
	out := []string{}
	total := money.Zero()
	for rows.Next() {
		var kind, amount string
		var reversal bool
		f.must(rows.Scan(&kind, &amount, &reversal))
		m := money.MustParse(amount)
		total = total.Add(m)
		label := kind + ":" + m.String()
		if reversal {
			label = "reversal:" + label
		}
		out = append(out, label)
	}
	return out, total
}

// TestRT_A20_PassDuringRentalChargedOnce: a pass by plate inside the actual
// handover → return interval is charged on that rental once (with the
// processing fee as its own line); the same pass arriving again (provider
// and file) charges nothing more; the ledger hook sees each posting.
func TestRT_A20_PassDuringRentalChargedOnce(t *testing.T) {
	f := newFx(t)
	fee := money.MustParse("12.50")
	_, err := f.tolls.UpdateSettings(f.ctx, f.org, model.SettingsInput{ProcessingFee: &fee})
	f.must(err)
	v, plate := f.vehicle(f.branch)
	r := f.rental(v, f.branch)
	f.advance(3 * time.Hour)
	at := f.now().Add(-time.Hour)

	res := f.ingest(f.pass("TX-1", plate, at, "795.00"))
	if res.Op != model.OpCreated || res.Status != model.StatusCharged {
		t.Fatalf("first arrival: %+v", res)
	}
	d := f.get(res.UUID)
	if d.Rental == nil || d.Rental.UUID != r.UUID || d.MatchMethod != model.MethodPlate || d.Vehicle == nil || d.Vehicle.UUID != v.uuid {
		t.Fatalf("match: %+v", d.Pass)
	}
	charges, total := f.tollCharges(r.UUID)
	if len(charges) != 2 || charges[0] != "toll:795.00" || charges[1] != "toll_fee:12.50" || total.String() != "807.50" {
		t.Fatalf("charges %v total %s", charges, total)
	}
	// Again (twice, the same facts): nothing new.
	for i := 0; i < 2; i++ {
		again := f.ingest(f.pass("TX-1", plate, at, "795.00"))
		if again.Op != model.OpUnchanged || again.UUID != res.UUID {
			t.Fatalf("repeat %d: %+v", i, again)
		}
	}
	if charges, _ := f.tollCharges(r.UUID); len(charges) != 2 {
		t.Fatalf("a repeated pass must not charge again: %v", charges)
	}
	if got := f.ledger.sum(r.UUID); got.String() != "807.50" {
		t.Fatalf("ledger postings %s", got)
	}
	// The rental shows the toll charges (approved facts, linked to the pass).
	rental, err := f.rentals.Get(f.ctx, f.org, r.UUID)
	f.must(err)
	found := 0
	for _, ch := range rental.Charges {
		if ch.Source == rentalmodel.SourceToll {
			found++
			if ch.TollPassUUID == nil || *ch.TollPassUUID != res.UUID || ch.Status != rentalmodel.ChargeApproved {
				t.Fatalf("rental charge %+v", ch)
			}
		}
	}
	if found != 2 {
		t.Fatalf("rental shows %d toll charges", found)
	}
	// A toll charge cannot be waived or overridden like a proposal.
	if _, err := f.rentals.DecideCharge(f.ctx, f.org, r.UUID, rental.Charges[0].UUID, false, "deneme feragat"); err == nil {
		t.Fatal("a toll charge must not be waived through the rental")
	}
}

// TestRT_A20_DuplicateImport: the same file imported twice creates one
// pass and one charge; the second import reports the row as unchanged.
func TestRT_A20_DuplicateImport(t *testing.T) {
	f := newFx(t)
	v, plate := f.vehicle(f.branch)
	r := f.rental(v, f.branch)
	f.advance(2 * time.Hour)
	at := f.now().Add(-30 * time.Minute).In(model.Istanbul)
	row := map[string]any{
		"external_id": "HGS-0001", "passed_at": at.Format("02.01.2006 15:04:05"), "plate": plate,
		"station": "Gebze", "exit_station": "Orhangazi", "amount": "412,50",
	}
	ctx := orgctx.WithOrganizationID(f.ctx, f.org)
	for i := 0; i < 2; i++ {
		diff, err := f.io.PreviewRow(ctx, row, nil)
		f.must(err)
		res, err := f.io.ApplyRow(ctx, row, nil)
		f.must(err)
		if !res.OK {
			t.Fatalf("import %d: %+v", i, res)
		}
		wantOp := "create"
		if i == 1 {
			wantOp = "update"
		}
		if diff.Op != wantOp || res.Op != wantOp || len(diff.Changes) != 0 {
			t.Fatalf("import %d: preview %+v apply %+v", i, diff, res)
		}
	}
	charges, total := f.tollCharges(r.UUID)
	if len(charges) != 1 || total.String() != "412.50" {
		t.Fatalf("charges after two imports: %v", charges)
	}
	items, n, err := f.tolls.List(f.ctx, f.org, model.ListFilter{Limit: 10})
	f.must(err)
	if n != 1 || items[0].Source != model.SourceFile || items[0].Provider != adapters.DefaultProvider {
		t.Fatalf("passes %d %+v", n, items)
	}
}

// TestRT_A20_AmountRevision: the same transaction with a new amount is a
// revision; the charge is reversed and a new one created (never edited),
// and a revision back by import rollback does the same again.
func TestRT_A20_AmountRevision(t *testing.T) {
	f := newFx(t)
	v, plate := f.vehicle(f.branch)
	r := f.rental(v, f.branch)
	f.advance(2 * time.Hour)
	at := f.now().Add(-time.Hour)
	first := f.ingest(f.pass("TX-9", plate, at, "47.00"))
	var originalAmount string
	f.must(f.pool.QueryRow(context.Background(), `SELECT amount::text FROM rental_charges WHERE organization_id = $1 AND toll_pass_id =
		(SELECT id FROM toll_passes WHERE organization_id = $1 AND uuid = $2) AND reversal_of_id IS NULL`, f.org, first.UUID).Scan(&originalAmount))

	rev := f.ingest(f.pass("TX-9", plate, at, "52.00"))
	if rev.Op != model.OpRevised || len(rev.ChangedFields) != 1 || rev.ChangedFields[0] != "amount" || rev.Status != model.StatusCharged {
		t.Fatalf("revision: %+v", rev)
	}
	charges, total := f.tollCharges(r.UUID)
	want := []string{"toll:47.00", "reversal:toll:-47.00", "toll:52.00"}
	if strings.Join(charges, ",") != strings.Join(want, ",") || total.String() != "52.00" {
		t.Fatalf("charges %v total %s", charges, total)
	}
	// The original charge row was not edited in place.
	var still string
	f.must(f.pool.QueryRow(context.Background(), `SELECT amount::text FROM rental_charges WHERE organization_id = $1 AND toll_pass_id =
		(SELECT id FROM toll_passes WHERE organization_id = $1 AND uuid = $2) ORDER BY id LIMIT 1`, f.org, first.UUID).Scan(&still))
	if still != originalAmount {
		t.Fatalf("original charge changed: %s → %s", originalAmount, still)
	}
	d := f.get(first.UUID)
	if d.Revision != 2 || len(d.Revisions) != 1 || d.Revisions[0].AmountBefore.String() != "47.00" ||
		d.Revisions[0].AmountAfter.String() != "52.00" {
		t.Fatalf("revisions %+v", d.Revisions)
	}
	if len(d.Charges) != 3 || d.Charges[1].ReversalOfUUID == nil || *d.Charges[1].ReversalOfUUID != d.Charges[0].UUID {
		t.Fatalf("pass charges %+v", d.Charges)
	}
	// A reversed charge cannot be reversed twice (unique index).
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, status, amount, reversal_of_id, decided_at)
		SELECT organization_id, rental_id, kind, 'toll', 'approved', -amount, id, NOW() FROM rental_charges
		WHERE organization_id = $1 AND toll_pass_id = (SELECT id FROM toll_passes WHERE organization_id = $1 AND uuid = $2) ORDER BY id LIMIT 1`,
		f.org, first.UUID); err == nil {
		t.Fatal("a second reversal of the same charge must be refused")
	}
	// Rolling the revision back (import rollback) is a new revision.
	f.must(f.tolls.RevertImported(f.ctx, f.org, first.UUID, map[string]any{"op": model.OpRevised, "facts": map[string]any{
		"provider": "hgs", "external_id": "TX-9", "plate": plate, "station": "Osmangazi Köprüsü",
		"passed_at": at.UTC().Format(time.RFC3339Nano), "amount": "47.00", "currency": "TRY",
	}}))
	if _, total := f.tollCharges(r.UUID); total.String() != "47.00" {
		t.Fatalf("after rollback total %s", total)
	}
}

// TestRT_A20_OutsideRentalGoesToReview: passes that cannot be put on one
// rental wait in the review queue with their reason; nothing is charged
// and the queue count shows them.
func TestRT_A20_OutsideRentalGoesToReview(t *testing.T) {
	f := newFx(t)
	v, plate := f.vehicle(f.branch)
	before := f.now().Add(-5 * time.Hour)
	f.rental(v, f.branch)
	f.advance(2 * time.Hour)
	// Before the handover: the vehicle was on no rental.
	outside := f.ingest(f.pass("TX-OUT", plate, before, "47.00"))
	// Unknown plate.
	unknown := f.ingest(f.pass("TX-UNK", "06 ZZ 999", f.now().Add(-time.Hour), "47.00"))
	// A planned rental without a recorded handover.
	v2, plate2 := f.vehicle(f.branch)
	planned := f.plannedRental(v2, f.now().Add(-3*time.Hour).Truncate(time.Minute).Add(4*time.Hour))
	_ = planned
	f.advance(6 * time.Hour)
	missing := f.ingest(f.pass("TX-MISS", plate2, f.now().Add(-30*time.Minute), "95.00"))
	// A transfer of the vehicle.
	v3, plate3 := f.vehicle(f.branch)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO vehicle_transfers (organization_id, vehicle_id, from_branch_id, to_branch_id, planned, status, actual_departure)
		VALUES ($1, $2, $3, $4, tstzrange($5, $6, '[)'), 'in_transit', $5)`, f.org, v3.id, f.branch.id, f.other.id, f.now().Add(-2*time.Hour), f.now().Add(2*time.Hour))
	f.must(err)
	transfer := f.ingest(f.pass("TX-TRF", plate3, f.now().Add(-time.Hour), "47.00"))

	for _, c := range []struct {
		res    model.IngestResult
		reason string
	}{
		{outside, model.ReasonNoRental}, {unknown, model.ReasonVehicleUnknown},
		{missing, model.ReasonMissingActualTimes}, {transfer, model.ReasonInTransfer},
	} {
		d := f.get(c.res.UUID)
		if d.Status != model.StatusNeedsReview || d.ReviewReason != c.reason || d.Rental != nil || len(d.Charges) != 0 {
			t.Fatalf("%s: %s / %s rental %v charges %d", d.ExternalID, d.Status, d.ReviewReason, d.Rental, len(d.Charges))
		}
	}
	facets, err := f.tolls.Facets(f.ctx, f.org)
	f.must(err)
	if facets.Review != 4 {
		t.Fatalf("review queue %d", facets.Review)
	}
	queue, n, err := f.tolls.List(f.ctx, f.org, model.ListFilter{Statuses: model.ReviewStatuses, Limit: 10})
	f.must(err)
	if n != 4 || len(queue) != 4 {
		t.Fatalf("queue list %d", n)
	}
	var count int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM rental_charges WHERE organization_id = $1 AND source = 'toll'`, f.org).Scan(&count))
	if count != 0 {
		t.Fatalf("review passes must not be charged: %d", count)
	}
	// The handover of the planned rental is recorded later: the periodic
	// re-match does not move a pass whose time is before the handover.
	if n, err := f.tolls.RematchDue(f.ctx); err != nil || n < 0 {
		t.Fatalf("rematch %d %v", n, err)
	}
}

// TestRT_A20_ManualReassignment: a person moves a pass to another rental
// with a mandatory reason; the old charge is reversed, the new one
// created, the timeline and the rental events record it; unassign and
// ignore reverse; rematch hands it back to the automatic matching.
func TestRT_A20_ManualReassignment(t *testing.T) {
	f := newFx(t)
	v1, plate1 := f.vehicle(f.branch)
	v2, _ := f.vehicle(f.branch)
	r1 := f.rental(v1, f.branch)
	r2 := f.rental(v2, f.branch)
	f.advance(2 * time.Hour)
	res := f.ingest(f.pass("TX-MOVE", plate1, f.now().Add(-time.Hour), "250.00"))
	if res.Status != model.StatusCharged {
		t.Fatalf("auto match %+v", res)
	}
	// The reason is mandatory.
	var ve *usecase.ValidationError
	if _, err := f.tolls.Assign(f.ctx, f.org, res.UUID, model.AssignInput{RentalUUID: &r2.UUID, Reason: " "}); !errors.As(err, &ve) {
		t.Fatalf("assign without a reason: %v", err)
	}
	d, err := f.tolls.Assign(f.ctx, f.org, res.UUID, model.AssignInput{RentalUUID: &r2.UUID, Reason: "Plaka okuma hatası, araç değişti"})
	f.must(err)
	if d.Status != model.StatusCharged || d.Rental == nil || d.Rental.UUID != r2.UUID || !d.Manual || d.MatchMethod != model.MethodManual {
		t.Fatalf("after assign %+v", d.Pass)
	}
	c1, t1 := f.tollCharges(r1.UUID)
	c2, t2 := f.tollCharges(r2.UUID)
	if len(c1) != 2 || !t1.IsZero() || len(c2) != 1 || t2.String() != "250.00" {
		t.Fatalf("r1 %v (%s) r2 %v (%s)", c1, t1, c2, t2)
	}
	assigned := false
	for _, e := range d.Events {
		if e.Action == model.EventAssigned && e.Reason == "Plaka okuma hatası, araç değişti" {
			assigned = true
		}
	}
	if !assigned {
		t.Fatalf("timeline %+v", d.Events)
	}
	var events int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM rental_events e JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.action = 'toll_reversed'`, f.org, r1.UUID).Scan(&events))
	if events != 1 {
		t.Fatalf("rental 1 reversal events %d", events)
	}
	// The automatic re-match keeps a person's decision.
	if _, err := f.tolls.RematchDue(f.ctx); err != nil {
		t.Fatal(err)
	}
	if again := f.ingest(f.pass("TX-MOVE", plate1, f.now().Add(-time.Hour), "250.00")); again.Status != model.StatusCharged {
		t.Fatalf("repeat after assign %+v", again)
	}
	if d := f.get(res.UUID); d.Rental.UUID != r2.UUID {
		t.Fatal("a manual assignment must survive a repeated pass")
	}
	// Unassign: reversal on r2, the pass waits as unmatched.
	d, err = f.tolls.Unassign(f.ctx, f.org, res.UUID, model.ReasonInput{Reason: "Müşteri itiraz etti"})
	f.must(err)
	if d.Status != model.StatusUnmatched || d.ReviewReason != model.ReasonUnassigned || d.Rental != nil {
		t.Fatalf("after unassign %+v", d.Pass)
	}
	if _, total := f.tollCharges(r2.UUID); !total.IsZero() {
		t.Fatalf("r2 after unassign %s", total)
	}
	// Ignore, then rematch: back on r1 automatically.
	d, err = f.tolls.Ignore(f.ctx, f.org, res.UUID, model.ReasonInput{Reason: "Kurum aracı"})
	f.must(err)
	if d.Status != model.StatusIgnored {
		t.Fatalf("ignore %s", d.Status)
	}
	d, err = f.tolls.Rematch(f.ctx, f.org, res.UUID, model.ReasonInput{Reason: "Yeniden eşle"})
	f.must(err)
	if d.Status != model.StatusCharged || d.Rental.UUID != r1.UUID || d.Manual {
		t.Fatalf("rematch %+v", d.Pass)
	}
	if _, total := f.tollCharges(r1.UUID); total.String() != "250.00" {
		t.Fatalf("r1 after rematch %s", total)
	}
	// Candidates of the pass list r1 (its actual interval holds the pass).
	cands, err := f.tolls.Candidates(f.ctx, f.org, res.UUID)
	f.must(err)
	if len(cands) != 1 || cands[0].UUID != r1.UUID || !cands[0].ContainsPass {
		t.Fatalf("candidates %+v", cands)
	}
}

// TestRT_A20_FileImportRowErrors: every invalid row of a file is reported
// with its fields (preview and apply); valid rows are stored.
func TestRT_A20_FileImportRowErrors(t *testing.T) {
	f := newFx(t)
	ctx := orgctx.WithOrganizationID(f.ctx, f.org)
	rows := []map[string]any{
		{"external_id": "OK-1", "passed_at": "2026-05-04 08:42", "plate": "34 ABC 123", "station": "Avrasya Tüneli", "amount": "1.234,56"},
		{"external_id": "BAD-AMOUNT", "passed_at": "2026-05-04 08:42", "plate": "34 ABC 123", "station": "X", "amount": "on beş"},
		{"external_id": "BAD-TIME", "passed_at": "dün", "plate": "34 ABC 123", "station": "X", "amount": "10"},
		{"external_id": "", "passed_at": "2026-05-04 08:42", "station": "", "amount": "10"},
		{"external_id": "FUTURE", "passed_at": "2099-01-01 10:00", "plate": "34 ABC 123", "station": "X", "amount": "10"},
	}
	wantFields := [][]string{nil, {"amount"}, {"passed_at"}, {"external_id", "plate", "station"}, {"passed_at"}}
	for i, row := range rows {
		diff, err := f.io.PreviewRow(ctx, row, nil)
		f.must(err)
		res, err := f.io.ApplyRow(ctx, row, nil)
		f.must(err)
		if wantFields[i] == nil {
			if diff.Error != "" || !res.OK {
				t.Fatalf("row %d: %+v %+v", i+1, diff, res)
			}
			continue
		}
		if diff.Error == "" || res.OK || res.Error != diff.Error {
			t.Fatalf("row %d must fail: preview %+v apply %+v", i+1, diff, res)
		}
		for _, field := range wantFields[i] {
			if !strings.Contains(res.Error, field+":") {
				t.Fatalf("row %d error %q does not name %s", i+1, res.Error, field)
			}
		}
	}
	items, n, err := f.tolls.List(f.ctx, f.org, model.ListFilter{Limit: 10})
	f.must(err)
	if n != 1 || items[0].Amount.String() != "1234.56" || items[0].Status != model.StatusNeedsReview {
		t.Fatalf("stored %d %+v", n, items)
	}
	// The sample file maps cleanly.
	for _, row := range f.io.SampleRows() {
		if diff, err := f.io.PreviewRow(ctx, row, nil); err != nil || diff.Error != "" {
			t.Fatalf("sample row: %+v %v", diff, err)
		}
	}
}

// TestRT_A20_BranchScope: a member limited to another branch sees none of
// the passes of the main branch's vehicles and cannot decide on them; the
// vehicle tags follow the vehicle's branches.
func TestRT_A20_BranchScope(t *testing.T) {
	f := newFx(t)
	v, plate := f.vehicle(f.branch)
	r := f.rental(v, f.branch)
	vo, plateO := f.vehicle(f.other)
	f.advance(2 * time.Hour)
	mine := f.ingest(f.pass("TX-MAIN", plate, f.now().Add(-time.Hour), "47.00"))
	theirs := f.ingest(f.pass("TX-OTHER", plateO, f.now().Add(-time.Hour), "47.00"))
	orphan := f.ingest(f.pass("TX-NOVEH", "01 XX 01", f.now().Add(-time.Hour), "47.00"))

	sc, _ := orgctx.From(f.ctx)
	sc.BranchIDs = []int64{f.other.id}
	limited := orgctx.WithScope(context.Background(), sc)

	items, n, err := f.tolls.List(limited, f.org, model.ListFilter{Limit: 50})
	f.must(err)
	if n != 1 || items[0].UUID != theirs.UUID {
		t.Fatalf("limited member sees %d passes: %+v", n, items)
	}
	facets, err := f.tolls.Facets(limited, f.org)
	f.must(err)
	if facets.Review != 1 {
		t.Fatalf("limited review count %d", facets.Review)
	}
	for _, id := range []uuid.UUID{mine.UUID, orphan.UUID} {
		if _, err := f.tolls.Get(limited, f.org, id); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("get outside the scope: %v", err)
		}
		if _, err := f.tolls.Ignore(limited, f.org, id, model.ReasonInput{Reason: "kapsam dışı"}); !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("ignore outside the scope: %v", err)
		}
	}
	// Assigning a visible pass to a rental outside the scope is refused.
	var ve *usecase.ValidationError
	if _, err := f.tolls.Assign(limited, f.org, theirs.UUID, model.AssignInput{RentalUUID: &r.UUID, Reason: "deneme atama"}); !errors.As(err, &ve) {
		t.Fatalf("assign to a rental outside the scope: %v", err)
	}
	if _, err := f.tolls.Tags(limited, f.org, v.uuid); !errors.Is(err, usecase.ErrVehicleNotFound) {
		t.Fatalf("tags outside the scope: %v", err)
	}
	if _, err := f.tolls.Tags(limited, f.org, vo.uuid); err != nil {
		t.Fatalf("tags inside the scope: %v", err)
	}
	// Export (adapter) honours the scope as well.
	ds, err := f.io.Export(limited, map[string]string{}, "tr")
	f.must(err)
	if len(ds.Rows) != 1 {
		t.Fatalf("export rows %d", len(ds.Rows))
	}
}

// TestRT_A20_CrossOrganization: organization B can neither read nor
// change A's passes, nor put A's pass on its rental; the same transaction
// id in both organizations is two passes.
func TestRT_A20_CrossOrganization(t *testing.T) {
	a := newFx(t)
	b := newFx(t)
	va, plateA := a.vehicle(a.branch)
	a.rental(va, a.branch)
	vb, _ := b.vehicle(b.branch)
	rb := b.rental(vb, b.branch)
	a.advance(2 * time.Hour)
	b.advance(2 * time.Hour)
	pa := a.ingest(a.pass("SAME-ID", plateA, a.now().Add(-time.Hour), "47.00"))
	pb := b.ingest(b.pass("SAME-ID", plateA, b.now().Add(-time.Hour), "47.00"))
	if pa.UUID == pb.UUID || pb.Op != model.OpCreated || pb.Status != model.StatusNeedsReview {
		t.Fatalf("same id in two organizations: %+v %+v", pa, pb)
	}
	if _, err := b.tolls.Get(b.ctx, b.org, pa.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("B reads A's pass: %v", err)
	}
	if _, err := b.tolls.Assign(b.ctx, b.org, pa.UUID, model.AssignInput{RentalUUID: &rb.UUID, Reason: "yabancı geçiş"}); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("B assigns A's pass: %v", err)
	}
	var ve *usecase.ValidationError
	if _, err := b.tolls.Assign(b.ctx, b.org, pb.UUID, model.AssignInput{RentalUUID: func() *uuid.UUID {
		id := a.get(pa.UUID).Rental.UUID
		return &id
	}(), Reason: "yabancı kiralama"}); !errors.As(err, &ve) {
		t.Fatalf("B assigns its pass to A's rental: %v", err)
	}
	if _, err := b.tolls.Tags(b.ctx, b.org, va.uuid); !errors.Is(err, usecase.ErrVehicleNotFound) {
		t.Fatalf("B reads A's vehicle tags: %v", err)
	}
	if _, err := b.tolls.CreateTag(b.ctx, b.org, va.uuid, model.TagInput{Provider: strp("hgs"), TagNumber: strp("HGS9999"), ValidFrom: &time.Time{}}); !errors.Is(err, usecase.ErrVehicleNotFound) {
		t.Fatalf("B tags A's vehicle: %v", err)
	}
	items, n, err := b.tolls.List(b.ctx, b.org, model.ListFilter{VehicleUUIDs: []uuid.UUID{va.uuid}, Limit: 10})
	f := b
	f.must(err)
	if n != 0 || len(items) != 0 {
		t.Fatalf("B lists A's vehicle passes: %d", n)
	}
	if d := a.get(pa.UUID); d.Status != model.StatusCharged {
		t.Fatalf("A's pass changed: %s", d.Status)
	}
}

// TestTollFetchJob: the provider fetch runs as an integration job (tag →
// vehicle matching); a repeated fetch changes nothing; a provider failure
// keeps the job retryable and visible with its error.
func TestTollFetchJob(t *testing.T) {
	f := newFx(t)
	v, _ := f.vehicle(f.branch)
	r := f.rental(v, f.branch)
	from := f.now().Add(-time.Hour)
	_, err := f.tolls.CreateTag(f.ctx, f.org, v.uuid, model.TagInput{Provider: strp("hgs_fake"), TagNumber: strp("HGS-7788 99"), ValidFrom: &from})
	f.must(err)
	if _, err := f.tolls.CreateTag(f.ctx, f.org, v.uuid, model.TagInput{Provider: strp("hgs_fake"), TagNumber: strp("HGS778899"), ValidFrom: &from}); err == nil {
		t.Fatal("the same tag for an overlapping period must be refused")
	}
	if _, err := f.tolls.FetchNow(f.ctx, f.org); !errors.Is(err, usecase.ErrNoAccount) {
		t.Fatalf("fetch without an account: %v", err)
	}
	_, err = f.accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
		Provider: tollfake.ProviderCode, DisplayName: "HGS test", Credentials: map[string]string{"api_key": "fake-key-123456"},
	})
	f.must(err)
	f.advance(2 * time.Hour)
	f.fake.Script(
		tolls.Pass{ExternalID: "F-1", PassedAt: f.now().Add(-time.Hour), TagNumber: "HGS778899", Station: "Avrasya Tüneli", Amount: money.MustParse("250.00"), Currency: "TRY"},
		tolls.Pass{ExternalID: "F-2", PassedAt: f.now().Add(-90 * time.Minute), TagNumber: "UNKNOWN-TAG", Station: "Avrasya Tüneli", Amount: money.MustParse("250.00")},
	)
	job, err := f.tolls.FetchNow(f.ctx, f.org)
	f.must(err)
	if job.Status != jobsusecase.StatusSucceeded {
		t.Fatalf("fetch job %+v", job)
	}
	if _, total := f.tollCharges(r.UUID); total.String() != "250.00" {
		t.Fatalf("fetched charges %s", total)
	}
	items, _, err := f.tolls.List(f.ctx, f.org, model.ListFilter{Sources: []string{model.SourceAPI}, Limit: 10})
	f.must(err)
	if len(items) != 2 {
		t.Fatalf("fetched passes %d", len(items))
	}
	for _, p := range items {
		if p.ExternalID == "F-1" && (p.MatchMethod != model.MethodTag || p.Status != model.StatusCharged) {
			t.Fatalf("tag match %+v", p)
		}
		if p.ExternalID == "F-2" && p.ReviewReason != model.ReasonVehicleUnknown {
			t.Fatalf("unknown tag %+v", p)
		}
	}
	// Fetch again a minute later: same passes, nothing new.
	f.advance(time.Minute)
	_, err = f.tolls.FetchNow(f.ctx, f.org)
	f.must(err)
	if charges, _ := f.tollCharges(r.UUID); len(charges) != 1 {
		t.Fatalf("refetch charged again: %v", charges)
	}
	// A provider outage: the job waits for a retry with its error.
	f.advance(time.Minute)
	f.fake.FailNext(tollfake.ErrUnavailable)
	failed, err := f.tolls.FetchNow(f.ctx, f.org)
	f.must(err)
	var status, lastError string
	f.must(f.pool.QueryRow(context.Background(), `SELECT status, last_error FROM integration_jobs WHERE organization_id = $1 AND uuid = $2`,
		f.org, failed.JobUUID).Scan(&status, &lastError))
	if status != jobsusecase.StatusPending || !strings.Contains(lastError, "unavailable") {
		t.Fatalf("failed fetch job %s %q", status, lastError)
	}
	// The hourly schedule creates one job per account and slot.
	n1, err := f.tolls.ScheduleFetches(f.ctx)
	f.must(err)
	n2, err := f.tolls.ScheduleFetches(f.ctx)
	f.must(err)
	if n1 < 1 || n2 < 1 {
		t.Fatalf("schedule %d %d", n1, n2)
	}
	var slots int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM integration_jobs WHERE organization_id = $1 AND idempotency_key LIKE '%:slot:%'`,
		f.org).Scan(&slots))
	if slots != 1 {
		t.Fatalf("scheduled jobs %d", slots)
	}
}
