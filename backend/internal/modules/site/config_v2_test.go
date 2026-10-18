package site_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/themes"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/usecase"
)

func TestConfigV2Validation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*model.Config)
	}{
		{"unknown template", func(c *model.Config) { c.Template = "classic-design" }},
		{"studio template id", func(c *model.Config) { c.Template = "aurum-design" }},
		{"invalid aurum columns", func(c *model.Config) {
			d, _ := themes.Find("aurum")
			tc := themes.Defaults(d)
			tc.Settings["fleetColumns"] = "1"
			c.Template = d.ID
			c.Templates[d.ID] = tc
		}},
		{"aurum rejects branches", func(c *model.Config) {
			d, _ := themes.Find("aurum")
			tc := themes.Defaults(d)
			tc.Sections = append(tc.Sections, themes.Section{ID: "branches", Enabled: true})
			c.Templates[d.ID] = tc
		}},
		{"invalid classic columns", func(c *model.Config) {
			d, _ := themes.Find("classic")
			tc := themes.Defaults(d)
			tc.Settings["fleetColumns"] = "4"
			c.Template = d.ID
			c.Templates[d.ID] = tc
		}},
		{"classic columns are strings of the enum", func(c *model.Config) {
			d, _ := themes.Find("classic")
			tc := themes.Defaults(d)
			tc.Settings["fleetColumns"] = " 2"
			c.Templates[d.ID] = tc
		}},
		{"classic rejects horizon setting", func(c *model.Config) {
			d, _ := themes.Find("classic")
			tc := themes.Defaults(d)
			tc.Settings["heroAlign"] = "center"
			c.Templates[d.ID] = tc
		}},
		{"classic rejects newsletter", func(c *model.Config) {
			d, _ := themes.Find("classic")
			tc := themes.Defaults(d)
			tc.Sections = append(tc.Sections, themes.Section{ID: "newsletter", Enabled: true})
			c.Template = d.ID
			c.Templates[d.ID] = tc
		}},
		{"unknown section", func(c *model.Config) {
			tc := c.Templates[c.Template]
			tc.Sections = append(tc.Sections, themes.Section{ID: "unknown"})
			c.Templates[c.Template] = tc
		}},
		{"unsupported section", func(c *model.Config) {
			d, _ := themes.Find("horizon-design")
			tc := themes.Defaults(d)
			tc.Sections = append(tc.Sections, themes.Section{ID: "advantages"})
			c.Templates[d.ID] = tc
		}},
		{"app unavailable", func(c *model.Config) {
			tc := c.Templates[c.Template]
			for i := range tc.Sections {
				if tc.Sections[i].ID == "app_promo" {
					tc.Sections[i].Enabled = true
				}
			}
			c.Templates[c.Template] = tc
		}},
		{"duplicate", func(c *model.Config) {
			tc := c.Templates[c.Template]
			tc.Sections = append(tc.Sections, tc.Sections[0])
			c.Templates[c.Template] = tc
		}},
		{"hero first", func(c *model.Config) {
			tc := c.Templates[c.Template]
			tc.Sections[0], tc.Sections[1] = tc.Sections[1], tc.Sections[0]
			c.Templates[c.Template] = tc
		}},
		{"hero enabled", func(c *model.Config) {
			tc := c.Templates[c.Template]
			tc.Sections[0].Enabled = false
			c.Templates[c.Template] = tc
		}},
		{"unknown setting", func(c *model.Config) {
			tc := c.Templates[c.Template]
			tc.Settings["oops"] = "value"
			c.Templates[c.Template] = tc
		}},
		{"invalid horizon alignment", func(c *model.Config) {
			d, _ := themes.Find("horizon-design")
			tc := themes.Defaults(d)
			tc.Settings["heroAlign"] = "right"
			c.Template = d.ID
			c.Templates[d.ID] = tc
		}},
		{"horizon rejects technowide setting", func(c *model.Config) {
			d, _ := themes.Find("horizon-design")
			tc := themes.Defaults(d)
			tc.Settings["heroLayout"] = "split"
			c.Templates[d.ID] = tc
		}},
		{"invalid setting", func(c *model.Config) {
			tc := c.Templates[c.Template]
			tc.Settings["heroLayout"] = "javascript:"
			c.Templates[c.Template] = tc
		}},
		{"menu javascript", func(c *model.Config) { c.Menus.Header = []model.MenuItem{{Href: "javascript:alert(1)"}} }},
		{"menu protocol relative", func(c *model.Config) { c.Menus.Header = []model.MenuItem{{Href: "//evil.test"}} }},
		{"menu encoded slash", func(c *model.Config) { c.Menus.Header = []model.MenuItem{{Href: "/%2fevil.test"}} }},
		{"menu backslash", func(c *model.Config) { c.Menus.Header = []model.MenuItem{{Href: "/\\evil.test"}} }},
		{"social foreign host", func(c *model.Config) {
			c.Brand.Socials = map[string]string{"instagram": "https://instagram.com.evil.test"}
		}},
		{"social script", func(c *model.Config) { c.Brand.Socials = map[string]string{"x": "javascript:alert(1)"} }},
		{"cta script", func(c *model.Config) {
			ct := c.Content["tr"]
			ct.HeroSlides[0].CTAHref = "javascript:alert(1)"
			c.Content["tr"] = ct
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c, err := usecase.NormalizeConfig(model.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			tt.change(&c)
			_, err = usecase.NormalizeConfig(c)
			if err == nil {
				t.Fatal("accepted invalid config")
			}
			if tt.name == "app unavailable" {
				var ve *usecase.ValidationError
				if !errors.As(err, &ve) || ve.Details[0].Code != "SECTION_UNAVAILABLE" {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestLegacyConfigV2Normalization(t *testing.T) {
	c := model.DefaultConfig()
	c.Theme.Radius = "full"
	ct := c.Content["tr"]
	ct.HeroTitle = "Legacy title"
	ct.About = "Preserved story"
	c.Content["tr"] = ct
	n, err := usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	if n.Theme.Radius != "pill" || n.Theme.HeadingFont != "theme" || n.Template != themes.Default || n.Content["tr"].About != "Preserved story" || n.Content["tr"].HeroSlides[0].Title != "Legacy title" {
		t.Fatalf("bad normalization: %+v", n)
	}
	tc := n.Templates[n.Template]
	tc.Sections = tc.Sections[:1]
	n.Templates[n.Template] = tc
	n, err = usecase.NormalizeConfig(n)
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range n.Templates[n.Template].Sections[1:] {
		if section.Enabled {
			t.Fatal("missing section appended enabled")
		}
	}
	again, err := usecase.NormalizeConfig(n)
	if err != nil || !reflect.DeepEqual(n, again) {
		t.Fatal("normalization not idempotent", err)
	}
	for _, radius := range []string{"none", "full", "soft", "sharp", "xs", "sm", "md", "lg", "xl", "pill"} {
		c.Theme.Radius = radius
		if _, err := usecase.NormalizeConfig(c); err != nil {
			t.Fatal(radius, err)
		}
	}
}

func TestConfigV2LinksAndTemplateMemory(t *testing.T) {
	c := model.DefaultConfig()
	c.Menus.Header = []model.MenuItem{{Href: "/fleet?type=car", Label: map[string]string{"tr": "Araçlar", "en": "Fleet"}, Kind: "page"}, {Href: "https://example.com/path", Kind: "link"}}
	c.Brand.Socials = map[string]string{"instagram": "https://www.instagram.com/rentopia", "whatsapp": "https://wa.me/905001234567"}
	d, _ := themes.Find("classic")
	tc := themes.Defaults(d)
	tc.Settings["fleetColumns"] = "2"
	c.Templates = map[string]themes.Config{"classic": tc}
	n, err := usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tc, n.Templates["classic"]) {
		t.Fatal("inactive template lost")
	}
	data, _ := json.Marshal(n)
	var round model.Config
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	round, err = usecase.NormalizeConfig(round)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(n, round) {
		t.Fatal("template memory lost after roundtrip")
	}
	if strings.Contains(string(data), "object_key") {
		t.Fatal("storage key in document")
	}
}

func TestConfigV2SavePreservesInactiveTemplate(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	c, err := usecase.NormalizeConfig(model.DefaultConfig())
	e.must(err)
	d, _ := themes.Find("classic")
	tc := themes.Defaults(d)
	tc.Settings["fleetColumns"] = "2"
	tc.Sections[1].Enabled = false
	c.Templates[d.ID] = tc
	saved, err := e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: ptr(0), Config: &c})
	e.must(err)
	// Current template-only editor payload keeps the inactive template intact.
	c.Templates = map[string]themes.Config{themes.Default: c.Templates[themes.Default]}
	saved, err = e.svc.Save(ctx, e.a.id, e.a.slug, 0, model.SaveInput{Version: &saved.Version, Config: &c})
	e.must(err)
	if !reflect.DeepEqual(saved.Config.Templates["classic"], tc) {
		t.Fatal("inactive template configuration lost")
	}
	for _, status := range saved.SectionStatuses {
		if status.ID == "newsletter" && status.Status != "available" {
			t.Fatal(status)
		}
		if status.ID == "branches" && status.Status != "unsupported_by_theme" {
			t.Fatal(status)
		}
		if status.ID == "app_promo" && status.CanEnable {
			t.Fatal(status)
		}
	}
}

func TestConfigV2SwitchTechnowideHorizonPreservesBoth(t *testing.T) {
	c, err := usecase.NormalizeConfig(model.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	tw := c.Templates[themes.Default]
	tw.Settings["heroLayout"] = "split"
	for i := range tw.Sections {
		if tw.Sections[i].ID == "faq" {
			tw.Sections[i].Enabled = false
		}
	}
	c.Templates[themes.Default] = tw

	// Switch to Horizon: it gets its defaults, Technowide keeps its edits.
	c.Template = "horizon-design"
	c, err = usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	hz := c.Templates["horizon-design"]
	d, _ := themes.Find("horizon-design")
	if !reflect.DeepEqual(hz, themes.Defaults(d)) || hz.Settings["heroAlign"] != "left" {
		t.Fatalf("horizon defaults: %+v", hz)
	}
	if !reflect.DeepEqual(c.Templates[themes.Default], tw) {
		t.Fatal("technowide configuration lost on switch")
	}

	// Edit Horizon: center the hero, move fleet before campaigns, hide about.
	hz.Settings["heroAlign"] = "center"
	hz.Sections = []themes.Section{{ID: "hero", Enabled: true}, {ID: "fleet", Enabled: true}, {ID: "search", Enabled: true}, {ID: "campaigns", Enabled: true}, {ID: "about"}, {ID: "faq", Enabled: true}, {ID: "guide"}}
	c.Templates["horizon-design"] = hz
	c, err = usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}

	// Back to Technowide and again to Horizon: both keep their own state.
	c.Template = themes.Default
	c, err = usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Template = "horizon-design"
	c, err = usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Templates["horizon-design"], hz) {
		t.Fatalf("horizon configuration lost: %+v", c.Templates["horizon-design"])
	}
	if !reflect.DeepEqual(c.Templates[themes.Default], tw) {
		t.Fatal("technowide configuration lost")
	}
}

func TestConfigV2SwitchToClassicPreservesEveryTemplate(t *testing.T) {
	c, err := usecase.NormalizeConfig(model.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	tw := c.Templates[themes.Default]
	tw.Settings["heroLayout"] = "split"
	c.Templates[themes.Default] = tw

	// Horizon with its own edit, then classic: it gets its defaults.
	c.Template = "horizon-design"
	c, err = usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	hz := c.Templates["horizon-design"]
	hz.Settings["heroAlign"] = "center"
	c.Templates["horizon-design"] = hz
	c.Template = "classic"
	c, err = usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := themes.Find("classic")
	cl := c.Templates["classic"]
	if !reflect.DeepEqual(cl, themes.Defaults(d)) || cl.Settings["fleetColumns"] != "3" {
		t.Fatalf("classic defaults: %+v", cl)
	}

	// Edit classic: two columns, branches before the fleet, campaigns off.
	cl.Settings["fleetColumns"] = "2"
	cl.Sections = []themes.Section{{ID: "hero", Enabled: true}, {ID: "branches", Enabled: true}, {ID: "search", Enabled: true}, {ID: "fleet", Enabled: true}, {ID: "advantages", Enabled: true}, {ID: "about", Enabled: true}, {ID: "faq", Enabled: true}, {ID: "campaigns"}, {ID: "guide", Enabled: true}}
	c.Templates["classic"] = cl
	c, err = usecase.NormalizeConfig(c)
	if err != nil {
		t.Fatal(err)
	}

	// Round trip through the other templates: everyone keeps their state.
	for _, id := range []string{themes.Default, "horizon-design", "classic"} {
		c.Template = id
		if c, err = usecase.NormalizeConfig(c); err != nil {
			t.Fatal(id, err)
		}
	}
	if !reflect.DeepEqual(c.Templates["classic"], cl) {
		t.Fatalf("classic configuration lost: %+v", c.Templates["classic"])
	}
	if !reflect.DeepEqual(c.Templates["horizon-design"], hz) {
		t.Fatal("horizon configuration lost")
	}
	if !reflect.DeepEqual(c.Templates[themes.Default], tw) {
		t.Fatal("technowide configuration lost")
	}
}

func TestConfigV2ClassicAndAurumKeepTheirOwnColumns(t *testing.T) {
	c, err := usecase.NormalizeConfig(model.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	c.Template = "classic"
	if c, err = usecase.NormalizeConfig(c); err != nil {
		t.Fatal(err)
	}
	cl := c.Templates["classic"]
	cl.Settings["fleetColumns"] = "2"
	c.Templates["classic"] = cl

	// Aurum gets its own defaults: the same setting key is per template.
	c.Template = "aurum"
	if c, err = usecase.NormalizeConfig(c); err != nil {
		t.Fatal(err)
	}
	d, _ := themes.Find("aurum")
	au := c.Templates["aurum"]
	if !reflect.DeepEqual(au, themes.Defaults(d)) || au.Settings["fleetColumns"] != "3" {
		t.Fatalf("aurum defaults: %+v", au)
	}
	au.Sections = []themes.Section{{ID: "hero", Enabled: true}, {ID: "fleet", Enabled: true}, {ID: "search", Enabled: true}, {ID: "advantages"}, {ID: "campaigns", Enabled: true}, {ID: "about", Enabled: true}, {ID: "faq", Enabled: true}, {ID: "guide", Enabled: true}}
	c.Templates["aurum"] = au

	for _, id := range []string{"classic", themes.Default, "horizon-design", "aurum"} {
		c.Template = id
		if c, err = usecase.NormalizeConfig(c); err != nil {
			t.Fatal(id, err)
		}
	}
	if !reflect.DeepEqual(c.Templates["aurum"], au) {
		t.Fatalf("aurum configuration lost: %+v", c.Templates["aurum"])
	}
	if !reflect.DeepEqual(c.Templates["classic"], cl) || c.Templates["classic"].Settings["fleetColumns"] != "2" {
		t.Fatalf("classic configuration lost: %+v", c.Templates["classic"])
	}
}
