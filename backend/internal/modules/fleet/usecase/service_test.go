package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- pure rules (always run) ---------------------------------------------------

func TestVersionChanged(t *testing.T) {
	t.Parallel()
	base := DefaultVersionFields()
	base.GuaranteeType = model.GuaranteeGroup
	base.RentalGroupID = pgtype.Int8{Int64: 7, Valid: true}
	base.GuaranteeLabel = "Ekonomi Manuel"

	same := base
	same.GuaranteeLabel = "Ekonomi Manuel (renamed)" // snapshot label only
	same.ChangeNote = "no-op"
	same.CreatedBy = pgtype.Int8{Int64: 3, Valid: true}
	same.DepositAmount = money("0.00") // same amount, other scale
	if VersionChanged(base, same) {
		t.Fatal("label / note / author / numeric scale must not create a version")
	}

	cases := map[string]func(v *repository.VersionFields){
		"guarantee type":   func(v *repository.VersionFields) { v.GuaranteeType = model.GuaranteeExactVariant },
		"group":            func(v *repository.VersionFields) { v.RentalGroupID = pgtype.Int8{Int64: 8, Valid: true} },
		"catalog model":    func(v *repository.VersionFields) { v.CatalogModelID = pgtype.Int8{Int64: 1, Valid: true} },
		"catalog type":     func(v *repository.VersionFields) { v.CatalogTypeID = pgtype.Int8{Int64: 1, Valid: true} },
		"variant level":    func(v *repository.VersionFields) { v.VariantLevel = pgtype.Text{String: "type", Valid: true} },
		"vehicle":          func(v *repository.VersionFields) { v.VehicleID = pgtype.UUID{Bytes: uuid.New(), Valid: true} },
		"deposit":          func(v *repository.VersionFields) { v.DepositAmount = money("1500") },
		"driver age":       func(v *repository.VersionFields) { v.MinDriverAge = 25 },
		"license years":    func(v *repository.VersionFields) { v.MinLicenseYears = 3 },
		"km per day":       func(v *repository.VersionFields) { v.IncludedKmPerDay = pgtype.Int4{Int32: 300, Valid: true} },
		"extra km fee":     func(v *repository.VersionFields) { v.ExtraKmFee = money("2.5") },
		"fuel policy":      func(v *repository.VersionFields) { v.FuelPolicy = "full_to_full" },
		"conditions notes": func(v *repository.VersionFields) { v.Notes = "Yurt dışı çıkış yasak" },
	}
	for name, mutate := range cases {
		next := base
		mutate(&next)
		if !VersionChanged(base, next) {
			t.Errorf("%s change must create a version", name)
		}
	}
}

func TestApplyConditionsValidation(t *testing.T) {
	t.Parallel()
	v := DefaultVersionFields()
	ve := &ValidationError{}
	bad := json.Number("12.345")
	age := int16(17)
	policy := "tank_full"
	applyConditions(ve, &v, model.ConditionsInput{DepositAmount: &bad, MinDriverAge: &age, FuelPolicy: &policy})
	fields := map[string]bool{}
	for _, d := range ve.Details {
		fields[d.Field] = true
	}
	for _, want := range []string{"conditions.deposit_amount", "conditions.min_driver_age", "conditions.fuel_policy"} {
		if !fields[want] {
			t.Errorf("missing validation detail %s (got %v)", want, ve.Details)
		}
	}
	ok := &ValidationError{}
	dep := json.Number("1500,50")
	applyConditions(ok, &v, model.ConditionsInput{DepositAmount: &dep})
	if len(ok.Details) != 0 || FormatMoney(v.DepositAmount) != "1500.50" {
		t.Fatalf("comma decimal: %v %s", ok.Details, FormatMoney(v.DepositAmount))
	}
}

// --- database-backed (TEST_DATABASE_URL) ------------------------------------------

// memDriver is the subset of storage.Driver the image use cases call.
type memDriver struct {
	storage.Driver
	m *storage.Memory
}

func (d memDriver) Upload(ctx context.Context, f storage.File, p string) error {
	return d.m.Upload(ctx, f, p)
}

func (d memDriver) Download(ctx context.Context, p string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, p)
}
func (d memDriver) Delete(ctx context.Context, p string) error { return d.m.Delete(ctx, p) }

type fixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	svc        *Service
	orgA, orgB int64
	actor      int64
	modelClio  db.CatalogModel
	modelEgea  db.CatalogModel
	typeEgea14 db.CatalogType
}

func newFixture(t *testing.T) *fixture {
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
	t.Cleanup(pool.Close)
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	orgA, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "fleet-a-" + suffix, Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "fleet-b-" + suffix, Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "fleet-" + suffix + "@example.test", PasswordHash: "x", Name: "Fleet", Surname: "Tester", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Platform catalog rows of the worked examples.
	var brandRenault, brandFiat int64
	if err := pool.QueryRow(ctx, `WITH d AS (INSERT INTO catalog_display_brands (name, slug) VALUES ($2, $3) RETURNING id)
		INSERT INTO catalog_brands (tsb_code, tsb_name, display_name, display_brand_id) SELECT $1, 'RENAULT', 'Renault', d.id FROM d RETURNING id`,
		"RN"+suffix, "Renault "+suffix, "renault-"+strings.ToLower(suffix)).Scan(&brandRenault); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `WITH d AS (INSERT INTO catalog_display_brands (name, slug) VALUES ($2, $3) RETURNING id)
		INSERT INTO catalog_brands (tsb_code, tsb_name, display_name, display_brand_id) SELECT $1, 'FIAT', 'Fiat', d.id FROM d RETURNING id`,
		"FT"+suffix, "Fiat "+suffix, "fiat-"+strings.ToLower(suffix)).Scan(&brandFiat); err != nil {
		t.Fatal(err)
	}
	clio, err := q.CreateCatalogModel(ctx, db.CreateCatalogModelParams{BrandID: brandRenault, Name: "Clio", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	egea, err := q.CreateCatalogModel(ctx, db.CreateCatalogModelParams{BrandID: brandFiat, Name: "Egea Cross", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	var egea14 db.CatalogType
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_types (brand_id, model_id, tsb_type_code, raw_name, first_seen_period, last_seen_period)
		VALUES ($1, $2, $3, 'Egea Cross 1.4 Fire', '2026-01', '2026-01') RETURNING id, uuid`, brandFiat, egea.ID, "T"+suffix).Scan(&egea14.ID, &egea14.Uuid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM catalog_brands WHERE id = ANY($1)", []int64{brandRenault, brandFiat})
		_, _ = pool.Exec(bg, "DELETE FROM catalog_display_brands WHERE name = ANY($1)", []string{"Renault " + suffix, "Fiat " + suffix})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", user.ID)
	})
	svc := New(repository.NewPostgres(pool, q), memDriver{m: storage.NewMemory()}, nil)
	return &fixture{
		ctx: ctx, pool: pool, svc: svc, orgA: orgA.ID, orgB: orgB.ID, actor: user.ID,
		modelClio: clio, modelEgea: egea, typeEgea14: egea14,
	}
}

func ptr[T any](v T) *T { return &v }

func (f *fixture) segment(t *testing.T, orgID int64, code string) model.Segment {
	t.Helper()
	s, err := f.svc.CreateSegment(f.ctx, orgID, model.SegmentInput{Code: ptr(code), Name: ptr("Segment " + code)})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSegmentsIsolationAndCodes(t *testing.T) {
	f := newFixture(t)
	eco := f.segment(t, f.orgA, "eko")
	if eco.Code != "EKO" {
		t.Fatalf("code not normalized: %q", eco.Code)
	}
	// Same code in another organization is fine; twice in one is not.
	f.segment(t, f.orgB, "EKO")
	if _, err := f.svc.CreateSegment(f.ctx, f.orgA, model.SegmentInput{Code: ptr("EKO"), Name: ptr("x")}); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code: %v", err)
	}
	// Another organization's uuid reads as not found.
	if _, err := f.svc.GetSegment(f.ctx, f.orgB, eco.UUID); !errors.Is(err, ErrSegmentNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
	if _, err := f.svc.PatchSegment(f.ctx, f.orgB, eco.UUID, model.SegmentInput{Name: ptr("hack")}); !errors.Is(err, ErrSegmentNotFound) {
		t.Fatalf("cross-org patch: %v", err)
	}
	if err := f.svc.DeleteSegment(f.ctx, f.orgB, eco.UUID); !errors.Is(err, ErrSegmentNotFound) {
		t.Fatalf("cross-org delete: %v", err)
	}
	items, total, err := f.svc.ListSegments(f.ctx, f.orgB, model.SegmentFilter{Limit: 50})
	if err != nil || total != 1 || len(items) != 1 || items[0].UUID == eco.UUID {
		t.Fatalf("org B list = %v %d %v", items, total, err)
	}
	// Code is reusable after a soft delete.
	if err := f.svc.DeleteSegment(f.ctx, f.orgA, eco.UUID); err != nil {
		t.Fatal(err)
	}
	f.segment(t, f.orgA, "EKO")
}

func TestRentalGroupsRules(t *testing.T) {
	f := newFixture(t)
	segA := f.segment(t, f.orgA, "EKO")
	segB := f.segment(t, f.orgB, "EKO")

	in := model.RentalGroupInput{
		Code: ptr("eko-m"), Name: ptr("Clio/Corsa/i20 veya benzeri"), SegmentUUID: &segA.UUID,
		Transmission:      model.OptionalString{Set: true, Value: ptr("manual")},
		Fuel:              model.OptionalString{Set: true, Value: ptr("petrol")},
		BodyType:          model.OptionalString{Set: true, Value: ptr("hatchback")},
		MinSeats:          model.OptionalInt{Set: true, Value: ptr(int64(5))},
		AirConditioning:   ptr(true),
		AcrissCode:        model.OptionalString{Set: true, Value: ptr("ecmr")},
		CatalogModelUUIDs: &[]uuid.UUID{f.modelClio.Uuid},
	}
	g, err := f.svc.CreateGroup(f.ctx, f.orgA, in)
	if err != nil {
		t.Fatal(err)
	}
	if g.Code != "EKO-M" || g.AcrissCode == nil || *g.AcrissCode != "ECMR" || len(g.CatalogModels) != 1 ||
		g.CatalogModels[0].Label != "Renault Clio" || g.Segment.UUID != segA.UUID {
		t.Fatalf("group = %+v", g)
	}
	criteria, err := f.svc.GroupCriteria(f.ctx, f.orgA, g.UUID)
	if err != nil || criteria.Transmission != "manual" || criteria.MinSeats != 5 || len(criteria.CatalogModelIDs) != 1 {
		t.Fatalf("criteria = %+v %v", criteria, err)
	}

	// ACRISS letters are checked against the matrix.
	bad := in
	bad.Code = ptr("BAD")
	bad.AcrissCode = model.OptionalString{Set: true, Value: ptr("EXTR")}
	var ve *ValidationError
	if _, err := f.svc.CreateGroup(f.ctx, f.orgA, bad); !errors.As(err, &ve) || ve.Details[0].Field != "acriss_code" {
		t.Fatalf("acriss validation: %v", err)
	}
	// A segment of another organization is not found (no cross-org FK).
	cross := in
	cross.Code = ptr("CROSS")
	cross.SegmentUUID = &segB.UUID
	if _, err := f.svc.CreateGroup(f.ctx, f.orgA, cross); !errors.As(err, &ve) || ve.Details[0].Field != "segment_uuid" {
		t.Fatalf("cross-org segment: %v", err)
	}
	// Code unique per organization.
	if _, err := f.svc.CreateGroup(f.ctx, f.orgA, in); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate group code: %v", err)
	}
	inB := in
	inB.SegmentUUID = &segB.UUID
	if _, err := f.svc.CreateGroup(f.ctx, f.orgB, inB); err != nil {
		t.Fatalf("same code in org B: %v", err)
	}
	// Isolation.
	if _, err := f.svc.GetGroup(f.ctx, f.orgB, g.UUID); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("cross-org group get: %v", err)
	}
	if _, err := f.svc.SetGroupActive(f.ctx, f.orgB, g.UUID, false); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("cross-org bulk: %v", err)
	}
	// Patch clears an attribute and replaces criteria.
	patched, err := f.svc.PatchGroup(f.ctx, f.orgA, g.UUID, model.RentalGroupInput{
		Fuel: model.OptionalString{Set: true}, CatalogModelUUIDs: &[]uuid.UUID{},
		CatalogTypeUUIDs: &[]uuid.UUID{f.typeEgea14.Uuid},
	})
	if err != nil || patched.Fuel != nil || len(patched.CatalogModels) != 0 || len(patched.CatalogTypes) != 1 ||
		patched.CatalogTypes[0].Label != "Fiat Egea Cross 1.4 Fire" {
		t.Fatalf("patched = %+v %v", patched, err)
	}
	// The segment is in use now.
	if err := f.svc.DeleteSegment(f.ctx, f.orgA, segA.UUID); !errors.Is(err, ErrSegmentInUse) {
		t.Fatalf("segment in use: %v", err)
	}
}

func TestProductVersioning(t *testing.T) {
	f := newFixture(t)
	seg := f.segment(t, f.orgA, "EKO")
	group, err := f.svc.CreateGroup(f.ctx, f.orgA, model.RentalGroupInput{
		Code: ptr("EKO-M"), Name: ptr("Ekonomi manuel"), SegmentUUID: &seg.UUID,
		Transmission: model.OptionalString{Set: true, Value: ptr("manual")},
	})
	if err != nil {
		t.Fatal(err)
	}
	deposit := json.Number("1500")
	p, err := f.svc.CreateProduct(f.ctx, f.orgA, f.actor, model.RentalProductInput{
		Code: ptr("clio-eko"), Title: ptr("Ekonomi Manuel"), SegmentUUID: &seg.UUID,
		RepresentativeModelUUID: model.OptionalUUID{Set: true, Value: &f.modelClio.Uuid},
		Guarantee:               &model.GuaranteeInput{Type: model.GuaranteeGroup, RentalGroupUUID: &group.UUID},
		Conditions:              &model.ConditionsInput{DepositAmount: &deposit},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.CurrentVersion != 1 || p.DisplayText != "Renault Clio veya benzeri" || p.GuaranteeBadge != model.BadgeOrSimilar ||
		p.Version.Conditions.DepositAmount != "1500.00" || p.Version.CreatedBy == nil || p.Status != model.StatusDraft {
		t.Fatalf("product = %+v", p)
	}
	v1 := p.Version

	// Title / description / status edits do not create a version.
	res, err := f.svc.PatchProduct(f.ctx, f.orgA, f.actor, p.UUID, model.RentalProductInput{
		Title: ptr("Ekonomi Manuel Plus"), Description: ptr("Şehir içi"), Status: ptr(model.StatusPublished),
	})
	if err != nil || res.VersionCreated || res.Product.CurrentVersion != 1 || res.Product.Title != "Ekonomi Manuel Plus" {
		t.Fatalf("title edit = %+v %v", res, err)
	}
	// Re-sending the same guarantee and conditions is not a change.
	res, err = f.svc.PatchProduct(f.ctx, f.orgA, f.actor, p.UUID, model.RentalProductInput{
		Guarantee:  &model.GuaranteeInput{Type: model.GuaranteeGroup, RentalGroupUUID: &group.UUID},
		Conditions: &model.ConditionsInput{DepositAmount: ptr(json.Number("1500.00"))},
	})
	if err != nil || res.VersionCreated || res.Product.CurrentVersion != 1 {
		t.Fatalf("no-op guarantee = %+v %v", res, err)
	}
	// A conditions edit creates version 2 (guarantee carried over).
	res, err = f.svc.PatchProduct(f.ctx, f.orgA, f.actor, p.UUID, model.RentalProductInput{
		Conditions: &model.ConditionsInput{MinDriverAge: ptr(int16(25))}, ChangeNote: ptr("Yaş sınırı"),
	})
	if err != nil || !res.VersionCreated || res.Product.CurrentVersion != 2 ||
		res.Product.Version.Guarantee.Type != model.GuaranteeGroup || res.Product.Version.Conditions.MinDriverAge != 25 ||
		res.Product.Version.Conditions.DepositAmount != "1500.00" || res.Product.Version.ChangeNote != "Yaş sınırı" {
		t.Fatalf("conditions edit = %+v %v", res, err)
	}
	// A guarantee edit creates version 3: "Fiat Egea Cross 1.4 Fire (model garantili)".
	res, err = f.svc.PatchProduct(f.ctx, f.orgA, f.actor, p.UUID, model.RentalProductInput{
		Guarantee: &model.GuaranteeInput{
			Type: model.GuaranteeExactVariant, VariantLevel: ptr(model.VariantLevelType), CatalogTypeUUID: &f.typeEgea14.Uuid,
		},
	})
	if err != nil || !res.VersionCreated || res.Product.CurrentVersion != 3 {
		t.Fatalf("guarantee edit = %+v %v", res, err)
	}
	cur := res.Product.Version
	if cur.Guarantee.CatalogType == nil || cur.Guarantee.CatalogModel == nil || cur.Guarantee.CatalogModel.UUID != f.modelEgea.Uuid ||
		res.Product.DisplayText != "Fiat Egea Cross 1.4 Fire (model garantili)" || res.Product.GuaranteeBadge != model.BadgeModelGuaranteed ||
		cur.Conditions.MinDriverAge != 25 {
		t.Fatalf("variant version = %+v (%s)", cur, res.Product.DisplayText)
	}
	// History: newest first, version 1 unchanged and not current.
	versions, total, err := f.svc.ListVersions(f.ctx, f.orgA, p.UUID, 20, 0)
	if err != nil || total != 3 || versions[0].Version != 3 || !versions[0].IsCurrent || versions[2].IsCurrent {
		t.Fatalf("versions = %+v %d %v", versions, total, err)
	}
	old, err := f.svc.GetVersion(f.ctx, f.orgA, p.UUID, v1.UUID)
	if err != nil || old.Version != 1 || old.Guarantee.Type != model.GuaranteeGroup || old.Conditions.MinDriverAge != 21 ||
		old.Guarantee.RentalGroup == nil || old.Guarantee.RentalGroup.UUID != group.UUID {
		t.Fatalf("v1 = %+v %v", old, err)
	}
	// Past versions are immutable in the database too.
	if _, err := f.pool.Exec(f.ctx, `UPDATE rental_product_versions SET min_driver_age = 30 WHERE uuid = $1`, v1.UUID); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("version update must fail: %v", err)
	}
	// The group was guaranteed by v1 only: it is free again.
	if err := f.svc.DeleteGroup(f.ctx, f.orgA, group.UUID); err != nil {
		t.Fatalf("delete group no longer guaranteed: %v", err)
	}

	// Validation: GROUP needs a group, EXACT_VARIANT a level.
	var ve *ValidationError
	_, err = f.svc.CreateProduct(f.ctx, f.orgA, f.actor, model.RentalProductInput{
		Code: ptr("X1"), Title: ptr("x"), SegmentUUID: &seg.UUID, Guarantee: &model.GuaranteeInput{Type: model.GuaranteeGroup},
	})
	if !errors.As(err, &ve) || ve.Details[0].Field != "guarantee.rental_group_uuid" {
		t.Fatalf("group guarantee validation: %v", err)
	}
	_, err = f.svc.CreateProduct(f.ctx, f.orgA, f.actor, model.RentalProductInput{
		Code: ptr("X2"), Title: ptr("x"), SegmentUUID: &seg.UUID, Guarantee: &model.GuaranteeInput{Type: model.GuaranteeExactVariant},
	})
	if !errors.As(err, &ve) || ve.Details[0].Field != "guarantee.variant_level" {
		t.Fatalf("variant guarantee validation: %v", err)
	}
	// EXACT_VEHICLE: the vehicle id is a placeholder until vehicles exist.
	car := uuid.New()
	ev, err := f.svc.CreateProduct(f.ctx, f.orgA, f.actor, model.RentalProductInput{
		Code: ptr("X3"), Title: ptr("34 ABC 123"), SegmentUUID: &seg.UUID,
		Guarantee: &model.GuaranteeInput{Type: model.GuaranteeExactVehicle, VehicleID: &car},
	})
	if err != nil || ev.Version.Guarantee.VehicleID == nil || *ev.Version.Guarantee.VehicleID != car ||
		ev.DisplayText != "34 ABC 123 (araç garantili)" {
		t.Fatalf("exact vehicle = %+v %v", ev, err)
	}
}

func TestProductsIsolationAndImages(t *testing.T) {
	f := newFixture(t)
	segA := f.segment(t, f.orgA, "SUV")
	segB := f.segment(t, f.orgB, "SUV")
	newProduct := func(orgID int64, seg uuid.UUID, code string) model.RentalProduct {
		t.Helper()
		p, err := f.svc.CreateProduct(f.ctx, orgID, f.actor, model.RentalProductInput{
			Code: ptr(code), Title: ptr("Egea Cross"), SegmentUUID: &seg,
			Guarantee: &model.GuaranteeInput{
				Type: model.GuaranteeExactVariant, VariantLevel: ptr(model.VariantLevelModel), CatalogModelUUID: &f.modelEgea.Uuid,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	pa := newProduct(f.orgA, segA.UUID, "EGEA")
	newProduct(f.orgB, segB.UUID, "EGEA") // same code, other organization
	if _, err := f.svc.CreateProduct(f.ctx, f.orgA, f.actor, model.RentalProductInput{
		Code: ptr("egea"), Title: ptr("dup"), SegmentUUID: &segA.UUID,
		Guarantee: &model.GuaranteeInput{Type: model.GuaranteeExactVehicle},
	}); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate product code: %v", err)
	}
	for name, err := range map[string]error{
		"get": func() error { _, err := f.svc.GetProduct(f.ctx, f.orgB, pa.UUID); return err }(),
		"patch": func() error {
			_, err := f.svc.PatchProduct(f.ctx, f.orgB, f.actor, pa.UUID, model.RentalProductInput{Title: ptr("x")})
			return err
		}(),
		"versions": func() error { _, _, err := f.svc.ListVersions(f.ctx, f.orgB, pa.UUID, 10, 0); return err }(),
		"bulk":     func() error { _, err := f.svc.SetProductActive(f.ctx, f.orgB, pa.UUID, false); return err }(),
		"delete":   f.svc.DeleteProduct(f.ctx, f.orgB, pa.UUID),
	} {
		if !errors.Is(err, ErrProductNotFound) {
			t.Errorf("cross-org %s: %v", name, err)
		}
	}
	if p, err := f.svc.GetProduct(f.ctx, f.orgA, pa.UUID); err != nil || p.DisplayText != "Fiat Egea Cross (model garantili)" {
		t.Fatalf("product A = %+v %v", p, err)
	}

	// Images: sniffed type, API-owned URL, order, public stream, delete.
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	upload := func(name string) model.ProductImage {
		t.Helper()
		img, err := f.svc.UploadImage(f.ctx, f.orgA, pa.UUID, ImageUpload{Body: bytes.NewReader(png), Size: int64(len(png)), Filename: name})
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	first, second := upload("front.png"), upload("side.png")
	if !strings.HasPrefix(first.PublicURL, "/v1/public/media/rental-products/") || first.ContentType != "image/png" || second.SortOrder != 1 {
		t.Fatalf("images = %+v %+v", first, second)
	}
	html := []byte("<html><script>alert(1)</script></html>")
	var ve *ValidationError
	if _, err := f.svc.UploadImage(f.ctx, f.orgA, pa.UUID, ImageUpload{Body: bytes.NewReader(html), Size: int64(len(html))}); !errors.As(err, &ve) {
		t.Fatalf("html upload must be rejected: %v", err)
	}
	if _, err := f.svc.UploadImage(f.ctx, f.orgB, pa.UUID, ImageUpload{Body: bytes.NewReader(png), Size: int64(len(png))}); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("cross-org upload: %v", err)
	}
	ordered, err := f.svc.ReorderImages(f.ctx, f.orgA, pa.UUID, []uuid.UUID{second.UUID, first.UUID})
	if err != nil || ordered[0].UUID != second.UUID {
		t.Fatalf("reorder = %+v %v", ordered, err)
	}
	if _, err := f.svc.ReorderImages(f.ctx, f.orgA, pa.UUID, []uuid.UUID{second.UUID}); !errors.As(err, &ve) {
		t.Fatalf("partial reorder must fail: %v", err)
	}
	var orgUUID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT uuid FROM organizations WHERE id = $1`, f.orgA).Scan(&orgUUID); err != nil {
		t.Fatal(err)
	}
	rc, ct, _, err := f.svc.PublicImage(f.ctx, orgUUID, first.UUID)
	if err != nil || ct != "image/png" {
		t.Fatalf("public image: %v %s", err, ct)
	}
	_ = rc.Close()
	var orgBUUID uuid.UUID
	_ = f.pool.QueryRow(f.ctx, `SELECT uuid FROM organizations WHERE id = $1`, f.orgB).Scan(&orgBUUID)
	if _, _, _, err := f.svc.PublicImage(f.ctx, orgBUUID, first.UUID); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("public image via other org: %v", err)
	}
	if err := f.svc.DeleteImage(f.ctx, f.orgA, pa.UUID, first.UUID); err != nil {
		t.Fatal(err)
	}
	if p, _ := f.svc.GetProduct(f.ctx, f.orgA, pa.UUID); len(p.Images) != 1 {
		t.Fatalf("images after delete = %d", len(p.Images))
	}

	// List filters stay inside the organization.
	items, total, err := f.svc.ListProducts(f.ctx, f.orgA, model.RentalProductFilter{
		Limit: 20, GuaranteeTypes: []string{model.GuaranteeExactVariant}, SegmentUUIDs: []uuid.UUID{segA.UUID},
	})
	if err != nil || total != 1 || items[0].UUID != pa.UUID {
		t.Fatalf("filtered list = %v %d %v", items, total, err)
	}
	facets, err := f.svc.ProductFacets(f.ctx, f.orgA)
	if err != nil || len(facets.GuaranteeType) != 1 || facets.GuaranteeType[0].Count != 1 {
		t.Fatalf("facets = %+v %v", facets, err)
	}
}
