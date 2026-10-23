// Package usecase implements the branded site (TEC-729 / TEC-730,
// docs/design/branded-site.md): the per-organization site settings
// (theme, layout, content, SEO, analytics, address), draft / publish with
// a revision history, preview tokens, image uploads, the online publish
// state of the catalog and the host resolution the `site` app calls on
// every request.
//
// Live vs draft: enabled, subdomain, default_locale and the linked key take
// effect on save. The document (config) and the images are a draft until
// published; visitors only ever get the published snapshot, and the draft
// only reaches a browser with a valid preview token for that site.
//
// The organization always comes from the host (a subdomain of the site
// base URL now, a verified custom domain with TEC-731) or, in the panel,
// from orgctx. A host that names no enabled site, or whose organization is
// suspended, reads as not found: another organization's content is never
// served for it.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	pamodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/themes"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// SiteChannel is the sales channel the site sells as (orgsettings code).
const SiteChannel = "web"

// siteKeyName names the publishable key the site creates for itself.
const siteKeyName = "Markalı site"

var (
	// ErrNotFound: no enabled site answers the host (404).
	ErrNotFound = errors.New("site not found")
	// ErrVersionConflict: the settings changed since the caller read them (409).
	ErrVersionConflict = errors.New("the site settings were changed by someone else; reload and try again")
	// ErrSubdomainTaken: another organization uses the subdomain (409).
	ErrSubdomainTaken = errors.New("this subdomain is already in use")
	// ErrNothingToPublish: the draft equals the published snapshot (409).
	ErrNothingToPublish = errors.New("there are no unpublished changes")
	// ErrRevisionNotFound: no such published revision (404).
	ErrRevisionNotFound = errors.New("revision not found")
	// ErrCatalogItemNotFound: no such branch, subgroup or extra (404).
	ErrCatalogItemNotFound = errors.New("record not found")
	// ErrPreviewInvalid: the preview token is malformed, expired or names
	// another site (401).
	ErrPreviewInvalid = errors.New("the preview link is invalid or has expired")
	// ErrStorageUnavailable: uploads need object storage (503).
	ErrStorageUnavailable = errors.New("storage is not configured")
)

// Keys is the public API key port (publicapi usecase): the site books with
// a publishable `web` key whose allowed origins are the site's hosts.
type Keys interface {
	CreateKey(ctx context.Context, orgID int64, in pamodel.KeyInput) (pamodel.IssuedKey, error)
	GetKey(ctx context.Context, orgID int64, id uuid.UUID) (pamodel.Key, error)
	PatchKey(ctx context.Context, orgID int64, id uuid.UUID, in pamodel.KeyInput) (pamodel.Key, error)
	RotateKey(ctx context.Context, orgID int64, id uuid.UUID, in pamodel.RotateInput) (pamodel.IssuedKey, error)
}

// SettingsReader is the organization settings port (sales channels).
type SettingsReader interface {
	Current(ctx context.Context, orgID int64) (osmodel.Version, error)
}

// CustomDomains is the hook for verified custom domains (TEC-731). Nil =
// subdomains only.
type CustomDomains interface {
	// Lookup returns the organization of a verified custom domain, or
	// ErrNotFound.
	Lookup(ctx context.Context, host string) (int64, error)
	// Hosts lists the verified custom domains of an organization (they are
	// added to the site key's allowed origins and shown in the panel).
	Hosts(ctx context.Context, orgID int64) ([]model.Host, error)
}

// DefaultBaseURL is the development site base (the `site` app on :3100).
const DefaultBaseURL = "http://localhost:3100"

// BaseURL is the site base: every organization's site answers on
// {subdomain}.{host}[:port] with the same scheme.
type BaseURL struct {
	scheme, host, port string
}

// ParseBaseURL reads SITE_BASE_URL ("https://rentopia.app",
// "http://localhost:3100").
func ParseBaseURL(raw string) (BaseURL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") {
		return BaseURL{}, fmt.Errorf("site: SITE_BASE_URL must be scheme://host[:port], got %q", raw)
	}
	b := BaseURL{scheme: u.Scheme, host: strings.ToLower(u.Hostname()), port: u.Port()}
	if (b.scheme == "https" && b.port == "443") || (b.scheme == "http" && b.port == "80") {
		b.port = ""
	}
	return b, nil
}

// Host is the host[:port] of a subdomain.
func (b BaseURL) Host(sub string) string {
	h := sub + "." + b.host
	if b.port != "" {
		h += ":" + b.port
	}
	return h
}

// Origin is the browser origin of a subdomain.
func (b BaseURL) Origin(sub string) string { return b.scheme + "://" + b.Host(sub) }

// Subdomain returns the label of a host directly under the base host.
func (b BaseURL) Subdomain(host string) (string, bool) {
	label, ok := strings.CutSuffix(host, "."+b.host)
	if !ok || label == "" || strings.Contains(label, ".") {
		return "", false
	}
	return label, true
}

// NormalizeHost returns the lower-case host name of a Host header value
// without port or trailing dot, or false for anything that is not a plain
// DNS name.
func NormalizeHost(raw string) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(raw))
	if h == "" || len(h) > 260 {
		return "", false
	}
	if strings.Contains(h, ":") {
		host, _, err := net.SplitHostPort(h)
		if err != nil {
			return "", false
		}
		h = host
	}
	h = strings.TrimSuffix(h, ".")
	if h == "" || len(h) > 253 || strings.HasPrefix(h, ".") || strings.Contains(h, "..") {
		return "", false
	}
	for _, r := range h {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return "", false
		}
	}
	return h, true
}

// Service is the branded site.
type Service struct {
	q          *db.Queries
	keys       Keys
	settings   SettingsReader
	storage    Store
	domains    CustomDomains
	base       BaseURL
	portal     string
	previewKey []byte
	// features gates the public site by the organization's plan
	// (TEC-674): site for every host resolution, widget for embedding.
	features features.AccessPolicy
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service. portalBase is the app's public URL (the customer
// portal lives at {portalBase}/r/{slug}).
func New(q *db.Queries, keys Keys, settings SettingsReader, base BaseURL, portalBase string) *Service {
	return &Service{
		q: q, keys: keys, settings: settings, base: base, portal: strings.TrimRight(strings.TrimSpace(portalBase), "/"),
		log: slog.Default(), now: time.Now,
	}
}

// SetFeatures gates the public site and the widget by the organization's
// plan (nil: no gate).
func (s *Service) SetFeatures(p features.AccessPolicy) { s.features = p }

// SetStorage enables image uploads and the image stream.
func (s *Service) SetStorage(d Store) { s.storage = d }

// Store reads and writes stored objects (storage.Driver).
type Store interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// SetCustomDomains plugs in verified custom domains (TEC-731).
func (s *Service) SetCustomDomains(d CustomDomains) { s.domains = d }

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// ----------------------------------------------------------------- panel

// siteOrigins are the origins the site answers on: its subdomain and the
// verified custom domains.
func (s *Service) siteHosts(ctx context.Context, orgID int64, sub string) ([]model.Host, error) {
	hosts := []model.Host{{Host: s.base.Host(sub), Origin: s.base.Origin(sub), Kind: model.HostSubdomain}}
	if s.domains != nil {
		extra, err := s.domains.Hosts(ctx, orgID)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, extra...)
	}
	return hosts, nil
}

func origins(hosts []model.Host) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, h.Origin)
	}
	return out
}

func (s *Service) channelActive(ctx context.Context, orgID int64) (bool, error) {
	if s.settings == nil {
		return true, nil
	}
	v, err := s.settings.Current(ctx, orgID)
	if err != nil {
		return false, err
	}
	for _, c := range v.Settings.SalesChannels {
		if c.Code == SiteChannel {
			return c.IsActive, nil
		}
	}
	return false, nil
}

func (s *Service) keyUsable(status pgtype.Text, expires pgtype.Timestamptz) bool {
	return status.Valid && status.String == pamodel.KeyActive && (!expires.Valid || expires.Time.After(s.now()))
}

// decodeConfig reads a stored document onto the defaults. A document that
// no longer validates (an older shape) falls back to the defaults field by
// field rather than failing the site.
func decodeConfig(raw []byte) model.Config {
	c := model.DefaultConfig()
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &c)
	}
	n, err := NormalizeConfig(c)
	if err != nil {
		n, _ = NormalizeConfig(model.DefaultConfig())
	}
	return n
}

// Get returns the site settings of an organization (the defaults before
// the first save).
func (s *Service) Get(ctx context.Context, orgID int64, orgSlug string) (model.Settings, error) {
	row, err := s.q.GetSiteSettings(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		sub := SubdomainFromSlug(orgSlug)
		hosts, err := s.siteHosts(ctx, orgID, sub)
		if err != nil {
			return model.Settings{}, err
		}
		cfg, _ := NormalizeConfig(model.DefaultConfig())
		blockers := []string{model.BlockSiteDisabled, model.BlockKeyMissing}
		if active, err := s.channelActive(ctx, orgID); err != nil {
			return model.Settings{}, err
		} else if !active {
			blockers = append(blockers, model.BlockChannelOff)
		}
		return model.Settings{
			ThemeRegistry: themes.Registry(), SectionStatuses: themes.Statuses(cfg.Template),
			Subdomain: sub, DefaultLocale: model.Locales[0], Config: cfg, URL: hosts[0].Origin, Hosts: hosts,
			BookingBlockers: blockers, HasUnpublishedChanges: true,
		}, nil
	}
	if err != nil {
		return model.Settings{}, err
	}
	hosts, err := s.siteHosts(ctx, orgID, row.Subdomain)
	if err != nil {
		return model.Settings{}, err
	}
	draft := decodeConfig(row.Config)
	out := model.Settings{
		ThemeRegistry: themes.Registry(), SectionStatuses: themes.Statuses(draft.Template),
		Exists: true, Enabled: row.Enabled, Subdomain: row.Subdomain, DefaultLocale: row.DefaultLocale,
		Config: draft, Version: int(row.Version), URL: hosts[0].Origin, Hosts: hosts,
		Assets:          model.AssetState{Logo: row.LogoObjectKey.Valid, Favicon: row.FaviconObjectKey.Valid, Cover: row.CoverObjectKey.Valid},
		BookingBlockers: []string{},
	}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time.UTC()
		out.UpdatedAt = &t
	}
	if row.PublishedRevision.Valid {
		rev := int(row.PublishedRevision.Int32)
		out.PublishedRevision = &rev
		if row.PublishedAt.Valid {
			t := row.PublishedAt.Time.UTC()
			out.PublishedAt = &t
		}
		out.HasUnpublishedChanges = !sameDocument(draft, decodeConfig(row.PublishedConfig)) ||
			row.LogoObjectKey != row.PublishedLogoObjectKey || row.FaviconObjectKey != row.PublishedFaviconObjectKey ||
			row.CoverObjectKey != row.PublishedCoverObjectKey
	} else {
		out.HasUnpublishedChanges = true
	}
	out.Live = row.Enabled && row.PublishedRevision.Valid
	if !row.Enabled {
		out.BookingBlockers = append(out.BookingBlockers, model.BlockSiteDisabled)
	}
	usable := false
	if row.ApiKeyUuid.Valid {
		k, err := s.keys.GetKey(ctx, orgID, uuid.UUID(row.ApiKeyUuid.Bytes))
		if err == nil {
			out.APIKey = &model.KeyRef{UUID: k.UUID, Status: k.Status}
			if k.PublishableKey != nil {
				out.APIKey.PublishableKey = *k.PublishableKey
			}
			usable = k.Status == pamodel.KeyActive && (k.ExpiresAt == nil || k.ExpiresAt.After(s.now()))
			if !usable {
				out.BookingBlockers = append(out.BookingBlockers, model.BlockKeyInactive)
			}
		}
	}
	if out.APIKey == nil {
		out.BookingBlockers = append(out.BookingBlockers, model.BlockKeyMissing)
	}
	active, err := s.channelActive(ctx, orgID)
	if err != nil {
		return model.Settings{}, err
	}
	if !active {
		out.BookingBlockers = append(out.BookingBlockers, model.BlockChannelOff)
	}
	out.BookingEnabled = row.Enabled && usable && active
	return out, nil
}

// sameDocument compares two normalized documents.
func sameDocument(a, b model.Config) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}

// ensureKey returns the id of a usable publishable web key for the site,
// creating one when there is none, and keeps the site's origins in its
// allowed origins (previous site origins are replaced).
func (s *Service) ensureKey(ctx context.Context, orgID int64, current pgtype.UUID, previous, next []string,
	create bool) (pgtype.Int8, error) {
	if current.Valid {
		// A key rotated in Ayarlar → Public API: follow the new key.
		next, err := s.q.GetSiteKeySuccessor(ctx, db.GetSiteKeySuccessorParams{OrganizationID: orgID, Uuid: uuid.UUID(current.Bytes)})
		switch {
		case err == nil:
			current = pgtype.UUID{Bytes: next, Valid: true}
		case !errors.Is(err, pgx.ErrNoRows):
			return pgtype.Int8{}, err
		}
	}
	if current.Valid {
		k, err := s.keys.GetKey(ctx, orgID, uuid.UUID(current.Bytes))
		if err == nil && k.Status == pamodel.KeyActive && (k.ExpiresAt == nil || k.ExpiresAt.After(s.now())) {
			merged := []string{}
			for _, o := range k.AllowedOrigins {
				if !slices.Contains(previous, o) && !slices.Contains(next, o) {
					merged = append(merged, o)
				}
			}
			merged = append(merged, next...)
			if !sameSet(merged, k.AllowedOrigins) {
				if _, err := s.keys.PatchKey(ctx, orgID, k.UUID, pamodel.KeyInput{AllowedOrigins: &merged}); err != nil {
					return pgtype.Int8{}, err
				}
			}
			id, err := s.q.GetSitePublicAPIKeyID(ctx, db.GetSitePublicAPIKeyIDParams{OrganizationID: orgID, Uuid: k.UUID})
			if err != nil {
				return pgtype.Int8{}, err
			}
			return pgtype.Int8{Int64: id, Valid: true}, nil
		}
	}
	if !create {
		// Disabled site without a usable key: keep the link as it is.
		if !current.Valid {
			return pgtype.Int8{}, nil
		}
		id, err := s.q.GetSitePublicAPIKeyID(ctx, db.GetSitePublicAPIKeyIDParams{OrganizationID: orgID, Uuid: uuid.UUID(current.Bytes)})
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.Int8{}, nil
		}
		return pgtype.Int8{Int64: id, Valid: err == nil}, err
	}
	kind, channel, name := pamodel.KindPublishable, SiteChannel, siteKeyName
	scopes := slices.Clone(pamodel.AllScopes)
	allowed := slices.Clone(next)
	issued, err := s.keys.CreateKey(ctx, orgID, pamodel.KeyInput{
		Kind: &kind, Name: &name, Channel: &channel, Scopes: &scopes, AllowedOrigins: &allowed,
	})
	if err != nil {
		return pgtype.Int8{}, err
	}
	id, err := s.q.GetSitePublicAPIKeyID(ctx, db.GetSitePublicAPIKeyIDParams{OrganizationID: orgID, Uuid: issued.Key.UUID})
	if err != nil {
		return pgtype.Int8{}, err
	}
	return pgtype.Int8{Int64: id, Valid: true}, nil
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, v := range a {
		if !slices.Contains(b, v) {
			return false
		}
	}
	return true
}

// Save stores the site settings when in.Version is the version the caller
// edited (0 for the first save). Switching the site on links (or creates)
// its publishable key; the key always allows exactly the site's current
// origins in addition to any origin staff added by hand.
func (s *Service) Save(ctx context.Context, orgID int64, orgSlug string, userID int64, in model.SaveInput) (model.Settings, error) {
	ve := &ValidationError{}
	if in.Version == nil || *in.Version < 0 {
		ve.add("version", "required", "send the version you edited (0 for the first save)")
		return model.Settings{}, ve
	}
	row, err := s.q.GetSiteSettings(ctx, orgID)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.Settings{}, err
	}
	curVersion := 0
	enabled, sub, locale, cfg := false, SubdomainFromSlug(orgSlug), model.Locales[0], model.DefaultConfig()
	var keyUUID pgtype.UUID
	if exists {
		curVersion = int(row.Version)
		enabled, sub, locale, cfg = row.Enabled, row.Subdomain, row.DefaultLocale, decodeConfig(row.Config)
		keyUUID = row.ApiKeyUuid
	}
	if *in.Version != curVersion {
		return model.Settings{}, ErrVersionConflict
	}
	prevSub := sub
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.Subdomain != nil {
		sub = NormalizeSubdomain(*in.Subdomain)
	}
	validateSubdomain(ve, sub)
	if in.DefaultLocale != nil {
		locale = strings.TrimSpace(*in.DefaultLocale)
	}
	if !slices.Contains(model.Locales, locale) {
		ve.add("default_locale", "invalid", "one of: "+strings.Join(model.Locales, ", "))
	}
	if in.Config != nil {
		previousTemplates := cfg.Templates
		cfg = *in.Config
		// An update to one template retains the settings of all other templates.
		merged := map[string]themes.Config{}
		for id, tc := range previousTemplates {
			merged[id] = tc
		}
		for id, tc := range cfg.Templates {
			merged[id] = tc
		}
		cfg.Templates = merged
	}
	cfg, err = NormalizeConfig(cfg)
	if err != nil {
		var cve *ValidationError
		if errors.As(err, &cve) {
			ve.Details = append(ve.Details, cve.Details...)
		} else {
			return model.Settings{}, err
		}
	}
	if err := ve.orNil(); err != nil {
		return model.Settings{}, err
	}
	taken, err := s.q.SiteSubdomainTaken(ctx, db.SiteSubdomainTakenParams{Subdomain: sub, OrganizationID: orgID})
	if err != nil {
		return model.Settings{}, err
	}
	if taken {
		return model.Settings{}, ErrSubdomainTaken
	}
	prevHosts, err := s.siteHosts(ctx, orgID, prevSub)
	if err != nil {
		return model.Settings{}, err
	}
	nextHosts, err := s.siteHosts(ctx, orgID, sub)
	if err != nil {
		return model.Settings{}, err
	}
	keyID, err := s.ensureKey(ctx, orgID, keyUUID, origins(prevHosts), origins(nextHosts), enabled)
	if err != nil {
		return model.Settings{}, err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return model.Settings{}, err
	}
	by := pgtype.Int8{Int64: userID, Valid: userID > 0}
	if exists {
		_, err = s.q.UpdateSiteSettings(ctx, db.UpdateSiteSettingsParams{
			OrganizationID: orgID, ExpectedVersion: int32(curVersion), Enabled: enabled, Subdomain: sub,
			DefaultLocale: locale, Config: data, ApiKeyID: keyID, UpdatedBy: by,
		})
	} else {
		_, err = s.q.InsertSiteSettings(ctx, db.InsertSiteSettingsParams{
			OrganizationID: orgID, Enabled: enabled,
			Subdomain: sub, DefaultLocale: locale, Config: data,
			ApiKeyID: keyID, UpdatedBy: by,
		})
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return model.Settings{}, ErrVersionConflict
	case isUnique(err, "uq_site_settings_subdomain"):
		return model.Settings{}, ErrSubdomainTaken
	case err != nil:
		return model.Settings{}, mediaDBError(err)
	}
	return s.Get(ctx, orgID, orgSlug)
}

func isUnique(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == constraint
}
