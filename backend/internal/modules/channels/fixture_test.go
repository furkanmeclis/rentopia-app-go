package channels_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/usecase"
	customerrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	pricingrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	pricingusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels/channelfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with a branch (location IST), products on the
// availability engine and pricing (1000 TRY / day), the reservations and
// customers modules behind the Booker adapter, and the channel manager
// with yolcu360_fake connected and mapped.
type fx struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	res      *resusecase.Service
	svc      *usecase.Service
	fake     *channelfake.Provider
	obilet   *channelfake.Provider
	accounts *accountsusecase.Service
	org      int64
	suffix   string
	segment  int64
	branch   ref
	product  ref
	account  uuid.UUID
	plate    int
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var perms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantChannelsRead, rbac.PermTenantChannelsWrite, rbac.PermTenantChannelsSync,
	rbac.PermTenantIntegrationsManage,
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
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "chn-" + f.suffix, Name: "Chn " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org = org.ID
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM integration_jobs WHERE organization_id = $1", f.org)
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = orgctx.WithScope(ctx, orgctx.Scope{
		ID: f.org, UUID: org.Uuid, Slug: "chn-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms,
	})
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment))

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	f.res = resusecase.New(pool, q, avail, pricing, settings)

	cipher, err := crypto.NewFieldCipher(strings.Repeat("c", 32))
	f.must(err)
	customers := customerusecase.New(customerrepository.NewPostgres(pool, q), cipher, nil, nil, nil)
	vault, err := integrations.NewVault(cipher)
	f.must(err)
	f.fake = channelfake.New()
	f.obilet = channelfake.NewObilet()
	f.accounts = accountsusecase.New(q, vault, integrations.NewRegistry(f.fake, f.obilet))
	f.svc, err = usecase.New(pool, q, cipher, f.accounts, adapters.NewBooker(f.res, customers), f.fake, f.obilet)
	f.must(err)
	f.res.SetLifecycleListener(f.svc)

	f.branch = f.newBranch("MRK")
	f.product = f.newProduct("EKO", 1000)
	f.configure()
	acc, err := f.accounts.Connect(f.ctx, f.org, accountsusecase.ConnectInput{
		Kind: channels.Kind, Provider: channelfake.ProviderCode, DisplayName: "Yolcu360 test",
		Credentials: map[string]string{"supplier_code": "SUP-" + f.suffix, "api_key": "k-" + f.suffix},
	})
	f.must(err)
	f.account = acc.UUID
	f.mapping(model.MappingInput{EntityType: model.EntityBranch, BranchUUID: model.OptionalUUID{Set: true, Value: &f.branch.uuid}, ExternalCode: sp("IST")})
	f.mapping(model.MappingInput{EntityType: model.EntityProduct, ProductUUID: &f.product.uuid, ExternalCode: sp("Y-ECO")})
	return f
}

func sp(v string) *string { return &v }

func (f *fx) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *fx) mapping(in model.MappingInput) model.Mapping {
	f.t.Helper()
	in.AccountUUID = &f.account
	m, err := f.svc.CreateMapping(f.ctx, f.org, in)
	f.must(err)
	return m
}

func (f *fx) newBranch(code string) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", City: "İstanbul", Address: code + " Cad. No:1",
		WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"), PrepMinutes: 60, IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

// vehicle adds a vehicle at the branch, member of the product's group.
func (f *fx) vehicle(group string) ref {
	f.plate++
	plate := fmt.Sprintf("34 CH %d%s", f.plate, strings.ToUpper(f.suffix[:3]))
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Fiat', 'Egea', 2025, $3, $3, 1000) RETURNING id, uuid`,
		f.org, plate, f.branch.id).Scan(&r.id, &r.uuid))
	_, err := f.pool.Exec(context.Background(), `INSERT INTO vehicle_rental_groups (organization_id, vehicle_id, rental_group_id)
		SELECT $1, $2, g.id FROM rental_groups g WHERE g.organization_id = $1 AND g.code = $3`, f.org, r.id, group)
	f.must(err)
	return r
}

// newProduct adds a published product with its own group (code G<code>)
// and a common rate plan of daily price per day.
func (f *fx) newProduct(code string, daily int) ref {
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
	_, err = f.pool.Exec(ctx, `INSERT INTO rate_bands (organization_id, rate_plan_version_id, min_days, max_days, daily_price) VALUES ($1, $2, 1, NULL, $3)`,
		f.org, versionID, fmt.Sprint(daily))
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plan_versions SET status = 'active', activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, versionID)
	f.must(err)
	_, err = f.pool.Exec(ctx, `UPDATE rate_plans SET status = 'active', active_version = 1, activated_at = NOW() WHERE organization_id = $1 AND id = $2`, f.org, planID)
	f.must(err)
	return p
}

// configure stores settings with the yolcu360 and obilet channels active.
func (f *fx) configure() {
	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	for i := range s.SalesChannels {
		if s.SalesChannels[i].Code == "yolcu360" || s.SalesChannels[i].Code == "obilet" {
			s.SalesChannels[i].IsActive = true
		}
	}
	data, _ := json.Marshal(s)
	_, err := f.pool.Exec(context.Background(), `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	f.must(err)
}

// fakeAccount is the adapter view of the connected account.
func (f *fx) fakeAccount() integrations.Account {
	f.t.Helper()
	a, err := f.accounts.AccountByUUID(f.ctx, f.org, f.account)
	f.must(err)
	return a
}

func (f *fx) pull() model.SyncRun {
	f.t.Helper()
	run, err := f.svc.PullNow(f.ctx, f.org, f.account)
	f.must(err)
	return run
}

func (f *fx) push() model.SyncRun {
	f.t.Helper()
	run, err := f.svc.PushNow(f.ctx, f.org, f.account)
	f.must(err)
	return run
}

// window is a booking range starting in days at 10:00 local.
func window(days, length int) (time.Time, time.Time) {
	loc, _ := time.LoadLocation("Europe/Istanbul")
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, loc).AddDate(0, 0, days)
	return start.UTC(), start.AddDate(0, 0, length).UTC()
}

func booking(id, ref string, version int64, status string, start, end time.Time, total, commission string) channels.Message {
	typ := channels.MessageBooking
	if version > 1 {
		typ = channels.MessageModification
	}
	if status == channels.StatusCancelled {
		typ = channels.MessageCancellation
	}
	return channels.Message{
		ID: id, Type: typ, ExternalRef: ref, Version: version, Status: status, ProductCode: "Y-ECO",
		PickupLocationCode: "IST", ReturnLocationCode: "IST", StartsAt: start, EndsAt: end,
		Customer: channels.Customer{FirstName: "Ayşe", LastName: "Demir", Email: "ayse@example.test", Phone: "+905321234567", Nationality: "TR"},
		Currency: "TRY", Total: money.MustParse(total), Commission: money.MustParse(commission), Prepaid: money.MustParse(total),
		CollectedBy: channels.CollectedByChannel,
	}
}

// reservationsOf lists the reservations of an external reference.
func (f *fx) reservationsOf(ref string) []resmodel.Reservation {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT uuid FROM reservations WHERE organization_id = $1 AND channel = 'yolcu360' AND external_reference = $2`, f.org, ref)
	f.must(err)
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		f.must(rows.Scan(&id))
		ids = append(ids, id)
	}
	rows.Close()
	out := []resmodel.Reservation{}
	for _, id := range ids {
		r, err := f.res.Get(f.ctx, f.org, id)
		f.must(err)
		out = append(out, r)
	}
	return out
}

// sellable answers the engine's sellable count of the product for a range
// through the channel.
func (f *fx) sellable(start, end time.Time) int {
	f.t.Helper()
	res, err := f.res.Search(f.ctx, f.org, resmodel.QuoteSearchInput{
		Where:   resmodel.Where{PickupBranchUUID: &f.branch.uuid, StartsAt: &start, EndsAt: &end},
		Channel: "yolcu360", ProductUUIDs: []uuid.UUID{f.product.uuid},
	})
	f.must(err)
	for _, it := range res.Items {
		if it.Product.UUID == f.product.uuid {
			return it.Sellable
		}
	}
	return 0
}

func (f *fx) messages(ref string) []model.Message {
	f.t.Helper()
	items, _, err := f.svc.ListMessages(f.ctx, f.org, model.MessageFilter{ExternalRef: ref, SortField: "created_at", Limit: 100})
	f.must(err)
	return items
}

func (f *fx) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	f.must(f.pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}
