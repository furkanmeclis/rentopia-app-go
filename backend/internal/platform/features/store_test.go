package features_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The seeded catalog, the plan defaults (Komple = everything, Yönetim =
// nothing), the default for an organization without a plan row and the
// override precedence, against the real schema.
func TestStoreResolvesPlanAndOverrides(t *testing.T) {
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
	store := features.NewStore(q)

	keys, err := q.ListFeatureKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(keys, ",") != strings.Join(features.AllStrings(), ",") {
		t.Fatalf("features table %v differs from the catalog %v", keys, features.AllStrings())
	}
	plans, err := q.ListPlans(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string][]string{}
	for _, p := range plans {
		byCode[p.Code] = p.Features
	}
	if strings.Join(byCode[features.PlanComplete], ",") != strings.Join(features.AllStrings(), ",") {
		t.Fatalf("complete plan features: %v", byCode[features.PlanComplete])
	}
	if f, ok := byCode[features.PlanManagement]; !ok || len(f) != 0 {
		t.Fatalf("management plan features: %v (present %v)", f, ok)
	}

	org, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: "feat-" + uuid.NewString()[:8], Name: "Features"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", org.ID) })

	enabled := func() string {
		t.Helper()
		f, err := store.EnabledFeatures(ctx, org.ID)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(features.Strings(f), ",")
	}
	// No plan row: the default plan (Komple).
	if got := enabled(); got != strings.Join(features.AllStrings(), ",") {
		t.Fatalf("without a plan row: %s", got)
	}
	if p, err := store.Plan(ctx, org.ID); err != nil || p.Code != features.DefaultPlan || p.Assigned {
		t.Fatalf("default plan: %+v %v", p, err)
	}
	mgmt, err := q.GetPlanByCode(ctx, features.PlanManagement)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertOrganizationPlan(ctx, db.UpsertOrganizationPlanParams{OrganizationID: org.ID, PlanID: mgmt.ID}); err != nil {
		t.Fatal(err)
	}
	if got := enabled(); got != "" {
		t.Fatalf("management plan enables %q", got)
	}
	// Override on beats the plan's off; clearing it falls back to the plan.
	if err := q.UpsertOrganizationFeature(ctx, db.UpsertOrganizationFeatureParams{
		OrganizationID: org.ID, FeatureKey: "public_api", Enabled: true, UpdatedBy: pgtype.Int8{},
	}); err != nil {
		t.Fatal(err)
	}
	if got := enabled(); got != "public_api" {
		t.Fatalf("override on: %q", got)
	}
	complete, err := q.GetPlanByCode(ctx, features.PlanComplete)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertOrganizationPlan(ctx, db.UpsertOrganizationPlanParams{OrganizationID: org.ID, PlanID: complete.ID}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertOrganizationFeature(ctx, db.UpsertOrganizationFeatureParams{
		OrganizationID: org.ID, FeatureKey: "sms", Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	got := enabled()
	if strings.Contains(got, "sms") || !strings.Contains(got, "public_api") || !strings.Contains(got, "site") {
		t.Fatalf("override off on complete: %q", got)
	}
	if _, err := q.DeleteOrganizationFeature(ctx, db.DeleteOrganizationFeatureParams{OrganizationID: org.ID, FeatureKey: "sms"}); err != nil {
		t.Fatal(err)
	}
	if got := enabled(); !strings.Contains(got, "sms") {
		t.Fatalf("cleared override: %q", got)
	}
	if p, err := store.Plan(ctx, org.ID); err != nil || p.Code != features.PlanComplete || !p.Assigned {
		t.Fatalf("assigned plan: %+v %v", p, err)
	}
}
