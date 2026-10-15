package fieldtasks_test

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/usecase"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	inspmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	inspusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with two branches (MRK with an airport and an
// address zone delivery point, HVL), a product, a customer, field staff
// and the real services wired like the API; removed when the test ends.
type fx struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	res      *resusecase.Service
	rentals  *rentalusecase.Service
	insp     *inspusecase.Service
	field    *usecase.Service
	notes    *fakeNotifier
	events   *fakeEvents
	org      int64
	orgUUID  uuid.UUID
	slug     string
	suffix   string
	segment  int64
	branch   ref
	other    ref
	airport  ref
	zone     ref
	office   ref
	product  ref
	customer ref
	driver   ref
	manager  member
	staff    member
	plate    int
	users    []int64
	clockMu  sync.Mutex
	shift    time.Duration
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

type member struct {
	id   int64
	uuid uuid.UUID
}

var managerPerms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose,
	rbac.PermTenantFieldTasksRead, rbac.PermTenantFieldTasksAssign, rbac.PermTenantFieldTasksExecute,
	rbac.PermTenantSignatureWaive, rbac.PermTenantTransfersRead, rbac.PermTenantTransfersWrite,
	rbac.PermTenantPriceOverride,
}

// Delivery staff (teslim_iade): rentals read / write and their own field
// tasks only.
var staffPerms = []string{
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantFieldTasksExecute,
}

type fakeNotifier struct {
	mu  sync.Mutex
	got []notifmodel.EnqueueInput
}

func (n *fakeNotifier) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.got = append(n.got, in)
	return nil, nil
}

type fakeEvents struct {
	mu  sync.Mutex
	got []string
}

func (e *fakeEvents) Publish(_ context.Context, resource, action string, uuids ...string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, id := range uuids {
		e.got = append(e.got, resource+":"+action+":"+id)
	}
}

func (e *fakeEvents) has(resource string, id uuid.UUID) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, g := range e.got {
		if strings.HasPrefix(g, resource+":") && strings.HasSuffix(g, ":"+id.String()) {
			return true
		}
	}
	return false
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
	f := &fx{t: t, pool: pool, q: q, suffix: strings.ReplaceAll(uuid.NewString()[:8], "-", ""), notes: &fakeNotifier{}, events: &fakeEvents{}}
	f.slug = "fld-" + f.suffix
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: f.slug, Name: "Fld " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", f.org)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", f.users)
		pool.Close()
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
	f.rentals.SetResourceEvents(usecase.MirrorPublisher{Publisher: f.events})
	fleet := fleetopsusecase.New(fleetopsrepository.NewPostgres(pool, q), vehicles)
	f.field = usecase.New(pool, q)
	f.field.SetClock(f.now)
	f.field.SetTransfers(fleet)
	f.field.SetFinisher(adapters.Finisher{Rentals: f.rentals})
	f.field.SetNotifier(f.notes)
	f.field.SetResourceEvents(f.events)
	f.res.SetDeliveryQuoter(f.field)
	f.res.SetLifecycleListener(resusecase.Listeners{f.rentals, f.field})
	f.insp = inspusecase.New(pool, q, f.rentals, storage.NewMemory(), inspusecase.NewMediaSigner("test-secret"))
	f.insp.SetClock(f.now)

	allDay := `{"mon":{"open":"00:00","close":"23:59"},"tue":{"open":"00:00","close":"23:59"},"wed":{"open":"00:00","close":"23:59"},` +
		`"thu":{"open":"00:00","close":"23:59"},"fri":{"open":"00:00","close":"23:59"},"sat":{"open":"00:00","close":"23:59"},"sun":{"open":"00:00","close":"23:59"}}`
	f.branch = f.newBranch("MRK", allDay)
	f.other = f.newBranch("HVL", allDay)
	f.product = f.newProduct("EKO")
	f.customer, f.driver = f.newCustomer()
	// Airport point: delivery extra "Havalimanı teslim" 750 per rental,
	// one hour of notice.
	var extra int64
	f.must(pool.QueryRow(ctx, `INSERT INTO extras (organization_id, code, name, extra_type, charge_type, status)
		VALUES ($1, 'TESLIM', 'Havalimanı teslim', 'delivery', 'per_rental', 'published') RETURNING id`, f.org).Scan(&extra))
	_, err = pool.Exec(ctx, `INSERT INTO extra_prices (organization_id, extra_id, price, valid_from) VALUES ($1, $2, 750, NOW() - INTERVAL '1 day')`, f.org, extra)
	f.must(err)
	f.must(pool.QueryRow(ctx, `INSERT INTO service_points (organization_id, branch_id, type, name, delivery_enabled, delivery_extra_id, min_notice_minutes)
		VALUES ($1, $2, 'airport', 'SAW Havalimanı', TRUE, $3, 60) RETURNING id, uuid`, f.org, f.branch.id, extra).Scan(&f.airport.id, &f.airport.uuid))
	// Address zone: Kadıköy / Ataşehir, 09:00-18:00, 10 km around the
	// centre, flat fee 250.
	f.must(pool.QueryRow(ctx, `INSERT INTO service_points (organization_id, branch_id, type, name, delivery_enabled, extra_fee, working_hours,
		service_districts, latitude, longitude, service_radius_km)
		VALUES ($1, $2, 'address_zone', 'Anadolu yakası', TRUE, 250, $3, ARRAY['Kadıköy', 'Ataşehir'], 40.99, 29.03, 10) RETURNING id, uuid`,
		f.org, f.branch.id, `{"mon":{"open":"09:00","close":"18:00"},"tue":{"open":"09:00","close":"18:00"},"wed":{"open":"09:00","close":"18:00"},`+
			`"thu":{"open":"09:00","close":"18:00"},"fri":{"open":"09:00","close":"18:00"},"sat":{"open":"09:00","close":"18:00"},"sun":{"open":"09:00","close":"18:00"}}`).
		Scan(&f.zone.id, &f.zone.uuid))
	f.must(pool.QueryRow(ctx, `INSERT INTO service_points (organization_id, branch_id, type, name) VALUES ($1, $2, 'office', 'Ofis') RETURNING id, uuid`,
		f.org, f.branch.id).Scan(&f.office.id, &f.office.uuid))

	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = intp(60), intp(60), intp(30)
	data, _ := json.Marshal(s)
	_, err = pool.Exec(ctx, `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	f.must(err)

	f.manager = f.newMember("sube_yoneticisi", nil)
	f.staff = f.newMember("teslim_iade", nil)
	f.ctx = f.as(f.manager, nil, managerPerms...)
	return f
}

func intp(v int) *int { return &v }

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

// as is a member's context: principal + organization scope (branches nil
// = every branch).
func (f *fx) as(m member, branches []int64, perms ...string) context.Context {
	ctx := authctx.WithPrincipal(context.Background(), authctx.Principal{UserID: m.uuid, UserInternal: m.id})
	return orgctx.WithScope(ctx, orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: f.slug, Status: orgctx.StatusActive, Permissions: perms, BranchIDs: branches,
	})
}

func (f *fx) newBranch(code, hours string) ref {
	b, err := f.q.CreateBranch(context.Background(), db.CreateBranchParams{
		OrganizationID: f.org, Code: code, Name: code + " Şube", WorkingHours: []byte(hours),
		HolidayExceptions: []byte("[]"), PrepMinutes: 30, IsActive: true,
	})
	f.must(err)
	return ref{b.ID, b.Uuid}
}

// newMember adds a user with a template role (branches: selected scope).
func (f *fx) newMember(role string, branches []int64) member {
	ctx := context.Background()
	u, err := f.q.CreateUser(ctx, db.CreateUserParams{
		Email: role + "-" + uuid.NewString()[:8] + "-fld@example.test", PasswordHash: "x", Name: role, Surname: "Fld", Status: "active",
	})
	f.must(err)
	f.users = append(f.users, u.ID)
	scope := "all"
	if branches != nil {
		scope = "selected"
	}
	var memberID int64
	f.must(f.pool.QueryRow(ctx, `INSERT INTO organization_members (organization_id, user_id, role_id, branch_scope)
		SELECT $1, $2, r.id, $4 FROM roles r WHERE r.slug = $3 AND r.scope = 'tenant' AND r.organization_id IS NULL RETURNING id`,
		f.org, u.ID, role, scope).Scan(&memberID))
	_, err = f.pool.Exec(ctx, `INSERT INTO organization_member_roles (organization_id, member_id, role_id)
		SELECT $1, $2, r.id FROM roles r WHERE r.slug = $3 AND r.scope = 'tenant' AND r.organization_id IS NULL ON CONFLICT DO NOTHING`,
		f.org, memberID, role)
	f.must(err)
	for _, b := range branches {
		_, err := f.pool.Exec(ctx, `INSERT INTO organization_member_branches (organization_id, member_id, branch_id) VALUES ($1, $2, $3)`, f.org, memberID, b)
		f.must(err)
	}
	return member{id: u.ID, uuid: u.Uuid}
}

func (f *fx) vehicle(branch ref) ref {
	f.plate++
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 FD %d", f.plate), branch.id).Scan(&r.id, &r.uuid))
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
	_, err := f.pool.Exec(ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type, rental_group_id,
		min_driver_age, min_license_years) VALUES ($1, $2, 1, 'GROUP', $3, 18, 1)`, f.org, p.id, group)
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

func (f *fx) newCustomer() (ref, ref) {
	ctx := context.Background()
	var c, d ref
	f.must(f.pool.QueryRow(ctx, `INSERT INTO customers (organization_id, customer_type, first_name, last_name, phone, email)
		VALUES ($1, 'individual', 'Ayşe', 'Yılmaz', '+905321112233', 'ayse@example.test') RETURNING id, uuid`, f.org).Scan(&c.id, &c.uuid))
	now := time.Now().UTC()
	f.must(f.pool.QueryRow(ctx, `INSERT INTO drivers (organization_id, customer_id, first_name, last_name, birth_date, licence_enc, licence_bidx, licence_hint, licence_issued_on)
		VALUES ($1, $2, 'Ayşe', 'Yılmaz', $3, 'v1:x', $4, '****456', $5) RETURNING id, uuid`,
		f.org, c.id, now.AddDate(-30, 0, 0), uuid.NewString(), now.AddDate(-10, 0, 0)).Scan(&d.id, &d.uuid))
	return c, d
}

// quote prices a reservation request.
func (f *fx) quote(w resmodel.Where) (resmodel.Quote, error) {
	return f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{Where: w, Channel: "branch", ProductUUID: &f.product.uuid, CustomerUUID: &f.customer.uuid})
}

// reserve creates a confirmed reservation from a quote request.
func (f *fx) reserve(w resmodel.Where) resmodel.Reservation {
	f.t.Helper()
	qt, err := f.quote(w)
	f.must(err)
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &qt.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	f.must(err)
	if r.Status != resmodel.StatusConfirmed {
		f.t.Fatalf("reservation status %s", r.Status)
	}
	return r
}

// tasksOf returns the field tasks of a reservation by type.
func (f *fx) tasksOf(res uuid.UUID) map[string]model.Task {
	f.t.Helper()
	items, _, err := f.field.List(f.ctx, f.org, model.Filter{ReservationUUIDs: []uuid.UUID{res}, Limit: 50})
	f.must(err)
	out := map[string]model.Task{}
	for _, t := range items {
		if prev, ok := out[t.TaskType]; ok && model.IsLive(prev.Status) && !model.IsLive(t.Status) {
			continue
		}
		out[t.TaskType] = t
	}
	return out
}

func (f *fx) vehicleState(v uuid.UUID) (status string, branch int64) {
	f.t.Helper()
	f.must(f.pool.QueryRow(context.Background(), `SELECT status, current_branch_id FROM vehicles WHERE organization_id = $1 AND uuid = $2`,
		f.org, v).Scan(&status, &branch))
	return
}

func signaturePNG() string {
	raw, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")
	return base64.StdEncoding.EncodeToString(raw)
}

func strokes() json.RawMessage {
	pts := [][]float64{}
	for i := 0; i < 12; i++ {
		pts = append(pts, []float64{float64(10 + i*5), float64(40 + i%3), 0.5, float64(i * 16)})
	}
	b, _ := json.Marshal(map[string]any{"width": 600, "height": 200, "strokes": [][][]float64{pts}})
	return b
}

func jpeg(payload string) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xD8, 0xFF, 0xFE, 0, byte(len(payload) + 2)})
	b.WriteString(payload)
	b.Write([]byte{0xFF, 0xD9})
	return b.Bytes()
}

// inspect runs a complete inspection form of a rental at a service point
// as ctx (every photo, the signature).
func (f *fx) inspect(ctx context.Context, rental uuid.UUID, kind string, point uuid.UUID, km int32) inspmodel.Inspection {
	f.t.Helper()
	ins, _, err := f.insp.Start(ctx, f.org, inspmodel.StartInput{RentalUUID: &rental, Kind: kind})
	f.must(err)
	equipment := map[string]bool{}
	for _, it := range ins.Template.Checklist {
		equipment[it.Key] = true
	}
	drv, reviewed, fuel := f.driver.uuid, true, int16(80)
	branch := f.branch.uuid
	_, err = f.insp.UpdateDraft(ctx, f.org, ins.UUID, inspmodel.DraftInput{
		BranchUUID: &branch, PointUUID: &point, Km: &km, Fuel: &fuel, Equipment: equipment, CustomerReviewed: &reviewed,
		Verification: &inspmodel.Verification{PlateConfirmed: true, VehicleOK: true, DriverUUID: &drv, LicenceChecked: true, IdentityChecked: true},
	})
	f.must(err)
	for _, sl := range ins.Template.PhotoSlots {
		data := jpeg(sl.Key + uuid.NewString())
		up, _, err := f.insp.AddPhoto(ctx, f.org, ins.UUID, inspmodel.PhotoInput{
			Slot: sl.Key, ContentType: "image/jpeg", SizeBytes: int64(len(data)), ClientRef: uuid.NewString(),
		})
		f.must(err)
		_, err = f.insp.UploadPhoto(ctx, f.org, ins.UUID, up.Photo.UUID, inspusecase.FileUpload{Body: bytes.NewReader(data), Size: int64(len(data))})
		f.must(err)
		_, err = f.insp.CompletePhoto(ctx, f.org, ins.UUID, up.Photo.UUID)
		f.must(err)
	}
	_, err = f.insp.SaveSignature(ctx, f.org, ins.UUID, inspmodel.SignatureInput{SignerName: "Ayşe Yılmaz", PNGBase64: signaturePNG(), Strokes: strokes()})
	f.must(err)
	out, err := f.insp.Complete(ctx, f.org, ins.UUID, inspmodel.CompleteInput{})
	f.must(err)
	return out
}

func stateCode(err error) string {
	var se *usecase.StateError
	if errors.As(err, &se) {
		return se.Code
	}
	var rse *rentalusecase.StateError
	if errors.As(err, &rse) {
		return rse.Code
	}
	return ""
}

func details(err error) map[string]string {
	out := map[string]string{}
	var ve *resusecase.ValidationError
	if errors.As(err, &ve) {
		for _, d := range ve.Details {
			out[d.Field] = d.Code
		}
	}
	var fve *usecase.ValidationError
	if errors.As(err, &fve) {
		for _, d := range fve.Details {
			out[d.Field] = d.Code
		}
	}
	return out
}

// at is a time n days ahead at hh:mm in Istanbul.
func at(days, hh, mm int) time.Time {
	d := rentaltime.LocalDate(time.Now()).AddDays(days)
	return time.Date(d.Year, d.Month, d.Day, hh, mm, 0, 0, rentaltime.Location()).UTC()
}

// TestRT_A29_AirportDelivery: an airport delivery reservation prices the
// delivery and collection fees, opens the delivery and collection tasks,
// the manager assigns the delivery, the executor sees it in "my tasks"
// (and nothing else), starts the trip, completes it through the handover
// form at the airport point, and the vehicle stays at the operating
// branch of the point; the collection completes through the return form.
func TestRT_A29_AirportDelivery(t *testing.T) {
	f := newFx(t)
	v := f.vehicle(f.branch)
	start, end := at(2, 10, 0), at(4, 18, 0)
	ap := f.airport.uuid
	w := resmodel.Where{
		PickupPointUUID: &ap, ReturnPointUUID: &ap, StartsAt: &start, EndsAt: &end,
		PickupDelivery: &model.Delivery{FlightNumber: "tk 2412", MeetingNote: "Dış hatlar çıkışı, kapı 3"},
	}
	qt, err := f.quote(w)
	f.must(err)
	if !qt.DeliveryFee.Equal(money.MustParse("1500")) {
		t.Fatalf("delivery + collection fee = %s, want 1500", qt.DeliveryFee)
	}
	lines := 0
	for _, l := range qt.Lines {
		if l.Kind == resmodel.LineDelivery {
			lines++
			if !l.Amount.Equal(money.MustParse("750")) || l.Name != "SAW Havalimanı" {
				t.Fatalf("delivery line %+v", l)
			}
		}
	}
	if lines != 2 || qt.PickupDelivery == nil || qt.PickupDelivery.FlightNumber != "TK2412" {
		t.Fatalf("quote lines %d, pickup delivery %+v", lines, qt.PickupDelivery)
	}
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &qt.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	f.must(err)
	if !r.Price.Total.Equal(qt.Total) || r.PickupDelivery == nil {
		t.Fatalf("reservation %+v", r)
	}

	// Confirmed with delivery: delivery (handover) and collection (return)
	// tasks at the airport, before any rental.
	tasks := f.tasksOf(r.UUID)
	delivery, collection := tasks[model.TypeHandover], tasks[model.TypeReturn]
	if delivery.UUID == uuid.Nil || collection.UUID == uuid.Nil {
		t.Fatalf("tasks of a delivery reservation: %+v", tasks)
	}
	if !delivery.Delivery || delivery.Origin != model.OriginReservation || delivery.Status != model.StatusPlanned ||
		delivery.Location.Kind != model.LocationPoint || delivery.Location.Point.UUID != f.airport.uuid ||
		delivery.FlightNumber != "TK2412" || delivery.Contact.Name != "Ayşe Yılmaz" || !delivery.DueAt.Equal(start) || delivery.Rental != nil {
		t.Fatalf("delivery task %+v", delivery)
	}
	if !collection.DueAt.Equal(end) || collection.Branch.UUID != f.branch.uuid {
		t.Fatalf("collection task %+v", collection)
	}

	// The manager assigns the delivery: the executor is notified.
	assigned, err := f.field.Assign(f.ctx, f.org, delivery.UUID, &f.staff.uuid)
	f.must(err)
	if assigned.Status != model.StatusAssigned || assigned.Assignee == nil || assigned.Assignee.UUID != f.staff.uuid {
		t.Fatalf("assigned %+v", assigned)
	}
	if len(f.notes.got) != 1 || *f.notes.got[0].UserID != f.staff.id || *f.notes.got[0].OrganizationID != f.org ||
		f.notes.got[0].TemplateCode != "field_tasks.assigned" || !strings.Contains(*f.notes.got[0].ActionURL, delivery.UUID.String()) {
		t.Fatalf("assignment notification %+v", f.notes.got)
	}
	if !f.events.has(model.ResourceMine, delivery.UUID) || !f.events.has(model.Resource, delivery.UUID) {
		t.Fatalf("live events %v", f.events.got)
	}

	// The executor sees their task and nothing else.
	staff := f.as(f.staff, nil, staffPerms...)
	mine, total, err := f.field.Mine(staff, f.org, model.Filter{DueFrom: &start, Limit: 20})
	f.must(err)
	if total != 1 || mine[0].UUID != delivery.UUID {
		t.Fatalf("my tasks %d %+v", total, mine)
	}
	if _, err := f.field.Get(staff, f.org, collection.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("an executor reads another task: %v", err)
	}
	if _, err := f.field.Start(staff, f.org, collection.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("an executor starts another task: %v", err)
	}
	if _, err := f.field.Cancel(staff, f.org, delivery.UUID, "olmaz"); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("an executor cancels: %v", err)
	}

	// The plate is fixed: the rental adopts the delivery task (same task,
	// assignee kept) and sets its vehicle.
	rental, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &r.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	got, err := f.field.Get(staff, f.org, delivery.UUID)
	f.must(err)
	if got.Rental == nil || got.Rental.UUID != rental.UUID || got.Vehicle == nil || got.Vehicle.UUID != v.uuid ||
		got.Assignee == nil || got.Inspection != "handover" || got.Location.Point == nil {
		t.Fatalf("delivery task after the rental %+v", got)
	}

	// Start the trip; completing by hand is refused (the handover form
	// completes it).
	got, err = f.field.Start(staff, f.org, delivery.UUID)
	f.must(err)
	if got.Status != model.StatusEnRoute || got.StartedAt == nil {
		t.Fatalf("started %+v", got)
	}
	if _, err := f.field.Complete(staff, f.org, delivery.UUID, ""); stateCode(err) != "record_handover" {
		t.Fatalf("complete a delivery by hand: %v", err)
	}
	f.inspect(staff, rental.UUID, inspmodel.KindHandover, f.airport.uuid, 1100)
	got, err = f.field.Get(f.ctx, f.org, delivery.UUID)
	f.must(err)
	if got.Status != model.StatusCompleted || got.CompletedAt == nil {
		t.Fatalf("delivery after the handover form %+v", got)
	}
	status, branch := f.vehicleState(v.uuid)
	if status != "rented" || branch != f.branch.id {
		t.Fatalf("vehicle after the delivery: %s at %d (want rented at the point's branch %d)", status, branch, f.branch.id)
	}
	var pickupPoint int64
	f.must(f.pool.QueryRow(context.Background(), `SELECT actual_pickup_point_id FROM rentals WHERE organization_id = $1 AND uuid = $2`,
		f.org, rental.UUID).Scan(&pickupPoint))
	if pickupPoint != f.airport.id {
		t.Fatalf("actual pick-up point %d, want the airport %d", pickupPoint, f.airport.id)
	}

	// The handover adopted the collection task as the rental's return task.
	back, err := f.field.Get(f.ctx, f.org, collection.UUID)
	f.must(err)
	if back.Rental == nil || back.Rental.UUID != rental.UUID || back.Inspection != "return" || back.Location.Point.UUID != f.airport.uuid {
		t.Fatalf("collection after the handover %+v", back)
	}
	ops, _, err := f.rentals.ListTasks(f.ctx, f.org, rentalmodel.TaskFilter{RentalUUIDs: []uuid.UUID{rental.UUID}, TaskTypes: []string{"return"}, Limit: 10})
	f.must(err)
	if len(ops) != 1 || ops[0].UUID != collection.UUID {
		t.Fatalf("one return task, the collection: %+v", ops)
	}
	_, err = f.field.Assign(f.ctx, f.org, collection.UUID, &f.staff.uuid)
	f.must(err)
	_, err = f.field.Start(staff, f.org, collection.UUID)
	f.must(err)
	f.advance(50 * time.Hour)
	f.inspect(staff, rental.UUID, inspmodel.KindReturn, f.airport.uuid, 1300)
	back, err = f.field.Get(f.ctx, f.org, collection.UUID)
	f.must(err)
	if back.Status != model.StatusCompleted {
		t.Fatalf("collection after the return form %+v", back)
	}
	status, branch = f.vehicleState(v.uuid)
	if branch != f.branch.id || status == "rented" {
		t.Fatalf("vehicle after the collection: %s at %d", status, branch)
	}

	// The timeline of the delivery: created, assigned, en route, completed.
	events, err := f.field.Events(f.ctx, f.org, delivery.UUID)
	f.must(err)
	seen := []string{}
	for _, e := range events {
		if e.Action == "status_changed" {
			seen = append(seen, *e.ToStatus)
		} else {
			seen = append(seen, e.Action)
		}
	}
	if got := strings.Join(seen, ","); !strings.HasPrefix(got, "created,assignee_changed,assigned") ||
		!strings.Contains(got, "en_route") || !strings.HasSuffix(got, "completed") {
		t.Fatalf("timeline %s", got)
	}
}

// TestDeliveryEligibility: an address zone refuses an address outside its
// districts or radius, a time outside its hours, a missing address; an
// airport refuses a time inside its notice; an office does not deliver to
// an address; the quote search shows the same refusal.
func TestDeliveryEligibility(t *testing.T) {
	f := newFx(t)
	f.vehicle(f.branch)
	zone, ap, office := f.zone.uuid, f.airport.uuid, f.office.uuid
	b := f.branch.uuid
	open, late := at(3, 11, 0), at(3, 20, 0)
	end := at(5, 11, 0)
	cases := []struct {
		name  string
		w     resmodel.Where
		field string
		code  string
	}{
		{"outside the districts", resmodel.Where{PickupPointUUID: &zone, ReturnBranchUUID: &b, StartsAt: &open, EndsAt: &end,
			PickupDelivery: &model.Delivery{Address: "Barbaros Blv. 1", District: "Beşiktaş"}}, "pickup_delivery.district", "outside_area"},
		{"outside the radius", resmodel.Where{PickupPointUUID: &zone, ReturnBranchUUID: &b, StartsAt: &open, EndsAt: &end,
			PickupDelivery: &model.Delivery{Address: "Bağdat Cd. 1", District: "Kadıköy", Latitude: fp(41.5), Longitude: fp(29.03)}},
			"pickup_delivery.address", "outside_area"},
		{"outside the hours", resmodel.Where{PickupPointUUID: &zone, ReturnBranchUUID: &b, StartsAt: &late, EndsAt: &end,
			PickupDelivery: &model.Delivery{Address: "Bağdat Cd. 1", District: "kadiköy"}}, "pickup_point_uuid", "outside_hours"},
		{"no address", resmodel.Where{PickupPointUUID: &zone, ReturnBranchUUID: &b, StartsAt: &open, EndsAt: &end},
			"pickup_delivery.address", "address_required"},
		{"office address", resmodel.Where{PickupPointUUID: &office, ReturnBranchUUID: &b, StartsAt: &open, EndsAt: &end,
			PickupDelivery: &model.Delivery{Address: "Bağdat Cd. 1"}}, "pickup_delivery.address", "not_delivery_point"},
		{"address without a point", resmodel.Where{PickupBranchUUID: &b, StartsAt: &open, EndsAt: &end,
			PickupDelivery: &model.Delivery{Address: "Bağdat Cd. 1"}}, "pickup_delivery.address", "point_required"},
	}
	for _, c := range cases {
		_, err := f.quote(c.w)
		if got := details(err); got[c.field] != c.code {
			t.Fatalf("%s: %v (%v)", c.name, got, err)
		}
	}
	// The notice of the airport (60 minutes).
	soon := f.now().Add(30 * time.Minute).Truncate(time.Minute)
	soonEnd := soon.Add(24 * time.Hour)
	if _, err := f.quote(resmodel.Where{PickupPointUUID: &ap, ReturnBranchUUID: &b, StartsAt: &soon, EndsAt: &soonEnd}); details(err)["pickup_point_uuid"] != "too_soon" {
		t.Fatalf("too soon: %v", err)
	}
	// The search refuses the same way.
	if _, err := f.res.Search(f.ctx, f.org, resmodel.QuoteSearchInput{
		Where: resmodel.Where{PickupPointUUID: &zone, ReturnBranchUUID: &b, StartsAt: &late, EndsAt: &end,
			PickupDelivery: &model.Delivery{Address: "Bağdat Cd. 1", District: "Kadıköy"}}, Channel: "branch",
	}); details(err)["pickup_point_uuid"] != "outside_hours" {
		t.Fatalf("search outside the hours: %v", err)
	}

	// Inside: the zone's flat fee is one delivery line; the search total
	// carries it too; the office adds nothing.
	inside := resmodel.Where{PickupPointUUID: &zone, ReturnBranchUUID: &b, StartsAt: &open, EndsAt: &end,
		PickupDelivery: &model.Delivery{Address: "Bağdat Cd. 1", District: "Kadıköy", Latitude: fp(40.98), Longitude: fp(29.05)}}
	qt, err := f.quote(inside)
	f.must(err)
	if !qt.DeliveryFee.Equal(money.MustParse("250")) || !qt.Total.Equal(money.MustParse("2250")) {
		t.Fatalf("zone quote fee %s total %s", qt.DeliveryFee, qt.Total)
	}
	found, err := f.res.Search(f.ctx, f.org, resmodel.QuoteSearchInput{Where: inside, Channel: "branch"})
	f.must(err)
	if len(found.Items) != 1 || found.Items[0].Price == nil || !found.Items[0].Price.Total.Equal(money.MustParse("2250")) {
		t.Fatalf("search with delivery %+v", found.Items)
	}
	plain, err := f.quote(resmodel.Where{PickupPointUUID: &office, ReturnBranchUUID: &b, StartsAt: &open, EndsAt: &end})
	f.must(err)
	if !plain.DeliveryFee.IsZero() {
		t.Fatalf("office fee %s", plain.DeliveryFee)
	}
	// A staff fee replacing the computed one needs price.override.
	noOverride := f.as(f.manager, nil, rbac.PermTenantQuotesWrite, rbac.PermTenantQuotesRead)
	if _, err := f.res.CreateQuote(noOverride, f.org, resmodel.QuoteInput{Where: inside, Channel: "branch", ProductUUID: &f.product.uuid,
		DeliveryFee: "100"}); !errors.Is(err, resusecase.ErrForbidden) {
		t.Fatalf("staff fee without price.override: %v", err)
	}
	over, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{Where: inside, Channel: "branch", ProductUUID: &f.product.uuid, DeliveryFee: "100"})
	f.must(err)
	if !over.DeliveryFee.Equal(money.MustParse("100")) {
		t.Fatalf("staff fee %s", over.DeliveryFee)
	}

	// The wizard's picker lists the delivery points with their fees.
	points, err := f.field.DeliveryPoints(f.ctx, f.org, &b, &f.product.uuid)
	f.must(err)
	fees := map[string]string{}
	for _, p := range points {
		fees[p.Name] = p.Fee + "/" + p.FeeSource
	}
	if fees["SAW Havalimanı"] != "750.00/extra" || fees["Anadolu yakası"] != "250.00/flat" || len(points) != 2 {
		t.Fatalf("delivery points %v", fees)
	}
}

func fp(v float64) *float64 { return &v }

// TestDeliveryTasksFollowTheReservation: a change of dates moves the
// delivery tasks, dropping the delivery cancels the task before the
// rental, and cancelling the reservation cancels the rest.
func TestDeliveryTasksFollowTheReservation(t *testing.T) {
	f := newFx(t)
	f.vehicle(f.branch)
	ap, b := f.airport.uuid, f.branch.uuid
	start, end := at(3, 9, 0), at(5, 9, 0)
	r := f.reserve(resmodel.Where{PickupPointUUID: &ap, ReturnPointUUID: &ap, StartsAt: &start, EndsAt: &end})
	before := f.tasksOf(r.UUID)
	_, err := f.field.Assign(f.ctx, f.org, before[model.TypeHandover].UUID, &f.staff.uuid)
	f.must(err)

	// Two hours later: both tasks move, the assignee stays.
	start2, end2 := start.Add(2*time.Hour), end.Add(2*time.Hour)
	cq, err := f.res.ChangeQuote(f.ctx, f.org, r.UUID, resmodel.ChangeQuoteInput{Where: resmodel.Where{StartsAt: &start2, EndsAt: &end2}})
	f.must(err)
	_, err = f.res.ApplyChange(f.ctx, f.org, r.UUID, resmodel.ApplyChangeInput{QuoteUUID: &cq.UUID})
	f.must(err)
	moved := f.tasksOf(r.UUID)
	if !moved[model.TypeHandover].DueAt.Equal(start2) || !moved[model.TypeReturn].DueAt.Equal(end2) ||
		moved[model.TypeHandover].UUID != before[model.TypeHandover].UUID || moved[model.TypeHandover].Assignee == nil {
		t.Fatalf("tasks after a date change %+v", moved)
	}
	events, err := f.field.Events(f.ctx, f.org, moved[model.TypeHandover].UUID)
	f.must(err)
	rescheduled := false
	for _, e := range events {
		rescheduled = rescheduled || e.Action == "rescheduled"
	}
	if !rescheduled {
		t.Fatalf("no rescheduled event: %+v", events)
	}

	// The pick-up becomes the branch desk: the delivery task is cancelled.
	cq, err = f.res.ChangeQuote(f.ctx, f.org, r.UUID, resmodel.ChangeQuoteInput{Where: resmodel.Where{PickupBranchUUID: &b}})
	f.must(err)
	if !cq.DeliveryFee.Equal(money.MustParse("750")) {
		t.Fatalf("change quote keeps the collection fee only: %s", cq.DeliveryFee)
	}
	_, err = f.res.ApplyChange(f.ctx, f.org, r.UUID, resmodel.ApplyChangeInput{QuoteUUID: &cq.UUID})
	f.must(err)
	after := f.tasksOf(r.UUID)
	if after[model.TypeHandover].Status != model.StatusCancelled || after[model.TypeReturn].Status != model.StatusPlanned {
		t.Fatalf("tasks after dropping the delivery %+v", after)
	}

	// Cancelling the reservation cancels the collection.
	_, err = f.res.Cancel(f.ctx, f.org, r.UUID, "müşteri vazgeçti")
	f.must(err)
	final := f.tasksOf(r.UUID)
	if final[model.TypeReturn].Status != model.StatusCancelled || !strings.Contains(final[model.TypeReturn].CompletionNote, "cancelled") {
		t.Fatalf("tasks after the cancellation %+v", final)
	}
}

// TestFieldTaskTransitions: the state machine refuses moves that do not
// fit, an issue needs a note and the trip resumes after it, a cleaning
// task completes by hand, a transfer moves the vehicle's branch through
// the fleet transfer, and the timeline records each step.
func TestFieldTaskTransitions(t *testing.T) {
	f := newFx(t)
	v := f.vehicle(f.branch)
	b := f.branch.uuid
	due := f.now().Add(2 * time.Hour).Truncate(time.Minute)
	clean, err := f.field.Create(f.ctx, f.org, model.CreateInput{
		TaskType: model.TypeCleaning, BranchUUID: &b, VehicleUUID: &v.uuid, DueAt: &due, AssigneeUUID: &f.staff.uuid,
	})
	f.must(err)
	if clean.Status != model.StatusAssigned || clean.PlannedMinutes != 45 {
		t.Fatalf("created %+v", clean)
	}
	staff := f.as(f.staff, nil, staffPerms...)
	if _, err := f.field.ReportIssue(staff, f.org, clean.UUID, ""); details(err)["note"] != "required" {
		t.Fatalf("issue without a note: %v", err)
	}
	got, err := f.field.Start(staff, f.org, clean.UUID)
	f.must(err)
	got, err = f.field.ReportIssue(staff, f.org, got.UUID, "Yıkama makinesi arızalı")
	f.must(err)
	if got.Status != model.StatusIssueReported || got.IssueNote != "Yıkama makinesi arızalı" || got.IssueAt == nil {
		t.Fatalf("issue %+v", got)
	}
	if _, err := f.field.ReportIssue(staff, f.org, got.UUID, "yine"); stateCode(err) != "invalid_transition" {
		t.Fatalf("issue twice: %v", err)
	}
	got, err = f.field.Start(staff, f.org, got.UUID)
	f.must(err)
	if got.Status != model.StatusEnRoute {
		t.Fatalf("resumed %+v", got)
	}
	got, err = f.field.Complete(staff, f.org, got.UUID, "temiz")
	f.must(err)
	if got.Status != model.StatusCompleted || got.CompletedBy == nil || got.CompletedBy.UUID != f.staff.uuid {
		t.Fatalf("completed %+v", got)
	}
	if _, err := f.field.Start(staff, f.org, got.UUID); stateCode(err) != "task_closed" {
		t.Fatalf("start a completed task: %v", err)
	}
	if _, err := f.field.Assign(f.ctx, f.org, got.UUID, &f.manager.uuid); stateCode(err) != "task_closed" {
		t.Fatalf("assign a completed task: %v", err)
	}
	events, err := f.field.Events(f.ctx, f.org, clean.UUID)
	f.must(err)
	trail := []string{}
	for _, e := range events {
		if e.ToStatus != nil {
			trail = append(trail, *e.ToStatus)
		}
	}
	if strings.Join(trail, ",") != "planned,assigned,en_route,issue_reported,en_route,completed" {
		t.Fatalf("status trail %v", trail)
	}
	for _, e := range events {
		if e.ToStatus != nil && *e.ToStatus == model.StatusIssueReported && (e.Note != "Yıkama makinesi arızalı" || e.Actor == nil) {
			t.Fatalf("issue event %+v", e)
		}
	}

	// Cancelling needs a reason and the planner.
	other, err := f.field.Create(f.ctx, f.org, model.CreateInput{TaskType: model.TypeOther, BranchUUID: &b, DueAt: &due})
	f.must(err)
	if _, err := f.field.Cancel(f.ctx, f.org, other.UUID, ""); details(err)["note"] != "required" {
		t.Fatalf("cancel without a reason: %v", err)
	}
	if _, err := f.field.Complete(staff, f.org, other.UUID, ""); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("an executor completes an unassigned task: %v", err)
	}
	got, err = f.field.Cancel(f.ctx, f.org, other.UUID, "gerek kalmadı")
	f.must(err)
	if got.Status != model.StatusCancelled {
		t.Fatalf("cancelled %+v", got)
	}

	// Transfer MRK → HVL: start puts the vehicle in transfer, completion
	// moves its current branch.
	h := f.other.uuid
	tr, err := f.field.Create(f.ctx, f.org, model.CreateInput{
		TaskType: model.TypeTransfer, BranchUUID: &b, ToBranchUUID: &h, VehicleUUID: &v.uuid, DueAt: &due, AssigneeUUID: &f.staff.uuid,
	})
	f.must(err)
	if tr.Transfer == nil || tr.Transfer.Status != "planned" || tr.ToBranch == nil || tr.ToBranch.UUID != h {
		t.Fatalf("transfer task %+v", tr)
	}
	_, err = f.field.Start(staff, f.org, tr.UUID)
	f.must(err)
	if status, _ := f.vehicleState(v.uuid); status != "in_transfer" {
		t.Fatalf("vehicle on the road: %s", status)
	}
	done, err := f.field.Complete(staff, f.org, tr.UUID, "")
	f.must(err)
	status, branch := f.vehicleState(v.uuid)
	if done.Status != model.StatusCompleted || done.Transfer.Status != "completed" || branch != f.other.id || status != "available" {
		t.Fatalf("transfer done %+v; vehicle %s at %d", done, status, branch)
	}

	// Pure state machine table.
	for _, c := range []struct{ from, action, to string }{
		{model.StatusPlanned, model.ActionAssign, model.StatusAssigned},
		{model.StatusAssigned, model.ActionUnassign, model.StatusPlanned},
		{model.StatusEnRoute, model.ActionReport, model.StatusIssueReported},
		{model.StatusIssueReported, model.ActionStart, model.StatusEnRoute},
	} {
		if to, ok := model.Next(c.from, c.action); !ok || to != c.to {
			t.Fatalf("%s --%s--> %s, got %s %v", c.from, c.action, c.to, to, ok)
		}
	}
	for _, c := range [][2]string{
		{model.StatusPlanned, model.ActionReport}, {model.StatusEnRoute, model.ActionStart},
		{model.StatusCompleted, model.ActionStart}, {model.StatusCancelled, model.ActionAssign},
	} {
		if _, ok := model.Next(c[0], c[1]); ok {
			t.Fatalf("%s --%s--> must be refused", c[0], c[1])
		}
	}
}

// TestFieldTaskBranchScope: a member limited to HVL sees HVL tasks only,
// cannot open a task at MRK, and an executor limited to HVL cannot be
// assigned an MRK task.
func TestFieldTaskBranchScope(t *testing.T) {
	f := newFx(t)
	b, h := f.branch.uuid, f.other.uuid
	due := f.now().Add(3 * time.Hour).Truncate(time.Minute)
	mrk, err := f.field.Create(f.ctx, f.org, model.CreateInput{TaskType: model.TypeOther, BranchUUID: &b, DueAt: &due})
	f.must(err)
	hvl, err := f.field.Create(f.ctx, f.org, model.CreateInput{TaskType: model.TypeOther, BranchUUID: &h, DueAt: &due})
	f.must(err)
	scoped := f.as(f.manager, []int64{f.other.id}, managerPerms...)
	items, total, err := f.field.List(scoped, f.org, model.Filter{Limit: 50})
	f.must(err)
	if total != 1 || items[0].UUID != hvl.UUID {
		t.Fatalf("scoped board %d %+v", total, items)
	}
	facets, err := f.field.Facets(scoped, f.org)
	f.must(err)
	for _, fv := range facets.Branch {
		if fv.Value == b.String() {
			t.Fatalf("scoped facets name MRK: %+v", facets.Branch)
		}
	}
	if _, err := f.field.Get(scoped, f.org, mrk.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("scoped get of MRK: %v", err)
	}
	if _, err := f.field.Assign(scoped, f.org, mrk.UUID, &f.staff.uuid); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("scoped assign of MRK: %v", err)
	}
	if _, err := f.field.Create(scoped, f.org, model.CreateInput{TaskType: model.TypeOther, BranchUUID: &b, DueAt: &due}); details(err)["branch_uuid"] != "not_found" {
		t.Fatalf("scoped create at MRK: %v", err)
	}
	hvlStaff := f.newMember("teslim_iade", []int64{f.other.id})
	if _, err := f.field.Assign(f.ctx, f.org, mrk.UUID, &hvlStaff.uuid); details(err)["user_uuid"] != "branch_not_allowed" {
		t.Fatalf("assign an HVL executor to MRK: %v", err)
	}
	_, err = f.field.Assign(f.ctx, f.org, hvl.UUID, &hvlStaff.uuid)
	f.must(err)
	// A member whose roles do not work field tasks cannot be assigned.
	sales := f.newMember("muhasebe", nil)
	if _, err := f.field.Assign(f.ctx, f.org, hvl.UUID, &sales.uuid); details(err)["user_uuid"] != "not_field_staff" {
		t.Fatalf("assign a non field member: %v", err)
	}
	// Assignees in the scoped view: those covering HVL.
	people, err := f.field.Assignees(scoped, f.org, "")
	f.must(err)
	names := map[uuid.UUID]bool{}
	for _, p := range people {
		names[p.UUID] = true
	}
	if !names[hvlStaff.uuid] || !names[f.staff.uuid] || names[sales.uuid] {
		t.Fatalf("assignees %+v", people)
	}
}

// TestFieldTaskIsolation: another organization's tasks, members, points
// and branches are unknown.
func TestFieldTaskIsolation(t *testing.T) {
	a := newFx(t)
	b := newFx(t)
	due := a.now().Add(3 * time.Hour).Truncate(time.Minute)
	ab := a.branch.uuid
	task, err := a.field.Create(a.ctx, a.org, model.CreateInput{TaskType: model.TypeOther, BranchUUID: &ab, DueAt: &due})
	f := a
	f.must(err)
	if _, err := b.field.Get(b.ctx, b.org, task.UUID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("B reads A's task: %v", err)
	}
	if _, err := b.field.Assign(b.ctx, b.org, task.UUID, &b.staff.uuid); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("B assigns A's task: %v", err)
	}
	if _, err := a.field.Assign(a.ctx, a.org, task.UUID, &b.staff.uuid); details(err)["user_uuid"] != "not_found" {
		t.Fatalf("A assigns B's member: %v", err)
	}
	bp := b.airport.uuid
	if _, err := a.field.Patch(a.ctx, a.org, task.UUID, model.PatchInput{PlaceInput: model.PlaceInput{PointUUID: &bp}}); details(err)["point_uuid"] != "not_found" {
		t.Fatalf("A places a task at B's point: %v", err)
	}
	bb := b.branch.uuid
	if _, err := a.field.Create(a.ctx, a.org, model.CreateInput{TaskType: model.TypeOther, BranchUUID: &bb, DueAt: &due}); details(err)["branch_uuid"] != "not_found" {
		t.Fatalf("A opens a task at B's branch: %v", err)
	}
	items, _, err := b.field.List(b.ctx, b.org, model.Filter{Limit: 50})
	f.must(err)
	for _, it := range items {
		if it.UUID == task.UUID {
			t.Fatalf("B's board lists A's task")
		}
	}
	start, end := at(3, 10, 0), at(4, 10, 0)
	if _, err := a.quote(resmodel.Where{PickupPointUUID: &bp, StartsAt: &start, EndsAt: &end}); details(err)["pickup_point_uuid"] != "not_found" {
		t.Fatalf("A quotes at B's point: %v", err)
	}
	points, err := a.field.DeliveryPoints(a.ctx, a.org, &bb, nil)
	f.must(err)
	if len(points) != 0 {
		t.Fatalf("A lists B's delivery points: %+v", points)
	}
}
