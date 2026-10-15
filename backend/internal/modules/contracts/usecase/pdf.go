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

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// requestPDF marks a version's PDF as pending and renders it: in the
// worker when a queue is wired, else in the request (best effort; a failure
// is recorded on the version and retried by the next download).
func (s *Service) requestPDF(ctx context.Context, orgID, versionID int64) {
	n, err := s.q.SetDocumentVersionPDFPending(ctx, db.SetDocumentVersionPDFPendingParams{OrganizationID: orgID, ID: versionID})
	if err != nil || n != 1 {
		return
	}
	if s.queue != nil {
		if err := s.queue.EnqueueDocumentPDF(orgID, versionID); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			s.log.Warn("document_pdf_enqueue_failed", "version_id", versionID, "error", err)
			s.failPDF(ctx, orgID, versionID, "queue: "+err.Error())
		}
		return
	}
	if err := s.RenderPDF(ctx, orgID, versionID); err != nil {
		s.log.Warn("document_pdf_render_failed", "version_id", versionID, "error", err)
	}
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
		return PDFFile{}, ErrPDFNotReady
	}
	if size <= 0 {
		size = v.PdfSizeBytes
	}
	name := strings.ToLower(doc.ReservationReference) + "-" + doc.Kind + "-v" + strconv.Itoa(int(v.Version)) + ".pdf"
	return PDFFile{Body: rc, Size: size, Filename: name, Version: v.Version, Kind: doc.Kind}, nil
}
