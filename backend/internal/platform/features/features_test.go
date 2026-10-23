package features

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

func ptr(v bool) *bool { return &v }

// The override beats the plan default in both directions; no override
// means the plan default.
func TestEffectiveOverrideBeatsPlan(t *testing.T) {
	t.Parallel()
	states := []State{
		{Key: Site, InPlan: true},                            // plan on
		{Key: Widget, InPlan: false},                         // plan off
		{Key: PublicAPI, InPlan: false, Override: ptr(true)}, // override on
		{Key: SMS, InPlan: true, Override: ptr(false)},       // override off
		{Key: Kabis, InPlan: true, Override: ptr(true)},      // override equal to plan
		{Key: "unknown", InPlan: true},                       // unknown keys never count
	}
	got := Strings(Effective(states))
	want := []string{"site", "public_api", "kabis"}
	if len(got) != len(want) {
		t.Fatalf("effective = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("effective = %v, want %v", got, want)
		}
	}
	if !(State{InPlan: false, Override: ptr(true)}).Enabled() || (State{InPlan: true, Override: ptr(false)}).Enabled() {
		t.Fatal("State.Enabled ignores the override")
	}
}

func TestCatalogKeysAreUniqueAndParse(t *testing.T) {
	t.Parallel()
	seen := map[Feature]bool{}
	for _, info := range Catalog() {
		if seen[info.Key] {
			t.Fatalf("duplicate feature %q", info.Key)
		}
		seen[info.Key] = true
		if info.NameTR == "" || info.NameEN == "" {
			t.Fatalf("feature %q needs tr and en labels", info.Key)
		}
		if f, ok := Parse(string(info.Key)); !ok || f != info.Key {
			t.Fatalf("Parse(%q) = %q, %v", info.Key, f, ok)
		}
	}
	if Valid("panel") || Valid("") {
		t.Fatal("unknown keys must not parse")
	}
}

func TestMappings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind, provider string
		want           Feature
		ok             bool
	}{
		{"sms", "netgsm", SMS, true},
		{"kabis", "kabis_fake", Kabis, true},
		{"toll", "hgs_fake", HGS, true},
		{"einvoice", "einvoice", EInvoice, true},
		{"whatsapp", "wuzapi", WhatsApp, true},
		{"channel", "yolcu360", ChannelYolcu360, true},
		{"channel", "yolcu360_fake", ChannelYolcu360, true},
		{"channel", "obilet_fake", ChannelObilet, true},
		{"payment", "iyzico", "", false},
		{"channel", "unknown", "", false},
	}
	for _, c := range cases {
		got, ok := ForIntegration(c.kind, c.provider)
		if got != c.want || ok != c.ok {
			t.Errorf("ForIntegration(%q, %q) = %q, %v; want %q, %v", c.kind, c.provider, got, ok, c.want, c.ok)
		}
	}
	for code, want := range map[string]Feature{"web": Site, "widget": Widget, "api": PublicAPI, "yolcu360": ChannelYolcu360, "obilet": ChannelObilet} {
		if got, ok := ForSalesChannel(code); !ok || got != want {
			t.Errorf("ForSalesChannel(%q) = %q, %v", code, got, ok)
		}
	}
	if _, ok := ForSalesChannel("branch"); ok {
		t.Error("the branch counter is never gated")
	}
	for provider, want := range map[string]Feature{"kabis_fake": Kabis, "hgs_fake": HGS, "einvoice": EInvoice, "wuzapi": WhatsApp, "obilet": ChannelObilet} {
		if got, ok := ForProvider(provider); !ok || got != want {
			t.Errorf("ForProvider(%q) = %q, %v", provider, got, ok)
		}
	}
	for _, provider := range []string{"netgsm", "log", "iyzico"} {
		if _, ok := ForProvider(provider); ok {
			t.Errorf("job retries of %s must never be gated", provider)
		}
	}
	if f, ok := ForNotifyChannel("sms"); !ok || f != SMS {
		t.Error("sms channel must map to the sms feature")
	}
	for _, ch := range []string{"email", "push", "inapp"} {
		if _, ok := ForNotifyChannel(ch); ok {
			t.Errorf("%s must never be gated", ch)
		}
	}
}

type countingPolicy struct {
	calls int
	on    Static
}

func (p *countingPolicy) Enabled(ctx context.Context, orgID int64, f Feature) (bool, error) {
	p.calls++
	return p.on.Enabled(ctx, orgID, f)
}

func TestRequire(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := Require(ctx, nil, 1, PublicAPI); err != nil {
		t.Fatalf("nil policy must allow: %v", err)
	}
	p := &countingPolicy{on: Static{Site}}
	if err := Require(ctx, p, 1, Site); err != nil {
		t.Fatal(err)
	}
	err := Require(ctx, p, 1, PublicAPI)
	if !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("err = %v, want ErrNotEnabled", err)
	}
	if f, ok := FeatureOf(err); !ok || f != PublicAPI {
		t.Fatalf("FeatureOf = %q, %v", f, ok)
	}
	// A request scope of the same organization answers without the policy.
	scoped := orgctx.WithScope(ctx, orgctx.Scope{ID: 7, Features: []string{"widget"}})
	p.calls = 0
	if err := Require(scoped, p, 7, Widget); err != nil {
		t.Fatal(err)
	}
	if err := Require(scoped, p, 7, Site); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("scope without site: %v", err)
	}
	if p.calls != 0 {
		t.Fatalf("policy asked %d times despite the scope", p.calls)
	}
	// Another organization's id, or a scope without features, asks the policy.
	if err := Require(scoped, p, 8, Site); err != nil || p.calls != 1 {
		t.Fatalf("other organization: %v (calls %d)", err, p.calls)
	}
	bare := orgctx.WithOrganizationID(ctx, 9)
	if err := Require(bare, p, 9, PublicAPI); !errors.Is(err, ErrNotEnabled) || p.calls != 2 {
		t.Fatalf("job scope: %v (calls %d)", err, p.calls)
	}
}

type loaderFunc func(ctx context.Context, orgID int64) ([]Feature, error)

func (f loaderFunc) EnabledFeatures(ctx context.Context, orgID int64) ([]Feature, error) {
	return f(ctx, orgID)
}

func TestLoaderPolicy(t *testing.T) {
	t.Parallel()
	p := LoaderPolicy{Loader: loaderFunc(func(_ context.Context, orgID int64) ([]Feature, error) {
		if orgID == 0 {
			return nil, errors.New("boom")
		}
		return []Feature{SMS}, nil
	})}
	if ok, err := p.Enabled(context.Background(), 1, SMS); !ok || err != nil {
		t.Fatalf("sms: %v %v", ok, err)
	}
	if ok, _ := p.Enabled(context.Background(), 1, WhatsApp); ok {
		t.Fatal("whatsapp must be off")
	}
	if _, err := p.Enabled(context.Background(), 0, SMS); err == nil {
		t.Fatal("loader errors must surface")
	}
}
