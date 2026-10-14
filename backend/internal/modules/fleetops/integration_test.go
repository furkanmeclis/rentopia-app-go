package fleetops_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops"
	fleetopsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/adapters"
	fleetopshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/handler"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	vehicleadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/adapters"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

type fakeCentrifugo struct {
	mu       sync.Mutex
	channels []string
}

func (f *fakeCentrifugo) Publish(_ context.Context, channel string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.channels = append(f.channels, channel)
	return nil
}

func (f *fakeCentrifugo) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.channels
	f.channels = nil
	return out
}

// TestFleetOpsEndToEnd drives blocks, transfers and one-way rules over HTTP
// with the real organization resolver and SQL: the EXCLUDE constraints,
// open-ended blocks and "Engeli kapat", the block / transfer 409s, the
// transfer lifecycle moving the vehicle (status + current branch, vehicle
// events), LocationAt, the busy intervals view, product-specific one-way
// fees, organization isolation, branch scope, live channels and the
// "Bakıma al" bulk action with rollback. Runs with TEST_DATABASE_URL.
func TestFleetOpsEndToEnd(t *testing.T) {
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
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-fops-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, carol, dave := newUser("alice"), newUser("carol"), newUser("dave")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "FleetOps " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Other " + suffix, OwnerUserID: dave.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", []int64{org.ID, other.ID})
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, carol.ID, dave.ID})
	})
	branchA, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: org.ID, Code: "MRK"})
	if err != nil {
		t.Fatal(err)
	}
	newBranch := func(code, name string) db.Branch {
		b, err := q.CreateBranch(ctx, db.CreateBranchParams{
			OrganizationID: org.ID, Code: code, Name: name, WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
			PrepMinutes: 60, IsActive: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	branchB, branchC := newBranch("IST", "İstanbul"), newBranch("ANK", "Ankara")
	// Carol: branch manager restricted to Ankara.
	member, err := orgSvc.AddMember(ctx, org.UUID, carol.Uuid, "sube_yoneticisi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_members SET branch_scope = 'selected' WHERE id = $1`, member.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_member_branches (organization_id, member_id, branch_id)
		SELECT organization_id, id, $2 FROM organization_members WHERE id = $1`, member.ID, branchC.ID); err != nil {
		t.Fatal(err)
	}

	vehicleSvc := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	plate, year, manual, brand, mdl := "34 FOP 1", int16(2024), true, "Fiat", "Egea"
	vehicle, err := vehicleSvc.Create(ctx, org.ID, vehiclemodel.VehicleInput{
		Plate: &plate, ManualEntry: &manual, ManualBrand: &brand, ManualModel: &mdl, ModelYear: &year,
		HomeBranchUUID: &branchA.Uuid,
	})
	if err != nil {
		t.Fatal(err)
	}
	vid := vehicle.UUID.String()

	fake := &fakeCentrifugo{}
	events := resourceevents.New(fake, nil)
	events.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(q)))
	svc := fleetopsusecase.New(fleetopsrepository.NewPostgres(pool, q), vehicleSvc)
	h := fleetopshandler.New(svc, activity.NewRecorder(q, nil))
	h.SetResourceEvents(events)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	fleetops.RegisterRoutes(mux, h, fleetops.EngineHandlers{}, tokens,
		mapLoader{alice.Uuid: alice.ID, carol.Uuid: carol.ID, dave.Uuid: dave.ID}, orgSvc)

	callIn := func(slug string, user db.User, method, path string, body any) (int, map[string]any) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, slug)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		if data, ok := env["data"].(map[string]any); ok {
			return rr.Code, data
		}
		errBody, _ := env["error"].(map[string]any)
		return rr.Code, errBody
	}
	call := func(user db.User, method, path string, body any) (int, map[string]any) {
		t.Helper()
		return callIn(org.Slug, user, method, path, body)
	}
	firstDetailCode := func(body map[string]any) string {
		details, _ := body["details"].([]any)
		if len(details) == 0 {
			return ""
		}
		code, _ := details[0].(map[string]any)["code"].(string)
		return code
	}
	vehicleStatus := func() (string, uuid.UUID) {
		t.Helper()
		v, err := vehicleSvc.Get(ctx, org.ID, vehicle.UUID)
		if err != nil {
			t.Fatal(err)
		}
		return v.Status, v.CurrentBranch.UUID
	}
	blocks := "/v1/tenant/vehicles/" + vid + "/blocks"
	now := time.Now().UTC()
	at := func(hours float64) string {
		return now.Add(time.Duration(hours * float64(time.Hour))).Format(time.RFC3339)
	}

	// --- Blocks -----------------------------------------------------------
	// "Bakıma al": open-ended maintenance from now, status synced.
	code, res := call(alice, http.MethodPost, blocks, map[string]any{"kind": "maintenance", "reason": "Periyodik", "sync_status": true})
	if code != http.StatusCreated || res["vehicle_status"] != "maintenance" || res["status_changed"] != true {
		t.Fatalf("bakıma al = %d %v", code, res)
	}
	block := res["block"].(map[string]any)
	if block["ends_at"] != nil || block["state"] != "active" {
		t.Fatalf("open-ended block = %v", block)
	}
	blockID := block["uuid"].(string)
	if chans := (func() []string { events.Wait(); return fake.take() })(); !contains(chans, tenantmodule.ResourceChannel(org.UUID, fleetops.ResourceBlocks)) ||
		!contains(chans, tenantmodule.ResourceChannel(org.UUID, "tenant.vehicles")) {
		t.Fatalf("block publications = %v", chans)
	}
	// A second block overlapping the open-ended one: 409 with the block.
	code, res = call(alice, http.MethodPost, blocks, map[string]any{"kind": "damage", "starts_at": at(24 * 30)})
	if code != http.StatusConflict || firstDetailCode(res) != fleetopsusecase.ConflictBlockOverlap {
		t.Fatalf("overlapping block = %d %v", code, res)
	}
	// The EXCLUDE constraint holds without the use case too.
	_, err = pool.Exec(ctx, `INSERT INTO vehicle_blocks (organization_id, vehicle_id, kind, during)
		SELECT organization_id, id, 'other', tstzrange(NOW() + interval '1 day', NOW() + interval '2 days', '[)')
		FROM vehicles WHERE uuid = $1`, vehicle.UUID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23P01" {
		t.Fatalf("exclude constraint = %v", err)
	}
	// A transfer during the open-ended block: 409 block_overlap.
	code, res = call(alice, http.MethodPost, "/v1/tenant/transfers", map[string]any{
		"vehicle_uuid": vid, "to_branch_uuid": branchB.Uuid, "planned_departure": at(2), "planned_arrival": at(4),
	})
	if code != http.StatusConflict || firstDetailCode(res) != fleetopsusecase.ConflictBlockOverlap {
		t.Fatalf("transfer over block = %d %v", code, res)
	}
	// "Engeli kapat" with the status: back to available.
	time.Sleep(5 * time.Millisecond)
	code, res = call(alice, http.MethodPost, blocks+"/"+blockID+"/release", map[string]any{"sync_status": true})
	if code != http.StatusOK || res["block"].(map[string]any)["state"] != "ended" || res["vehicle_status"] != "available" {
		t.Fatalf("release = %d %v", code, res)
	}
	if code, _ := call(alice, http.MethodPost, blocks+"/"+blockID+"/release", nil); code != http.StatusBadRequest {
		t.Fatalf("release twice = %d", code)
	}
	if code, page := call(alice, http.MethodGet, blocks+"?state=active", nil); code != http.StatusOK || page["total"].(float64) != 0 {
		t.Fatalf("active blocks = %d %v", code, page)
	}
	if code, page := call(alice, http.MethodGet, blocks+"?state=ended", nil); code != http.StatusOK || page["total"].(float64) != 1 {
		t.Fatalf("ended blocks = %d %v", code, page)
	}

	// --- Transfers --------------------------------------------------------
	// From defaults to where the vehicle is at the departure (Merkez).
	code, tr := call(alice, http.MethodPost, "/v1/tenant/transfers", map[string]any{
		"vehicle_uuid": vid, "to_branch_uuid": branchB.Uuid, "planned_departure": at(2), "planned_arrival": at(4),
		"cost": "1250.50", "responsible_user_uuid": carol.Uuid,
	})
	if code != http.StatusCreated || tr["from_branch"].(map[string]any)["code"] != "MRK" || tr["cost"] != "1250.50" ||
		tr["status"] != "planned" || tr["responsible"] == nil {
		t.Fatalf("create transfer = %d %v", code, tr)
	}
	trID := tr["uuid"].(string)
	events.Wait()
	if chans := fake.take(); !contains(chans, tenantmodule.ResourceChannel(org.UUID, fleetops.ResourceTransfers)) {
		t.Fatalf("transfer publications = %v", chans)
	}
	// Overlapping open transfer: 409 transfer_overlap.
	code, res = call(alice, http.MethodPost, "/v1/tenant/transfers", map[string]any{
		"vehicle_uuid": vid, "to_branch_uuid": branchC.Uuid, "planned_departure": at(3), "planned_arrival": at(5),
	})
	if code != http.StatusConflict || firstDetailCode(res) != fleetopsusecase.ConflictTransferOverlap {
		t.Fatalf("overlapping transfer = %d %v", code, res)
	}
	// LocationAt: after the planned arrival the vehicle is at İstanbul, so
	// a later transfer leaves from there; Ankara is refused.
	code, res = call(alice, http.MethodPost, "/v1/tenant/transfers", map[string]any{
		"vehicle_uuid": vid, "from_branch_uuid": branchC.Uuid, "to_branch_uuid": branchA.Uuid,
		"planned_departure": at(10), "planned_arrival": at(12),
	})
	if code != http.StatusBadRequest || firstDetailCode(res) != "not_at_branch" {
		t.Fatalf("wrong origin = %d %v", code, res)
	}
	code, tr2 := call(alice, http.MethodPost, "/v1/tenant/transfers", map[string]any{
		"vehicle_uuid": vid, "to_branch_uuid": branchA.Uuid, "planned_departure": at(10), "planned_arrival": at(12),
	})
	if code != http.StatusCreated || tr2["from_branch"].(map[string]any)["code"] != "IST" {
		t.Fatalf("chained transfer = %d %v", code, tr2)
	}
	if code, c := call(alice, http.MethodPost, "/v1/tenant/transfers/"+tr2["uuid"].(string)+"/cancel", map[string]any{"note": "gerek yok"}); code != http.StatusOK || c["status"] != "cancelled" {
		t.Fatalf("cancel = %d %v", code, c)
	}
	// A block over the planned transfer is allowed and reports it.
	code, res = call(alice, http.MethodPost, blocks, map[string]any{"kind": "cleaning", "starts_at": at(3), "ends_at": at(3.5)})
	affected, _ := res["affected"].([]any)
	if code != http.StatusCreated || len(affected) != 1 || affected[0].(map[string]any)["kind"] != "transfer" ||
		affected[0].(map[string]any)["uuid"] != trID {
		t.Fatalf("block over transfer = %d %v", code, res)
	}
	cleaning := res["block"].(map[string]any)["uuid"].(string)
	// Busy intervals of the vehicle (one place for the engine): the
	// transfer ends at İstanbul; the released maintenance block (it ended
	// just now) and the cleaning block are there too.
	busy, err := q.ListVehicleBusyIntervals(ctx, db.ListVehicleBusyIntervalsParams{
		OrganizationID: org.ID, VehicleID: vehicleID(t, ctx, pool, vehicle.UUID),
		FromAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, b := range busy {
		kinds[b.Kind]++
		if b.Kind == "transfer" && (b.BranchAfterID.Int64 != branchB.ID || b.RefUuid.String() != trID) {
			t.Fatalf("busy transfer = %+v", b)
		}
	}
	if kinds["transfer"] != 1 || kinds["block"] != 2 {
		t.Fatalf("busy intervals = %+v", busy)
	}
	if code, _ := call(alice, http.MethodDelete, blocks+"/"+cleaning, nil); code != http.StatusOK {
		t.Fatalf("delete block = %d", code)
	}

	// Lifecycle: start -> in_transfer, complete -> at İstanbul, preparing.
	code, started := call(alice, http.MethodPost, "/v1/tenant/transfers/"+trID+"/start", nil)
	if code != http.StatusOK || started["status"] != "in_transit" || started["actual_departure"] == nil {
		t.Fatalf("start = %d %v", code, started)
	}
	if st, br := vehicleStatus(); st != "in_transfer" || br != branchA.Uuid {
		t.Fatalf("vehicle after start = %s %s", st, br)
	}
	departure, _ := time.Parse(time.RFC3339Nano, started["actual_departure"].(string))
	code, done := call(alice, http.MethodPost, "/v1/tenant/transfers/"+trID+"/complete", map[string]any{"arrival_status": "preparing"})
	if code != http.StatusOK || done["status"] != "completed" || done["actual_arrival"] == nil {
		t.Fatalf("complete = %d %v", code, done)
	}
	if st, br := vehicleStatus(); st != "preparing" || br != branchB.Uuid {
		t.Fatalf("vehicle after complete = %s %s", st, br)
	}
	if code, res := call(alice, http.MethodPost, "/v1/tenant/transfers/"+trID+"/start", nil); code != http.StatusConflict ||
		firstDetailCode(res) != fleetopsusecase.ConflictState {
		t.Fatalf("start a completed transfer = %d %v", code, res)
	}
	// created + maintenance + available + in_transfer + preparing.
	if _, total, err := vehicleSvc.Events(ctx, org.ID, vehicle.UUID, 20, 0); err != nil || total != 5 {
		t.Fatalf("vehicle events = %d %v", total, err)
	}
	// LocationAt: at İstanbul now, at Merkez before the departure.
	if loc, err := vehicleSvc.LocationAt(ctx, org.ID, vehicle.UUID, time.Now()); err != nil || loc.BranchID != branchB.ID || loc.InTransit {
		t.Fatalf("location now = %+v %v", loc, err)
	}
	if loc, err := vehicleSvc.LocationAt(ctx, org.ID, vehicle.UUID, departure.Add(-time.Second)); err != nil || loc.BranchID != branchA.ID {
		t.Fatalf("location before departure = %+v %v", loc, err)
	}

	// --- One-way rules ----------------------------------------------------
	code, rule := call(alice, http.MethodPost, "/v1/tenant/one-way-rules", map[string]any{
		"from_branch_uuid": branchA.Uuid, "to_branch_uuid": branchB.Uuid, "fee": 500,
	})
	if code != http.StatusCreated || rule["fee"] != "500.00" || rule["allowed"] != true || rule["rental_product"] != nil {
		t.Fatalf("create rule = %d %v", code, rule)
	}
	ruleID := rule["uuid"].(string)
	if code, res := call(alice, http.MethodPost, "/v1/tenant/one-way-rules", map[string]any{
		"from_branch_uuid": branchA.Uuid, "to_branch_uuid": branchB.Uuid, "fee": 100,
	}); code != http.StatusConflict || firstDetailCode(res) != "taken" {
		t.Fatalf("duplicate generic rule = %d %v", code, res)
	}
	var segment int64
	var product uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, org.ID).Scan(&segment); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id)
		VALUES ($1, 'EGEA', 'Fiat Egea veya benzeri', $2) RETURNING uuid`, org.ID, segment).Scan(&product); err != nil {
		t.Fatal(err)
	}
	if code, res := call(alice, http.MethodPost, "/v1/tenant/one-way-rules", map[string]any{
		"from_branch_uuid": branchA.Uuid, "to_branch_uuid": branchB.Uuid, "rental_product_uuid": product, "fee": "250",
	}); code != http.StatusCreated || res["rental_product"].(map[string]any)["code"] != "EGEA" {
		t.Fatalf("product rule = %d %v", code, res)
	}
	resolve := func(from, to uuid.UUID, extra string) map[string]any {
		t.Helper()
		code, res := call(alice, http.MethodGet, "/v1/tenant/one-way-rules/resolve?from_branch="+from.String()+"&to_branch="+to.String()+extra, nil)
		if code != http.StatusOK {
			t.Fatalf("resolve = %d %v", code, res)
		}
		return res
	}
	if r := resolve(branchA.Uuid, branchB.Uuid, "&rental_product="+product.String()); r["fee"] != "250.00" || r["product_specific"] != true {
		t.Fatalf("product quote = %v", r)
	}
	if r := resolve(branchA.Uuid, branchB.Uuid, ""); r["fee"] != "500.00" || r["allowed"] != true || r["product_specific"] != false {
		t.Fatalf("generic quote = %v", r)
	}
	if r := resolve(branchB.Uuid, branchA.Uuid, ""); r["allowed"] != false {
		t.Fatalf("no rule quote = %v", r)
	}

	// --- Branch scope (carol: Ankara only) ----------------------------------
	if code, _ := call(carol, http.MethodGet, "/v1/tenant/transfers/"+trID, nil); code != http.StatusNotFound {
		t.Fatalf("carol transfer = %d", code)
	}
	if code, page := call(carol, http.MethodGet, "/v1/tenant/transfers", nil); code != http.StatusOK || page["total"].(float64) != 0 {
		t.Fatalf("carol transfers = %d %v", code, page)
	}
	if code, _ := call(carol, http.MethodGet, blocks, nil); code != http.StatusNotFound {
		t.Fatalf("carol blocks = %d", code)
	}
	if code, res := call(carol, http.MethodPost, "/v1/tenant/one-way-rules", map[string]any{
		"from_branch_uuid": branchA.Uuid, "to_branch_uuid": branchC.Uuid,
	}); code != http.StatusBadRequest || firstDetailCode(res) != "not_found" {
		t.Fatalf("carol rule from Merkez = %d %v", code, res)
	}
	if code, _ := call(carol, http.MethodPost, "/v1/tenant/one-way-rules", map[string]any{
		"from_branch_uuid": branchC.Uuid, "to_branch_uuid": branchA.Uuid, "allowed": false,
	}); code != http.StatusCreated {
		t.Fatalf("carol rule from Ankara = %d", code)
	}
	if code, page := call(carol, http.MethodGet, "/v1/tenant/one-way-rules", nil); code != http.StatusOK || page["total"].(float64) != 1 {
		t.Fatalf("carol rules = %d %v", code, page)
	}
	if code, _ := call(carol, http.MethodGet, "/v1/tenant/one-way-rules/"+ruleID, nil); code != http.StatusNotFound {
		t.Fatalf("carol rule A->B = %d", code)
	}
	if code, page := call(alice, http.MethodGet, "/v1/tenant/one-way-rules", nil); code != http.StatusOK || page["total"].(float64) != 3 {
		t.Fatalf("alice rules = %d %v", code, page)
	}

	// --- Organization isolation (dave owns another organization) -------------
	for _, path := range []string{"/v1/tenant/transfers/" + trID, "/v1/tenant/one-way-rules/" + ruleID, blocks} {
		if code, _ := callIn(other.Slug, dave, http.MethodGet, path, nil); code != http.StatusNotFound {
			t.Fatalf("other organization %s = %d", path, code)
		}
	}
	if code, page := callIn(other.Slug, dave, http.MethodGet, "/v1/tenant/transfers", nil); code != http.StatusOK || page["total"].(float64) != 0 {
		t.Fatalf("other organization transfers = %d %v", code, page)
	}

	// --- "Bakıma al" bulk action with rollback -----------------------------
	bulk := vehicleadapters.NewBulk(vehicleSvc)
	bulk.SetMaintenance(fleetopsadapters.NewMaintenance(svc))
	jctx := bulkengine.WithParams(orgctx.WithOrganizationID(ctx, org.ID), map[string]string{"reason": "Toplu", "sync_status": "true"})
	item, err := bulk.ApplyItem(jctx, vehicleadapters.ActionStartMaintenance, vid)
	if err != nil || !item.OK || item.Previous["status"] != "preparing" {
		t.Fatalf("bulk bakıma al = %+v %v", item, err)
	}
	if st, _ := vehicleStatus(); st != "maintenance" {
		t.Fatalf("status after bulk = %s", st)
	}
	// A second run on the same vehicle fails that item only (overlap).
	if again, err := bulk.ApplyItem(jctx, vehicleadapters.ActionStartMaintenance, vid); err != nil || again.OK {
		t.Fatalf("bulk again = %+v %v", again, err)
	}
	prev := map[string]any{}
	raw, _ := json.Marshal(item.Previous)
	_ = json.Unmarshal(raw, &prev)
	if err := bulk.RevertItem(jctx, vehicleadapters.ActionStartMaintenance, vid, prev); err != nil {
		t.Fatal(err)
	}
	if st, _ := vehicleStatus(); st != "preparing" {
		t.Fatalf("status after rollback = %s", st)
	}
	if code, page := call(alice, http.MethodGet, blocks+"?state=active,scheduled", nil); code != http.StatusOK || page["total"].(float64) != 0 {
		t.Fatalf("blocks after rollback = %d %v", code, page)
	}
}

func vehicleID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) int64 {
	t.Helper()
	var out int64
	if err := pool.QueryRow(ctx, `SELECT id FROM vehicles WHERE uuid = $1`, id).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
