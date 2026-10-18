// Package themes defines the versioned storefront template contract.
package themes

import "slices"

const Default = "technowide-design"

type Section struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type Setting struct {
	Key     string   `json:"key"`
	Type    string   `json:"type"`
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

type Definition struct {
	ID            string    `json:"id"`
	Available     bool      `json:"available"`
	Sections      []string  `json:"sections"`
	RequiredFirst string    `json:"required_first"`
	Settings      []Setting `json:"settings"`
}

type Config struct {
	Sections []Section         `json:"sections"`
	Settings map[string]string `json:"settings"`
}

type SectionStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CanEnable bool   `json:"can_enable"`
}

var SectionIDs = []string{"hero", "search", "fleet", "campaigns", "advantages", "about", "faq", "guide", "newsletter", "app_promo", "branches"}

func Registry() []Definition {
	return []Definition{
		{
			ID: Default, Available: true, RequiredFirst: "hero",
			Sections: []string{"hero", "search", "campaigns", "fleet", "advantages", "newsletter", "guide", "about", "faq", "app_promo"},
			Settings: []Setting{{Key: "heroLayout", Type: "enum", Values: []string{"image", "split"}, Default: "image"}},
		},
		{
			ID: "horizon-design", Available: true, RequiredFirst: "hero",
			Sections: []string{"hero", "search", "campaigns", "fleet", "guide", "about", "faq"},
			Settings: []Setting{{Key: "heroAlign", Type: "enum", Values: []string{"left", "center"}, Default: "left"}},
		},
		{
			ID: "classic", Available: true, RequiredFirst: "hero",
			Sections: []string{"hero", "search", "fleet", "advantages", "branches", "about", "faq", "campaigns", "guide"},
			Settings: []Setting{{Key: "fleetColumns", Type: "enum", Values: []string{"2", "3"}, Default: "3"}},
		},
		{
			ID: "aurum", RequiredFirst: "hero",
			Sections: []string{"hero", "search", "advantages", "fleet", "campaigns", "about", "faq", "guide"},
			Settings: []Setting{{Key: "fleetColumns", Type: "enum", Values: []string{"2", "3"}, Default: "3"}},
		},
	}
}

func Find(id string) (Definition, bool) {
	for _, d := range Registry() {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

func Defaults(d Definition) Config {
	c := Config{Sections: []Section{}, Settings: map[string]string{}}
	for _, id := range d.Sections {
		c.Sections = append(c.Sections, Section{ID: id, Enabled: id != "app_promo" && !NeedsService(id)})
	}
	for _, s := range d.Settings {
		c.Settings[s.Key] = s.Default
	}
	return c
}

func NeedsService(_ string) bool {
	return false
}

func Statuses(template string) []SectionStatus {
	d, _ := Find(template)
	out := []SectionStatus{}
	for _, id := range SectionIDs {
		status := "available"
		if !slices.Contains(d.Sections, id) {
			status = "unsupported_by_theme"
		} else if NeedsService(id) {
			status = "needs_service"
		}
		out = append(out, SectionStatus{ID: id, Status: status, CanEnable: status == "available" && id != "app_promo"})
	}
	return out
}
