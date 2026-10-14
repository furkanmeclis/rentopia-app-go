package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/repository"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- pure rules (always run) ---------------------------------------------------

func TestSniffDocumentType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		head []byte
		ext  string
		ok   bool
	}{
		{"pdf", []byte("%PDF-1.7\n%âãÏÓ\n1 0 obj"), "pdf", true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'}, "jpg", true},
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), "png", true},
		{"webp", []byte("RIFF\x24\x00\x00\x00WEBPVP8 "), "webp", true},
		{"html", []byte("<!DOCTYPE html><html><script>alert(1)</script>"), "", false},
		{"svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), "", false},
		{"text", []byte("plain text pretending to be a pdf"), "", false},
		{"zip", []byte("PK\x03\x04\x14\x00\x00\x00"), "", false},
	}
	for _, tc := range cases {
		_, ext, ok := SniffDocumentType(tc.head)
		if ok != tc.ok || ext != tc.ext {
			t.Errorf("%s: ext=%q ok=%v, want %q %v", tc.name, ext, ok, tc.ext, tc.ok)
		}
	}
}

func TestCleanFilename(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"../../etc/passwd":      "passwd",
		`C:\scans\ruhsat.pdf`:   "ruhsat.pdf",
		"":                      "document.pdf",
		"muayene \"2026\".pdf":  "muayene 2026.pdf",
		"trafik\x00sigorta.pdf": "trafiksigorta.pdf",
	} {
		if got := cleanFilename(in, "pdf"); got != want {
			t.Errorf("cleanFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReminderCadence(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	days := func(n int) *int { return &n }
	a := func(state string, left *int) alert {
		return alert{Alert: model.Alert{State: state, DaysLeft: left}}
	}
	prev := func(daysAgo int, state string) repository.LastReminder {
		return repository.LastReminder{Day: today.AddDate(0, 0, -daysAgo), State: state}
	}
	cases := []struct {
		name string
		a    alert
		prev repository.LastReminder
		seen bool
		want bool
	}{
		{"first time", a(model.AlertExpiring, days(20)), repository.LastReminder{}, false, true},
		{"state changed", a(model.AlertOverdue, days(-1)), prev(1, model.AlertExpiring), true, true},
		{"yesterday, no milestone", a(model.AlertExpiring, days(12)), prev(1, model.AlertExpiring), true, false},
		{"milestone 7", a(model.AlertExpiring, days(7)), prev(1, model.AlertExpiring), true, true},
		{"milestone 0", a(model.AlertExpiring, days(0)), prev(1, model.AlertExpiring), true, true},
		{"a week later", a(model.AlertOverdue, days(-9)), prev(7, model.AlertOverdue), true, true},
		{"km plan, 3 days", a(model.AlertExpiring, nil), prev(3, model.AlertExpiring), true, false},
	}
	for _, tc := range cases {
		if got := shouldRemind(tc.a, tc.prev, tc.seen, today); got != tc.want {
			t.Errorf("%s: shouldRemind = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestReminderItemText(t *testing.T) {
	t.Parallel()
	three, past := 3, -2
	date := "2026-10-17"
	km := int32(120000)
	doc := model.Alert{Kind: model.AlertDocument, State: model.AlertExpiring, Type: model.DocInspection, DueDate: &date, DaysLeft: &three}
	if got := ReminderItemText(i18n.LocaleTR, doc); got != "Muayene: 3 gün kaldı (17.10.2026)" {
		t.Errorf("tr document = %q", got)
	}
	doc.DaysLeft, doc.State = &past, model.AlertOverdue
	if got := ReminderItemText(i18n.LocaleEN, doc); got != "Inspection: expired (2026-10-17)" {
		t.Errorf("en expired = %q", got)
	}
	plan := model.Alert{Kind: model.AlertMaintenance, State: model.AlertOverdue, Type: "periodic", DueKm: &km}
	if got := ReminderItemText(i18n.LocaleTR, plan); got != "Periyodik bakımı gecikti (120000 km)" {
		t.Errorf("tr maintenance = %q", got)
	}
}

// --- database-backed (TEST_DATABASE_URL) --------------------------------------------

// memDriver adapts the in-memory store to storage.Driver (the methods the
// use case calls).
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

type captured struct {
	mu  sync.Mutex
	ins []notifmodel.EnqueueInput
}

func (c *captured) Enqueue(_ context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ins = append(c.ins, in)
	return nil, nil
}

func (c *captured) take() []notifmodel.EnqueueInput {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.ins
	c.ins = nil
	return out
}

type fixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	q        *db.Queries
	vehicles *vehicleusecase.Service
	svc      *Service
	store    *storage.Memory
	notifier *captured
	today    time.Time
	org      db.Organization
	other    db.Organization
	// Branch A ("Merkez") and B of org; vehicle vA lives in A, vB in B,
	// vOther in the other organization.
	branchA, branchB db.Branch
	vA, vB, vOther   uuid.UUID
	// alice: owner (all branches); carol: branch manager selected [B];
	// dave: sales (read only).
	alice, carol, dave db.User
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
	f := &fixture{ctx: ctx, pool: pool, q: q, store: storage.NewMemory(), notifier: &captured{}}
	f.today = vehiclemodel.Today(time.Now())
	newUser := func(name, locale string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-vr-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test",
			Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE users SET locale = $2 WHERE id = $1`, u.ID, locale); err != nil {
			t.Fatal(err)
		}
		return u
	}
	f.alice, f.carol, f.dave = newUser("alice", "tr"), newUser("carol", "en"), newUser("dave", "tr")
	owner2 := newUser("erin", "tr")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Records " + suffix, OwnerUserID: f.alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Other " + suffix, OwnerUserID: owner2.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.org, err = q.GetOrganizationByID(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.other, err = q.GetOrganizationByID(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, other.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{f.alice.ID, f.carol.ID, f.dave.ID, owner2.ID})
	})
	if f.branchA, err = q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: org.ID, Code: "MRK"}); err != nil {
		t.Fatal(err)
	}
	if f.branchB, err = q.CreateBranch(ctx, db.CreateBranchParams{
		OrganizationID: org.ID, Code: "IST", Name: "İstanbul", WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
		PrepMinutes: 60, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	carol, err := orgSvc.AddMember(ctx, org.UUID, f.carol.Uuid, "sube_yoneticisi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_members SET branch_scope = 'selected' WHERE id = $1`, carol.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_member_branches (organization_id, member_id, branch_id)
		SELECT organization_id, id, $2 FROM organization_members WHERE id = $1`, carol.ID, f.branchB.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := orgSvc.AddMember(ctx, org.UUID, f.dave.Uuid, "rezervasyon_satis"); err != nil {
		t.Fatal(err)
	}

	f.vehicles = vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	f.svc = New(repository.NewPostgres(q), f.vehicles, memDriver{m: f.store}, f.notifier, nil)
	vehicle := func(orgID int64, plate string, branch uuid.UUID) uuid.UUID {
		v, err := f.vehicles.Create(ctx, orgID, vehiclemodel.VehicleInput{
			Plate: vehiclemodel.Str(plate), ManualEntry: boolPtr(true), ManualBrand: vehiclemodel.Str("Fiat"),
			ManualModel: vehiclemodel.Str("Egea"), ModelYear: int16Ptr(2024), HomeBranchUUID: &branch,
			OdometerKm: int32Ptr(10000),
		})
		if err != nil {
			t.Fatal(err)
		}
		return v.UUID
	}
	f.vA = vehicle(org.ID, "34 VR "+strings.ToUpper(suffix[:3]), f.branchA.Uuid)
	f.vB = vehicle(org.ID, "06 VR "+strings.ToUpper(suffix[:3]), f.branchB.Uuid)
	otherBranch, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: other.ID, Code: "MRK"})
	if err != nil {
		t.Fatal(err)
	}
	f.vOther = vehicle(other.ID, "35 VR "+strings.ToUpper(suffix[:3]), otherBranch.Uuid)
	return f
}

func boolPtr(v bool) *bool    { return &v }
func int16Ptr(v int16) *int16 { return &v }
func int32Ptr(v int32) *int32 { return &v }

func (f *fixture) day(offset int) string {
	return f.today.AddDate(0, 0, offset).Format("2006-01-02")
}

func (f *fixture) doc(t *testing.T, ctx context.Context, orgID int64, vehicle uuid.UUID, docType string, until int) model.Document {
	t.Helper()
	res, err := f.svc.CreateDocument(ctx, orgID, vehicle, model.DocumentInput{
		DocType: vehiclemodel.Str(docType), DocumentNumber: vehiclemodel.Str("N-" + docType),
		ValidUntil: vehiclemodel.String(f.day(until)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.Document
}

// branchCtx scopes ctx to a member restricted to the given branches.
func (f *fixture) branchCtx(branches ...int64) context.Context {
	return orgctx.WithScope(f.ctx, orgctx.Scope{ID: f.org.ID, UUID: f.org.Uuid, Slug: f.org.Slug, BranchIDs: branches})
}

func TestDocumentsOrganizationIsolationAndBranchScope(t *testing.T) {
	f := newFixture(t)
	d := f.doc(t, f.ctx, f.org.ID, f.vA, model.DocInspection, 10)
	f.doc(t, f.ctx, f.org.ID, f.vB, model.DocInspection, 5)

	// Another organization: the vehicle and its documents do not exist.
	if _, err := f.svc.ListDocuments(f.ctx, f.other.ID, f.vA); !errors.Is(err, ErrVehicleNotFound) {
		t.Fatalf("other org list = %v", err)
	}
	if _, err := f.svc.PatchDocument(f.ctx, f.other.ID, f.vA, d.UUID, model.DocumentInput{}); !errors.Is(err, ErrVehicleNotFound) {
		t.Fatalf("other org patch = %v", err)
	}
	if _, err := f.svc.CreateDocument(f.ctx, f.org.ID, f.vOther, model.DocumentInput{DocType: vehiclemodel.Str("kasko")}); !errors.Is(err, ErrVehicleNotFound) {
		t.Fatalf("create on other org vehicle = %v", err)
	}
	// A document through another vehicle of the same organization.
	if _, err := f.svc.PatchDocument(f.ctx, f.org.ID, f.vB, d.UUID, model.DocumentInput{}); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("document via another vehicle = %v", err)
	}
	if _, err := f.svc.DeleteDocument(f.ctx, f.org.ID, f.vB, d.UUID); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("delete via another vehicle = %v", err)
	}

	// Branch scope: carol-like scope [B] does not see vehicle A anywhere.
	scoped := f.branchCtx(f.branchB.ID)
	if _, err := f.svc.ListDocuments(scoped, f.org.ID, f.vA); !errors.Is(err, ErrVehicleNotFound) {
		t.Fatalf("out of scope list = %v", err)
	}
	if _, err := f.svc.CreateMaintenance(scoped, f.org.ID, f.vA, model.MaintenanceInput{
		Kind: vehiclemodel.Str("planned"), MaintenanceType: vehiclemodel.Str("oil"), PlannedKm: vehiclemodel.Int(20000),
	}); !errors.Is(err, ErrVehicleNotFound) {
		t.Fatalf("out of scope maintenance = %v", err)
	}
	if _, err := f.svc.OpenDocumentFile(scoped, f.org.ID, f.vA, d.UUID); !errors.Is(err, ErrVehicleNotFound) {
		t.Fatalf("out of scope download = %v", err)
	}
	if docs, err := f.svc.ListDocuments(scoped, f.org.ID, f.vB); err != nil || len(docs) != 1 {
		t.Fatalf("in scope list = %v %v", docs, err)
	}
	view, err := f.svc.Alerts(scoped, f.org.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range view.Items {
		if a.Vehicle.UUID != f.vB {
			t.Fatalf("scoped alerts leak %s", a.Vehicle.Plate)
		}
	}
	if len(view.Items) != 1 {
		t.Fatalf("scoped alerts = %+v", view.Items)
	}
	rows, err := f.svc.ExportDocuments(scoped, f.org.ID, model.ExportFilter{})
	if err != nil || len(rows) != 1 || rows[0].Document.DocType != model.DocInspection {
		t.Fatalf("scoped export = %+v %v", rows, err)
	}
	// Export in the worker context (organization only, no member scope).
	all, err := f.svc.ExportDocuments(orgctx.WithOrganizationID(f.ctx, f.org.ID), f.org.ID, model.ExportFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("org export = %d %v", len(all), err)
	}
	byVehicle, err := f.svc.ExportDocuments(f.ctx, f.org.ID, model.ExportFilterFromQuery(map[string]string{"vehicle": f.vA.String()}))
	if err != nil || len(byVehicle) != 1 {
		t.Fatalf("export by vehicle = %d %v", len(byVehicle), err)
	}
	if bad, _ := f.svc.ExportDocuments(f.ctx, f.org.ID, model.ExportFilterFromQuery(map[string]string{"vehicle": "nope"})); len(bad) != 0 {
		t.Fatalf("invalid vehicle filter exported %d rows", len(bad))
	}
}

func TestExpiryComputation(t *testing.T) {
	f := newFixture(t)
	old := f.doc(t, f.ctx, f.org.ID, f.vA, model.DocInspection, -300)
	current := f.doc(t, f.ctx, f.org.ID, f.vA, model.DocInspection, 10)
	f.doc(t, f.ctx, f.org.ID, f.vA, model.DocTrafficIns, 200)
	f.doc(t, f.ctx, f.org.ID, f.vB, model.DocKasko, -3)
	if _, err := f.svc.CreateDocument(f.ctx, f.org.ID, f.vB, model.DocumentInput{DocType: vehiclemodel.Str("ruhsat")}); err != nil {
		t.Fatal(err)
	}

	docs, err := f.svc.ListDocuments(f.ctx, f.org.ID, f.vA)
	if err != nil {
		t.Fatal(err)
	}
	status := map[uuid.UUID]model.Document{}
	for _, d := range docs {
		status[d.UUID] = d
	}
	if d := status[current.UUID]; !d.IsCurrent || d.Status != vehiclemodel.ExpiryExpiring || d.DaysLeft == nil || *d.DaysLeft != 10 {
		t.Fatalf("current inspection = %+v", d)
	}
	if d := status[old.UUID]; d.IsCurrent || d.Status != vehiclemodel.ExpiryExpired {
		t.Fatalf("old inspection = %+v", d)
	}

	// Vehicle projection: current documents and the next one to renew.
	vA, err := f.vehicles.Get(f.ctx, f.org.ID, f.vA)
	if err != nil {
		t.Fatal(err)
	}
	if vA.InspectionUntil == nil || *vA.InspectionUntil != f.day(10) || vA.InsuranceUntil == nil || *vA.InsuranceUntil != f.day(200) ||
		vA.KaskoUntil != nil || vA.DocumentStatus != vehiclemodel.ExpiryExpiring || *vA.DocumentExpiresAt != f.day(10) {
		t.Fatalf("vehicle A expiry = %+v", vA)
	}
	vB, err := f.vehicles.Get(f.ctx, f.org.ID, f.vB)
	if err != nil {
		t.Fatal(err)
	}
	if vB.DocumentStatus != vehiclemodel.ExpiryExpired || vB.KaskoUntil == nil || vB.MaintenanceStatus != vehiclemodel.MaintenanceNone {
		t.Fatalf("vehicle B expiry = %+v", vB)
	}

	// Renewing the inspection makes vehicle A valid again (latest wins).
	if _, err := f.svc.PatchDocument(f.ctx, f.org.ID, f.vA, current.UUID, model.DocumentInput{
		ValidUntil: vehiclemodel.String(f.day(400)),
	}); err != nil {
		t.Fatal(err)
	}
	if vA, _ = f.vehicles.Get(f.ctx, f.org.ID, f.vA); vA.DocumentStatus != vehiclemodel.ExpiryValid {
		t.Fatalf("after renewal = %s", vA.DocumentStatus)
	}

	// List filter, sort and facets.
	list, total, err := f.vehicles.List(f.ctx, f.org.ID, vehiclemodel.ListFilter{
		Limit: 10, DocumentStatuses: []string{vehiclemodel.ExpiryExpired, vehiclemodel.ExpiryExpiring},
	})
	if err != nil || total != 1 || list[0].UUID != f.vB {
		t.Fatalf("expired filter = %d %v", total, err)
	}
	list, _, err = f.vehicles.List(f.ctx, f.org.ID, vehiclemodel.ListFilter{Limit: 10, SortField: "document_expires_at"})
	if err != nil || len(list) != 2 || list[0].UUID != f.vB {
		t.Fatalf("sort by document_expires_at = %v", err)
	}
	facets, err := f.vehicles.Facets(f.ctx, f.org.ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, fc := range facets.DocumentStatus {
		counts[fc.Value] = fc.Count
	}
	if counts[vehiclemodel.ExpiryExpired] != 1 || counts[vehiclemodel.ExpiryValid] != 1 {
		t.Fatalf("document_status facets = %+v", facets.DocumentStatus)
	}

	// Maintenance: a km plan within 1000 km is due; completing it clears it.
	plan, err := f.svc.CreateMaintenance(f.ctx, f.org.ID, f.vA, model.MaintenanceInput{
		Kind: vehiclemodel.Str("planned"), MaintenanceType: vehiclemodel.Str("periodic"), PlannedKm: vehiclemodel.Int(10600),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Item.Status != vehiclemodel.MaintenanceDue || plan.Item.KmLeft == nil || *plan.Item.KmLeft != 600 {
		t.Fatalf("plan = %+v", plan.Item)
	}
	if vA, _ = f.vehicles.Get(f.ctx, f.org.ID, f.vA); vA.NextMaintenanceKm == nil || *vA.NextMaintenanceKm != 10600 ||
		vA.MaintenanceStatus != vehiclemodel.MaintenanceDue {
		t.Fatalf("vehicle next maintenance = %+v", vA)
	}
	done, err := f.svc.CompleteMaintenance(f.ctx, f.org.ID, f.vA, plan.Item.UUID, model.CompleteInput{
		DoneKm: vehiclemodel.Int(10550), Cost: vehiclemodel.String("2450,5"), ServiceProvider: vehiclemodel.Str("Yetkili servis"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if done.Item.Kind != model.KindDone || done.Item.DoneDate == nil || *done.Item.DoneDate != f.day(0) ||
		done.Item.Cost == nil || *done.Item.Cost != "2450.50" || done.Item.Currency != "TRY" || done.Item.PlannedKm == nil {
		t.Fatalf("completed = %+v", done.Item)
	}
	if _, err := f.svc.CompleteMaintenance(f.ctx, f.org.ID, f.vA, plan.Item.UUID, model.CompleteInput{}); err == nil {
		t.Fatal("completing twice must fail")
	}
	if vA, _ = f.vehicles.Get(f.ctx, f.org.ID, f.vA); vA.NextMaintenanceKm != nil || vA.MaintenanceStatus != vehiclemodel.MaintenanceNone {
		t.Fatalf("after completion = %+v", vA)
	}
	items, err := f.svc.ListMaintenance(f.ctx, f.org.ID, f.vA)
	if err != nil || len(items) != 1 {
		t.Fatalf("one row after completion: %d %v", len(items), err)
	}

	// Validation.
	var ve *ValidationError
	if _, err := f.svc.CreateMaintenance(f.ctx, f.org.ID, f.vA, model.MaintenanceInput{
		Kind: vehiclemodel.Str("planned"), MaintenanceType: vehiclemodel.Str("oil"),
	}); !errors.As(err, &ve) || ve.Details[0].Field != "planned_date" {
		t.Fatalf("plan without target = %v", err)
	}
	if _, err := f.svc.CreateMaintenance(f.ctx, f.org.ID, f.vA, model.MaintenanceInput{
		Kind: vehiclemodel.Str("done"), MaintenanceType: vehiclemodel.Str("oil"), DoneDate: vehiclemodel.String(f.day(3)),
	}); !errors.As(err, &ve) || ve.Details[0].Code != "in_future" {
		t.Fatalf("done in the future = %v", err)
	}
	if _, err := f.svc.CreateMaintenance(f.ctx, f.org.ID, f.vA, model.MaintenanceInput{
		Kind: vehiclemodel.Str("done"), MaintenanceType: vehiclemodel.Str("oil"), DoneDate: vehiclemodel.String(f.day(0)),
		Cost: vehiclemodel.String("-5"),
	}); !errors.As(err, &ve) || ve.Details[0].Field != "cost" {
		t.Fatalf("negative cost = %v", err)
	}
	if _, err := f.svc.PatchMaintenance(f.ctx, f.org.ID, f.vA, plan.Item.UUID, model.MaintenanceInput{Kind: vehiclemodel.Str("planned")}); !errors.As(err, &ve) {
		t.Fatalf("kind change = %v", err)
	}
	if _, err := f.svc.CreateDocument(f.ctx, f.org.ID, f.vA, model.DocumentInput{
		DocType: vehiclemodel.Str("kasko"), ValidFrom: vehiclemodel.String(f.day(5)), ValidUntil: vehiclemodel.String(f.day(1)),
	}); !errors.As(err, &ve) || ve.Details[0].Field != "valid_until" {
		t.Fatalf("until before from = %v", err)
	}
	if _, err := f.svc.CreateDocument(f.ctx, f.org.ID, f.vA, model.DocumentInput{DocType: vehiclemodel.Str("passport")}); !errors.As(err, &ve) {
		t.Fatalf("unknown type = %v", err)
	}
}

func TestDocumentFiles(t *testing.T) {
	f := newFixture(t)
	d := f.doc(t, f.ctx, f.org.ID, f.vA, model.DocKasko, 100)
	upload := func(body []byte, name string) (DocumentChange, error) {
		return f.svc.UploadDocumentFile(f.ctx, f.org.ID, f.vA, d.UUID, FileUpload{
			Body: bytes.NewReader(body), Size: int64(len(body)), Filename: name,
		})
	}
	var ve *ValidationError
	if _, err := upload([]byte("<html><body>not a pdf</body></html>"), "kasko.pdf"); !errors.As(err, &ve) || ve.Details[0].Code != "invalid_type" {
		t.Fatalf("html upload = %v", err)
	}
	if _, err := f.svc.UploadDocumentFile(f.ctx, f.org.ID, f.vA, d.UUID, FileUpload{
		Body: bytes.NewReader([]byte("%PDF-1.4")), Size: MaxDocumentFileBytes + 1, Filename: "big.pdf",
	}); !errors.As(err, &ve) || ve.Details[0].Code != "too_large" {
		t.Fatalf("too large = %v", err)
	}
	if _, err := f.svc.OpenDocumentFile(f.ctx, f.org.ID, f.vA, d.UUID); !errors.Is(err, ErrNoFile) {
		t.Fatalf("download without file = %v", err)
	}
	pdf := []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer\n%%EOF")
	res, err := upload(pdf, "../kasko poliçe.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if res.Document.File == nil || res.Document.File.ContentType != "application/pdf" || res.Document.File.Name != "kasko poliçe.pdf" {
		t.Fatalf("file = %+v", res.Document.File)
	}
	var firstKey string
	if err := f.pool.QueryRow(f.ctx, `SELECT file_object_key FROM vehicle_documents WHERE uuid = $1`, d.UUID).Scan(&firstKey); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(firstKey, "organizations/"+f.org.Uuid.String()+"/vehicles/"+f.vA.String()+"/documents/"+d.UUID.String()+"/") {
		t.Fatalf("object key = %s", firstKey)
	}
	file, err := f.svc.OpenDocumentFile(f.ctx, f.org.ID, f.vA, d.UUID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(file.Body)
	_ = file.Body.Close()
	if !bytes.Equal(got, pdf) || file.ContentType != "application/pdf" {
		t.Fatalf("downloaded %q (%s)", got, file.ContentType)
	}
	// Replacing the file removes the old object.
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if _, err := upload(png, "scan.png"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := f.store.Exists(f.ctx, firstKey); ok {
		t.Fatal("replaced object still stored")
	}
	// Another organization cannot read it.
	if _, err := f.svc.OpenDocumentFile(f.ctx, f.other.ID, f.vA, d.UUID); !errors.Is(err, ErrVehicleNotFound) {
		t.Fatalf("other org download = %v", err)
	}
}

func TestReminderJobRecipientsAndIdempotency(t *testing.T) {
	f := newFixture(t)
	f.doc(t, f.ctx, f.org.ID, f.vA, model.DocInspection, 12)
	f.doc(t, f.ctx, f.org.ID, f.vB, model.DocTrafficIns, -2)
	if _, err := f.svc.CreateMaintenance(f.ctx, f.org.ID, f.vB, model.MaintenanceInput{
		Kind: vehiclemodel.Str("planned"), MaintenanceType: vehiclemodel.Str("tire"), PlannedDate: vehiclemodel.String(f.day(9)),
	}); err != nil {
		t.Fatal(err)
	}
	// Far from due: no reminder.
	f.doc(t, f.ctx, f.org.ID, f.vA, model.DocKasko, 200)
	// The other organization has its own alert.
	f.doc(t, f.ctx, f.other.ID, f.vOther, model.DocInspection, 1)

	orgRow := db.ListVehicleReminderOrganizationsRow{ID: f.org.ID, Uuid: f.org.Uuid, Slug: f.org.Slug}
	stats, err := f.svc.RemindOrganization(f.ctx, orgRow, f.today)
	if err != nil {
		t.Fatal(err)
	}
	sent := f.notifier.take()
	if stats.Subjects != 3 || stats.Notifications != 3 || len(sent) != 3 {
		t.Fatalf("first run: stats=%+v sent=%d", stats, len(sent))
	}
	byUser := map[int64][]notifmodel.EnqueueInput{}
	for _, in := range sent {
		if in.OrganizationID == nil || *in.OrganizationID != f.org.ID || in.TemplateCode != ReminderTemplate {
			t.Fatalf("notification not organization-stamped: %+v", in)
		}
		byUser[*in.UserID] = append(byUser[*in.UserID], in)
	}
	// alice (all branches): both vehicles; carol (selected [B]): vehicle B
	// only; dave (read only): nothing.
	if len(byUser[f.alice.ID]) != 2 || len(byUser[f.carol.ID]) != 1 || len(byUser[f.dave.ID]) != 0 {
		t.Fatalf("recipients: alice=%d carol=%d dave=%d", len(byUser[f.alice.ID]), len(byUser[f.carol.ID]), len(byUser[f.dave.ID]))
	}
	carolNote := byUser[f.carol.ID][0]
	wantLink := "/t/" + f.org.Slug + "/fleet/vehicles/" + f.vB.String() + "?tab=documents"
	if carolNote.ActionURL == nil || *carolNote.ActionURL != wantLink || carolNote.Language != "en" ||
		!strings.Contains(carolNote.TemplateVars["items"], "Traffic insurance: expired") ||
		!strings.Contains(carolNote.TemplateVars["items"], "Tires maintenance is due") {
		t.Fatalf("carol notification = %+v", carolNote)
	}

	// Same day again: idempotent.
	if stats, err = f.svc.RemindOrganization(f.ctx, orgRow, f.today); err != nil || stats.Notifications != 0 || len(f.notifier.take()) != 0 {
		t.Fatalf("second run: %+v %v", stats, err)
	}
	// Next day: nothing new (no state change, no milestone).
	if stats, err = f.svc.RemindOrganization(f.ctx, orgRow, f.today.AddDate(0, 0, 1)); err != nil || stats.Subjects != 0 {
		t.Fatalf("next day: %+v %v", stats, err)
	}
	// Day 5: the inspection reaches its 7-day milestone (12 - 5).
	if stats, err = f.svc.RemindOrganization(f.ctx, orgRow, f.today.AddDate(0, 0, 5)); err != nil || stats.Subjects != 1 {
		t.Fatalf("milestone day: %+v %v", stats, err)
	}
	sent = f.notifier.take()
	if len(sent) != 1 || *sent[0].UserID != f.alice.ID {
		t.Fatalf("milestone recipients = %+v", sent)
	}
	var recipients int32
	if err := f.pool.QueryRow(f.ctx, `SELECT recipients FROM vehicle_reminder_deliveries
		WHERE organization_id = $1 AND reminder_date = $2 AND subject_kind = 'document'`,
		f.org.ID, f.today.AddDate(0, 0, 5)).Scan(&recipients); err != nil || recipients != 1 {
		t.Fatalf("delivery log recipients = %d %v", recipients, err)
	}
	// The other organization was never touched by org A's run.
	var otherRows int
	if err := f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM vehicle_reminder_deliveries WHERE organization_id = $1`, f.other.ID).Scan(&otherRows); err != nil || otherRows != 0 {
		t.Fatalf("other organization deliveries = %d %v", otherRows, err)
	}
}
