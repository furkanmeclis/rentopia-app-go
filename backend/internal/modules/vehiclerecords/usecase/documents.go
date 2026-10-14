package usecase

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/repository"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxDocumentFileBytes caps one document file (10 MiB).
const MaxDocumentFileBytes = 10 << 20

// documentMIMEToExt are the accepted file types, sniffed from the bytes (the
// client-declared type and the file name are ignored).
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

// DocumentChange is the outcome of a document mutation: the vehicle (for
// activity and live events) and the document.
type DocumentChange struct {
	VehicleUUID uuid.UUID
	Plate       string
	Document    model.Document
}

// ListDocuments returns the live documents of a vehicle (history included).
func (s *Service) ListDocuments(ctx context.Context, orgID int64, vehicleUUID uuid.UUID) ([]model.Document, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListDocuments(ctx, orgID, v.ID, MaxVehicleItems)
	if err != nil {
		return nil, err
	}
	return s.mapDocuments(rows), nil
}

// CreateDocument records a document of a vehicle (metadata; the file is
// uploaded with UploadDocumentFile).
func (s *Service) CreateDocument(ctx context.Context, orgID int64, vehicleUUID uuid.UUID, in model.DocumentInput) (DocumentChange, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return DocumentChange{}, err
	}
	fields := repository.DocumentFields{}
	if err := applyDocument(&fields, in, true); err != nil {
		return DocumentChange{}, err
	}
	row, err := s.repo.CreateDocument(ctx, orgID, v.ID, fields, actorArg(ctx))
	if err != nil {
		return DocumentChange{}, err
	}
	return s.documentChange(ctx, orgID, v, row.Uuid)
}

// PatchDocument updates the given metadata fields of a document.
func (s *Service) PatchDocument(ctx context.Context, orgID int64, vehicleUUID, id uuid.UUID, in model.DocumentInput) (DocumentChange, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return DocumentChange{}, err
	}
	current, err := s.repo.GetDocument(ctx, orgID, v.ID, id)
	if err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	fields := repository.DocumentFields{
		DocType: current.DocType, DocumentNumber: current.DocumentNumber, Issuer: current.Issuer,
		ValidFrom: current.ValidFrom, ValidUntil: current.ValidUntil, Note: current.Note,
	}
	if err := applyDocument(&fields, in, false); err != nil {
		return DocumentChange{}, err
	}
	if _, err := s.repo.UpdateDocument(ctx, orgID, v.ID, id, fields); err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	return s.documentChange(ctx, orgID, v, id)
}

// DeleteDocument soft-deletes a document. Its file stays in storage (the
// row keeps the key) so a deleted document can be restored by support.
func (s *Service) DeleteDocument(ctx context.Context, orgID int64, vehicleUUID, id uuid.UUID) (DocumentChange, error) {
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return DocumentChange{}, err
	}
	current, err := s.repo.GetDocument(ctx, orgID, v.ID, id)
	if err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	if err := s.repo.DeleteDocument(ctx, orgID, v.ID, id); err != nil {
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	out := DocumentChange{VehicleUUID: v.Uuid, Plate: v.Plate}
	out.Document = s.mapDocuments([]repository.DocumentRow{current})[0]
	return out, nil
}

// UploadDocumentFile stores (or replaces) the scanned file of a document in
// private storage. The type is sniffed from the bytes: PDF, JPEG, PNG or
// WebP, at most 10 MiB.
func (s *Service) UploadDocumentFile(ctx context.Context, orgID int64, vehicleUUID, id uuid.UUID, in FileUpload) (DocumentChange, error) {
	if s.storage == nil {
		return DocumentChange{}, ErrStorageUnavailable
	}
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return DocumentChange{}, err
	}
	current, err := s.repo.GetDocument(ctx, orgID, v.ID, id)
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
	mime, ext, ok := SniffDocumentType(head)
	if !ok {
		return DocumentChange{}, validation("file", "invalid_type", "file must be a PDF, JPEG, PNG or WebP")
	}
	orgUUID, err := s.repo.OrganizationUUID(ctx, orgID)
	if err != nil {
		return DocumentChange{}, err
	}
	key := storage.VehicleDocumentObjectKey(orgUUID.String(), v.Uuid.String(), id.String(), uuid.NewString(), ext)
	name := cleanFilename(in.Filename, ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: name,
	}, key); err != nil {
		return DocumentChange{}, err
	}
	if _, err := s.repo.SetDocumentFile(ctx, orgID, v.ID, id, repository.FileFields{
		ObjectKey: key, Name: name, ContentType: mime, SizeBytes: in.Size,
	}); err != nil {
		_ = s.storage.Delete(ctx, key)
		return DocumentChange{}, mapNotFound(err, ErrDocumentNotFound)
	}
	// The replaced file is no longer referenced by any row.
	if current.FileObjectKey.Valid && current.FileObjectKey.String != key {
		_ = s.storage.Delete(ctx, current.FileObjectKey.String)
	}
	return s.documentChange(ctx, orgID, v, id)
}

// DocumentFile is an open document file.
type DocumentFile struct {
	Body        io.ReadCloser
	ContentType string
	Name        string
	Size        int64
	VehicleUUID uuid.UUID
	Document    model.Document
}

// OpenDocumentFile opens the file of a document for streaming.
func (s *Service) OpenDocumentFile(ctx context.Context, orgID int64, vehicleUUID, id uuid.UUID) (DocumentFile, error) {
	if s.storage == nil {
		return DocumentFile{}, ErrStorageUnavailable
	}
	v, err := s.vehicle(ctx, orgID, vehicleUUID)
	if err != nil {
		return DocumentFile{}, err
	}
	row, err := s.repo.GetDocument(ctx, orgID, v.ID, id)
	if err != nil {
		return DocumentFile{}, mapNotFound(err, ErrDocumentNotFound)
	}
	if !row.FileObjectKey.Valid {
		return DocumentFile{}, ErrNoFile
	}
	rc, size, err := s.storage.Download(ctx, row.FileObjectKey.String)
	if err != nil {
		s.log.Warn("vehicle_document_file_missing", "document", id.String(), "error", err)
		return DocumentFile{}, ErrNoFile
	}
	if size <= 0 {
		size = row.FileSizeBytes
	}
	return DocumentFile{
		Body: rc, ContentType: row.FileContentType, Name: row.FileName, Size: size, VehicleUUID: v.Uuid,
		Document: s.mapDocuments([]repository.DocumentRow{row})[0],
	}, nil
}

// SniffDocumentType returns the MIME type and extension of an accepted
// document file from its first bytes.
func SniffDocumentType(head []byte) (string, string, bool) {
	mime := strings.ToLower(http.DetectContentType(head))
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	ext, ok := documentMIMEToExt[mime]
	return mime, ext, ok
}

// cleanFilename keeps the base name (at most 255 characters) and falls back
// to "document.<ext>".
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

func (s *Service) documentChange(ctx context.Context, orgID int64, v db.Vehicle, id uuid.UUID) (DocumentChange, error) {
	rows, err := s.repo.ListDocuments(ctx, orgID, v.ID, MaxVehicleItems)
	if err != nil {
		return DocumentChange{}, err
	}
	for _, d := range s.mapDocuments(rows) {
		if d.UUID == id {
			return DocumentChange{VehicleUUID: v.Uuid, Plate: v.Plate, Document: d}, nil
		}
	}
	return DocumentChange{}, ErrDocumentNotFound
}

func applyDocument(f *repository.DocumentFields, in model.DocumentInput, create bool) error {
	ve := &ValidationError{}
	if in.DocType != nil {
		t := strings.TrimSpace(*in.DocType)
		if !vehiclemodel.Contains(model.DocumentTypes, t) {
			ve.add("doc_type", "invalid", "doc_type must be one of "+strings.Join(model.DocumentTypes, ", "))
		} else {
			f.DocType = t
		}
	} else if create {
		ve.add("doc_type", "required", "doc_type is required")
	}
	applyText(ve, "document_number", in.DocumentNumber, 128, &f.DocumentNumber)
	applyText(ve, "issuer", in.Issuer, 255, &f.Issuer)
	applyText(ve, "note", in.Note, 2000, &f.Note)
	applyDate(ve, "valid_from", in.ValidFrom, &f.ValidFrom)
	applyDate(ve, "valid_until", in.ValidUntil, &f.ValidUntil)
	if f.ValidFrom.Valid && f.ValidUntil.Valid && f.ValidUntil.Time.Before(f.ValidFrom.Time) {
		ve.add("valid_until", "before_valid_from", "valid_until must not be before valid_from")
	}
	return ve.orNil()
}

// mapDocuments converts rows and marks the current document of each type
// (latest valid_until; a dated document wins over an undated one; ties go to
// the newest row).
func (s *Service) mapDocuments(rows []repository.DocumentRow) []model.Document {
	today := s.today()
	current := map[string]repository.DocumentRow{}
	for _, row := range rows {
		best, ok := current[row.DocType]
		if !ok || newerDocument(row, best) {
			current[row.DocType] = row
		}
	}
	out := make([]model.Document, 0, len(rows))
	for _, row := range rows {
		d := model.Document{
			UUID: row.Uuid, DocType: row.DocType, DocumentNumber: row.DocumentNumber, Issuer: row.Issuer,
			ValidFrom: datePtr(row.ValidFrom), ValidUntil: datePtr(row.ValidUntil),
			Status:    vehiclemodel.DocumentState(timePtr(row.ValidUntil), today),
			IsCurrent: current[row.DocType].ID == row.ID, Note: row.Note,
			CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
		}
		if row.ValidUntil.Valid {
			days := vehiclemodel.DaysUntil(today, row.ValidUntil.Time)
			d.DaysLeft = &days
		}
		if row.FileObjectKey.Valid {
			d.File = &model.FileInfo{Name: row.FileName, ContentType: row.FileContentType, SizeBytes: row.FileSizeBytes}
		}
		if row.CreatorUuid.Valid {
			d.CreatedBy = &model.Actor{
				UUID: row.CreatorUuid.Bytes, Name: row.CreatorName.String,
				Surname: row.CreatorSurname.String, Email: row.CreatorEmail.String,
			}
		}
		out = append(out, d)
	}
	return out
}

func newerDocument(a, b repository.DocumentRow) bool {
	switch {
	case a.ValidUntil.Valid && !b.ValidUntil.Valid:
		return true
	case !a.ValidUntil.Valid && b.ValidUntil.Valid:
		return false
	case a.ValidUntil.Valid && !a.ValidUntil.Time.Equal(b.ValidUntil.Time):
		return a.ValidUntil.Time.After(b.ValidUntil.Time)
	default:
		return a.ID > b.ID
	}
}

// dateArg converts a day to a DATE.
func dateArg(t time.Time) pgtype.Date {
	return repository.DateArg(t)
}
