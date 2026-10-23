package publicapi_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// RT-A28 / TEC-674: on the Yönetim plan the public API and the site's
// booking sessions answer 403 FEATURE_NOT_ENABLED and no new API key can
// be created; existing keys stay and work again once the platform enables
// the feature. Another organization is not affected.
func TestPlanFeatureGate(t *testing.T) {
	e := newEnv(t)
	secret := e.key(e.a, model.KindSecret, "api")
	site := e.key(e.a, model.KindPublishable, "web", "https://www.rentacar.test")
	other := e.key(e.b, model.KindSecret, "api")
	loc := "/v1/public/locations"
	e.expect(e.do(call{method: "GET", path: loc, bearer: secret.Secret}), http.StatusOK, "default plan")

	e.svc.SetFeatures(features.LoaderPolicy{Loader: features.NewStore(e.q)})
	mgmt, err := e.q.GetPlanByCode(e.a.ctx, features.PlanManagement)
	e.must(err)
	e.must(e.q.UpsertOrganizationPlan(e.a.ctx, db.UpsertOrganizationPlanParams{OrganizationID: e.a.id, PlanID: mgmt.ID}))

	r := e.do(call{method: "GET", path: loc, bearer: secret.Secret})
	if r.code != http.StatusForbidden || r.errCode() != response.CodeFeatureNotEnabled {
		t.Fatalf("api key on Yönetim: %d %s", r.code, r.raw)
	}
	r = e.do(call{method: "POST", path: "/v1/public/sessions", origin: "https://www.rentacar.test",
		body: map[string]any{"publishable_key": site.Secret}})
	if r.code != http.StatusForbidden || r.errCode() != response.CodeFeatureNotEnabled {
		t.Fatalf("site session on Yönetim: %d %s", r.code, r.raw)
	}
	kind, name, channel := model.KindSecret, "partner", "api"
	if _, err := e.svc.CreateKey(e.a.ctx, e.a.id, model.KeyInput{Kind: &kind, Name: &name, Channel: &channel}); !errors.Is(err, features.ErrNotEnabled) {
		t.Fatalf("key created on Yönetim: %v", err)
	}
	e.expect(e.do(call{method: "GET", path: loc, bearer: other.Secret}), http.StatusOK, "other organization")

	// The platform enables the site only: the site books again, the API
	// key stays refused.
	e.must(e.q.UpsertOrganizationFeature(e.a.ctx, db.UpsertOrganizationFeatureParams{
		OrganizationID: e.a.id, FeatureKey: string(features.Site), Enabled: true,
	}))
	tok := e.session(site.Secret, "https://www.rentacar.test")
	e.expect(e.do(call{method: "GET", path: loc, bearer: tok, origin: "https://www.rentacar.test"}), http.StatusOK, "site session")
	if r := e.do(call{method: "GET", path: loc, bearer: secret.Secret}); r.code != http.StatusForbidden {
		t.Fatalf("api key with only the site on: %d", r.code)
	}
}
