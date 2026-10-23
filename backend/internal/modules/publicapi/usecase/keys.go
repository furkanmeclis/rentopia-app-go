package usecase

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Key limits.
const (
	maxOrigins           = 20
	defaultSecretLimit   = 120
	defaultBrowserLimit  = 60
	minRateLimit         = 10
	maxRateLimit         = 6000
	defaultGraceHours    = 24
	maxGraceHours        = 168
	secretKeyBytes       = 32
	publishableKeyBytes  = 24
	displayPrefixLetters = 4
)

// KeySortColumns are the sortable key list columns.
var KeySortColumns = map[string]string{"name": "name", "last_used_at": "last_used_at", "created_at": "created_at"}

func actorArg(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

// keyView maps a key row (list / get share the shape).
func keyView(k db.PublicApiKey, createdBy pgtype.UUID, name, surname pgtype.Text, rotatedFrom pgtype.UUID) model.Key {
	out := model.Key{
		UUID: k.Uuid, Kind: k.Kind, Name: k.Name, Channel: k.Channel, KeyPrefix: k.KeyPrefix, KeyLast4: k.KeyLast4,
		Scopes: k.Scopes, AllowedOrigins: k.AllowedOrigins, RateLimitPerMinute: int(k.RateLimitPerMinute), Status: k.Status,
		ExpiresAt: tsPtr(k.ExpiresAt), LastUsedAt: tsPtr(k.LastUsedAt), RevokedAt: tsPtr(k.RevokedAt),
		CreatedAt: k.CreatedAt.Time.UTC(), UpdatedAt: k.UpdatedAt.Time.UTC(),
	}
	if k.PublishableKey.Valid {
		v := k.PublishableKey.String
		out.PublishableKey = &v
	}
	if createdBy.Valid {
		out.CreatedBy = &model.Actor{UUID: createdBy.Bytes, Name: name.String, Surname: surname.String}
	}
	if rotatedFrom.Valid {
		id := uuid.UUID(rotatedFrom.Bytes)
		out.RotatedFromUUID = &id
	}
	if out.Scopes == nil {
		out.Scopes = []string{}
	}
	if out.AllowedOrigins == nil {
		out.AllowedOrigins = []string{}
	}
	return out
}

func listKeyView(r db.ListPublicAPIKeysRow) model.Key {
	return keyView(db.PublicApiKey{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, Kind: r.Kind, Name: r.Name, Channel: r.Channel,
		KeyPrefix: r.KeyPrefix, KeyLast4: r.KeyLast4, PublishableKey: r.PublishableKey, Scopes: r.Scopes,
		AllowedOrigins: r.AllowedOrigins, RateLimitPerMinute: r.RateLimitPerMinute, Status: r.Status, ExpiresAt: r.ExpiresAt,
		LastUsedAt: r.LastUsedAt, RevokedAt: r.RevokedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.RotatedFromUuid)
}

// ListKeys returns a page of the organization's keys.
func (s *Service) ListKeys(ctx context.Context, orgID int64, f model.KeyFilter) ([]model.Key, int64, error) {
	sortField := KeySortColumns[f.SortField]
	params := db.ListPublicAPIKeysParams{
		OrganizationID: orgID, Q: textArg(f.Q), Kinds: nilIfEmpty(f.Kinds), Channels: nilIfEmpty(f.Channels),
		Statuses: nilIfEmpty(f.Statuses), SortField: sortField, SortDesc: f.SortDesc, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	rows, err := s.q.ListPublicAPIKeys(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountPublicAPIKeys(ctx, db.CountPublicAPIKeysParams{
		OrganizationID: orgID, Q: params.Q, Kinds: params.Kinds, Channels: params.Channels, Statuses: params.Statuses,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Key, 0, len(rows))
	for _, r := range rows {
		out = append(out, listKeyView(r))
	}
	return out, total, nil
}

// GetKey returns one key.
func (s *Service) GetKey(ctx context.Context, orgID int64, id uuid.UUID) (model.Key, error) {
	r, err := s.q.GetPublicAPIKey(ctx, db.GetPublicAPIKeyParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Key{}, ErrNotFound
	}
	if err != nil {
		return model.Key{}, err
	}
	return listKeyView(db.ListPublicAPIKeysRow(r)), nil
}

type keyFields struct {
	kind, name, channel string
	scopes, origins     []string
	rateLimit           int
}

// validateKey checks a create (current = nil) or patch input.
func validateKey(in model.KeyInput, current *keyFields) (keyFields, error) {
	ve := &ValidationError{}
	f := keyFields{}
	if current != nil {
		f = *current
	}
	if current == nil {
		switch {
		case in.Kind == nil:
			ve.add("kind", "required", "kind is required")
		case !contains(model.KeyKinds, *in.Kind):
			ve.add("kind", "invalid", "kind is secret or publishable")
		default:
			f.kind = *in.Kind
		}
	} else if in.Kind != nil && *in.Kind != current.kind {
		ve.add("kind", "immutable", "the kind of a key cannot change")
	}
	if in.Name != nil || current == nil {
		name := ""
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
		}
		switch {
		case name == "":
			ve.add("name", "required", "name is required")
		case utf8.RuneCountInString(name) > 120:
			ve.add("name", "too_long", "name is at most 120 characters")
		default:
			f.name = name
		}
	}
	if current == nil {
		f.channel = "api"
		if f.kind == model.KindPublishable {
			f.channel = "web"
		}
		if in.Channel != nil {
			f.channel = strings.TrimSpace(*in.Channel)
		}
		if !contains(model.Channels, f.channel) {
			ve.add("channel", "invalid", "channel is web, widget or api")
		}
	} else if in.Channel != nil && *in.Channel != current.channel {
		ve.add("channel", "immutable", "issue a new key to sell on another channel")
	}
	if in.Scopes != nil || current == nil {
		scopes := model.AllScopes
		if in.Scopes != nil {
			scopes = *in.Scopes
		}
		clean := []string{}
		for i, sc := range scopes {
			sc = strings.TrimSpace(sc)
			if !contains(model.AllScopes, sc) {
				ve.add("scopes."+itoa(i), "invalid", "unknown scope")
				continue
			}
			if !contains(clean, sc) {
				clean = append(clean, sc)
			}
		}
		if len(clean) == 0 {
			ve.add("scopes", "required", "at least one scope is required")
		}
		f.scopes = clean
	}
	if in.AllowedOrigins != nil || current == nil {
		origins := []string{}
		if in.AllowedOrigins != nil {
			origins = *in.AllowedOrigins
		}
		clean := []string{}
		for i, o := range origins {
			norm, ok := NormalizeOrigin(o)
			if !ok {
				ve.add("allowed_origins."+itoa(i), "invalid", "an origin is scheme://host[:port] (https, or http for localhost)")
				continue
			}
			if !contains(clean, norm) {
				clean = append(clean, norm)
			}
		}
		if len(clean) > maxOrigins {
			ve.add("allowed_origins", "too_many", "at most 20 origins")
		}
		f.origins = clean
	}
	switch {
	case f.kind == model.KindPublishable && len(f.origins) == 0:
		ve.add("allowed_origins", "required", "a publishable key needs at least one allowed origin")
	case f.kind == model.KindSecret && len(f.origins) > 0:
		ve.add("allowed_origins", "not_allowed", "secret keys are used server to server; they have no origins")
	}
	if in.RateLimitPerMinute != nil {
		f.rateLimit = *in.RateLimitPerMinute
	} else if current == nil {
		f.rateLimit = defaultSecretLimit
		if f.kind == model.KindPublishable {
			f.rateLimit = defaultBrowserLimit
		}
	}
	if f.rateLimit < minRateLimit || f.rateLimit > maxRateLimit {
		ve.add("rate_limit_per_minute", "out_of_range", "the rate limit is 10 to 6000 requests per minute")
	}
	return f, ve.orNil()
}

// NormalizeOrigin returns the canonical form of a browser origin
// ("https://www.example.com", "http://localhost:3000"): https anywhere,
// http only for localhost / *.localhost / 127.0.0.1, no path, query or credentials.
func NormalizeOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	switch scheme {
	case "https":
	case "http":
		// *.localhost resolves to loopback (RFC 6761): the branded site's
		// development subdomains (acme.localhost:3100).
		if host != "localhost" && host != "127.0.0.1" && !strings.HasSuffix(host, ".localhost") {
			return "", false
		}
	default:
		return "", false
	}
	if host == "" || strings.ContainsAny(host, "*_ ") {
		return "", false
	}
	out := scheme + "://" + host
	if port := u.Port(); port != "" {
		if (scheme == "https" && port != "443") || (scheme == "http" && port != "80") {
			out += ":" + port
		}
	}
	return out, true
}

func newKeySecret(kind string) (string, error) {
	if kind == model.KindPublishable {
		t, err := randomToken(publishableKeyBytes)
		return model.PrefixPublishable + t, err
	}
	t, err := randomToken(secretKeyBytes)
	return model.PrefixSecret + t, err
}

// insertKey stores a new key and returns its secret and uuid.
func insertKey(ctx context.Context, orgID int64, q *db.Queries, f keyFields, rotatedFrom pgtype.Int8) (string, uuid.UUID, error) {
	secret, err := newKeySecret(f.kind)
	if err != nil {
		return "", uuid.Nil, err
	}
	prefixLen := len(model.PrefixSecret) + displayPrefixLetters
	params := db.CreatePublicAPIKeyParams{
		OrganizationID: orgID, Kind: f.kind, Name: f.name, Channel: f.channel, KeyPrefix: secret[:prefixLen],
		KeyLast4: last4(secret), KeyHash: TokenHash(secret), Scopes: f.scopes, AllowedOrigins: f.origins,
		RateLimitPerMinute: int32(f.rateLimit), RotatedFromID: rotatedFrom, CreatedBy: actorArg(ctx),
	}
	if params.AllowedOrigins == nil {
		params.AllowedOrigins = []string{}
	}
	if f.kind == model.KindPublishable {
		params.PublishableKey = pgtype.Text{String: secret, Valid: true}
	}
	row, err := q.CreatePublicAPIKey(ctx, params)
	if err != nil {
		return "", uuid.Nil, err
	}
	return secret, row.Uuid, nil
}

// CreateKey issues a key. The secret is returned once.
func (s *Service) CreateKey(ctx context.Context, orgID int64, in model.KeyInput) (model.IssuedKey, error) {
	f, err := validateKey(in, nil)
	if err != nil {
		return model.IssuedKey{}, err
	}
	// A key for a channel outside the plan would never answer (TEC-674).
	if feat, ok := features.ForSalesChannel(f.channel); ok {
		if err := features.Require(ctx, s.features, orgID, feat); err != nil {
			return model.IssuedKey{}, err
		}
	}
	secret, newID, err := insertKey(ctx, orgID, s.q, f, pgtype.Int8{})
	if err != nil {
		return model.IssuedKey{}, err
	}
	key, err := s.GetKey(ctx, orgID, newID)
	if err != nil {
		return model.IssuedKey{}, err
	}
	s.publish(ctx, model.ResourceKeys, "created", newID)
	return model.IssuedKey{Key: key, Secret: secret}, nil
}

// PatchKey changes the name, scopes, origins or rate limit of an active key.
func (s *Service) PatchKey(ctx context.Context, orgID int64, id uuid.UUID, in model.KeyInput) (model.Key, error) {
	cur, err := s.q.GetPublicAPIKey(ctx, db.GetPublicAPIKeyParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Key{}, ErrNotFound
	}
	if err != nil {
		return model.Key{}, err
	}
	if cur.Status != model.KeyActive {
		return model.Key{}, ErrKeyNotActive
	}
	f, err := validateKey(in, &keyFields{
		kind: cur.Kind, name: cur.Name, channel: cur.Channel, scopes: cur.Scopes, origins: cur.AllowedOrigins,
		rateLimit: int(cur.RateLimitPerMinute),
	})
	if err != nil {
		return model.Key{}, err
	}
	if f.origins == nil {
		f.origins = []string{}
	}
	n, err := s.q.UpdatePublicAPIKey(ctx, db.UpdatePublicAPIKeyParams{
		OrganizationID: orgID, Uuid: id, Name: f.name, Scopes: f.scopes, AllowedOrigins: f.origins,
		RateLimitPerMinute: int32(f.rateLimit),
	})
	if err != nil {
		return model.Key{}, err
	}
	if n == 0 {
		return model.Key{}, ErrKeyNotActive
	}
	s.publish(ctx, model.ResourceKeys, "updated", id)
	return s.GetKey(ctx, orgID, id)
}

// RevokeKey stops a key at once.
func (s *Service) RevokeKey(ctx context.Context, orgID int64, id uuid.UUID) (model.Key, error) {
	if _, err := s.GetKey(ctx, orgID, id); err != nil {
		return model.Key{}, err
	}
	n, err := s.q.RevokePublicAPIKey(ctx, db.RevokePublicAPIKeyParams{
		OrganizationID: orgID, Uuid: id, Now: ts(s.clock()), RevokedBy: actorArg(ctx),
	})
	if err != nil {
		return model.Key{}, err
	}
	if n == 0 {
		return model.Key{}, ErrKeyNotActive
	}
	s.publish(ctx, model.ResourceKeys, "updated", id)
	return s.GetKey(ctx, orgID, id)
}

// RotateKey issues a successor with the same settings. The old key keeps
// working for the grace period (default 24 h, 0 = revoked now).
func (s *Service) RotateKey(ctx context.Context, orgID int64, id uuid.UUID, in model.RotateInput) (model.IssuedKey, error) {
	grace := defaultGraceHours
	if in.GraceHours != nil {
		grace = *in.GraceHours
	}
	if grace < 0 || grace > maxGraceHours {
		return model.IssuedKey{}, validation("grace_hours", "out_of_range", "grace_hours is 0 to 168")
	}
	cur, err := s.q.GetPublicAPIKey(ctx, db.GetPublicAPIKeyParams{OrganizationID: orgID, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.IssuedKey{}, ErrNotFound
	}
	if err != nil {
		return model.IssuedKey{}, err
	}
	if cur.Status != model.KeyActive {
		return model.IssuedKey{}, ErrKeyNotActive
	}
	f := keyFields{
		kind: cur.Kind, name: cur.Name, channel: cur.Channel, scopes: cur.Scopes, origins: cur.AllowedOrigins,
		rateLimit: int(cur.RateLimitPerMinute),
	}
	var secret string
	var newID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		var err error
		secret, newID, err = insertKey(ctx, orgID, q, f, pgtype.Int8{Int64: cur.ID, Valid: true})
		if err != nil {
			return err
		}
		if grace == 0 {
			_, err = q.RevokePublicAPIKey(ctx, db.RevokePublicAPIKeyParams{
				OrganizationID: orgID, Uuid: id, Now: ts(s.clock()), RevokedBy: actorArg(ctx),
			})
			return err
		}
		_, err = q.ExpirePublicAPIKey(ctx, db.ExpirePublicAPIKeyParams{
			OrganizationID: orgID, ID: cur.ID, ExpiresAt: ts(s.clock().Add(time.Duration(grace) * time.Hour)),
		})
		return err
	})
	if err != nil {
		return model.IssuedKey{}, err
	}
	key, err := s.GetKey(ctx, orgID, newID)
	if err != nil {
		return model.IssuedKey{}, err
	}
	s.publish(ctx, model.ResourceKeys, "created", newID)
	s.publish(ctx, model.ResourceKeys, "updated", id)
	return model.IssuedKey{Key: key, Secret: secret}, nil
}

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
