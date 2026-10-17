package site_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	cm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	cu "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	nu "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	na "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/adapters"
	ne "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	ph "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/handler"
	pm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
	lm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/model"
	lu "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

func TestCampaignStorefront(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true)})
	e.must(err)
	e.publish(e.a, saved.Version)
	svc := cu.New(e.pool, e.q)
	media := e.uploadMedia(e.a)
	foreign := e.uploadMedia(e.b)
	create := func(org org, code string) cm.Campaign {
		out, err := svc.Create(ctx, org.id, cm.CampaignInput{Name: ptr(code), CampaignType: ptr("coupon"), Code: cm.Optional[string]{Set: true, Value: &code}, DiscountKind: ptr("percent"), DiscountValue: ptr(cm.Decimal("10"))})
		e.must(err)
		return out
	}
	c := create(e.a, "PUBLIC")
	_, err = svc.PatchStorefront(ctx, e.a.id, c.UUID, cm.StorefrontInput{ShowOnSite: ptr(true), SiteTitle: &map[string]string{"tr": "Kampanya", "en": "Offer"}, SiteMediaUUID: cm.Optional[uuid.UUID]{Set: true, Value: &media.UUID}})
	e.must(err)
	_, err = svc.PatchStorefront(ctx, e.a.id, c.UUID, cm.StorefrontInput{SiteMediaUUID: cm.Optional[uuid.UUID]{Set: true, Value: &foreign.UUID}})
	var ve *cu.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("foreign media accepted: %v", err)
	}
	_, err = svc.GetStorefront(ctx, e.b.id, c.UUID)
	if !errors.Is(err, cu.ErrNotFound) {
		t.Fatal(err)
	}
	_, err = svc.PatchStorefront(ctx, e.b.id, c.UUID, cm.StorefrontInput{ShowOnSite: ptr(false)})
	if !errors.Is(err, cu.ErrNotFound) {
		t.Fatal(err)
	}
	_, err = svc.PatchStorefront(ctx, e.a.id, c.UUID, cm.StorefrontInput{SiteTitle: &map[string]string{"tr": strings.Repeat("ş", 161)}})
	if !errors.As(err, &ve) {
		t.Fatal("unbounded title", err)
	}
	_, err = e.pool.Exec(ctx, "UPDATE campaigns SET site_media_uuid=$1 WHERE organization_id=$2 AND uuid=$3", foreign.UUID, e.a.id, c.UUID)
	if err == nil {
		t.Fatal("cross-org FK accepted")
	}
	if err = e.svc.DeleteMedia(ctx, e.a.id, media.UUID); !errors.Is(err, usecase.ErrMediaInUse) {
		t.Fatal("campaign reference not protected", err)
	}
	for _, state := range []string{"hidden", "inactive", "expired", "future", "exhausted", "foreign", "private", "repeat", "wrongchannel"} {
		org := e.a
		if state == "foreign" {
			org = e.b
		}
		other := create(org, strings.ToUpper(state))
		_, err = svc.PatchStorefront(ctx, org.id, other.UUID, cm.StorefrontInput{ShowOnSite: ptr(state != "hidden"), SiteSort: ptr(int32(10))})
		e.must(err)
		switch state {
		case "inactive":
			_, err = e.pool.Exec(ctx, "UPDATE campaigns SET is_active=false WHERE organization_id=$1 AND uuid=$2", org.id, other.UUID)
		case "expired":
			_, err = e.pool.Exec(ctx, "UPDATE campaigns SET end_date='2000-01-01' WHERE organization_id=$1 AND uuid=$2", org.id, other.UUID)
		case "future":
			_, err = e.pool.Exec(ctx, "UPDATE campaigns SET start_date='2099-01-01' WHERE organization_id=$1 AND uuid=$2", org.id, other.UUID)
		case "exhausted":
			_, err = e.pool.Exec(ctx, "UPDATE campaigns SET usage_limit=1,used_count=1 WHERE organization_id=$1 AND uuid=$2", org.id, other.UUID)
		case "private":
			_, err = e.pool.Exec(ctx, "UPDATE campaigns SET customer_uuids=$3 WHERE organization_id=$1 AND uuid=$2", org.id, other.UUID, []uuid.UUID{uuid.New()})
		case "repeat":
			_, err = e.pool.Exec(ctx, "UPDATE campaigns SET campaign_type='repeat_customer',code=NULL,min_completed_rentals=1 WHERE organization_id=$1 AND uuid=$2", org.id, other.UUID)
		case "wrongchannel":
			_, err = e.pool.Exec(ctx, "UPDATE campaigns SET channels=ARRAY['api'] WHERE organization_id=$1 AND uuid=$2", org.id, other.UUID)
		}
		e.must(err)
	}
	cards, err := cu.PublicCampaigns(ctx, e.q, e.a.id, "web", nil)
	e.must(err)
	if len(cards) != 3 || cards[0].UUID != c.UUID || cards[0].Code != "PUBLIC" || cards[0].MediaURL == nil {
		t.Fatalf("cards: %+v", cards)
	}
	for _, card := range cards[1:] {
		if card.Code != "" {
			t.Fatalf("private code leaked: %+v", card)
		}
	}
	resolved, err := e.svc.Resolve(ctx, strings.TrimPrefix(saved.URL, "https://"), "")
	e.must(err)
	if len(resolved.Campaigns) != 3 {
		t.Fatal(resolved.Campaigns)
	}
	rc, _, err := e.svc.PublicMedia(ctx, resolved.SiteUUID, media.UUID, "")
	e.must(err)
	_ = rc.Close()
	_, err = svc.PatchStorefront(ctx, e.a.id, c.UUID, cm.StorefrontInput{ShowOnSite: ptr(false)})
	e.must(err)
	if rc, _, err = e.svc.PublicMedia(ctx, resolved.SiteUUID, media.UUID, ""); err == nil {
		_ = rc.Close()
		t.Fatal("hidden campaign media public")
	}
	// A soft-deleted campaign no longer protects its unused media; FK clears UUID only.
	e.must(svc.Delete(ctx, e.a.id, c.UUID))
	e.must(e.svc.DeleteMedia(ctx, e.a.id, media.UUID))
	var orgID int64
	var mediaID pgtype.UUID
	e.must(e.pool.QueryRow(ctx, "SELECT organization_id,site_media_uuid FROM campaigns WHERE organization_id=$1 AND uuid=$2", e.a.id, c.UUID).Scan(&orgID, &mediaID))
	if orgID != e.a.id || mediaID.Valid {
		t.Fatal("SET NULL affected organization or failed to clear reference")
	}
}

func TestSiteLeadsPublic(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Enabled: ptr(true)})
	e.must(err)
	e.publish(e.a, saved.Version)
	origin := saved.URL
	leadsvc := lu.New(e.q, "test-private-salt")
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	public := ph.NewPublic(e.keys, ratelimit.New(rdb, "leads-test"))
	public.SetStorefront(e.q, leadsvc)
	mux := http.NewServeMux()
	public.Mount(mux)
	token := func() string {
		session, _, err := e.keys.MintSession(ctx, saved.APIKey.PublishableKey, origin, "")
		e.must(err)
		return session.Token
	}
	bearer := token()
	request := func(body any, auth, ip string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/public/site/leads", bytes.NewReader(raw))
		req.RemoteAddr = ip + ":1234"
		req.Header.Set("Authorization", "Bearer "+auth)
		req.Header.Set(ph.HeaderForwardedOrigin, origin)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	expect := func(body any, auth, ip string, want int) {
		t.Helper()
		r := request(body, auth, ip)
		if r.Code != want {
			t.Fatalf("status %d want %d: %s", r.Code, want, r.Body.String())
		}
	}
	base := map[string]any{"kind": "newsletter", "email": " User@Example.com ", "consent_kvkk": true}
	expect(base, bearer, "192.0.2.10", 202)
	expect(base, bearer, "192.0.2.10", 202)
	expect(map[string]any{"honeypot": "bot"}, bearer, "192.0.2.10", 202)
	var count int
	var hash, agent string
	e.must(e.pool.QueryRow(ctx, "SELECT count(*) FROM site_leads WHERE organization_id=$1", e.a.id).Scan(&count))
	if count != 1 {
		t.Fatalf("duplicate/honeypot stored: %d", count)
	}
	e.must(e.pool.QueryRow(ctx, "SELECT ip_hash,user_agent FROM site_leads WHERE organization_id=$1", e.a.id).Scan(&hash, &agent))
	if len(hash) != 64 || hash == "192.0.2.10" || hash != leadsvc.IPHash(e.a.id, "192.0.2.10") || hash == leadsvc.IPHash(e.b.id, "192.0.2.10") {
		t.Fatal("IP not privately hashed")
	}
	expect(map[string]any{"kind": "contact", "email": "x@example.com", "consent_kvkk": true}, bearer, "192.0.2.10", 400)
	expect(map[string]any{"kind": "newsletter", "email": "x@example.com"}, bearer, "192.0.2.10", 400)
	r := request(base, bearer, "192.0.2.11")
	if r.Code != 429 || r.Header().Get("Retry-After") == "" {
		t.Fatal("session limit", r.Code, r.Body.String())
	}
	mini.FastForward(time.Hour)
	// Ten separate sessions still share one IP limit.
	for i := 0; i < 10; i++ {
		expect(base, token(), "192.0.2.20", 202)
	}
	expect(base, token(), "192.0.2.20", 429)
	// Every kind validates its required fields and persists only valid input.
	for _, kind := range []string{"contact", "corporate", "franchise"} {
		expect(map[string]any{"kind": kind, "email": "bad-kind@example.com", "consent_kvkk": true}, token(), "192.0.2.31", 400)
	}
	for _, in := range []lm.Input{
		{Kind: "contact", Name: "Ada", Email: "ada@example.com", Message: "Question", ConsentKVKK: true},
		{Kind: "corporate", Name: "Ada", Company: "Fleet", Email: "ada@example.com", Phone: "555", FleetSize: ptr(int32(12)), ConsentKVKK: true},
		{Kind: "franchise", Name: "Ada", Email: "ada@example.com", Phone: "555", City: "İzmir", ConsentKVKK: true},
	} {
		expect(in, token(), "192.0.2.30", 202)
	}
	rows, _, err := leadsvc.List(ctx, db.ListSiteLeadsParams{OrganizationID: e.a.id, PageLimit: 100})
	e.must(err)
	if len(rows) != 4 {
		t.Fatal(rows)
	}
	ascending, total, err := leadsvc.List(ctx, db.ListSiteLeadsParams{OrganizationID: e.a.id, PageLimit: 100, SortDesc: false})
	e.must(err)
	if total != 4 {
		t.Fatal(total)
	}
	for i := 1; i < len(ascending); i++ {
		if ascending[i].CreatedAt.Before(ascending[i-1].CreatedAt) {
			t.Fatal("ascending order violated")
		}
	}
	filtered, total, err := leadsvc.List(ctx, db.ListSiteLeadsParams{OrganizationID: e.a.id, PageLimit: 1, SortDesc: true,
		Kind: pgtype.Text{String: "corporate", Valid: true}, Status: pgtype.Text{String: "new", Valid: true}, Q: "Fleet",
		DateFrom: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}, DateTo: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}})
	e.must(err)
	if total != 1 || len(filtered) != 1 || filtered[0].Kind != "corporate" {
		t.Fatal("list filters", total, filtered)
	}

	for _, row := range rows {
		if _, err = leadsvc.Get(ctx, e.b.id, row.UUID); !errors.Is(err, lu.ErrNotFound) {
			t.Fatal("foreign get", err)
		}
		if _, err = leadsvc.Patch(ctx, e.b.id, row.UUID, 0, lm.Patch{Status: ptr("spam")}); !errors.Is(err, lu.ErrNotFound) {
			t.Fatal("foreign patch", err)
		}
	}
	changed, err := leadsvc.Patch(ctx, e.a.id, rows[0].UUID, 0, lm.Patch{Status: ptr("in_progress"), Note: ptr("Called")})
	e.must(err)
	if changed.Status != "in_progress" || changed.Note != "Called" || changed.HandledAt == nil {
		t.Fatal(changed)
	}
	// Disabled published form is rejected; draft edits alone do not change live behavior.
	cfg := saved.Config
	cfg.Leads.Newsletter = false
	saved, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: &saved.Version, Config: &cfg})
	e.must(err)
	expect(base, token(), "192.0.2.40", 202)
	e.publish(e.a, saved.Version)
	expect(base, token(), "192.0.2.40", 404)
	secret, err := e.keys.CreateKey(ctx, e.a.id, pm.KeyInput{Name: ptr("private"), Kind: ptr(pm.KindSecret), Channel: ptr("web")})
	e.must(err)
	// Secret keys never enter this API, with or without a browser origin.
	req := httptest.NewRequest(http.MethodPost, "/v1/public/site/leads", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+secret.Secret)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	// Campaign endpoints enforce the same storefront boundary and foreign/hidden lookup is 404.
	req = httptest.NewRequest(http.MethodGet, "/v1/public/campaigns/"+uuid.NewString(), nil)
	req.Header.Set("Authorization", "Bearer "+token())
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

func TestSiteLeadNotifications(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	createMember := func(o org, role string) int64 {
		user, err := e.q.CreateUser(ctx, db.CreateUserParams{Email: uuid.NewString() + "@lead.test", PasswordHash: "unused", Name: "Reader", Surname: "Test", Status: "active"})
		e.must(err)
		t.Cleanup(func() {
			_, _ = e.pool.Exec(ctx, "DELETE FROM notifications WHERE user_id=$1", user.ID)
			_, _ = e.pool.Exec(ctx, "DELETE FROM users WHERE id=$1", user.ID)
		})
		var roleID int64
		e.must(e.pool.QueryRow(ctx, "SELECT id FROM roles WHERE scope='tenant' AND organization_id IS NULL AND slug=$1", role).Scan(&roleID))
		member, err := e.q.AddOrganizationMember(ctx, db.AddOrganizationMemberParams{OrganizationID: o.id, UserID: user.ID, RoleID: roleID})
		e.must(err)
		e.must(e.q.AddMemberRole(ctx, db.AddMemberRoleParams{OrganizationID: o.id, MemberID: member.ID, RoleID: roleID}))
		return user.ID
	}
	owner := createMember(e.a, "owner")
	_ = createMember(e.a, "member")
	_ = createMember(e.b, "owner")
	notifier := nu.New(e.q, nil, []providers.Provider{providers.InappProvider{}}, nil)
	engine := ne.New(e.pool, e.q, "")
	engine.Register(na.NewPush(notifier, e.q, na.VAPID{}))
	svc := lu.New(e.q, "notification-test-salt")
	svc.SetNotifier(engine)
	in := lm.Input{Kind: "newsletter", Email: "notice@lead.test", ConsentKVKK: true}
	e.must(svc.Submit(ctx, e.a.id, in, "https://site.test", "192.0.2.1", ""))
	e.must(svc.Submit(ctx, e.a.id, in, "https://site.test", "192.0.2.1", ""))
	in.Honeypot = "bot"
	e.must(svc.Submit(ctx, e.a.id, in, "https://site.test", "192.0.2.1", ""))
	rows, err := e.pool.Query(ctx, "SELECT user_id,body FROM notifications WHERE organization_id=$1 AND source_event='notify.site_lead.created'", e.a.id)
	e.must(err)
	defer rows.Close()
	count := 0
	for rows.Next() {
		var user int64
		var body string
		e.must(rows.Scan(&user, &body))
		count++
		if user != owner || strings.Contains(body, "notice@lead.test") {
			t.Fatal("recipient or PII leak", user, body)
		}
	}
	e.must(rows.Err())
	if count != 1 {
		t.Fatalf("notifications %d, want exactly one owner notification", count)
	}
}
