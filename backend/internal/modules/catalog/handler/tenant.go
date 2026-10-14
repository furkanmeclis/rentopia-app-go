package handler

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Tenant routes are read-only lookups for organization vehicle forms. The
// catalog is platform data: no organization scoping is applied to the rows,
// RequireOrganization only checks that the caller is a member.

func (h *Handler) TenantBrands(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, nil)
	if !ok {
		return
	}
	year, ok := parseYearParam(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.TenantBrands(r.Context(), model.TenantFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Year: year,
		Category: strings.TrimSpace(r.URL.Query().Get("category")),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) TenantModels(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, nil)
	if !ok {
		return
	}
	brand, ok := parseQueryUUID(w, r, "brand")
	if !ok {
		return
	}
	if brand == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "brand", Message: "brand is required", Code: "required"}})
		return
	}
	year, ok := parseYearParam(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.TenantModels(r.Context(), model.TenantFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Year: year, BrandUUID: *brand,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) TenantTypes(w http.ResponseWriter, r *http.Request) {
	q, ok := parseListQuery(w, r, nil)
	if !ok {
		return
	}
	mdl, ok := parseQueryUUID(w, r, "model")
	if !ok {
		return
	}
	if mdl == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "model", Message: "model is required", Code: "required"}})
		return
	}
	year, ok := parseYearParam(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.TenantTypes(r.Context(), model.TenantFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q, Year: year, ModelUUID: *mdl,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}
