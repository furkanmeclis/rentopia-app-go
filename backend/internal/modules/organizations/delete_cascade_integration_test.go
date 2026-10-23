package organizations_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	finmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	finusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments/iyzico/iyzicofake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cascadeTables must hold rows of the filled organization before the delete
// (so the test really crosses the module boundaries); after it no table with
// an organization_id column may hold a row of that organization.
var cascadeTables = []string{
	"branches", "vehicle_segments", "vehicles", "vehicle_rental_groups",
	"rental_groups", "rental_products", "rental_product_versions",
	"rate_plans", "rate_plan_versions", "rate_bands",
	"date_rules", "date_rule_versions", "date_rule_version_targets",
	"organization_settings_versions", "customers", "drivers",
	"quotes", "reservations", "vehicle_allocations",
	"rentals", "rental_vehicles", "operation_tasks", "contract_templates", "documents", "document_versions",
	"integration_accounts", "payments", "accounts", "ledger_entries", "collections",
}

var cascadePerms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose,
	rbac.PermTenantContractsRead, rbac.PermTenantContractsWrite, rbac.PermTenantContractsActivate,
	rbac.PermTenantDamageApprove, rbac.PermTenantSignatureWaive, rbac.PermTenantDocumentDownload,
	rbac.PermTenantPaymentsRead, rbac.PermTenantPaymentsCreateLink, rbac.PermTenantPaymentCollect,
	rbac.PermTenantIntegrationsManage,
	rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite, rbac.PermTenantFinanceLedgerAdjust,
	rbac.PermTenantFinanceCashRead, rbac.PermTenantFinanceCashWrite,
}

type cascadeFx struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	org     int64
	segment int64
	group   int64
	branch  cascadeRef
	product cascadeRef
}

type cascadeRef struct {
	id   int64
	uuid uuid.UUID
}

func (f *cascadeFx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *cascadeFx) id(sql string, args ...any) int64 {
	f.t.Helper()
	var id int64
	f.must(f.pool.QueryRow(context.Background(), sql, args...).Scan(&id))
	return id
}

func (f *cascadeFx) exec(sql string, args ...any) {
	f.t.Helper()
	_, err := f.pool.Exec(context.Background(), sql, args...)
	f.must(err)
}

func (f *cascadeFx) newBranch(code string) cascadeRef {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: 30, IsActive: true,
	})
	f.must(err)
	return cascadeRef{b.ID, b.Uuid}
}

// newCascadeFx creates an organization with a branch, a segment, a rental
// group, a GROUP product (version + active rate plan) and settings.
func newCascadeFx(t *testing.T, pool *pgxpool.Pool, prefix string) *cascadeFx {
	t.Helper()
	ctx := context.Background()
	q := db.New(pool)
	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: prefix + "-" + suffix, Name: prefix + " " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// No-op once the test deleted it; keeps the database clean on failure.
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID)
	})
	f := &cascadeFx{t: t, pool: pool, q: q, org: org.ID}
	f.ctx = orgctx.WithScope(ctx, orgctx.Scope{
		ID: org.ID, UUID: org.Uuid, Slug: org.Slug, Status: orgctx.StatusActive, Permissions: cascadePerms,
	})
	f.segment = f.id(`INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org)
	f.branch = f.newBranch("MRK")
	f.must(pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, 'EKO', 'EKO ürün', $2, 'published') RETURNING id, uuid`, f.org, f.segment).Scan(&f.product.id, &f.product.uuid))
	f.group = f.id(`INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, 'GEKO', 'GEKO', $2) RETURNING id`, f.org, f.segment)
	f.exec(`INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years)
		VALUES ($1, $2, 1, 'GROUP', $3, 18, 1)`, f.org, f.product.id, f.group)
	plan := f.id(`INSERT INTO rate_plans (organization_id, rental_product_id, scope, name) VALUES ($1, $2, 'common', 'Tarife') RETURNING id`, f.org, f.product.id)
	planVersion := f.id(`INSERT INTO rate_plan_versions (organization_id, rate_plan_id, version) VALUES ($1, $2, 1) RETURNING id`, f.org, plan)
	f.exec(`INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, 1, NULL, 1000)`, f.org, planVersion)
	f.exec(`UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, planVersion)
	f.exec(`UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, plan)

	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	minutes := 60
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = &minutes, &minutes, &minutes
	s.Company.LegalName = "Test Oto A.Ş."
	data, _ := json.Marshal(s)
	f.exec(`INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	return f
}

// activeDateRule adds a date rule whose active (immutable) version targets
// the product, the segment and the given branch.
func (f *cascadeFx) activeDateRule(name string, branch int64) {
	rule := f.id(`INSERT INTO date_rules (organization_id, name) VALUES ($1, $2) RETURNING id`, f.org, name)
	version := f.id(`INSERT INTO date_rule_versions (organization_id, date_rule_id, version, start_date, end_date, product_scope, all_branches, kind, value)
		VALUES ($1, $2, 1, CURRENT_DATE, CURRENT_DATE + 30, 'products', FALSE, 'percent', 10) RETURNING id`, f.org, rule)
	f.exec(`INSERT INTO date_rule_version_targets (organization_id, date_rule_version_id, rental_product_id) VALUES ($1, $2, $3)`, f.org, version, f.product.id)
	f.exec(`INSERT INTO date_rule_version_targets (organization_id, date_rule_version_id, segment_id) VALUES ($1, $2, $3)`, f.org, version, f.segment)
	f.exec(`INSERT INTO date_rule_version_targets (organization_id, date_rule_version_id, branch_id) VALUES ($1, $2, $3)`, f.org, version, branch)
	f.exec(`UPDATE date_rule_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, version)
	f.exec(`UPDATE date_rules SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, rule)
}

func cascadePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// orgTables lists every base table with an organization_id column.
func orgTables(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT c.table_name FROM information_schema.columns c
		JOIN information_schema.tables t ON t.table_schema = c.table_schema AND t.table_name = c.table_name
		WHERE c.table_schema = current_schema() AND c.column_name = 'organization_id' AND t.table_type = 'BASE TABLE'
		ORDER BY c.table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func orgRows(t *testing.T, pool *pgxpool.Pool, table string, org int64) int {
	t.Helper()
	var n int
	sql := fmt.Sprintf("SELECT COUNT(*) FROM %q WHERE organization_id = $1", table)
	if err := pool.QueryRow(context.Background(), sql, org).Scan(&n); err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	return n
}

// TestOrganizationHardDeleteCascades fills an organization through the
// fleet, product, pricing, reservation, rental, contract, payment and
// finance modules, then deletes it with one statement: the delete succeeds
// and no organization-scoped table keeps a row of it.
func TestOrganizationHardDeleteCascades(t *testing.T) {
	pool := cascadePool(t)
	f := newCascadeFx(t, pool, "del")
	ctx := context.Background()
	q := f.q

	// Fleet: a vehicle in the rental group, a second branch, a date rule
	// (active version targeting the product, segment and branch).
	other := f.newBranch("HVL")
	var vehicle cascadeRef
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, '34 DEL 1', TRUE, 'Renault', 'Clio', 2024, $2, $2, 1000) RETURNING id, uuid`,
		f.org, f.branch.id).Scan(&vehicle.id, &vehicle.uuid))
	f.exec(`INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id) VALUES ($1, $2, $3)`, f.org, vehicle.id, f.group)
	f.activeDateRule("YAZ", other.id)

	var customer, driver cascadeRef
	f.must(pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', 'Ayşe', 'Yılmaz', '+905321112233', 'ayse@example.test') RETURNING id, uuid`, f.org).Scan(&customer.id, &customer.uuid))
	now := time.Now().UTC()
	f.must(pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, 'Ayşe', 'Yılmaz', $3, 'v1:x', $4, '****456', $5) RETURNING id, uuid`,
		f.org, customer.id, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&driver.id, &driver.uuid))

	// The modules, wired like the server.
	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	res := resusecase.New(pool, q, avail, pricing, settings)
	contracts := contractsusecase.New(pool, q, settings, storage.NewMemory())
	contracts.SetRenderer(cascadePDF{})
	rentals := rentalusecase.New(pool, q, avail, rentaladapters.NewReservations(res), vehicles)
	rentals.SetDocuments(contracts)
	res.SetLifecycleListener(rentals)

	fake := iyzicofake.New()
	t.Cleanup(fake.Server.Close)
	fake.AddAccount("sandbox-api-key-0001", "sandbox-secret-key-0001")
	fc, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	f.must(err)
	vault, err := integrations.NewVault(fc)
	f.must(err)
	adapter := iyzico.New(iyzico.Config{SandboxBaseURL: fake.URL(), LiveBaseURL: "http://127.0.0.1:1"})
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(adapter))
	pay := paymentsusecase.New(pool, q, accounts, res, paymentsusecase.Config{PublicBaseURL: "https://app.test"}, adapter)
	res.SetPaymentLinks(pay)
	_, err = accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
		Provider: iyzico.ProviderCode, DisplayName: "iyzico sandbox", Mode: integrations.ModeSandbox,
		Credentials: map[string]string{iyzico.FieldAPIKey: "sandbox-api-key-0001", iyzico.FieldSecretKey: "sandbox-secret-key-0001"},
	})
	f.must(err)
	fin := finusecase.New(pool, q, cascadeStore{m: storage.NewMemory()})
	rentals.SetLedger(fin)
	pay.SetLedger(fin)
	fin.SetPayments(pay)

	// Reservation (quote, allocation) → rental (documents, tasks) →
	// payment, deposit, handover (ledger accruals).
	start := time.Now().Add(2 * time.Hour).Truncate(time.Minute)
	end := start.Add(48 * time.Hour)
	quote, err := res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{
		Where:   resmodel.Where{PickupBranchUUID: &f.branch.uuid, StartsAt: &start, EndsAt: &end},
		Channel: "branch", ProductUUID: &f.product.uuid,
	})
	f.must(err)
	reservation, err := res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &quote.UUID, CustomerUUID: &customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: driver.uuid}},
	})
	f.must(err)
	rental, err := rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &reservation.UUID, VehicleUUID: &vehicle.uuid})
	f.must(err)
	amount := money.MustParse("500")
	_, err = pay.Collect(f.ctx, f.org, reservation.UUID, paymodel.CollectInput{Amount: &amount, Method: "cash"})
	f.must(err)
	ledger, err := fin.CustomerLedger(f.ctx, f.org, customer.uuid)
	f.must(err)
	if ledger.Account == nil {
		t.Fatal("the customer has no account after the payment")
	}
	deposit := money.MustParse("2000")
	_, err = fin.Collect(f.ctx, f.org, ledger.Account.UUID, finmodel.CollectionInput{
		Amount: &deposit, Purpose: finmodel.PurposeDeposit, Method: "card_pos", RentalUUID: &rental.UUID,
	})
	f.must(err)
	km, fuel := int32(1100), int16(80)
	_, err = rentals.Handover(f.ctx, f.org, rental.UUID, rentalmodel.HandoverInput{Km: &km, Fuel: &fuel})
	f.must(err)
	// A rental contract (documents, document versions) from an active
	// template; added after the handover so it does not wait for a signature.
	f.exec(`INSERT INTO contract_templates (organization_id, kind, name, version, status, activated_at)
		VALUES ($1, 'rental_contract', 'Kira sözleşmesi', 1, 'active', NOW())`, f.org)
	_, err = contracts.PrepareDocument(f.ctx, f.org, contractsmodel.PrepareInput{Kind: contractsmodel.KindRentalContract, ReservationUUID: &reservation.UUID})
	f.must(err)

	for _, table := range cascadeTables {
		if orgRows(t, pool, table, f.org) == 0 {
			t.Errorf("fixture left %s empty", table)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	tag, err := pool.Exec(ctx, "DELETE FROM organizations WHERE id = $1", f.org)
	if err != nil {
		t.Fatalf("organization delete: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("organization delete removed %d rows", tag.RowsAffected())
	}
	for _, table := range orgTables(t, pool) {
		if n := orgRows(t, pool, table, f.org); n != 0 {
			t.Errorf("%s keeps %d rows of the deleted organization", table, n)
		}
	}
}

// TestDeferredKeysStillProtect: the deferred keys (migration 000133) only
// move the check to commit. A rental group used by a product version, or a
// branch / segment / product selected by an activated date rule, still
// cannot be hard-deleted on its own.
func TestDeferredKeysStillProtect(t *testing.T) {
	pool := cascadePool(t)
	f := newCascadeFx(t, pool, "dfk")
	target := f.newBranch("TGT")
	f.activeDateRule("BAYRAM", target.id)

	cases := []struct {
		constraint string
		sql        string
		arg        int64
	}{
		{"fk_rental_product_versions_group", "DELETE FROM rental_groups WHERE organization_id = $1 AND id = $2", f.group},
		{"fk_date_rule_version_targets_branch", "DELETE FROM branches WHERE organization_id = $1 AND id = $2", target.id},
	}
	for _, c := range cases {
		_, err := pool.Exec(context.Background(), c.sql, f.org, c.arg)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" || pgErr.ConstraintName != c.constraint {
			t.Errorf("%s: want a %s violation, got %v", c.sql, c.constraint, err)
		}
	}
	if _, err := pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org); err != nil {
		t.Fatalf("organization delete: %v", err)
	}
}

type cascadePDF struct{}

func (cascadePDF) RenderPDF(_ context.Context, html []byte) ([]byte, error) {
	return append([]byte("%PDF-1.4\n"), html[:16]...), nil
}

// cascadeStore is the in-memory object store behind the finance module.
type cascadeStore struct {
	storage.Driver
	m *storage.Memory
}

func (d cascadeStore) Upload(ctx context.Context, f storage.File, path string) error {
	return d.m.Upload(ctx, f, path)
}

func (d cascadeStore) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, path)
}

func (d cascadeStore) Delete(ctx context.Context, path string) error { return d.m.Delete(ctx, path) }
