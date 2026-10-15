package adapters_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCustomerImportAndBulk: the İdes import matches an existing customer
// by the blind index of its identity number inside the organization only,
// the dry-run diff and the rollback snapshot carry no plaintext identity,
// rollback restores the customer, an export without the reveal permission
// is masked even when it asks for sensitive values, and bulk tagging never
// touches another organization's customer. Runs with TEST_DATABASE_URL.
func TestCustomerImportAndBulk(t *testing.T) {
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
	u, err := q.CreateUser(ctx, db.CreateUserParams{
		Email: "cust-io-" + suffix + "@example.test", PasswordHash: "x", Name: "IO", Surname: "Test", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	orgA, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Cust IO A " + suffix, OwnerUserID: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	orgB, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Cust IO B " + suffix, OwnerUserID: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{orgA.ID, orgB.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = $1", u.ID)
	})
	cipher, err := crypto.NewFieldCipher(strings.Repeat("i", 32))
	if err != nil {
		t.Fatal(err)
	}
	svc := usecase.New(repository.NewPostgres(pool, q), cipher, nil, nil, nil)
	io := adapters.NewIO(svc)
	ctxA := orgctx.WithScope(ctx, orgctx.Scope{ID: orgA.ID, Permissions: []string{rbac.PermTenantCustomersRead}})
	ctxB := orgctx.WithScope(ctx, orgctx.Scope{ID: orgB.ID})

	existing, err := svc.Create(ctx, orgA.ID, model.CustomerInput{
		FirstName: model.Str("Ayşe"), LastName: model.Str("Yılmaz"), IdentityType: model.String("tckn"),
		IdentityNumber: model.Str("10000000146"), Phone: model.Str("05321234512"),
	})
	if err != nil {
		t.Fatal(err)
	}
	row := map[string]any{
		"customer_type": "Bireysel", "first_name": "Ayşe", "last_name": "Kaya", "tckn": "10000000146",
		"phone": "0532 999 88 77", "tags": "VIP; Filo",
	}
	// Dry run in A: an update of the existing customer, masked values.
	diff, err := io.PreviewRow(ctxA, row, nil)
	if err != nil || diff.Error != "" || diff.Op != "update" {
		t.Fatalf("preview in A = %+v, %v", diff, err)
	}
	raw, _ := json.Marshal(diff.Changes)
	if strings.Contains(string(raw), "10000000146") || strings.Contains(string(raw), "9998877") || !strings.Contains(string(raw), "last_name") {
		t.Fatalf("diff = %s", raw)
	}
	// The same identity in B is a new customer of B (no cross-organization match).
	if diff, err := io.PreviewRow(ctxB, row, nil); err != nil || diff.Op != "create" {
		t.Fatalf("preview in B = %+v, %v", diff, err)
	}
	// Apply in A, then roll back.
	res, err := io.ApplyRow(ctxA, row, nil)
	if err != nil || !res.OK || res.Op != "update" || res.EntityUUID != existing.UUID.String() {
		t.Fatalf("apply = %+v, %v", res, err)
	}
	snap, _ := json.Marshal(res.Previous)
	if strings.Contains(string(snap), "10000000146") {
		t.Fatalf("rollback snapshot holds the plaintext identity: %s", snap)
	}
	updated, _ := svc.Get(ctx, orgA.ID, existing.UUID)
	if updated.LastName != "Kaya" || strings.Join(updated.Tags, ",") != "VIP,Filo" {
		t.Fatalf("updated = %+v", updated)
	}
	if err := io.RevertRow(ctxA, res.EntityType, res.EntityUUID, res.Previous); err != nil {
		t.Fatal(err)
	}
	restored, _ := svc.Get(ctx, orgA.ID, existing.UUID)
	if restored.LastName != "Yılmaz" || restored.IdentityMasked != "100******46" || restored.PhoneMasked != existing.PhoneMasked {
		t.Fatalf("restored = %+v", restored)
	}
	// A bad checksum is a row error, not a job failure.
	if res, err := io.ApplyRow(ctxA, map[string]any{"first_name": "x", "last_name": "y", "tckn": "10000000147"}, nil); err != nil || res.OK {
		t.Fatalf("bad row = %+v, %v", res, err)
	}

	// Export: include_sensitive without the reveal permission stays masked.
	ds, err := io.Export(ctxA, ioengine.ExportQuery{"include_sensitive": "true"}, "tr")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(ds.Rows)
	if strings.Contains(string(out), "10000000146") || !strings.Contains(string(out), "100******46") {
		t.Fatalf("export without reveal: %s", out)
	}
	ctxReveal := orgctx.WithScope(ctx, orgctx.Scope{ID: orgA.ID, Permissions: []string{rbac.PermTenantCustomersReveal}})
	ds, _ = io.Export(ctxReveal, ioengine.ExportQuery{"include_sensitive": "true"}, "tr")
	if out, _ := json.Marshal(ds.Rows); !strings.Contains(string(out), "10000000146") {
		t.Fatalf("export with reveal lacks plaintext: %s", out)
	}

	// Bulk: B's customer is not a target of A.
	other, err := svc.Create(ctx, orgB.ID, model.CustomerInput{FirstName: model.Str("B"), LastName: model.Str("Müşteri")})
	if err != nil {
		t.Fatal(err)
	}
	bulk := adapters.NewBulk(svc)
	bctx := bulkengine.WithParams(ctxA, map[string]string{"tag": "Kampanya"})
	item, err := bulk.ApplyItem(bctx, adapters.ActionAddTag, other.UUID.String())
	if err != nil || item.OK {
		t.Fatalf("bulk on B's customer = %+v, %v", item, err)
	}
	item, err = bulk.ApplyItem(bctx, adapters.ActionAddTag, existing.UUID.String())
	if err != nil || !item.OK {
		t.Fatalf("bulk add_tag = %+v, %v", item, err)
	}
	if err := bulk.RevertItem(ctxA, adapters.ActionAddTag, existing.UUID.String(), roundTrip(item.Previous)); err != nil {
		t.Fatal(err)
	}
	if c, _ := svc.Get(ctx, orgA.ID, existing.UUID); strings.Contains(strings.Join(c.Tags, ","), "Kampanya") {
		t.Fatalf("bulk rollback left the tag: %v", c.Tags)
	}
	if c, _ := svc.Get(ctx, orgB.ID, other.UUID); len(c.Tags) != 0 {
		t.Fatalf("B's customer changed: %v", c.Tags)
	}
}

// roundTrip mimics the JSON storage of bulk previous values.
func roundTrip(m map[string]any) map[string]any {
	raw, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
