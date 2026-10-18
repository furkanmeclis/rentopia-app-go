package themes

import (
	"slices"
	"testing"
)

func TestHorizonIsSelectable(t *testing.T) {
	d, ok := Find("horizon-design")
	if !ok || !d.Available {
		t.Fatal("horizon-design must be available")
	}
	// The studio's Horizon (template-studio core/registry.ts): no advantages,
	// newsletter or app promotion; search is explicit in every theme.
	want := []string{"hero", "search", "campaigns", "fleet", "guide", "about", "faq"}
	if !slices.Equal(d.Sections, want) {
		t.Fatalf("sections = %v", d.Sections)
	}
	if d.RequiredFirst != "hero" || len(d.Settings) != 1 || d.Settings[0].Key != "heroAlign" ||
		!slices.Equal(d.Settings[0].Values, []string{"left", "center"}) || d.Settings[0].Default != "left" {
		t.Fatalf("settings = %+v", d.Settings)
	}
	for _, d := range Registry() {
		if !d.Available {
			t.Fatalf("%s is registered but unavailable: every template has a renderer", d.ID)
		}
	}
}

func TestAurumIsSelectable(t *testing.T) {
	d, ok := Find("aurum")
	if !ok || !d.Available {
		t.Fatal("aurum must be available")
	}
	// The studio's aurum-design (core/registry.ts): no newsletter, app or
	// branches section; search is explicit.
	want := []string{"hero", "search", "advantages", "fleet", "campaigns", "about", "faq", "guide"}
	if !slices.Equal(d.Sections, want) {
		t.Fatalf("sections = %v", d.Sections)
	}
	if d.RequiredFirst != "hero" || len(d.Settings) != 1 || d.Settings[0].Key != "fleetColumns" ||
		!slices.Equal(d.Settings[0].Values, []string{"2", "3"}) || d.Settings[0].Default != "3" {
		t.Fatalf("settings = %+v", d.Settings)
	}
	if c := Defaults(d); c.Settings["fleetColumns"] != "3" || len(c.Sections) != len(want) {
		t.Fatal(c)
	}
	for _, st := range Statuses("aurum") {
		switch st.ID {
		case "newsletter", "app_promo", "branches":
			if st.Status != "unsupported_by_theme" || st.CanEnable {
				t.Fatal(st)
			}
		default:
			if st.Status != "available" || !st.CanEnable {
				t.Fatal(st)
			}
		}
	}
}

func TestClassicIsSelectable(t *testing.T) {
	d, ok := Find("classic")
	if !ok || !d.Available {
		t.Fatal("classic must be available")
	}
	// The studio's classic (template-studio core/registry.ts): branches
	// instead of newsletter / app promotion; search is explicit.
	want := []string{"hero", "search", "fleet", "advantages", "branches", "about", "faq", "campaigns", "guide"}
	if !slices.Equal(d.Sections, want) {
		t.Fatalf("sections = %v", d.Sections)
	}
	if d.RequiredFirst != "hero" || len(d.Settings) != 1 || d.Settings[0].Key != "fleetColumns" ||
		!slices.Equal(d.Settings[0].Values, []string{"2", "3"}) || d.Settings[0].Default != "3" {
		t.Fatalf("settings = %+v", d.Settings)
	}
	c := Defaults(d)
	if c.Settings["fleetColumns"] != "3" || len(c.Settings) != 1 {
		t.Fatal(c.Settings)
	}
	for _, s := range c.Sections {
		if !s.Enabled {
			t.Fatalf("default %s enabled=%v", s.ID, s.Enabled)
		}
	}
	for _, st := range Statuses("classic") {
		switch st.ID {
		case "newsletter", "app_promo":
			if st.Status != "unsupported_by_theme" || st.CanEnable {
				t.Fatal(st)
			}
		default:
			if st.Status != "available" || !st.CanEnable {
				t.Fatal(st)
			}
		}
	}
}

func TestHorizonDefaultsAndStatuses(t *testing.T) {
	d, _ := Find("horizon-design")
	c := Defaults(d)
	if c.Settings["heroAlign"] != "left" {
		t.Fatal(c.Settings)
	}
	for _, s := range c.Sections {
		if !s.Enabled {
			t.Fatalf("default %s enabled=%v", s.ID, s.Enabled)
		}
	}
	for _, st := range Statuses("horizon-design") {
		switch st.ID {
		case "advantages", "newsletter", "app_promo", "branches":
			if st.Status != "unsupported_by_theme" || st.CanEnable {
				t.Fatal(st)
			}
		default:
			if st.Status != "available" || !st.CanEnable {
				t.Fatal(st)
			}
		}
	}
}
