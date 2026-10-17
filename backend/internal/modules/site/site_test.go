package site_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi"
	pahandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/handler"
	pamodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	pausecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Branded site (TEC-729) against a real database: two organizations with
// their own sites on the same base domain, the public API for sessions,
// and a custom domain hook (TEC-731 plugs in there).

type org struct {
	id   int64
	slug string
}

type env struct {
	t        *testing.T
	pool     *pgxpool.Pool
	q        *db.Queries
	svc      *usecase.Service
	keys     *pausecase.Service
	server   *httptest.Server
	store    *storage.Memory
	domains  *fakeDomains
	a, b     org
	settings *orgsettingsusecase.Service
}

type stubResolver struct{}

func (stubResolver) Resolve(context.Context, string, int64) (orgctx.Scope, error) {
	return orgctx.Scope{}, orgctx.ErrNotMember
}

// fakeDomains maps verified custom domains to organizations.
type fakeDomains struct{ byHost map[string]int64 }

func (f *fakeDomains) Lookup(_ context.Context, host string) (int64, error) {
	if id, ok := f.byHost[host]; ok {
		return id, nil
	}
	return 0, usecase.ErrNotFound
}

func (f *fakeDomains) Hosts(_ context.Context, orgID int64) ([]model.Host, error) {
	out := []model.Host{}
	for h, id := range f.byHost {
		if id == orgID {
			out = append(out, model.Host{Host: h, Origin: "https://" + h, Kind: model.HostCustom})
		}
	}
	return out, nil
}

func (e *env) must(err error) {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
}

func newEnv(t *testing.T) *env {
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
	e := &env{t: t, pool: pool, q: q, domains: &fakeDomains{byHost: map[string]int64{}}}
	e.settings = orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, q))
	fc, err := crypto.NewFieldCipher(strings.Repeat("s", 32))
	e.must(err)
	e.keys, err = pausecase.New(pool, q, fc, e.settings, nil, nil, nil)
	e.must(err)
	base, err := usecase.ParseBaseURL("https://sites.test")
	e.must(err)
	e.svc = usecase.New(q, e.keys, e.settings, base, "https://app.test/")
	e.store = storage.NewMemory()
	e.svc.SetStorage(e.store)

	mux := http.NewServeMux()
	publicapi.RegisterRoutes(mux, pahandler.NewPublic(e.keys, nil), pahandler.NewPanel(e.keys, nil), nil, nil, stubResolver{})
	site.RegisterRoutes(mux, handler.New(e.svc, nil, nil), nil, nil, stubResolver{})
	e.server = httptest.NewServer(mux)
	t.Cleanup(e.server.Close)

	suffix := strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	e.a = e.newOrg("Site-A_" + suffix)
	e.b = e.newOrg("site-b-" + suffix)
	return e
}

func (e *env) newOrg(slug string) org {
	ctx := context.Background()
	o, err := e.q.CreateOrganization(ctx, db.CreateOrganizationParams{Slug: slug, Name: "Org " + slug})
	e.must(err)
	e.t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM organizations WHERE id = $1", o.ID)
	})
	e.setWeb(o.ID, true)
	return org{id: o.ID, slug: slug}
}

// setWeb stores a settings version with the web channel on or off.
func (e *env) setWeb(orgID int64, on bool) {
	cur, err := e.settings.Current(context.Background(), orgID)
	e.must(err)
	s := cur.Settings
	if cur.Version == 0 {
		s = osmodel.Defaults()
	}
	for i := range s.SalesChannels {
		if s.SalesChannels[i].Code == "web" {
			s.SalesChannels[i].IsActive = on
		}
	}
	data, _ := json.Marshal(s)
	_, err = e.pool.Exec(context.Background(),
		`INSERT INTO organization_settings_versions (organization_id, version, data) VALUES ($1, $2, $3)`, orgID, cur.Version+1, data)
	e.must(err)
}

func ptr[T any](v T) *T { return &v }

type answer struct {
	code int
	data json.RawMessage
	err  struct {
		Code    string `json:"code"`
		Details []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"details"`
	}
	raw []byte
}

func (e *env) call(method, path string, body any, headers map[string]string) answer {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.server.URL+path, rd)
	e.must(err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	e.must(err)
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	_ = json.Unmarshal(raw, &env)
	out := answer{code: res.StatusCode, data: env.Data, raw: raw}
	if len(env.Error) > 0 {
		_ = json.Unmarshal(env.Error, &out.err)
	}
	return out
}

func (e *env) resolve(host string) (model.Resolved, answer) {
	e.t.Helper()
	r := e.call(http.MethodGet, "/v1/public/site/resolve?host="+host, nil, nil)
	var out model.Resolved
	if r.code == http.StatusOK {
		e.must(json.Unmarshal(r.data, &out))
	}
	return out, r
}

func hasField(err error, field, code string) bool {
	var ve *usecase.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	for _, d := range ve.Details {
		if d.Field == field && d.Code == code {
			return true
		}
	}
	return false
}

func themed(primary, title string) *model.Config {
	c := model.DefaultConfig()
	c.Theme.Colors.Primary = primary
	c.Theme.Font = "poppins"
	c.Layout.Hero = "split"
	tr := c.Content["tr"]
	tr.HeroTitle = title
	tr.FAQ = []model.FAQItem{{Question: "Depozito var mı?", Answer: "Evet."}}
	c.Content["tr"] = tr
	c.Contact.Email = "info@example.com"
	c.Analytics.GA4MeasurementID = "g-abc1234"
	return &c
}

func TestSettingsAndKey(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// Before the first save: defaults, a subdomain derived from the slug.
	got, err := e.svc.Get(ctx, e.a.id, e.a.slug)
	e.must(err)
	wantSub := strings.ToLower(strings.ReplaceAll(e.a.slug, "_", "-"))
	if got.Exists || got.Version != 0 || got.Subdomain != wantSub || got.URL != "https://"+wantSub+".sites.test" || got.Enabled {
		t.Fatalf("defaults = %+v", got)
	}
	if got.Config.Theme.Font != "inter" || got.Config.Layout.Hero != "centered" {
		t.Fatalf("default config = %+v", got.Config)
	}

	// Version is required and must match.
	if _, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{}); !hasField(err, "version", "required") {
		t.Fatalf("no version: %v", err)
	}
	if _, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(3)}); !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale first save: %v", err)
	}

	// Validation covers every free-form value.
	bad := model.DefaultConfig()
	bad.Theme.Colors.Primary = "red;}body{display:none"
	bad.Theme.Font = "comic"
	bad.Layout.Cards = "carousel"
	bad.Content["de"] = model.Content{}
	bad.Analytics.GA4MeasurementID = "UA-1234"
	bad.Contact.Email = "not-an-email"
	tr := bad.Content["tr"]
	tr.FAQ = []model.FAQItem{{Question: "?"}}
	bad.Content["tr"] = tr
	_, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Config: &bad, Subdomain: ptr("www")})
	for _, f := range [][2]string{
		{"config.theme.colors.primary", "invalid"}, {"config.theme.font", "invalid"}, {"config.layout.cards", "invalid"},
		{"config.content.de", "invalid"}, {"config.analytics.ga4_measurement_id", "invalid"},
		{"config.contact.email", "invalid"}, {"config.content.tr.faq.0", "required"}, {"subdomain", "reserved"},
	} {
		if !hasField(err, f[0], f[1]) {
			t.Errorf("validation misses %s %s: %v", f[0], f[1], err)
		}
	}
	if _, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Subdomain: ptr("-bad-")}); !hasField(err, "subdomain", "invalid") {
		t.Errorf("bad subdomain: %v", err)
	}

	// First save switches the site on and creates its publishable key.
	got, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{
		Version: ptr(0), Enabled: ptr(true), Subdomain: ptr("Acme-" + e.a.slug[7:]), Config: themed("#FF0000", "A firması"),
	})
	e.must(err)
	subA := "acme-" + e.a.slug[7:]
	originA := "https://" + subA + ".sites.test"
	if !got.Exists || got.Version != 1 || !got.Enabled || got.Subdomain != subA || got.APIKey == nil || !got.BookingEnabled {
		t.Fatalf("saved = %+v", got)
	}
	if got.Config.Theme.Colors.Primary != "#ff0000" || got.Config.Analytics.GA4MeasurementID != "G-ABC1234" {
		t.Fatalf("normalized config = %+v", got.Config.Theme.Colors)
	}
	key, err := e.keys.GetKey(ctx, e.a.id, got.APIKey.UUID)
	e.must(err)
	if key.Kind != "publishable" || key.Channel != "web" || !slices.Equal(key.AllowedOrigins, []string{originA}) {
		t.Fatalf("site key = %+v", key)
	}
	keyA := key.UUID

	// The same version cannot be saved twice (concurrent editor).
	if _, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0)}); !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale save: %v", err)
	}

	// B cannot take A's subdomain.
	if _, err := e.svc.Save(ctx, e.b.id, e.b.slug, 0, model.SaveInput{Version: ptr(0), Subdomain: ptr(subA)}); !errors.Is(err, usecase.ErrSubdomainTaken) {
		t.Fatalf("taken subdomain: %v", err)
	}

	// A hand-added origin survives a subdomain change; the old site origin
	// is replaced.
	extra := []string{originA, "https://partner.example.com"}
	_, err = e.keys.PatchKey(ctx, e.a.id, keyA, pamodel.KeyInput{AllowedOrigins: &extra})
	e.must(err)
	got, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(1), Subdomain: ptr(subA + "-yeni")})
	e.must(err)
	key, err = e.keys.GetKey(ctx, e.a.id, keyA)
	e.must(err)
	if got.Version != 2 || got.APIKey.UUID != keyA ||
		!sameSet(key.AllowedOrigins, []string{"https://partner.example.com", "https://" + subA + "-yeni.sites.test"}) {
		t.Fatalf("after subdomain change: %+v origins %v", got, key.AllowedOrigins)
	}
	// Content saved earlier is kept when only the subdomain changes.
	if got.Config.Content["tr"].HeroTitle != "A firması" {
		t.Fatalf("config lost: %+v", got.Config.Content)
	}

	// A revoked key is replaced on the next save of an enabled site.
	_, err = e.keys.RevokeKey(ctx, e.a.id, keyA)
	e.must(err)
	got, err = e.svc.Get(ctx, e.a.id, e.a.slug)
	e.must(err)
	if got.BookingEnabled || got.APIKey == nil || got.APIKey.Status != "revoked" {
		t.Fatalf("revoked key view = %+v", got)
	}
	got, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(2)})
	e.must(err)
	if got.APIKey == nil || got.APIKey.UUID == keyA || !got.BookingEnabled {
		t.Fatalf("new key = %+v", got)
	}

	// A disabled site without a key creates none.
	got, err = e.svc.Save(ctx, e.b.id, e.b.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(false)})
	e.must(err)
	if got.APIKey != nil || got.BookingEnabled {
		t.Fatalf("disabled site key = %+v", got)
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, v := range a {
		if !slices.Contains(b, v) {
			return false
		}
	}
	return true
}

func TestResolveIsolation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	subA, subB := "alpha-"+e.a.slug[7:], "beta-"+e.b.slug[7:]
	a, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(subA), Config: themed("#ff0000", "Alfa")})
	e.must(err)
	b, err := e.svc.Save(ctx, e.b.id, e.b.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(subB), Config: themed("#00ff00", "Beta")})
	e.must(err)
	keyA, err := e.keys.GetKey(ctx, e.a.id, a.APIKey.UUID)
	e.must(err)
	keyB, err := e.keys.GetKey(ctx, e.b.id, b.APIKey.UUID)
	e.must(err)

	// Same image, two hosts, two looks: each host gets only its own site.
	ra, r := e.resolve(subA + ".sites.test")
	if r.code != http.StatusOK {
		t.Fatalf("resolve A = %d %s", r.code, r.raw)
	}
	rb, r := e.resolve(strings.ToUpper(subB) + ".Sites.Test.:443")
	if r.code != http.StatusOK {
		t.Fatalf("resolve B = %d %s", r.code, r.raw)
	}
	if ra.Organization.Slug != e.a.slug || ra.Config.Theme.Colors.Primary != "#ff0000" || ra.Config.Content["tr"].HeroTitle != "Alfa" ||
		ra.PublishableKey != *keyA.PublishableKey || ra.Origin != "https://"+subA+".sites.test" || !ra.BookingEnabled ||
		ra.PortalURL != "https://app.test/r/"+e.a.slug || ra.HostKind != model.HostSubdomain || ra.Version != 1 {
		t.Fatalf("A resolved = %+v", ra)
	}
	if rb.Organization.Slug != e.b.slug || rb.Config.Theme.Colors.Primary != "#00ff00" || rb.PublishableKey != *keyB.PublishableKey ||
		rb.Origin != "https://"+subB+".sites.test" {
		t.Fatalf("B resolved = %+v", rb)
	}
	if strings.Contains(string(mustJSON(t, ra)), e.b.slug) || strings.Contains(string(mustJSON(t, rb)), e.a.slug) {
		t.Fatal("a resolved site mentions the other organization")
	}

	// Unknown, nested, bare, malformed hosts: safe 404, never a fallback.
	for _, h := range []string{
		"unknown.sites.test", "x." + subA + ".sites.test", "sites.test", subA + ".sites.test.evil.com", subA + ".other.test",
		"%09x" + subA + ".sites.test", subA + "%2F.sites.test", "", "www.sites.test",
	} {
		if _, r := e.resolve(h); r.code != http.StatusNotFound && r.code != http.StatusBadRequest {
			t.Errorf("resolve %q = %d %s", h, r.code, r.raw)
		} else if h != "" && r.err.Code != handler.CodeSiteNotFound {
			t.Errorf("resolve %q code = %s", h, r.err.Code)
		}
	}

	// The resolved credential mints sessions for its own origin only, and
	// the data endpoints answer for that organization.
	s := e.call(http.MethodPost, "/v1/public/sessions", map[string]string{"publishable_key": ra.PublishableKey},
		map[string]string{"X-Forwarded-Origin": ra.Origin})
	if s.code != http.StatusCreated {
		t.Fatalf("session A = %d %s", s.code, s.raw)
	}
	var sess struct {
		Token        string `json:"token"`
		Organization struct {
			Slug string `json:"slug"`
		} `json:"organization"`
	}
	e.must(json.Unmarshal(s.data, &sess))
	if sess.Organization.Slug != e.a.slug {
		t.Fatalf("session org = %s", sess.Organization.Slug)
	}
	if r := e.call(http.MethodPost, "/v1/public/sessions", map[string]string{"publishable_key": ra.PublishableKey},
		map[string]string{"X-Forwarded-Origin": rb.Origin}); r.code != http.StatusForbidden {
		t.Fatalf("A's key from B's origin = %d %s", r.code, r.raw)
	}
	auth := map[string]string{"Authorization": "Bearer " + sess.Token, "X-Forwarded-Origin": ra.Origin}
	if r := e.call(http.MethodGet, "/v1/public/locations", nil, auth); r.code != http.StatusOK {
		t.Fatalf("locations with A's session = %d %s", r.code, r.raw)
	}
	auth["X-Forwarded-Origin"] = rb.Origin
	if r := e.call(http.MethodGet, "/v1/public/locations", nil, auth); r.code != http.StatusForbidden {
		t.Fatalf("A's session on B's host = %d %s", r.code, r.raw)
	}

	// Web channel off: the site renders, booking is off.
	e.setWeb(e.b.id, false)
	if rb, _ := e.resolve(subB + ".sites.test"); rb.BookingEnabled || rb.PublishableKey != "" {
		t.Fatalf("channel off = %+v", rb)
	}

	// Disabled site and suspended organization read as not found.
	_, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(1), Enabled: ptr(false)})
	e.must(err)
	if _, r := e.resolve(subA + ".sites.test"); r.code != http.StatusNotFound {
		t.Fatalf("disabled site = %d", r.code)
	}
	_, err = e.pool.Exec(ctx, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, e.b.id)
	e.must(err)
	if _, r := e.resolve(subB + ".sites.test"); r.code != http.StatusNotFound {
		t.Fatalf("suspended organization = %d", r.code)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCustomDomainHookAndAssets(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.svc.SetCustomDomains(e.domains)
	e.domains.byHost["www.brand-b.test"] = e.b.id

	b, err := e.svc.Save(ctx, e.b.id, e.b.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Config: themed("#123456", "Marka")})
	e.must(err)
	if len(b.Hosts) != 2 || b.Hosts[1].Origin != "https://www.brand-b.test" {
		t.Fatalf("hosts = %+v", b.Hosts)
	}
	key, err := e.keys.GetKey(ctx, e.b.id, b.APIKey.UUID)
	e.must(err)
	if !slices.Contains(key.AllowedOrigins, "https://www.brand-b.test") || !slices.Contains(key.AllowedOrigins, b.URL) {
		t.Fatalf("key origins = %v", key.AllowedOrigins)
	}
	r, ans := e.resolve("WWW.brand-b.test")
	if ans.code != http.StatusOK || r.Organization.Slug != e.b.slug || r.HostKind != model.HostCustom || r.Origin != "https://www.brand-b.test" {
		t.Fatalf("custom domain = %d %+v", ans.code, r)
	}
	if _, ans := e.resolve("www.brand-a.test"); ans.code != http.StatusNotFound {
		t.Fatalf("unverified domain = %d", ans.code)
	}

	// Images: none until uploaded; the logo falls back to the organization's.
	if r.Assets.Logo != nil || r.Assets.Cover != nil {
		t.Fatalf("assets = %+v", r.Assets)
	}
	logoKey := "organizations/" + uuid.NewString() + "/logo.png"
	e.must(e.store.Upload(ctx, storage.File{Body: strings.NewReader("\x89PNG-logo"), ContentType: "image/png"}, logoKey))
	_, err = e.pool.Exec(ctx, `UPDATE organizations SET logo_object_key = $2 WHERE id = $1`, e.b.id, logoKey)
	e.must(err)
	r, _ = e.resolve(b.Subdomain + ".sites.test")
	if r.Assets.Logo == nil {
		t.Fatal("logo path missing")
	}
	res, err := http.Get(e.server.URL + *r.Assets.Logo)
	e.must(err)
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "\x89PNG-logo" || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("logo = %d %q %s", res.StatusCode, body, res.Header.Get("Content-Type"))
	}
	for _, p := range []string{
		"/v1/public/site/assets/" + r.SiteUUID.String() + "/cover",
		"/v1/public/site/assets/" + r.SiteUUID.String() + "/secret",
		"/v1/public/site/assets/" + uuid.NewString() + "/logo",
	} {
		if res, err := http.Get(e.server.URL + p); err != nil || res.StatusCode != http.StatusNotFound {
			t.Errorf("%s = %v %v", p, res.StatusCode, err)
		}
	}
}

func TestHostHelpers(t *testing.T) {
	base, err := usecase.ParseBaseURL("http://localhost:3100")
	if err != nil {
		t.Fatal(err)
	}
	if got := base.Origin("acme"); got != "http://acme.localhost:3100" {
		t.Fatalf("origin = %s", got)
	}
	if norm, ok := pausecase.NormalizeOrigin(base.Origin("acme")); !ok || norm != "http://acme.localhost:3100" {
		t.Fatalf("dev origin refused by the public API: %q %v", norm, ok)
	}
	for in, want := range map[string]string{
		"Acme.Localhost:3100": "acme.localhost", "acme.localhost.": "acme.localhost", "[::1]:80": "", "a b.test": "", "a..b": "",
	} {
		got, ok := usecase.NormalizeHost(in)
		if (want == "") == ok || got != want {
			t.Errorf("NormalizeHost(%q) = %q %v", in, got, ok)
		}
	}
	if _, err := usecase.ParseBaseURL("ftp://x"); err == nil {
		t.Error("ftp base accepted")
	}
	for slug, want := range map[string]string{"Rent_A Car": "rent-a-car", "www": "site-www", "--x--": "x"} {
		if got := usecase.SubdomainFromSlug(slug); got != want {
			t.Errorf("SubdomainFromSlug(%q) = %q", slug, got)
		}
	}
}
