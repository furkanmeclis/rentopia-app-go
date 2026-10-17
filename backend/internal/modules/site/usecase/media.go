package usecase

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"slices"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const MaxMediaBytes int64 = 10 << 20

var (
	ErrMediaInUse = errors.New("media is referenced by a draft or published snapshot")
	ErrMediaQuota = errors.New("site media quota is 200 files and 500 MB")
)

func mediaDBError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23514" {
		switch pe.ConstraintName {
		case "site_media_in_use":
			return ErrMediaInUse
		case "site_media_quota":
			return ErrMediaQuota
		case "site_media_reference":
			ve := &ValidationError{}
			ve.add("config", "invalid_media", "unknown, deleted or foreign media reference")
			return ve
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func mediaModel(row db.SiteMedium) model.Media {
	alt := map[string]string{}
	_ = json.Unmarshal(row.Alt, &alt)
	return model.Media{UUID: row.Uuid, Kind: row.Kind, MIME: row.Mime, Size: row.Size, Width: int(row.Width), Height: int(row.Height), Alt: alt, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func normalizeAlt(alt map[string]string) ([]byte, error) {
	ve := &ValidationError{}
	alt = localizedText(ve, "alt", alt, 300)
	if err := ve.orNil(); err != nil {
		return nil, err
	}
	return json.Marshal(alt)
}

func (s *Service) ListMedia(ctx context.Context, orgID int64, limit, offset int) (model.MediaList, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.q.ListSiteMedia(ctx, db.ListSiteMediaParams{OrganizationID: orgID, PageLimit: int32(limit), PageOffset: int32(offset)})
	if err != nil {
		return model.MediaList{}, err
	}
	total, err := s.q.CountSiteMedia(ctx, orgID)
	out := model.MediaList{Items: []model.Media{}, Total: total, Limit: limit, Offset: offset}
	for _, row := range rows {
		out.Items = append(out.Items, mediaModel(row))
	}
	return out, err
}

func (s *Service) UploadMedia(ctx context.Context, orgID int64, orgUUID uuid.UUID, userID int64, up ImageUpload, in model.MediaInput) (model.Media, error) {
	if s.storage == nil {
		return model.Media{}, ErrStorageUnavailable
	}
	if up.Size > MaxMediaBytes {
		return model.Media{}, imageError("too_large", "at most 10 MB")
	}
	data, err := io.ReadAll(io.LimitReader(up.Body, MaxMediaBytes+1))
	if err != nil {
		return model.Media{}, err
	}
	if int64(len(data)) > MaxMediaBytes {
		return model.Media{}, imageError("too_large", "at most 10 MB")
	}
	mime, err := storage.DetectLogoMIME("", data)
	if err != nil {
		return model.Media{}, imageError("invalid_type", "PNG, JPEG or WebP only")
	}
	dimensions, err := mediaDimensions(data, mime)
	if err != nil || dimensions.Width < 1 || dimensions.Height < 1 {
		return model.Media{}, imageError("invalid_type", "valid image dimensions required")
	}
	// Bound decoded dimensions too; a tiny compressed image must not exhaust renderers.
	if dimensions.Width > 32768 || dimensions.Height > 32768 || int64(dimensions.Width)*int64(dimensions.Height) > 100000000 {
		return model.Media{}, imageError("invalid", "image dimensions exceed 100 megapixels or 32768 pixels per side")
	}
	alt, err := normalizeAlt(in.Alt)
	if err != nil {
		return model.Media{}, err
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		return model.Media{}, err
	}
	id := uuid.New()
	key := storage.SiteMediaObjectKey(orgUUID.String(), id.String(), ext)
	if err := s.storage.Upload(ctx, storage.File{Body: bytes.NewReader(data), Size: int64(len(data)), ContentType: mime, Filename: up.Filename}, key); err != nil {
		return model.Media{}, err
	}
	row, err := s.q.InsertSiteMedia(ctx, db.InsertSiteMediaParams{OrganizationID: orgID, Uuid: id, Mime: mime, Size: int64(len(data)), Width: int32(dimensions.Width), Height: int32(dimensions.Height), Alt: alt, ObjectKey: key, CreatedBy: actor(userID)})
	if err != nil {
		if cleanupErr := s.storage.Delete(ctx, key); cleanupErr != nil {
			s.log.Warn("site_media_cleanup_failed", "error", cleanupErr)
		}
		return model.Media{}, mediaDBError(err)
	}
	return mediaModel(row), nil
}

func (s *Service) PatchMedia(ctx context.Context, orgID int64, id uuid.UUID, in model.MediaInput) (model.Media, error) {
	alt, err := normalizeAlt(in.Alt)
	if err != nil {
		return model.Media{}, err
	}
	row, err := s.q.PatchSiteMedia(ctx, db.PatchSiteMediaParams{OrganizationID: orgID, Uuid: id, Alt: alt})
	return mediaModel(row), mediaDBError(err)
}

func (s *Service) DeleteMedia(ctx context.Context, orgID int64, id uuid.UUID) error {
	key, err := s.q.DeleteSiteMedia(ctx, db.DeleteSiteMediaParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return mediaDBError(err)
	}
	if s.storage != nil {
		if err := s.storage.Delete(ctx, key); err != nil {
			s.log.Warn("site_media_delete_failed", "error", err)
		}
	}
	return nil
}

func configMedia(c model.Config) []string {
	ids := []string{}
	if c.About.Image != "" {
		ids = append(ids, c.About.Image)
	}
	for _, content := range c.Content {
		for _, slide := range content.HeroSlides {
			if slide.Media != "" && !slices.Contains(ids, slide.Media) {
				ids = append(ids, slide.Media)
			}
		}
	}
	return ids
}

func mediaPath(site uuid.UUID, id string, preview string) string {
	path := fmt.Sprintf("/v1/public/site/media/%s/%s", site, id)
	if preview != "" {
		path += "?preview=" + preview
	}
	return path
}

func (s *Service) PublicMedia(ctx context.Context, site, id uuid.UUID, preview string) (io.ReadCloser, string, error) {
	if s.storage == nil {
		return nil, "", ErrNotFound
	}
	row, err := s.q.GetPublicSiteMedia(ctx, db.GetPublicSiteMediaParams{SiteUuid: site, MediaUuid: id})
	if err != nil {
		return nil, "", mediaDBError(err)
	}
	now := s.now()
	if row.OrganizationStatus != orgctx.StatusActive || (row.AccessStartsAt.Valid && now.Before(row.AccessStartsAt.Time)) || (row.AccessEndsAt.Valid && !now.Before(row.AccessEndsAt.Time)) {
		return nil, "", ErrNotFound
	}
	config := row.PublishedConfig
	if preview != "" {
		if !s.verifyPreview(preview, site) {
			return nil, "", ErrNotFound
		}
		config = row.DraftConfig
	} else if !row.Enabled || !row.PublishedRevision.Valid {
		return nil, "", ErrNotFound
	}
	if !slices.Contains(configMedia(decodeConfig(config)), id.String()) {
		return nil, "", ErrNotFound
	}
	rc, _, err := s.storage.Download(ctx, row.ObjectKey)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return rc, row.Mime, nil
}

// mediaDimensions reads only the image header, without allocating pixel buffers.
func mediaDimensions(data []byte, mime string) (image.Config, error) {
	if mime != "image/webp" {
		c, _, err := image.DecodeConfig(bytes.NewReader(data))
		return c, err
	}
	invalid := errors.New("invalid WebP header")
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || int64(binary.LittleEndian.Uint32(data[4:8]))+8 != int64(len(data)) {
		return image.Config{}, invalid
	}
	for pos := 12; pos+8 <= len(data); {
		kind := string(data[pos : pos+4])
		size := int64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		if int64(start)+size > int64(len(data)) {
			return image.Config{}, invalid
		}
		chunk := data[start : int64(start)+size]
		switch kind {
		case "VP8X":
			if len(chunk) != 10 {
				return image.Config{}, invalid
			}
			w := 1 + int(chunk[4]) + int(chunk[5])<<8 + int(chunk[6])<<16
			h := 1 + int(chunk[7]) + int(chunk[8])<<8 + int(chunk[9])<<16
			return image.Config{Width: w, Height: h}, nil
		case "VP8 ":
			if len(chunk) < 10 || !bytes.Equal(chunk[3:6], []byte{0x9d, 0x01, 0x2a}) {
				return image.Config{}, invalid
			}
			return image.Config{Width: int(binary.LittleEndian.Uint16(chunk[6:8]) & 0x3fff), Height: int(binary.LittleEndian.Uint16(chunk[8:10]) & 0x3fff)}, nil
		case "VP8L":
			if len(chunk) < 5 || chunk[0] != 0x2f {
				return image.Config{}, invalid
			}
			bits := binary.LittleEndian.Uint32(chunk[1:5])
			if bits>>29 != 0 {
				return image.Config{}, invalid
			}
			return image.Config{Width: int(bits&0x3fff) + 1, Height: int((bits>>14)&0x3fff) + 1}, nil
		}
		pos = start + int(size) + (int(size) & 1)
	}
	return image.Config{}, invalid
}
