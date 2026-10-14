package usecase

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
)

const (
	// MaxProductImages caps the images of one product.
	MaxProductImages = 12
	// MaxProductImageBytes caps one image upload (5 MiB).
	MaxProductImageBytes = 5 << 20
)

var imageMIMEToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

// PublicImagePath is the API-owned public URL of a product image. The
// browser (and later the website / channels) use it as is.
func PublicImagePath(orgUUID, imageUUID uuid.UUID) string {
	return "/v1/public/media/rental-products/" + orgUUID.String() + "/images/" + imageUUID.String()
}

func mapImage(row db.RentalProductImage, orgUUID uuid.UUID) model.ProductImage {
	return model.ProductImage{
		UUID: row.Uuid, PublicURL: PublicImagePath(orgUUID, row.Uuid), Filename: row.Filename,
		ContentType: row.ContentType, SizeBytes: row.SizeBytes, SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt.Time.UTC(),
	}
}

// ImageUpload is one uploaded file.
type ImageUpload struct {
	Body     io.Reader
	Size     int64
	Filename string
}

// UploadImage stores an image of a product (appended last).
func (s *Service) UploadImage(ctx context.Context, orgID int64, productUUID uuid.UUID, in ImageUpload) (model.ProductImage, error) {
	if s.storage == nil {
		return model.ProductImage{}, ErrStorageUnavailable
	}
	product, err := s.repo.GetProduct(ctx, orgID, productUUID)
	if err != nil {
		return model.ProductImage{}, mapNotFound(err, ErrProductNotFound)
	}
	count, err := s.repo.CountImages(ctx, orgID, product.ID)
	if err != nil {
		return model.ProductImage{}, err
	}
	if count >= MaxProductImages {
		return model.ProductImage{}, validation("file", "too_many", "a product can have at most 12 images")
	}
	if in.Size <= 0 {
		return model.ProductImage{}, validation("file", "required", "file is required")
	}
	if in.Size > MaxProductImageBytes {
		return model.ProductImage{}, validation("file", "too_large", "image must be at most 5 MiB")
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.Body, head)
	head = head[:n]
	// Sniff the bytes: a client-declared type must not smuggle HTML / SVG
	// into public storage.
	mime := strings.ToLower(http.DetectContentType(head))
	if i := strings.Index(mime, ";"); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	ext, ok := imageMIMEToExt[mime]
	if !ok {
		return model.ProductImage{}, validation("file", "invalid_type", "image must be JPEG, PNG or WebP")
	}
	orgUUID, err := s.repo.OrganizationUUID(ctx, orgID)
	if err != nil {
		return model.ProductImage{}, err
	}
	imageUUID := uuid.New()
	key := storage.RentalProductImageObjectKey(orgUUID.String(), product.Uuid.String(), imageUUID.String(), ext)
	filename := strings.TrimSpace(in.Filename)
	for utf8.RuneCountInString(filename) > 255 {
		r := []rune(filename)
		filename = string(r[len(r)-255:])
	}
	if err := s.storage.Upload(ctx, storage.File{
		Body: io.MultiReader(bytes.NewReader(head), in.Body), Size: in.Size, ContentType: mime, Filename: filename,
	}, key); err != nil {
		return model.ProductImage{}, err
	}
	row, err := s.repo.CreateImage(ctx, orgID, product.ID, repository.ImageFields{
		UUID: imageUUID, ObjectKey: key, Filename: filename, ContentType: mime, SizeBytes: in.Size,
	})
	if err != nil {
		_ = s.storage.Delete(ctx, key)
		return model.ProductImage{}, err
	}
	_ = s.repo.TouchProduct(ctx, orgID, product.ID)
	return mapImage(row, orgUUID), nil
}

// DeleteImage removes an image of a product (row and object).
func (s *Service) DeleteImage(ctx context.Context, orgID int64, productUUID, imageUUID uuid.UUID) error {
	product, err := s.repo.GetProduct(ctx, orgID, productUUID)
	if err != nil {
		return mapNotFound(err, ErrProductNotFound)
	}
	img, err := s.repo.GetImage(ctx, orgID, product.ID, imageUUID)
	if err != nil {
		return mapNotFound(err, ErrImageNotFound)
	}
	if err := s.repo.DeleteImage(ctx, orgID, product.ID, imageUUID); err != nil {
		return mapNotFound(err, ErrImageNotFound)
	}
	if s.storage != nil {
		_ = s.storage.Delete(ctx, img.ObjectKey)
	}
	_ = s.repo.TouchProduct(ctx, orgID, product.ID)
	return nil
}

// ReorderImages sets the image order of a product. The list must name every
// image of the product exactly once.
func (s *Service) ReorderImages(ctx context.Context, orgID int64, productUUID uuid.UUID, order []uuid.UUID) ([]model.ProductImage, error) {
	product, err := s.repo.GetProduct(ctx, orgID, productUUID)
	if err != nil {
		return nil, mapNotFound(err, ErrProductNotFound)
	}
	images, err := s.repo.ImagesByProducts(ctx, orgID, []int64{product.ID})
	if err != nil {
		return nil, err
	}
	existing := map[uuid.UUID]bool{}
	for _, img := range images[product.ID] {
		existing[img.Uuid] = true
	}
	if len(uniqueUUIDs(order)) != len(order) || len(order) != len(existing) {
		return nil, validation("image_uuids", "invalid", "image_uuids must list every image of the product once")
	}
	for _, id := range order {
		if !existing[id] {
			return nil, validation("image_uuids", "invalid", "image_uuids must list every image of the product once")
		}
	}
	err = s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		for i, id := range order {
			if err := tx.SetImageSort(ctx, orgID, product.ID, id, int32(i)); err != nil {
				return mapNotFound(err, ErrImageNotFound)
			}
		}
		return tx.TouchProduct(ctx, orgID, product.ID)
	})
	if err != nil {
		return nil, err
	}
	p, err := s.GetProduct(ctx, orgID, productUUID)
	if err != nil {
		return nil, err
	}
	return p.Images, nil
}

// PublicImage opens a product image for the public media route.
func (s *Service) PublicImage(ctx context.Context, orgUUID, imageUUID uuid.UUID) (io.ReadCloser, string, int64, error) {
	if s.storage == nil {
		return nil, "", 0, ErrStorageUnavailable
	}
	row, err := s.repo.PublicImage(ctx, orgUUID, imageUUID)
	if err != nil {
		return nil, "", 0, mapNotFound(err, ErrImageNotFound)
	}
	rc, size, err := s.storage.Download(ctx, row.ObjectKey)
	if err != nil {
		return nil, "", 0, ErrImageNotFound
	}
	if size <= 0 {
		size = row.SizeBytes
	}
	return rc, row.ContentType, size, nil
}
