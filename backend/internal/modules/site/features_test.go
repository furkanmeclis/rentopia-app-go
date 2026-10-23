package site_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
)

// TEC-674: a published site of an organization whose plan lacks the site
// feature answers exactly like a disabled site (404 SITE_NOT_FOUND, also
// with a preview token and on articles / sitemap), so an anonymous host
// never reveals the organization or its plan; without the widget feature
// the site resolves with no embedding origins.
func TestPublicSiteOutsidePlanIsNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := "plan-" + e.a.slug[7:]
	host := sub + ".sites.test"
	s, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(sub), Config: themed("#123456", "Plan")})
	e.must(err)
	e.publish(e.a, s.Version)
	e.svc.SetFeatures(features.LoaderPolicy{Loader: features.NewStore(e.q)})
	if r, ans := e.resolve(host); ans.code != http.StatusOK || len(r.Config.Widget.AllowedDomains) == 0 {
		t.Fatalf("default plan = %d %+v", ans.code, r.Config.Widget)
	}

	// Widget off: still served, nobody may embed it.
	e.must(e.q.UpsertOrganizationFeature(ctx, db.UpsertOrganizationFeatureParams{
		OrganizationID: e.a.id, FeatureKey: string(features.Widget), Enabled: false,
	}))
	if r, ans := e.resolve(host); ans.code != http.StatusOK || len(r.Config.Widget.AllowedDomains) != 0 {
		t.Fatalf("widget off = %d %+v", ans.code, r.Config.Widget)
	}

	// Site off: the same answer as a disabled site everywhere.
	e.must(e.q.UpsertOrganizationFeature(ctx, db.UpsertOrganizationFeatureParams{
		OrganizationID: e.a.id, FeatureKey: string(features.Site), Enabled: false,
	}))
	preview, err := e.svc.IssuePreview(ctx, e.a.id, e.a.slug, 0)
	e.must(err)
	for name, path := range map[string]string{
		"resolve":  "/v1/public/site/resolve?host=" + host,
		"preview":  "/v1/public/site/resolve?host=" + host + "&preview=" + url.QueryEscape(preview.Token),
		"articles": "/v1/public/site/articles?host=" + host,
		"sitemap":  "/v1/public/site/sitemap?host=" + host,
	} {
		r := e.call(http.MethodGet, path, nil, nil)
		if r.code != http.StatusNotFound || r.err.Code != handler.CodeSiteNotFound {
			t.Errorf("%s outside the plan = %d %s, want 404 %s", name, r.code, r.err.Code, handler.CodeSiteNotFound)
		}
	}
	// The other organization is not affected.
	subB := "planb-" + e.b.slug[7:]
	b, err := e.svc.Save(ctx, e.b.id, e.b.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true), Subdomain: ptr(subB), Config: themed("#654321", "B")})
	e.must(err)
	e.publish(e.b, b.Version)
	if _, ans := e.resolve(subB + ".sites.test"); ans.code != http.StatusOK {
		t.Fatalf("other organization = %d", ans.code)
	}
}
