package handler

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// SegmentsMeta serves GET /v1/tenant/segments/meta.
func (h *Handler) SegmentsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, SegmentsMeta())
}

// SegmentFacets serves GET /v1/tenant/segments/facets.
func (h *Handler) SegmentFacets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.SegmentFacets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// ListSegments serves GET /v1/tenant/segments.
func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q, filters, ok := listQuery(w, r, usecase.SegmentSortColumns, model.SegmentQueryKeys)
	if !ok {
		return
	}
	f := model.SegmentFilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField = q.Limit, q.Offset, "sort_order"
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListSegments(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateSegment serves POST /v1/tenant/segments.
func (h *Handler) CreateSegment(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.SegmentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	seg, err := h.svc.CreateSegment(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "segments.created", model.ResourceSegments, seg.UUID, map[string]any{"code": seg.Code, "name": seg.Name})
	response.JSON(w, r, http.StatusCreated, seg)
}

// GetSegment serves GET /v1/tenant/segments/{uuid}.
func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	seg, err := h.svc.GetSegment(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, seg)
}

// PatchSegment serves PATCH /v1/tenant/segments/{uuid}.
func (h *Handler) PatchSegment(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.SegmentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	seg, err := h.svc.PatchSegment(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	fields := []string{}
	for name, set := range map[string]bool{
		"code": in.Code != nil, "name": in.Name != nil, "description": in.Description != nil,
		"sort_order": in.SortOrder != nil, "is_active": in.IsActive != nil,
	} {
		if set {
			fields = append(fields, name)
		}
	}
	h.record(r, "segments.updated", model.ResourceSegments, seg.UUID, map[string]any{"code": seg.Code, "fields": sorted(fields)})
	response.JSON(w, r, http.StatusOK, seg)
}

// DeleteSegment serves DELETE /v1/tenant/segments/{uuid} (soft delete).
func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteSegment(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "segments.deleted", model.ResourceSegments, id, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// SegmentActivity serves GET /v1/tenant/segments/{uuid}/activity.
func (h *Handler) SegmentActivity(w http.ResponseWriter, r *http.Request) {
	h.activityList(w, r, model.ResourceSegments)
}
