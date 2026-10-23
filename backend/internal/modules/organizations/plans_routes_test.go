package organizations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations"
	orghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/handler"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-674 / RT-A28: the platform plan screen (plan, period, overrides,
// usage), its permission guards, the plan of new organizations, /auth/me
// and the server-side gate: an organization on the Yönetim plan gets 403
// FEATURE_NOT_ENABLED on a public_api route until the platform enables it.
func TestPlatformPlanRoutes(t *testing.T) {
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
	var userIDs, orgIDs []int64
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = ANY($1)", orgIDs)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", userIDs)
	})
	newUser := func(name string) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{
			Email: name + "-plans-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		return u
	}
	operator, viewer, support, owner := newUser("operator"), newUser("viewer"), newUser("support"), newUser("owner")
	planner := newUser("planner")

	repo := orgrepository.NewPostgres(pool, q)
	orgSvc := orgusecase.New(repo, branchusecase.NewOrganizationHooks(nil))
	store := features.NewStore(q)
	orgSvc.SetFeatures(store)
	h := orghandler.New(orgSvc, nil)
	h.SetPlans(orgusecase.NewPlans(orgSvc, orgrepository.NewPlans(q), store))
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	loader := permLoader{}
	grant := func(u db.User, perms ...string) {
		loader[u.Uuid] = struct {
			id    int64
			perms []string
		}{u.ID, perms}
	}
	grant(operator, rbac.PermPlatformOrganizationsRead, rbac.PermPlatformOrganizationsWrite,
		rbac.PermPlatformOrganizationsPlanRead, rbac.PermPlatformOrganizationsPlanWrite)
	grant(viewer, rbac.PermPlatformOrganizationsRead, rbac.PermPlatformOrganizationsPlanRead)
	grant(support, rbac.PermPlatformOrganizationsRead)
	// Plan permissions without platform.organizations.write.
	grant(planner, rbac.PermPlatformOrganizationsRead, rbac.PermPlatformOrganizationsPlanRead, rbac.PermPlatformOrganizationsPlanWrite)
	grant(owner)

	mux := http.NewServeMux()
	organizations.RegisterRoutes(mux, h, organizations.PlatformEngineHandlers{}, tokens, loader)
	// A public_api tenant route behind the real resolver (no cache, so a
	// plan change shows at once) and RequireFeature, like
	// /v1/tenant/public-api/*.
	mux.Handle("POST /v1/tenant/public-api/keys", middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response.JSON(w, r, http.StatusCreated, map[string]string{"status": "created"})
		}),
		middleware.Authenticate(tokens, loader), middleware.RequireOrganization(orgSvc), middleware.RequireFeature(features.PublicAPI),
	))

	type envelope struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Field, Code string
			} `json:"details"`
		} `json:"error"`
	}
	call := func(as *db.User, method, path, orgSlug string, body any) (int, envelope) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		if as != nil {
			access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: as.Uuid})
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+access)
		}
		if orgSlug != "" {
			req.Header.Set(orgctx.HeaderName, orgSlug)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env envelope
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		return rr.Code, env
	}
	type planView struct {
		Plan struct {
			Code, Name string
		} `json:"plan"`
		PeriodStartsAt *time.Time `json:"period_starts_at"`
		PeriodEndsAt   *time.Time `json:"period_ends_at"`
		UpdatedAt      *time.Time `json:"updated_at"`
		Features       []struct {
			Key         string `json:"key"`
			PlanDefault bool   `json:"plan_default"`
			Override    *bool  `json:"override"`
			Enabled     bool   `json:"enabled"`
			Usage       *int64 `json:"usage"`
		} `json:"features"`
		EnabledFeatures []string `json:"enabled_features"`
	}
	decode := func(step string, env envelope) planView {
		t.Helper()
		var v planView
		if err := json.Unmarshal(env.Data, &v); err != nil {
			t.Fatalf("%s: %v (%s)", step, err, env.Data)
		}
		return v
	}
	expect := func(step string, want, got int, env envelope) {
		t.Helper()
		if got != want {
			t.Fatalf("%s = %d (%s %s), want %d", step, got, env.Error.Code, env.Data, want)
		}
	}

	// A new organization gets the default plan (Komple): everything on.
	code, env := call(&operator, http.MethodPost, "/v1/platform/organizations", "", map[string]any{
		"name": "Plan Org " + suffix, "owner_user_uuid": owner.Uuid,
	})
	expect("create", http.StatusCreated, code, env)
	var created struct {
		Organization struct {
			UUID string `json:"uuid"`
			Slug string `json:"slug"`
		} `json:"organization"`
	}
	_ = json.Unmarshal(env.Data, &created)
	orgUUID, slug := created.Organization.UUID, created.Organization.Slug
	org, err := orgSvc.GetOrganization(ctx, uuid.MustParse(orgUUID))
	if err != nil {
		t.Fatal(err)
	}
	orgIDs = append(orgIDs, org.ID)
	planPath := "/v1/platform/organizations/" + orgUUID + "/plan"

	code, env = call(&viewer, http.MethodGet, planPath, "", nil)
	expect("viewer reads plan", http.StatusOK, code, env)
	v := decode("initial plan", env)
	if v.Plan.Code != features.PlanComplete || v.UpdatedAt == nil || len(v.EnabledFeatures) != len(features.All()) {
		t.Fatalf("initial plan = %+v", v)
	}
	if len(v.Features) != len(features.All()) || v.Features[0].Key != "site" || v.Features[0].Usage == nil || *v.Features[0].Usage != 0 {
		t.Fatalf("feature states = %+v", v.Features)
	}
	code, env = call(&owner, http.MethodPost, "/v1/tenant/public-api/keys", slug, map[string]any{})
	expect("complete plan creates a key", http.StatusCreated, code, env)

	// Guards: plan.read to read, plan.write to change.
	code, env = call(&support, http.MethodGet, planPath, "", nil)
	expect("support without plan.read", http.StatusForbidden, code, env)
	code, env = call(&viewer, http.MethodPut, planPath, "", map[string]any{"code": "management"})
	expect("viewer changes plan", http.StatusForbidden, code, env)
	code, env = call(&planner, http.MethodPut, planPath, "", map[string]any{"code": "management"})
	expect("plan.write without organizations.write changes plan", http.StatusForbidden, code, env)
	code, env = call(&planner, http.MethodPut, "/v1/platform/organizations/"+orgUUID+"/features/sms", "", map[string]any{"enabled": false})
	expect("plan.write without organizations.write overrides a feature", http.StatusForbidden, code, env)
	code, env = call(&planner, http.MethodGet, planPath, "", nil)
	expect("planner reads plan", http.StatusOK, code, env)
	code, env = call(&viewer, http.MethodGet, "/v1/platform/plans", "", nil)
	expect("list plans", http.StatusOK, code, env)
	var plans struct {
		Items []struct {
			Code     string   `json:"code"`
			Features []string `json:"features"`
		} `json:"items"`
	}
	_ = json.Unmarshal(env.Data, &plans)
	codes := []string{}
	for _, p := range plans.Items {
		codes = append(codes, p.Code)
	}
	sort.Strings(codes)
	if strings.Join(codes, ",") != "complete,management" {
		t.Fatalf("plans = %+v", plans)
	}

	// RT-A28: on Yönetim the public API is refused server side.
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	code, env = call(&operator, http.MethodPut, planPath, "", map[string]any{
		"code": "management", "period_starts_at": start, "period_ends_at": start.AddDate(1, 0, 0),
	})
	expect("set management", http.StatusOK, code, env)
	v = decode("management", env)
	if v.Plan.Code != "management" || len(v.EnabledFeatures) != 0 || v.PeriodStartsAt == nil || !v.PeriodStartsAt.Equal(start) {
		t.Fatalf("management view = %+v", v)
	}
	code, env = call(&owner, http.MethodPost, "/v1/tenant/public-api/keys", slug, map[string]any{})
	if code != http.StatusForbidden || env.Error.Code != response.CodeFeatureNotEnabled ||
		len(env.Error.Details) != 1 || env.Error.Details[0].Code != "public_api" {
		t.Fatalf("RT-A28 public API on Yönetim = %d %+v", code, env.Error)
	}

	// An override enables it for this organization only; null clears it.
	featurePath := "/v1/platform/organizations/" + orgUUID + "/features/public_api"
	code, env = call(&operator, http.MethodPut, featurePath, "", map[string]any{"enabled": true})
	expect("override on", http.StatusOK, code, env)
	v = decode("override on", env)
	if strings.Join(v.EnabledFeatures, ",") != "public_api" {
		t.Fatalf("override on = %v", v.EnabledFeatures)
	}
	for _, f := range v.Features {
		if f.Key == "public_api" && (f.PlanDefault || f.Override == nil || !*f.Override || !f.Enabled) {
			t.Fatalf("public_api state = %+v", f)
		}
	}
	code, env = call(&owner, http.MethodPost, "/v1/tenant/public-api/keys", slug, map[string]any{})
	expect("override lets the key through", http.StatusCreated, code, env)
	me, err := orgSvc.ListForUser(ctx, owner.ID)
	if err != nil || len(me) != 1 || me[0].Plan.Code != "management" || strings.Join(me[0].Features, ",") != "public_api" {
		t.Fatalf("/auth/me organizations = %+v %v", me, err)
	}
	code, env = call(&operator, http.MethodPut, featurePath, "", map[string]any{"enabled": nil})
	expect("override cleared", http.StatusOK, code, env)
	if v = decode("cleared", env); len(v.EnabledFeatures) != 0 {
		t.Fatalf("cleared = %v", v.EnabledFeatures)
	}
	code, env = call(&owner, http.MethodPost, "/v1/tenant/public-api/keys", slug, map[string]any{})
	expect("cleared override refuses again", http.StatusForbidden, code, env)

	// Validation.
	code, env = call(&operator, http.MethodPut, "/v1/platform/organizations/"+orgUUID+"/features/teleport", "", map[string]any{"enabled": true})
	expect("unknown feature", http.StatusNotFound, code, env)
	code, env = call(&operator, http.MethodPut, planPath, "", map[string]any{"code": "gold"})
	expect("unknown plan", http.StatusBadRequest, code, env)
	code, env = call(&operator, http.MethodPut, planPath, "", map[string]any{
		"code": "complete", "period_starts_at": start, "period_ends_at": start.AddDate(0, 0, -1),
	})
	expect("period end before start", http.StatusBadRequest, code, env)
	code, env = call(&operator, http.MethodGet, "/v1/platform/organizations/"+uuid.NewString()+"/plan", "", nil)
	expect("unknown organization", http.StatusNotFound, code, env)

	// The platform may pick the plan of a new organization.
	code, env = call(&operator, http.MethodPost, "/v1/platform/organizations", "", map[string]any{
		"name": "Yönetim Org " + suffix, "owner_user_uuid": owner.Uuid, "plan_code": "management",
	})
	expect("create on management", http.StatusCreated, code, env)
	_ = json.Unmarshal(env.Data, &created)
	second, _ := orgSvc.GetOrganization(ctx, uuid.MustParse(created.Organization.UUID))
	orgIDs = append(orgIDs, second.ID)
	code, env = call(&operator, http.MethodGet, "/v1/platform/organizations/"+created.Organization.UUID+"/plan", "", nil)
	if v = decode("second", env); code != http.StatusOK || v.Plan.Code != "management" {
		t.Fatalf("second plan = %d %+v", code, v)
	}
	code, env = call(&operator, http.MethodPost, "/v1/platform/organizations", "", map[string]any{
		"name": "Bad Plan " + suffix, "owner_user_uuid": owner.Uuid, "plan_code": "gold",
	})
	expect("create with unknown plan", http.StatusBadRequest, code, env)
	var n int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM organizations WHERE name = $1", "Bad Plan "+suffix).Scan(&n)
	if n != 0 {
		t.Fatal("an organization was created with an unknown plan")
	}
}
