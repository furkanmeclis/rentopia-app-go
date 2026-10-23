// Package usecase implements the public booking API (TEC-728,
// docs/design/public-api.md): organization-resolved credentials (secret
// keys for servers, publishable keys that mint short-lived booking
// sessions for allowed origins), idempotent writes, the public catalog,
// quotes and reservations through the same services the panel uses, the
// customer's status / request / payment actions through the customer
// portal's use cases, and signed reservation webhooks.
//
// Every method takes the organization from a verified credential (or the
// panel's orgctx); another organization's records read as not found.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	portalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/usecase"
	customerusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	resusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrUnauthenticated: no valid credential (401).
	ErrUnauthenticated = errors.New("a valid API key or booking session is required")
	// ErrOriginNotAllowed: the browser origin is not allowed for the key or
	// differs from the session's (403).
	ErrOriginNotAllowed = errors.New("this origin is not allowed to use the key")
	// ErrSecretKeyInBrowser: a secret key was sent from a browser (403).
	ErrSecretKeyInBrowser = errors.New("secret keys must never be used from a browser; use a publishable key")
	// ErrScope: the key lacks the scope of the route (403).
	ErrScope = errors.New("the key does not have the scope this action needs")
	// ErrChannelDisabled: the key's sales channel is switched off (403).
	ErrChannelDisabled = errors.New("the sales channel of this key is not active")
	// ErrOrganizationUnavailable: suspended / outside its access window (403).
	ErrOrganizationUnavailable = errors.New("the organization cannot take bookings right now")
	// ErrNotFound: no such record for this organization (404).
	ErrNotFound = errors.New("not found")
	// ErrReservationToken: the reservation access token is missing or wrong
	// (404, the reservation's existence is not revealed).
	ErrReservationToken = errors.New("reservation not found")
	// ErrIdempotencyKeyRequired: a write without Idempotency-Key (400).
	ErrIdempotencyKeyRequired = errors.New("the Idempotency-Key header is required for this request")
	// ErrIdempotencyInProgress: the same key is still being processed (409).
	ErrIdempotencyInProgress = errors.New("a request with this Idempotency-Key is still being processed")
	// ErrIdempotencyMismatch: the key was used with another request (422).
	ErrIdempotencyMismatch = errors.New("this Idempotency-Key was used with a different request")
	// ErrKeyNotActive: the key is revoked (409 on panel actions).
	ErrKeyNotActive = errors.New("the key is revoked")
	// ErrWebhookTarget: the endpoint URL is not allowed (400 on url).
	ErrWebhookTarget = errors.New("the webhook URL must be a public https address")
)

// Detail is a field problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400.
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

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// QuoteReplacedError wraps a QUOTE_PRICE_CHANGED answer whose replacement
// quote was re-issued through the public API.
type QuoteReplacedError struct {
	Err         *resusecase.QuoteError
	Replacement any
}

func (e *QuoteReplacedError) Error() string { return e.Err.Message }

// SettingsReader is the organization settings port (orgsettings).
type SettingsReader interface {
	Current(ctx context.Context, orgID int64) (osmodel.Version, error)
}

// Limits are the tunables of the public API.
type Limits struct {
	// SessionTTL is the lifetime of a booking session token.
	SessionTTL time.Duration
	// IdempotencyTTL keeps the answer of a write.
	IdempotencyTTL time.Duration
	// IdempotencyStale lets a write that never finished be retried.
	IdempotencyStale time.Duration
	// WebhookTimeout bounds one delivery attempt.
	WebhookTimeout time.Duration
	// WebhookMaxAttempts before a delivery is failed.
	WebhookMaxAttempts int
}

// DefaultLimits are the production values.
func DefaultLimits() Limits {
	return Limits{
		SessionTTL: 30 * time.Minute, IdempotencyTTL: 24 * time.Hour, IdempotencyStale: 2 * time.Minute,
		WebhookTimeout: 10 * time.Second, WebhookMaxAttempts: 8,
	}
}

// Service is the public booking API.
type Service struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	cipher       *crypto.FieldCipher
	secrets      *crypto.Sealer
	bodies       *crypto.Sealer
	settings     SettingsReader
	reservations *resusecase.Service
	customers    *customerusecase.Service
	portal       *portalusecase.Service
	events       resourceevents.Publisher
	activity     *activity.Recorder
	log          *slog.Logger
	now          func() time.Time
	limits       Limits
	client       *http.Client
	// allowPrivate lets webhooks target private / loopback addresses and
	// plain http (development and tests only).
	allowPrivate bool
	immediate    bool
	baseURL      string
	// features gates keys by the organization's plan (TEC-674): a key
	// sells as a channel whose feature (web = site, widget, api =
	// public_api) must be enabled.
	features features.AccessPolicy
}

// New builds the service. cipher signs session tokens and seals webhook
// secrets and stored answers (purpose-separated keys).
func New(pool *pgxpool.Pool, q *db.Queries, cipher *crypto.FieldCipher, settings SettingsReader,
	reservations *resusecase.Service, customers *customerusecase.Service, portal *portalusecase.Service) (*Service, error) {
	if cipher == nil {
		return nil, errors.New("publicapi: field cipher is required")
	}
	secrets, err := cipher.Sealer("public_api_webhooks")
	if err != nil {
		return nil, err
	}
	bodies, err := cipher.Sealer("public_api_idempotency")
	if err != nil {
		return nil, err
	}
	s := &Service{
		pool: pool, q: q, cipher: cipher, secrets: secrets, bodies: bodies, settings: settings,
		reservations: reservations, customers: customers, portal: portal,
		events: resourceevents.Nop{}, log: slog.Default(), now: time.Now, limits: DefaultLimits(),
	}
	s.client = &http.Client{Timeout: s.limits.WebhookTimeout, Transport: s.webhookTransport(), CheckRedirect: noRedirect}
	return s, nil
}

// SetFeatures gates keys by the organization's plan (nil: no gate).
func (s *Service) SetFeatures(p features.AccessPolicy) { s.features = p }

// SetActivity records panel actions.
func (s *Service) SetActivity(r *activity.Recorder) { s.activity = r }

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(p resourceevents.Publisher) {
	if p != nil {
		s.events = p
	}
}

// SetLogger replaces the logger.
func (s *Service) SetLogger(l *slog.Logger) {
	if l != nil {
		s.log = l
	}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// SetLimits replaces the tunables (tests).
func (s *Service) SetLimits(l Limits) {
	s.limits = l
	s.client.Timeout = l.WebhookTimeout
}

// AllowPrivateWebhooks lets webhooks reach private addresses over http
// (development and tests; never production).
func (s *Service) AllowPrivateWebhooks(allow bool) {
	s.allowPrivate = allow
	s.client.Transport = s.webhookTransport()
}

// SetBaseURL names the API base integrators call (shown in the panel).
func (s *Service) SetBaseURL(u string) { s.baseURL = strings.TrimRight(strings.TrimSpace(u), "/") }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func (s *Service) publish(ctx context.Context, resource, action string, id uuid.UUID) {
	s.events.Publish(ctx, resource, action, id.String())
}

// orgContext scopes a context to the credential's organization (every
// branch): the public caller is not a member.
func orgContext(ctx context.Context, id int64, orgUUID uuid.UUID, slug, name, status string) context.Context {
	return orgctx.WithScope(ctx, orgctx.Scope{ID: id, UUID: orgUUID, Slug: slug, Name: name, Status: status})
}

// ------------------------------------------------------------- helpers

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("publicapi: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// TokenHash is the stored SHA-256 hex of a key or token.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func int32ToPg(v int) pgtype.Int4 { return pgtype.Int4{Int32: int32(v), Valid: true} }

func last4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[len(s)-4:]
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func nilIfEmpty(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return v
}

func textArg(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	return pgtype.Text{String: v, Valid: v != ""}
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
