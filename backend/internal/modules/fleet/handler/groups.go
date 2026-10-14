package handler

import (
	"net/http"
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// GroupsMeta serves GET /v1/tenant/rental-groups/meta.
func (h *Handler) GroupsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, GroupsMeta())
}

// GroupFacets serves GET /v1/tenant/rental-groups/facets.
func (h *Handler) GroupFacets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.GroupFacets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// ListGroups serves GET /v1/tenant/rental-groups.
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q, filters, ok := listQuery(w, r, usecase.GroupSortColumns, model.GroupQueryKeys)
	if !ok {
		return
	}
	f := model.GroupFilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField, f.SortDesc = q.Limit, q.Offset, "created_at", true
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListGroups(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateGroup serves POST /v1/tenant/rental-groups.
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.RentalGroupInput
	if !decodeJSON(w, r, &in) {
		return
	}
	g, err := h.svc.CreateGroup(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rental_groups.created", model.ResourceRentalGroups, g.UUID, map[string]any{"code": g.Code, "name": g.Name})
	response.JSON(w, r, http.StatusCreated, g)
}

// GetGroup serves GET /v1/tenant/rental-groups/{uuid}.
func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	g, err := h.svc.GetGroup(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, g)
}

// PatchGroup serves PATCH /v1/tenant/rental-groups/{uuid}.
func (h *Handler) PatchGroup(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.RentalGroupInput
	if !decodeJSON(w, r, &in) {
		return
	}
	g, err := h.svc.PatchGroup(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	fields := []string{}
	for name, set := range map[string]bool{
		"code": in.Code != nil, "name": in.Name != nil, "description": in.Description != nil,
		"segment_uuid": in.SegmentUUID != nil, "transmission": in.Transmission.Set, "fuel": in.Fuel.Set,
		"body_type": in.BodyType.Set, "min_seats": in.MinSeats.Set, "min_luggage": in.MinLuggage.Set,
		"air_conditioning": in.AirConditioning != nil, "acriss_code": in.AcrissCode.Set,
		"catalog_model_uuids": in.CatalogModelUUIDs != nil, "catalog_type_uuids": in.CatalogTypeUUIDs != nil,
		"is_active": in.IsActive != nil,
	} {
		if set {
			fields = append(fields, name)
		}
	}
	h.record(r, "rental_groups.updated", model.ResourceRentalGroups, g.UUID, map[string]any{"code": g.Code, "fields": sorted(fields)})
	response.JSON(w, r, http.StatusOK, g)
}

// DeleteGroup serves DELETE /v1/tenant/rental-groups/{uuid} (soft delete).
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteGroup(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rental_groups.deleted", model.ResourceRentalGroups, id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// GroupActivity serves GET /v1/tenant/rental-groups/{uuid}/activity.
func (h *Handler) GroupActivity(w http.ResponseWriter, r *http.Request) {
	h.activityList(w, r, model.ResourceRentalGroups)
}

func sorted(v []string) []string {
	sort.Strings(v)
	return v
}
