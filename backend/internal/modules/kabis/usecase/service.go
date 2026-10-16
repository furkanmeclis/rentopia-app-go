// Package usecase holds KABİS rental notifications (TEC-726, RT-A21,
// docs/design/kabis.md).
//
// A notification is created only from an ACTUAL event, inside the rental's
// own transaction: the handover (rental_start), the return (rental_end),
// and a correction when the person / vehicle data of a reported start
// changes (drivers, vehicle swap). A reservation is never reported. Each
// notification has one integration job (the generic runner,
// docs/design/integration-jobs.md):
//
//	blocked            required data is missing (missing_fields says which)
//	waiting_dependency no working adapter (no account; the official method
//	                   is not verified yet, TEC-682)
//	pending → submitted / succeeded / failed / needs_verification (runner)
//	reported_manually / reconciled (a person, with a reason and reference)
//
// An unknown outcome is never sent again blindly: the runner checks the
// status first (provider lookup by our reference) and only a confirmed
// "not filed" sends again. Only succeeded / reported_manually /
// reconciled mean the authority has the record; a pending row is a local
// record and is never shown as an official notification.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PayloadPurpose is the field cipher purpose of sealed payloads.
const PayloadPurpose = "kabis.payload"

// MaxAttempts bounds the automatic provider calls of a notification job.
const MaxAttempts = 8

// MaxAttachmentBytes caps a resolution attachment.
const MaxAttachmentBytes = 10 << 20

var (
	// ErrNotFound: unknown notification (another organization's, or outside
	// the branch scope).
	ErrNotFound = errors.New("KABİS notification not found")
	// ErrNotRetryable: the notification is final.
	ErrNotRetryable = errors.New("the notification cannot be retried in its current state")
	// ErrNotResolvable: the resolution does not apply to the current state.
	ErrNotResolvable = errors.New("the notification cannot be marked in its current state")
	// ErrBusy: a worker holds the job right now.
	ErrBusy = errors.New("the notification is being processed; try again shortly")
	// ErrNoAttachment: the notification has no attachment.
	ErrNoAttachment = errors.New("the notification has no attachment")
	// ErrStorageUnavailable: no object storage is configured.
	ErrStorageUnavailable = errors.New("object storage is not configured")
)

// DependencyError: no working adapter; nothing was sent (409).
type DependencyError struct{ Reason string }

func (e *DependencyError) Error() string {
	return "waiting on dependency (" + e.Reason + "): no working KABİS adapter; nothing was sent"
}

// DataMissingError: required data is missing; nothing was sent (409).
type DataMissingError struct{ Fields []string }

func (e *DataMissingError) Error() string {
	return fmt.Sprintf("required data is missing: %v", e.Fields)
}

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

// Store is the private object storage of attachments.
type Store interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// Service implements KABİS notifications.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	sealer    *crypto.Sealer
	cipher    *crypto.FieldCipher
	runner    *jobsusecase.Runner
	providers map[string]kabis.Provider
	store     Store
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
}

// New builds the service, registers the runner operations of every
// provider that can send (the fake outside production; the official slot
// cannot send until TEC-682) and listens to the runner's state changes.
func New(pool *pgxpool.Pool, q *db.Queries, cipher *crypto.FieldCipher, runner *jobsusecase.Runner,
	ops *integrations.OperationRegistry, providers ...kabis.Provider) (*Service, error) {
	if cipher == nil {
		return nil, errors.New("kabis: field cipher required")
	}
	sealer, err := cipher.Sealer(PayloadPurpose)
	if err != nil {
		return nil, err
	}
	s := &Service{
		pool: pool, q: q, sealer: sealer, cipher: cipher, runner: runner, providers: map[string]kabis.Provider{},
		events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		s.providers[p.Provider()] = p
		if p.Ready() != nil {
			continue
		}
		for _, ev := range model.Events {
			ops.Register(p.Provider(), &operation{svc: s, provider: p, kind: model.KindOf(ev)})
		}
	}
	if runner != nil {
		runner.AddListener(s)
	}
	return s, nil
}

// SetStore wires the private object storage (attachments).
func (s *Service) SetStore(st Store) {
	if st != nil {
		if d, ok := st.(storage.Driver); ok && d == nil {
			return
		}
		s.store = st
	}
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

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func (s *Service) publish(ctx context.Context, action string, ids ...string) {
	if s.events == nil || len(ids) == 0 {
		return
	}
	s.events.Publish(ctx, model.Resource, action, ids...)
}
