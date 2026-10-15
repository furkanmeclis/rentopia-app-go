package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// exifHead is how much of a file is kept to read its EXIF block.
const exifHead = 256 << 10

func itoa(i int) string { return strconv.Itoa(i) }

func strconvFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func (s *Service) orgUUID(ctx context.Context, orgID int64) (string, error) {
	if sc, ok := orgctx.From(ctx); ok && sc.ID == orgID && sc.UUID != uuid.Nil {
		return sc.UUID.String(), nil
	}
	u, err := s.q.GetOrganizationUUIDForInspections(ctx, orgID)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *Service) deleteObject(ctx context.Context, key string) {
	if s.store == nil || key == "" {
		return
	}
	if err := s.store.Delete(ctx, key); err != nil {
		s.log.Warn("inspection_object_delete_failed", "error", err)
	}
}

func uploadPath(inspection, photo uuid.UUID) string {
	return "/v1/tenant/inspections/" + inspection.String() + "/photos/" + photo.String() + "/file"
}

// AddPhoto announces a photo of a slot (or a damage) and answers the
// API-issued upload address. A repeated client_ref (the device's upload
// queue retrying) answers the same photo.
func (s *Service) AddPhoto(ctx context.Context, orgID int64, id uuid.UUID, in model.PhotoInput) (model.PhotoUpload, bool, error) {
	if s.store == nil {
		return model.PhotoUpload{}, false, ErrStorageUnavailable
	}
	row, r, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.PhotoUpload{}, false, err
	}
	ve := &ValidationError{}
	in.Slot = strings.TrimSpace(in.Slot)
	in.ClientRef = strings.TrimSpace(in.ClientRef)
	in.ContentType = strings.ToLower(strings.TrimSpace(in.ContentType))
	var snap model.TemplateSnapshot
	_ = json.Unmarshal(row.TemplateSnapshot, &snap)
	known := in.Slot == model.SlotExtra || in.Slot == model.SlotDamage
	for _, sl := range snap.PhotoSlots {
		known = known || sl.Key == in.Slot
	}
	if !known {
		ve.add("slot", "invalid", "not a photo slot of the form")
	}
	var damageID pgtype.Int8
	if in.Slot == model.SlotDamage {
		if in.DamageUUID == nil {
			ve.add("damage_uuid", "required", "a damage photo names its damage")
		} else {
			d, err := s.q.GetInspectionDamage(ctx, db.GetInspectionDamageParams{OrganizationID: orgID, InspectionID: row.ID, Uuid: *in.DamageUUID})
			if notFound(err) {
				ve.add("damage_uuid", "not_found", "damage was not found")
			} else if err != nil {
				return model.PhotoUpload{}, false, err
			} else {
				damageID = int8Of(d.ID)
			}
		}
	} else if in.DamageUUID != nil {
		ve.add("damage_uuid", "invalid", "only a damage photo names a damage")
	}
	if !model.Contains(model.PhotoContentTypes, in.ContentType) {
		ve.add("content_type", "invalid_type", "images only: JPEG, PNG, WebP or HEIC")
	}
	if in.SizeBytes <= 0 || in.SizeBytes > model.MaxPhotoBytes {
		ve.add("size_bytes", "too_large", "a photo is 1 byte - 15 MiB")
	}
	if len(in.ClientRef) > 64 {
		ve.add("client_ref", "too_long", "client_ref is at most 64 characters")
	}
	if in.DeviceCapturedAt != nil && in.DeviceCapturedAt.After(s.clock().Add(24*time.Hour)) {
		ve.add("device_captured_at", "in_future", "the device time is far in the future")
	}
	if err := ve.orNil(); err != nil {
		return model.PhotoUpload{}, false, err
	}
	if in.ClientRef != "" {
		if existing, err := s.q.GetInspectionPhotoByClientRef(ctx, db.GetInspectionPhotoByClientRefParams{
			OrganizationID: orgID, InspectionID: row.ID, ClientRef: pgtype.Text{String: in.ClientRef, Valid: true},
		}); err == nil {
			out, err := s.photoUpload(ctx, orgID, row, existing)
			return out, false, err
		} else if !notFound(err) {
			return model.PhotoUpload{}, false, err
		}
	}
	org, err := s.orgUUID(ctx, orgID)
	if err != nil {
		return model.PhotoUpload{}, false, err
	}
	photoID := uuid.New()
	ext := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "image/heic": "heic", "image/heif": "heif"}[in.ContentType]
	key := storage.InspectionPhotoObjectKey(org, r.UUID.String(), row.Uuid.String(), photoID.String(), ext)
	var clientRef pgtype.Text
	if in.ClientRef != "" {
		clientRef = pgtype.Text{String: in.ClientRef, Valid: true}
	}
	created, err := s.q.InsertInspectionPhoto(ctx, db.InsertInspectionPhotoParams{
		OrganizationID: orgID, InspectionID: row.ID, RentalID: row.RentalID, InspectionVersion: row.Version, Slot: in.Slot,
		DamageID: damageID, ClientRef: clientRef, Status: model.PhotoPending, ObjectKey: key, ContentType: in.ContentType,
		DeclaredSizeBytes: in.SizeBytes, DeviceCapturedAt: tsPtr(in.DeviceCapturedAt), Exif: []byte("{}"),
		ScanStatus: model.ScanSkipped, CreatedBy: int8Ptr(actorID(ctx)),
	})
	if uniqueViolation(err, "uq_inspection_photos_client_ref") {
		existing, gerr := s.q.GetInspectionPhotoByClientRef(ctx, db.GetInspectionPhotoByClientRefParams{
			OrganizationID: orgID, InspectionID: row.ID, ClientRef: clientRef,
		})
		if gerr != nil {
			return model.PhotoUpload{}, false, gerr
		}
		out, gerr := s.photoUpload(ctx, orgID, row, existing)
		return out, false, gerr
	}
	if err != nil {
		if immutable(err) {
			return model.PhotoUpload{}, false, stateErr("finalized", "a finalized form cannot change")
		}
		return model.PhotoUpload{}, false, err
	}
	s.publish(ctx, model.ResourceInspections, "updated", id)
	out, err := s.photoUpload(ctx, orgID, row, created.Uuid)
	return out, true, err
}

func (s *Service) photoUpload(ctx context.Context, orgID int64, row db.GetInspectionRow, photo uuid.UUID) (model.PhotoUpload, error) {
	p, err := s.photoView(ctx, orgID, row.ID, photo)
	if err != nil {
		return model.PhotoUpload{}, err
	}
	return model.PhotoUpload{Photo: p, Upload: model.UploadTarget{
		Method: "PUT", Path: uploadPath(row.Uuid, photo), Field: "file", MaxBytes: model.MaxPhotoBytes,
	}}, nil
}

func (s *Service) photoView(ctx context.Context, orgID, inspectionID int64, photo uuid.UUID) (model.Photo, error) {
	rows, err := s.q.ListInspectionPhotos(ctx, db.ListInspectionPhotosParams{OrganizationID: orgID, InspectionID: inspectionID})
	if err != nil {
		return model.Photo{}, err
	}
	for _, p := range rows {
		if p.Uuid == photo {
			return photoOf(p), nil
		}
	}
	return model.Photo{}, ErrPhotoNotFound
}

func (s *Service) photoRow(ctx context.Context, orgID int64, row db.GetInspectionRow, photo uuid.UUID) (db.GetInspectionPhotoRow, error) {
	p, err := s.q.GetInspectionPhoto(ctx, db.GetInspectionPhotoParams{OrganizationID: orgID, InspectionID: row.ID, Uuid: photo})
	if notFound(err) {
		return p, ErrPhotoNotFound
	}
	return p, err
}

// FileUpload is the bytes of an upload.
type FileUpload struct {
	Body io.Reader
	Size int64
}

// UploadPhoto stores the bytes of an announced photo (the type is sniffed
// from the bytes; images only, at most 15 MiB). The photo is "uploaded"
// until the completion step verifies and hashes the stored object.
func (s *Service) UploadPhoto(ctx context.Context, orgID int64, id, photo uuid.UUID, in FileUpload) (model.Photo, error) {
	if s.store == nil {
		return model.Photo{}, ErrStorageUnavailable
	}
	row, _, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Photo{}, err
	}
	p, err := s.photoRow(ctx, orgID, row, photo)
	if err != nil {
		return model.Photo{}, err
	}
	if p.Status == model.PhotoCompleted {
		return model.Photo{}, stateErr("completed", "the photo is already completed")
	}
	if in.Body == nil || in.Size <= 0 {
		return model.Photo{}, validation("file", "required", "file is required")
	}
	if in.Size > model.MaxPhotoBytes {
		return model.Photo{}, validation("file", "too_large", "a photo is at most 15 MiB")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	mime, ext, ok := SniffImage(head)
	if !ok {
		return model.Photo{}, validation("file", "invalid_type", "images only: JPEG, PNG, WebP or HEIC")
	}
	heicFamily := (ext == "heif" || ext == "heic") && (strings.HasSuffix(p.ObjectKey, ".heic") || strings.HasSuffix(p.ObjectKey, ".heif"))
	if !strings.HasSuffix(p.ObjectKey, "."+ext) && !heicFamily {
		return model.Photo{}, validation("file", "type_mismatch", "the file is not the announced image type")
	}
	if err := s.store.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), io.LimitReader(in.Body, model.MaxPhotoBytes)), Size: in.Size, ContentType: mime,
		Filename: photo.String() + "." + ext,
	}, p.ObjectKey); err != nil {
		return model.Photo{}, err
	}
	cnt, err := s.q.SetInspectionPhotoUploaded(ctx, db.SetInspectionPhotoUploadedParams{OrganizationID: orgID, ID: p.ID, ContentType: mime, SizeBytes: in.Size})
	if err != nil {
		if immutable(err) {
			return model.Photo{}, stateErr("finalized", "a finalized form cannot change")
		}
		return model.Photo{}, err
	}
	if cnt != 1 {
		return model.Photo{}, ErrConcurrent
	}
	s.publish(ctx, model.ResourceInspections, "updated", id)
	return s.photoView(ctx, orgID, row.ID, photo)
}

// CompletePhoto verifies that the stored object exists, computes its
// SHA-256 on the server, reads the EXIF time / GPS (kept apart from the
// server time) and records the server upload time. The optional scanner
// runs on the same bytes.
func (s *Service) CompletePhoto(ctx context.Context, orgID int64, id, photo uuid.UUID) (model.Photo, error) {
	if s.store == nil {
		return model.Photo{}, ErrStorageUnavailable
	}
	row, _, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Photo{}, err
	}
	p, err := s.photoRow(ctx, orgID, row, photo)
	if err != nil {
		return model.Photo{}, err
	}
	switch p.Status {
	case model.PhotoCompleted:
		return s.photoView(ctx, orgID, row.ID, photo)
	case model.PhotoPending:
		return model.Photo{}, stateErr("not_uploaded", "the photo was not uploaded yet")
	}
	ok, err := s.store.Exists(ctx, p.ObjectKey)
	if err != nil {
		return model.Photo{}, err
	}
	if !ok {
		return model.Photo{}, stateErr("object_missing", "the uploaded file was not found; upload it again")
	}
	rc, _, err := s.store.Download(ctx, p.ObjectKey)
	if err != nil {
		return model.Photo{}, stateErr("object_missing", "the uploaded file could not be read; upload it again")
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	var head bytes.Buffer
	var scanBuf *bytes.Buffer
	writers := []io.Writer{h, &limitWriter{w: &head, n: exifHead}}
	if s.scanner != nil {
		scanBuf = &bytes.Buffer{}
		writers = append(writers, scanBuf)
	}
	size, err := io.Copy(io.MultiWriter(writers...), io.LimitReader(rc, model.MaxPhotoBytes+1))
	if err != nil {
		return model.Photo{}, err
	}
	if size == 0 || size > model.MaxPhotoBytes || (p.SizeBytes > 0 && size != p.SizeBytes) {
		return model.Photo{}, stateErr("size_mismatch", "the stored file does not match the upload; upload it again")
	}
	if _, _, ok := SniffImage(head.Bytes()); !ok {
		return model.Photo{}, stateErr("invalid_type", "the stored file is not an image")
	}
	scan := model.ScanSkipped
	if s.scanner != nil {
		clean, err := s.scanner.Scan(ctx, bytes.NewReader(scanBuf.Bytes()))
		if err != nil {
			return model.Photo{}, err
		}
		scan = model.ScanClean
		if !clean {
			s.deleteObject(ctx, p.ObjectKey)
			return model.Photo{}, stateErr("infected", "the file was rejected by the malware scan")
		}
	}
	ex := ReadEXIF(head.Bytes())
	var lat, lng pgtype.Numeric
	if ex.Lat != nil && ex.Lng != nil {
		lat, lng = numOf(ex.Lat, 1e6), numOf(ex.Lng, 1e6)
	}
	cnt, err := s.q.CompleteInspectionPhoto(ctx, db.CompleteInspectionPhotoParams{
		OrganizationID: orgID, ID: p.ID, Sha256: pgtype.Text{String: hex.EncodeToString(h.Sum(nil)), Valid: true}, SizeBytes: size,
		UploadedAt: ts(s.clock()), UploadedBy: int8Ptr(actorID(ctx)), ExifTakenAt: tsPtr(ex.TakenAt), ExifTakenLocal: ex.TakenLocal,
		ExifGpsLat: lat, ExifGpsLng: lng, Exif: mustJSON(map[string]string{"make": ex.Make, "model": ex.Model}), ScanStatus: scan,
	})
	if err != nil {
		if immutable(err) {
			return model.Photo{}, stateErr("finalized", "a finalized form cannot change")
		}
		return model.Photo{}, err
	}
	if cnt != 1 {
		return s.photoView(ctx, orgID, row.ID, photo)
	}
	s.publish(ctx, model.ResourceInspections, "updated", id)
	return s.photoView(ctx, orgID, row.ID, photo)
}

type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := len(p)
		if k > l.n {
			k = l.n
		}
		if _, err := l.w.Write(p[:k]); err != nil {
			return 0, err
		}
		l.n -= k
	}
	return len(p), nil
}

// DeletePhoto removes a photo of a draft (retake).
func (s *Service) DeletePhoto(ctx context.Context, orgID int64, id, photo uuid.UUID) (model.Inspection, error) {
	row, _, err := s.draft(ctx, orgID, id)
	if err != nil {
		return model.Inspection{}, err
	}
	p, err := s.photoRow(ctx, orgID, row, photo)
	if err != nil {
		return model.Inspection{}, err
	}
	if _, err := s.q.DeleteInspectionPhoto(ctx, db.DeleteInspectionPhotoParams{OrganizationID: orgID, InspectionID: row.ID, ID: p.ID}); err != nil {
		return model.Inspection{}, err
	}
	// Copies of a revision share the object of the finalized version.
	if !strings.Contains(p.ObjectKey, "/"+row.Uuid.String()+"/") {
		return s.after(ctx, orgID, id)
	}
	s.deleteObject(ctx, p.ObjectKey)
	return s.after(ctx, orgID, id)
}

// MediaLinks issues short-lived API links to the completed photos (and
// the signature) of a form the caller can read.
func (s *Service) MediaLinks(ctx context.Context, orgID int64, id uuid.UUID, in model.MediaLinksInput) (model.MediaLinks, error) {
	row, _, err := s.row(ctx, orgID, id)
	if err != nil {
		return model.MediaLinks{}, err
	}
	out := model.MediaLinks{Items: []model.MediaLink{}}
	if s.media == nil {
		return out, nil
	}
	want := map[uuid.UUID]bool{}
	for _, p := range in.PhotoUUIDs {
		want[p] = true
	}
	photos, err := s.q.ListInspectionPhotos(ctx, db.ListInspectionPhotosParams{OrganizationID: orgID, InspectionID: row.ID})
	if err != nil {
		return out, err
	}
	for _, p := range photos {
		if p.Status != model.PhotoCompleted || (len(want) > 0 && !want[p.Uuid]) {
			continue
		}
		token, exp := s.media.Sign(orgID, MediaPhoto, p.Uuid)
		out.Items = append(out.Items, model.MediaLink{UUID: p.Uuid, Kind: MediaPhoto, URL: MediaPath(token), ExpiresAt: exp})
	}
	if in.Signature || len(in.PhotoUUIDs) == 0 {
		if sig, err := s.q.GetInspectionSignature(ctx, db.GetInspectionSignatureParams{OrganizationID: orgID, InspectionID: row.ID}); err == nil {
			token, exp := s.media.Sign(orgID, MediaSignature, sig.Uuid)
			out.Items = append(out.Items, model.MediaLink{UUID: sig.Uuid, Kind: MediaSignature, URL: MediaPath(token), ExpiresAt: exp})
		}
	}
	return out, nil
}

// MediaFile is an open evidence object.
type MediaFile struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
}

// OpenMedia verifies a media link and opens its object (the organization
// comes from the signed token; the key must lie under its prefix).
func (s *Service) OpenMedia(ctx context.Context, token string) (MediaFile, error) {
	if s.media == nil || s.store == nil {
		return MediaFile{}, ErrMediaLink
	}
	t, err := s.media.Verify(token)
	if err != nil {
		return MediaFile{}, ErrMediaLink
	}
	var key, ctype string
	var size int64
	switch t.Kind {
	case MediaPhoto:
		row, err := s.q.GetInspectionMediaObject(ctx, db.GetInspectionMediaObjectParams{OrganizationID: t.OrgID, Uuid: t.ID})
		if err != nil {
			return MediaFile{}, ErrMediaLink
		}
		key, ctype, size = row.ObjectKey, row.ContentType, row.SizeBytes
	case MediaSignature:
		row, err := s.q.GetInspectionSignatureObject(ctx, db.GetInspectionSignatureObjectParams{OrganizationID: t.OrgID, Uuid: t.ID})
		if err != nil {
			return MediaFile{}, ErrMediaLink
		}
		key, ctype, size = row.ObjectKey, "image/png", row.SizeBytes
	}
	org, err := s.q.GetOrganizationUUIDForInspections(ctx, t.OrgID)
	if err != nil || !storage.OrganizationOwnsKey(key, org.String()) {
		return MediaFile{}, ErrMediaLink
	}
	rc, n, err := s.store.Download(ctx, key)
	if err != nil {
		return MediaFile{}, errors.Join(ErrMediaLink, err)
	}
	if n > 0 {
		size = n
	}
	return MediaFile{Body: rc, ContentType: ctype, Size: size}, nil
}
