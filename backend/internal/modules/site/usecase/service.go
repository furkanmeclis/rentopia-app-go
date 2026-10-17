// Package usecase implements the branded site (TEC-729,
// docs/design/branded-site.md): the per-organization site settings
// (theme, layout, content, SEO, analytics, address) and the host
// resolution the `site` app calls on every request.
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
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
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
)

// Keys is the public API key port (publicapi usecase): the site books with
// a publishable `web` key whose allowed origins are the site's hosts.
type Keys interface {
	CreateKey(ctx context.Context, orgID int64, in pamodel.KeyInput) (pamodel.IssuedKey, error)
	GetKey(ctx context.Context, orgID int64, id uuid.UUID) (pamodel.Key, error)
	PatchKey(ctx context.Context, orgID int64, id uuid.UUID, in pamodel.KeyInput) (pamodel.Key, error)
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
	q        *db.Queries
	keys     Keys
	settings SettingsReader
	storage  Downloader
	domains  CustomDomains
	base     BaseURL
	portal   string
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

// SetStorage enables the image stream.
func (s *Service) SetStorage(d Downloader) { s.storage = d }

// Downloader reads stored objects (storage.Driver).
type Downloader interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
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
		return model.Settings{
			Subdomain: sub, DefaultLocale: model.Locales[0], Config: cfg, URL: hosts[0].Origin, Hosts: hosts,
		}, nil
	}
	if err != nil {
		return model.Settings{}, err
	}
	hosts, err := s.siteHosts(ctx, orgID, row.Subdomain)
	if err != nil {
		return model.Settings{}, err
	}
	out := model.Settings{
		Exists: true, Enabled: row.Enabled, Subdomain: row.Subdomain, DefaultLocale: row.DefaultLocale,
		Config: decodeConfig(row.Config), Version: int(row.Version), URL: hosts[0].Origin, Hosts: hosts,
		Assets: model.AssetState{Logo: row.LogoObjectKey.Valid, Favicon: row.FaviconObjectKey.Valid, Cover: row.CoverObjectKey.Valid},
	}
	if row.UpdatedAt.Valid {
		t := row.UpdatedAt.Time.UTC()
		out.UpdatedAt = &t
	}
	if row.ApiKeyUuid.Valid {
		k, err := s.keys.GetKey(ctx, orgID, uuid.UUID(row.ApiKeyUuid.Bytes))
		if err == nil {
			out.APIKey = &model.KeyRef{UUID: k.UUID, Status: k.Status}
			usable := k.Status == pamodel.KeyActive && (k.ExpiresAt == nil || k.ExpiresAt.After(s.now()))
			if usable {
				active, err := s.channelActive(ctx, orgID)
				if err != nil {
					return model.Settings{}, err
				}
				out.BookingEnabled = row.Enabled && active
			}
		}
	}
	return out, nil
}

// ensureKey returns the id of a usable publishable web key for the site,
// creating one when there is none, and keeps the site's origins in its
// allowed origins (previous site origins are replaced).
func (s *Service) ensureKey(ctx context.Context, orgID int64, current pgtype.UUID, previous, next []string,
	create bool) (pgtype.Int8, error) {
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
		cfg = *in.Config
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
			OrganizationID: orgID, Enabled: enabled, Subdomain: sub, DefaultLocale: locale, Config: data,
			ApiKeyID: keyID, UpdatedBy: by,
		})
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return model.Settings{}, ErrVersionConflict
	case isUnique(err, "uq_site_settings_subdomain"):
		return model.Settings{}, ErrSubdomainTaken
	case err != nil:
		return model.Settings{}, err
	}
	return s.Get(ctx, orgID, orgSlug)
}

func isUnique(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == constraint
}

// ---------------------------------------------------------------- public

// resolvedRow is the shared shape of the two resolve lookups.
type resolvedRow = db.ResolveSiteBySubdomainRow

// lookup finds the enabled site of a host.
func (s *Service) lookup(ctx context.Context, host string) (resolvedRow, model.Host, error) {
	if sub, ok := s.base.Subdomain(host); ok {
		row, err := s.q.ResolveSiteBySubdomain(ctx, sub)
		if errors.Is(err, pgx.ErrNoRows) {
			return resolvedRow{}, model.Host{}, ErrNotFound
		}
		if err != nil {
			return resolvedRow{}, model.Host{}, err
		}
		return row, model.Host{Host: s.base.Host(sub), Origin: s.base.Origin(sub), Kind: model.HostSubdomain}, nil
	}
	if s.domains == nil || host == s.base.host {
		return resolvedRow{}, model.Host{}, ErrNotFound
	}
	orgID, err := s.domains.Lookup(ctx, host)
	if err != nil {
		return resolvedRow{}, model.Host{}, err
	}
	row, err := s.q.ResolveSiteByOrganization(ctx, orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return resolvedRow{}, model.Host{}, ErrNotFound
	}
	if err != nil {
		return resolvedRow{}, model.Host{}, err
	}
	return resolvedRow(row), model.Host{Host: host, Origin: "https://" + host, Kind: model.HostCustom}, nil
}

func (s *Service) assetPath(site uuid.UUID, kind string, version int32) *string {
	p := fmt.Sprintf("/v1/public/site/assets/%s/%s?v=%d", site, kind, version)
	return &p
}

// Resolve returns the site a request host names. Unknown hosts, disabled
// sites and unavailable organizations are all ErrNotFound.
func (s *Service) Resolve(ctx context.Context, rawHost string) (model.Resolved, error) {
	host, ok := NormalizeHost(rawHost)
	if !ok {
		return model.Resolved{}, ErrNotFound
	}
	row, h, err := s.lookup(ctx, host)
	if err != nil {
		return model.Resolved{}, err
	}
	now := s.now()
	if row.OrganizationStatus != orgctx.StatusActive || (row.AccessStartsAt.Valid && now.Before(row.AccessStartsAt.Time)) ||
		(row.AccessEndsAt.Valid && !now.Before(row.AccessEndsAt.Time)) {
		return model.Resolved{}, ErrNotFound
	}
	out := model.Resolved{
		SiteUUID: row.Uuid, Version: int(row.Version), Host: h.Host, HostKind: h.Kind, Origin: h.Origin,
		DefaultLocale: row.DefaultLocale, Locales: slices.Clone(model.Locales),
		Organization: model.Organization{UUID: row.OrganizationUuid, Slug: row.OrganizationSlug, Name: row.OrganizationName},
		Config:       decodeConfig(row.Config),
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
	}
	if s.portal != "" {
		out.PortalURL = s.portal + "/r/" + url.PathEscape(row.OrganizationSlug)
	}
	if row.LogoObjectKey.Valid || row.OrganizationLogoObjectKey.Valid {
		out.Assets.Logo = s.assetPath(row.Uuid, "logo", row.Version)
	}
	if row.FaviconObjectKey.Valid {
		out.Assets.Favicon = s.assetPath(row.Uuid, "favicon", row.Version)
	}
	if row.CoverObjectKey.Valid {
		out.Assets.Cover = s.assetPath(row.Uuid, "cover", row.Version)
	}
	if s.keyUsable(row.ApiKeyStatus, row.ApiKeyExpiresAt) && row.ApiKeyKind.String == pamodel.KindPublishable && row.PublishableKey.Valid {
		active, err := s.channelActive(ctx, row.OrganizationID)
		if err != nil {
			return model.Resolved{}, err
		}
		if active {
			out.PublishableKey, out.BookingEnabled = row.PublishableKey.String, true
		}
	}
	return out, nil
}

// Asset opens an image of an enabled site (the logo falls back to the
// organization's logo).
func (s *Service) Asset(ctx context.Context, site uuid.UUID, kind string) (io.ReadCloser, string, error) {
	if s.storage == nil || !slices.Contains(model.AssetKinds, kind) {
		return nil, "", ErrNotFound
	}
	row, err := s.q.GetSiteAssetKeys(ctx, site)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	var key pgtype.Text
	switch kind {
	case "logo":
		key = row.LogoObjectKey
		if !key.Valid {
			key = row.OrganizationLogoObjectKey
		}
	case "favicon":
		key = row.FaviconObjectKey
	case "cover":
		key = row.CoverObjectKey
	}
	if !key.Valid || key.String == "" {
		return nil, "", ErrNotFound
	}
	rc, _, err := s.storage.Download(ctx, key.String)
	if err != nil {
		s.log.Warn("site_asset_download_failed", "kind", kind, "error", err)
		return nil, "", ErrNotFound
	}
	return rc, storage.MIMEFromLogoKey(key.String), nil
}
