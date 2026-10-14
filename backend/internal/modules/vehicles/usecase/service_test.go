package usecase

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/eligibility"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- pure rules (always run) ---------------------------------------------------

func TestNormalizePlate(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		" 34 abc  123 ": {"34 ABC 123", "34ABC123"},
		"06-ab-1234":    {"06-AB-1234", "06AB1234"},
	}
	for in, want := range cases {
		if got := NormalizePlate(in); got != want[0] {
			t.Errorf("NormalizePlate(%q) = %q", in, got)
		}
		if got := plateKey(NormalizePlate(in)); got != want[1] {
			t.Errorf("plateKey(%q) = %q", in, got)
		}
	}
}

func TestImportValueMapping(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"Otomatik": "automatic", "MANUEL": "manual", "Düz": "manual", "Yarı Otomatik": "automatic"} {
		if got, ok := TransmissionCode(in); !ok || got != want {
			t.Errorf("TransmissionCode(%q) = %q %v", in, got, ok)
		}
	}
	for in, want := range map[string]string{"Dizel": "diesel", "BENZİN": "petrol", "Benzin/LPG": "lpg", "Hibrit": "hybrid", "Elektrik": "electric"} {
		if got, ok := FuelCode(in); !ok || got != want {
			t.Errorf("FuelCode(%q) = %q %v", in, got, ok)
		}
	}
	for in, want := range map[string]string{"Özmal": "owned", "Tedarikçi": "supplied", "supplied": "supplied"} {
		if got, ok := OwnershipCode(in); !ok || got != want {
			t.Errorf("OwnershipCode(%q) = %q %v", in, got, ok)
		}
	}
	if _, ok := FuelCode("hydrogen"); ok {
		t.Error("unknown fuel must not map")
	}
}

// --- database-backed (TEST_DATABASE_URL) ------------------------------------------

type fixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	q          *db.Queries
	svc        *Service
	orgA, orgB int64
	// Branches of org A (A1 "Merkez", A2 "Şube 2") and org B.
	branchA1, branchA2, branchB db.Branch
	// Catalog: one display brand with two TSB brands that both list "Egea"
	// (grouped model); typeEgea is a type of the first, typeSibling of the
	// second.
	displayBrand          string
	modelEgea, modelTwin  int64
	typeEgea, typeSibling db.GetVehicleCatalogTypeRow
	typeCode              string
	segment               int64
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
	f := &fixture{ctx: ctx, pool: pool, q: q}
	orgA, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "veh-a-" + suffix, Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "veh-b-" + suffix, Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	f.orgA, f.orgB = orgA.ID, orgB.ID
	branch := func(orgID int64, code, name string) db.Branch {
		b, err := q.CreateBranch(ctx, db.CreateBranchParams{
			OrganizationID: orgID, Code: code, Name: name, WorkingHours: []byte("{}"),
			HolidayExceptions: []byte("[]"), PrepMinutes: 60, IsActive: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	f.branchA1, f.branchA2, f.branchB = branch(f.orgA, "MRK", "Merkez"), branch(f.orgA, "IST", "İstanbul"), branch(f.orgB, "MRK", "Merkez")

	f.displayBrand = "Fiat " + suffix
	var display int64
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_display_brands (name, slug) VALUES ($1, $2) RETURNING id`,
		f.displayBrand, "fiat-"+strings.ToLower(suffix)).Scan(&display); err != nil {
		t.Fatal(err)
	}
	var brand1, brand2 int64
	for i, dst := range []*int64{&brand1, &brand2} {
		if err := pool.QueryRow(ctx, `INSERT INTO catalog_brands (tsb_code, tsb_name, display_name, display_brand_id)
			VALUES ($1, $2, $3, $4) RETURNING id`, []string{"F1", "F2"}[i]+suffix, "FIAT", f.displayBrand, display).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_models (brand_id, name, source) VALUES ($1, 'Egea', 'manual') RETURNING id`, brand1).Scan(&f.modelEgea); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_models (brand_id, name, source) VALUES ($1, 'Egea', 'auto') RETURNING id`, brand2).Scan(&f.modelTwin); err != nil {
		t.Fatal(err)
	}
	f.typeCode = "K" + suffix
	var typeID, siblingID int64
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_types (brand_id, model_id, tsb_type_code, raw_name, years, suggested_transmission, suggested_fuel, first_seen_period, last_seen_period)
		VALUES ($1, $2, $3, 'EGEA 1.4 FIRE EASY', '{2023,2024}', 'manual', 'petrol', '2026-01', '2026-09') RETURNING id`, brand1, f.modelEgea, f.typeCode).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_types (brand_id, model_id, tsb_type_code, raw_name, years, first_seen_period, last_seen_period)
		VALUES ($1, $2, $3, 'EGEA 1.6 MJET URBAN', '{2024}', '2026-01', '2026-09') RETURNING id`, brand2, f.modelTwin, "S"+suffix).Scan(&siblingID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO catalog_valuations (type_id, model_year, period, value) VALUES
		($1, 2024, '2026-08', 1000000), ($1, 2024, '2026-09', 1100000), ($1, 2023, '2026-09', 900000)`, typeID); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewPostgres(pool, q)
	for _, id := range []int64{typeID, siblingID} {
		var u uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT uuid FROM catalog_types WHERE id = $1`, id).Scan(&u); err != nil {
			t.Fatal(err)
		}
		row, err := repo.GetCatalogType(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		if id == typeID {
			f.typeEgea = row
		} else {
			f.typeSibling = row
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, f.orgA).Scan(&f.segment); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{f.orgA, f.orgB})
		_, _ = pool.Exec(bg, "DELETE FROM catalog_brands WHERE id = ANY($1)", []int64{brand1, brand2})
		_, _ = pool.Exec(bg, "DELETE FROM catalog_display_brands WHERE id = $1", display)
	})
	f.svc = New(repo, nil)
	return f
}

func ptr[T any](v T) *T { return &v }

// input is a valid catalog vehicle of org A at branch A1.
func (f *fixture) input(plate string) model.VehicleInput {
	return model.VehicleInput{
		Plate: ptr(plate), CatalogTypeUUID: model.OptionalUUID{Set: true, Value: &f.typeEgea.Uuid},
		ModelYear: ptr(int16(2024)), HomeBranchUUID: &f.branchA1.Uuid,
	}
}

func (f *fixture) create(t *testing.T, orgID int64, in model.VehicleInput) model.Vehicle {
	t.Helper()
	v, err := f.svc.Create(f.ctx, orgID, in)
	if err != nil {
		t.Fatalf("create %v: %v", *in.Plate, err)
	}
	return v
}

func (f *fixture) group(t *testing.T, code, sql string, args ...any) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	all := append([]any{f.orgA, f.segment, code}, args...)
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_groups (organization_id, segment_id, code, name`+sql, all...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func validationFields(err error) []string {
	var ve *ValidationError
	if !errors.As(err, &ve) {
		return nil
	}
	out := []string{}
	for _, d := range ve.Details {
		out = append(out, d.Field+":"+d.Code)
	}
	return out
}

func TestVehicleCatalogKaskoAndIsolation(t *testing.T) {
	f := newFixture(t)
	v := f.create(t, f.orgA, f.input("34 abc 123"))
	if v.Plate != "34 ABC 123" || v.Catalog == nil || v.Brand != f.displayBrand || v.Model != "Egea" {
		t.Fatalf("vehicle = %+v", v)
	}
	// Catalog suggestions fill omitted attributes; current branch = home.
	if v.Transmission == nil || *v.Transmission != "manual" || v.Fuel == nil || *v.Fuel != "petrol" ||
		v.CurrentBranch.UUID != f.branchA1.Uuid || v.Status != model.StatusAvailable || !v.InService {
		t.Fatalf("defaults = %+v", v)
	}
	// Kasko: latest period of the type + model year.
	if v.KaskoValue == nil || *v.KaskoValue != "1100000.00" || *v.KaskoPeriod != "2026-09" {
		t.Fatalf("kasko = %v %v", v.KaskoValue, v.KaskoPeriod)
	}
	kasko, err := f.svc.Kasko(f.ctx, f.orgA, v.UUID)
	if err != nil || len(kasko.Items) != 2 || kasko.Items[0].Period != "2026-09" || kasko.Items[1].Value != "1000000.00" {
		t.Fatalf("kasko history = %+v %v", kasko, err)
	}
	// Model year must be a valid year of the catalog type.
	in := f.input("34 ABC 124")
	in.ModelYear = ptr(int16(2020))
	if _, err := f.svc.Create(f.ctx, f.orgA, in); !slices.Contains(validationFields(err), "model_year:invalid_year") {
		t.Fatalf("invalid year: %v", err)
	}
	if _, err := f.svc.Patch(f.ctx, f.orgA, v.UUID, model.VehicleInput{ModelYear: ptr(int16(2019))}); !slices.Contains(validationFields(err), "model_year:invalid_year") {
		t.Fatalf("invalid year on patch: %v", err)
	}
	// Patching the year keeps the kasko lookup in sync.
	v2, err := f.svc.Patch(f.ctx, f.orgA, v.UUID, model.VehicleInput{ModelYear: ptr(int16(2023))})
	if err != nil || v2.KaskoValue == nil || *v2.KaskoValue != "900000.00" {
		t.Fatalf("patch year: %+v %v", v2.KaskoValue, err)
	}
	// Catalog or manual entry is required.
	missing := model.VehicleInput{Plate: ptr("34 ABC 125"), ModelYear: ptr(int16(2024)), HomeBranchUUID: &f.branchA1.Uuid}
	if _, err := f.svc.Create(f.ctx, f.orgA, missing); !slices.Contains(validationFields(err), "catalog_type_uuid:required") {
		t.Fatalf("missing catalog: %v", err)
	}
	manual := missing
	manual.ManualEntry, manual.ManualBrand, manual.ManualModel = ptr(true), ptr("Togg"), ptr("T10X")
	mv := f.create(t, f.orgA, manual)
	if !mv.ManualEntry || mv.Catalog != nil || mv.Label != "Togg T10X" || mv.KaskoValue != nil {
		t.Fatalf("manual = %+v", mv)
	}
	// Another organization's branch is unknown here.
	other := f.input("34 ABC 126")
	other.HomeBranchUUID = &f.branchB.Uuid
	if _, err := f.svc.Create(f.ctx, f.orgA, other); !slices.Contains(validationFields(err), "home_branch_uuid:not_found") {
		t.Fatalf("foreign branch: %v", err)
	}
	// Isolation: org B sees nothing of org A.
	if _, err := f.svc.Get(f.ctx, f.orgB, v.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org get: %v", err)
	}
	if _, err := f.svc.Patch(f.ctx, f.orgB, v.UUID, model.VehicleInput{Color: ptr("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org patch: %v", err)
	}
	if _, err := f.svc.ChangeStatus(f.ctx, f.orgB, v.UUID, model.StatusInput{Status: model.StatusDamaged}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org status: %v", err)
	}
	if err := f.svc.Delete(f.ctx, f.orgB, v.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org delete: %v", err)
	}
	if items, total, err := f.svc.List(f.ctx, f.orgB, model.ListFilter{Limit: 50}); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("org B list = %d %v", total, err)
	}
	items, total, err := f.svc.List(f.ctx, f.orgA, model.ListFilter{Limit: 50, Q: "34abc", SortField: "plate"})
	if err != nil || total != 2 || items[0].Plate != "34 ABC 123" {
		t.Fatalf("plate search = %d %v", total, err)
	}
	if _, total, _ := f.svc.List(f.ctx, f.orgA, model.ListFilter{Limit: 50, Q: "egea"}); total != 1 {
		t.Fatalf("model search = %d", total)
	}
	facets, err := f.svc.Facets(f.ctx, f.orgA)
	if err != nil || len(facets.Status) != 1 || facets.Status[0].Count != 2 || len(facets.HomeBranch) != 1 || facets.HomeBranch[0].Label != "Merkez" {
		t.Fatalf("facets = %+v %v", facets, err)
	}
}

func TestVehiclePlateUniqueness(t *testing.T) {
	f := newFixture(t)
	first := f.create(t, f.orgA, f.input("34 ABC 123"))
	// Same plate key (spaces / dashes / case ignored) in the organization.
	if _, err := f.svc.Create(f.ctx, f.orgA, f.input("34-abc-123")); !errors.Is(err, ErrPlateTaken) {
		t.Fatalf("duplicate plate: %v", err)
	}
	// Another organization may use it.
	other := f.input("34 ABC 123")
	other.HomeBranchUUID = &f.branchB.Uuid
	if _, err := f.svc.Create(f.ctx, f.orgB, other); err != nil {
		t.Fatalf("other org: %v", err)
	}
	// Only active vehicles hold a plate: deactivate the first, reuse it.
	if _, err := f.svc.SetActive(f.ctx, f.orgA, first.UUID, false); err != nil {
		t.Fatal(err)
	}
	second := f.create(t, f.orgA, f.input("34ABC123"))
	if _, err := f.svc.SetActive(f.ctx, f.orgA, first.UUID, true); !errors.Is(err, ErrPlateTaken) {
		t.Fatalf("reactivate: %v", err)
	}
	if _, err := f.svc.Patch(f.ctx, f.orgA, first.UUID, model.VehicleInput{IsActive: ptr(true)}); !errors.Is(err, ErrPlateTaken) {
		t.Fatalf("reactivate by patch: %v", err)
	}
	// Renaming to a taken plate fails; a soft delete frees the plate.
	third := f.create(t, f.orgA, f.input("06 XY 99"))
	if _, err := f.svc.Patch(f.ctx, f.orgA, third.UUID, model.VehicleInput{Plate: ptr("34 abc 123")}); !errors.Is(err, ErrPlateTaken) {
		t.Fatalf("rename: %v", err)
	}
	if err := f.svc.Delete(f.ctx, f.orgA, second.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Patch(f.ctx, f.orgA, third.UUID, model.VehicleInput{Plate: ptr("34 abc 123")}); err != nil {
		t.Fatalf("rename after delete: %v", err)
	}
}

func TestVehicleStatusEvents(t *testing.T) {
	f := newFixture(t)
	v := f.create(t, f.orgA, f.input("34 ABC 123"))
	events, total, err := f.svc.Events(f.ctx, f.orgA, v.UUID, 50, 0)
	if err != nil || total != 1 || events[0].EventType != "created" || events[0].FromStatus != nil || events[0].ToStatus != model.StatusAvailable {
		t.Fatalf("initial events = %+v %v", events, err)
	}
	res, err := f.svc.ChangeStatus(f.ctx, f.orgA, v.UUID, model.StatusInput{
		Status: model.StatusMaintenance, OdometerKm: ptr(int32(12000)), FuelLevel: model.Int(50), Note: "Periyodik bakım",
	})
	if err != nil || !res.Changed || res.FromStatus != model.StatusAvailable || res.Vehicle.Status != model.StatusMaintenance ||
		res.Vehicle.OdometerKm != 12000 || *res.Vehicle.FuelLevel != 50 {
		t.Fatalf("change = %+v %v", res, err)
	}
	// Same status: no-op (no event), a new km reading is still saved.
	res, err = f.svc.ChangeStatus(f.ctx, f.orgA, v.UUID, model.StatusInput{Status: model.StatusMaintenance, OdometerKm: ptr(int32(12050))})
	if err != nil || res.Changed || res.Vehicle.OdometerKm != 12050 {
		t.Fatalf("same status = %+v %v", res, err)
	}
	// Permissive transitions: any → any, recorded.
	if _, err := f.svc.ChangeStatus(f.ctx, f.orgA, v.UUID, model.StatusInput{Status: model.StatusRented}); err != nil {
		t.Fatal(err)
	}
	// A status in a patch is a recorded transition too.
	if _, err := f.svc.Patch(f.ctx, f.orgA, v.UUID, model.VehicleInput{Status: ptr(model.StatusCleaning)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ChangeStatus(f.ctx, f.orgA, v.UUID, model.StatusInput{Status: "flying"}); !slices.Contains(validationFields(err), "status:invalid") {
		t.Fatalf("invalid status: %v", err)
	}
	events, total, err = f.svc.Events(f.ctx, f.orgA, v.UUID, 50, 0)
	if err != nil || total != 4 {
		t.Fatalf("events total = %d %v", total, err)
	}
	newest, maint := events[0], events[2]
	if newest.ToStatus != model.StatusCleaning || *newest.FromStatus != model.StatusRented ||
		maint.Note != "Periyodik bakım" || *maint.OdometerKm != 12000 || *maint.FromStatus != model.StatusAvailable {
		t.Fatalf("events = %+v", events)
	}
	// Bulk form: returns the previous status for rollback.
	prev, err := f.svc.SetStatus(f.ctx, f.orgA, v.UUID, model.StatusAvailable, "")
	if err != nil || prev != model.StatusCleaning {
		t.Fatalf("set status = %q %v", prev, err)
	}
}

func TestVehicleGroupEligibility(t *testing.T) {
	f := newFixture(t)
	v := f.create(t, f.orgA, f.input("34 ABC 123")) // manual petrol Egea
	auto := f.group(t, "AUTO", `, transmission, min_seats) VALUES ($1, $2, $3, 'Otomatik', 'automatic', 5) RETURNING uuid`)
	egea := f.group(t, "EGEA", `) VALUES ($1, $2, $3, 'Egea veya benzeri') RETURNING uuid`)
	open := f.group(t, "OPEN", `) VALUES ($1, $2, $3, 'Her araç') RETURNING uuid`)
	// The group names the other TSB brand's "Egea": grouped models match.
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO rental_group_catalog_models (organization_id, rental_group_id, catalog_model_id)
		SELECT organization_id, id, $2 FROM rental_groups WHERE uuid = $1`, egea, f.modelTwin); err != nil {
		t.Fatal(err)
	}

	_, err := f.svc.AddGroup(f.ctx, f.orgA, v.UUID, auto)
	ne, ok := IsNotEligible(err)
	if !ok || !slices.Contains(ne.Reasons, eligibility.ReasonTransmission) || !slices.Contains(ne.Reasons, eligibility.ReasonSeats) {
		t.Fatalf("not eligible = %v", err)
	}
	if _, err := f.svc.AddGroup(f.ctx, f.orgA, v.UUID, egea); err != nil {
		t.Fatalf("grouped model: %v", err)
	}
	// Another organization's group reads as not found.
	var foreign uuid.UUID
	var segB int64
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'E') RETURNING id`, f.orgB).Scan(&segB); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_groups (organization_id, segment_id, code, name) VALUES ($1, $2, 'X', 'X') RETURNING uuid`, f.orgB, segB).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddGroup(f.ctx, f.orgA, v.UUID, foreign); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("foreign group: %v", err)
	}

	view, err := f.svc.Groups(f.ctx, f.orgA, v.UUID)
	if err != nil || len(view.Items) != 1 || !view.Items[0].Eligible || len(view.Suggestions) != 1 || view.Suggestions[0].Group.UUID != open {
		t.Fatalf("groups = %+v %v", view, err)
	}
	// Changed attributes keep the membership but flag it.
	if _, err := f.svc.Patch(f.ctx, f.orgA, v.UUID, model.VehicleInput{
		ManualEntry: ptr(true), ManualBrand: ptr("Fiat"), ManualModel: ptr("Doblo"),
	}); err != nil {
		t.Fatal(err)
	}
	view, _ = f.svc.Groups(f.ctx, f.orgA, v.UUID)
	if len(view.Items) != 1 || view.Items[0].Eligible || view.Items[0].Reasons[0] != eligibility.ReasonCatalogNotAllowed {
		t.Fatalf("flagged membership = %+v", view.Items)
	}
	if err := f.svc.RemoveGroup(f.ctx, f.orgA, v.UUID, egea); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RemoveGroup(f.ctx, f.orgA, v.UUID, egea); !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	// Bulk replace returns the previous set and checks eligibility.
	if _, err := f.svc.SetGroups(f.ctx, f.orgA, v.UUID, []uuid.UUID{open}, true, true); err != nil {
		t.Fatal(err)
	}
	if prev, err := f.svc.SetGroups(f.ctx, f.orgA, v.UUID, []uuid.UUID{auto}, true, true); err == nil || len(prev) != 1 || prev[0] != open {
		t.Fatalf("replace with ineligible = %v %v", prev, err)
	}
	// Group filter of the list.
	if _, total, _ := f.svc.List(f.ctx, f.orgA, model.ListFilter{Limit: 10, GroupUUIDs: []uuid.UUID{open}}); total != 1 {
		t.Fatalf("group filter = %d", total)
	}
}

func TestProductEligibleVehicles(t *testing.T) {
	f := newFixture(t)
	manual := f.create(t, f.orgA, f.input("34 ABC 123"))
	autoIn := f.input("34 ABC 124")
	autoIn.Transmission = model.String("automatic")
	automatic := f.create(t, f.orgA, autoIn)
	sibling := f.input("34 ABC 125")
	sibling.CatalogTypeUUID = model.OptionalUUID{Set: true, Value: &f.typeSibling.Uuid}
	twin := f.create(t, f.orgA, sibling)
	inactiveIn := f.input("34 ABC 126")
	inactiveIn.IsActive = ptr(false)
	inactiveIn.Transmission = model.String("automatic")
	f.create(t, f.orgA, inactiveIn)

	auto := f.group(t, "AUTO", `, transmission) VALUES ($1, $2, $3, 'Otomatik', 'automatic') RETURNING uuid`)
	product := func(code, guarantee string, extra string, args ...any) uuid.UUID {
		t.Helper()
		var id, productID int64
		var pu uuid.UUID
		if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id) VALUES ($1, $2, $2, $3) RETURNING id, uuid`,
			f.orgA, code, f.segment).Scan(&productID, &pu); err != nil {
			t.Fatal(err)
		}
		all := append([]any{f.orgA, productID, guarantee}, args...)
		if err := f.pool.QueryRow(f.ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type`+extra, all...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return pu
	}
	group := product("P-GRP", "GROUP", `, rental_group_id) SELECT $1, $2, 1, $3, id FROM rental_groups WHERE uuid = $4 RETURNING id`, auto)
	byModel := product("P-MDL", "EXACT_VARIANT", `, variant_level, catalog_model_id) VALUES ($1, $2, 1, $3, 'model', $4) RETURNING id`, f.modelTwin)
	typed := product("P-TYP", "EXACT_VARIANT", `, variant_level, catalog_type_id) VALUES ($1, $2, 1, $3, 'type', $4) RETURNING id`, f.typeSibling.ID)
	exact := product("P-VEH", "EXACT_VEHICLE", `, vehicle_id) VALUES ($1, $2, 1, $3, $4) RETURNING id`, manual.UUID)

	uuids := func(p uuid.UUID) []uuid.UUID {
		t.Helper()
		out, err := f.svc.ProductVehicles(f.ctx, f.orgA, p, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids := []uuid.UUID{}
		for _, v := range out.Items {
			ids = append(ids, v.UUID)
		}
		if int(out.Total) != len(ids) {
			t.Fatalf("total %d != %d", out.Total, len(ids))
		}
		return ids
	}
	if got := uuids(group); !slices.Equal(got, []uuid.UUID{automatic.UUID}) {
		t.Errorf("group guarantee = %v", got)
	}
	// The grouped model accepts both TSB brands' Egea.
	if got := uuids(byModel); len(got) != 3 {
		t.Errorf("model guarantee = %v", got)
	}
	if got := uuids(typed); !slices.Equal(got, []uuid.UUID{twin.UUID}) {
		t.Errorf("type guarantee = %v", got)
	}
	if got := uuids(exact); !slices.Equal(got, []uuid.UUID{manual.UUID}) {
		t.Errorf("vehicle guarantee = %v", got)
	}
	if _, err := f.svc.ProductVehicles(f.ctx, f.orgB, group, 50, 0); !errors.Is(err, ErrProductNotFound) {
		t.Errorf("cross-org product: %v", err)
	}
	// Branch scope applies: a member of branch A2 sees none of them.
	scoped := orgctx.WithScope(f.ctx, orgctx.Scope{ID: f.orgA, Slug: "a", BranchIDs: []int64{f.branchA2.ID}})
	if out, err := f.svc.ProductVehicles(scoped, f.orgA, byModel, 50, 0); err != nil || out.Total != 0 {
		t.Errorf("scoped = %d %v", out.Total, err)
	}
}

// TestVehicleBranchScope: a member whose scope is "selected [A2]" sees a
// vehicle when its home OR current branch is A2, gets 404 otherwise and
// never sees it in list, count, facets, export or bulk targets.
func TestVehicleBranchScope(t *testing.T) {
	f := newFixture(t)
	homeA1 := f.create(t, f.orgA, f.input("34 AA 1"))
	atA2In := f.input("34 AA 2")
	atA2In.CurrentBranchUUID = &f.branchA2.Uuid
	atA2 := f.create(t, f.orgA, atA2In) // home A1, currently at A2
	homeA2In := f.input("34 AA 3")
	homeA2In.HomeBranchUUID = &f.branchA2.Uuid
	homeA2 := f.create(t, f.orgA, homeA2In)

	ctx := orgctx.WithScope(f.ctx, orgctx.Scope{ID: f.orgA, Slug: "a", BranchIDs: []int64{f.branchA2.ID}})
	if _, err := f.svc.Get(ctx, f.orgA, homeA1.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out of scope get: %v", err)
	}
	if _, err := f.svc.ChangeStatus(ctx, f.orgA, homeA1.UUID, model.StatusInput{Status: model.StatusDamaged}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out of scope status: %v", err)
	}
	if _, _, err := f.svc.Events(ctx, f.orgA, homeA1.UUID, 10, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out of scope events: %v", err)
	}
	if err := f.svc.Delete(ctx, f.orgA, homeA1.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("out of scope delete: %v", err)
	}
	for _, id := range []uuid.UUID{atA2.UUID, homeA2.UUID} {
		if _, err := f.svc.Get(ctx, f.orgA, id); err != nil {
			t.Fatalf("in scope get: %v", err)
		}
	}
	items, total, err := f.svc.List(ctx, f.orgA, model.ListFilter{Limit: 50})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("scoped list = %d %v", total, err)
	}
	for _, v := range items {
		if v.UUID == homeA1.UUID {
			t.Fatal("out of scope vehicle listed")
		}
	}
	facets, _ := f.svc.Facets(ctx, f.orgA)
	if len(facets.Status) != 1 || facets.Status[0].Count != 2 {
		t.Fatalf("scoped facets = %+v", facets.Status)
	}
	rows, _ := f.svc.ExportRows(ctx, f.orgA, model.ListFilter{})
	targets, _ := f.svc.TargetUUIDs(ctx, f.orgA, model.ListFilter{}, 100)
	if len(rows) != 2 || len(targets) != 2 || slices.Contains(targets, homeA1.UUID) {
		t.Fatalf("export / bulk targets = %d %v", len(rows), targets)
	}
	// A new vehicle (or a home change) needs the home branch in the scope;
	// the current branch may be any branch.
	outside := f.input("34 AA 4")
	if _, err := f.svc.Create(ctx, f.orgA, outside); !slices.Contains(validationFields(err), "home_branch_uuid:not_found") {
		t.Fatalf("create outside scope: %v", err)
	}
	if _, err := f.svc.MoveCurrentBranch(ctx, f.orgA, atA2.UUID, f.branchA1.Uuid); err != nil {
		t.Fatalf("move to another branch: %v", err)
	}
	// Home A1 + current A1: now outside the scope.
	if _, err := f.svc.Get(ctx, f.orgA, atA2.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("moved away: %v", err)
	}
	if _, err := f.svc.Patch(ctx, f.orgA, homeA2.UUID, model.VehicleInput{HomeBranchUUID: &f.branchA1.Uuid}); !slices.Contains(validationFields(err), "home_branch_uuid:not_found") {
		t.Fatalf("home change outside scope: %v", err)
	}
	// Every-branch members see everything.
	all := orgctx.WithScope(f.ctx, orgctx.Scope{ID: f.orgA, Slug: "a"})
	if _, total, _ := f.svc.List(all, f.orgA, model.ListFilter{Limit: 50}); total != 3 {
		t.Fatalf("unrestricted list = %d", total)
	}
}
