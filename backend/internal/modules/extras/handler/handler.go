// Package handler is the HTTP layer of the extras resource (TEC-702):
// catalog, prices, branch stock, equipment availability and allocations.
// Every handler runs behind RequireOrganization and reads the organization
// id from orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the extras routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	events   resourceevents.Publisher
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// SetResourceEvents publishes live list changes (optional).
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) { h.events = pub }

// Meta serves GET /v1/tenant/extras/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/extras/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, f)
}

// List serves GET /v1/tenant/extras.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.SortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	f := model.FilterFromQuery(map[string]string{
		"extra_type": joinValues(values, "extra_type"), "charge_type": joinValues(values, "charge_type"),
		"status": joinValues(values, "status"), "is_physical": values.Get("is_physical"), "channel": values.Get("channel"),
	})
	f.Q, f.Limit, f.Offset = q.Q, q.Limit, q.Offset
	if raw := strings.TrimSpace(values.Get("is_physical")); raw != "" && f.IsPhysical == nil {
		response.ValidationError(w, r, []response.Detail{{Field: "is_physical", Message: "is_physical must be true or false", Code: "invalid"}})
		return
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/extras.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.ExtraInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	e, err := h.svc.Create(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "extras.created", model.Resource, e.UUID, resourceevents.ActionCreated, map[string]any{"code": e.Code, "name": e.Name})
	response.JSON(w, r, http.StatusCreated, e)
}

// Get serves GET /v1/tenant/extras/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	e, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

// Patch serves PATCH /v1/tenant/extras/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ExtraInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	e, err := h.svc.Patch(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "extras.updated", model.Resource, e.UUID, resourceevents.ActionUpdated, map[string]any{"code": e.Code})
	response.JSON(w, r, http.StatusOK, e)
}

// Delete serves DELETE /v1/tenant/extras/{uuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "extras.deleted", model.Resource, id, resourceevents.ActionDeleted, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/extras/{uuid}/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), orgctx.Must(r.Context()).ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Prices serves GET /v1/tenant/extras/{uuid}/prices.
func (h *Handler) Prices(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	m, err := h.svc.Prices(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, m)
}

// SetPrices serves PUT /v1/tenant/extras/{uuid}/prices.
func (h *Handler) SetPrices(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.PricesInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	m, n, err := h.svc.SetPrices(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"rows": n}
	if in.EffectiveFrom != nil {
		payload["effective_from"] = in.EffectiveFrom.UTC()
	}
	h.record(r, "extras.prices_updated", model.Resource, id, resourceevents.ActionUpdated, payload)
	response.JSON(w, r, http.StatusOK, m)
}

// PriceHistory serves GET /v1/tenant/extras/{uuid}/price-history.
func (h *Handler) PriceHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.PriceHistory(r.Context(), orgctx.Must(r.Context()).ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Stock serves GET /v1/tenant/extras/{uuid}/stock.
func (h *Handler) Stock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.Stock(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SetStock serves PUT /v1/tenant/extras/{uuid}/stock.
func (h *Handler) SetStock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.StockInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	res, err := h.svc.SetStock(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	changes := []map[string]any{}
	for _, it := range in.Items {
		if it.BranchUUID != nil && it.Quantity != nil {
			changes = append(changes, map[string]any{"branch_uuid": it.BranchUUID.String(), "quantity": *it.Quantity})
		}
	}
	h.record(r, "extras.stock_updated", model.Resource, id, resourceevents.ActionUpdated, map[string]any{"items": changes})
	response.JSON(w, r, http.StatusOK, res)
}

// Availability serves POST /v1/tenant/extras/availability.
func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	var in model.AvailabilityInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	res, err := h.svc.Availability(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// ListAllocations serves GET /v1/tenant/equipment-allocations
// ?extra=&branch=&status=&kind=.
func (h *Handler) ListAllocations(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.AllocationSortColumns)
	if !ok {
		return
	}
	values := r.URL.Query()
	f := model.AllocationFilter{
		Limit: q.Limit, Offset: q.Offset, Q: q.Q,
		Statuses: model.SplitList(joinValues(values, "status")), Kinds: model.SplitList(joinValues(values, "kind")),
	}
	if raw := strings.TrimSpace(values.Get("extra")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "extra", Message: "extra is invalid", Code: "invalid"}})
			return
		}
		f.ExtraUUID = &id
	}
	for _, raw := range model.SplitList(joinValues(values, "branch")) {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "branch", Message: "branch is invalid", Code: "invalid"}})
			return
		}
		f.BranchUUIDs = append(f.BranchUUIDs, id)
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListAllocations(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateAllocation serves POST /v1/tenant/equipment-allocations
// (ReserveEquipment).
func (h *Handler) CreateAllocation(w http.ResponseWriter, r *http.Request) {
	var in model.ReserveInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	a, err := h.svc.Reserve(r.Context(), orgctx.Must(r.Context()).ID, in, actorID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordAllocation(r, "extras.allocation_created", a, resourceevents.ActionCreated)
	response.JSON(w, r, http.StatusCreated, a)
}

// GetAllocation serves GET /v1/tenant/equipment-allocations/{uuid}.
func (h *Handler) GetAllocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	a, err := h.svc.GetAllocation(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

// ConfirmAllocation serves POST /v1/tenant/equipment-allocations/{uuid}/confirm.
func (h *Handler) ConfirmAllocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ConfirmInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	a, err := h.svc.Confirm(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordAllocation(r, "extras.allocation_confirmed", a, resourceevents.ActionUpdated)
	response.JSON(w, r, http.StatusOK, a)
}

// ReleaseAllocation serves DELETE /v1/tenant/equipment-allocations/{uuid}.
func (h *Handler) ReleaseAllocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	a, err := h.svc.Release(r.Context(), orgctx.Must(r.Context()).ID, id, "released", actorID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordAllocation(r, "extras.allocation_released", a, resourceevents.ActionUpdated)
	response.JSON(w, r, http.StatusOK, a)
}

// recordAllocation audits an allocation change on its extra and tells the
// allocation lists and the extra's detail.
func (h *Handler) recordAllocation(r *http.Request, action string, a model.Allocation, ev string) {
	h.publish(r, model.ResourceAllocations, ev, a.UUID)
	h.record(r, action, model.Resource, a.Extra.UUID, resourceevents.ActionUpdated, map[string]any{
		"allocation_uuid": a.UUID.String(), "branch_uuid": a.Branch.UUID.String(), "quantity": a.Quantity, "kind": a.Kind,
	})
}

func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, ev string, payload map[string]any) {
	h.publish(r, resource, ev, id)
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func (h *Handler) publish(r *http.Request, resource, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), resource, action, id.String())
}

func actorID(r *http.Request) *int64 {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok || p.UserInternal == 0 {
		return nil
	}
	id := p.UserInternal
	return &id
}

func joinValues(values url.Values, key string) string {
	parts := []string{}
	for _, v := range values[key] {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ",")
}

func listQuery(w http.ResponseWriter, r *http.Request, sortable map[string]string) (apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
		writeError(w, r, err)
		return apiquery.Query{}, false
	}
	return q, true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

// decodeJSON decodes a body; an empty body is allowed when optional.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return true
		}
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		details := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	var se *usecase.StockError
	if errors.As(err, &se) {
		response.ErrorWithData(w, r, http.StatusConflict, response.CodeEquipmentUnavailable,
			"Not enough free units at this branch for the whole range",
			[]response.Detail{{Field: "quantity", Code: "insufficient_stock", Message: se.Error()}},
			map[string]int{"stock": se.Stock, "available": se.Available, "requested": se.Requested})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Extra was not found")
	case errors.Is(err, usecase.ErrAllocationNotFound):
		response.NotFound(w, r, "Equipment allocation was not found")
	case errors.Is(err, usecase.ErrCodeTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "code", Message: err.Error(), Code: "taken",
		}})
	case errors.Is(err, usecase.ErrNotActive), errors.Is(err, usecase.ErrHoldExpired), errors.Is(err, usecase.ErrNotHold):
		code := "not_active"
		switch {
		case errors.Is(err, usecase.ErrHoldExpired):
			code = "hold_expired"
		case errors.Is(err, usecase.ErrNotHold):
			code = "not_hold"
		}
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{Code: code, Message: err.Error()}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
