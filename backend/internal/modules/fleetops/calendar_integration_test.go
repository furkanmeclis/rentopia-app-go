package fleetops_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops"
	fleetopshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/handler"
	fleetopsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/repository"
	fleetopsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehiclerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestFleetCalendar drives GET /v1/tenant/fleet/calendar over HTTP with the
// real resolver and SQL: window validation, the vehicle page (plate order,
// window total), interval clipping against the window, transfer and
// maintenance details, the branch scope of a restricted member and
// organization isolation. Runs with TEST_DATABASE_URL.
func TestFleetCalendar(t *testing.T) {
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
			Email: name + "-fcal-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, carol, dave := newUser("alice"), newUser("carol"), newUser("dave")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Calendar " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Other Cal " + suffix, OwnerUserID: dave.ID})
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
	otherBranch, err := q.GetBranchByCode(ctx, db.GetBranchByCodeParams{OrganizationID: other.ID, Code: "MRK"})
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
	newVehicle := func(orgID int64, plate string, home uuid.UUID) vehiclemodel.Vehicle {
		year, manual, brand, mdl := int16(2024), true, "Fiat", "Egea"
		v, err := vehicleSvc.Create(ctx, orgID, vehiclemodel.VehicleInput{
			Plate: &plate, ManualEntry: &manual, ManualBrand: &brand, ManualModel: &mdl, ModelYear: &year,
			HomeBranchUUID: &home,
		})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v1 := newVehicle(org.ID, "34 CAL 1", branchA.Uuid)
	v2 := newVehicle(org.ID, "06 CAL 2", branchC.Uuid)
	v3 := newVehicle(other.ID, "35 CAL 3", otherBranch.Uuid)

	svc := fleetopsusecase.New(fleetopsrepository.NewPostgres(pool, q), vehicleSvc)
	h := fleetopshandler.New(svc, nil)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	fleetops.RegisterRoutes(mux, h, fleetops.EngineHandlers{}, tokens,
		mapLoader{alice.Uuid: alice.ID, carol.Uuid: carol.ID, dave.Uuid: dave.ID}, orgSvc)
	call := func(slug string, user db.User, method, path string, body any) (int, map[string]any) {
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

	// Fixtures relative to a day boundary so the window maths is exact.
	day := time.Now().UTC().Truncate(24 * time.Hour)
	from, to := day.AddDate(0, 0, -7), day.AddDate(0, 0, 7)
	rfc := func(t time.Time) string { return t.Format(time.RFC3339) }

	// v1: an ended block that starts before the window (clipped left), a
	// transfer A → B inside the window and a done maintenance item.
	if code, body := call(org.Slug, alice, "POST", "/v1/tenant/vehicles/"+v1.UUID.String()+"/blocks", map[string]any{
		"kind": "damage", "starts_at": rfc(day.AddDate(0, 0, -10)), "ends_at": rfc(day.AddDate(0, 0, -5)), "reason": "kaza",
	}); code != http.StatusCreated {
		t.Fatalf("block v1 = %d %v", code, body)
	}
	code, tr := call(org.Slug, alice, "POST", "/v1/tenant/transfers", map[string]any{
		"vehicle_uuid": v1.UUID, "to_branch_uuid": branchB.Uuid,
		"planned_departure": rfc(day.AddDate(0, 0, 2)), "planned_arrival": rfc(day.AddDate(0, 0, 2).Add(4 * time.Hour)),
	})
	if code != http.StatusCreated {
		t.Fatalf("transfer v1 = %d %v", code, tr)
	}
	// v2: an open-ended block from yesterday (clipped right) and a plan.
	if code, body := call(org.Slug, alice, "POST", "/v1/tenant/vehicles/"+v2.UUID.String()+"/blocks", map[string]any{
		"kind": "maintenance", "starts_at": rfc(day.AddDate(0, 0, -1)),
	}); code != http.StatusCreated {
		t.Fatalf("block v2 = %d %v", code, body)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vehicle_maintenance (organization_id, vehicle_id, kind, maintenance_type, done_date, done_km)
		SELECT organization_id, id, 'done', 'oil', $2::date, 12000 FROM vehicles WHERE uuid = $1`, v1.UUID, day.AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vehicle_maintenance (organization_id, vehicle_id, kind, maintenance_type, planned_date)
		SELECT organization_id, id, 'planned', 'periodic', $2::date FROM vehicles WHERE uuid = $1`, v2.UUID, day.AddDate(0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	// Outside the window: must not show up.
	if _, err := pool.Exec(ctx, `INSERT INTO vehicle_maintenance (organization_id, vehicle_id, kind, maintenance_type, planned_date)
		SELECT organization_id, id, 'planned', 'tire', $2::date FROM vehicles WHERE uuid = $1`, v2.UUID, day.AddDate(0, 0, 40)); err != nil {
		t.Fatal(err)
	}
	if code, body := call(other.Slug, dave, "POST", "/v1/tenant/vehicles/"+v3.UUID.String()+"/blocks", map[string]any{
		"kind": "cleaning", "starts_at": rfc(day),
	}); code != http.StatusCreated {
		t.Fatalf("block v3 = %d %v", code, body)
	}

	calendar := func(slug string, user db.User, params url.Values) (int, map[string]any) {
		t.Helper()
		return call(slug, user, "GET", "/v1/tenant/fleet/calendar?"+params.Encode(), nil)
	}
	window := func(extra ...string) url.Values {
		v := url.Values{"from": {rfc(from)}, "to": {rfc(to)}}
		for i := 0; i+1 < len(extra); i += 2 {
			v.Set(extra[i], extra[i+1])
		}
		return v
	}
	detailCode := func(body map[string]any) string {
		details, _ := body["details"].([]any)
		if len(details) == 0 {
			return ""
		}
		c, _ := details[0].(map[string]any)["code"].(string)
		return c
	}
	rows := func(body map[string]any) []map[string]any {
		items, _ := body["items"].([]any)
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			out = append(out, it.(map[string]any))
		}
		return out
	}
	plateOf := func(row map[string]any) string {
		return row["vehicle"].(map[string]any)["plate"].(string)
	}
	list := func(row map[string]any, key string) []map[string]any {
		items, _ := row[key].([]any)
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			out = append(out, it.(map[string]any))
		}
		return out
	}

	t.Run("window validation", func(t *testing.T) {
		if code, body := calendar(org.Slug, alice, url.Values{"to": {rfc(to)}}); code != http.StatusBadRequest || detailCode(body) != "required" {
			t.Errorf("missing from = %d %v", code, body)
		}
		if code, body := calendar(org.Slug, alice, url.Values{"from": {"yesterday"}, "to": {rfc(to)}}); code != http.StatusBadRequest || detailCode(body) != "invalid" {
			t.Errorf("bad from = %d %v", code, body)
		}
		if code, body := calendar(org.Slug, alice, url.Values{"from": {rfc(to)}, "to": {rfc(from)}}); code != http.StatusBadRequest || detailCode(body) != "before_from" {
			t.Errorf("to before from = %d %v", code, body)
		}
		if code, body := calendar(org.Slug, alice, url.Values{"from": {rfc(from)}, "to": {rfc(from.AddDate(0, 0, 94))}}); code != http.StatusBadRequest || detailCode(body) != "range_too_large" {
			t.Errorf("94 days = %d %v", code, body)
		}
		if code, body := calendar(org.Slug, alice, url.Values{"from": {rfc(from)}, "to": {rfc(from.AddDate(0, 0, 93))}}); code != http.StatusOK {
			t.Errorf("93 days = %d %v", code, body)
		}
		if code, body := calendar(org.Slug, alice, window("vehicle", "nope")); code != http.StatusBadRequest {
			t.Errorf("bad vehicle = %d %v", code, body)
		}
	})

	t.Run("owner sees the page with clipped bars", func(t *testing.T) {
		code, body := calendar(org.Slug, alice, window())
		if code != http.StatusOK {
			t.Fatalf("calendar = %d %v", code, body)
		}
		got := rows(body)
		if body["total"].(float64) != 2 || len(got) != 2 || plateOf(got[0]) != "06 CAL 2" || plateOf(got[1]) != "34 CAL 1" {
			t.Fatalf("rows = %v (total %v)", got, body["total"])
		}
		layers, _ := body["layers"].([]any)
		// block, transfer, hold, reservation (tenant.availability.read), maintenance.
		if len(layers) != 5 {
			t.Errorf("layers = %v", layers)
		}

		// v2: open-ended block clipped to the window end; plan inside, the
		// one 40 days out is not listed.
		v2Bars := list(got[0], "intervals")
		if len(v2Bars) != 1 || v2Bars[0]["kind"] != "block" || v2Bars[0]["ends_at"] != nil ||
			v2Bars[0]["continues_after"] != true || v2Bars[0]["continues_before"] != false ||
			v2Bars[0]["clip_end"] != rfc(to) || v2Bars[0]["clip_start"] != rfc(day.AddDate(0, 0, -1)) {
			t.Errorf("v2 bars = %v", v2Bars)
		}
		v2Maint := list(got[0], "maintenance")
		if len(v2Maint) != 1 || v2Maint[0]["date"] != day.AddDate(0, 0, 3).Format(time.DateOnly) ||
			v2Maint[0]["kind"] != "planned" || v2Maint[0]["state"] != "due" {
			t.Errorf("v2 maintenance = %v", v2Maint)
		}

		// v1: block clipped at the window start, then the transfer.
		v1Bars := list(got[1], "intervals")
		if len(v1Bars) != 2 {
			t.Fatalf("v1 bars = %v", v1Bars)
		}
		block, transfer := v1Bars[0], v1Bars[1]
		if block["kind"] != "block" || block["continues_before"] != true || block["continues_after"] != false ||
			block["clip_start"] != rfc(from) || block["starts_at"] != rfc(day.AddDate(0, 0, -10)) ||
			block["clip_end"] != rfc(day.AddDate(0, 0, -5)) {
			t.Errorf("v1 block = %v", block)
		}
		if b, _ := block["block"].(map[string]any); b == nil || b["kind"] != "damage" || b["reason"] != "kaza" {
			t.Errorf("v1 block details = %v", block["block"])
		}
		td, _ := transfer["transfer"].(map[string]any)
		if transfer["kind"] != "transfer" || transfer["uuid"] != tr["uuid"] || td == nil || td["status"] != "planned" ||
			td["in_scope"] != true || td["to_branch"].(map[string]any)["code"] != "IST" ||
			td["planned_departure"] != rfc(day.AddDate(0, 0, 2)) {
			t.Errorf("v1 transfer = %v", transfer)
		}
		if m := list(got[1], "maintenance"); len(m) != 1 || m[0]["state"] != "done" || m[0]["done_km"].(float64) != 12000 {
			t.Errorf("v1 maintenance = %v", m)
		}

		// A window that ends before the transfer and starts after the block.
		code, body = calendar(org.Slug, alice, url.Values{
			"from": {rfc(day.AddDate(0, 0, -4))}, "to": {rfc(day.AddDate(0, 0, -2))}, "vehicle": {v1.UUID.String()},
		})
		if got := rows(body); code != http.StatusOK || len(got) != 1 || len(list(got[0], "intervals")) != 0 {
			t.Errorf("empty window = %d %v", code, body)
		}
	})

	t.Run("filters and paging", func(t *testing.T) {
		code, body := calendar(org.Slug, alice, window("q", "34cal", "limit", "1"))
		if got := rows(body); code != http.StatusOK || len(got) != 1 || plateOf(got[0]) != "34 CAL 1" || body["total"].(float64) != 1 {
			t.Errorf("q = %d %v", code, body)
		}
		code, body = calendar(org.Slug, alice, window("limit", "1", "offset", "1"))
		if got := rows(body); code != http.StatusOK || len(got) != 1 || plateOf(got[0]) != "34 CAL 1" || body["total"].(float64) != 2 {
			t.Errorf("page 2 = %d %v", code, body)
		}
		code, body = calendar(org.Slug, alice, window("home_branch", branchC.Uuid.String()))
		if got := rows(body); code != http.StatusOK || len(got) != 1 || plateOf(got[0]) != "06 CAL 2" {
			t.Errorf("home branch = %d %v", code, body)
		}
		code, body = calendar(org.Slug, alice, window("status", "maintenance"))
		if got := rows(body); code != http.StatusOK || len(got) != 0 {
			// Blocks never change the status by themselves (no sync_status).
			t.Errorf("status = %d %v", code, body)
		}
	})

	t.Run("branch scope", func(t *testing.T) {
		code, body := calendar(org.Slug, carol, window())
		got := rows(body)
		if code != http.StatusOK || len(got) != 1 || plateOf(got[0]) != "06 CAL 2" || body["total"].(float64) != 1 {
			t.Fatalf("carol = %d %v", code, body)
		}
		code, body = calendar(org.Slug, carol, window("vehicle", v1.UUID.String()))
		if code != http.StatusOK || len(rows(body)) != 0 || body["total"].(float64) != 0 {
			t.Errorf("carol asking for v1 = %d %v", code, body)
		}
	})

	t.Run("organization isolation", func(t *testing.T) {
		code, body := calendar(org.Slug, alice, window("vehicle", v3.UUID.String()))
		if code != http.StatusOK || len(rows(body)) != 0 {
			t.Errorf("alice asking for v3 = %d %v", code, body)
		}
		code, body = calendar(other.Slug, dave, window())
		got := rows(body)
		if code != http.StatusOK || len(got) != 1 || plateOf(got[0]) != "35 CAL 3" || len(list(got[0], "intervals")) != 1 {
			t.Errorf("dave = %d %v", code, body)
		}
		if code, _ := calendar(other.Slug, alice, window()); code != http.StatusForbidden {
			t.Errorf("non-member = %d", code)
		}
	})
}
