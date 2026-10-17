package site_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	pamodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/google/uuid"
)

// TEC-730: draft / publish, history, preview tokens, images, the catalog
// publish state and the site key renewal.

// pngBytes is a minimal PNG header (sniffed as image/png).
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), bytes.Repeat([]byte{1}, 64)...)

func upload(body []byte, contentType string) usecase.ImageUpload {
	return usecase.ImageUpload{Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: contentType, Filename: "x"}
}

func (e *env) orgUUID(o org) uuid.UUID {
	e.t.Helper()
	var id uuid.UUID
	e.must(e.pool.QueryRow(context.Background(), `SELECT uuid FROM organizations WHERE id = $1`, o.id).Scan(&id))
	return id
}

func titled(title string) *model.Config {
	c := model.DefaultConfig()
	tr := c.Content["tr"]
	tr.HeroTitle = title
	c.Content["tr"] = tr
	return &c
}

func TestDraftPublishAndHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := "pub-" + e.a.slug[7:]
	host := sub + ".sites.test"

	// Publishing a never-saved site publishes its defaults (stored first).
	got, rev, err := e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(0)})
	e.must(err)
	if rev != 1 || got.PublishedRevision == nil || *got.PublishedRevision != 1 || got.HasUnpublishedChanges || got.Live {
		t.Fatalf("first publish = %d %+v", rev, got)
	}
	// Nothing changed: nothing to publish.
	if _, _, err := e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version)}); !errors.Is(err, usecase.ErrNothingToPublish) {
		t.Fatalf("empty publish: %v", err)
	}
	if _, _, err := e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{}); !hasField(err, "version", "required") {
		t.Fatalf("publish without version: %v", err)
	}

	// Live fields apply on save; the document is a draft.
	got, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(got.Version), Enabled: ptr(true), Subdomain: ptr(sub), Config: titled("Birinci")})
	e.must(err)
	if !got.Live || !got.HasUnpublishedChanges {
		t.Fatalf("after save = %+v", got)
	}
	r, ans := e.resolve(host)
	if ans.code != http.StatusOK || r.Config.Content["tr"].HeroTitle != "" || r.Version != 1 || r.Preview {
		t.Fatalf("visitors see the draft before publishing: %d %+v", ans.code, r.Config.Content["tr"])
	}

	// A stale publish is refused; the current one goes live as revision 2.
	if _, _, err := e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version - 1)}); !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale publish: %v", err)
	}
	got, rev, err = e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version)})
	e.must(err)
	if rev != 2 || got.HasUnpublishedChanges {
		t.Fatalf("second publish = %d %+v", rev, got)
	}
	if r, _ := e.resolve(host); r.Config.Content["tr"].HeroTitle != "Birinci" || r.Version != 2 {
		t.Fatalf("published = %+v", r)
	}

	// Edit again: the resolver keeps serving revision 2.
	got, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(got.Version), Config: titled("İkinci")})
	e.must(err)
	if r, _ := e.resolve(host); r.Config.Content["tr"].HeroTitle != "Birinci" {
		t.Fatalf("draft leaked to visitors: %+v", r.Config.Content["tr"])
	}
	got, _, err = e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version)})
	e.must(err)

	versions, err := e.svc.Versions(ctx, e.a.id)
	e.must(err)
	if len(versions) != 3 || versions[0].Revision != 3 || !versions[0].Current || versions[2].Current {
		t.Fatalf("history = %+v", versions)
	}

	// Restore revision 2 into the draft; it is live only after publishing.
	if _, _, err := e.svc.Restore(ctx, e.a.id, e.a.slug, 0, versions[1].UUID, model.VersionInput{Version: ptr(got.Version + 5)}); !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale restore: %v", err)
	}
	got, _, err = e.svc.Restore(ctx, e.a.id, e.a.slug, 0, versions[1].UUID, model.VersionInput{Version: ptr(got.Version)})
	e.must(err)
	if got.Config.Content["tr"].HeroTitle != "Birinci" || !got.HasUnpublishedChanges {
		t.Fatalf("restored draft = %+v", got)
	}
	if r, _ := e.resolve(host); r.Config.Content["tr"].HeroTitle != "İkinci" {
		t.Fatalf("restore went live without publishing: %+v", r.Config.Content["tr"])
	}
	_, rev, err = e.svc.Publish(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version)})
	e.must(err)
	if r, _ := e.resolve(host); rev != 4 || r.Config.Content["tr"].HeroTitle != "Birinci" || r.Version != 4 {
		t.Fatalf("rolled back = %d %+v", rev, r)
	}

	// Another organization's revision is not found, whatever the version.
	_, _, err = e.svc.Publish(ctx, e.b.id, e.b.slug, 0, model.VersionInput{Version: ptr(0)})
	e.must(err)
	bVersions, err := e.svc.Versions(ctx, e.b.id)
	e.must(err)
	if _, _, err := e.svc.Restore(ctx, e.a.id, e.a.slug, 0, bVersions[0].UUID, model.VersionInput{Version: ptr(got.Version)}); !errors.Is(err, usecase.ErrRevisionNotFound) {
		t.Fatalf("foreign revision: %v", err)
	}

	// Switching off is live: the site stops resolving at once.
	cur, err := e.svc.Get(ctx, e.a.id, e.a.slug)
	e.must(err)
	_, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(cur.Version), Enabled: ptr(false)})
	e.must(err)
	if _, ans := e.resolve(host); ans.code != http.StatusNotFound {
		t.Fatalf("disabled site = %d", ans.code)
	}
}

func TestPreviewToken(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	subA, subB := "pva-"+e.a.slug[7:], "pvb-"+e.b.slug[7:]
	a, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(subA), Config: titled("Yayında")})
	e.must(err)
	e.publish(e.a, a.Version)
	_, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(a.Version), Config: titled("Taslak")})
	e.must(err)
	// B is off and never published: only a preview shows it.
	_, err = e.svc.Save(ctx, e.b.id, e.b.slug, 0, model.SaveInput{Version: ptr(0), Subdomain: ptr(subB), Config: titled("B taslak")})
	e.must(err)

	pa, err := e.svc.IssuePreview(ctx, e.a.id, e.a.slug, 0)
	e.must(err)
	if pa.URL != "https://"+subA+".sites.test/?preview="+pa.Token || time.Until(pa.ExpiresAt) > usecase.PreviewTTL+time.Minute {
		t.Fatalf("preview = %+v", pa)
	}
	pb, err := e.svc.IssuePreview(ctx, e.b.id, e.b.slug, 0)
	e.must(err)

	previewOf := func(host, token string) (model.Resolved, answer) {
		r := e.call(http.MethodGet, "/v1/public/site/resolve?host="+host+"&preview="+url.QueryEscape(token), nil, nil)
		var out model.Resolved
		if r.code == http.StatusOK {
			e.must(jsonUnmarshal(r.data, &out))
		}
		return out, r
	}
	r, ans := previewOf(subA+".sites.test", pa.Token)
	if ans.code != http.StatusOK || !r.Preview || r.Config.Content["tr"].HeroTitle != "Taslak" || r.PanelOrigin != "https://app.test" {
		t.Fatalf("A preview = %d %+v", ans.code, r)
	}
	if r, _ := e.resolve(subA + ".sites.test"); r.Config.Content["tr"].HeroTitle != "Yayında" || r.Preview || r.PanelOrigin != "" {
		t.Fatalf("A without token = %+v", r)
	}
	if r, ans := previewOf(subB+".sites.test", pb.Token); ans.code != http.StatusOK || r.Config.Content["tr"].HeroTitle != "B taslak" {
		t.Fatalf("B preview of an unpublished, disabled site = %d %+v", ans.code, r)
	}

	// Another organization's token, a tampered or an expired one: refused.
	tampered := pa.Token[:len(pa.Token)-2] + "xx"
	// The same signature on another site's id.
	parts := strings.Split(pa.Token, ".")
	bSite, _ := previewOf(subB+".sites.test", pb.Token)
	forged := strings.Join([]string{parts[0], bSite.SiteUUID.String(), parts[2], parts[3]}, ".")
	for name, c := range map[string][2]string{
		"A's token on B": {subB + ".sites.test", pa.Token}, "B's token on A": {subA + ".sites.test", pb.Token},
		"tampered": {subA + ".sites.test", tampered}, "garbage": {subA + ".sites.test", "sp1.x.y.z"}, "forged": {subB + ".sites.test", forged},
	} {
		if _, ans := previewOf(c[0], c[1]); ans.code != http.StatusUnauthorized || ans.err.Code != handler.CodePreviewInvalid {
			t.Errorf("%s = %d %s", name, ans.code, ans.raw)
		}
	}
	e.svc.SetClock(func() time.Time { return time.Now().Add(usecase.PreviewTTL + time.Minute) })
	if _, ans := previewOf(subA+".sites.test", pa.Token); ans.code != http.StatusUnauthorized {
		t.Fatalf("expired token = %d", ans.code)
	}
	e.svc.SetClock(time.Now)
	// Another signing secret (another deployment) does not verify.
	e.svc.SetPreviewSecret(strings.Repeat("q", 32))
	if _, ans := previewOf(subA+".sites.test", pa.Token); ans.code != http.StatusUnauthorized {
		t.Fatalf("token of another secret = %d", ans.code)
	}
}

func TestImages(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := "img-" + e.a.slug[7:]
	orgA := e.orgUUID(e.a)
	got, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(sub)})
	e.must(err)

	// Type and size are checked from the bytes: SVG (script carrier), HTML
	// declared as PNG and oversized files are refused.
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	for name, c := range map[string]struct {
		body []byte
		ct   string
		code string
	}{
		"svg":         {svg, "image/svg+xml", "invalid_type"},
		"html as png": {[]byte("<html><script>alert(1)</script></html>"), "image/png", "invalid_type"},
		"empty":       {nil, "image/png", "required"},
		"too large":   {append(append([]byte{}, pngBytes...), bytes.Repeat([]byte{0}, 1<<20)...), "image/png", "too_large"},
	} {
		kind := "logo"
		if name == "too large" {
			kind = "favicon"
		}
		if _, err := e.svc.SetImage(ctx, e.a.id, orgA, e.a.slug, 0, kind, ptr(got.Version), upload(c.body, c.ct)); !hasField(err, "file", c.code) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.svc.SetImage(ctx, e.a.id, orgA, e.a.slug, 0, "logo", ptr(got.Version+1), upload(pngBytes, "image/png")); !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale upload: %v", err)
	}

	// An upload is a draft change: visitors do not see it until publish.
	got, err = e.svc.SetImage(ctx, e.a.id, orgA, e.a.slug, 0, "cover", ptr(got.Version), upload(pngBytes, "image/png"))
	e.must(err)
	if !got.Assets.Cover || !got.HasUnpublishedChanges {
		t.Fatalf("after upload = %+v", got)
	}
	e.publish(e.a, got.Version)
	r, _ := e.resolve(sub + ".sites.test")
	if r.Assets.Cover == nil {
		t.Fatal("published cover missing")
	}
	firstKey := e.coverKey(e.a)
	if !strings.HasPrefix(firstKey, "organizations/"+orgA.String()+"/site/cover-") {
		t.Fatalf("object key %q is not under the organization's prefix", firstKey)
	}

	// Replacing the draft keeps the published object (revision 1 shows it).
	other := append(append([]byte{}, pngBytes...), 'x')
	got, err = e.svc.SetImage(ctx, e.a.id, orgA, e.a.slug, 0, "cover", ptr(got.Version), upload(other, "image/png"))
	e.must(err)
	if _, _, err := e.store.Download(ctx, firstKey); err != nil {
		t.Fatalf("published object deleted: %v", err)
	}
	secondKey := e.coverKey(e.a)
	get := func(path string) (int, string) {
		res, err := http.Get(e.server.URL + path)
		e.must(err)
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, body := get(*r.Assets.Cover); code != http.StatusOK || body != string(pngBytes) {
		t.Fatalf("visitors get the draft image: %d", code)
	}
	p, err := e.svc.IssuePreview(ctx, e.a.id, e.a.slug, 0)
	e.must(err)
	if p.Assets.Cover == nil {
		t.Fatal("preview cover path missing")
	}
	if code, body := get(*p.Assets.Cover); code != http.StatusOK || body != string(other) {
		t.Fatalf("preview image = %d", code)
	}
	draftOnly := strings.Split(*p.Assets.Cover, "&preview=")[0]
	if code, body := get(draftOnly); code != http.StatusOK || body != string(pngBytes) {
		t.Fatalf("draft path without token = %d (must serve the published image)", code)
	}

	// Replacing an unpublished draft image deletes it; removing clears it.
	got, err = e.svc.SetImage(ctx, e.a.id, orgA, e.a.slug, 0, "cover", ptr(got.Version), upload(pngBytes, "image/png"))
	e.must(err)
	if _, _, err := e.store.Download(ctx, secondKey); err == nil {
		t.Fatal("unreferenced draft image kept")
	}
	got, err = e.svc.RemoveImage(ctx, e.a.id, e.a.slug, 0, "cover", ptr(got.Version))
	e.must(err)
	if got.Assets.Cover {
		t.Fatalf("removed cover = %+v", got.Assets)
	}
	if _, _, err := e.store.Download(ctx, firstKey); err != nil {
		t.Fatalf("published object deleted on remove: %v", err)
	}
}

func (e *env) coverKey(o org) string {
	e.t.Helper()
	var key string
	e.must(e.pool.QueryRow(context.Background(), `SELECT cover_object_key FROM site_settings WHERE organization_id = $1`, o.id).Scan(&key))
	return key
}

// TestLegacyRowStaysPublished runs the 000124 backfill on a row saved the
// old way (live on save, no revision): it must stay live unchanged.
func TestLegacyRowStaysPublished(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := "legacy-" + e.a.slug[7:]
	_, err := e.pool.Exec(ctx, `INSERT INTO site_settings (organization_id, enabled, subdomain, config)
		VALUES ($1, TRUE, $2, '{"theme":{"font":"lato"},"content":{"tr":{"hero_title":"Eski"}}}')`, e.a.id, sub)
	e.must(err)
	if _, ans := e.resolve(sub + ".sites.test"); ans.code != http.StatusNotFound {
		t.Fatalf("before backfill = %d", ans.code)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000124_site_publish.up.sql"))
	e.must(err)
	_, rest, ok := strings.Cut(string(raw), "-- backfill:begin")
	backfill, _, ok2 := strings.Cut(rest, "-- backfill:end")
	if !ok || !ok2 {
		t.Fatal("backfill markers missing in 000124")
	}
	_, err = e.pool.Exec(ctx, backfill)
	e.must(err)
	r, ans := e.resolve(sub + ".sites.test")
	if ans.code != http.StatusOK || r.Config.Theme.Font != "lato" || r.Config.Content["tr"].HeroTitle != "Eski" || r.Version != 1 {
		t.Fatalf("legacy site after backfill = %d %+v", ans.code, r)
	}
	got, err := e.svc.Get(ctx, e.a.id, e.a.slug)
	e.must(err)
	if got.HasUnpublishedChanges || got.PublishedRevision == nil || !got.Live {
		t.Fatalf("legacy settings = %+v", got)
	}
	// Running it again changes nothing.
	_, err = e.pool.Exec(ctx, backfill)
	e.must(err)
	if v, _ := e.svc.Versions(ctx, e.a.id); len(v) != 1 {
		t.Fatalf("backfill is not idempotent: %+v", v)
	}
}

func TestCatalogOnlineState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := "cat-" + e.a.slug[7:]
	a, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(sub)})
	e.must(err)
	e.publish(e.a, a.Version)
	var shown, hidden, bBranch, product, extraAll, extraWeb uuid.UUID
	e.must(e.pool.QueryRow(ctx, `INSERT INTO branches (organization_id, code, name) VALUES ($1, 'S1', 'Görünen') RETURNING uuid`, e.a.id).Scan(&shown))
	e.must(e.pool.QueryRow(ctx, `INSERT INTO branches (organization_id, code, name) VALUES ($1, 'S2', 'Gizli') RETURNING uuid`, e.a.id).Scan(&hidden))
	e.must(e.pool.QueryRow(ctx, `INSERT INTO branches (organization_id, code, name) VALUES ($1, 'S1', 'B şubesi') RETURNING uuid`, e.b.id).Scan(&bBranch))
	var segment int64
	e.must(e.pool.QueryRow(ctx, `INSERT INTO vehicle_segments (organization_id, code, name) VALUES ($1, 'ECO', 'Ekonomi') RETURNING id`, e.a.id).Scan(&segment))
	e.must(e.pool.QueryRow(ctx, `INSERT INTO rental_products (organization_id, code, title, segment_id, status) VALUES ($1, 'P1', 'Ekonomik', $2, 'published') RETURNING uuid`, e.a.id, segment).Scan(&product))
	_, err = e.pool.Exec(ctx, `INSERT INTO rental_product_versions (organization_id, product_id, version, guarantee_type)
		SELECT organization_id, id, 1, 'EXACT_VEHICLE' FROM rental_products WHERE uuid = $1`, product)
	e.must(err)
	e.must(e.pool.QueryRow(ctx, `INSERT INTO extras (organization_id, code, name, extra_type, charge_type, status) VALUES ($1, 'E1', 'Bebek koltuğu', 'child_seat', 'per_day', 'published') RETURNING uuid`, e.a.id).Scan(&extraAll))
	e.must(e.pool.QueryRow(ctx, `INSERT INTO extras (organization_id, code, name, extra_type, charge_type, status, channels) VALUES ($1, 'E2', 'Ek sürücü', 'additional_driver', 'per_rental', 'published', '{web,branch}') RETURNING uuid`, e.a.id).Scan(&extraWeb))

	cat, err := e.svc.Catalog(ctx, e.a.id)
	e.must(err)
	if len(cat.Branches) != 2 || len(cat.Subgroups) != 1 || len(cat.Extras) != 2 || !cat.Branches[0].ShowOnline || !cat.Extras[0].ShowOnline {
		t.Fatalf("catalog = %+v", cat)
	}
	// Body is checked after the lookup; another organization's record is
	// not found.
	if _, err := e.svc.SetBranchOnline(ctx, e.a.id, bBranch, model.VisibilityInput{ShowOnline: ptr(false)}); !errors.Is(err, usecase.ErrCatalogItemNotFound) {
		t.Fatalf("foreign branch: %v", err)
	}
	if _, err := e.svc.SetBranchOnline(ctx, e.a.id, hidden, model.VisibilityInput{}); !hasField(err, "show_online", "required") {
		t.Fatalf("no show_online: %v", err)
	}
	b, err := e.svc.SetBranchOnline(ctx, e.a.id, hidden, model.VisibilityInput{ShowOnline: ptr(false)})
	e.must(err)
	if b.ShowOnline {
		t.Fatalf("branch = %+v", b)
	}
	if _, err := e.svc.SetSubgroupOnline(ctx, e.a.id, product, model.VisibilityInput{ShowOnline: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	// Extras: the storefront flag; their sales channels stay as they are.
	x, err := e.svc.SetExtraOnline(ctx, e.a.id, extraAll, model.VisibilityInput{ShowOnline: ptr(false)})
	e.must(err)
	var channels []string
	e.must(e.pool.QueryRow(ctx, `SELECT COALESCE(channels::text[], '{}') FROM extras WHERE uuid = $1`, extraAll).Scan(&channels))
	if x.ShowOnline || len(channels) != 0 {
		t.Fatalf("extra off = %+v channels %v", x, channels)
	}
	if _, err := e.svc.SetExtraOnline(ctx, e.b.id, extraAll, model.VisibilityInput{ShowOnline: ptr(true)}); !errors.Is(err, usecase.ErrCatalogItemNotFound) {
		t.Fatalf("foreign extra: %v", err)
	}
	x, err = e.svc.SetExtraOnline(ctx, e.a.id, extraWeb, model.VisibilityInput{ShowOnline: ptr(true)})
	e.must(err)
	if !x.ShowOnline {
		t.Fatalf("extra on = %+v", x)
	}

	// The public API respects it: listings skip, availability and quotes
	// refuse a hidden branch / subgroup.
	r, _ := e.resolve(sub + ".sites.test")
	s := e.call(http.MethodPost, "/v1/public/sessions", map[string]string{"publishable_key": r.PublishableKey},
		map[string]string{"X-Forwarded-Origin": r.Origin})
	if s.code != http.StatusCreated {
		t.Fatalf("session = %d %s", s.code, s.raw)
	}
	var sess struct {
		Token string `json:"token"`
	}
	e.must(jsonUnmarshal(s.data, &sess))
	auth := map[string]string{"Authorization": "Bearer " + sess.Token, "X-Forwarded-Origin": r.Origin}
	loc := e.call(http.MethodGet, "/v1/public/locations", nil, auth)
	if loc.code != http.StatusOK || !strings.Contains(string(loc.raw), shown.String()) || strings.Contains(string(loc.raw), hidden.String()) {
		t.Fatalf("locations = %d %s", loc.code, loc.raw)
	}
	ex := e.call(http.MethodGet, "/v1/public/extras", nil, auth)
	if strings.Contains(string(ex.raw), extraAll.String()) || !strings.Contains(string(ex.raw), extraWeb.String()) {
		t.Fatalf("extras = %s", ex.raw)
	}
	start := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Hour)
	q := url.Values{
		"pickup_branch_uuid": {hidden.String()}, "starts_at": {start.Format(time.RFC3339)},
		"ends_at": {start.Add(48 * time.Hour).Format(time.RFC3339)},
	}
	if av := e.call(http.MethodGet, "/v1/public/availability?"+q.Encode(), nil, auth); av.code != http.StatusBadRequest ||
		!strings.Contains(string(av.raw), "pickup_branch_uuid") {
		t.Fatalf("availability at a hidden branch = %d %s", av.code, av.raw)
	}
	quote := e.call(http.MethodPost, "/v1/public/quotes", map[string]any{
		"subgroup_uuid": product, "pickup_branch_uuid": shown, "starts_at": start, "ends_at": start.Add(48 * time.Hour),
	}, map[string]string{"Authorization": auth["Authorization"], "X-Forwarded-Origin": r.Origin, "Idempotency-Key": uuid.NewString()})
	if quote.code != http.StatusBadRequest || !strings.Contains(string(quote.raw), "subgroup_uuid") {
		t.Fatalf("quote for a hidden subgroup = %d %s", quote.code, quote.raw)
	}
	quote = e.call(http.MethodPost, "/v1/public/quotes", map[string]any{
		"subgroup_uuid": product, "pickup_branch_uuid": shown, "starts_at": start, "ends_at": start.Add(48 * time.Hour),
		"extras": []map[string]any{{"extra_uuid": extraAll, "quantity": 1}},
	}, map[string]string{"Authorization": auth["Authorization"], "X-Forwarded-Origin": r.Origin, "Idempotency-Key": uuid.NewString()})
	if quote.code != http.StatusBadRequest || !strings.Contains(string(quote.raw), "extras.0.extra_uuid") {
		t.Fatalf("quote with a hidden extra = %d %s", quote.code, quote.raw)
	}

	// Integrations with a secret server key ignore the storefront flag: the
	// hidden branch, subgroup and extra are listed and not refused.
	kind, channel, name := pamodel.KindSecret, "web", "ERP"
	scopes := slices.Clone(pamodel.AllScopes)
	issued, err := e.keys.CreateKey(ctx, e.a.id, pamodel.KeyInput{Kind: &kind, Name: &name, Channel: &channel, Scopes: &scopes})
	e.must(err)
	server := map[string]string{"Authorization": "Bearer " + issued.Secret}
	loc = e.call(http.MethodGet, "/v1/public/locations", nil, server)
	if loc.code != http.StatusOK || !strings.Contains(string(loc.raw), hidden.String()) {
		t.Fatalf("secret key locations = %d %s", loc.code, loc.raw)
	}
	if sg := e.call(http.MethodGet, "/v1/public/vehicle-subgroups", nil, server); sg.code != http.StatusOK ||
		!strings.Contains(string(sg.raw), product.String()) {
		t.Fatalf("secret key subgroups = %d %s", sg.code, sg.raw)
	}
	if sg := e.call(http.MethodGet, "/v1/public/vehicle-subgroups", nil, auth); strings.Contains(string(sg.raw), product.String()) {
		t.Fatalf("storefront subgroups list a hidden one: %s", sg.raw)
	}
	if ex := e.call(http.MethodGet, "/v1/public/extras", nil, server); !strings.Contains(string(ex.raw), extraAll.String()) {
		t.Fatalf("secret key extras = %s", ex.raw)
	}
	// Availability and quotes with a secret key: publicapi
	// TestStorefrontPublishState (it has the reservations engine).
}

func TestRenewKey(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := "key-" + e.a.slug[7:]
	got, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(sub)})
	e.must(err)
	e.publish(e.a, got.Version)
	oldKey := got.APIKey.UUID
	r1, _ := e.resolve(sub + ".sites.test")

	if _, err := e.svc.RenewKey(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version + 1)}); !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale renew: %v", err)
	}
	got, err = e.svc.RenewKey(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version)})
	e.must(err)
	if got.APIKey == nil || got.APIKey.UUID == oldKey || !got.BookingEnabled {
		t.Fatalf("renewed = %+v", got)
	}
	newKey, err := e.keys.GetKey(ctx, e.a.id, got.APIKey.UUID)
	e.must(err)
	old, err := e.keys.GetKey(ctx, e.a.id, oldKey)
	e.must(err)
	if old.ExpiresAt == nil || old.ExpiresAt.After(time.Now().Add(61*time.Minute)) || len(newKey.AllowedOrigins) != 1 ||
		newKey.AllowedOrigins[0] != "https://"+sub+".sites.test" {
		t.Fatalf("rotation: old %+v new %+v", old, newKey)
	}
	if r2, _ := e.resolve(sub + ".sites.test"); r2.PublishableKey == r1.PublishableKey || r2.PublishableKey == "" {
		t.Fatal("the site still books with the old key")
	}

	// A rotation in Ayarlar → Public API is followed on the next save.
	issued, err := e.keys.RotateKey(ctx, e.a.id, got.APIKey.UUID, pamodelRotate(24))
	e.must(err)
	got, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(got.Version)})
	e.must(err)
	if got.APIKey.UUID != issued.Key.UUID {
		t.Fatalf("site did not follow the rotated key: %v want %v", got.APIKey.UUID, issued.Key.UUID)
	}

	// A revoked key is replaced by a new one.
	_, err = e.keys.RevokeKey(ctx, e.a.id, got.APIKey.UUID)
	e.must(err)
	revoked := got.APIKey.UUID
	got, err = e.svc.RenewKey(ctx, e.a.id, e.a.slug, 0, model.VersionInput{Version: ptr(got.Version)})
	e.must(err)
	if got.APIKey.UUID == revoked || got.APIKey.Status != "active" {
		t.Fatalf("after revoke = %+v", got.APIKey)
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func pamodelRotate(hours int) pamodel.RotateInput { return pamodel.RotateInput{GraceHours: &hours} }
