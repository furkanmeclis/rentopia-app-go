// Package usecase implements e-invoices / e-archive invoices (TEC-722,
// RT-A22, docs/design/einvoice.md).
//
// An invoice is a document on top of the ledger: a draft is cut from the
// live accruals of a rental or an account (rent, extras, one-way fee,
// charges, tolls) and LINKS them through ledger_entry_links. It never posts
// a debt or revenue line (RT-A22 + RT-A23); the database refuses a second
// live link of the same ledger item, so an item is invoiced at most once
// until its invoice is cancelled or returned.
//
//	draft ──finalize──▶ queued ──runner──▶ sent / awaiting_result ──▶ succeeded
//	  │  (number, go-ubltr XML,               │                      │
//	  │   XSD + Schematron)                   └──▶ failed            ├─ e-Arşiv cancel ─▶ cancelled
//	  └──cancel──▶ cancelled                                         └─ IADE (return) succeeded ─▶ returned
//
// Sending is an integration job (docs/design/integration-jobs.md): the UUID
// (ETTN) exists from the draft on and is sent unchanged on every attempt; an
// unknown outcome is checked with Status first and only a confirmed "not
// there" sends again. A send failure never touches the ledger.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/ubl"
	osmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReceiverPurpose is the field cipher purpose of sealed receiver identity
// numbers.
const ReceiverPurpose = "invoices.receiver"

// TaxpayerIndexPurpose is the blind index purpose of the taxpayer cache.
const TaxpayerIndexPurpose = "invoices.taxpayer"

// MaxAttempts bounds the automatic provider calls of a send job.
const MaxAttempts = 8

// MaxListTargets bounds lists used by engines (export, bulk).
const MaxListTargets = 5000

var (
	// ErrNotFound: unknown invoice (another organization's, or outside the
	// branch scope).
	ErrNotFound = errors.New("invoice not found")
	// ErrNotDraft: the invoice is no longer a draft.
	ErrNotDraft = errors.New("the invoice is not a draft")
	// ErrNothingToInvoice: no invoiceable item (all invoiced already, or
	// none accrued).
	ErrNothingToInvoice = errors.New("there is nothing to invoice")
	// ErrAlreadyInvoiced: a standing invoice already covers an item.
	ErrAlreadyInvoiced = errors.New("an item is already invoiced")
	// ErrNotRetryable: the send cannot be retried now.
	ErrNotRetryable = errors.New("the invoice cannot be retried in its current state")
	// ErrNotCancellable: the invoice cannot be cancelled now.
	ErrNotCancellable = errors.New("the invoice cannot be cancelled in its current state")
	// ErrUseReturn: an issued e-Fatura is undone with a return invoice.
	ErrUseReturn = errors.New("an issued e-Fatura cannot be cancelled; issue a return (IADE) invoice")
	// ErrNotReturnable: only an issued sales invoice can be returned (once).
	ErrNotReturnable = errors.New("the invoice cannot be returned")
	// ErrBusy: a worker holds the send right now.
	ErrBusy = errors.New("the invoice is being sent; try again shortly")
	// ErrNoDocument: the invoice has no XML (still a draft).
	ErrNoDocument = errors.New("the invoice has no document yet")
	// ErrStorageUnavailable: no object storage is configured.
	ErrStorageUnavailable = errors.New("object storage is not configured")
	// ErrPDFUnavailable: the PDF could not be rendered (no Chromium).
	ErrPDFUnavailable = errors.New("the PDF renderer is not available")
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

func (e *ValidationError) err() error {
	if e == nil || len(e.Details) == 0 {
		return nil
	}
	return e
}

// DocumentError: the document does not pass the module checks, the XSD or
// the Schematron; nothing was numbered or sent (422). Problems name the
// form fields.
type DocumentError struct{ Problems []ubl.Problem }

func (e *DocumentError) Error() string {
	if e == nil || len(e.Problems) == 0 {
		return "the invoice document is not valid"
	}
	return "the invoice document is not valid: " + e.Problems[0].Field + ": " + e.Problems[0].Message
}

// DependencyError: no working integrator; nothing was sent (409).
type DependencyError struct{ Reason string }

func (e *DependencyError) Error() string {
	return "waiting on dependency (" + e.Reason + "): no working e-invoice integrator; nothing was sent"
}

// Store is the private object storage of documents.
type Store interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// CompanySettings returns the organization's current settings (company:
// legal name, tax number / office, address).
type CompanySettings interface {
	Current(ctx context.Context, orgID int64) (osmodel.Version, error)
}

// Service implements invoices.
type Service struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	cipher    *crypto.FieldCipher
	sealer    *crypto.Sealer
	runner    *jobsusecase.Runner
	accounts  AccountSource
	providers map[string]einvoice.Provider
	settings  CompanySettings
	store     Store
	pdf       pdfrender.Renderer
	events    resourceevents.Publisher
	log       *slog.Logger
	now       func() time.Time
}

// AccountSource opens an organization's integration account by id (the
// integration accounts service).
type AccountSource interface {
	AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error)
}

// New builds the service, registers the runner operations of every
// provider that can send (the fake outside production; the official slot
// waits for TEC-685) and listens to the runner's state changes.
func New(pool *pgxpool.Pool, q *db.Queries, cipher *crypto.FieldCipher, settings CompanySettings, accounts AccountSource,
	runner *jobsusecase.Runner, ops *integrations.OperationRegistry, providers ...einvoice.Provider) (*Service, error) {
	if cipher == nil {
		return nil, errors.New("invoices: field cipher required")
	}
	sealer, err := cipher.Sealer(ReceiverPurpose)
	if err != nil {
		return nil, err
	}
	s := &Service{
		pool: pool, q: q, cipher: cipher, sealer: sealer, runner: runner, accounts: accounts, settings: settings,
		providers: map[string]einvoice.Provider{}, events: resourceevents.Nop{}, log: slog.Default(), now: time.Now,
	}
	for _, p := range providers {
		if p == nil {
			continue
		}
		s.providers[p.Provider()] = p
		if p.Ready() != nil {
			continue
		}
		ops.Register(p.Provider(), &sendOperation{svc: s, provider: p})
		ops.Register(p.Provider(), &cancelOperation{svc: s, provider: p})
	}
	if runner != nil {
		runner.AddListener(s)
	}
	return s, nil
}

// SetStore wires the private object storage (XML / PDF).
func (s *Service) SetStore(st Store) {
	if st != nil {
		if d, ok := st.(storage.Driver); ok && d == nil {
			return
		}
		s.store = st
	}
}

// SetPDFRenderer wires the PDF renderer (headless Chromium).
func (s *Service) SetPDFRenderer(r pdfrender.Renderer) { s.pdf = r }

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

// istanbul is the display / issue-date zone.
var istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*3600)
}()

func (s *Service) today() string { return s.now().In(istanbul).Format(time.DateOnly) }

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func actorID(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func orgKey(orgID int64) string { return "org:" + strconv.FormatInt(orgID, 10) }

func (s *Service) publish(ctx context.Context, action string, ids ...string) {
	if s.events == nil || len(ids) == 0 {
		return
	}
	s.events.Publish(ctx, model.Resource, action, ids...)
}

func (s *Service) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("invoices: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// receiverAAD binds a sealed receiver identity to its invoice.
func receiverAAD(orgID int64, id uuid.UUID) string {
	return "invoices.receiver|" + orgKey(orgID) + "|invoice:" + id.String()
}

func (s *Service) sealIdentity(orgID int64, id uuid.UUID, v string) (string, error) {
	if v == "" {
		return "", nil
	}
	return s.sealer.Seal(v, receiverAAD(orgID, id))
}

func (s *Service) openIdentity(orgID int64, id uuid.UUID, enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	v, err := s.sealer.Open(enc, receiverAAD(orgID, id))
	if err != nil {
		return "", fmt.Errorf("invoices: open receiver identity: %w", err)
	}
	return v, nil
}
