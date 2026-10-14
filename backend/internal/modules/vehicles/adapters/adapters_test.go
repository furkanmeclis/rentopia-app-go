package adapters

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestImportSchemaMapsIdesHeaders(t *testing.T) {
	t.Parallel()
	headers := []string{"Plaka", "Şasi", "Marka", "Model", "Tip/Kasko Kodu", "Model Yılı", "Renk", "Vites", "Yakıt", "Şube", "Mülkiyet", "KM"}
	got := ioengine.SuggestMapping(headers, NewIO(nil).ImportSchema(), i18n.Normalize("tr"))
	want := map[string]string{
		"Plaka": "plate", "Şasi": "vin", "Marka": "brand", "Model": "model", "Tip/Kasko Kodu": "type_code",
		"Model Yılı": "model_year", "Renk": "color", "Vites": "transmission", "Yakıt": "fuel", "Şube": "branch",
		"Mülkiyet": "ownership", "KM": "odometer_km",
	}
	for header, key := range want {
		if got[header] != key {
			t.Errorf("%s → %q, want %q", header, got[header], key)
		}
	}
}

// TestFleetImportAdapter imports İdes fleet rows through the io adapter:
// catalog match by kasko code, by brand + model, manual fallback, Turkish
// values, the dry-run diff of an existing plate and rollback. It also runs a
// parameterised bulk action and its rollback. Runs with TEST_DATABASE_URL.
func TestFleetImportAdapter(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	suffix := uuid.NewString()[:8]
	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "veh-io-" + suffix, Name: "IO"})
	if err != nil {
		t.Fatal(err)
	}
	brandName := "Renault " + suffix
	var display, brand, mdl int64
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_display_brands (name, slug) VALUES ($1, $2) RETURNING id`,
		brandName, "renault-"+strings.ToLower(suffix)).Scan(&display); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM catalog_brands WHERE id = $1", brand)
		_, _ = pool.Exec(bg, "DELETE FROM catalog_display_brands WHERE id = $1", display)
	})
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_brands (tsb_code, tsb_name, display_name, display_brand_id) VALUES ($1, 'RENAULT', $2, $3) RETURNING id`,
		"R"+suffix, brandName, display).Scan(&brand); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO catalog_models (brand_id, name, source) VALUES ($1, 'Clio', 'manual') RETURNING id`, brand).Scan(&mdl); err != nil {
		t.Fatal(err)
	}
	code := "C" + suffix
	if _, err := pool.Exec(ctx, `INSERT INTO catalog_types (brand_id, model_id, tsb_type_code, raw_name, years, first_seen_period, last_seen_period) VALUES
		($1, $2, $3, 'CLIO 1.0 TCE JOY', '{2024,2025}', '2026-01', '2026-09'),
		($1, $2, $4, 'CLIO 1.5 DCI TOUCH', '{2022}', '2026-01', '2026-09')`, brand, mdl, code, "D"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateBranch(ctx, db.CreateBranchParams{
		OrganizationID: org.ID, Code: "ANK", Name: "Ankara", WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
		PrepMinutes: 60, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(repository.NewPostgres(pool, q), nil)
	io := NewIO(svc)
	octx := orgctx.WithOrganizationID(ctx, org.ID)
	if _, err := io.ApplyRow(ctx, map[string]any{"plate": "06 X 1"}, nil); err == nil {
		t.Fatal("adapter without an organization must fail")
	}

	rows := []map[string]any{
		// Kasko code match; Turkish values; dotted km.
		{"plate": "06 abc 01", "vin": "vf1rja00x12345678", "brand": brandName, "model": "Clio", "type_code": code,
			"model_year": "2025", "color": "Beyaz", "transmission": "Otomatik", "fuel": "Benzin", "branch": "ank",
			"ownership": "Özmal", "odometer_km": "12.500"},
		// Brand + model: the only type valid for 2022.
		{"plate": "06 ABC 02", "brand": brandName, "model": "clio", "model_year": "2022", "transmission": "Manuel",
			"fuel": "Dizel", "branch": "Ankara", "ownership": "Tedarikçi", "supplier_name": "Filo A.Ş."},
		// Not in the catalog: manual entry.
		{"plate": "06 ABC 03", "brand": "Togg", "model": "T10X", "type_name": "V2 RWD", "model_year": "2025",
			"fuel": "Elektrik", "branch": "ANK"},
	}
	created := make([]string, 0, len(rows))
	for i, row := range rows {
		res, err := io.ApplyRow(octx, row, map[string]any{"fleet_pool": "main"})
		if err != nil || !res.OK || res.Op != "create" {
			t.Fatalf("row %d = %+v %v", i, res, err)
		}
		created = append(created, res.EntityUUID)
	}
	get := func(id string) model.Vehicle {
		t.Helper()
		v, err := svc.Get(octx, org.ID, uuid.MustParse(id))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v1, v2, v3 := get(created[0]), get(created[1]), get(created[2])
	if v1.Catalog == nil || v1.Catalog.TypeName != "CLIO 1.0 TCE JOY" || v1.VIN != "VF1RJA00X12345678" || *v1.Transmission != "automatic" ||
		*v1.Fuel != "petrol" || v1.OdometerKm != 12500 || v1.HomeBranch.Code != "ANK" || v1.Ownership != "owned" || v1.FleetPool != "MAIN" {
		t.Fatalf("row 1 = %+v", v1)
	}
	if v2.Catalog == nil || v2.Catalog.TypeName != "CLIO 1.5 DCI TOUCH" || v2.Ownership != "supplied" || v2.SupplierName != "Filo A.Ş." {
		t.Fatalf("row 2 = %+v", v2)
	}
	if !v3.ManualEntry || v3.Label != "Togg T10X V2 RWD" || *v3.Fuel != "electric" {
		t.Fatalf("row 3 = %+v", v3)
	}
	// Invalid rows are row errors, not job failures.
	for _, bad := range []map[string]any{
		{"plate": "06 ABC 09", "brand": brandName, "model": "Clio", "type_code": code, "model_year": "2019", "branch": "ANK"},
		{"plate": "06 ABC 10", "brand": "Togg", "model": "T10X", "model_year": "2025", "branch": "IZM"},
		{"plate": "06 ABC 11", "brand": "Togg", "model": "T10X", "model_year": "2025", "branch": "ANK", "fuel": "hidrojen"},
	} {
		if res, err := io.ApplyRow(octx, bad, nil); err != nil || res.OK {
			t.Fatalf("bad row %v = %+v %v", bad, res, err)
		}
	}

	// Dry run of the same plate: an update listing the changed fields.
	diff, err := io.PreviewRow(octx, map[string]any{"plate": "06ABC01", "color": "Siyah", "odometer_km": "13000"}, nil)
	if err != nil || diff.Error != "" || diff.Op != "update" || len(diff.Changes) != 2 {
		t.Fatalf("preview = %+v %v", diff, err)
	}
	if diff.Changes[0].Field != "color" || diff.Changes[0].From != "Beyaz" || diff.Changes[0].To != "Siyah" {
		t.Fatalf("preview changes = %+v", diff.Changes)
	}
	if v, _ := svc.Get(octx, org.ID, v1.UUID); v.Color != "Beyaz" {
		t.Fatal("preview wrote the row")
	}
	if diff, _ := io.PreviewRow(octx, map[string]any{"plate": "06 NEW 1", "brand": "Togg", "model": "T10X", "model_year": "2025", "branch": "ANK"}, nil); diff.Op != "create" {
		t.Fatalf("preview create = %+v", diff)
	}

	// Update by plate keeps a snapshot; rollback restores it.
	res, err := io.ApplyRow(octx, map[string]any{"plate": "06-ABC-01", "color": "Siyah"}, nil)
	if err != nil || !res.OK || res.Op != "update" || res.EntityUUID != created[0] {
		t.Fatalf("update = %+v %v", res, err)
	}
	if err := io.RevertRow(octx, res.EntityType, res.EntityUUID, res.Previous); err != nil {
		t.Fatal(err)
	}
	if v := get(created[0]); v.Color != "Beyaz" {
		t.Fatalf("revert update = %q", v.Color)
	}
	if err := io.RevertRow(octx, "vehicle", created[2], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(octx, org.ID, v3.UUID); err == nil {
		t.Fatal("revert create kept the vehicle")
	}

	// Export carries the kasko code and the branch code.
	ds, err := io.Export(octx, ioengine.ExportQuery{"status": "available"}, i18n.Normalize("tr"))
	if err != nil || len(ds.Rows) != 2 || ds.Rows[0]["type_code"] != code || ds.Rows[0]["home_branch"] != "ANK" {
		t.Fatalf("export = %+v %v", ds.Rows, err)
	}

	// Bulk set_status with params, then rollback.
	bulk := NewBulk(svc)
	if _, err := bulk.ResolveTargets(octx, ActionSetStatus, bulkengine.BulkTarget{Scope: "ids", IDs: created[:1], Params: map[string]string{"status": "flying"}}); err == nil {
		t.Fatal("invalid status param accepted")
	}
	targets, err := bulk.ResolveTargets(octx, ActionSetStatus, bulkengine.BulkTarget{
		Scope: "query", Query: map[string]string{"q": "06abc"}, Params: map[string]string{"status": "damaged"},
	})
	if err != nil || len(targets) != 2 {
		t.Fatalf("targets = %v %v", targets, err)
	}
	pctx := bulkengine.WithParams(octx, map[string]string{"status": "damaged", "note": "Dolu"})
	item, err := bulk.ApplyItem(pctx, ActionSetStatus, created[0])
	if err != nil || !item.OK || item.Previous["status"] != "available" {
		t.Fatalf("bulk item = %+v %v", item, err)
	}
	if v := get(created[0]); v.Status != "damaged" {
		t.Fatalf("bulk status = %s", v.Status)
	}
	if err := bulk.RevertItem(octx, ActionSetStatus, created[0], item.Previous); err != nil {
		t.Fatal(err)
	}
	if v := get(created[0]); v.Status != "available" {
		t.Fatalf("bulk revert = %s", v.Status)
	}
}
