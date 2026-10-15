package usecase

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxDocumentFileBytes caps one document file (10 MiB).
const MaxDocumentFileBytes = 10 << 20

// documentMIMEToExt are the accepted file types, sniffed from the bytes.
var documentMIMEToExt = map[string]string{
	"application/pdf": "pdf",
	"image/jpeg":      "jpg",
	"image/png":       "png",
	"image/webp":      "webp",
}

// FileUpload is one uploaded file.
type FileUpload struct {
	Body     io.Reader
	Size     int64
	Filename string
}

// DocumentChange is the outcome of a document mutation.
type DocumentChange struct {
	CustomerUUID uuid.UUID
	Document     model.Document
}

// ListDocuments returns the live documents of a customer.
func (s *Service) ListDocuments(ctx context.Context, orgID int64, customerUUID uuid.UUID) ([]model.Document, error) {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListDocuments(ctx, orgID, c.ID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Document, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapDocument(row))
	}
	return out, nil
}

// CreateDocument records a document (metadata; the file is uploaded with
// UploadDocumentFile). driver_uuid must be a driver of the same customer.
func (s *Service) CreateDocument(ctx context.Context, orgID int64, customerUUID uuid.UUID, in model.DocumentInput) (DocumentChange, error) {
	c, err := s.liveCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return DocumentChange{}, err
	}
	ve := &ValidationError{}
	docType := ""
	if in.DocType != nil {
		docType = strings.TrimSpace(*in.DocType)
	}
	if !model.Contains(model.DocumentTypes, docType) {
		ve.add("doc_type", "invalid", "doc_type must be one of "+strings.Join(model.DocumentTypes, ", "))
	}
	var validUntil pgtype.Date
	applyDate(ve, "valid_until", in.ValidUntil, &validUntil)
	note := ""
	applyText(ve, "note", in.Note, 2000, &note)
	var driverID pgtype.Int8
	if in.DriverUUID != nil {
		d, err := s.repo.GetDriver(ctx, orgID, c.ID, *in.DriverUUID)
		if err != nil {
			if mapNotFound(err, ErrDriverNotFound) == ErrDriverNotFound {
				ve.add("driver_uuid", "not_found", "driver was not found")
			} else {
				return DocumentChange{}, err
			}
		} else {
			driverID = pgtype.Int8{Int64: d.ID, Valid: true}
		}
	}
	if err := ve.orNil(); err != nil {
		return DocumentChange{}, err
	}
	row, err := s.repo.CreateDocument(ctx, db.CreateCustomerDocumentParams{
		OrganizationID: orgID, CustomerID: c.ID, DriverID: driverID, DocType: docType, ValidUntil: validUntil,
		Note: note, CreatedBy: actorArg(ctx),
	})
	if err != nil {
		return DocumentChange{}, mapErr(err)
	}
	return s.documentChange(ctx, orgID, c, row.Uuid)
}

// DeleteDocument soft-deletes a document (the file stays until the
// customer is anonymized, so support can restore a mistake).
func (s *Service) DeleteDocument(ctx context.Context, orgID int64, customerUUID, id uuid.UUID) (DocumentChange, error) {
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return DocumentChange{}, err
	}
	current, err := s.repo.GetDocument(ctx, orgID, c.ID, id)
	if err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	if err := s.repo.DeleteDocument(ctx, orgID, c.ID, id); err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	return DocumentChange{CustomerUUID: c.Uuid, Document: mapDocument(current)}, nil
}

// UploadDocumentFile stores (or replaces) the file of a document in private
// storage. The type is sniffed from the bytes: PDF, JPEG, PNG or WebP, at
// most 10 MiB.
func (s *Service) UploadDocumentFile(ctx context.Context, orgID int64, customerUUID, id uuid.UUID, in FileUpload) (DocumentChange, error) {
	if s.storage == nil {
		return DocumentChange{}, ErrStorageUnavailable
	}
	c, err := s.liveCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return DocumentChange{}, err
	}
	current, err := s.repo.GetDocument(ctx, orgID, c.ID, id)
	if err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	if in.Size <= 0 || in.Body == nil {
		return DocumentChange{}, validation("file", "required", "file is required")
	}
	if in.Size > MaxDocumentFileBytes {
		return DocumentChange{}, validation("file", "too_large", "file must be at most 10 MiB")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	mime, ext, ok := sniffDocumentType(head)
	if !ok {
		return DocumentChange{}, validation("file", "invalid_type", "file must be a PDF, JPEG, PNG or WebP")
	}
	orgUUID, err := s.repo.OrganizationUUID(ctx, orgID)
	if err != nil {
		return DocumentChange{}, err
	}
	key := storage.CustomerDocumentObjectKey(orgUUID.String(), c.Uuid.String(), id.String(), uuid.NewString(), ext)
	name := cleanFilename(in.Filename, ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: name,
	}, key); err != nil {
		return DocumentChange{}, err
	}
	if _, err := s.repo.SetDocumentFile(ctx, db.SetCustomerDocumentFileParams{
		OrganizationID: orgID, CustomerID: c.ID, Uuid: id, FileObjectKey: pgtype.Text{String: key, Valid: true},
		FileName: name, FileContentType: mime, FileSizeBytes: in.Size,
	}); err != nil {
		_ = s.storage.Delete(ctx, key)
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	if current.FileObjectKey.Valid && current.FileObjectKey.String != key {
		_ = s.storage.Delete(ctx, current.FileObjectKey.String)
	}
	return s.documentChange(ctx, orgID, c, id)
}

// DocumentFile is an open document file.
type DocumentFile struct {
	Body         io.ReadCloser
	ContentType  string
	Name         string
	Size         int64
	CustomerUUID uuid.UUID
	Document     model.Document
}

// OpenDocumentFile opens the file of a document for streaming (the handler
// checks tenant.document.download + tenant.customers.reveal and records the
// download).
func (s *Service) OpenDocumentFile(ctx context.Context, orgID int64, customerUUID, id uuid.UUID) (DocumentFile, error) {
	if s.storage == nil {
		return DocumentFile{}, ErrStorageUnavailable
	}
	c, err := s.getCustomer(ctx, orgID, customerUUID)
	if err != nil {
		return DocumentFile{}, err
	}
	row, err := s.repo.GetDocument(ctx, orgID, c.ID, id)
	if err != nil {
		return DocumentFile{}, mapNotFound(err, ErrDocumentNotFound)
	}
	if !row.FileObjectKey.Valid {
		return DocumentFile{}, ErrNoFile
	}
	orgUUID, err := s.repo.OrganizationUUID(ctx, orgID)
	if err != nil {
		return DocumentFile{}, err
	}
	// Defence in depth: only stream keys under this organization's prefix.
	if !storage.OrganizationOwnsKey(row.FileObjectKey.String, orgUUID.String()) {
		s.log.Error("customer_document_file_outside_organization", "document", id.String())
		return DocumentFile{}, ErrNoFile
	}
	rc, size, err := s.storage.Download(ctx, row.FileObjectKey.String)
	if err != nil {
		s.log.Warn("customer_document_file_missing", "document", id.String(), "error", err)
		return DocumentFile{}, ErrNoFile
	}
	if size <= 0 {
		size = row.FileSizeBytes
	}
	return DocumentFile{
		Body: rc, ContentType: row.FileContentType, Name: row.FileName, Size: size, CustomerUUID: c.Uuid,
		Document: mapDocument(row),
	}, nil
}

func (s *Service) documentChange(ctx context.Context, orgID int64, c db.Customer, id uuid.UUID) (DocumentChange, error) {
	row, err := s.repo.GetDocument(ctx, orgID, c.ID, id)
	if err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	return DocumentChange{CustomerUUID: c.Uuid, Document: mapDocument(row)}, nil
}

func mapDocument(row repository.DocumentRow) model.Document {
	out := model.Document{
		UUID: row.Uuid, DocType: row.DocType, ValidUntil: datePtr(row.ValidUntil), HasFile: row.FileObjectKey.Valid,
		FileName: row.FileName, ContentType: row.FileContentType, SizeBytes: row.FileSizeBytes, Note: row.Note,
		CreatedAt: row.CreatedAt.Time.UTC(),
	}
	if row.DriverUuid.Valid {
		out.Driver = &model.DriverRef{
			UUID: row.DriverUuid.Bytes, FullName: strings.TrimSpace(row.DriverFirstName.String + " " + row.DriverLastName.String),
		}
	}
	return out
}

func sniffDocumentType(head []byte) (string, string, bool) {
	mime := strings.ToLower(http.DetectContentType(head))
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	ext, ok := documentMIMEToExt[mime]
	return mime, ext, ok
}

func cleanFilename(name, ext string) string {
	name = strings.TrimSpace(path.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "document." + ext
	}
	for utf8.RuneCountInString(name) > 255 {
		r := []rune(name)
		name = string(r[len(r)-255:])
	}
	return name
}
