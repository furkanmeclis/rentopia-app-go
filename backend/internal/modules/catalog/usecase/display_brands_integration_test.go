package usecase_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// memDriver serves the logo calls from the in-memory store.
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

// TestCatalogDisplayBrandsAgainstPostgres covers display brands (mapping on
// import, merge / unmerge / rename / logo), grouped tenant models and the
// auto model rebuild. Like the import test it TRUNCATES the catalog tables
// and only runs with CATALOG_TEST_DATABASE_URL.
func TestCatalogDisplayBrandsAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("CATALOG_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CATALOG_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `TRUNCATE catalog_brands, catalog_display_brands, catalog_imports RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(repository.NewPostgres(pool, db.New(pool)), memDriver{m: storage.NewMemory()}, nil, nil, nil)

	oct := &tsbcatalog.Sheet{Period: "2026-10", Rows: []tsbcatalog.Row{
		{BrandCode: "52", BrandName: "FIAT", TypeCode: "1", TypeName: "EGEA SEDAN URBAN 1.4 FIRE 95", Values: map[int]int64{2026: 900000}},
		{BrandCode: "52", BrandName: "FIAT", TypeCode: "2", TypeName: "500 DOLCEVITA 1.0 HYBRID", Values: map[int]int64{2026: 800000}},
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "1", TypeName: "EGEA CROSS STREET 1.4 FIRE 95", Values: map[int]int64{2026: 1000000}},
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "2", TypeName: "EGEA HB LOUNGE 1.6 M.JET 130", Values: map[int]int64{2026: 1100000}},
		{BrandCode: "21", BrandName: "BMW", TypeCode: "1", TypeName: "320d SEDAN 2.0 190", Values: map[int]int64{2026: 3000000}},
		{BrandCode: "21", BrandName: "BMW", TypeCode: "2", TypeName: "330i 2.0 M SPORT", Values: map[int]int64{2026: 3500000}},
		{BrandCode: "125", BrandName: "RANGE ROVER", TypeCode: "1", TypeName: "3.0 D300 HSE", Values: map[int]int64{2026: 9000000}},
	}}
	plan, res, err := svc.Run(ctx, oct, "", true)
	if err != nil {
		t.Fatal(err)
	}
	// FIAT and TOFAS-FIAT share one display brand.
	if res.BrandsInserted != 4 || res.DisplayBrandsInserted != 3 || plan.Counts.DisplayBrandsNew != 3 {
		t.Fatalf("first apply: %+v / %+v", res, plan.Counts)
	}
	if plan.Counts.UnresolvedModels != 0 {
		t.Fatalf("patterns should resolve every type: %+v", plan.Counts)
	}
	_, res, err = svc.Run(ctx, oct, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.BrandsInserted+res.BrandsUpdated+res.DisplayBrandsInserted+res.ModelsInserted+res.TypesInserted+
		res.TypesUpdated+res.TypesDeactivated+res.ValuationsUpserted != 0 || len(res.UnresolvedByCategory) != 0 {
		t.Fatalf("re-apply changed rows: %+v", res)
	}

	merged, _, err := svc.ListDisplayBrands(ctx, model.DisplayBrandFilter{Limit: 10, MergedOnly: true, SortField: "name"})
	if err != nil || len(merged) != 1 || merged[0].Name != "Fiat" || merged[0].BrandsCount != 2 {
		t.Fatalf("merged display brands: %v %+v", err, merged)
	}
	fiat := merged[0]

	// Tenant: one Fiat, one Egea (both TSB brands), types of both brands.
	brands, _, err := svc.TenantBrands(ctx, model.TenantFilter{Limit: 10, Q: "fiat"})
	if err != nil || len(brands) != 1 || brands[0].UUID != fiat.UUID {
		t.Fatalf("tenant brands: %v %+v", err, brands)
	}
	models, _, err := svc.TenantModels(ctx, model.TenantFilter{Limit: 20, BrandUUID: fiat.UUID})
	if err != nil {
		t.Fatal(err)
	}
	var egea *model.TenantModel
	for i := range models {
		if models[i].Name == "EGEA" {
			if egea != nil {
				t.Fatalf("EGEA listed twice: %+v", models)
			}
			egea = &models[i]
		}
	}
	if egea == nil || len(models) != 3 { // EGEA, EGEA CROSS, 500 DOLCEVITA… (by group key)
		t.Fatalf("tenant models: %+v", models)
	}
	types, total, err := svc.TenantTypes(ctx, model.TenantFilter{Limit: 20, ModelUUID: egea.UUID})
	if err != nil || total != 2 || len(types) != 2 || types[0].Model.UUID != egea.UUID || types[0].Brand.UUID != fiat.UUID {
		t.Fatalf("tenant types of grouped EGEA: %v %d %+v", err, total, types)
	}
	// A TSB brand uuid still works as the brand reference (older clients).
	tsbFiat, _, err := svc.ListBrands(ctx, model.BrandFilter{Limit: 10, Q: "TOFAS", SortField: "display_name"})
	if err != nil || len(tsbFiat) != 1 || tsbFiat[0].DisplayBrand.UUID != fiat.UUID {
		t.Fatalf("tsb brand: %v %+v", err, tsbFiat)
	}
	if again, _, err := svc.TenantModels(ctx, model.TenantFilter{Limit: 20, BrandUUID: tsbFiat[0].UUID}); err != nil || len(again) != len(models) {
		t.Fatalf("tenant models by tsb brand: %v %+v", err, again)
	}

	// Unmerge TOFAS-FIAT into its own display brand, then merge it back.
	split, err := svc.UnmergeBrand(ctx, fiat.UUID, tsbFiat[0].UUID, "")
	if err != nil || split.Name != "Tofas-Fiat" || split.BrandsCount != 1 || split.Slug != "tofas-fiat" {
		t.Fatalf("unmerge: %v %+v", err, split)
	}
	if _, err := svc.UnmergeBrand(ctx, split.UUID, tsbFiat[0].UUID, ""); err == nil {
		t.Fatal("unmerging a single-brand display brand must fail")
	}
	back, err := svc.MergeDisplayBrands(ctx, split.UUID, fiat.UUID)
	if err != nil || back.BrandsCount != 2 || len(back.Brands) != 2 {
		t.Fatalf("merge back: %v %+v", err, back)
	}
	if _, err := svc.GetDisplayBrand(ctx, split.UUID); err == nil {
		t.Fatal("merged display brand should be deleted")
	}

	// Rename: members' display_name follows, slug changes.
	newName := "FIAT Türkiye"
	renamed, err := svc.PatchDisplayBrand(ctx, fiat.UUID, model.PatchDisplayBrandInput{Name: &newName})
	if err != nil || renamed.Slug != "fiat-turkiye" {
		t.Fatalf("rename: %v %+v", err, renamed)
	}
	b, err := svc.GetBrand(ctx, tsbFiat[0].UUID)
	if err != nil || b.DisplayName != newName {
		t.Fatalf("member display name: %v %+v", err, b)
	}

	// Re-map a TSB brand by display name (creates "Bmw Group").
	bmw, _, err := svc.ListBrands(ctx, model.BrandFilter{Limit: 10, Q: "BMW", SortField: "display_name"})
	if err != nil || len(bmw) != 1 {
		t.Fatalf("bmw: %v %+v", err, bmw)
	}
	group := "Bmw Group"
	moved, err := svc.PatchBrand(ctx, bmw[0].UUID, model.PatchBrandInput{DisplayName: &group})
	if err != nil || moved.DisplayBrand.Name != group {
		t.Fatalf("remap: %v %+v", err, moved)
	}
	if _, err := svc.GetDisplayBrand(ctx, bmw[0].DisplayBrand.UUID); err == nil {
		t.Fatal("empty display brand should be removed")
	}

	// Logo: stored under catalog/brands/{uuid}/, served by the API.
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	withLogo, err := svc.SetDisplayBrandLogo(ctx, fiat.UUID, storage.File{
		Body: bytes.NewReader(png), Size: int64(len(png)), ContentType: "image/png", Filename: "fiat.png",
	}, "png")
	if err != nil || withLogo.LogoURL == nil {
		t.Fatalf("logo: %v %+v", err, withLogo)
	}
	rc, mime, err := svc.OpenDisplayBrandLogo(ctx, fiat.UUID)
	if err != nil || mime != "image/png" {
		t.Fatalf("open logo: %v %q", err, mime)
	}
	_ = rc.Close()
	if cleared, err := svc.ClearDisplayBrandLogo(ctx, fiat.UUID); err != nil || cleared.LogoURL != nil {
		t.Fatalf("clear logo: %v %+v", err, cleared)
	}

	// Cross-brand model merge renames into one group; bulk rules apply.
	all, _, err := svc.ListModels(ctx, model.ModelFilter{Limit: 50, DisplayBrandUUID: &fiat.UUID, SortField: "name"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]model.Model{}
	for _, m := range all {
		byName[m.Brand.TSBName+"/"+m.Name] = m
	}
	cross, ok := byName["TOFAS-FIAT/EGEA CROSS"]
	if !ok {
		t.Fatalf("models: %+v", all)
	}
	fiatEgea := byName["FIAT/EGEA"]
	if _, err := svc.MergeModel(ctx, cross.UUID, fiatEgea.UUID); err != nil {
		t.Fatal(err)
	}
	// TOFAS-FIAT already had EGEA: EGEA CROSS was merged into it.
	if _, err := svc.GetModel(ctx, cross.UUID); err == nil {
		t.Fatal("EGEA CROSS should be merged away")
	}
	grouped, err := svc.GetModel(ctx, fiatEgea.UUID)
	if err != nil || grouped.GroupSize != 2 {
		t.Fatalf("group size: %v %+v", err, grouped)
	}
	bulk, err := svc.BulkModelRules(ctx, []uuid.UUID{fiatEgea.UUID})
	if err != nil || bulk.RulesSaved != 1 {
		t.Fatalf("bulk rules: %v %+v", err, bulk)
	}

	// Rebuild keeps manual models / rules and is a no-op the second time.
	if _, err := svc.RebuildAutoModels(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := svc.RebuildAutoModels(ctx)
	if err != nil || again.TypesChanged != 0 || again.ModelsCreated != 0 || again.ModelsDeleted != 0 {
		t.Fatalf("second rebuild: %v %+v", err, again)
	}
	_, res, err = svc.Run(ctx, oct, "", true)
	if err != nil || res.TypesInserted+res.TypesUpdated+res.ModelsInserted+res.BrandsInserted+res.DisplayBrandsInserted != 0 {
		t.Fatalf("re-apply after curation: %v %+v", err, res)
	}
}
