package usecase

import (
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/google/uuid"
)

// Text limits of the site document.
const (
	maxHeroTitle    = 120
	maxHeroSubtitle = 300
	maxLongText     = 20000
	maxFAQ          = 40
	maxQuestion     = 300
	maxAnswer       = 3000
	maxSEOTitle     = 70
	maxSEODesc      = 300
	maxContactField = 300
	maxWidgetHosts  = 20
	maxRentalDays   = 90
)

var (
	colorRe     = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	subdomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	ga4Re       = regexp.MustCompile(`^G-[A-Z0-9]{4,16}$`)
	phoneRe     = regexp.MustCompile(`^\+?[0-9 ()-]{7,20}$`)
)

// Detail is a field problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400 with every problem of the document.
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if e == nil || len(e.Details) == 0 {
		return nil
	}
	return e
}

// NormalizeSubdomain lower-cases and trims a subdomain label.
func NormalizeSubdomain(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// SubdomainFromSlug derives the default subdomain of an organization.
func SubdomainFromSlug(slug string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(slug) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.' || r == ' ':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	if out == "" || slices.Contains(model.ReservedSubdomains, out) {
		out = "site-" + out
		out = strings.Trim(out, "-")
	}
	return out
}

func validateSubdomain(ve *ValidationError, v string) {
	switch {
	case !subdomainRe.MatchString(v):
		ve.add("subdomain", "invalid", "a subdomain is 1-63 lowercase letters, digits or hyphens and cannot start or end with a hyphen")
	case slices.Contains(model.ReservedSubdomains, v):
		ve.add("subdomain", "reserved", "this subdomain is reserved")
	}
}

func checkLen(ve *ValidationError, field, v string, maxLen int) {
	if utf8.RuneCountInString(v) > maxLen {
		ve.add(field, "too_long", "at most "+strconv.Itoa(maxLen)+" characters")
	}
	if strings.ContainsRune(v, 0) {
		ve.add(field, "invalid", "contains a NUL character")
	}
}

func checkColors(ve *ValidationError, prefix string, c *model.Colors, def model.Colors) {
	fields := []struct {
		name string
		v    *string
		def  string
	}{
		{"primary", &c.Primary, def.Primary}, {"primary_foreground", &c.PrimaryForeground, def.PrimaryForeground},
		{"accent", &c.Accent, def.Accent}, {"background", &c.Background, def.Background},
		{"foreground", &c.Foreground, def.Foreground}, {"muted", &c.Muted, def.Muted},
		{"muted_foreground", &c.MutedForeground, def.MutedForeground}, {"border", &c.Border, def.Border},
	}
	for _, f := range fields {
		*f.v = strings.TrimSpace(*f.v)
		if *f.v == "" {
			*f.v = f.def
			continue
		}
		if !colorRe.MatchString(*f.v) {
			ve.add(prefix+"."+f.name, "invalid", "a color is #rrggbb")
			continue
		}
		*f.v = strings.ToLower(*f.v)
	}
}

func oneOf(ve *ValidationError, field string, v *string, allowed []string, def string) {
	*v = strings.TrimSpace(*v)
	if *v == "" {
		*v = def
		return
	}
	if !slices.Contains(allowed, *v) {
		ve.add(field, "invalid", "one of: "+strings.Join(allowed, ", "))
	}
}

// NormalizeConfig fills defaults and validates the whole document. Every
// value reaching the site is either an enum, a #rrggbb color, a checked ID
// or plain text the site escapes.
func NormalizeConfig(c model.Config) (model.Config, error) {
	ve := &ValidationError{}
	def := model.DefaultConfig()

	checkColors(ve, "config.theme.colors", &c.Theme.Colors, def.Theme.Colors)
	checkColors(ve, "config.theme.dark_colors", &c.Theme.DarkColors, def.Theme.DarkColors)
	oneOf(ve, "config.theme.font", &c.Theme.Font, model.Fonts, def.Theme.Font)
	oneOf(ve, "config.theme.radius", &c.Theme.Radius, model.Radii, def.Theme.Radius)
	oneOf(ve, "config.theme.mode", &c.Theme.Mode, model.Modes, def.Theme.Mode)
	oneOf(ve, "config.layout.hero", &c.Layout.Hero, model.Heroes, def.Layout.Hero)
	oneOf(ve, "config.layout.cards", &c.Layout.Cards, model.Cards, def.Layout.Cards)
	oneOf(ve, "config.layout.footer", &c.Layout.Footer, model.Footer, def.Layout.Footer)

	content := map[string]model.Content{}
	for loc := range c.Content {
		if !slices.Contains(model.Locales, loc) {
			ve.add("config.content."+loc, "invalid", "locales are "+strings.Join(model.Locales, ", "))
		}
	}
	seo := map[string]model.SEO{}
	for loc := range c.SEO {
		if !slices.Contains(model.Locales, loc) {
			ve.add("config.seo."+loc, "invalid", "locales are "+strings.Join(model.Locales, ", "))
		}
	}
	for _, loc := range model.Locales {
		p := "config.content." + loc
		ct := c.Content[loc]
		ct.HeroTitle, ct.HeroSubtitle = strings.TrimSpace(ct.HeroTitle), strings.TrimSpace(ct.HeroSubtitle)
		ct.About, ct.Terms, ct.Privacy = strings.TrimSpace(ct.About), strings.TrimSpace(ct.Terms), strings.TrimSpace(ct.Privacy)
		checkLen(ve, p+".hero_title", ct.HeroTitle, maxHeroTitle)
		checkLen(ve, p+".hero_subtitle", ct.HeroSubtitle, maxHeroSubtitle)
		checkLen(ve, p+".about", ct.About, maxLongText)
		checkLen(ve, p+".terms", ct.Terms, maxLongText)
		checkLen(ve, p+".privacy", ct.Privacy, maxLongText)
		if len(ct.FAQ) > maxFAQ {
			ve.add(p+".faq", "too_many", "at most "+strconv.Itoa(maxFAQ)+" questions")
		}
		faq := make([]model.FAQItem, 0, len(ct.FAQ))
		for i, f := range ct.FAQ {
			f.Question, f.Answer = strings.TrimSpace(f.Question), strings.TrimSpace(f.Answer)
			fp := p + ".faq." + strconv.Itoa(i)
			if f.Question == "" || f.Answer == "" {
				ve.add(fp, "required", "a question and its answer are required")
			}
			checkLen(ve, fp+".question", f.Question, maxQuestion)
			checkLen(ve, fp+".answer", f.Answer, maxAnswer)
			faq = append(faq, f)
		}
		ct.FAQ = faq
		content[loc] = ct

		s := c.SEO[loc]
		s.Title, s.Description = strings.TrimSpace(s.Title), strings.TrimSpace(s.Description)
		checkLen(ve, "config.seo."+loc+".title", s.Title, maxSEOTitle)
		checkLen(ve, "config.seo."+loc+".description", s.Description, maxSEODesc)
		seo[loc] = s
	}
	c.Content, c.SEO = content, seo

	ct := &c.Contact
	ct.Phone, ct.Email = strings.TrimSpace(ct.Phone), strings.TrimSpace(ct.Email)
	ct.Address, ct.WhatsApp = strings.TrimSpace(ct.Address), strings.TrimSpace(ct.WhatsApp)
	if ct.Phone != "" && !phoneRe.MatchString(ct.Phone) {
		ve.add("config.contact.phone", "invalid", "a phone number has 7-20 digits, spaces, (), - and an optional leading +")
	}
	if ct.WhatsApp != "" && !phoneRe.MatchString(ct.WhatsApp) {
		ve.add("config.contact.whatsapp", "invalid", "a phone number has 7-20 digits, spaces, (), - and an optional leading +")
	}
	if ct.Email != "" {
		if a, err := mail.ParseAddress(ct.Email); err != nil || a.Address != ct.Email {
			ve.add("config.contact.email", "invalid", "not an e-mail address")
		}
	}
	checkLen(ve, "config.contact.email", ct.Email, maxContactField)
	checkLen(ve, "config.contact.address", ct.Address, maxContactField)

	normalizeWidget(ve, &c.Widget)

	c.Analytics.GA4MeasurementID = strings.ToUpper(strings.TrimSpace(c.Analytics.GA4MeasurementID))
	if id := c.Analytics.GA4MeasurementID; id != "" && !ga4Re.MatchString(id) {
		ve.add("config.analytics.ga4_measurement_id", "invalid", "a GA4 measurement ID looks like G-XXXXXXX")
	}
	return c, ve.orNil()
}

// widgetOrigin normalizes an embedding origin: https://host[:port], or
// http for localhost development hosts. "" = invalid.
func widgetOrigin(raw string) string {
	v := strings.TrimSpace(raw)
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	host, ok := NormalizeHost(u.Host)
	if !ok || (!strings.Contains(host, ".") && host != "localhost") {
		return ""
	}
	local := host == "localhost" || strings.HasSuffix(host, ".localhost")
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && local:
	default:
		return ""
	}
	out := u.Scheme + "://" + host
	if p := u.Port(); p != "" && (u.Scheme != "https" || p != "443") && (u.Scheme != "http" || p != "80") {
		out += ":" + p
	}
	return out
}

func normalizeWidget(ve *ValidationError, w *model.Widget) {
	if len(w.AllowedDomains) > maxWidgetHosts {
		ve.add("config.widget.allowed_domains", "too_many", "at most "+strconv.Itoa(maxWidgetHosts)+" domains")
	}
	hosts := make([]string, 0, len(w.AllowedDomains))
	for i, d := range w.AllowedDomains {
		o := widgetOrigin(d)
		if o == "" {
			ve.add("config.widget.allowed_domains."+strconv.Itoa(i), "invalid", "an address is https://example.com (http only for localhost)")
			continue
		}
		if !slices.Contains(hosts, o) {
			hosts = append(hosts, o)
		}
	}
	w.AllowedDomains = hosts
	f := &w.InitialFilters
	f.PickupBranchUUID = strings.ToLower(strings.TrimSpace(f.PickupBranchUUID))
	if f.PickupBranchUUID != "" {
		if _, err := uuid.Parse(f.PickupBranchUUID); err != nil {
			ve.add("config.widget.initial_filters.pickup_branch_uuid", "invalid", "not a branch id")
		}
	}
	if f.RentalDays < 0 || f.RentalDays > maxRentalDays {
		ve.add("config.widget.initial_filters.rental_days", "out_of_range", "0 to "+strconv.Itoa(maxRentalDays)+" days")
	}
}
