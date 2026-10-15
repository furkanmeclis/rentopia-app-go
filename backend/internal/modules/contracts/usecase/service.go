// Package usecase holds contract templates and documents (TEC-710):
// templates are rich text with {{placeholder}} tokens, one active version
// per kind; a document of a reservation (and its rental) is rendered from
// the active template and the data of the moment into an immutable version
// (HTML, SHA-256 content hash, data snapshot, PDF). Any change of the data a
// template uses renders a new version and supersedes the previous one; an
// approval is bound to the exact content hash, so a new version always
// needs a new approval (RT-A18). docs/design/contracts.md is the design.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTemplateNotFound = errors.New("contract template not found")
	ErrDocumentNotFound = errors.New("document not found")
	ErrVersionNotFound  = errors.New("document version not found")
	// ErrForbidden: the action needs a permission the caller lacks.
	ErrForbidden = errors.New("missing permission")
	// ErrNoTemplate: no active template of the kind (409).
	ErrNoTemplate = errors.New("no active template of this kind")
	// ErrPDFNotReady: the PDF is rendering or failed (409).
	ErrPDFNotReady = errors.New("the PDF is not ready yet")
	// ErrStorageUnavailable: no object storage configured.
	ErrStorageUnavailable = errors.New("storage is not configured")
)

// StateError is answered with 409 INVALID_STATE.
type StateError struct {
	Code    string
	Message string
}

func (e *StateError) Error() string { return e.Message }

func stateErr(code, msg string) error { return &StateError{Code: code, Message: msg} }

// Detail is one field-level validation problem.
type Detail struct {
	Field, Code, Message string
}

// ValidationError carries field details (400).
type ValidationError struct {
	Details []Detail
}

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

// Settings reads the organization settings (company identity).
type Settings interface {
	Current(ctx context.Context, orgID int64) (osmodel.Version, error)
}

// PDFQueue queues the render of a version (the worker renders it); nil =
// render in the request.
type PDFQueue interface {
	EnqueueDocumentPDF(orgID, versionID int64) error
}

// Store is the object storage contracts use (PDFs).
type Store interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// Service implements templates and documents.
type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	settings Settings
	storage  Store
	renderer pdfrender.Renderer
	queue    PDFQueue
	events   resourceevents.Publisher
	log      *slog.Logger
	now      func() time.Time
	// remote is remote approval (TEC-711); nil = not wired here.
	remote *remote
}

// New builds the service. renderer / queue may be nil.
func New(pool *pgxpool.Pool, q *db.Queries, settings Settings, store Store) *Service {
	s := &Service{
		pool: pool, q: q, settings: settings, events: resourceevents.Nop{},
		log: slog.Default(), now: time.Now,
	}
	// A nil driver must not become a non-nil interface.
	if store != nil && !isNilStore(store) {
		s.storage = store
	}
	return s
}

func isNilStore(st Store) bool {
	switch v := st.(type) {
	case storage.Driver:
		return v == nil
	}
	return false
}

// SetRenderer wires the PDF renderer (headless Chromium).
func (s *Service) SetRenderer(r pdfrender.Renderer) { s.renderer = r }

// SetQueue makes PDF renders asynchronous (worker).
func (s *Service) SetQueue(q PDFQueue) { s.queue = q }

// SetResourceEvents publishes live list changes.
func (s *Service) SetResourceEvents(pub resourceevents.Publisher) {
	if pub != nil {
		s.events = pub
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

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("contracts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) publish(ctx context.Context, resource, action string, ids ...uuid.UUID) {
	if s.events == nil || len(ids) == 0 {
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	s.events.Publish(ctx, resource, action, out...)
}

func actorID(ctx context.Context) *int64 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		id := p.UserInternal
		return &id
	}
	return nil
}

func has(ctx context.Context, slug string) bool {
	sc, ok := orgctx.From(ctx)
	return ok && sc.Has(slug)
}

func inScope(ctx context.Context, ids ...int64) bool {
	for _, id := range ids {
		if id != 0 && orgctx.CheckBranch(ctx, id) == nil {
			return true
		}
	}
	return false
}

func int8Ptr(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t.UTC(), Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func uniqueViolation(err error, constraint string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505" && (constraint == "" || pe.ConstraintName == constraint)
}

func textOf(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }
