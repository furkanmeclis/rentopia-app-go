package vehicles_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/adapters"
	vehiclehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/handler"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

type publication struct {
	channel string
	event   resourceevents.Event
}

type fakeCentrifugo struct {
	mu   sync.Mutex
	pubs []publication
}

func (f *fakeCentrifugo) Publish(_ context.Context, channel string, data any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev, _ := data.(resourceevents.Event)
	f.pubs = append(f.pubs, publication{channel: channel, event: ev})
	return nil
}

func (f *fakeCentrifugo) take() []publication {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.pubs
	f.pubs = nil
	return out
}

// TestTenantVehiclesEndToEnd drives /v1/tenant/vehicles over HTTP with the
// real organization resolver, SQL, bulk engine and live events: mutations
// publish on the organization channel, a branch-restricted member gets 404
// outside its branches, an ineligible group answers 422 NOT_ELIGIBLE and a
// parameterised bulk action changes the status. Runs with TEST_DATABASE_URL.
func TestTenantVehiclesEndToEnd(t *testing.T) {
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
			Email: name + "-veh-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, carol, bob := newUser("alice"), newUser("carol"), newUser("bob")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Vehicles " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, carol.ID, bob.ID})
	})
	// Branch B for carol (branch manager, selected [B]); "Merkez" is A.
	var branchA, branchB db.Branch
	if branchA, err = q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: org.ID, Code: "MRK"}); err != nil {
		t.Fatal(err)
	}
	if branchB, err = q.CreateBranch(ctx, db.CreateBranchParams{
		OrganizationID: org.ID, Code: "IST", Name: "İstanbul", WorkingHours: []byte("{}"), HolidayExceptions: []byte("[]"),
		PrepMinutes: 60, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	member, err := orgSvc.AddMember(ctx, org.UUID, carol.Uuid, "sube_yoneticisi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE organization_members SET branch_scope = 'selected' WHERE id = $1`, member.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO organization_member_branches (organization_id, member_id, branch_id)
		SELECT organization_id, id, $2 FROM organization_members WHERE id = $1`, member.ID, branchB.ID); err != nil {
		t.Fatal(err)
	}

	fake := &fakeCentrifugo{}
	events := resourceevents.New(fake, nil)
	events.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(q)))
	svc := vehicleusecase.New(vehiclerepository.NewPostgres(pool, q), nil)
	rec := activity.NewRecorder(q, nil)
	bulkSvc := bulkusecase.New(q, bulkengine.NewRegistry(adapters.NewBulk(svc)), nil, nil, rec, config.BulkConfig{}, nil)
	bulkSvc.SetResourceEvents(events)
	h := vehiclehandler.New(svc, rec)
	h.SetResourceEvents(events)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	loader := mapLoader{alice.Uuid: alice.ID, carol.Uuid: carol.ID, bob.Uuid: bob.ID}
	vehicles.RegisterRoutes(mux, h, vehicles.EngineHandlers{
		Bulk: bulkhandler.New(bulkSvc).TenantExecute(vehicles.Resource),
	}, tokens, loader, orgSvc)

	call := func(user db.User, method, path string, body any) (int, map[string]any) {
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
		req.Header.Set(orgctx.HeaderName, org.Slug)
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
	wantChannel := tenantmodule.ResourceChannel(org.UUID, vehicles.Resource)
	expect := func(step, action string, id string) {
		t.Helper()
		events.Wait()
		pubs := fake.take()
		if len(pubs) != 1 || pubs[0].channel != wantChannel || pubs[0].event.Action != action ||
			len(pubs[0].event.UUIDs) == 0 || pubs[0].event.UUIDs[0] != id {
			t.Fatalf("%s: publications = %+v", step, pubs)
		}
	}

	code, created := call(alice, http.MethodPost, "/v1/tenant/vehicles", map[string]any{
		"plate": "34 abc 123", "manual_entry": true, "manual_brand": "Fiat", "manual_model": "Egea",
		"model_year": 2024, "home_branch_uuid": branchA.Uuid, "transmission": "manual",
	})
	if code != http.StatusCreated || created["plate"] != "34 ABC 123" {
		t.Fatalf("create = %d %v", code, created)
	}
	id := created["uuid"].(string)
	expect("create", resourceevents.ActionCreated, id)

	if code, _ := call(alice, http.MethodPost, "/v1/tenant/vehicles", map[string]any{
		"plate": "34-ABC-123", "manual_entry": true, "manual_brand": "Fiat", "manual_model": "Egea",
		"model_year": 2024, "home_branch_uuid": branchA.Uuid,
	}); code != http.StatusConflict {
		t.Fatalf("duplicate plate = %d", code)
	}

	code, v := call(alice, http.MethodPost, "/v1/tenant/vehicles/"+id+"/status", map[string]any{
		"status": "maintenance", "odometer_km": 15000, "note": "Lastik",
	})
	if code != http.StatusOK || v["status"] != "maintenance" {
		t.Fatalf("status = %d %v", code, v)
	}
	expect("status", resourceevents.ActionUpdated, id)
	if code, page := call(alice, http.MethodGet, "/v1/tenant/vehicles/"+id+"/events", nil); code != http.StatusOK || page["total"].(float64) != 2 {
		t.Fatalf("events = %d %v", code, page)
	}

	// Ineligible group: 422 NOT_ELIGIBLE with the failed criteria.
	var segment int64
	var auto uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'EKO', 'Ekonomi') RETURNING id`, org.ID).Scan(&segment); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO rental_groups (organization_id, segment_id, code, name, transmission)
		VALUES ($1, $2, 'AUTO', 'Otomatik', 'automatic') RETURNING uuid`, org.ID, segment).Scan(&auto); err != nil {
		t.Fatal(err)
	}
	code, body := call(alice, http.MethodPost, "/v1/tenant/vehicles/"+id+"/rental-groups", map[string]any{"rental_group_uuid": auto})
	details, _ := body["details"].([]any)
	if code != http.StatusUnprocessableEntity || body["code"] != vehiclehandler.CodeNotEligible || len(details) != 1 ||
		details[0].(map[string]any)["code"] != "transmission_mismatch" {
		t.Fatalf("not eligible = %d %v", code, body)
	}
	fake.take()

	// Parameterised bulk: set_status with params; an invalid status is a 400.
	if code, _ := call(alice, http.MethodPost, "/v1/tenant/vehicles/bulk", map[string]any{
		"action": "set_status", "target": map[string]any{"scope": "ids", "ids": []string{id}, "params": map[string]string{"status": "nope"}},
	}); code != http.StatusBadRequest {
		t.Fatalf("bulk invalid param = %d", code)
	}
	if code, res := call(alice, http.MethodPost, "/v1/tenant/vehicles/bulk", map[string]any{
		"action": "set_status", "target": map[string]any{"scope": "ids", "ids": []string{id}, "params": map[string]string{"status": "available"}},
	}); code != http.StatusOK || res["summary"].(map[string]any)["succeeded"].(float64) != 1 {
		t.Fatalf("bulk = %d %v", code, res)
	}
	expect("bulk", resourceevents.ActionBulk, id)
	if _, v := call(alice, http.MethodGet, "/v1/tenant/vehicles/"+id, nil); v["status"] != "available" {
		t.Fatalf("after bulk = %v", v["status"])
	}

	// Carol (selected [IST]) does not see the Merkez vehicle.
	if code, _ := call(carol, http.MethodGet, "/v1/tenant/vehicles/"+id, nil); code != http.StatusNotFound {
		t.Fatalf("carol get = %d", code)
	}
	if code, page := call(carol, http.MethodGet, "/v1/tenant/vehicles", nil); code != http.StatusOK || page["total"].(float64) != 0 {
		t.Fatalf("carol list = %d %v", code, page)
	}
	// Moving it to İstanbul makes it visible (current branch in scope).
	if code, _ := call(alice, http.MethodPatch, "/v1/tenant/vehicles/"+id, map[string]any{"current_branch_uuid": branchB.Uuid}); code != http.StatusOK {
		t.Fatalf("move = %d", code)
	}
	expect("patch", resourceevents.ActionUpdated, id)
	if code, _ := call(carol, http.MethodGet, "/v1/tenant/vehicles/"+id, nil); code != http.StatusOK {
		t.Fatalf("carol get after move = %d", code)
	}
	// Bob is not a member.
	if code, _ := call(bob, http.MethodGet, "/v1/tenant/vehicles", nil); code != http.StatusForbidden {
		t.Fatalf("non-member = %d", code)
	}

	if code, _ := call(alice, http.MethodDelete, "/v1/tenant/vehicles/"+id, nil); code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
	expect("delete", resourceevents.ActionDeleted, id)

	// Subscription: members with tenant.vehicles.read only.
	reg := resourceevents.NewRegistry()
	vehicles.RegisterLiveResources(reg)
	rule := tenantmodule.ChannelRule(reg, orgSvc)
	allowed := func(user db.User) bool {
		t.Helper()
		pctx := authctx.WithPrincipal(ctx, authctx.Principal{UserID: user.Uuid, UserInternal: user.ID})
		handled, ok, err := rule(pctx, user.Uuid, false, wantChannel)
		if err != nil || !handled {
			t.Fatalf("rule: handled=%v err=%v", handled, err)
		}
		return ok
	}
	if !allowed(alice) || !allowed(carol) {
		t.Fatal("members with tenant.vehicles.read denied")
	}
	if allowed(bob) {
		t.Fatal("non-member allowed")
	}
}
