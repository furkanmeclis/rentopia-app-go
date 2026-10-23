package notifyengine_test

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/catalog"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/model"
	engine "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
)

// TEC-674: SMS outside the organization's plan is skipped and the
// dispatch falls through to the next channel (e-mail, never gated) even
// when the rule has no fallback; with the feature back, SMS goes out.
func TestSMSOutsidePlanFallsThroughToEmail(t *testing.T) {
	e := newEnv(t, true)
	e.eng.SetFeatures(features.LoaderPolicy{Loader: features.NewStore(e.q)})
	// The organization switches SMS off (override on its default plan).
	if err := e.q.UpsertOrganizationFeature(e.ctx, db.UpsertOrganizationFeatureParams{
		OrganizationID: e.org, FeatureKey: string(features.SMS), Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.eng.PutRule(e.ctx, e.org, catalog.ReservationConfirmed, model.RuleInput{
		Enabled: true, Channels: []string{"sms", "email"}, Fallback: false,
	}); err != nil {
		t.Fatal(err)
	}
	out := e.emit(t, catalog.ReservationConfirmed, "plan-off")
	d := e.dispatch(t, out.Dispatch.Uuid)
	if d.Status != engine.DispatchDelivered || attempts(d) != "sms:skipped,email:sent" {
		t.Fatalf("sms off: %s %s (%s)", d.Status, attempts(d), d.LastError)
	}
	if !strings.Contains(d.Attempts[0].Error, "plan") {
		t.Fatalf("skip reason: %q", d.Attempts[0].Error)
	}
	if n := len(e.fake.Messages()); n != 0 {
		t.Fatalf("%d SMS sent outside the plan", n)
	}
	if e.mail.count() != 1 {
		t.Fatalf("e-mail count %d", e.mail.count())
	}

	// Override cleared: the default plan (Komple) has SMS again.
	if _, err := e.q.DeleteOrganizationFeature(e.ctx, db.DeleteOrganizationFeatureParams{
		OrganizationID: e.org, FeatureKey: string(features.SMS),
	}); err != nil {
		t.Fatal(err)
	}
	again := e.emit(t, catalog.ReservationConfirmed, "plan-on")
	if d := e.dispatch(t, again.Dispatch.Uuid); attempts(d) != "sms:sent" {
		t.Fatalf("sms on: %s %s", d.Status, attempts(d))
	}
	if n := len(e.fake.Messages()); n != 1 {
		t.Fatalf("sms messages %d", n)
	}
}
