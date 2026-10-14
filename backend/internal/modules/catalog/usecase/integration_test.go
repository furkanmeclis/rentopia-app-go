package usecase_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tsbcatalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

type liveEvent struct{ resource, action, uuid string }

type liveRecorder struct {
	mu     sync.Mutex
	events []liveEvent
}

func (l *liveRecorder) Publish(_ context.Context, resource, action string, uuids ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ev := liveEvent{resource: resource, action: action}
	if len(uuids) > 0 {
		ev.uuid = uuids[0]
	}
	l.events = append(l.events, ev)
}

func (l *liveRecorder) take() []liveEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.events
	l.events = nil
	return out
}

// TestCatalogImportAgainstPostgres runs plan + apply against a real
// database. It TRUNCATES the catalog tables, so it only runs when
// CATALOG_TEST_DATABASE_URL points at a throwaway, migrated database.
func TestCatalogImportAgainstPostgres(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `TRUNCATE catalog_brands, catalog_imports RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	q := db.New(pool)
	svc := usecase.New(repository.NewPostgres(pool, q), nil, nil, nil, nil)
	live := &liveRecorder{}
	svc.SetResourceEvents(live)

	oct := &tsbcatalog.Sheet{Period: "2026-10", Rows: []tsbcatalog.Row{
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "1402", TypeName: "EGEA CROSS STREET 1.4 FIRE 95 E6D", Values: map[int]int64{2024: 1051836, 2023: 971010}},
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "1403", TypeName: "EGEA CROSS LOUNGE 1.6 M.JET 130 DCT", Values: map[int]int64{2026: 1300000}},
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "1500", TypeName: "EGEA SEDAN URBAN 1.4 FIRE 95", Values: map[int]int64{2026: 900000}},
		{BrandCode: "900", BrandName: "MOTORSIKLET", TypeCode: "1", TypeName: "KAWASAKI NINJA 300", Values: map[int]int64{2026: 300000}},
	}}
	_, res, err := svc.Run(ctx, oct, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.BrandsInserted != 2 || res.ModelsInserted != 2 || res.TypesInserted != 4 || res.ValuationsUpserted != 5 {
		t.Fatalf("first apply: %+v", res)
	}
	if res.UnresolvedByCategory[tsbcatalog.CategoryOther] != 1 {
		t.Fatalf("unresolved: %+v", res.UnresolvedByCategory)
	}

	// Idempotent: the same period again changes nothing.
	plan, res, err := svc.Run(ctx, oct, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.BrandsInserted+res.BrandsUpdated+res.ModelsInserted+res.TypesInserted+res.TypesUpdated+
		res.TypesDeactivated+res.ValuationsUpserted != 0 || plan.Counts.ValuationsUpserted != 0 {
		t.Fatalf("second apply changed rows: %+v / %+v", res, plan.Counts)
	}

	// Assign the motorcycle a manual model with a prefix rule.
	brands, _, err := svc.ListBrands(ctx, model.BrandFilter{Limit: 10, Q: "MOTORSIKLET", SortField: "display_name"})
	if err != nil || len(brands) != 1 {
		t.Fatalf("brands: %v %+v", err, brands)
	}
	ninja, err := svc.CreateModel(ctx, brands[0].UUID, "Kawasaki Ninja")
	if err != nil {
		t.Fatal(err)
	}
	types, _, err := svc.ListTypes(ctx, model.TypeFilter{Limit: 10, Unresolved: true, SortField: "raw_name"})
	if err != nil || len(types) != 1 {
		t.Fatalf("unresolved types: %v %+v", err, types)
	}
	if _, err := svc.PatchType(ctx, types[0].UUID, model.PatchTypeInput{
		SetModel: true, ModelUUID: &ninja.UUID, SaveRule: true, RulePrefix: "KAWASAKI NINJA",
	}); err != nil {
		t.Fatal(err)
	}
	// Curation edits reach the live catalog lists.
	if got := live.take(); len(got) != 2 ||
		got[0] != (liveEvent{usecase.LiveModels, resourceevents.ActionCreated, ninja.UUID.String()}) ||
		got[1] != (liveEvent{usecase.LiveTypes, resourceevents.ActionBulk, ""}) {
		t.Fatalf("live events = %+v", got)
	}

	// Next month: 1403 drops out, 1402 changes value, a new Ninja appears.
	nov := &tsbcatalog.Sheet{Period: "2026-11", Rows: []tsbcatalog.Row{
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "1402", TypeName: "EGEA CROSS STREET 1.4 FIRE 95 E6D", Values: map[int]int64{2024: 1060000, 2023: 971010}},
		{BrandCode: "100", BrandName: "TOFAS-FIAT", TypeCode: "1500", TypeName: "EGEA SEDAN URBAN 1.4 FIRE 95", Values: map[int]int64{2026: 905000}},
		{BrandCode: "900", BrandName: "MOTORSIKLET", TypeCode: "1", TypeName: "KAWASAKI NINJA 300", Values: map[int]int64{2026: 310000}},
		{BrandCode: "900", BrandName: "MOTORSIKLET", TypeCode: "2", TypeName: "KAWASAKI NINJA 650", Values: map[int]int64{2026: 500000}},
	}}
	_, res, err = svc.Run(ctx, nov, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.TypesInserted != 1 || res.TypesDeactivated != 1 || res.ValuationsUpserted != 5 || len(res.UnresolvedByCategory) != 0 {
		t.Fatalf("november apply: %+v", res)
	}
	var history int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM catalog_valuations`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 10 { // October rows are kept
		t.Fatalf("valuation history = %d, want 10", history)
	}

	// Tenant lookup returns the latest value for the requested year.
	year := int32(2024)
	models, _, err := svc.ListModels(ctx, model.ModelFilter{Limit: 10, Q: "EGEA CROSS", SortField: "name"})
	if err != nil || len(models) != 1 {
		t.Fatalf("models: %v %+v", err, models)
	}
	tt, _, err := svc.TenantTypes(ctx, model.TenantFilter{Limit: 10, ModelUUID: models[0].UUID, Year: &year})
	if err != nil || len(tt) != 1 || tt[0].KaskoValue == nil || *tt[0].KaskoValue != 1060000 || *tt[0].KaskoPeriod != "2026-11" {
		t.Fatalf("tenant types: %v %+v", err, tt)
	}
	detail, err := svc.GetType(ctx, tt[0].UUID)
	if err != nil || detail.LatestPeriod != "2026-11" || len(detail.Valuations) != 2 || len(detail.History) != 4 {
		t.Fatalf("detail: %v %+v", err, detail)
	}
}
