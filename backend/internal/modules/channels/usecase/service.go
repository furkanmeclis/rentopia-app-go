// Package usecase is the channel manager core (TEC-736, RT-A25,
// docs/design/channel-manager.md), shared by every channel adapter
// (Yolcu360, Obilet):
//
//   - outbound: availability, price, stop-sale and quota per mapped product,
//     location and day, pushed as a full snapshot of the horizon (on events
//     such as a sale or a cancellation, and periodically), with a validity
//     and an offline policy so the channel never sells the last known
//     capacity indefinitely;
//   - inbound: booking / modification / cancellation messages pulled from
//     the channel. A message is stored once per account and id (a repeat
//     only counts a delivery); messages of one booking are serialised and
//     reconciled by version: a version at or below the applied one changes
//     nothing, so a repeat never creates a second reservation or payment
//     and an old modification arriving after a cancellation is ignored;
//   - mappings (branch, product, extra ↔ external codes), the error queue
//     (retry / dismiss), the sync log and the reconciliation report.
//
// Reservations are created, changed and cancelled through the Booker port
// (the reservations and customers modules) under a system scope of the
// organization; a booking collected by the channel is never sent to a
// payment and is not accrued on the customer's account.
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PayloadPurpose is the field cipher purpose of sealed message payloads.
const PayloadPurpose = "channels.message"

// lockFor bounds how long one sync of an account may hold it.
const lockFor = 5 * time.Minute

// maxPullPages bounds the pages one pull reads.
const maxPullPages = 20

var (
	// ErrAccountNotFound: unknown channel account (or another
	// organization's).
	ErrAccountNotFound = errors.New("channel account not found")
	// ErrMappingNotFound: unknown mapping.
	ErrMappingNotFound = errors.New("channel mapping not found")
	// ErrMessageNotFound: unknown message.
	ErrMessageNotFound = errors.New("channel message not found")
	// ErrBusy: another sync of the account is running.
	ErrBusy = errors.New("a sync of this channel account is running; try again shortly")
	// ErrAccountInactive: the account is disabled.
	ErrAccountInactive = errors.New("the channel account is disabled")
	// ErrAdapterUnavailable: no adapter for the account's provider in this
	// environment (a development adapter in production).
	ErrAdapterUnavailable = errors.New("the channel adapter is not available in this environment")
	// ErrNotSimulator: the adapter cannot simulate bookings.
	ErrNotSimulator = errors.New("only a development adapter can simulate bookings")
	// ErrNotRetryable: the message is not in the error queue.
	ErrNotRetryable = errors.New("only failed messages can be retried or dismissed")
	// ErrUnsupported: the adapter does not support the operation.
	ErrUnsupported = errors.New("the channel adapter does not support this operation")
	// ErrMappingTaken: the internal record or the external code is mapped
	// already for the account.
	ErrMappingTaken = errors.New("this record or external code is already mapped for the account")
)

// Detail is a field-level validation problem.
type Detail struct{ Field, Code, Message string }

// ValidationError carries field details (400).
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

// AccountSource gives the adapter view of an integration account
// (decrypted credentials), implemented by the integration accounts module.
type AccountSource interface {
	AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error)
}

// Service implements the channel manager.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	sealer    *crypto.Sealer
	accounts  AccountSource
	booker    Booker
	providers map[string]channels.Provider
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries, cipher *crypto.FieldCipher, accounts AccountSource, booker Booker,
	providers ...channels.Provider) (*Service, error) {
	if cipher == nil {
		return nil, errors.New("channels: field cipher required")
	}
	sealer, err := cipher.Sealer(PayloadPurpose)
	if err != nil {
		return nil, err
	}
	s := &Service{
		pool: pool, q: q, sealer: sealer, accounts: accounts, booker: booker, providers: map[string]channels.Provider{},
		events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
	}
	for _, p := range providers {
		if p != nil {
			s.providers[p.Provider()] = p
		}
	}
	return s, nil
}

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

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

// ReservationChanged implements the reservations lifecycle listener: a
// confirmation, a cancellation or a change moves sellable capacity, so
// every channel account of the organization pushes at the next sweep.
func (s *Service) ReservationChanged(ctx context.Context, orgID int64, _ uuid.UUID, _ string) {
	s.requestPush(ctx, orgID)
}

// RequestPush asks every channel account of the organization to push at
// the next sweep (capacity changed outside reservations: a block, a fleet
// change).
func (s *Service) RequestPush(ctx context.Context, orgID int64) { s.requestPush(ctx, orgID) }

func (s *Service) requestPush(ctx context.Context, orgID int64) {
	if err := s.q.RequestChannelPush(context.WithoutCancel(ctx), db.RequestChannelPushParams{OrganizationID: orgID, At: ts(s.clock())}); err != nil {
		s.log.Warn("channel_push_request_failed", "organization_id", orgID, "error", err)
	}
}

func (s *Service) publish(ctx context.Context, resource, action string, ids ...uuid.UUID) {
	if s.events == nil || len(ids) == 0 {
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil {
			out = append(out, id.String())
		}
	}
	if len(out) > 0 {
		s.events.Publish(ctx, resource, action, out...)
	}
}

// systemPermissions are what the channel manager may do on its own (no
// member): create, confirm, change and cancel reservations, create
// customers, read quotes.
var systemPermissions = []string{
	rbac.PermTenantReservationsRead, rbac.PermTenantReservationsWrite, rbac.PermTenantReservationsConfirm,
	rbac.PermTenantReservationsCancel, rbac.PermTenantQuotesRead, rbac.PermTenantQuotesWrite,
	rbac.PermTenantCustomersRead, rbac.PermTenantCustomersWrite, rbac.PermTenantChannelsRead,
	rbac.PermTenantChannelsSync,
}

// systemContext scopes ctx to the organization with every branch and the
// channel manager's own permissions (the worker sweep and the inbound
// processing act for the channel, not for a member).
func systemContext(ctx context.Context, orgID int64) context.Context {
	sc, ok := orgctx.From(ctx)
	if !ok || sc.ID != orgID {
		sc = orgctx.Scope{ID: orgID, Status: orgctx.StatusActive}
	}
	sc.BranchIDs = nil
	sc.Permissions = append(append([]string{}, sc.Permissions...), systemPermissions...)
	return orgctx.WithScope(ctx, sc)
}

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil || t.IsZero() {
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

func int8Of(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func textOr(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// requireAllBranches: channel configuration and sync act for the whole
// organization.
func requireAllBranches(ctx context.Context) error {
	if err := orgctx.RequireAllBranches(ctx); err != nil {
		return err
	}
	return nil
}
