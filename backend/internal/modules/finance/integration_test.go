package finance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
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
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with branches, a product, a customer, rentals,
// payments (iyzico fake) and finance wired together like the server.
type fx struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	res      *resusecase.Service
	rentals  *rentalusecase.Service
	pay      *paymentsusecase.Service
	fin      *usecase.Service
	fake     *iyzicofake.Fake
	accounts *accountsusecase.Service
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
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var allPerms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose,
	rbac.PermTenantDamageApprove, rbac.PermTenantSignatureWaive, rbac.PermTenantPriceOverride,
	rbac.PermTenantPaymentsRead, rbac.PermTenantPaymentsCreateLink, rbac.PermTenantPaymentCollect, rbac.PermTenantPaymentRefund,
	rbac.PermTenantIntegrationsManage,
	rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite, rbac.PermTenantFinanceLedgerAdjust,
	rbac.PermTenantFinanceCashRead, rbac.PermTenantFinanceCashWrite, rbac.PermTenantFinanceExpensesRead,
	rbac.PermTenantFinanceExpensesWrite,
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
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "fin-" + f.suffix, Name: "Fin " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	fake := iyzicofake.New()
	f.fake = fake
	t.Cleanup(func() {
		fake.Server.Close()
		// Ledger rows are append-only: only the organization cascade
		// removes them.
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = f.scoped(nil)
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment))

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	f.res = resusecase.New(pool, q, avail, pricing, settings)
	f.rentals = rentalusecase.New(pool, q, avail, rentaladapters.NewReservations(f.res), vehicles)
	f.res.SetLifecycleListener(f.rentals)

	fake.AddAccount("sandbox-api-key-0001", "sandbox-secret-key-0001")
	fc, err := crypto.NewFieldCipher(strings.Repeat("k", 32))
	f.must(err)
	vault, err := integrations.NewVault(fc)
	f.must(err)
	adapter := iyzico.New(iyzico.Config{SandboxBaseURL: fake.URL(), LiveBaseURL: "http://127.0.0.1:1"})
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(adapter))
	f.accounts = accounts
	f.pay = paymentsusecase.New(pool, q, accounts, f.res, paymentsusecase.Config{PublicBaseURL: "https://app.test"}, adapter)
	f.res.SetPaymentLinks(f.pay)
	if _, err := accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
		Provider: iyzico.ProviderCode, DisplayName: "iyzico sandbox", Mode: integrations.ModeSandbox,
		Credentials: map[string]string{iyzico.FieldAPIKey: "sandbox-api-key-0001", iyzico.FieldSecretKey: "sandbox-secret-key-0001"},
	}); err != nil {
		t.Fatal(err)
	}

	f.fin = usecase.New(pool, q, memDriver{m: storage.NewMemory()})
	f.rentals.SetLedger(f.fin)
	f.pay.SetLedger(f.fin)

	f.branch = f.newBranch("MRK")
	f.other = f.newBranch("HVL")
	f.product = f.newProduct("EKO")
	f.customer, f.driver = f.newCustomer("Ayşe", "Yılmaz")
	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = intp(60), intp(60), intp(30)
	data, _ := json.Marshal(s)
	_, err = pool.Exec(ctx, `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	f.must(err)
	return f
}

func intp(v int) *int { return &v }

func (f *fx) scoped(branches []int64, perms ...string) context.Context {
	if len(perms) == 0 {
		perms = allPerms
	}
	return orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "fin-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms, BranchIDs: branches,
	})
}

func (f *fx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fx) newBranch(code string) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte("{}"),
		HolidayExceptions: []byte("[]"), PrepMinutes: 30, IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

func (f *fx) vehicle() ref {
	f.plate++
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 FN %d", f.plate), f.branch.id).Scan(&r.id, &r.uuid))
	_, err := f.pool.Exec(context.Background(), `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, g.id FROM rental_groups g WHERE g.organization_id = $1 ON CONFLICT DO NOTHING`, f.org, r.id)
	f.must(err)
	return r
}

func (f *fx) newProduct(code string) ref {
	ctx := context.Background()
	var p ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status)
		VALUES ($1, $2, $3, $4, 'published') RETURNING id, uuid`, f.org, code, code+" ürün", f.segment).Scan(&p.id, &p.uuid))
	var group int64
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rental_groups (organization_id, code, name, segment_id) VALUES ($1, $2, $2, $3) RETURNING id`,
		f.org, "G"+code, f.segment).Scan(&group))
	// The product asks for a 2 000 deposit (security deposits, TEC-719).
	_, err := f.pool.Exec(ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id, min_driver_age, min_license_years,
		deposit_amount) VALUES ($1, $2, 1, 'GROUP', $3, 18, 1, 2000)`, f.org, p.id, group)
	f.must(err)
	var planID, versionID int64
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rate_plans (organization_id, rental_product_id, scope, name) VALUES ($1, $2, 'common', 'Tarife') RETURNING id`,
		f.org, p.id).Scan(&planID))
	f.must(f.pool.QueryRow(ctx, `INSERT INTO rate_plan_versions (organization_id, rate_plan_id, version) VALUES ($1, $2, 1) RETURNING id`,
		f.org, planID).Scan(&versionID))
	_, err = f.pool.Exec(ctx, `INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, 1, NULL, 1000)`,
		f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, planID)
	f.must(err)
	return p
}

func (f *fx) newCustomer(first, last string) (ref, ref) {
	ctx := context.Background()
	var c, d ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', $2, $3, '+905321112233', 'ayse@example.test') RETURNING id, uuid`, f.org, first, last).Scan(&c.id, &c.uuid))
	now := time.Now().UTC()
	f.must(f.pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, $3, $4, $5, 'v1:x', $6, '****456', $7) RETURNING id, uuid`,
		f.org, c.id, first, last, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return c, d
}

// reserve creates a confirmed reservation of the customer at branch b.
func (f *fx) reserve(b ref, from, to time.Time) resmodel.Reservation {
	f.t.Helper()
	w := resmodel.Where{PickupBranchUUID: &b.uuid, StartsAt: &from, EndsAt: &to}
	q, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{Where: w, Channel: "branch", ProductUUID: &f.product.uuid})
	f.must(err)
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &q.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	f.must(err)
	if r.Status != resmodel.StatusConfirmed {
		f.t.Fatalf("reservation status %s", r.Status)
	}
	return r
}

// rental creates a rental of a new confirmed reservation (not handed over).
func (f *fx) rental(b ref) (rentalmodel.Rental, resmodel.Reservation) {
	f.t.Helper()
	v := f.vehicle()
	start := time.Now().Add(-time.Hour).Truncate(time.Minute)
	if b.id != f.branch.id {
		start = time.Now().Add(2 * time.Hour).Truncate(time.Minute)
	}
	res := f.reserve(b, start.Add(3*time.Hour), start.Add(51*time.Hour))
	r, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &res.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	return r, res
}

func (f *fx) handover(r rentalmodel.Rental) rentalmodel.Rental {
	f.t.Helper()
	km, fuel := int32(1100), int16(80)
	out, err := f.rentals.Handover(f.ctx, f.org, r.UUID, rentalmodel.HandoverInput{Km: &km, Fuel: &fuel})
	f.must(err)
	return out
}

func (f *fx) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	f.must(f.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

func (f *fx) total(reservation uuid.UUID) money.Money {
	f.t.Helper()
	var s string
	f.must(f.pool.QueryRow(context.Background(), `SELECT total_amount::text FROM reservations WHERE organization_id = $1 AND uuid = $2`,
		f.org, reservation).Scan(&s))
	return money.MustParse(s)
}

func (f *fx) account() model.Account {
	f.t.Helper()
	c, err := f.fin.CustomerLedger(f.ctx, f.org, f.customer.uuid)
	f.must(err)
	if c.Account == nil {
		f.t.Fatal("the customer has no account")
	}
	return *c.Account
}

func mp(s string) *money.Money {
	v := money.MustParse(s)
	return &v
}

// liveRent is the sum of the live rent accruals of a rental and their count.
func (f *fx) liveRent(rental uuid.UUID) (money.Money, int) {
	f.t.Helper()
	var sum string
	var n int
	f.must(f.pool.QueryRow(context.Background(), `SELECT COALESCE(SUM(e.amount), 0)::text, COUNT(*) FROM ledger_entries e
		JOIN rentals r ON r.organization_id = e.organization_id AND r.id = e.rental_id
		WHERE e.organization_id = $1 AND r.uuid = $2 AND e.kind = 'accrual' AND e.purpose = 'rent'
		  AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = e.organization_id AND x.reverses_entry_id = e.id)`,
		f.org, rental).Scan(&sum, &n))
	return money.MustParse(sum), n
}

// TestAppendOnly: ledger entries and cash movements refuse UPDATE and
// DELETE; only an organization delete cascades through them.
func TestAppendOnly(t *testing.T) {
	f := newFx(t)
	acc, _, err := f.fin.CreateAccount(f.ctx, f.org, model.AccountCreateInput{CustomerUUID: &f.customer.uuid})
	f.must(err)
	e, err := f.fin.Collect(f.ctx, f.org, acc.UUID, model.CollectionInput{Amount: mp("250"), Method: "bank_transfer"})
	f.must(err)
	for _, stmt := range []string{
		"UPDATE ledger_entries SET amount = 1 WHERE uuid = $1",
		"UPDATE ledger_entries SET description = 'x' WHERE uuid = $1",
		"DELETE FROM ledger_entries WHERE uuid = $1",
	} {
		if _, err := f.pool.Exec(context.Background(), stmt, e.UUID); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: %v, want the append-only refusal", stmt, err)
		}
	}
	cash, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp("KASA"), Name: sp("Kasa"), Kind: sp("cash"),
		BranchUUID: &f.branch.uuid, OpeningBalance: mp("100")})
	f.must(err)
	mv, err := f.fin.RecordMovement(f.ctx, f.org, cash.UUID, model.MovementInput{Kind: model.MoveCollection, Amount: mp("50")})
	f.must(err)
	if _, err := f.pool.Exec(context.Background(), "UPDATE cash_movements SET amount = 1 WHERE uuid = $1", mv.UUID); err == nil {
		t.Error("a movement amount changed")
	}
	if _, err := f.pool.Exec(context.Background(), "DELETE FROM cash_movements WHERE uuid = $1", mv.UUID); err == nil {
		t.Error("a movement was deleted")
	}
	// The category is the one change a movement allows.
	cats, err := f.fin.ListCategories(f.ctx, f.org, "", true)
	f.must(err)
	if len(cats) != len(model.DefaultCategories) {
		t.Fatalf("%d default categories", len(cats))
	}
	if _, err := f.fin.SetMovementCategory(f.ctx, f.org, mv.UUID, &cats[0].UUID); err != nil {
		t.Fatalf("set category: %v", err)
	}
}

func sp(s string) *string { return &s }

// TestReversalFlow: a correction is a reversal that names its entry, needs
// a reason and the adjust permission, happens once, and is the exact
// opposite (the database refuses anything else); accruals follow their
// rental.
func TestReversalFlow(t *testing.T) {
	f := newFx(t)
	acc, _, err := f.fin.CreateAccount(f.ctx, f.org, model.AccountCreateInput{CustomerUUID: &f.customer.uuid})
	f.must(err)
	cash, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp("BANKA"), Name: sp("Banka"), Kind: sp("bank")})
	f.must(err)
	e, err := f.fin.Collect(f.ctx, f.org, acc.UUID, model.CollectionInput{Amount: mp("400"), Method: "bank_transfer", CashAccountUUID: &cash.UUID})
	f.must(err)
	if got, _ := f.fin.GetCashAccount(f.ctx, f.org, cash.UUID); !got.Balance.Equal(money.MustParse("400")) {
		t.Fatalf("bank balance after the deposit %s", got.Balance)
	}
	noAdjust := f.scoped(nil, rbac.PermTenantFinanceAccountsRead, rbac.PermTenantFinanceAccountsWrite)
	if _, err := f.fin.Reverse(noAdjust, f.org, e.UUID, model.ReverseInput{Reason: "yanlış kayıt"}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("reverse without permission: %v", err)
	}
	if _, err := f.fin.Reverse(f.ctx, f.org, e.UUID, model.ReverseInput{}); err == nil {
		t.Fatal("a reversal without a reason was accepted")
	}
	rev, err := f.fin.Reverse(f.ctx, f.org, e.UUID, model.ReverseInput{Reason: "yanlış kayıt"})
	f.must(err)
	if rev.Kind != model.KindReversal || rev.Direction != model.Debit || !rev.Amount.Equal(e.Amount) || rev.ReversesUUID == nil || *rev.ReversesUUID != e.UUID {
		t.Fatalf("reversal %+v", rev)
	}
	if got, _ := f.fin.GetEntry(f.ctx, f.org, e.UUID); got.ReversedByUUID == nil || *got.ReversedByUUID != rev.UUID {
		t.Fatalf("original does not name its reversal: %+v", got)
	}
	// The bank deposit is reversed with it.
	if got, _ := f.fin.GetCashAccount(f.ctx, f.org, cash.UUID); !got.Balance.IsZero() {
		t.Fatalf("bank balance after the reversal %s", got.Balance)
	}
	var se *usecase.StateError
	if _, err := f.fin.Reverse(f.ctx, f.org, e.UUID, model.ReverseInput{Reason: "tekrar"}); !errors.As(err, &se) || se.Code != "already_reversed" {
		t.Fatalf("second reversal: %v", err)
	}
	if _, err := f.fin.Reverse(f.ctx, f.org, rev.UUID, model.ReverseInput{Reason: "tekrar"}); !errors.As(err, &se) || se.Code != "reversal_of_reversal" {
		t.Fatalf("reversal of a reversal: %v", err)
	}
	if got, _ := f.fin.GetAccount(f.ctx, f.org, acc.UUID); !got.Balance.Balance.IsZero() {
		t.Fatalf("balance after reversal %s", got.Balance.Balance)
	}
	// The database refuses a reversal that is not the exact opposite.
	var accID, entryID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id, account_id FROM ledger_entries WHERE uuid = $1`, e.UUID).Scan(&entryID, &accID))
	e2, err := f.fin.Collect(f.ctx, f.org, acc.UUID, model.CollectionInput{Amount: mp("100")})
	f.must(err)
	var e2ID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM ledger_entries WHERE uuid = $1`, e2.UUID).Scan(&e2ID))
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO ledger_entries (organization_id, account_id, kind, direction, amount, entry_date,
		due_date, reason, source_type, reverses_entry_id) VALUES ($1, $2, 'reversal', 'debit', 99, CURRENT_DATE, CURRENT_DATE, 'deneme', 'manual', $3)`,
		f.org, accID, e2ID); err == nil {
		t.Error("a reversal of another amount was accepted")
	}
	// Adjustment: needs a reason, links its entry.
	adj, err := f.fin.Adjust(f.ctx, f.org, acc.UUID, model.AdjustmentInput{Direction: model.Debit, Amount: mp("30"), Reason: "kur farkı", AdjustsEntryUUID: &e2.UUID})
	f.must(err)
	if adj.Kind != model.KindAdjustment || adj.AdjustsUUID == nil || *adj.AdjustsUUID != e2.UUID {
		t.Fatalf("adjustment %+v", adj)
	}
	// An accrual follows its rental (manual reversal refused).
	r, _ := f.rental(f.branch)
	f.handover(r)
	items, _, err := f.fin.ListEntries(f.ctx, f.org, model.EntryFilter{RentalUUID: &r.UUID, Kinds: []string{model.KindAccrual}, Limit: 10})
	f.must(err)
	if len(items) != 1 {
		t.Fatalf("accruals %d", len(items))
	}
	if _, err := f.fin.Reverse(f.ctx, f.org, items[0].UUID, model.ReverseInput{Reason: "deneme"}); !errors.As(err, &se) || se.Code != "accrual_managed_by_rental" {
		t.Fatalf("manual accrual reversal: %v", err)
	}
}

// TestRT_A23_RentDebitedOnce: an invoice placeholder link, a prepayment
// (payments module), a deposit and its offset, a repeated sync and an
// extension on the same rental — the rent is a debit exactly once (one
// live accrual whose amount is the rental total).
func TestRT_A23_RentDebitedOnce(t *testing.T) {
	f := newFx(t)
	r, res := f.rental(f.branch)
	// A prepayment before the handover (branch collection): a credit on
	// the customer account, linked to the reservation.
	pmt, err := f.pay.Collect(f.ctx, f.org, res.UUID, paymodel.CollectInput{Amount: mp("500"), Method: "cash"})
	f.must(err)
	acc := f.account()
	if !acc.Balance.Balance.Equal(money.MustParse("-500")) {
		t.Fatalf("balance after the prepayment %s", acc.Balance.Balance)
	}
	// Nothing accrues before the handover.
	if _, n := f.liveRent(r.UUID); n != 0 {
		t.Fatalf("%d rent accruals before the handover", n)
	}
	// A deposit taken at the counter.
	dep, err := f.fin.Collect(f.ctx, f.org, acc.UUID, model.CollectionInput{Amount: mp("2000"), Purpose: model.PurposeDeposit, Method: "card_pos", RentalUUID: &r.UUID})
	f.must(err)
	if dep.Rental == nil || dep.Rental.UUID != r.UUID {
		t.Fatalf("deposit not linked to the rental: %+v", dep)
	}
	f.handover(r)
	total := f.total(res.UUID)
	sum, n := f.liveRent(r.UUID)
	if n != 1 || !sum.Equal(total) {
		t.Fatalf("after the handover: %d live rent accruals summing %s, want 1 of %s", n, sum, total)
	}
	// Syncing again (any later rental action) posts nothing.
	f.must(f.fin.SyncRental(f.ctx, nil, f.org, rentalID(f, r.UUID)))
	if _, n := f.liveRent(r.UUID); n != 1 {
		t.Fatalf("a repeated sync accrued again (%d)", n)
	}
	// The database refuses a second accrual of the same key.
	var accID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM accounts WHERE uuid = $1`, acc.UUID).Scan(&accID))
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO ledger_entries (organization_id, account_id, kind, direction, purpose, amount,
		entry_date, due_date, source_type, accrual_key) VALUES ($1, $2, 'accrual', 'debit', 'rent', 10, CURRENT_DATE, CURRENT_DATE, 'rental', $3)`,
		f.org, accID, usecase.RentKey(rentalID(f, r.UUID))); err == nil {
		t.Fatal("a second rent accrual of the same rental was accepted")
	}
	// Invoicing (TEC-722 placeholder): the invoice links the accrual and
	// posts nothing.
	items, _, err := f.fin.ListEntries(f.ctx, f.org, model.EntryFilter{RentalUUID: &r.UUID, Kinds: []string{model.KindAccrual}, Limit: 10})
	f.must(err)
	before := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1`, f.org)
	f.must(f.fin.LinkDocument(f.ctx, f.org, items[0].UUID, model.DocumentInvoice, "GIB2026000000001"))
	f.must(f.fin.LinkDocument(f.ctx, f.org, items[0].UUID, model.DocumentInvoice, "GIB2026000000001"))
	if after := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1`, f.org); after != before {
		t.Fatalf("linking an invoice posted %d entries", after-before)
	}
	linked, err := f.fin.GetEntry(f.ctx, f.org, items[0].UUID)
	f.must(err)
	if len(linked.Links) != 1 || linked.Links[0].DocumentRef != "GIB2026000000001" {
		t.Fatalf("links %+v", linked.Links)
	}
	// The deposit is a liability kept apart (purpose deposit): it never
	// reduces the receivable. Applying it to the rent (offset pair) moves
	// the amount out of the deposit and settles the rent.
	acc = f.account()
	if !acc.Balance.DepositHeld.Equal(money.MustParse("2000")) {
		t.Fatalf("deposit held %s", acc.Balance.DepositHeld)
	}
	if want := total.Sub(money.MustParse("500")); !acc.Balance.Balance.Equal(want) {
		t.Fatalf("balance with the deposit held %s, want %s (rent - prepayment)", acc.Balance.Balance, want)
	}
	balanceBefore := acc.Balance.Balance
	if _, err := f.fin.Offset(f.ctx, f.org, acc.UUID, model.OffsetInput{Amount: mp("2500"), RentalUUID: &r.UUID, Reason: "depozito mahsubu"}); err == nil {
		t.Fatal("an offset above the deposit was accepted")
	}
	off, err := f.fin.Offset(f.ctx, f.org, acc.UUID, model.OffsetInput{Amount: mp("1500"), RentalUUID: &r.UUID, Reason: "depozito mahsubu"})
	f.must(err)
	if len(off) != 2 || off[0].OffsetGroup == nil || *off[0].OffsetGroup != *off[1].OffsetGroup {
		t.Fatalf("offset %+v", off)
	}
	acc = f.account()
	if !acc.Balance.Balance.Equal(balanceBefore.Sub(money.MustParse("1500"))) || !acc.Balance.DepositHeld.Equal(money.MustParse("500")) {
		t.Fatalf("after the offset: balance %s (was %s), deposit %s", acc.Balance.Balance, balanceBefore, acc.Balance.DepositHeld)
	}
	// Rent once: debits of purpose rent net of reversals = the total; the
	// rental balance = rent - prepayment - the part of the deposit applied.
	led, err := f.fin.RentalLedger(f.ctx, f.org, r.UUID)
	f.must(err)
	want := total.Sub(money.MustParse("500")).Sub(money.MustParse("1500"))
	if !led.Balance.Equal(want) {
		t.Fatalf("rental balance %s, want %s", led.Balance, want)
	}
	// An extension re-prices the rent: reversal + one new accrual.
	ends := res.EndsAt.Add(24 * time.Hour)
	if _, err := f.rentals.Extend(f.ctx, f.org, r.UUID, rentalmodel.ExtendInput{EndsAt: &ends, Reason: "uzatma"}); err != nil {
		t.Fatalf("extend: %v", err)
	}
	newTotal := f.total(res.UUID)
	if newTotal.Cmp(total) <= 0 {
		t.Fatalf("total after the extension %s (was %s)", newTotal, total)
	}
	sum, n = f.liveRent(r.UUID)
	if n != 1 || !sum.Equal(newTotal) {
		t.Fatalf("after the extension: %d live rent accruals summing %s, want 1 of %s", n, sum, newTotal)
	}
	if got := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND kind = 'reversal' AND reason = $2`,
		f.org, model.ReasonAmountChanged); got != 1 {
		t.Fatalf("%d amount-changed reversals", got)
	}
	// The prepayment is the payments module's collection, posted once.
	if got := f.count(`SELECT count(*) FROM ledger_entries e JOIN payments p ON p.organization_id = e.organization_id AND p.id = e.payment_id
		WHERE e.organization_id = $1 AND p.uuid = $2`, f.org, pmt.UUID); got != 1 {
		t.Fatalf("%d entries of the prepayment", got)
	}
}

func rentalID(f *fx, id uuid.UUID) int64 {
	var out int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM rentals WHERE organization_id = $1 AND uuid = $2`, f.org, id).Scan(&out))
	return out
}

// TestWebhookPostingIsIdempotent: an iyzico payment notified three times
// (two concurrently), a staff verify and the catch-up sweep post one
// collection; a refund posts one refund entry however often it is posted.
func TestWebhookPostingIsIdempotent(t *testing.T) {
	f := newFx(t)
	_, res := f.rental(f.branch)
	link, err := f.pay.CreateLink(f.ctx, f.org, res.UUID, paymodel.CreateLinkInput{Amount: mp("750")})
	f.must(err)
	token := link.URL[strings.LastIndex(link.URL, "/")+1:]
	before := map[string]bool{}
	for _, tk := range f.fake.Tokens() {
		before[tk] = true
	}
	if _, err := f.pay.StartCheckout(context.Background(), token, "10.0.0.7", "tr"); err != nil {
		t.Fatal(err)
	}
	fakeToken := ""
	for _, tk := range f.fake.Tokens() {
		if !before[tk] {
			fakeToken = tk
		}
	}
	body, sig, err := f.fake.Complete(fakeToken, iyzicofake.StatePaid)
	f.must(err)
	deliver := func() error {
		h := http.Header{}
		h.Set(iyzico.SignatureHeader, sig)
		_, err := f.pay.Webhook(context.Background(), iyzico.ProviderCode, h, body)
		return err
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- deliver() }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		f.must(err)
	}
	f.must(deliver())
	var paymentID int64
	var paymentUUID uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `SELECT id, uuid FROM payments WHERE organization_id = $1 AND status = 'succeeded' AND provider = 'iyzico'`,
		f.org).Scan(&paymentID, &paymentUUID))
	// Posting again (verify path, sweep) changes nothing.
	f.must(f.fin.PostPayment(f.ctx, f.org, paymentID))
	if _, err := f.pay.Verify(f.ctx, f.org, paymentUUID); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := f.fin.PostPending(context.Background()); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND payment_id = $2 AND kind = 'collection'`, f.org, paymentID); got != 1 {
		t.Fatalf("%d collection entries for one payment", got)
	}
	acc := f.account()
	if !acc.Balance.Credit.Equal(money.MustParse("750")) {
		t.Fatalf("credit %s", acc.Balance.Credit)
	}
	// A partial refund: one refund entry, posted again → still one.
	if _, err := f.pay.Refund(f.ctx, f.org, paymentUUID, paymodel.RefundInput{Amount: mp("200"), Reason: "kısmi iade"}, "10.0.0.7"); err != nil {
		t.Fatalf("refund: %v", err)
	}
	var refundID int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT id FROM payment_refunds WHERE organization_id = $1`, f.org).Scan(&refundID))
	f.must(f.fin.PostRefund(f.ctx, f.org, refundID))
	if _, err := f.fin.PostPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.count(`SELECT count(*) FROM ledger_entries WHERE organization_id = $1 AND payment_refund_id = $2`, f.org, refundID); got != 1 {
		t.Fatalf("%d refund entries", got)
	}
	acc = f.account()
	if !acc.Balance.Balance.Equal(money.MustParse("-550")) {
		t.Fatalf("balance after the refund %s", acc.Balance.Balance)
	}
}

// TestDisputedChargeNotAccrued: a disputed return charge is no debt until
// the damage approver approves it with a note; a waived charge never is.
func TestDisputedChargeNotAccrued(t *testing.T) {
	f := newFx(t)
	r, _ := f.rental(f.branch)
	r = f.handover(r)
	rid := rentalID(f, r.UUID)
	var disputed, plain uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason, disputed)
		VALUES ($1, $2, 'damage', 'manual', 2000, 'çizik', TRUE) RETURNING uuid`, f.org, rid).Scan(&disputed))
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO rental_charges (organization_id, rental_id, kind, source, amount, reason)
		VALUES ($1, $2, 'fuel', 'manual', 300, 'yakıt eksik') RETURNING uuid`, f.org, rid).Scan(&plain))
	f.must(f.fin.SyncRental(f.ctx, nil, f.org, rid))
	charges := func() int {
		return f.count(`SELECT count(*) FROM ledger_entries e WHERE e.organization_id = $1 AND e.rental_id = $2 AND e.kind = 'accrual'
			AND e.purpose = 'charge' AND NOT EXISTS (SELECT 1 FROM ledger_entries x WHERE x.organization_id = e.organization_id AND x.reverses_entry_id = e.id)`,
			f.org, rid)
	}
	if n := charges(); n != 0 {
		t.Fatalf("%d proposed / disputed charges accrued", n)
	}
	noDamage := f.scoped(nil, rbac.PermTenantRentalsRead, rbac.PermTenantRentalsClose, rbac.PermTenantFinanceAccountsRead)
	if _, err := f.rentals.DecideCharge(noDamage, f.org, r.UUID, disputed, true, "onaylandı"); !errors.Is(err, rentalusecase.ErrForbidden) {
		t.Fatalf("approving a disputed charge without damage.approve: %v", err)
	}
	if n := charges(); n != 0 {
		t.Fatalf("refused approval accrued %d", n)
	}
	if _, err := f.rentals.DecideCharge(f.ctx, f.org, r.UUID, plain, false, "müşteri ile anlaşıldı"); err != nil {
		t.Fatal(err)
	}
	if n := charges(); n != 0 {
		t.Fatalf("a waived charge accrued (%d)", n)
	}
	if _, err := f.rentals.DecideCharge(f.ctx, f.org, r.UUID, disputed, true, "fotoğraflarla doğrulandı"); err != nil {
		t.Fatal(err)
	}
	if n := charges(); n != 1 {
		t.Fatalf("approved disputed charge: %d accruals, want 1", n)
	}
	acc := f.account()
	want := f.total(reservationOf(f, r.UUID)).Add(money.MustParse("2000"))
	if !acc.Balance.Debit.Equal(want) {
		t.Fatalf("debit %s, want rent + approved damage %s", acc.Balance.Debit, want)
	}
}

func reservationOf(f *fx, rental uuid.UUID) uuid.UUID {
	var out uuid.UUID
	f.must(f.pool.QueryRow(context.Background(), `SELECT res.uuid FROM rentals r JOIN reservations res ON res.organization_id = r.organization_id
		AND res.id = r.reservation_id WHERE r.organization_id = $1 AND r.uuid = $2`, f.org, rental).Scan(&out))
	return out
}

// TestBalancesAndStatuses: aging applies payments to the oldest debts;
// statuses receivable / payable / due soon / overdue / closed; the
// organization summary with its branch and due breakdowns.
func TestBalancesAndStatuses(t *testing.T) {
	f := newFx(t)
	today := time.Now().In(time.FixedZone("TR", 3*3600))
	d := func(days int) *string { s := today.AddDate(0, 0, days).Format("2006-01-02"); return &s }
	open := func(first, last string) model.Account {
		c, _ := f.newCustomer(first, last)
		a, _, err := f.fin.CreateAccount(f.ctx, f.org, model.AccountCreateInput{CustomerUUID: &c.uuid})
		f.must(err)
		return a
	}
	debit := func(a model.Account, amount string, due *string, branch ref) {
		_, err := f.fin.Adjust(f.ctx, f.org, a.UUID, model.AdjustmentInput{Direction: model.Debit, Amount: mp(amount), DueDate: due,
			EntryDate: d(-60), Reason: "açılış bakiyesi", BranchUUID: &branch.uuid})
		f.must(err)
	}
	overdue, soon, later, closed, payable := open("Vadesi", "Geçen"), open("Vadesi", "Yakın"), open("İleri", "Vade"), open("Kapalı", "Cari"), open("Fazla", "Ödeme")
	debit(overdue, "1000", d(-40), f.branch)
	debit(overdue, "500", d(3), f.branch)
	_, err := f.fin.Collect(f.ctx, f.org, overdue.UUID, model.CollectionInput{Amount: mp("400"), BranchUUID: &f.branch.uuid})
	f.must(err)
	debit(soon, "300", d(2), f.other)
	debit(later, "800", d(30), f.other)
	debit(closed, "200", d(-5), f.branch)
	_, err = f.fin.Collect(f.ctx, f.org, closed.UUID, model.CollectionInput{Amount: mp("200"), BranchUUID: &f.branch.uuid})
	f.must(err)
	_, err = f.fin.Collect(f.ctx, f.org, payable.UUID, model.CollectionInput{Amount: mp("150"), BranchUUID: &f.other.uuid})
	f.must(err)
	check := func(a model.Account, status, balance, od, ds string) {
		t.Helper()
		got, err := f.fin.GetAccount(f.ctx, f.org, a.UUID)
		f.must(err)
		b := got.Balance
		if b.Status != status || !b.Balance.Equal(money.MustParse(balance)) || !b.Overdue.Equal(money.MustParse(od)) || !b.DueSoon.Equal(money.MustParse(ds)) {
			t.Errorf("%s: status %s balance %s overdue %s due soon %s; want %s %s %s %s", a.Name, b.Status, b.Balance, b.Overdue, b.DueSoon,
				status, balance, od, ds)
		}
	}
	// 1000 (overdue) + 500 (due in 3 days) - 400 paid → 600 overdue, 500 due soon.
	check(overdue, model.BalanceOverdue, "1100", "600", "500")
	check(soon, model.BalanceDueSoon, "300", "0", "300")
	check(later, model.BalanceReceivable, "800", "0", "0")
	check(closed, model.BalanceClosed, "0", "0", "0")
	check(payable, model.BalancePayable, "-150", "0", "0")
	// Filters: status and "open charges due before".
	items, _, err := f.fin.ListAccounts(f.ctx, f.org, model.AccountFilter{BalanceStatuses: []string{model.BalanceOverdue, model.BalanceDueSoon}, Limit: 50})
	f.must(err)
	if len(items) != 2 {
		t.Errorf("overdue + due soon filter: %d accounts", len(items))
	}
	cut := today.AddDate(0, 0, 10)
	items, _, err = f.fin.ListAccounts(f.ctx, f.org, model.AccountFilter{DueBefore: &cut, Limit: 50})
	f.must(err)
	if len(items) != 2 {
		t.Errorf("due before +10 days: %d accounts", len(items))
	}
	sum, err := f.fin.Summary(f.ctx, f.org)
	f.must(err)
	if !sum.Receivable.Equal(money.MustParse("2200")) || !sum.Payable.Equal(money.MustParse("150")) || !sum.Net.Equal(money.MustParse("2050")) ||
		!sum.Overdue.Equal(money.MustParse("600")) || !sum.DueSoon.Equal(money.MustParse("800")) {
		t.Errorf("summary %+v", sum)
	}
	if sum.AccountsByStat[model.BalanceClosed] != 1 || sum.AccountsByStat[model.BalanceOverdue] != 1 || sum.AccountsByStat[model.BalancePayable] != 1 {
		t.Errorf("accounts by status %v", sum.AccountsByStat)
	}
	buckets := map[string]string{}
	for _, b := range sum.ByDue {
		buckets[b.Key] = b.Amount.String()
	}
	if buckets["overdue_30_plus"] != "600.00" || buckets["due_7_days"] != "800.00" || buckets["later"] != "800.00" {
		t.Errorf("due buckets %v", buckets)
	}
	byBranch := map[string]string{}
	for _, b := range sum.ByBranch {
		if b.Branch != nil {
			byBranch[b.Branch.Code] = b.Balance.String()
		}
	}
	if byBranch["MRK"] != "1100.00" || byBranch["HVL"] != "950.00" {
		t.Errorf("by branch %v", byBranch)
	}
}

// TestAtomicTransfer: a transfer writes both movements or none; a till
// cannot go negative; opposite concurrent transfers keep the total.
func TestAtomicTransfer(t *testing.T) {
	f := newFx(t)
	till, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp("KASA"), Name: sp("Kasa"), Kind: sp("cash"),
		BranchUUID: &f.branch.uuid, OpeningBalance: mp("1000")})
	f.must(err)
	bank, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp("BANKA"), Name: sp("Banka"), Kind: sp("bank"),
		OpeningBalance: mp("5000"), IBAN: sp("TR33 0006 1005 1978 6457 8413 26")})
	f.must(err)
	if _, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp("KASA"), Name: sp("Kasa 2"), Kind: sp("cash"),
		BranchUUID: &f.branch.uuid}); !errors.Is(err, usecase.ErrCodeTaken) {
		t.Fatalf("duplicate code: %v", err)
	}
	tr, err := f.fin.Transfer(f.ctx, f.org, model.TransferInput{FromAccountUUID: &till.UUID, ToAccountUUID: &bank.UUID, Amount: mp("600"), Description: "bankaya yatırıldı"})
	f.must(err)
	if tr.Out.TransferGroup == nil || tr.In.TransferGroup == nil || *tr.Out.TransferGroup != *tr.In.TransferGroup {
		t.Fatalf("transfer legs %+v", tr)
	}
	movements := func() int { return f.count(`SELECT count(*) FROM cash_movements WHERE organization_id = $1`, f.org) }
	n := movements()
	if _, err := f.fin.Transfer(f.ctx, f.org, model.TransferInput{FromAccountUUID: &till.UUID, ToAccountUUID: &bank.UUID, Amount: mp("401")}); err == nil {
		t.Fatal("a till went negative")
	}
	if movements() != n {
		t.Fatal("a refused transfer wrote movements")
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from, to := &till.UUID, &bank.UUID
			if i%2 == 0 {
				from, to = to, from
			}
			_, _ = f.fin.Transfer(f.ctx, f.org, model.TransferInput{FromAccountUUID: from, ToAccountUUID: to, Amount: mp("100")})
		}(i)
	}
	wg.Wait()
	a, _ := f.fin.GetCashAccount(f.ctx, f.org, till.UUID)
	b, _ := f.fin.GetCashAccount(f.ctx, f.org, bank.UUID)
	if !a.Balance.Add(b.Balance).Equal(money.MustParse("6000")) || a.Balance.IsNegative() {
		t.Fatalf("balances after concurrent transfers: till %s bank %s", a.Balance, b.Balance)
	}
	if (movements()-n)%2 != 0 {
		t.Fatal("a transfer wrote one leg")
	}
	// Reversing one leg reverses the transfer.
	rev, err := f.fin.ReverseMovement(f.ctx, f.org, tr.In.UUID, model.ReverseInput{Reason: "yanlış hesap"})
	f.must(err)
	if len(rev) != 2 {
		t.Fatalf("transfer reversal legs %d", len(rev))
	}
	// Expense paid from the till, with an attachment; voiding reverses
	// the payment.
	cats, err := f.fin.ListCategories(f.ctx, f.org, "", true)
	f.must(err)
	exp, err := f.fin.CreateExpense(f.ctx, f.org, model.ExpenseInput{CategoryUUID: &cats[3].UUID, TotalAmount: mp("120"), VATRate: ratep(20),
		CashAccountUUID: &till.UUID, Description: sp("Yakıt")})
	f.must(err)
	if !exp.NetAmount.Equal(money.MustParse("100")) || !exp.VATAmount.Equal(money.MustParse("20")) || exp.MovementUUID == nil {
		t.Fatalf("expense %+v", exp)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	exp, err = f.fin.UploadAttachment(f.ctx, f.org, exp.UUID, usecase.FileUpload{Body: strings.NewReader(string(png)), Size: int64(len(png)), Filename: "fis.png"})
	f.must(err)
	if exp.Attachment == nil || exp.Attachment.ContentType != "image/png" || !strings.HasPrefix(exp.Attachment.URL, "/v1/tenant/finance/expenses/") {
		t.Fatalf("attachment %+v", exp.Attachment)
	}
	var key string
	f.must(f.pool.QueryRow(context.Background(), `SELECT attachment_object_key FROM expenses WHERE uuid = $1`, exp.UUID).Scan(&key))
	if !strings.HasPrefix(key, storage.OrganizationPrefix(f.orgUUID.String())+"expenses/") {
		t.Fatalf("attachment key %s", key)
	}
	before, _ := f.fin.GetCashAccount(f.ctx, f.org, till.UUID)
	if _, err := f.fin.VoidExpense(f.ctx, f.org, exp.UUID, "mükerrer"); err != nil {
		t.Fatal(err)
	}
	after, _ := f.fin.GetCashAccount(f.ctx, f.org, till.UUID)
	if !after.Balance.Sub(before.Balance).Equal(money.MustParse("120")) {
		t.Fatalf("void did not return the payment: %s → %s", before.Balance, after.Balance)
	}
}

// memDriver adapts the in-memory store to storage.Driver.
type memDriver struct {
	storage.Driver
	m *storage.Memory
}

func (d memDriver) Upload(ctx context.Context, f storage.File, path string) error {
	return d.m.Upload(ctx, f, path)
}

func (d memDriver) Download(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, path)
}

func (d memDriver) Delete(ctx context.Context, path string) error { return d.m.Delete(ctx, path) }

func ratep(p int64) *money.Rate {
	r := money.Percent(p)
	return &r
}

// TestBranchScope: a member of one branch sees that branch's entries,
// accounts, cash accounts and expenses only (others answer not found).
func TestBranchScope(t *testing.T) {
	f := newFx(t)
	mrkCustomer := f.customer
	c2, _ := f.newCustomer("Hava", "Limanı")
	accA, _, err := f.fin.CreateAccount(f.ctx, f.org, model.AccountCreateInput{CustomerUUID: &mrkCustomer.uuid})
	f.must(err)
	_, err = f.fin.Collect(f.ctx, f.org, accA.UUID, model.CollectionInput{Amount: mp("100"), BranchUUID: &f.branch.uuid})
	f.must(err)
	accB, _, err := f.fin.CreateAccount(f.scoped([]int64{f.other.id}), f.org, model.AccountCreateInput{CustomerUUID: &c2.uuid})
	f.must(err)
	eB, err := f.fin.Collect(f.ctx, f.org, accB.UUID, model.CollectionInput{Amount: mp("900"), BranchUUID: &f.other.uuid})
	f.must(err)
	cashB, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp("KASA-HVL"), Name: sp("HVL kasa"), Kind: sp("cash"), BranchUUID: &f.other.uuid})
	f.must(err)
	head, err := f.fin.CreateCashAccount(f.ctx, f.org, model.CashAccountInput{Code: sp("BANKA"), Name: sp("Merkez banka"), Kind: sp("bank")})
	f.must(err)
	cats, _ := f.fin.ListCategories(f.ctx, f.org, "", true)
	expB, err := f.fin.CreateExpense(f.ctx, f.org, model.ExpenseInput{CategoryUUID: &cats[0].UUID, TotalAmount: mp("50"), BranchUUID: &f.other.uuid})
	f.must(err)

	mrk := f.scoped([]int64{f.branch.id})
	accounts, _, err := f.fin.ListAccounts(mrk, f.org, model.AccountFilter{Limit: 50})
	f.must(err)
	if len(accounts) != 1 || accounts[0].UUID != accA.UUID {
		t.Fatalf("MRK member sees %d accounts", len(accounts))
	}
	if _, err := f.fin.GetAccount(mrk, f.org, accB.UUID); !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Fatalf("HVL account for MRK: %v", err)
	}
	if _, err := f.fin.GetEntry(mrk, f.org, eB.UUID); !errors.Is(err, usecase.ErrEntryNotFound) {
		t.Fatalf("HVL entry for MRK: %v", err)
	}
	entries, _, err := f.fin.ListEntries(mrk, f.org, model.EntryFilter{Limit: 50})
	f.must(err)
	for _, e := range entries {
		if e.Branch == nil || e.Branch.UUID != f.branch.uuid {
			t.Fatalf("MRK member sees entry of %+v", e.Branch)
		}
	}
	for _, id := range []uuid.UUID{cashB.UUID, head.UUID} {
		if _, err := f.fin.GetCashAccount(mrk, f.org, id); !errors.Is(err, usecase.ErrCashAccountNotFound) {
			t.Fatalf("cash account %s for MRK: %v", id, err)
		}
	}
	if _, err := f.fin.GetExpense(mrk, f.org, expB.UUID); !errors.Is(err, usecase.ErrExpenseNotFound) {
		t.Fatalf("HVL expense for MRK: %v", err)
	}
	if _, err := f.fin.Transfer(mrk, f.org, model.TransferInput{FromAccountUUID: &cashB.UUID, ToAccountUUID: &head.UUID, Amount: mp("1")}); err == nil {
		t.Fatal("MRK member moved HVL money")
	}
	// A branch-scoped member must book in their branch.
	if _, err := f.fin.CreateCashAccount(mrk, f.org, model.CashAccountInput{Code: sp("X"), Name: sp("x"), Kind: sp("cash")}); err == nil {
		t.Fatal("a head office account created by a branch member")
	}
	sum, err := f.fin.Summary(mrk, f.org)
	f.must(err)
	if !sum.Payable.Equal(money.MustParse("100")) {
		t.Fatalf("MRK summary %+v", sum)
	}
}

// TestCrossOrganization: another organization's records are unknown.
func TestCrossOrganization(t *testing.T) {
	a := newFx(t)
	b := newFx(t)
	accB, _, err := b.fin.CreateAccount(b.ctx, b.org, model.AccountCreateInput{CustomerUUID: &b.customer.uuid})
	b.must(err)
	eB, err := b.fin.Collect(b.ctx, b.org, accB.UUID, model.CollectionInput{Amount: mp("10")})
	b.must(err)
	cashB, err := b.fin.CreateCashAccount(b.ctx, b.org, model.CashAccountInput{Code: sp("K"), Name: sp("K"), Kind: sp("bank")})
	b.must(err)
	cashA, err := a.fin.CreateCashAccount(a.ctx, a.org, model.CashAccountInput{Code: sp("K"), Name: sp("K"), Kind: sp("bank")})
	a.must(err)
	if _, err := a.fin.GetAccount(a.ctx, a.org, accB.UUID); !errors.Is(err, usecase.ErrAccountNotFound) {
		t.Errorf("account: %v", err)
	}
	if _, err := a.fin.GetEntry(a.ctx, a.org, eB.UUID); !errors.Is(err, usecase.ErrEntryNotFound) {
		t.Errorf("entry: %v", err)
	}
	if _, err := a.fin.Reverse(a.ctx, a.org, eB.UUID, model.ReverseInput{Reason: "deneme"}); !errors.Is(err, usecase.ErrEntryNotFound) {
		t.Errorf("reverse: %v", err)
	}
	if _, _, err := a.fin.CreateAccount(a.ctx, a.org, model.AccountCreateInput{CustomerUUID: &b.customer.uuid}); err == nil {
		t.Error("an account for another organization's customer")
	}
	if _, err := a.fin.Transfer(a.ctx, a.org, model.TransferInput{FromAccountUUID: &cashA.UUID, ToAccountUUID: &cashB.UUID, Amount: mp("1")}); err == nil {
		t.Error("a transfer into another organization's account")
	}
	if _, err := a.fin.CustomerLedger(a.ctx, a.org, b.customer.uuid); !errors.Is(err, usecase.ErrCustomerNotFound) {
		t.Errorf("customer ledger: %v", err)
	}
	items, _, err := a.fin.ListEntries(a.ctx, a.org, model.EntryFilter{Limit: 100})
	a.must(err)
	for _, e := range items {
		if e.UUID == eB.UUID {
			t.Error("another organization's entry listed")
		}
	}
}
