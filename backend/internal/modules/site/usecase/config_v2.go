package usecase

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/themes"
	"github.com/google/uuid"
)

var advantageIcons = []string{"shield-check", "clock", "car", "credit-card", "headphones", "map-pin", "check", "check-circle", "star", "heart", "award", "wallet", "zap", "users", "phone", "thumbs-up", "badge-check", "circle-check", "key-round", "fuel", "plane", "route", "wrench", "calendar", "globe"}

func safeHref(raw string) bool {
	if raw == "" {
		return true
	}
	if strings.ContainsAny(raw, "\\") || strings.IndexFunc(raw, unicode.IsSpace) >= 0 || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.ContainsAny(decoded, "\\\r\n\x00") || strings.HasPrefix(decoded, "//") {
		return false
	}
	relative := strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") && u.Host == "" && u.Scheme == ""
	https := u.Scheme == "https" && u.Hostname() != "" && (u.Port() == "" || u.Port() == "443")
	return relative || https
}

func localizedText(ve *ValidationError, field string, values map[string]string, limit int) map[string]string {
	out := map[string]string{}
	for loc := range values {
		if !slices.Contains(model.Locales, loc) {
			ve.add(field+"."+loc, "invalid", "unsupported locale")
		}
	}
	for _, loc := range model.Locales {
		out[loc] = strings.TrimSpace(values[loc])
		checkLen(ve, field+"."+loc, out[loc], limit)
	}
	return out
}

func mediaID(ve *ValidationError, field, value string) {
	if value == "" {
		return
	}
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.String() != value {
		ve.add(field, "invalid", "a canonical media UUID is required")
	}
}

func normalizeV2(c *model.Config, ve *ValidationError) {
	if c.Leads == nil {
		c.Leads = &model.LeadConfig{Contact: true, Corporate: true, Franchise: true, Newsletter: true}
	}
	normalizeTemplates(c, ve)
	oneOf(ve, "config.theme.heading_font", &c.Theme.HeadingFont,
		append(append([]string{}, model.Fonts...), "theme", "georgia"), "theme")
	normalizeBrandMenus(c, ve)
	normalizeEditorial(c, ve)
}

func normalizeTemplates(c *model.Config, ve *ValidationError) {
	if c.ConfigVersion != 0 && c.ConfigVersion != 1 && c.ConfigVersion != 2 {
		ve.add("config.config_version", "invalid", "unsupported config version")
	}
	c.ConfigVersion = 2
	if c.Template == "" {
		c.Template = themes.Default
	}
	active, ok := themes.Find(c.Template)
	if !ok || !active.Available {
		ve.add("config.template", "TEMPLATE_UNAVAILABLE", "template is not available")
	}
	if c.Templates == nil {
		c.Templates = map[string]themes.Config{}
	}
	if _, exists := c.Templates[c.Template]; !exists && ok {
		c.Templates[c.Template] = themes.Defaults(active)
	}
	for id, tc := range c.Templates {
		def, exists := themes.Find(id)
		p := "config.templates." + id
		if !exists {
			ve.add(p, "invalid", "unknown template")
			continue
		}
		if tc.Sections == nil {
			tc.Sections = themes.Defaults(def).Sections
		}
		seen := map[string]bool{}
		for i, section := range tc.Sections {
			field := fmt.Sprintf("%s.sections.%d", p, i)
			if !slices.Contains(themes.SectionIDs, section.ID) {
				ve.add(field, "invalid", "unknown section")
			} else if !slices.Contains(def.Sections, section.ID) {
				ve.add(field, "unsupported_by_theme", "section is not supported by this template")
			}
			if seen[section.ID] {
				ve.add(field, "duplicate", "duplicate section")
			}
			seen[section.ID] = true
			if section.Enabled && section.ID == "app_promo" {
				ve.add(field, "SECTION_UNAVAILABLE", "store links are not configured")
			}
			if section.Enabled && themes.NeedsService(section.ID) {
				ve.add(field, "SECTION_UNAVAILABLE", "section requires its backend service")
			}
		}
		if len(tc.Sections) == 0 || tc.Sections[0].ID != def.RequiredFirst || !tc.Sections[0].Enabled {
			ve.add(p+".sections", "invalid", "hero must be first and enabled")
		}
		for _, sid := range def.Sections {
			if !seen[sid] {
				tc.Sections = append(tc.Sections, themes.Section{ID: sid})
			}
		}
		if tc.Settings == nil {
			tc.Settings = map[string]string{}
		}
		for key := range tc.Settings {
			found := false
			for _, setting := range def.Settings {
				if setting.Key == key {
					found = true
				}
			}
			if !found {
				ve.add(p+".settings."+key, "invalid", "unknown setting")
			}
		}
		for _, setting := range def.Settings {
			value, exists := tc.Settings[setting.Key]
			if !exists {
				tc.Settings[setting.Key] = setting.Default
			} else if !slices.Contains(setting.Values, value) {
				ve.add(p+".settings."+setting.Key, "invalid", "unsupported setting value")
			}
		}
		c.Templates[id] = tc
	}
}

func normalizeBrandMenus(c *model.Config, ve *ValidationError) {
	c.Brand.Tagline = localizedText(ve, "config.brand.tagline", c.Brand.Tagline, 300)
	networks := map[string][]string{"instagram": {"instagram.com"}, "facebook": {"facebook.com", "fb.com"}, "x": {"x.com", "twitter.com"}, "youtube": {"youtube.com", "youtu.be"}, "linkedin": {"linkedin.com"}, "tiktok": {"tiktok.com"}, "whatsapp": {"wa.me", "whatsapp.com"}}
	if c.Brand.Socials == nil {
		c.Brand.Socials = map[string]string{}
	}
	for network, raw := range c.Brand.Socials {
		hosts, exists := networks[network]
		u, err := url.Parse(raw)
		valid := exists && safeHref(raw) && err == nil && u.Scheme == "https"
		hostOK := false
		if u != nil {
			for _, host := range hosts {
				if strings.ToLower(u.Hostname()) == host || strings.HasSuffix(strings.ToLower(u.Hostname()), "."+host) {
					hostOK = true
				}
			}
		}
		if !valid || !hostOK {
			ve.add("config.brand.socials."+network, "invalid", "HTTPS URL on the network host required")
		}
		checkLen(ve, "config.brand.socials."+network, raw, 2048)
	}
	for _, menu := range []struct {
		name  string
		items *[]model.MenuItem
	}{{"header", &c.Menus.Header}, {"footer", &c.Menus.Footer}} {
		if *menu.items == nil {
			*menu.items = []model.MenuItem{}
		}
		if len(*menu.items) > 12 {
			ve.add("config.menus."+menu.name, "too_many", "at most 12 items")
		}
		for i := range *menu.items {
			item := &(*menu.items)[i]
			p := fmt.Sprintf("config.menus.%s.%d", menu.name, i)
			item.Label = localizedText(ve, p+".label", item.Label, 120)
			if item.Href == "" || !safeHref(item.Href) {
				ve.add(p+".href", "invalid", "relative path or HTTPS required")
			}
			checkLen(ve, p+".href", item.Href, 2048)
			oneOf(ve, p+".kind", &item.Kind, []string{"link", "page"}, "link")
		}
	}
}

func normalizeEditorial(c *model.Config, ve *ValidationError) {
	mediaID(ve, "config.about.image", c.About.Image)
	if c.About.Highlights == nil {
		c.About.Highlights = map[string][]string{}
	}
	for loc := range c.About.Highlights {
		if !slices.Contains(model.Locales, loc) {
			ve.add("config.about.highlights."+loc, "invalid", "unsupported locale")
		}
	}
	for _, loc := range model.Locales {
		highlights := c.About.Highlights[loc]
		if highlights == nil {
			highlights = []string{}
		}
		if len(highlights) > 6 {
			ve.add("config.about.highlights."+loc, "too_many", "at most 6 highlights")
		}
		for i := range highlights {
			highlights[i] = strings.TrimSpace(highlights[i])
			checkLen(ve, "config.about.highlights."+loc, highlights[i], 300)
		}
		c.About.Highlights[loc] = highlights
		content := c.Content[loc]
		if content.HeroSlides == nil {
			content.HeroSlides = []model.HeroSlide{{Title: content.HeroTitle, Subtitle: content.HeroSubtitle, Enabled: true}}
		}
		if len(content.HeroSlides) < 1 || len(content.HeroSlides) > 8 {
			ve.add("config.content."+loc+".hero_slides", "invalid", "1 to 8 slides required")
		}
		for i := range content.HeroSlides {
			slide := &content.HeroSlides[i]
			p := fmt.Sprintf("config.content.%s.hero_slides.%d", loc, i)
			slide.Title, slide.Subtitle, slide.CTALabel = strings.TrimSpace(slide.Title), strings.TrimSpace(slide.Subtitle), strings.TrimSpace(slide.CTALabel)
			checkLen(ve, p+".title", slide.Title, 200)
			checkLen(ve, p+".subtitle", slide.Subtitle, 1000)
			checkLen(ve, p+".cta_label", slide.CTALabel, 120)
			checkLen(ve, p+".cta_href", slide.CTAHref, 2048)
			if !safeHref(slide.CTAHref) {
				ve.add(p+".cta_href", "invalid", "relative path or HTTPS required")
			}
			mediaID(ve, p+".media", slide.Media)
		}
		if content.Advantages == nil {
			content.Advantages = []model.Advantage{}
		}
		if len(content.Advantages) > 12 {
			ve.add("config.content."+loc+".advantages", "too_many", "at most 12 advantages")
		}
		for i := range content.Advantages {
			a := &content.Advantages[i]
			p := fmt.Sprintf("config.content.%s.advantages.%d", loc, i)
			if !slices.Contains(advantageIcons, a.Icon) {
				ve.add(p+".icon", "invalid", "unsupported Lucide icon")
			}
			a.Title, a.Text = strings.TrimSpace(a.Title), strings.TrimSpace(a.Text)
			checkLen(ve, p+".title", a.Title, 120)
			checkLen(ve, p+".text", a.Text, 1000)
		}
		c.Content[loc] = content
	}
}
