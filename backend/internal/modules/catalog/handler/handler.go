// Package handler exposes the vehicle catalog over HTTP: platform curation
// and imports under /v1/platform/catalog, read-only search under
// /v1/tenant/catalog.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the catalog routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// ---------- meta ----------

func (h *Handler) BrandsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformCatalogBrands())
}

func (h *Handler) ModelsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformCatalogModels())
}

func (h *Handler) TypesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformCatalogTypes())
}

func (h *Handler) ImportsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.PlatformCatalogImports())
}

// ---------- brands ----------

func (h *Handler) ListBrands(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, apiquery.CatalogBrandsSort)
	if !ok {
		return
	}
	isActive, ok := parseBoolParam(w, r, "is_active")
	if !ok {
		return
	}
	display, ok := parseQueryUUID(w, r, "display_brand")
	if !ok {
		return
	}
	items, total, err := h.svc.ListBrands(r.Context(), model.BrandFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, DisplayBrandUUID: display,
		Category: strings.TrimSpace(r.URL.Query().Get("category")), IsActive: isActive,
		SortField: firstSortField(q, "display_name"), SortDesc: firstSortDesc(q, false),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) GetBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	b, err := h.svc.GetBrand(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, b)
}

func (h *Handler) PatchBrand(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct {
		DisplayName      *string    `json:"display_name"`
		DisplayBrandUUID *uuid.UUID `json:"display_brand_uuid"`
		Category         *string    `json:"category"`
		IsActive         *bool      `json:"is_active"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	b, err := h.svc.PatchBrand(r.Context(), id, model.PatchBrandInput{
		DisplayName: in.DisplayName, DisplayBrandUUID: in.DisplayBrandUUID, Category: in.Category, IsActive: in.IsActive,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.brand_updated", "platform.catalog", &b.UUID, map[string]any{"display_name": b.DisplayName, "display_brand": b.DisplayBrand.Name, "category": b.Category, "is_active": b.IsActive}, r)
	}
	response.JSON(w, r, http.StatusOK, b)
}

// ---------- models ----------

func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, apiquery.CatalogModelsSort)
	if !ok {
		return
	}
	brand, ok := parseQueryUUID(w, r, "brand")
	if !ok {
		return
	}
	display, ok := parseQueryUUID(w, r, "display_brand")
	if !ok {
		return
	}
	items, total, err := h.svc.ListModels(r.Context(), model.ModelFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, BrandUUID: brand, DisplayBrandUUID: display,
		Source:    strings.TrimSpace(r.URL.Query().Get("source")),
		SortField: firstSortField(q, "brand"), SortDesc: firstSortDesc(q, false),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) GetModel(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	m, err := h.svc.GetModel(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, m)
}

func (h *Handler) CreateModel(w http.ResponseWriter, r *http.Request) {
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
	m, err := h.svc.CreateModel(r.Context(), in.BrandUUID, in.Name)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.model_created", "platform.catalog", &m.UUID, map[string]any{"name": m.Name, "brand": m.Brand.DisplayName}, r)
	}
	response.JSON(w, r, http.StatusCreated, m)
}

func (h *Handler) PatchModel(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct {
		Name   *string `json:"name"`
		Source *string `json:"source"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	m, err := h.svc.PatchModel(r.Context(), id, model.PatchModelInput{Name: in.Name, Source: in.Source})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.model_updated", "platform.catalog", &m.UUID, map[string]any{"name": m.Name, "source": m.Source}, r)
	}
	response.JSON(w, r, http.StatusOK, m)
}

func (h *Handler) MergeModel(w http.ResponseWriter, r *http.Request) {
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
	m, err := h.svc.MergeModel(r.Context(), id, in.IntoUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.model_merged", "platform.catalog", &m.UUID, map[string]any{"from_uuid": id.String(), "into": m.Name}, r)
	}
	response.JSON(w, r, http.StatusOK, m)
}

// ---------- types ----------

func (h *Handler) ListTypes(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, apiquery.CatalogTypesSort)
	if !ok {
		return
	}
	brand, ok := parseQueryUUID(w, r, "brand")
	if !ok {
		return
	}
	mdl, ok := parseQueryUUID(w, r, "model")
	if !ok {
		return
	}
	display, ok := parseQueryUUID(w, r, "display_brand")
	if !ok {
		return
	}
	year, ok := parseYearParam(w, r)
	if !ok {
		return
	}
	isActive, ok := parseBoolParam(w, r, "is_active")
	if !ok {
		return
	}
	unresolved, ok := parseBoolParam(w, r, "unresolved")
	if !ok {
		return
	}
	values := r.URL.Query()
	items, total, err := h.svc.ListTypes(r.Context(), model.TypeFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, BrandUUID: brand, ModelUUID: mdl, Year: year,
		DisplayBrandUUID: display,
		Unresolved:       unresolved != nil && *unresolved, IsActive: isActive,
		Category:     strings.TrimSpace(values.Get("category")),
		Fuel:         strings.TrimSpace(values.Get("fuel")),
		Transmission: strings.TrimSpace(values.Get("transmission")),
		SortField:    firstSortField(q, "brand"), SortDesc: firstSortDesc(q, false),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) GetType(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	t, err := h.svc.GetType(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

func (h *Handler) PatchType(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := decodeJSON(w, r, &raw); err != nil {
		return
	}
	in := model.PatchTypeInput{}
	for key, val := range raw {
		var err error
		switch key {
		case "model_uuid":
			in.SetModel = true
			var s *string
			if err = json.Unmarshal(val, &s); err == nil && s != nil && *s != "" {
				var u uuid.UUID
				if u, err = uuid.Parse(*s); err == nil {
					in.ModelUUID = &u
				}
			}
		case "save_rule":
			err = json.Unmarshal(val, &in.SaveRule)
		case "rule_prefix":
			err = json.Unmarshal(val, &in.RulePrefix)
		case "suggested_transmission":
			in.SetTransmission = true
			err = json.Unmarshal(val, &in.SuggestedTransmission)
		case "suggested_fuel":
			in.SetFuel = true
			err = json.Unmarshal(val, &in.SuggestedFuel)
		default:
			response.ValidationError(w, r, []response.Detail{{Field: key, Message: "unknown field", Code: "unknown"}})
			return
		}
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: key, Message: "invalid value", Code: "invalid"}})
			return
		}
	}
	t, err := h.svc.PatchType(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"raw_name": t.RawName, "save_rule": in.SaveRule}
	if t.Model != nil {
		payload["model"] = t.Model.Name
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), actor(r), "catalog.type_updated", "platform.catalog", &t.UUID, payload, r)
	}
	response.JSON(w, r, http.StatusOK, t)
}

// ---------- imports ----------

func (h *Handler) ListImports(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, apiquery.CatalogImportsSort)
	if !ok {
		return
	}
	items, total, err := h.svc.ListImports(r.Context(), model.ImportFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Status: strings.TrimSpace(r.URL.Query().Get("status")),
		SortField: firstSortField(q, "created_at"), SortDesc: firstSortDesc(q, true),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) GetImport(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	imp, err := h.svc.GetImport(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, imp)
}

func (h *Handler) UploadImport(w http.ResponseWriter, r *http.Request) {
	p := authctx.MustPrincipal(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "invalid multipart form or file too large")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
		response.BadRequest(w, r, response.CodeValidationError, "file must be an .xlsx workbook")
		return
	}
	imp, err := h.svc.Upload(r.Context(), p.UserInternal, header.Filename, file, r.FormValue("period"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, imp)
}

func (h *Handler) ApplyImport(w http.ResponseWriter, r *http.Request) {
	id, ok := parsePathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	imp, err := h.svc.RequestApply(r.Context(), p.UserInternal, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, imp)
}

// ---------- helpers ----------

func actor(r *http.Request) *int64 {
	p := authctx.MustPrincipal(r.Context())
	return &p.UserInternal
}

func parsePathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, name+" is invalid")
		return uuid.Nil, false
	}
	return id, true
}

func parseQueryUUID(w http.ResponseWriter, r *http.Request, name string) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " must be a uuid", Code: "invalid"}})
		return nil, false
	}
	return &id, true
}

func parseBoolParam(w http.ResponseWriter, r *http.Request, name string) (*bool, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " must be true or false", Code: "invalid"}})
		return nil, false
	}
	return &b, true
}

func parseYearParam(w http.ResponseWriter, r *http.Request) (*int32, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("year"))
	if raw == "" {
		return nil, true
	}
	y, err := strconv.Atoi(raw)
	if err != nil || y < 1950 || y > 2100 {
		response.ValidationError(w, r, []response.Detail{{Field: "year", Message: "year must be a model year", Code: "invalid"}})
		return nil, false
	}
	v := int32(y)
	return &v, true
}

func parseListQuery(w http.ResponseWriter, r *http.Request, allowed apiquery.SortColumns) (apiquery.Query, bool) {
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(r.URL.Query())
	if err := apiquery.ValidateSort(q.Sort, allowed); err != nil {
		writeError(w, r, err)
		return apiquery.Query{}, false
	}
	return q, true
}

func firstSortField(q apiquery.Query, def string) string {
	if len(q.Sort) == 0 {
		return def
	}
	return q.Sort[0].Field
}

func firstSortDesc(q apiquery.Query, def bool) bool {
	if len(q.Sort) == 0 {
		return def
	}
	return q.Sort[0].Desc
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, usecase.ErrConflict):
		response.Conflict(w, r, response.CodeConflict, "A record with the same name already exists")
	case errors.Is(err, usecase.ErrImportState):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
