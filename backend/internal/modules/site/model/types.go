// Package model holds the branded site types (TEC-729,
// docs/design/branded-site.md): the per-organization site settings the
// panel edits (TEC-730 builds the editor on this document) and the
// resolved site the `site` app loads for a request host.
package model

import (
	"time"

	"github.com/google/uuid"
)

// Resource is the activity / permission resource of the site settings.
const Resource = "tenant.site"

// Locales the site renders. The first is the default.
var Locales = []string{"tr", "en"}

// Theme choices. The site maps each to CSS; free-form values are refused so
// that nothing typed in the panel reaches a stylesheet unchecked.
var (
	Fonts  = []string{"inter", "roboto", "open_sans", "montserrat", "poppins", "lato", "nunito", "system"}
	Radii  = []string{"none", "sm", "md", "lg", "full"}
	Modes  = []string{"light", "dark", "system"}
	Heroes = []string{"centered", "split", "image", "minimal"}
	Cards  = []string{"grid", "list", "compact"}
	Footer = []string{"simple", "columns", "minimal"}
)

// Asset kinds streamed by GET /v1/public/site/assets/{uuid}/{kind}.
var AssetKinds = []string{"logo", "favicon", "cover"}

// ReservedSubdomains never resolve to a site (platform hosts on the same
// base domain).
var ReservedSubdomains = []string{
	"www", "app", "api", "admin", "panel", "platform", "realtime", "mail", "smtp", "static", "cdn", "assets",
	"media", "site", "sites", "help", "docs", "status", "widget", "portal", "auth", "login", "dokploy", "traefik",
}

// Host kinds.
const (
	HostSubdomain = "subdomain"
	// HostCustom is a verified custom domain (TEC-731).
	HostCustom = "custom"
)

// Colors are the theme tokens of one color scheme (#rrggbb).
type Colors struct {
	Primary           string `json:"primary"`
	PrimaryForeground string `json:"primary_foreground"`
	Accent            string `json:"accent"`
	Background        string `json:"background"`
	Foreground        string `json:"foreground"`
	Muted             string `json:"muted"`
	MutedForeground   string `json:"muted_foreground"`
	Border            string `json:"border"`
}

// Theme is the look of the site: tokens become CSS variables at render.
type Theme struct {
	Colors     Colors `json:"colors"`
	DarkColors Colors `json:"dark_colors"`
	Font       string `json:"font"`
	Radius     string `json:"radius"`
	// Mode is light, dark or system (follows the visitor).
	Mode string `json:"mode"`
}

// Layout picks one of the ready-made variants per section.
type Layout struct {
	Hero   string `json:"hero"`
	Cards  string `json:"cards"`
	Footer string `json:"footer"`
}

// FAQItem is one question of the FAQ page.
type FAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Content is the text of one locale. Plain text: blank lines separate
// paragraphs; no HTML is rendered.
type Content struct {
	HeroTitle    string    `json:"hero_title"`
	HeroSubtitle string    `json:"hero_subtitle"`
	About        string    `json:"about"`
	FAQ          []FAQItem `json:"faq"`
	Terms        string    `json:"terms"`
	// Privacy is the KVKK / privacy notice.
	Privacy string `json:"privacy"`
}

// Contact is shown in the footer and on the contact page.
type Contact struct {
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Address  string `json:"address"`
	WhatsApp string `json:"whatsapp"`
}

// SEO is the meta of one locale (empty = the organization name).
type SEO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Analytics IDs (empty = no tracking script).
type Analytics struct {
	GA4MeasurementID string `json:"ga4_measurement_id"`
}

// Config is the editable site document stored in site_settings.config.
type Config struct {
	Theme     Theme              `json:"theme"`
	Layout    Layout             `json:"layout"`
	Content   map[string]Content `json:"content"`
	SEO       map[string]SEO     `json:"seo"`
	Contact   Contact            `json:"contact"`
	Analytics Analytics          `json:"analytics"`
}

// DefaultColors / DefaultDarkColors are the neutral starting theme.
func DefaultColors() Colors {
	return Colors{
		Primary: "#1d4ed8", PrimaryForeground: "#ffffff", Accent: "#f59e0b", Background: "#ffffff",
		Foreground: "#0f172a", Muted: "#f1f5f9", MutedForeground: "#475569", Border: "#e2e8f0",
	}
}

// DefaultDarkColors is the dark scheme of the default theme.
func DefaultDarkColors() Colors {
	return Colors{
		Primary: "#60a5fa", PrimaryForeground: "#0b1220", Accent: "#fbbf24", Background: "#0b1220",
		Foreground: "#e2e8f0", Muted: "#1e293b", MutedForeground: "#94a3b8", Border: "#334155",
	}
}

// DefaultConfig is the document of a site that was never saved.
func DefaultConfig() Config {
	return Config{
		Theme:  Theme{Colors: DefaultColors(), DarkColors: DefaultDarkColors(), Font: "inter", Radius: "md", Mode: "light"},
		Layout: Layout{Hero: "centered", Cards: "grid", Footer: "simple"},
		Content: map[string]Content{
			"tr": {FAQ: []FAQItem{}},
			"en": {FAQ: []FAQItem{}},
		},
		SEO: map[string]SEO{"tr": {}, "en": {}},
	}
}

// Host is an address the site answers on.
type Host struct {
	Host   string `json:"host"`
	Origin string `json:"origin"`
	Kind   string `json:"kind"`
}

// KeyRef names the publishable key the site books with.
type KeyRef struct {
	UUID   uuid.UUID `json:"uuid"`
	Status string    `json:"status"`
}

// AssetState says which images are uploaded (TEC-730 adds the uploads).
type AssetState struct {
	Logo    bool `json:"logo"`
	Favicon bool `json:"favicon"`
	Cover   bool `json:"cover"`
}

// Settings is GET / PUT /v1/tenant/site.
type Settings struct {
	// Exists is false until the first save (the document is the default).
	Exists        bool   `json:"exists"`
	Enabled       bool   `json:"enabled"`
	Subdomain     string `json:"subdomain"`
	DefaultLocale string `json:"default_locale"`
	Config        Config `json:"config"`
	// Version is 0 before the first save; PUT sends the version it edited.
	Version int `json:"version"`
	// URL is the site's address (its subdomain).
	URL    string     `json:"url"`
	Hosts  []Host     `json:"hosts"`
	APIKey *KeyRef    `json:"api_key"`
	Assets AssetState `json:"assets"`
	// BookingEnabled: the web sales channel is on and the key is usable.
	BookingEnabled bool       `json:"booking_enabled"`
	UpdatedAt      *time.Time `json:"updated_at"`
}

// SaveInput is PUT /v1/tenant/site. Omitted fields keep their value;
// Version is required (0 for the first save).
type SaveInput struct {
	Version       *int    `json:"version"`
	Enabled       *bool   `json:"enabled"`
	Subdomain     *string `json:"subdomain"`
	DefaultLocale *string `json:"default_locale"`
	Config        *Config `json:"config"`
}

// Organization names the site's organization.
type Organization struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

// Assets are the image paths of a resolved site, relative to the API base
// (/v1/public/site/assets/…); nil = none.
type Assets struct {
	Logo    *string `json:"logo"`
	Favicon *string `json:"favicon"`
	Cover   *string `json:"cover"`
}

// Resolved is GET /v1/public/site/resolve: everything the site needs to
// render a host, and the credential it books with.
type Resolved struct {
	SiteUUID      uuid.UUID    `json:"site_uuid"`
	Version       int          `json:"version"`
	Host          string       `json:"host"`
	HostKind      string       `json:"host_kind"`
	Origin        string       `json:"origin"`
	DefaultLocale string       `json:"default_locale"`
	Locales       []string     `json:"locales"`
	Organization  Organization `json:"organization"`
	// PublishableKey mints booking sessions for Origin (POST
	// /v1/public/sessions). Empty when booking is off.
	PublishableKey string `json:"publishable_key"`
	BookingEnabled bool   `json:"booking_enabled"`
	Config         Config `json:"config"`
	Assets         Assets `json:"assets"`
	// PortalURL is the customer portal (reservation lookup, documents).
	PortalURL string    `json:"portal_url"`
	UpdatedAt time.Time `json:"updated_at"`
}
