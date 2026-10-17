package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	pamodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// resolvedRow is the shared shape of the two resolve lookups.
type resolvedRow = db.ResolveSiteBySubdomainRow

// lookup finds the site of a host (enabled or not; the caller decides).
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

// assetPath is the API path of a site image. Draft paths carry draft=1: the
// site's media route then forwards the visitor's preview token.
func assetPath(site uuid.UUID, kind string, version int32, draft bool) *string {
	p := fmt.Sprintf("/v1/public/site/assets/%s/%s?v=%d", site, kind, version)
	if draft {
		p += "&draft=1"
	}
	return &p
}

// PanelOrigin is the origin allowed to frame preview pages (the panel).
func (s *Service) PanelOrigin() string {
	u, err := url.Parse(s.portal)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// Resolve returns the published site a request host names. Unknown hosts,
// disabled or never published sites and unavailable organizations are all
// ErrNotFound. With a preview token (POST /v1/tenant/site/preview-token)
// for this host's site it returns the draft instead, also for a site that
// is off or was never published; a token for another site, or an expired
// one, is ErrPreviewInvalid.
func (s *Service) Resolve(ctx context.Context, rawHost, previewToken string) (model.Resolved, error) {
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
	preview := previewToken != ""
	if preview && !s.verifyPreview(previewToken, row.Uuid) {
		return model.Resolved{}, ErrPreviewInvalid
	}
	if !preview && (!row.Enabled || !row.PublishedRevision.Valid) {
		return model.Resolved{}, ErrNotFound
	}
	out := model.Resolved{
		SiteUUID: row.Uuid, Host: h.Host, HostKind: h.Kind, Origin: h.Origin,
		DefaultLocale: row.DefaultLocale, Locales: slices.Clone(model.Locales),
		Organization: model.Organization{UUID: row.OrganizationUuid, Slug: row.OrganizationSlug, Name: row.OrganizationName},
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
		Preview:      preview,
	}
	logo, favicon, cover := row.PublishedLogoObjectKey, row.PublishedFaviconObjectKey, row.PublishedCoverObjectKey
	var version int32
	if preview {
		out.Config, version = decodeConfig(row.Config), row.Version
		logo, favicon, cover = row.LogoObjectKey, row.FaviconObjectKey, row.CoverObjectKey
		out.PanelOrigin = s.PanelOrigin()
	} else {
		out.Config, version = decodeConfig(row.PublishedConfig), row.PublishedRevision.Int32
		if row.PublishedAt.Valid {
			out.UpdatedAt = row.PublishedAt.Time.UTC()
		}
	}
	out.Version = int(version)
	if s.portal != "" {
		out.PortalURL = s.portal + "/r/" + url.PathEscape(row.OrganizationSlug)
	}
	if logo.Valid || row.OrganizationLogoObjectKey.Valid {
		out.Assets.Logo = assetPath(row.Uuid, "logo", version, preview)
	}
	if favicon.Valid {
		out.Assets.Favicon = assetPath(row.Uuid, "favicon", version, preview)
	}
	if cover.Valid {
		out.Assets.Cover = assetPath(row.Uuid, "cover", version, preview)
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

// Asset opens an image of a published, enabled site (the logo falls back
// to the organization's logo). With a valid preview token for the site it
// opens the draft image instead (the caller must not cache it publicly).
func (s *Service) Asset(ctx context.Context, site uuid.UUID, kind, previewToken string) (io.ReadCloser, string, error) {
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
	logo, favicon, cover := row.PublishedLogoObjectKey, row.PublishedFaviconObjectKey, row.PublishedCoverObjectKey
	switch {
	case previewToken != "":
		if !s.verifyPreview(previewToken, site) {
			return nil, "", ErrNotFound
		}
		logo, favicon, cover = row.LogoObjectKey, row.FaviconObjectKey, row.CoverObjectKey
	case !row.Enabled || !row.PublishedRevision.Valid:
		return nil, "", ErrNotFound
	}
	var key pgtype.Text
	switch kind {
	case "logo":
		key = logo
		if !key.Valid {
			key = row.OrganizationLogoObjectKey
		}
	case "favicon":
		key = favicon
	case "cover":
		key = cover
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
