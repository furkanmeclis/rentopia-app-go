package inspections_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/usecase"
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fx is one organization with a branch, vehicles, a product (100 km a day,
// 5 TRY per extra km), settings (late fee 150 / hour after 30 min, fuel
// 10 TRY per missing percent) and a customer, removed when the test ends.
type fx struct {
	t         *testing.T
	ctx       context.Context
	pool      *pgxpool.Pool
	q         *db.Queries
	res       *resusecase.Service
	rentals   *rentalusecase.Service
	contracts *contractsusecase.Service
	svc       *usecase.Service
	store     *storage.Memory
	org       int64
	orgUUID   uuid.UUID
	suffix    string
	segment   int64
	branch    ref
	product   ref
	customer  ref
	driver    ref
	plate     int
	clockMu   sync.Mutex
	shift     time.Duration
}

type ref struct {
	id   int64
	uuid uuid.UUID
}

var allPerms = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsCancel,
	rbac.PermTenantReservationsConfirm, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantRentalsRead, rbac.PermTenantRentalsWrite, rbac.PermTenantRentalsClose,
	rbac.PermTenantContractsRead, rbac.PermTenantContractsWrite, rbac.PermTenantContractsActivate,
	rbac.PermTenantDamageApprove, rbac.PermTenantSignatureWaive, rbac.PermTenantPriceOverride,
}

type fakePDF struct{}

func (fakePDF) RenderPDF(_ context.Context, html []byte) ([]byte, error) {
	return append([]byte("%PDF-1.4\n"), html[:16]...), nil
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
	f := &fx{t: t, pool: pool, q: q, suffix: strings.ReplaceAll(uuid.NewString()[:8], "-", ""), store: storage.NewMemory()}
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "ins-" + f.suffix, Name: "Ins " + f.suffix})
	if err != nil {
		t.Fatal(err)
	}
	f.org, f.orgUUID = org.ID, org.Uuid
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", f.org)
		pool.Close()
	})
	f.ctx = f.as(allPerms...)
	f.must(pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.org).Scan(&f.segment))

	settings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	vehicles := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	avail := availusecase.New(pool, q, vehicles, settings)
	avail.SetClock(f.now)
	pricing := pricingusecase.New(pricingrepository.NewPostgres(pool, q), settings)
	f.res = resusecase.New(pool, q, avail, pricing, settings)
	f.res.SetClock(f.now)
	f.contracts = contractsusecase.New(pool, q, settings, f.store)
	f.contracts.SetRenderer(fakePDF{})
	f.contracts.SetClock(f.now)
	f.rentals = rentalusecase.New(pool, q, avail, rentaladapters.NewReservations(f.res), vehicles)
	f.rentals.SetClock(f.now)
	f.rentals.SetDocuments(f.contracts)
	f.res.SetLifecycleListener(f.rentals)
	f.svc = usecase.New(pool, q, f.rentals, f.store, usecase.NewMediaSigner("test-secret"))
	f.svc.SetContracts(f.contracts)
	f.svc.SetClock(f.now)

	b, err := q.CreateBranch(ctx, db.CreateBranchParams{
		OrganizationID: f.org, Code: "MRK", Name: "Merkez", WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
		PrepMinutes: 60, IsActive: true,
	})
	f.must(err)
	f.branch = ref{b.ID, b.Uuid}
	f.product = f.newProduct("EKO")
	f.customer, f.driver = f.newCustomer()
	grace, fee := 30, "150"
	s := osmodel.Defaults()
	s.Reservation.ApprovalMode = osmodel.ApprovalAuto
	s.Reservation.StaffReviewWindowMinutes, s.Reservation.PaymentHoldMinutes, s.Reservation.QuoteValidityMinutes = intp(60), intp(60), intp(30)
	s.Policies.LateReturnGraceMinutes, s.Policies.ExtraHourFee = &grace, fee
	s.Policies.FuelFeePerPercent = "10"
	s.Company.LegalName = "Test Oto A.Ş."
	data, _ := json.Marshal(s)
	_, err = pool.Exec(ctx, `INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, 1, $2)`, f.org, data)
	f.must(err)
	return f
}

// as is the organization context with the given permissions.
func (f *fx) as(perms ...string) context.Context {
	return orgctx.WithScope(context.Background(), orgctx.Scope{
		ID: f.org, UUID: f.orgUUID, Slug: "ins-" + f.suffix, Status: orgctx.StatusActive, Permissions: perms,
	})
}

func without(drop ...string) []string {
	out := []string{}
	for _, p := range allPerms {
		keep := true
		for _, d := range drop {
			keep = keep && p != d
		}
		if keep {
			out = append(out, p)
		}
	}
	return out
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

func (f *fx) vehicle() ref {
	f.plate++
	var r ref
	f.must(f.pool.QueryRow(context.Background(), `INSERT INTO vehicles (organization_id, plate, manual_entry, manual_brand, manual_model, model_year,
		home_branch_id, current_branch_id, odometer_km) VALUES ($1, $2, TRUE, 'Renault', 'Clio', 2024, $3, $3, 1000) RETURNING id, uuid`,
		f.org, fmt.Sprintf("34 IN %d", f.plate), f.branch.id).Scan(&r.id, &r.uuid))
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
		min_driver_age, min_license_years, included_km_per_day, extra_km_fee, fuel_policy, deposit_amount)
		VALUES ($1, $2, 1, 'GROUP', $3, 18, 1, 100, 5, 'same_to_same', 5000)`, f.org, p.id, group)
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

// rental creates a confirmed reservation of 24 hours from an hour from now
// and its rental (in preparation) on a new vehicle.
func (f *fx) rental() rentalmodel.Rental {
	f.t.Helper()
	v := f.vehicle()
	from := f.now().Add(time.Hour).Truncate(time.Minute)
	to := from.Add(24 * time.Hour)
	b := f.branch.uuid
	qt, err := f.res.CreateQuote(f.ctx, f.org, resmodel.QuoteInput{
		Where: resmodel.Where{PickupBranchUUID: &b, StartsAt: &from, EndsAt: &to}, Channel: "branch", ProductUUID: &f.product.uuid,
	})
	f.must(err)
	r, err := f.res.Create(f.ctx, f.org, resmodel.CreateInput{
		QuoteUUID: &qt.UUID, CustomerUUID: &f.customer.uuid, Drivers: []resmodel.DriverInput{{DriverUUID: f.driver.uuid}},
	})
	f.must(err)
	rental, err := f.rentals.Create(f.ctx, f.org, rentalmodel.CreateInput{ReservationUUID: &r.UUID, VehicleUUID: &v.uuid})
	f.must(err)
	return rental
}

// jpeg is a tiny JPEG with an EXIF block (make, capture time with offset,
// GPS) and a payload that makes every photo distinct.
func jpeg(payload string) []byte {
	tiff := exifTIFF()
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xD8})
	seg := append([]byte("Exif\x00\x00"), tiff...)
	b.Write([]byte{0xFF, 0xE1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)})
	b.Write(seg)
	b.Write([]byte{0xFF, 0xFE, 0, byte(len(payload) + 2)})
	b.WriteString(payload)
	b.Write([]byte{0xFF, 0xD9})
	return b.Bytes()
}

// photo announces, uploads and completes a photo of a slot.
func (f *fx) photo(ctx context.Context, ins uuid.UUID, slot string, damage *uuid.UUID, complete bool) (model.Photo, []byte) {
	f.t.Helper()
	data := jpeg(slot + uuid.NewString())
	up, _, err := f.svc.AddPhoto(ctx, f.org, ins, model.PhotoInput{
		Slot: slot, DamageUUID: damage, ContentType: "image/jpeg", SizeBytes: int64(len(data)), ClientRef: uuid.NewString(),
	})
	f.must(err)
	p, err := f.svc.UploadPhoto(ctx, f.org, ins, up.Photo.UUID, usecase.FileUpload{Body: bytes.NewReader(data), Size: int64(len(data))})
	f.must(err)
	if complete {
		p, err = f.svc.CompletePhoto(ctx, f.org, ins, up.Photo.UUID)
		f.must(err)
	}
	return p, data
}

func signaturePNG() string {
	// A 1x1 PNG.
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

// fill completes steps 1, 3 and 6 of the wizard on a draft.
func (f *fx) fill(ctx context.Context, ins model.Inspection, km int32, fuel int16, equipment map[string]bool) model.Inspection {
	f.t.Helper()
	if equipment == nil {
		equipment = map[string]bool{}
		for _, it := range ins.Template.Checklist {
			equipment[it.Key] = true
		}
	}
	drv := f.driver.uuid
	reviewed := true
	out, err := f.svc.UpdateDraft(ctx, f.org, ins.UUID, model.DraftInput{
		Km: &km, Fuel: &fuel, Equipment: equipment, CustomerReviewed: &reviewed,
		Verification: &model.Verification{PlateConfirmed: true, VehicleOK: true, DriverUUID: &drv, LicenceChecked: true, IdentityChecked: true},
	})
	f.must(err)
	return out
}

func stateCode(err error) string {
	var se *usecase.StateError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// handOver runs a complete handover form (every photo, signature).
func (f *fx) handOver(r rentalmodel.Rental) model.Inspection {
	f.t.Helper()
	ins, _, err := f.svc.Start(f.ctx, f.org, model.StartInput{RentalUUID: &r.UUID, Kind: model.KindHandover})
	f.must(err)
	ins = f.fill(f.ctx, ins, 1100, 80, nil)
	for _, sl := range ins.Template.PhotoSlots {
		f.photo(f.ctx, ins.UUID, sl.Key, nil, true)
	}
	_, err = f.svc.SaveSignature(f.ctx, f.org, ins.UUID, model.SignatureInput{SignerName: "Ayşe Yılmaz", PNGBase64: signaturePNG(), Strokes: strokes()})
	f.must(err)
	out, err := f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{})
	f.must(err)
	return out
}

// TestRT_A17_UnuploadedPhotoBlocksFinalize: the mobile handover form
// cannot become final while a photo is not uploaded and completed (the
// server enforces what the disabled "Tamamla" button shows); the
// completion hashes the stored bytes on the server, keeps EXIF apart from
// the server time, records the contract approval with the tablet
// signature, makes the rental active through the rentals use case, and the
// finalized form is immutable (a correction is a new version with a
// reason).
func TestRT_A17_UnuploadedPhotoBlocksFinalize(t *testing.T) {
	f := newFx(t)
	name := "Kiralama sözleşmesi"
	body := "<p>Kiracı: {{renter.full_name}}</p><p>Araç: {{vehicle.plate}}</p>"
	tpl, err := f.contracts.CreateTemplate(f.ctx, f.org, contractsmodel.TemplateInput{Kind: contractsmodel.KindRentalContract, Name: &name, BodyHTML: &body})
	f.must(err)
	_, err = f.contracts.ActivateTemplate(f.ctx, f.org, tpl.UUID)
	f.must(err)
	r := f.rental()
	if !r.Contract.Required || r.Contract.DocumentUUID == nil {
		t.Fatalf("contract state %+v", r.Contract)
	}

	ins, created, err := f.svc.Start(f.ctx, f.org, model.StartInput{RentalUUID: &r.UUID, Kind: model.KindHandover})
	f.must(err)
	if !created || ins.Status != model.StatusDraft || len(ins.Template.PhotoSlots) != 6 || ins.Rental.Amounts.DepositAmount != "5000.00" {
		t.Fatalf("started %+v", ins)
	}
	again, created, err := f.svc.Start(f.ctx, f.org, model.StartInput{RentalUUID: &r.UUID, Kind: model.KindHandover})
	f.must(err)
	if created || again.UUID != ins.UUID {
		t.Fatal("a second start must resume the draft")
	}
	ins = f.fill(f.ctx, ins, 1100, 80, nil)

	// Images only: the declared type and the bytes are both checked.
	if _, _, err := f.svc.AddPhoto(f.ctx, f.org, ins.UUID, model.PhotoInput{Slot: "front", ContentType: "application/pdf", SizeBytes: 10}); err == nil {
		t.Fatal("a PDF must be refused")
	}
	bad, _, err := f.svc.AddPhoto(f.ctx, f.org, ins.UUID, model.PhotoInput{Slot: "front", ContentType: "image/jpeg", SizeBytes: 9})
	f.must(err)
	if _, err := f.svc.UploadPhoto(f.ctx, f.org, ins.UUID, bad.Photo.UUID, usecase.FileUpload{Body: strings.NewReader("%PDF-1.4\n"), Size: 9}); err == nil {
		t.Fatal("non-image bytes must be refused")
	}
	if _, err := f.svc.DeletePhoto(f.ctx, f.org, ins.UUID, bad.Photo.UUID); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(bad.Upload.Path, "/v1/tenant/inspections/"+ins.UUID.String()+"/photos/") {
		t.Fatalf("upload address %q is not an API path", bad.Upload.Path)
	}

	// Five slots done; "dashboard" announced but never uploaded.
	var front model.Photo
	var frontBytes []byte
	for _, sl := range ins.Template.PhotoSlots[:5] {
		p, data := f.photo(f.ctx, ins.UUID, sl.Key, nil, true)
		if sl.Key == "front" {
			front, frontBytes = p, data
		}
	}
	dashBytes := jpeg("dashboard")
	dash, _, err := f.svc.AddPhoto(f.ctx, f.org, ins.UUID, model.PhotoInput{
		Slot: "dashboard", ContentType: "image/jpeg", SizeBytes: int64(len(dashBytes)), ClientRef: "dev-1",
	})
	f.must(err)
	// The device queue retrying the same photo gets the same row.
	retry, created, err := f.svc.AddPhoto(f.ctx, f.org, ins.UUID, model.PhotoInput{
		Slot: "dashboard", ContentType: "image/jpeg", SizeBytes: int64(len(dashBytes)), ClientRef: "dev-1",
	})
	f.must(err)
	if created || retry.Photo.UUID != dash.Photo.UUID {
		t.Fatal("a repeated client_ref must answer the same photo")
	}

	// Evidence: server hash of the stored bytes, server time, EXIF apart.
	sum := sha256.Sum256(frontBytes)
	if front.Status != model.PhotoCompleted || front.SHA256 != hex.EncodeToString(sum[:]) || front.UploadedAt == nil {
		t.Fatalf("completed photo %+v", front)
	}
	if front.EXIF.TakenAt == nil || !front.EXIF.TakenAt.Equal(time.Date(2026, 5, 4, 7, 2, 3, 0, time.UTC)) ||
		front.EXIF.GPSLat == nil || *front.EXIF.GPSLat != 41.0125 || front.EXIF.Make != "Rentopia" {
		t.Fatalf("exif %+v", front.EXIF)
	}
	if front.UploadedAt.Sub(f.now()).Abs() > time.Minute {
		t.Fatalf("uploaded_at %v is not the server time", front.UploadedAt)
	}
	var key, rentalPart string
	f.must(f.pool.QueryRow(context.Background(), `SELECT object_key FROM inspection_photos WHERE organization_id = $1 AND uuid = $2`, f.org, front.UUID).Scan(&key))
	rentalPart = "organizations/" + f.orgUUID.String() + "/rentals/" + r.UUID.String() + "/inspections/" + ins.UUID.String() + "/photos/"
	if !strings.HasPrefix(key, rentalPart) {
		t.Fatalf("object key %q", key)
	}

	// 1. Announced, not uploaded: refused.
	_, err = f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{})
	var se *usecase.StateError
	if !errors.As(err, &se) || se.Code != "inspection_incomplete" {
		t.Fatalf("completion with an un-uploaded photo = %v", err)
	}
	rd := se.Data.(model.Readiness)
	if rd.IncompletePhotos != 1 || !contains(rd.MissingSlots, "dashboard") || rd.PhotosReady {
		t.Fatalf("readiness %+v", rd)
	}
	got, err := f.svc.Get(f.ctx, f.org, ins.UUID)
	f.must(err)
	if got.Readiness.CanComplete || got.Status != model.StatusDraft {
		t.Fatalf("draft readiness %+v", got.Readiness)
	}
	// 2. Uploaded but not completed (not verified / hashed): refused.
	_, err = f.svc.UploadPhoto(f.ctx, f.org, ins.UUID, dash.Photo.UUID, usecase.FileUpload{Body: bytes.NewReader(dashBytes), Size: int64(len(dashBytes))})
	f.must(err)
	if _, err := f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{}); stateCode(err) != "inspection_incomplete" {
		t.Fatalf("completion with an uploaded, not completed photo = %v", err)
	}
	// 3. A damage needs its photo too.
	got, dmg, err := f.svc.AddDamage(f.ctx, f.org, ins.UUID, model.DamageInput{Zone: "left_front_door", DamageType: "scratch", Description: "çizik"})
	f.must(err)
	if got.Damages[0].IsNew {
		t.Fatal("a handover damage is a pre-existing one")
	}
	_, err = f.svc.CompletePhoto(f.ctx, f.org, ins.UUID, dash.Photo.UUID)
	f.must(err)
	_, err = f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{})
	if !errors.As(err, &se) || se.Code != "inspection_incomplete" || len(se.Data.(model.Readiness).DamagesWithoutPhoto) != 1 {
		t.Fatalf("completion with a damage without a photo = %v", err)
	}
	f.photo(f.ctx, ins.UUID, model.SlotDamage, &dmg, true)
	// 4. Photos ready; no signature and no waiver: refused.
	if _, err := f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{}); stateCode(err) != "signature_required" {
		t.Fatalf("completion without a signature = %v", err)
	}
	if r, _ := f.rentals.Get(f.ctx, f.org, r.UUID); r.Status != rentalmodel.StatusPreparation {
		t.Fatalf("rental moved while the form was refused: %s", r.Status)
	}

	// The tablet signature also approves the contract version shown.
	doc, err := f.contracts.GetDocument(f.ctx, f.org, *r.Contract.DocumentUUID)
	f.must(err)
	signed, err := f.svc.SaveSignature(f.ctx, f.org, ins.UUID, model.SignatureInput{
		SignerName: "Ayşe Yılmaz", PNGBase64: signaturePNG(), Strokes: strokes(),
		Contract: &model.ContractSignInput{DocumentUUID: doc.UUID, VersionUUID: doc.Current.UUID, ContentHash: doc.Current.ContentHash},
	})
	f.must(err)
	if signed.Signature == nil || signed.Signature.ContractApproval == nil || signed.Signature.PointCount != 12 || signed.SignatureStatus != model.SignatureSigned {
		t.Fatalf("signature %+v", signed.Signature)
	}
	doc, err = f.contracts.GetDocument(f.ctx, f.org, doc.UUID)
	f.must(err)
	if doc.Status != contractsmodel.DocumentApproved || len(doc.Approvals) != 1 || doc.Approvals[0].Method != contractsmodel.MethodSignature ||
		doc.Approvals[0].SignatureUUID == nil || *doc.Approvals[0].SignatureUUID != signed.Signature.UUID {
		t.Fatalf("contract approval %+v", doc.Approvals)
	}

	// Complete: the rental becomes active with the form's facts.
	done, err := f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{})
	f.must(err)
	if done.Status != model.StatusFinalized || done.FinalizedAt == nil || done.OccurredAt == nil {
		t.Fatalf("finalized %+v", done)
	}
	rental, err := f.rentals.Get(f.ctx, f.org, r.UUID)
	f.must(err)
	if rental.Status != rentalmodel.StatusActive || rental.ActualPickupAt == nil || len(rental.Legs) != 1 || *rental.Legs[0].OutKm != 1100 ||
		!rental.ActualPickupAt.Equal(*done.OccurredAt) {
		t.Fatalf("rental after the handover form %+v", rental)
	}
	var legLinked bool
	f.must(f.pool.QueryRow(context.Background(), `SELECT rental_vehicle_id IS NOT NULL FROM inspections WHERE organization_id = $1 AND uuid = $2`,
		f.org, ins.UUID).Scan(&legLinked))
	if !legLinked {
		t.Fatal("the finalized form names the leg it recorded")
	}

	// Immutable: API, direct SQL on the form and on its evidence.
	km := int32(1)
	if _, err := f.svc.UpdateDraft(f.ctx, f.org, ins.UUID, model.DraftInput{Km: &km}); stateCode(err) != "finalized" {
		t.Fatalf("patching a finalized form = %v", err)
	}
	if _, _, err := f.svc.AddPhoto(f.ctx, f.org, ins.UUID, model.PhotoInput{Slot: "front", ContentType: "image/jpeg", SizeBytes: 10}); stateCode(err) != "finalized" {
		t.Fatalf("adding a photo to a finalized form = %v", err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE inspections SET km = 1 WHERE organization_id = $1 AND uuid = $2`, f.org, ins.UUID); err == nil {
		t.Fatal("the database must refuse changing a finalized form")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE inspection_photos SET sha256 = repeat('0', 64) WHERE organization_id = $1 AND uuid = $2`, f.org, front.UUID); err == nil {
		t.Fatal("the database must refuse changing the evidence of a finalized form")
	}

	// Short-lived media links: the photo streams, a forged token does not.
	links, err := f.svc.MediaLinks(f.ctx, f.org, ins.UUID, model.MediaLinksInput{PhotoUUIDs: []uuid.UUID{front.UUID}})
	f.must(err)
	if len(links.Items) != 1 || !strings.HasPrefix(links.Items[0].URL, "/v1/public/media/inspections/") {
		t.Fatalf("media links %+v", links)
	}
	token := strings.TrimPrefix(links.Items[0].URL, "/v1/public/media/inspections/")
	file, err := f.svc.OpenMedia(context.Background(), token)
	f.must(err)
	streamed, _ := io.ReadAll(file.Body)
	_ = file.Body.Close()
	if !bytes.Equal(streamed, frontBytes) || file.ContentType != "image/jpeg" {
		t.Fatal("the media link must stream the stored photo")
	}
	if _, err := f.svc.OpenMedia(context.Background(), token[:len(token)-2]+"xx"); err == nil {
		t.Fatal("a forged media token must be refused")
	}

	// A correction is a new version with a reason.
	if _, err := f.svc.Revise(f.ctx, f.org, ins.UUID, ""); err == nil {
		t.Fatal("a revision needs a reason")
	}
	v2, err := f.svc.Revise(f.ctx, f.org, ins.UUID, "km yanlış girildi")
	f.must(err)
	if v2.Version != 2 || v2.Status != model.StatusDraft || v2.PreviousUUID == nil || *v2.PreviousUUID != ins.UUID ||
		len(v2.Photos) != 7 || v2.SignatureStatus != model.SignaturePending || len(v2.Versions) != 2 {
		t.Fatalf("revision %+v", v2)
	}
	for _, p := range v2.Photos {
		if p.Slot == "front" && p.SHA256 != front.SHA256 {
			t.Fatal("a revision keeps the evidence hashes")
		}
	}
	v1, err := f.svc.Get(f.ctx, f.org, ins.UUID)
	f.must(err)
	if v1.Status != model.StatusSuperseded || v1.Km == nil || *v1.Km != 1100 {
		t.Fatalf("v1 after the revision %+v", v1)
	}
}

// TestRT_A19_DisputedReturnNeverBecomesDebt: a return the customer does
// not sign and disputes is still recorded (dispute flag, signature status)
// and lands in the review queue; its charges (late fee from the policy
// snapshot, km overage, fuel, missing equipment, the damage) are proposed
// and disputed — none becomes final without the damage approver, the
// return check waits for the authorized review, and changing a computed
// amount needs tenant.price.override.
func TestRT_A19_DisputedReturnNeverBecomesDebt(t *testing.T) {
	f := newFx(t)
	// The jack has a price: missing at the return, it is charged.
	tplIn := model.DefaultTemplate()
	for i := range tplIn.Checklist {
		if tplIn.Checklist[i].Key == "jack" {
			tplIn.Checklist[i].Price = "750"
		}
	}
	tpl, err := f.svc.SaveTemplate(f.ctx, f.org, tplIn)
	f.must(err)
	if tpl.Version != 1 || tpl.Status != "active" {
		t.Fatalf("the first saved template is the active version 1: %+v", tpl)
	}
	r := f.rental()
	f.handOver(r)

	// 26 h 30 min after the handover: 1 h 30 min late (24 h planned + 1 h
	// early start), 2 started hours × 150 after the 30 min tolerance.
	f.advance(26*time.Hour + 30*time.Minute)
	ins, _, err := f.svc.Start(f.ctx, f.org, model.StartInput{RentalUUID: &r.UUID, Kind: model.KindReturn})
	f.must(err)
	if ins.Baseline == nil || ins.Baseline.Km == nil || *ins.Baseline.Km != 1100 || len(ins.Baseline.Photos) != 6 {
		t.Fatalf("return baseline %+v", ins.Baseline)
	}
	equipment := map[string]bool{}
	for _, it := range ins.Template.Checklist {
		equipment[it.Key] = it.Key != "jack"
	}
	// 1100 → 1350 km: 250 km driven, 100 km included for the one planned
	// day: 150 km over at 5 TRY. Fuel 80 → 60 at 10 TRY per percent.
	ins = f.fill(f.ctx, ins, 1350, 60, equipment)
	for _, sl := range ins.Template.PhotoSlots {
		f.photo(f.ctx, ins.UUID, sl.Key, nil, true)
	}
	_, dmg, err := f.svc.AddDamage(f.ctx, f.org, ins.UUID, model.DamageInput{
		Zone: "rear_bumper", DamageType: "dent", Severity: "moderate", Description: "arka tampon göçük", Disputed: true,
	})
	f.must(err)
	f.photo(f.ctx, ins.UUID, model.SlotDamage, &dmg, true)
	disputed, note := true, "Müşteri hasarın kendisine ait olmadığını belirtti"
	statement := "Tampon teslimde de böyleydi."
	ins, err = f.svc.UpdateDraft(f.ctx, f.org, ins.UUID, model.DraftInput{
		Disputed: &disputed, DisputeNote: &note, CustomerStatement: &statement,
		ChargeOverrides: &[]model.ChargeOverride{{Key: "damage:" + dmg.String(), Amount: money.MustParse("2000"), Reason: "tampon onarımı"}},
	})
	f.must(err)

	preview, err := f.svc.Charges(f.ctx, f.org, ins.UUID)
	f.must(err)
	lines := map[string]model.ChargeLine{}
	for _, l := range preview.Lines {
		lines[l.Key] = l
	}
	want := map[string]string{
		"late_return": "300.00", "mileage": "750.00", "fuel": "200.00", "equipment:jack": "750.00", "damage:" + dmg.String(): "2000.00",
	}
	for k, v := range want {
		if lines[k].Amount.String() != v || !lines[k].Disputed {
			t.Errorf("charge %s = %s disputed %v, want %s disputed (preview %+v)", k, lines[k].Amount, lines[k].Disputed, v, preview.Lines)
		}
	}
	// Changing a computed amount needs tenant.price.override.
	noOverride := f.as(without(rbac.PermTenantPriceOverride)...)
	mileage := []model.ChargeOverride{
		{Key: "damage:" + dmg.String(), Amount: money.MustParse("2000"), Reason: "tampon onarımı"},
		{Key: "mileage", Amount: money.MustParse("500"), Reason: "müşteri ile mutabakat"},
	}
	if _, err := f.svc.UpdateDraft(noOverride, f.org, ins.UUID, model.DraftInput{ChargeOverrides: &mileage}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("overriding without price.override = %v", err)
	}
	_, err = f.svc.UpdateDraft(f.ctx, f.org, ins.UUID, model.DraftInput{ChargeOverrides: &mileage})
	f.must(err)

	// No signature: the return is recorded with the refusal.
	if _, err := f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{}); stateCode(err) != "signature_required" {
		t.Fatalf("return without a signature or a reason = %v", err)
	}
	done, err := f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{NoSignatureReason: "Müşteri imzalamayı reddetti"})
	f.must(err)
	if done.Status != model.StatusFinalized || done.SignatureStatus != model.SignatureRefused || !done.Disputed || done.ReviewStatus != model.ReviewPending {
		t.Fatalf("finalized return %+v", done)
	}
	queue, total, err := f.svc.List(f.ctx, f.org, model.ListFilter{ReviewStatuses: []string{model.ReviewPending}, Limit: 10})
	f.must(err)
	if total != 1 || queue[0].UUID != ins.UUID {
		t.Fatalf("review queue %+v", queue)
	}
	rental, err := f.rentals.Get(f.ctx, f.org, r.UUID)
	f.must(err)
	if rental.Status != rentalmodel.StatusReturnCheck || rental.LateMinutes != 90 || len(rental.Charges) != 5 {
		t.Fatalf("rental after a disputed return: %s late %d charges %+v", rental.Status, rental.LateMinutes, rental.Charges)
	}
	for _, ch := range rental.Charges {
		if ch.Status != rentalmodel.ChargeProposed || !ch.Disputed || ch.InspectionUUID == nil || *ch.InspectionUUID != ins.UUID {
			t.Errorf("charge %s: status %s disputed %v inspection %v", ch.Kind, ch.Status, ch.Disputed, ch.InspectionUUID)
		}
		if ch.Kind == rentalmodel.ChargeMileage && (ch.Amount.String() != "500.00" || ch.OriginalAmount == nil || ch.OriginalAmount.String() != "750.00") {
			t.Errorf("overridden mileage charge %+v", ch)
		}
		if ch.Kind == rentalmodel.ChargeLateReturn && (ch.Source != rentalmodel.SourcePolicy || ch.Amount.String() != "300.00") {
			t.Errorf("late fee %+v", ch)
		}
	}
	tasks, _, err := f.rentals.ListTasks(f.ctx, f.org, rentalmodel.TaskFilter{RentalUUIDs: []uuid.UUID{r.UUID}, Limit: 20})
	f.must(err)
	types := map[string]string{}
	for _, tk := range tasks {
		types[tk.TaskType] = tk.Status + ":" + tk.Note
	}
	if !strings.HasPrefix(types[rentalmodel.TaskDeposit], rentalmodel.TaskPlanned+":hold") || types[rentalmodel.TaskCleaning] != "" {
		t.Fatalf("tasks after a disputed return: %v", types)
	}

	// The return check waits for the review.
	if _, err := f.rentals.CompleteReturn(f.ctx, f.org, r.UUID, rentalmodel.CompleteReturnInput{}); err == nil {
		t.Fatal("the return check must wait for the review")
	}
	// Disputed charges never become final without the damage approver.
	closeOnly := f.as(without(rbac.PermTenantDamageApprove)...)
	for _, ch := range rental.Charges {
		if _, err := f.rentals.DecideCharge(closeOnly, f.org, r.UUID, ch.UUID, true, "onaylandı"); !errors.Is(err, rentalusecase.ErrForbidden) {
			t.Errorf("approving disputed %s without damage.approve = %v", ch.Kind, err)
		}
	}
	// Accepting an unsigned return needs signature.waive.
	if _, err := f.svc.Review(f.as(without(rbac.PermTenantSignatureWaive)...), f.org, ins.UUID, model.ReviewInput{Note: "inceleme"}); !errors.Is(err, usecase.ErrForbidden) {
		t.Fatalf("review without signature.waive = %v", err)
	}
	reviewed, err := f.svc.Review(f.ctx, f.org, ins.UUID, model.ReviewInput{Note: "Kamera kayıtları incelendi", VehicleStatus: "maintenance"})
	f.must(err)
	if reviewed.ReviewStatus != model.ReviewResolved || reviewed.ReviewedAt == nil {
		t.Fatalf("reviewed %+v", reviewed)
	}
	rental, err = f.rentals.Get(f.ctx, f.org, r.UUID)
	f.must(err)
	if rental.Status != rentalmodel.StatusReturned {
		t.Fatalf("rental after the review %s", rental.Status)
	}
	for _, ch := range rental.Charges {
		if ch.Status != rentalmodel.ChargeProposed {
			t.Fatalf("the review decided charge %s", ch.Kind)
		}
	}
	var damageCharge rentalmodel.Charge
	for _, ch := range rental.Charges {
		if ch.Kind == rentalmodel.ChargeDamage {
			damageCharge = ch
		}
	}
	if _, err := f.rentals.DecideCharge(f.ctx, f.org, r.UUID, damageCharge.UUID, true, ""); err == nil {
		t.Fatal("approving a disputed charge needs a note")
	}
	approved, err := f.rentals.DecideCharge(f.ctx, f.org, r.UUID, damageCharge.UUID, true, "Kamera kaydı hasarı doğruluyor")
	f.must(err)
	for _, ch := range approved.Charges {
		if ch.UUID == damageCharge.UUID && ch.Status != rentalmodel.ChargeApproved {
			t.Fatalf("damage charge %+v", ch)
		}
	}
	tasks, _, err = f.rentals.ListTasks(f.ctx, f.org, rentalmodel.TaskFilter{RentalUUIDs: []uuid.UUID{r.UUID}, Limit: 20})
	f.must(err)
	maintenance := false
	for _, tk := range tasks {
		maintenance = maintenance || (tk.TaskType == rentalmodel.TaskMaintenance && tk.Status == rentalmodel.TaskPlanned)
	}
	if !maintenance {
		t.Fatalf("the reviewed return sends the vehicle to maintenance: %+v", tasks)
	}
}

// TestRT_A19_SignedReturnCompletes: a signed, undisputed return records
// the return, finishes the return check (the vehicle goes to cleaning
// through an operation task) and opens the deposit task; the return
// form's photos sit next to the handover's in the same slots.
func TestRT_A19_SignedReturnCompletes(t *testing.T) {
	f := newFx(t)
	r := f.rental()
	f.handOver(r)
	f.advance(20 * time.Hour)
	ins, _, err := f.svc.Start(f.ctx, f.org, model.StartInput{RentalUUID: &r.UUID, Kind: model.KindReturn})
	f.must(err)
	ins = f.fill(f.ctx, ins, 1150, 80, nil)
	for _, sl := range ins.Template.PhotoSlots {
		f.photo(f.ctx, ins.UUID, sl.Key, nil, true)
	}
	preview, err := f.svc.Charges(f.ctx, f.org, ins.UUID)
	f.must(err)
	if len(preview.Lines) != 0 || !preview.Total.IsZero() {
		t.Fatalf("an on-time, full return computes no charge: %+v", preview)
	}
	_, err = f.svc.SaveSignature(f.ctx, f.org, ins.UUID, model.SignatureInput{SignerName: "Ayşe Yılmaz", PNGBase64: signaturePNG(), Strokes: strokes()})
	f.must(err)
	done, err := f.svc.Complete(f.ctx, f.org, ins.UUID, model.CompleteInput{})
	f.must(err)
	if done.ReviewStatus != model.ReviewNone || done.Baseline == nil || len(done.Baseline.Photos) != len(done.Photos) {
		t.Fatalf("signed return %+v", done)
	}
	rental, err := f.rentals.Get(f.ctx, f.org, r.UUID)
	f.must(err)
	if rental.Status != rentalmodel.StatusReturned || rental.ActualReturnAt == nil || *rental.Legs[0].InKm != 1150 {
		t.Fatalf("rental after a signed return %+v", rental)
	}
	tasks, _, err := f.rentals.ListTasks(f.ctx, f.org, rentalmodel.TaskFilter{RentalUUIDs: []uuid.UUID{r.UUID}, Limit: 20})
	f.must(err)
	open := map[string]bool{}
	for _, tk := range tasks {
		if tk.Status == rentalmodel.TaskPlanned {
			open[tk.TaskType] = true
		}
	}
	if !open[rentalmodel.TaskCleaning] || !open[rentalmodel.TaskDeposit] {
		t.Fatalf("open tasks after a signed return: %v", open)
	}
	list, _, err := f.svc.List(f.ctx, f.org, model.ListFilter{RentalUUIDs: []uuid.UUID{r.UUID}, Limit: 10})
	f.must(err)
	if len(list) != 2 {
		t.Fatalf("forms of the rental: %+v", list)
	}
}
