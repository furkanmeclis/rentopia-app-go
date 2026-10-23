package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

// PDFStaleAfter is how long a requested render may stay pending before it
// is requested again: its queue task was lost (a restore with an older
// Redis, a worker that died mid-task). Longer than a render with all its
// queue retries (2 min timeout, 3 retries).
const PDFStaleAfter = 15 * time.Minute

// pdfSweepLimit caps one sweep run.
const pdfSweepLimit = 200

// TaskEnqueuer is the Asynq client (queue.Client). With it a render is
// enqueued under a task id derived from the version uuid (PDFTaskID), so
// enqueuing the same render while its task still exists is refused by
// Asynq: re-enqueuing after a restore never doubles a render.
type TaskEnqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// PDFTaskID is the Asynq task id of a version's render.
func PDFTaskID(version uuid.UUID) string { return "documents:render-pdf:" + version.String() }

// errPDFQueued: the render task already exists in the queue.
var errPDFQueued = errors.New("contracts: the render is already queued")

// enqueuePDF queues the render of a version (task id from its uuid).
func (s *Service) enqueuePDF(orgID, versionID int64, version uuid.UUID) error {
	te, ok := s.queue.(TaskEnqueuer)
	if !ok {
		return s.queue.EnqueueDocumentPDF(orgID, versionID)
	}
	task, err := queue.NewDocumentPDFTask(orgID, versionID)
	if err != nil {
		return err
	}
	_, err = te.Enqueue(task, asynq.Queue(queue.QueueExports), asynq.MaxRetry(3), asynq.Timeout(2*time.Minute),
		asynq.TaskID(PDFTaskID(version)))
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return errPDFQueued
	}
	return err
}

// markPDFPending requests a render: from none / failed, or again from a
// pending request older than PDFStaleAfter. ok is false when the version
// is ready, pending recently, or another caller won the request.
func (s *Service) markPDFPending(ctx context.Context, orgID, versionID int64) (uuid.UUID, bool) {
	now := s.clock()
	id, err := s.q.SetDocumentVersionPDFPending(ctx, db.SetDocumentVersionPDFPendingParams{
		RequestedAt: ts(now), OrganizationID: orgID, ID: versionID, StaleBefore: ts(now.Add(-PDFStaleAfter)),
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.Warn("document_pdf_request_failed", "version_id", versionID, "error", err)
		}
		return uuid.Nil, false
	}
	return id, true
}

// requestPDF marks a version's PDF as pending and renders it: in the
// worker when a queue is wired, else in the request (best effort; a failure
// is recorded on the version and retried by the next download).
func (s *Service) requestPDF(ctx context.Context, orgID, versionID int64) {
	version, ok := s.markPDFPending(ctx, orgID, versionID)
	if !ok {
		return
	}
	if s.queue != nil {
		if err := s.enqueuePDF(orgID, versionID, version); err != nil && !errors.Is(err, errPDFQueued) {
			s.log.Warn("document_pdf_enqueue_failed", "version_id", versionID, "error", err)
			s.failPDF(ctx, orgID, versionID, "queue: "+err.Error())
		}
		return
	}
	if err := s.RenderPDF(ctx, orgID, versionID); err != nil {
		s.log.Warn("document_pdf_render_failed", "version_id", versionID, "error", err)
	}
}

// SweepPendingPDFs requests again every render pending for longer than
// PDFStaleAfter, across organizations (worker: at start and every minute;
// TEC-962). The version is re-stamped first (one sweep wins), then its
// task is enqueued under the version's task id: a task that still exists
// is left alone, a lost one is queued once. Without a queue the render
// runs here. Returns the number of renders queued (or run).
func (s *Service) SweepPendingPDFs(ctx context.Context) (int, error) {
	rows, err := s.q.ListStalePendingDocumentPDFs(ctx, db.ListStalePendingDocumentPDFsParams{
		StaleBefore: ts(s.clock().Add(-PDFStaleAfter)), LimitCount: pdfSweepLimit,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if ctx.Err() != nil {
			break
		}
		octx := orgctx.WithOrganizationID(ctx, r.OrganizationID)
		version, ok := s.markPDFPending(octx, r.OrganizationID, r.ID)
		if !ok {
			continue
		}
		if s.queue == nil {
			if err := s.RenderPDF(octx, r.OrganizationID, r.ID); err != nil {
				s.log.Warn("document_pdf_render_failed", "version_id", r.ID, "error", err)
			}
			n++
			continue
		}
		switch err := s.enqueuePDF(r.OrganizationID, r.ID, version); {
		case errors.Is(err, errPDFQueued):
			// The task survived (e.g. a slow queue): nothing to do.
		case err != nil:
			s.log.Warn("document_pdf_requeue_failed", "version_id", r.ID, "error", err)
		default:
			s.log.Info("document_pdf_requeued", "organization_id", r.OrganizationID, "version", version)
			n++
		}
	}
	return n, nil
}

func (s *Service) failPDF(ctx context.Context, orgID, versionID int64, msg string) {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, _ = s.q.SetDocumentVersionPDFFailed(ctx, db.SetDocumentVersionPDFFailedParams{OrganizationID: orgID, ID: versionID, PdfError: msg})
}

// RenderPDF renders one version to PDF and stores it privately under the
// organization's prefix (worker task app:documents:render-pdf). A missing
// renderer / storage is recorded as a failure and not retried.
func (s *Service) RenderPDF(ctx context.Context, orgID, versionID int64) error {
	v, err := s.q.GetDocumentVersionByID(ctx, db.GetDocumentVersionByIDParams{OrganizationID: orgID, ID: versionID})
	if err != nil {
		return err
	}
	if v.PdfStatus == model.PDFReady {
		return nil
	}
	if s.renderer == nil || s.storage == nil {
		s.failPDF(ctx, orgID, versionID, "pdf renderer or storage is not configured")
		return nil
	}
	pdf, err := s.renderer.RenderPDF(ctx, []byte(v.Html))
	if err != nil {
		s.failPDF(ctx, orgID, versionID, err.Error())
		if errors.Is(err, pdfrender.ErrUnavailable) {
			return nil
		}
		return err
	}
	org, err := s.q.GetDocumentOrganization(ctx, orgID)
	if err != nil {
		return err
	}
	docUUID, err := s.documentUUIDOf(ctx, orgID, v.DocumentID)
	if err != nil {
		return err
	}
	key := storage.DocumentPDFObjectKey(org.Uuid.String(), docUUID.String(), v.Uuid.String())
	if err := s.storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(pdf), Size: int64(len(pdf)), ContentType: "application/pdf", Filename: v.Uuid.String() + ".pdf",
	}, key); err != nil {
		s.failPDF(ctx, orgID, versionID, "storage: "+err.Error())
		return err
	}
	sum := sha256.Sum256(pdf)
	_, err = s.q.SetDocumentVersionPDF(ctx, db.SetDocumentVersionPDFParams{
		OrganizationID: orgID, ID: versionID, PdfObjectKey: textOf(key), PdfSizeBytes: int64(len(pdf)),
		PdfSha256: textOf(hex.EncodeToString(sum[:])), PdfRenderedAt: ts(s.clock()),
	})
	if err == nil {
		s.publish(ctx, model.ResourceDocuments, "updated", docUUID)
	}
	return err
}

func (s *Service) documentUUIDOf(ctx context.Context, orgID, documentID int64) (uuid.UUID, error) {
	return s.q.GetDocumentUUIDByID(ctx, db.GetDocumentUUIDByIDParams{OrganizationID: orgID, ID: documentID})
}

// PDFFile is a PDF to stream.
type PDFFile struct {
	Body     io.ReadCloser
	Size     int64
	Filename string
	Version  int32
	Kind     string
}

// OpenPDF opens the PDF of a version (route: tenant.document.download).
// Not rendered yet: the render is (re)requested and ErrPDFNotReady
// answered.
func (s *Service) OpenPDF(ctx context.Context, orgID int64, docID, id uuid.UUID) (PDFFile, error) {
	doc, err := s.documentRow(ctx, orgID, docID)
	if err != nil {
		return PDFFile{}, err
	}
	v, err := s.versionRow(ctx, orgID, doc, id)
	if err != nil {
		return PDFFile{}, err
	}
	if s.storage == nil {
		return PDFFile{}, ErrStorageUnavailable
	}
	if v.PdfStatus != model.PDFReady {
		s.requestPDF(ctx, orgID, v.ID)
		v, err = s.versionRow(ctx, orgID, doc, id)
		if err != nil {
			return PDFFile{}, err
		}
		if v.PdfStatus != model.PDFReady {
			return PDFFile{}, ErrPDFNotReady
		}
	}
	org, err := s.q.GetDocumentOrganization(ctx, orgID)
	if err != nil {
		return PDFFile{}, err
	}
	// Defence in depth: only stream keys under this organization's prefix.
	if !v.PdfObjectKey.Valid || !storage.OrganizationOwnsKey(v.PdfObjectKey.String, org.Uuid.String()) {
		s.log.Error("document_pdf_outside_organization", "version", id.String())
		return PDFFile{}, ErrPDFNotReady
	}
	rc, size, err := s.storage.Download(ctx, v.PdfObjectKey.String)
	if err != nil {
		s.log.Error("document_pdf_download_failed", "version", id.String(), "error", err)
		return PDFFile{}, ErrPDFNotReady
	}
	if size <= 0 {
		size = v.PdfSizeBytes
	}
	name := strings.ToLower(doc.ReservationReference) + "-" + doc.Kind + "-v" + strconv.Itoa(int(v.Version)) + ".pdf"
	return PDFFile{Body: rc, Size: size, Filename: name, Version: v.Version, Kind: doc.Kind}, nil
}
