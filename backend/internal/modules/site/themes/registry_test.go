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
	for _, id := range []string{"classic", "aurum"} {
		if other, _ := Find(id); other.Available {
			t.Fatalf("%s must stay unavailable until its renderer ships", id)
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
