package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxImageBytes is the upload limit per image kind.
var MaxImageBytes = map[string]int64{
	"logo":    2 << 20,
	"favicon": 1 << 20,
	"cover":   5 << 20,
}

// ImageUpload is one uploaded file (PUT /v1/tenant/site/assets/{kind}).
type ImageUpload struct {
	Body        io.Reader
	Size        int64
	ContentType string
	Filename    string
}

func imageError(code, msg string) error {
	ve := &ValidationError{}
	ve.add("file", code, msg)
	return ve
}

func draftKey(row db.GetSiteSettingsRow, kind string) pgtype.Text {
	switch kind {
	case "logo":
		return row.LogoObjectKey
	case "favicon":
		return row.FaviconObjectKey
	default:
		return row.CoverObjectKey
	}
}

// dropUnused deletes an image object no draft and no snapshot names any
// more.
func (s *Service) dropUnused(ctx context.Context, orgID int64, key pgtype.Text) {
	if !key.Valid || key.String == "" {
		return
	}
	used, err := s.q.SiteObjectKeyInUse(ctx, db.SiteObjectKeyInUseParams{OrganizationID: orgID, ObjectKey: key.String})
	if err != nil || used {
		return
	}
	if err := s.storage.Delete(ctx, key.String); err != nil {
		s.log.Warn("site_image_delete_failed", "error", err)
	}
}

// SetImage stores a draft image (PNG, JPEG or WebP, sniffed from the bytes:
// SVG and anything else is refused) when version is the version the caller
// saw. The previous draft image is deleted unless a published revision
// still shows it.
func (s *Service) SetImage(ctx context.Context, orgID int64, orgUUID uuid.UUID, orgSlug string, userID int64, kind string,
	version *int, up ImageUpload) (model.Settings, error) {
	if !slices.Contains(model.AssetKinds, kind) {
		return model.Settings{}, ErrNotFound
	}
	if s.storage == nil {
		return model.Settings{}, ErrStorageUnavailable
	}
	v, err := requireVersion(model.VersionInput{Version: version})
	if err != nil {
		return model.Settings{}, err
	}
	limit := MaxImageBytes[kind]
	head := make([]byte, 512)
	n, _ := io.ReadFull(up.Body, head)
	head = head[:n]
	if n == 0 {
		return model.Settings{}, imageError("required", "a file is required")
	}
	if up.Size > limit {
		return model.Settings{}, imageError("too_large", "at most "+strconv.FormatInt(limit>>10, 10)+" KB")
	}
	mime, err := storage.DetectLogoMIME(up.ContentType, head)
	if err != nil {
		return model.Settings{}, imageError("invalid_type", "PNG, JPEG or WebP only")
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		return model.Settings{}, imageError("invalid_type", "PNG, JPEG or WebP only")
	}
	// Read the rest with the limit + 1 so a lying Size cannot pass.
	rest, err := io.ReadAll(io.LimitReader(up.Body, limit+1-int64(n)))
	if err != nil {
		return model.Settings{}, imageError("invalid", "the file could not be read")
	}
	data := append(head, rest...)
	if int64(len(data)) > limit {
		return model.Settings{}, imageError("too_large", "at most "+strconv.FormatInt(limit>>10, 10)+" KB")
	}
	row, v, err := s.ensureRow(ctx, orgID, orgSlug, userID, v)
	if err != nil {
		return model.Settings{}, err
	}
	key := storage.SiteImageObjectKey(orgUUID.String(), kind, uuid.NewString(), ext)
	if err := s.storage.Upload(ctx, storage.File{
		Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mime, Filename: up.Filename,
	}, key); err != nil {
		return model.Settings{}, err
	}
	previous := draftKey(row, kind)
	_, err = s.q.SetSiteDraftImage(ctx, db.SetSiteDraftImageParams{
		OrganizationID: orgID, ExpectedVersion: int32(v), Kind: kind, ObjectKey: pgtype.Text{String: key, Valid: true},
		UpdatedBy: actor(userID),
	})
	if err != nil {
		_ = s.storage.Delete(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Settings{}, ErrVersionConflict
		}
		return model.Settings{}, err
	}
	s.dropUnused(ctx, orgID, previous)
	return s.Get(ctx, orgID, orgSlug)
}

// RemoveImage clears a draft image (the logo then falls back to the
// organization logo).
func (s *Service) RemoveImage(ctx context.Context, orgID int64, orgSlug string, userID int64, kind string,
	version *int) (model.Settings, error) {
	if !slices.Contains(model.AssetKinds, kind) {
		return model.Settings{}, ErrNotFound
	}
	v, err := requireVersion(model.VersionInput{Version: version})
	if err != nil {
		return model.Settings{}, err
	}
	row, v, err := s.ensureRow(ctx, orgID, orgSlug, userID, v)
	if err != nil {
		return model.Settings{}, err
	}
	previous := draftKey(row, kind)
	_, err = s.q.SetSiteDraftImage(ctx, db.SetSiteDraftImageParams{
		OrganizationID: orgID, ExpectedVersion: int32(v), Kind: kind, UpdatedBy: actor(userID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Settings{}, ErrVersionConflict
	}
	if err != nil {
		return model.Settings{}, err
	}
	if s.storage != nil {
		s.dropUnused(ctx, orgID, previous)
	}
	return s.Get(ctx, orgID, orgSlug)
}
