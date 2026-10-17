package model

// HeroSlide is localized editorial content; media is a library UUID.
type HeroSlide struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	CTALabel string `json:"cta_label"`
	CTAHref  string `json:"cta_href"`
	Media    string `json:"media"`
	Enabled  bool   `json:"enabled"`
}

type Advantage struct {
	Icon  string `json:"icon"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type Brand struct {
	Tagline map[string]string `json:"tagline"`
	Socials map[string]string `json:"socials"`
}

type MenuItem struct {
	Label map[string]string `json:"label"`
	Href  string            `json:"href"`
	Kind  string            `json:"kind"`
}

type Menus struct {
	Header []MenuItem `json:"header"`
	Footer []MenuItem `json:"footer"`
}

// About adds media and localized highlights while preserving content.*.about.
type About struct {
	Image      string              `json:"image"`
	Highlights map[string][]string `json:"highlights"`
}
