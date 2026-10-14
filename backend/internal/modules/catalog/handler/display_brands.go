package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) DisplayBrandsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformCatalogDisplayBrands())
}

func (h *Handler) ListDisplayBrands(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, apiquery.CatalogDisplayBrandsSort)
	if !ok {
		return
	}
	isActive, ok := parseBoolParam(w, r, "is_active")
	if !ok {
		return
	}
	merged, ok := parseBoolParam(w, r, "merged")
	if !ok {
		return
	}
	items, total, err := h.svc.ListDisplayBrands(r.Context(), model.DisplayBrandFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, IsActive: isActive,
		MergedOnly: merged != nil && *merged,
		SortField:  firstSortField(q, "name"), SortDesc: firstSortDesc(q, false),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) GetDisplayBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.GetDisplayBrand(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) PatchDisplayBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct {
		Name     *string `json:"name"`
		IsActive *bool   `json:"is_active"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	d, err := h.svc.PatchDisplayBrand(r.Context(), id, model.PatchDisplayBrandInput{Name: in.Name, IsActive: in.IsActive})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.display_brand_updated", "platform.catalog", &d.UUID, map[string]any{"name": d.Name, "is_active": d.IsActive}, r)
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) MergeDisplayBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct {
		IntoUUID uuid.UUID `json:"into_uuid"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.IntoUUID == uuid.Nil {
		response.BadRequest(w, r, response.CodeValidationError, "into_uuid is required")
		return
	}
	d, err := h.svc.MergeDisplayBrands(r.Context(), id, in.IntoUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.display_brand_merged", "platform.catalog", &d.UUID, map[string]any{"from_uuid": id.String(), "into": d.Name, "tsb_names": d.TSBNames}, r)
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) UnmergeDisplayBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct {
		BrandUUID uuid.UUID `json:"brand_uuid"`
		Name      string    `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	if in.BrandUUID == uuid.Nil {
		response.BadRequest(w, r, response.CodeValidationError, "brand_uuid is required")
		return
	}
	d, err := h.svc.UnmergeBrand(r.Context(), id, in.BrandUUID, in.Name)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.display_brand_unmerged", "platform.catalog", &d.UUID, map[string]any{"from_uuid": id.String(), "name": d.Name, "tsb_names": d.TSBNames}, r)
	}
	response.JSON(w, r, http.StatusCreated, d)
}

func (h *Handler) UploadDisplayBrandLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	// Cap the whole body: ParseMultipartForm only bounds memory.
	r.Body = http.MaxBytesReader(w, r.Body, (3<<20)+(1<<20))
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("logo")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "logo is required")
		return
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	mime, err := storage.DetectLogoMIME(header.Header.Get("Content-Type"), head[:n])
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	if err := storage.ValidateLogoSize(header.Size); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
		return
	}
	body := io.MultiReader(strings.NewReader(string(head[:n])), file)
	d, err := h.svc.SetDisplayBrandLogo(r.Context(), id, storage.File{
		Body: body, Size: header.Size, ContentType: mime, Filename: header.Filename,
	}, ext)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.display_brand_logo_updated", "platform.catalog", &d.UUID, map[string]any{"name": d.Name}, r)
	}
	response.JSON(w, r, http.StatusOK, d)
}

func (h *Handler) DeleteDisplayBrandLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.ClearDisplayBrandLogo(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.display_brand_logo_removed", "platform.catalog", &d.UUID, map[string]any{"name": d.Name}, r)
	}
	response.JSON(w, r, http.StatusOK, d)
}

// StreamDisplayBrandLogo serves a display brand logo to any signed-in user
// (platform admins and tenant vehicle forms).
func (h *Handler) StreamDisplayBrandLogo(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rc, mime, err := h.svc.OpenDisplayBrandLogo(r.Context(), id)
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			response.NotFound(w, r, "logo not set")
			return
		}
		writeError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, rc)
}

func (h *Handler) BulkModelRules(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ModelUUIDs []uuid.UUID `json:"model_uuids"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	res, err := h.svc.BulkModelRules(r.Context(), in.ModelUUIDs)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.model_rules_saved", "platform.catalog", nil, map[string]any{
			"models": len(in.ModelUUIDs), "rules_saved": res.RulesSaved, "types_assigned": res.TypesAssigned,
		}, r)
	}
	response.JSON(w, r, http.StatusOK, res)
}
