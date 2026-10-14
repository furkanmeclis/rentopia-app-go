package fleet_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet"
	fleetadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/adapters"
	fleethandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/handler"
	fleetrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	fleetusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeCentrifugo struct {
	mu   sync.Mutex
	pubs []string
}

// Publish records "channel resource action uuids…" lines.
func (f *fakeCentrifugo) Publish(_ context.Context, channel string, data any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ev, _ := data.(resourceevents.Event)
	line := strings.TrimSpace(strings.Join(append([]string{channel, ev.Resource, ev.Action}, ev.UUIDs...), " "))
	f.pubs = append(f.pubs, line)
	return nil
}

func (f *fakeCentrifugo) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.pubs
	f.pubs = nil
	sort.Strings(out)
	return out
}

type memDriver struct {
	storage.Driver
	m *storage.Memory
}

func (d memDriver) Upload(ctx context.Context, f storage.File, p string) error {
	return d.m.Upload(ctx, f, p)
}

func (d memDriver) Download(ctx context.Context, p string) (io.ReadCloser, int64, error) {
	return d.m.Download(ctx, p)
}

func (d memDriver) Delete(ctx context.Context, p string) error { return d.m.Delete(ctx, p) }

type mapLoader map[uuid.UUID]int64

func (m mapLoader) LoadPrincipal(_ *http.Request, c jwt.Claims) (authctx.Principal, error) {
	id, _ := uuid.Parse(c.Subject)
	return authctx.Principal{UserID: id, UserInternal: m[id]}, nil
}

// TestFleetLiveEvents checks that segment, rental group and product
// mutations (product versions and images included) and the fleet bulk jobs
// publish on the organization's channels, with a refetch of the parent lists
// whose counts can move, and that only members with tenant.fleet.read may
// subscribe. Runs with TEST_DATABASE_URL.
func TestFleetLiveEvents(t *testing.T) {
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
			Email: name + "-fleetlive-" + suffix + "@example.test", PasswordHash: "x", Name: name, Surname: "Test", Status: "active",
		})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, bob := newUser("alice"), newUser("bob")
	orgSvc := orgusecase.New(orgrepository.NewPostgres(pool, q), branchusecase.NewOrganizationHooks(nil))
	org, err := orgSvc.CreateOrganization(ctx, orgmodel.CreateOrganizationInput{Name: "Fleet live " + suffix, OwnerUserID: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, "DELETE FROM organizations WHERE id = $1", org.ID)
		_, _ = pool.Exec(bg, "DELETE FROM users WHERE id = ANY($1)", []int64{alice.ID, bob.ID})
	})

	fake := &fakeCentrifugo{}
	events := resourceevents.New(fake, nil)
	events.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(q)))

	svc := fleetusecase.New(fleetrepository.NewPostgres(pool, q), memDriver{m: storage.NewMemory()}, nil)
	bulkSvc := bulkusecase.New(q, bulkengine.NewRegistry(
		fleetadapters.NewGroupsBulk(svc), fleetadapters.NewProductsBulk(svc),
	), nil, nil, nil, config.BulkConfig{}, nil)
	bulkSvc.SetResourceEvents(events)
	h := fleethandler.New(svc, nil)
	h.SetResourceEvents(events)
	tokens, err := jwt.NewManager("test-access-secret-0123456789abcdef", time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	bulkH := bulkhandler.New(bulkSvc)
	fleet.RegisterRoutes(mux, h, fleet.EngineHandlers{
		BulkGroups:   bulkH.TenantExecute(fleet.ResourceRentalGroups),
		BulkProducts: bulkH.TenantExecute(fleet.ResourceRentalProducts),
	}, tokens, mapLoader{alice.Uuid: alice.ID, bob.Uuid: bob.ID}, orgSvc)

	send := func(method, path, contentType string, body io.Reader) (int, map[string]any) {
		t.Helper()
		access, _, err := tokens.IssueAccess(jwt.AccessInput{UserID: alice.Uuid})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, body)
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set(orgctx.HeaderName, org.Slug)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var env map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &env)
		data, _ := env["data"].(map[string]any)
		if rr.Code >= 300 {
			t.Logf("%s %s = %d %s", method, path, rr.Code, rr.Body.String())
		}
		return rr.Code, data
	}
	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		return send(method, path, "application/json", &buf)
	}
	must := func(want, got int, step string) {
		t.Helper()
		if got != want {
			t.Fatalf("%s = %d, want %d", step, got, want)
		}
	}
	channel := func(resource string) string { return tenantmodule.ResourceChannel(org.UUID, resource) }
	pub := func(resource, action string, uuids ...string) string {
		return strings.TrimSpace(strings.Join(append([]string{channel(resource), resource, action}, uuids...), " "))
	}
	expect := func(step string, want ...string) {
		t.Helper()
		events.Wait()
		got := fake.take()
		sort.Strings(want)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("%s:\n got  %q\n want %q", step, got, want)
		}
	}

	// Segments.
	code, seg := call(http.MethodPost, "/v1/tenant/segments", map[string]any{"code": "EKO", "name": "Ekonomi"})
	must(http.StatusCreated, code, "create segment")
	segID := seg["uuid"].(string)
	expect("create segment", pub(fleet.ResourceSegments, resourceevents.ActionCreated, segID))
	code, _ = call(http.MethodPatch, "/v1/tenant/segments/"+segID, map[string]any{"name": "Ekonomi sınıfı"})
	must(http.StatusOK, code, "patch segment")
	expect("patch segment", pub(fleet.ResourceSegments, resourceevents.ActionUpdated, segID))

	// Rental groups: the segment counts move on create / delete.
	code, group := call(http.MethodPost, "/v1/tenant/rental-groups", map[string]any{
		"code": "EKO-M", "name": "Ekonomi manuel", "segment_uuid": segID,
	})
	must(http.StatusCreated, code, "create group")
	groupID := group["uuid"].(string)
	expect("create group",
		pub(fleet.ResourceRentalGroups, resourceevents.ActionCreated, groupID),
		pub(fleet.ResourceSegments, resourceevents.ActionBulk))
	code, _ = call(http.MethodPatch, "/v1/tenant/rental-groups/"+groupID, map[string]any{"name": "Ekonomi (manuel)"})
	must(http.StatusOK, code, "patch group")
	expect("patch group", pub(fleet.ResourceRentalGroups, resourceevents.ActionUpdated, groupID))

	// Products: create refreshes segment and group counts.
	code, product := call(http.MethodPost, "/v1/tenant/products", map[string]any{
		"code": "CLIO", "title": "Ekonomi Manuel", "segment_uuid": segID,
		"guarantee": map[string]any{"type": "GROUP", "rental_group_uuid": groupID},
	})
	must(http.StatusCreated, code, "create product")
	productID := product["uuid"].(string)
	expect("create product",
		pub(fleet.ResourceRentalProducts, resourceevents.ActionCreated, productID),
		pub(fleet.ResourceSegments, resourceevents.ActionBulk),
		pub(fleet.ResourceRentalGroups, resourceevents.ActionBulk))

	// A new version (conditions change) is one update of the product.
	code, patched := call(http.MethodPatch, "/v1/tenant/products/"+productID, map[string]any{
		"conditions": map[string]any{"min_driver_age": 25},
	})
	must(http.StatusOK, code, "version product")
	if v, _ := patched["current_version"].(float64); v != 2 {
		t.Fatalf("current_version = %v, want 2", patched["current_version"])
	}
	expect("version product", pub(fleet.ResourceRentalProducts, resourceevents.ActionUpdated, productID))

	// Images are updates of the product.
	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	fw, _ := mw.CreateFormFile("file", "front.png")
	_, _ = fw.Write(append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...))
	_ = mw.Close()
	code, image := send(http.MethodPost, "/v1/tenant/products/"+productID+"/images", mw.FormDataContentType(), &form)
	must(http.StatusCreated, code, "upload image")
	expect("upload image", pub(fleet.ResourceRentalProducts, resourceevents.ActionUpdated, productID))
	code, _ = call(http.MethodDelete, "/v1/tenant/products/"+productID+"/images/"+image["uuid"].(string), nil)
	must(http.StatusOK, code, "delete image")
	expect("delete image", pub(fleet.ResourceRentalProducts, resourceevents.ActionUpdated, productID))

	// Bulk jobs publish one bulk event with the affected uuids.
	bulk := func(path, id string) {
		t.Helper()
		code, _ := call(http.MethodPost, path, map[string]any{
			"action": "deactivate", "target": map[string]any{"scope": "ids", "ids": []string{id}},
		})
		must(http.StatusOK, code, "bulk "+path)
	}
	bulk("/v1/tenant/rental-groups/bulk", groupID)
	expect("bulk groups", pub(fleet.ResourceRentalGroups, resourceevents.ActionBulk, groupID))
	bulk("/v1/tenant/products/bulk", productID)
	expect("bulk products", pub(fleet.ResourceRentalProducts, resourceevents.ActionBulk, productID))

	code, _ = call(http.MethodDelete, "/v1/tenant/products/"+productID, nil)
	must(http.StatusOK, code, "delete product")
	expect("delete product",
		pub(fleet.ResourceRentalProducts, resourceevents.ActionDeleted, productID),
		pub(fleet.ResourceSegments, resourceevents.ActionBulk),
		pub(fleet.ResourceRentalGroups, resourceevents.ActionBulk))
	code, _ = call(http.MethodDelete, "/v1/tenant/rental-groups/"+groupID, nil)
	must(http.StatusOK, code, "delete group")
	expect("delete group",
		pub(fleet.ResourceRentalGroups, resourceevents.ActionDeleted, groupID),
		pub(fleet.ResourceSegments, resourceevents.ActionBulk))
	code, _ = call(http.MethodDelete, "/v1/tenant/segments/"+segID, nil)
	must(http.StatusOK, code, "delete segment")
	expect("delete segment", pub(fleet.ResourceSegments, resourceevents.ActionDeleted, segID))

	// Subscription authorization for the organization channels.
	reg := resourceevents.NewRegistry()
	fleet.RegisterLiveResources(reg)
	rule := tenantmodule.ChannelRule(reg, orgSvc)
	allowed := func(user db.User, ch string) bool {
		t.Helper()
		pctx := authctx.WithPrincipal(ctx, authctx.Principal{UserID: user.Uuid, UserInternal: user.ID})
		handled, ok, err := rule(pctx, user.Uuid, false, ch)
		if err != nil || !handled {
			t.Fatalf("rule(%s): handled=%v err=%v", ch, handled, err)
		}
		return ok
	}
	for _, resource := range []string{fleet.ResourceSegments, fleet.ResourceRentalGroups, fleet.ResourceRentalProducts} {
		if !allowed(alice, channel(resource)) {
			t.Fatalf("owner denied %s", resource)
		}
		if allowed(bob, channel(resource)) {
			t.Fatalf("non-member allowed %s", resource)
		}
	}
}
