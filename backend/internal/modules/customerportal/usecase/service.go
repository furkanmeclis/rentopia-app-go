// Package usecase is the customer portal (TEC-715,
// docs/design/customer-portal.md).
//
//   - Sign-in: a reservation reference plus a phone number or e-mail
//     address on file (the renter or a driver of that reservation). The
//     contact is compared through keyed hashes; the answer never tells
//     whether the reference exists or which half was wrong. A one-time code
//     (purpose customer_portal, subject reservation:<uuid>) goes out by SMS
//     through the organization's SMS account or by e-mail through the
//     platform mailer.
//   - Session: an opaque bearer token (stored as SHA-256 only) scoped to
//     the organization and the customer (or the driver), with a sliding idle
//     expiry inside a hard cap. It is never a staff session.
//   - Views and actions only reach the session's own reservations:
//     documents (streamed by the API), pay remaining (a pay-by-link link),
//     change / cancel requests that staff resolve.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	contractsmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	contractsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrPortalNotFound: no live organization with this slug.
	ErrPortalNotFound = errors.New("portal not found")
	// ErrChannelUnavailable: the organization cannot send codes on the
	// channel the customer chose (no SMS account / no mailer).
	ErrChannelUnavailable = errors.New("sign-in codes cannot be sent right now; contact the office")
	// ErrCodeInvalid: the one generic answer of a failed verification
	// (unknown reference, other contact, wrong / expired / locked code).
	ErrCodeInvalid = errors.New("the details or the code could not be verified")
	// ErrSessionInvalid: no valid customer session for this organization.
	ErrSessionInvalid = errors.New("customer session required")
	// ErrNotFound: the record is not one of the session's.
	ErrNotFound = errors.New("not found")
	// ErrPaymentUnavailable: online payment is not set up.
	ErrPaymentUnavailable = errors.New("online payment is not available; contact the office")
	// ErrNothingDue: nothing is left to pay online.
	ErrNothingDue = errors.New("nothing is due")
	// ErrRequestExists: an open request of the same type exists.
	ErrRequestExists = errors.New("an open request of this type already exists")
	// ErrPDFNotReady: the PDF is being prepared.
	ErrPDFNotReady = errors.New("the PDF is not ready yet")
	// ErrRequestNotFound: unknown request (staff; another organization's).
	ErrRequestNotFound = errors.New("customer request not found")
)

// Detail is a field problem.
type Detail struct{ Field, Code, Message string }

// ValidationError answers 400.
type ValidationError struct{ Details []Detail }

func (e *ValidationError) Error() string {
	if len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// StateError answers 409 INVALID_STATE.
type StateError struct{ Code, Message string }

func (e *StateError) Error() string { return e.Message }

func stateErr(code, msg string) error { return &StateError{Code: code, Message: msg} }

// OTP is the one-time code service (smsusecase.Service).
type OTP interface {
	IssueOTP(ctx context.Context, orgID int64, purpose, subjectRef, phone string) (smsusecase.Challenge, error)
	IssueEmailOTP(ctx context.Context, orgID int64, purpose, subjectRef, email string, msg smsusecase.EmailOTP) (smsusecase.Challenge, error)
	VerifyOTP(ctx context.Context, orgID int64, purpose, subjectRef, code string) (smsusecase.Verification, error)
	PhoneHash(orgID int64, normalized string) string
	EmailHash(orgID int64, normalized string) string
	HasActiveAccount(ctx context.Context, orgID int64) bool
	EmailAvailable() bool
}

// Reservations is the reservations port.
type Reservations interface {
	Get(ctx context.Context, orgID int64, id uuid.UUID) (resmodel.Reservation, error)
	Cancel(ctx context.Context, orgID int64, id uuid.UUID, reason string) (resmodel.Reservation, error)
}

// Payments is the payments port (pay-by-link).
type Payments interface {
	CreateLink(ctx context.Context, orgID int64, reservationUUID uuid.UUID, in paymodel.CreateLinkInput) (paymodel.Link, error)
	LinkURL(token string) string
}

// Accounts tells whether an organization has an active payment account.
type Accounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
}

// Documents is the contracts port.
type Documents interface {
	ListDocuments(ctx context.Context, orgID int64, f contractsmodel.DocumentFilter) ([]contractsmodel.DocumentListItem, int64, error)
	GetDocument(ctx context.Context, orgID int64, id uuid.UUID) (contractsmodel.Document, error)
	ListApprovalLinks(ctx context.Context, orgID int64, docID uuid.UUID) ([]contractsmodel.ApprovalLink, error)
	OpenPDF(ctx context.Context, orgID int64, docID, id uuid.UUID) (contractsusecase.PDFFile, error)
}

// Limits are the session and code timings.
type Limits struct {
	// SessionIdle is the sliding idle expiry; SessionMax the hard cap from
	// sign-in.
	SessionIdle time.Duration
	SessionMax  time.Duration
	// TouchEvery throttles the sliding update.
	TouchEvery time.Duration
}

// DefaultLimits are the production limits: 30 minutes idle inside 2 hours.
func DefaultLimits() Limits {
	return Limits{SessionIdle: 30 * time.Minute, SessionMax: 2 * time.Hour, TouchEvery: time.Minute}
}

// Service implements the portal.
type Service struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	otp          OTP
	reservations Reservations
	payments     Payments
	accounts     Accounts
	documents    Documents
	activity     *activity.Recorder
	events       resourceevents.Publisher
	log          *slog.Logger
	limits       Limits
	now          func() time.Time
}

// New builds the service. payments / accounts / documents may be nil (the
// matching views and actions are then unavailable).
func New(pool *pgxpool.Pool, q *db.Queries, otp OTP, reservations Reservations) *Service {
	return &Service{
		pool: pool, q: q, otp: otp, reservations: reservations, events: resourceevents.Nop{},
		log: slog.Default(), limits: DefaultLimits(), now: time.Now,
	}
}

// SetPayments wires pay remaining.
func (s *Service) SetPayments(p Payments, a Accounts) { s.payments, s.accounts = p, a }

// SetDocuments wires the documents view and downloads.
func (s *Service) SetDocuments(d Documents) { s.documents = d }

// SetActivity records sign-in and request events.
func (s *Service) SetActivity(r *activity.Recorder) { s.activity = r }

// SetResourceEvents publishes live list changes (customer requests).
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

// SetLimits replaces the limits (tests).
func (s *Service) SetLimits(l Limits) { s.limits = l }

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("customerportal: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// orgContext scopes a context to the organization (every branch): the
// customer is not a member, the session decides what is visible.
func orgContext(ctx context.Context, org db.GetPortalOrganizationRow) context.Context {
	return orgctx.WithScope(ctx, orgctx.Scope{ID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name, Status: org.Status})
}

// organization loads a live organization by slug (suspended or outside its
// access window: not found).
func (s *Service) organization(ctx context.Context, slug string) (db.GetPortalOrganizationRow, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || len(slug) > 64 {
		return db.GetPortalOrganizationRow{}, ErrPortalNotFound
	}
	org, err := s.q.GetPortalOrganization(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, ErrPortalNotFound
	}
	if err != nil {
		return org, err
	}
	now := s.clock()
	if org.Status != orgctx.StatusActive || (org.AccessStartsAt.Valid && now.Before(org.AccessStartsAt.Time)) ||
		(org.AccessEndsAt.Valid && !now.Before(org.AccessEndsAt.Time)) {
		return org, ErrPortalNotFound
	}
	return org, nil
}

// PortalInfo is the public header of the sign-in page.
type PortalInfo struct {
	Organization model.Organization `json:"organization"`
	Channels     struct {
		SMS   bool `json:"sms"`
		Email bool `json:"email"`
	} `json:"channels"`
}

// Info answers GET /v1/customer/{orgSlug}: the organization name and the
// channels it can send codes on.
func (s *Service) Info(ctx context.Context, slug string) (PortalInfo, error) {
	org, err := s.organization(ctx, slug)
	if err != nil {
		return PortalInfo{}, err
	}
	out := PortalInfo{Organization: model.Organization{Slug: org.Slug, Name: org.Name}}
	out.Channels.SMS = s.otp.HasActiveAccount(ctx, org.ID)
	out.Channels.Email = s.otp.EmailAvailable()
	return out, nil
}

// ------------------------------------------------------------- helpers

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// TokenHash is the stored form of a session token.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func uniqueViolation(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && (constraint == "" || pe.ConstraintName == constraint)
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// shortName keeps the first name(s) and the initial of the last name.
func shortName(full string) string {
	parts := strings.Fields(full)
	if len(parts) < 2 {
		return strings.TrimSpace(full)
	}
	last := []rune(parts[len(parts)-1])
	return strings.Join(parts[:len(parts)-1], " ") + " " + string(last[0]) + "."
}

func displayName(customerType, first, last, company string) string {
	if customerType == "corporate" && strings.TrimSpace(company) != "" {
		return strings.TrimSpace(company)
	}
	return shortName(strings.TrimSpace(first + " " + last))
}

func requestFor(c model.ClientInfo) *http.Request {
	r := &http.Request{Header: http.Header{}, RemoteAddr: net.JoinHostPort(c.IP, "0")}
	if c.UserAgent != "" {
		r.Header.Set("User-Agent", c.UserAgent)
	}
	return r
}

// audit records a portal event (no actor: the customer is not a user).
func (s *Service) audit(ctx context.Context, action, resource string, id *uuid.UUID, payload map[string]any, c *model.ClientInfo) {
	if s.activity == nil {
		return
	}
	var req *http.Request
	if c != nil {
		req = requestFor(*c)
	}
	s.activity.Record(ctx, nil, action, resource, id, payload, req)
}

func (s *Service) publish(ctx context.Context, resource, action string, ids ...uuid.UUID) {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	s.events.Publish(ctx, resource, action, out...)
}

// clip cuts v to at most n bytes (a rune cut in half is dropped).
func clip(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return strings.ToValidUTF8(v[:n], "")
}

