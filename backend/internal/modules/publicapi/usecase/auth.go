package usecase

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// sessionMACPurpose separates the session token MAC key from every other
// keyed hash of the field cipher.
const sessionMACPurpose = "public_api.session"

// keyRow is the shared shape of the two key lookups.
type keyRow = db.GetPublicAPIKeyByHashRow

// sessionClaims is the signed payload of a booking session token.
type sessionClaims struct {
	Org     int64  `json:"o"`
	Key     int64  `json:"k"`
	Session string `json:"s"`
	Origin  string `json:"g"`
	Expires int64  `json:"e"`
}

func (s *Service) sign(payload string) string {
	return s.cipher.BlindIndex(sessionMACPurpose, "v1", payload)
}

func (s *Service) issueSessionToken(c sessionClaims) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return model.PrefixSession + payload + "." + s.sign(payload), nil
}

func (s *Service) parseSessionToken(token string) (sessionClaims, error) {
	body := strings.TrimPrefix(token, model.PrefixSession)
	payload, sig, ok := strings.Cut(body, ".")
	if !ok || payload == "" || len(sig) != 64 || len(token) > 2048 {
		return sessionClaims{}, ErrUnauthenticated
	}
	if subtle.ConstantTimeCompare([]byte(s.sign(payload)), []byte(sig)) != 1 {
		return sessionClaims{}, ErrUnauthenticated
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return sessionClaims{}, ErrUnauthenticated
	}
	var c sessionClaims
	if err := json.Unmarshal(raw, &c); err != nil || c.Org <= 0 || c.Key <= 0 || c.Session == "" {
		return sessionClaims{}, ErrUnauthenticated
	}
	if !time.Unix(c.Expires, 0).After(s.clock()) {
		return sessionClaims{}, ErrUnauthenticated
	}
	return c, nil
}

// keyUsable reports whether a key may be used now.
func (s *Service) keyUsable(k keyRow, kind string) bool {
	if k.Kind != kind || k.Status != model.KeyActive {
		return false
	}
	return !k.ExpiresAt.Valid || k.ExpiresAt.Time.After(s.clock())
}

// checkOrganization refuses suspended organizations and those outside
// their access window.
func (s *Service) checkOrganization(k keyRow) error {
	now := s.clock()
	if k.OrganizationStatus != orgctx.StatusActive || (k.AccessStartsAt.Valid && now.Before(k.AccessStartsAt.Time)) ||
		(k.AccessEndsAt.Valid && !now.Before(k.AccessEndsAt.Time)) {
		return ErrOrganizationUnavailable
	}
	return nil
}

// checkChannel is the feature gate: the key sells as a sales channel that
// the organization must have switched on (orgsettings sales_channels).
// Package entitlements (public_api / widget / site) plug in here once
// packages exist (TEC-674).
func (s *Service) checkChannel(ctx context.Context, orgID int64, channel string) error {
	if s.settings == nil {
		return nil
	}
	v, err := s.settings.Current(ctx, orgID)
	if err != nil {
		return err
	}
	for _, c := range v.Settings.SalesChannels {
		if c.Code == channel {
			if c.IsActive {
				return nil
			}
			break
		}
	}
	return ErrChannelDisabled
}

func credentialOf(k keyRow) model.Credential {
	return model.Credential{
		OrganizationID: k.OrganizationID, OrganizationUUID: k.OrganizationUuid, OrganizationSlug: k.OrganizationSlug,
		OrganizationName: k.OrganizationName, OrganizationStat: k.OrganizationStatus, KeyID: k.ID, KeyUUID: k.Uuid,
		Kind: k.Kind, Channel: k.Channel, Scopes: k.Scopes, RateLimit: int(k.RateLimitPerMinute),
	}
}

func (s *Service) touch(ctx context.Context, k keyRow, ip string) {
	var addr *netip.Addr
	if a, err := netip.ParseAddr(strings.TrimSpace(ip)); err == nil {
		addr = &a
	}
	if err := s.q.TouchPublicAPIKey(ctx, db.TouchPublicAPIKeyParams{
		OrganizationID: k.OrganizationID, ID: k.ID, Now: ts(s.clock()), Ip: addr,
	}); err != nil {
		s.log.Warn("public_api_key_touch_failed", "error", err)
	}
}

// Authenticate resolves the bearer of a /v1/public request. origin is the
// browser origin ("" for server calls): a secret key is refused from a
// browser, a booking session only answers its own origin.
func (s *Service) Authenticate(ctx context.Context, bearer, origin, ip string) (model.Credential, error) {
	bearer = strings.TrimSpace(bearer)
	var k keyRow
	var cred model.Credential
	switch {
	case strings.HasPrefix(bearer, model.PrefixSecret):
		if origin != "" {
			return model.Credential{}, ErrSecretKeyInBrowser
		}
		row, err := s.q.GetPublicAPIKeyByHash(ctx, TokenHash(bearer))
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Credential{}, ErrUnauthenticated
		}
		if err != nil {
			return model.Credential{}, err
		}
		if !s.keyUsable(row, model.KindSecret) {
			return model.Credential{}, ErrUnauthenticated
		}
		k, cred = row, credentialOf(row)
	case strings.HasPrefix(bearer, model.PrefixSession):
		c, err := s.parseSessionToken(bearer)
		if err != nil {
			return model.Credential{}, err
		}
		row, err := s.q.GetPublicAPIKeyByID(ctx, db.GetPublicAPIKeyByIDParams{OrganizationID: c.Org, ID: c.Key})
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Credential{}, ErrUnauthenticated
		}
		if err != nil {
			return model.Credential{}, err
		}
		k = keyRow(row)
		if !s.keyUsable(k, model.KindPublishable) {
			return model.Credential{}, ErrUnauthenticated
		}
		// The origin must still be allowed (removing it ends its sessions)
		// and a browser call must come from the session's own origin.
		if !contains(k.AllowedOrigins, c.Origin) || (origin != "" && origin != c.Origin) {
			return model.Credential{}, ErrOriginNotAllowed
		}
		cred = credentialOf(k)
		cred.SessionID, cred.Origin = c.Session, c.Origin
	default:
		return model.Credential{}, ErrUnauthenticated
	}
	if err := s.checkOrganization(k); err != nil {
		return model.Credential{}, err
	}
	if err := s.checkChannel(ctx, k.OrganizationID, k.Channel); err != nil {
		return model.Credential{}, err
	}
	s.touch(ctx, k, ip)
	return cred, nil
}

// MintSession turns a publishable key used from an allowed origin into a
// short-lived booking session token (the long-lived secret never reaches
// a browser).
func (s *Service) MintSession(ctx context.Context, publishable, origin, ip string) (model.Session, model.Credential, error) {
	publishable = strings.TrimSpace(publishable)
	if !strings.HasPrefix(publishable, model.PrefixPublishable) {
		if strings.HasPrefix(publishable, model.PrefixSecret) {
			return model.Session{}, model.Credential{}, ErrSecretKeyInBrowser
		}
		return model.Session{}, model.Credential{}, ErrUnauthenticated
	}
	row, err := s.q.GetPublicAPIKeyByHash(ctx, TokenHash(publishable))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Session{}, model.Credential{}, ErrUnauthenticated
	}
	if err != nil {
		return model.Session{}, model.Credential{}, err
	}
	if !s.keyUsable(row, model.KindPublishable) {
		return model.Session{}, model.Credential{}, ErrUnauthenticated
	}
	norm, ok := NormalizeOrigin(origin)
	if !ok || !contains(row.AllowedOrigins, norm) {
		return model.Session{}, model.Credential{}, ErrOriginNotAllowed
	}
	if err := s.checkOrganization(row); err != nil {
		return model.Session{}, model.Credential{}, err
	}
	if err := s.checkChannel(ctx, row.OrganizationID, row.Channel); err != nil {
		return model.Session{}, model.Credential{}, err
	}
	sid, err := randomToken(12)
	if err != nil {
		return model.Session{}, model.Credential{}, err
	}
	exp := s.clock().Add(s.limits.SessionTTL).Truncate(time.Second)
	token, err := s.issueSessionToken(sessionClaims{Org: row.OrganizationID, Key: row.ID, Session: sid, Origin: norm, Expires: exp.Unix()})
	if err != nil {
		return model.Session{}, model.Credential{}, err
	}
	s.touch(ctx, row, ip)
	cred := credentialOf(row)
	cred.SessionID, cred.Origin = sid, norm
	return model.Session{
		Token: token, ExpiresAt: exp, Channel: row.Channel,
		Organization: model.Organization{Slug: row.OrganizationSlug, Name: row.OrganizationName, Currency: string(money.TRY)},
	}, cred, nil
}

// credentialContext scopes a context to the credential's organization.
func credentialContext(ctx context.Context, c model.Credential) context.Context {
	return orgContext(ctx, c.OrganizationID, c.OrganizationUUID, c.OrganizationSlug, c.OrganizationName, c.OrganizationStat)
}

// keyIDArg is the key id of a credential as a nullable column.
func keyIDArg(c model.Credential) pgtype.Int8 { return pgtype.Int8{Int64: c.KeyID, Valid: c.KeyID > 0} }
