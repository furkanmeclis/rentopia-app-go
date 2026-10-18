package site_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/themes"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/google/uuid"
)

func articleDocument(slug string) model.ArticleInput {
	return model.ArticleInput{Slug: map[string]string{"tr": slug, "en": slug + "-en"}, Title: map[string]string{"tr": "Rehber", "en": "Guide"},
		Summary: map[string]string{"tr": "Özet", "en": "Summary"}, Body: map[string]json.RawMessage{"tr": json.RawMessage(`[{"type":"p","children":[{"text":"İçerik","bold":true}]}]`), "en": json.RawMessage(`[{"type":"p","children":[{"text":"Content"}]}]`)}}
}

func TestArticleValidationDatabase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	nested := `{"text":"deep"}`
	for range 34 {
		nested = `{"type":"blockquote","children":[` + nested + `]}`
	}
	many := []string{}
	for range 5001 {
		many = append(many, `{"text":""}`)
	}
	cases := []struct{ name, body, field string }{
		{"unknown node", `[{"type":"script","children":[{"text":"bad"}]}]`, "body.tr.0.type"},
		{"unknown mark", `[{"type":"p","children":[{"text":"x","color":"red"}]}]`, "body.tr.0.children.0.color"},
		{"invalid mark", `[{"text":"x","bold":"yes"}]`, "body.tr.0.bold"},
		{"unknown key", `[{"type":"p","html":"<b>x</b>","children":[{"text":"x"}]}]`, "body.tr.0.html"},
		{"javascript", `[{"type":"a","href":"javascript:alert(1)","children":[{"text":"x"}]}]`, "body.tr.0.href"},
		{"data", `[{"type":"a","href":"data:text/html,x","children":[{"text":"x"}]}]`, "body.tr.0.href"},
		{"http", `[{"type":"a","href":"http://example.com","children":[{"text":"x"}]}]`, "body.tr.0.href"},
		{"protocol relative", `[{"type":"a","href":"//example.com","children":[{"text":"x"}]}]`, "body.tr.0.href"},
		{"encoded protocol relative", `[{"type":"a","href":"/%2fexample.com","children":[{"text":"x"}]}]`, "body.tr.0.href"},
		{"missing media", `[{"type":"img","alt":"Image","children":[{"text":""}]}]`, "body.tr.0.media_uuid"},
		{"wrong shape", `null`, "body.tr"},
		{"not array", `{"type":"p"}`, "body.tr"},
		{"size", `[{"text":"` + strings.Repeat("a", usecase.MaxArticleBodyBytes) + `"}]`, "body.tr"},
		{"depth", `[` + nested + `]`, "body.tr.0"},
		{"nodes", `[` + strings.Join(many, ",") + `]`, "body.tr.5000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := articleDocument("validation")
			in.Body["tr"] = json.RawMessage(tc.body)
			_, err := e.svc.CreateArticle(ctx, e.a.id, 0, in)
			var ve *usecase.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected validation, got %v", err)
			}
			found := false
			for _, d := range ve.Details {
				found = found || strings.HasPrefix(d.Field, tc.field)
			}
			if !found {
				t.Fatalf("missing path %s: %+v", tc.field, ve.Details)
			}
		})
	}
	for _, href := range []string{"/guide/tr/test", "guide/tr/test", "../guide/tr/test", "https://example.com/path?q=x", "mailto:test@example.com"} {
		in := articleDocument("safe-" + uuid.NewString())
		in.Body["tr"] = json.RawMessage(fmt.Sprintf(`[{"type":"a","href":%q,"children":[{"text":"link"}]}]`, href))
		_, err := e.svc.CreateArticle(ctx, e.a.id, 0, in)
		e.must(err)
	}
	// All supported elements and marks survive as JSON, including Plate lists.
	in := articleDocument("all-nodes")
	in.Body["tr"] = json.RawMessage(`[{"type":"ul","children":[{"type":"li","children":[{"type":"lic","children":[{"text":"List","italic":true,"underline":true,"strikethrough":false,"code":true}]}]}]},{"type":"h2","children":[{"text":"h"}]},{"type":"h3","children":[{"text":"h"}]},{"type":"h4","children":[{"text":"h"}]},{"type":"hr","children":[{"text":""}]},{"type":"ol","children":[{"type":"li","children":[{"text":"ordered"}]}]},{"type":"code_block","children":[{"type":"code_line","children":[{"text":"code"}]}]}]`)
	_, err := e.svc.CreateArticle(ctx, e.a.id, 0, in)
	e.must(err)
}

func TestArticleSlugsVersionsAndLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := articleDocument("same")
	a, err := e.svc.CreateArticle(ctx, e.a.id, 0, in)
	e.must(err)
	_, err = e.svc.CreateArticle(ctx, e.a.id, 0, in)
	if !errors.Is(err, usecase.ErrArticleSlugTaken) {
		t.Fatal("duplicate accepted", err)
	}
	_, err = e.svc.CreateArticle(ctx, e.b.id, 0, in)
	e.must(err)
	// Direct SQL cannot bypass the slug constraint or the inline-media guard.
	_, err = e.pool.Exec(ctx, `UPDATE site_articles SET slug='{"tr":"about","en":""}' WHERE organization_id=$1 AND uuid=$2`, e.a.id, a.UUID)
	if err == nil {
		t.Fatal("database accepted a reserved slug")
	}
	badBody := fmt.Sprintf(`{"tr":[{"type":"img","media_uuid":%q,"alt":"Image","children":[{"text":""}]}],"en":[]}`, uuid.NewString())
	_, err = e.pool.Exec(ctx, `UPDATE site_articles SET body=$3 WHERE organization_id=$1 AND uuid=$2`, e.a.id, a.UUID, []byte(badBody))
	if err == nil {
		t.Fatal("database accepted missing inline media")
	}

	in = articleDocument("other")
	in.Slug["en"] = "same-en"
	_, err = e.svc.CreateArticle(ctx, e.a.id, 0, in)
	if !errors.Is(err, usecase.ErrArticleSlugTaken) {
		t.Fatal("en duplicate accepted", err)
	}
	in = articleDocument("same-en")
	in.Slug["en"] = "same"
	_, err = e.svc.CreateArticle(ctx, e.a.id, 0, in)
	e.must(err) // uniqueness is per locale
	for _, slug := range []string{"about", "guide", "api", "vehicles", "branches", "contact", "UPPER", "has space", "-start", "end-", "a--b"} {
		in = articleDocument(slug)
		_, err = e.svc.CreateArticle(ctx, e.a.id, 0, in)
		var ve *usecase.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("slug %s: %v", slug, err)
		}
	}
	in = articleDocument("same")
	in.Version = ptr(a.Version + 1)
	_, err = e.svc.UpdateArticle(ctx, e.a.id, a.UUID, 0, in)
	if !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatal(err)
	}
	for _, method := range []string{"get", "update", "delete", "publish", "unpublish"} {
		switch method {
		case "get":
			_, err = e.svc.GetArticle(ctx, e.b.id, a.UUID)
		case "update":
			_, err = e.svc.UpdateArticle(ctx, e.b.id, a.UUID, 0, in)
		case "delete":
			err = e.svc.DeleteArticle(ctx, e.b.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)})
		default:
			_, err = e.svc.SetArticleStatus(ctx, e.b.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}, method == "publish")
		}
		if !errors.Is(err, usecase.ErrNotFound) {
			t.Fatalf("%s isolation: %v", method, err)
		}
	}
	in.Version = ptr(a.Version)
	in.Featured = true
	a, err = e.svc.UpdateArticle(ctx, e.a.id, a.UUID, 0, in)
	e.must(err)
	if a.Version != 2 || !a.Featured {
		t.Fatal(a)
	}
	a, err = e.svc.SetArticleStatus(ctx, e.a.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}, true)
	e.must(err)
	if a.Status != "published" || a.PublishedAt == nil || a.Version != 3 {
		t.Fatal(a)
	}
	_, err = e.svc.SetArticleStatus(ctx, e.a.id, a.UUID, 0, model.VersionInput{Version: ptr(2)}, false)
	if !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatal(err)
	}
	a, err = e.svc.SetArticleStatus(ctx, e.a.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}, false)
	e.must(err)
	if a.Status != "draft" || a.PublishedAt != nil {
		t.Fatal(a)
	}
	e.must(e.svc.DeleteArticle(ctx, e.a.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}))
	_, err = e.svc.GetArticle(ctx, e.a.id, a.UUID)
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal(err)
	}
	_, err = e.svc.CreateArticle(ctx, e.a.id, 0, articleDocument("same"))
	e.must(err)
	page, err := e.svc.ListArticles(ctx, db.ListSiteArticlesParams{OrganizationID: e.a.id, Status: "draft", Locale: "en", Q: "other-not-present", SortField: "created_at", SortDesc: true, PageLimit: 20})
	e.must(err)
	if page.Total != 0 || len(page.Items) != 0 {
		t.Fatal(page)
	}
}

func TestArticlesPublicAndSitemap(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	cfg := model.DefaultConfig()
	c := cfg.Content["tr"]
	c.About = "About us"
	c.Terms = "Terms"
	cfg.Content["tr"] = c
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Config: &cfg})
	e.must(err)
	e.publish(e.a, saved.Version)
	host := saved.Hosts[0].Host
	for _, o := range []org{e.a, e.b} {
		in := articleDocument("draft")
		_, err = e.svc.CreateArticle(ctx, o.id, 0, in)
		e.must(err)
		for i := range 4 {
			in = articleDocument(fmt.Sprintf("published-%d", i))
			a, err := e.svc.CreateArticle(ctx, o.id, 0, in)
			e.must(err)
			_, err = e.svc.SetArticleStatus(ctx, o.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}, true)
			e.must(err)
		}
	}
	var branch, hidden uuid.UUID
	var segment int64
	e.must(e.pool.QueryRow(ctx, `INSERT INTO branches(organization_id,code,name,city) VALUES($1,'PUBLIC','Public office','Istanbul') RETURNING uuid`, e.a.id).Scan(&branch))
	e.must(e.pool.QueryRow(ctx, `INSERT INTO branches(organization_id,code,name,city,show_online) VALUES($1,'HIDDEN','Hidden office','Istanbul',false) RETURNING uuid`, e.a.id).Scan(&hidden))
	e.must(e.pool.QueryRow(ctx, `INSERT INTO vehicle_segments(organization_id,code,name) VALUES($1,'ECON','Economic class') RETURNING id`, e.a.id).Scan(&segment))
	_, err = e.pool.Exec(ctx, `INSERT INTO rental_products(organization_id,code,title,segment_id,status) VALUES($1,'CAR','Car',$2,'published')`, e.a.id, segment)
	e.must(err)
	_, err = e.pool.Exec(ctx, `INSERT INTO rental_product_versions(organization_id,product_id,version,guarantee_type)
 SELECT organization_id,id,1,'EXACT_VEHICLE' FROM rental_products WHERE organization_id=$1 AND code='CAR'`, e.a.id)
	e.must(err)
	page, err := e.svc.PublicArticles(ctx, host, "tr", 2, 1)
	e.must(err)
	if page.Total != 4 || len(page.Items) != 2 || page.Items[0].Slug != "published-2" {
		t.Fatal(page)
	}
	resolved, err := e.svc.Resolve(ctx, host, "")
	e.must(err)
	if len(resolved.Articles["tr"]) != 3 || resolved.Articles["tr"][0].Slug != "published-3" || len(resolved.Articles["en"]) != 3 {
		t.Fatal(resolved.Articles)
	}
	for _, status := range themes.Statuses(themes.Default) {
		if status.ID == "guide" && status.Status != "available" {
			t.Fatal(status)
		}
	}
	for _, slug := range []string{"draft", "missing"} {
		_, err = e.svc.PublicArticle(ctx, host, "tr", slug)
		if !errors.Is(err, usecase.ErrNotFound) {
			t.Fatal(slug, err)
		}
	}
	a, err := e.svc.PublicArticle(ctx, host, "en", "published-0-en")
	e.must(err)
	if a.Title != "Guide" || !bytes.Contains(a.Body, []byte("Content")) {
		t.Fatal(a)
	}
	entries, err := e.svc.Sitemap(ctx, host)
	e.must(err)
	paths := map[string]bool{}
	for _, entry := range entries {
		paths[entry.Path] = true
		if entry.Lastmod.IsZero() {
			t.Fatal(entry)
		}
	}
	for _, path := range []string{"/", "/about", "/terms", "/branches/" + branch.String(), "/vehicles?segment=Economic+class", "/guide/tr/published-0", "/guide/en/published-0-en"} {
		if !paths[path] {
			t.Fatal("missing", path, entries)
		}
	}
	for _, path := range []string{"/privacy", "/faq", "/branches/" + hidden.String(), "/guide/tr/draft"} {
		if paths[path] {
			t.Fatal("unexpected", path)
		}
	}
	// HTTP public routes use the requested host, never a caller-supplied org ID.
	for _, path := range []string{"/v1/public/site/articles?locale=tr&limit=1&offset=1&host=" + host, "/v1/public/site/articles/by-slug/tr/published-0?host=" + host, "/v1/public/site/sitemap?host=" + host} {
		ans := e.call(http.MethodGet, path, nil, nil)
		if ans.code != 200 || bytes.Contains(ans.raw, []byte("organization_id")) {
			t.Fatal(ans.code, string(ans.raw))
		}
	}
	ans := e.call(http.MethodGet, "/v1/public/site/articles/by-slug/tr/draft?host="+host, nil, nil)
	if ans.code != 404 {
		t.Fatal(ans.code)
	}
	ans = e.call(http.MethodGet, "/v1/public/site/articles?locale=xx&host="+host, nil, nil)
	if ans.code != 400 {
		t.Fatal(ans.code)
	}
	// A unique foreign slug must be unknown, even if B's own site is published.
	foreign, err := e.svc.CreateArticle(ctx, e.b.id, 0, articleDocument("foreign-only"))
	e.must(err)
	_, err = e.svc.SetArticleStatus(ctx, e.b.id, foreign.UUID, 0, model.VersionInput{Version: ptr(foreign.Version)}, true)
	e.must(err)
	_, err = e.svc.PublicArticle(ctx, host, "tr", "foreign-only")
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatal(err)
	}
	for _, state := range []string{"disabled", "suspended", "unpublished", "access"} {
		switch state {
		case "disabled":
			_, err = e.pool.Exec(ctx, `UPDATE site_settings SET enabled=false WHERE organization_id=$1`, e.a.id)
		case "suspended":
			_, err = e.pool.Exec(ctx, `UPDATE site_settings SET enabled=true WHERE organization_id=$1`, e.a.id)
			e.must(err)
			_, err = e.pool.Exec(ctx, `UPDATE organizations SET status='suspended' WHERE id=$1`, e.a.id)
		case "unpublished":
			_, err = e.pool.Exec(ctx, `UPDATE organizations SET status='active' WHERE id=$1`, e.a.id)
			e.must(err)
			_, err = e.pool.Exec(ctx, `UPDATE site_settings SET published_revision=NULL WHERE organization_id=$1`, e.a.id)
		case "access":
			_, err = e.pool.Exec(ctx, `UPDATE site_settings SET published_revision=1 WHERE organization_id=$1`, e.a.id)
			e.must(err)
			_, err = e.pool.Exec(ctx, `UPDATE organizations SET access_ends_at=now()-interval '1 hour' WHERE id=$1`, e.a.id)
		}
		e.must(err)
		for _, path := range []string{"/v1/public/site/articles", "/v1/public/site/articles/by-slug/tr/published-0", "/v1/public/site/sitemap"} {
			ans = e.call(http.MethodGet, path+"?host="+host, nil, nil)
			if ans.code != 404 {
				t.Fatal(state, path, ans.code)
			}
		}
	}
}

func TestArticleMediaReferencesAndTenantFile(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	cover := e.uploadMedia(e.a)
	inline := e.uploadMedia(e.a)
	foreign := e.uploadMedia(e.b)
	in := articleDocument("article-media")
	in.CoverMediaUUID = &foreign.UUID
	_, err := e.svc.CreateArticle(ctx, e.a.id, 0, in)
	var ve *usecase.ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("foreign cover", err)
	}
	in.CoverMediaUUID = &cover.UUID
	in.Body["tr"] = json.RawMessage(fmt.Sprintf(`[{"type":"img","media_uuid":%q,"alt":"Car","children":[{"text":""}]}]`, foreign.UUID))
	_, err = e.svc.CreateArticle(ctx, e.a.id, 0, in)
	if !errors.As(err, &ve) {
		t.Fatal("foreign inline", err)
	}
	in.Body["tr"] = json.RawMessage(fmt.Sprintf(`[{"type":"img","media_uuid":%q,"alt":"Car","children":[{"text":""}]}]`, inline.UUID))
	a, err := e.svc.CreateArticle(ctx, e.a.id, 0, in)
	e.must(err)
	_, err = e.pool.Exec(ctx, `UPDATE site_articles SET cover_media_uuid=$3 WHERE organization_id=$1 AND uuid=$2`, e.a.id, a.UUID, foreign.UUID)
	if err == nil {
		t.Fatal("FK accepted foreign cover")
	}
	a, err = e.svc.SetArticleStatus(ctx, e.a.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}, true)
	e.must(err)
	for _, m := range []model.Media{cover, inline} {
		if err = e.svc.DeleteMedia(ctx, e.a.id, m.UUID); !errors.Is(err, usecase.ErrMediaInUse) {
			t.Fatal("published media unprotected", err)
		}
	}
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true)})
	e.must(err)
	e.publish(e.a, saved.Version)
	resolved, err := e.svc.Resolve(ctx, saved.Hosts[0].Host, "")
	e.must(err)
	for _, m := range []model.Media{cover, inline} {
		rc, _, err := e.svc.PublicMedia(ctx, resolved.SiteUUID, m.UUID, "")
		e.must(err)
		_ = rc.Close()
	}
	public, err := e.svc.PublicArticle(ctx, saved.Hosts[0].Host, "tr", "article-media")
	e.must(err)
	if public.CoverURL == nil || public.Media[inline.UUID.String()] == "" {
		t.Fatal(public)
	}
	a, err = e.svc.SetArticleStatus(ctx, e.a.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}, false)
	e.must(err)
	if rc, _, err := e.svc.PublicMedia(ctx, resolved.SiteUUID, inline.UUID, ""); err == nil {
		_ = rc.Close()
		t.Fatal("unpublished article media public")
	}
	// A draft cover FK clears when an unprotected image is deleted.
	e.must(e.svc.DeleteMedia(ctx, e.a.id, cover.UUID))
	a, err = e.svc.GetArticle(ctx, e.a.id, a.UUID)
	e.must(err)
	if a.CoverMediaUUID != nil {
		t.Fatal(a)
	}
	e.must(e.svc.DeleteMedia(ctx, e.a.id, inline.UUID))
	_, err = e.svc.SetArticleStatus(ctx, e.a.id, a.UUID, 0, model.VersionInput{Version: ptr(a.Version)}, true)
	if !errors.As(err, &ve) {
		t.Fatal("deleted inline published", err)
	}
	var orgUUID uuid.UUID
	e.must(e.pool.QueryRow(ctx, `SELECT uuid FROM organizations WHERE id=$1`, e.a.id).Scan(&orgUUID))
	own := e.uploadMedia(e.a)
	h := handler.New(e.svc, nil, nil)
	serve := func(id uuid.UUID) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenant/site/media/"+id.String()+"/file", nil)
		req.SetPathValue("uuid", id.String())
		scope := orgctx.WithScope(ctx, orgctx.Scope{ID: e.a.id, UUID: orgUUID})
		req = req.WithContext(authctx.WithPrincipal(scope, authctx.Principal{}))
		rr := httptest.NewRecorder()
		h.TenantMedia(rr, req)
		return rr
	}
	rr := serve(own.UUID)
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "image/png" || rr.Header().Get("Cache-Control") != "private, no-store" || !bytes.Equal(rr.Body.Bytes(), mediaPNG()) {
		t.Fatal(rr.Code, rr.Header())
	}
	rr = serve(foreign.UUID)
	if rr.Code != 404 {
		t.Fatal("foreign file", rr.Code)
	}
	if own.URL != "/api/v1/tenant/site/media/"+own.UUID.String()+"/file" {
		t.Fatal(own.URL)
	}
	// A corrupted key cannot bypass the additional storage ownership check.
	_, err = e.pool.Exec(ctx, `UPDATE site_media SET object_key=$3 WHERE organization_id=$1 AND uuid=$2`, e.a.id, own.UUID, "organizations/"+uuid.NewString()+"/bad.png")
	e.must(err)
	if rr = serve(own.UUID); rr.Code != 404 {
		t.Fatal("foreign prefix", rr.Code)
	}
}

func TestArticleMediaSerialization(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	media := e.uploadMedia(e.a)
	in := articleDocument("race")
	in.CoverMediaUUID = &media.UUID
	a, err := e.svc.CreateArticle(ctx, e.a.id, 0, in)
	e.must(err)
	tx, err := e.pool.Begin(ctx)
	e.must(err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `UPDATE site_articles SET status='published',published_at=now() WHERE organization_id=$1 AND uuid=$2`, e.a.id, a.UUID)
	e.must(err)
	result := make(chan error, 1)
	go func() { result <- e.svc.DeleteMedia(ctx, e.a.id, media.UUID) }()
	e.must(tx.Commit(ctx))
	if err = <-result; !errors.Is(err, usecase.ErrMediaInUse) {
		t.Fatal("concurrent delete lost article reference", err)
	}
}

func TestArticleTenantHTTPAndActivity(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var actorID int64
	e.must(e.pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,name,surname) VALUES($1,'test','Article','Editor') RETURNING id`, uuid.NewString()+"@article.test").Scan(&actorID))
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, actorID) })
	var orgUUID uuid.UUID
	e.must(e.pool.QueryRow(ctx, `SELECT uuid FROM organizations WHERE id=$1`, e.a.id).Scan(&orgUUID))
	h := handler.New(e.svc, activity.NewRecorder(e.q, nil), nil)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tenant/site/articles", h.CreateArticle)
	mux.HandleFunc("GET /v1/tenant/site/articles", h.ListArticles)
	mux.HandleFunc("GET /v1/tenant/site/articles/{uuid}", h.GetArticle)
	mux.HandleFunc("PUT /v1/tenant/site/articles/{uuid}", h.UpdateArticle)
	mux.HandleFunc("DELETE /v1/tenant/site/articles/{uuid}", h.DeleteArticle)
	mux.HandleFunc("POST /v1/tenant/site/articles/{uuid}/publish", h.PublishArticle)
	mux.HandleFunc("POST /v1/tenant/site/articles/{uuid}/unpublish", h.UnpublishArticle)
	call := func(method, path string, in any, want int) json.RawMessage {
		var body bytes.Buffer
		if in != nil {
			e.must(json.NewEncoder(&body).Encode(in))
		}
		req := httptest.NewRequest(method, path, &body)
		scope := orgctx.WithScope(ctx, orgctx.Scope{ID: e.a.id, UUID: orgUUID})
		req = req.WithContext(authctx.WithPrincipal(scope, authctx.Principal{UserInternal: actorID}))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, rr.Code, want, rr.Body.String())
		}
		var response struct {
			Data json.RawMessage `json:"data"`
		}
		e.must(json.Unmarshal(rr.Body.Bytes(), &response))
		return response.Data
	}
	in := articleDocument("http-article")
	raw := call("POST", "/v1/tenant/site/articles", in, 201)
	var a model.Article
	e.must(json.Unmarshal(raw, &a))
	if a.CreatedBy == nil || *a.CreatedBy != actorID {
		t.Fatal(a)
	}
	path := "/v1/tenant/site/articles/" + a.UUID.String()
	call("GET", path, nil, 200)
	call("GET", "/v1/tenant/site/articles?status=draft&locale=tr&q=Rehber&sort=-updated_at", nil, 200)
	call("GET", "/v1/tenant/site/articles?status=other", nil, 400)
	call("GET", "/v1/tenant/site/articles?locale=xx", nil, 400)
	call("GET", "/v1/tenant/site/articles?sort=title", nil, 400)
	call("GET", "/v1/tenant/site/articles?page=1", nil, 400)
	in.Version = ptr(a.Version)
	raw = call("PUT", path, in, 200)
	e.must(json.Unmarshal(raw, &a))
	call("PUT", path, in, 409)
	raw = call("POST", path+"/publish", model.VersionInput{Version: ptr(a.Version)}, 200)
	e.must(json.Unmarshal(raw, &a))
	raw = call("POST", path+"/unpublish", model.VersionInput{Version: ptr(a.Version)}, 200)
	e.must(json.Unmarshal(raw, &a))
	call("DELETE", path+"?version="+fmt.Sprint(a.Version), nil, 200)
	call("GET", path, nil, 404)
	var count int
	e.must(e.pool.QueryRow(ctx, `SELECT count(*) FROM activity_events WHERE organization_id=$1 AND resource='tenant.site_articles' AND resource_uuid=$2 AND actor_user_id=$3`, e.a.id, a.UUID, actorID).Scan(&count))
	if count != 5 {
		t.Fatalf("mutation activity count = %d", count)
	}
}
