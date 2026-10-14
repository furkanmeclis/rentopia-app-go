package handler

import (
	"io"
	"net/http"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// ProductsMeta serves GET /v1/tenant/products/meta.
func (h *Handler) ProductsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, ProductsMeta())
}

// ProductFacets serves GET /v1/tenant/products/facets.
func (h *Handler) ProductFacets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.ProductFacets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// ListProducts serves GET /v1/tenant/products.
func (h *Handler) ListProducts(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q, filters, ok := listQuery(w, r, usecase.ProductSortColumns, model.ProductQueryKeys)
	if !ok {
		return
	}
	f := model.ProductFilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField, f.SortDesc = q.Limit, q.Offset, "created_at", true
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListProducts(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateProduct serves POST /v1/tenant/products (creates version 1).
func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.RentalProductInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.CreateProduct(r.Context(), scope.ID, actorID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rental_products.created", model.ResourceRentalProducts, p.UUID, map[string]any{
		"code": p.Code, "title": p.Title, "guarantee_type": p.Version.Guarantee.Type, "version": p.CurrentVersion,
	})
	response.JSON(w, r, http.StatusCreated, p)
}

// GetProduct serves GET /v1/tenant/products/{uuid}.
func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.GetProduct(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// PatchProduct serves PATCH /v1/tenant/products/{uuid}. A changed guarantee
// or changed conditions create a new version.
func (h *Handler) PatchProduct(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.RentalProductInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := h.svc.PatchProduct(r.Context(), scope.ID, actorID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	fields := []string{}
	for name, set := range map[string]bool{
		"code": in.Code != nil, "title": in.Title != nil, "description": in.Description != nil,
		"segment_uuid": in.SegmentUUID != nil, "representative_model_uuid": in.RepresentativeModelUUID.Set,
		"representative_type_uuid": in.RepresentativeTypeUUID.Set, "status": in.Status != nil,
		"is_active": in.IsActive != nil, "guarantee": in.Guarantee != nil, "conditions": in.Conditions != nil,
	} {
		if set {
			fields = append(fields, name)
		}
	}
	p := res.Product
	h.record(r, "rental_products.updated", model.ResourceRentalProducts, p.UUID, map[string]any{
		"code": p.Code, "fields": sorted(fields),
	})
	if res.VersionCreated {
		h.record(r, "rental_products.version_created", model.ResourceRentalProducts, p.UUID, map[string]any{
			"version": p.CurrentVersion, "guarantee_type": p.Version.Guarantee.Type, "change_note": p.Version.ChangeNote,
		})
	}
	response.JSON(w, r, http.StatusOK, p)
}

// DeleteProduct serves DELETE /v1/tenant/products/{uuid} (soft delete).
func (h *Handler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteProduct(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rental_products.deleted", model.ResourceRentalProducts, id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// ProductActivity serves GET /v1/tenant/products/{uuid}/activity.
func (h *Handler) ProductActivity(w http.ResponseWriter, r *http.Request) {
	h.activityList(w, r, model.ResourceRentalProducts)
}

// ListVersions serves GET /v1/tenant/products/{uuid}/versions.
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.ListVersions(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetVersion serves GET /v1/tenant/products/{uuid}/versions/{versionUuid}.
func (h *Handler) GetVersion(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	versionID, ok := pathUUID(w, r, "versionUuid")
	if !ok {
		return
	}
	v, err := h.svc.GetVersion(r.Context(), scope.ID, id, versionID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// UploadImage serves POST /v1/tenant/products/{uuid}/images (multipart
// "file"). The response carries the API-owned public_url.
func (h *Handler) UploadImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	// Cap the whole body: ParseMultipartForm only bounds memory.
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxProductImageBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxProductImageBytes); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "image must be at most 5 MiB", Code: "too_large"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	img, err := h.svc.UploadImage(r.Context(), scope.ID, id, usecase.ImageUpload{
		Body: file, Size: header.Size, Filename: header.Filename,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rental_products.image_uploaded", model.ResourceRentalProducts, id, map[string]any{
		"image_uuid": img.UUID.String(), "filename": img.Filename,
	})
	response.JSON(w, r, http.StatusCreated, img)
}

// ReorderImages serves PUT /v1/tenant/products/{uuid}/images/order.
func (h *Handler) ReorderImages(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ImageOrderInput
	if !decodeJSON(w, r, &in) {
		return
	}
	images, err := h.svc.ReorderImages(r.Context(), scope.ID, id, in.ImageUUIDs)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rental_products.images_reordered", model.ResourceRentalProducts, id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]any{"items": images})
}

// DeleteImage serves DELETE /v1/tenant/products/{uuid}/images/{imageUuid}.
func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	imageID, ok := pathUUID(w, r, "imageUuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteImage(r.Context(), scope.ID, id, imageID); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rental_products.image_deleted", model.ResourceRentalProducts, id, map[string]any{"image_uuid": imageID.String()})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// PublicImage serves GET /v1/public/media/rental-products/{orgUuid}/images/{imageUuid}
// (no authentication: product images are shown on websites and channels).
func (h *Handler) PublicImage(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(r.PathValue("orgUuid"))
	if err != nil {
		response.NotFound(w, r, "Image was not found")
		return
	}
	imageID, err := uuid.Parse(r.PathValue("imageUuid"))
	if err != nil {
		response.NotFound(w, r, "Image was not found")
		return
	}
	rc, contentType, size, err := h.svc.PublicImage(r.Context(), orgID, imageID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Image objects are immutable: a replaced image gets a new uuid.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}
