// Package handler is the HTTP layer of reservations, quotes and resolution
// tasks. Every handler runs behind RequireOrganization and reads the
// organization from orgctx.Scope; the client never sends it. Lookups of a
// path uuid happen before body validation, so another organization's uuid
// answers 404 whatever the body.
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Response codes of the reservation flow.
const (
	CodeInvalidState      = "INVALID_STATE"
	CodePricing           = "PRICING_UNAVAILABLE"
	CodeDriverNotEligible = "DRIVER_NOT_ELIGIBLE"
	CodeExtensionConflict = "EXTENSION_CONFLICT"
)

// Handler serves /v1/tenant/reservations, /quotes and /resolution-tasks.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) audit(r *http.Request, resource, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

// ------------------------------------------------------------ reservations

// Meta serves GET /v1/tenant/reservations/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, ReservationsMeta())
}

// Facets serves GET /v1/tenant/reservations/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/reservations.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.Sortable)
	if !ok {
		return
	}
	values := r.URL.Query()
	filters := map[string]string{"q": q.Q}
	for _, key := range model.QueryKeys {
		filters[key] = joinValues(values, key)
	}
	f := model.FilterFromQuery(filters)
	f.Limit, f.Offset = q.Limit, q.Offset
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.List(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/reservations (from a quote).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.CreateInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	res, err := h.svc.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceReservations, "reservations.created", res.UUID, map[string]any{"reference": res.Reference, "status": res.Status})
	response.JSON(w, r, http.StatusCreated, res)
}

// Get serves GET /v1/tenant/reservations/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	res, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, res)
}

// Events serves GET /v1/tenant/reservations/{uuid}/events.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Events(r.Context(), orgID(r), id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// transitionHandler serves the body-less transitions (submit, approve,
// confirm).
func (h *Handler) transitionHandler(action string, fn func(*usecase.Service) func(r *http.Request, id uuid.UUID) (model.Reservation, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathUUID(w, r, "uuid")
		if !ok {
			return
		}
		var in struct{}
		if !decodeJSON(w, r, &in, true) {
			return
		}
		res, err := fn(h.svc)(r, id)
		if err != nil {
			writeError(w, r, err)
			return
		}
		h.audit(r, model.ResourceReservations, "reservations."+action, id, map[string]any{"status": res.Status})
		response.JSON(w, r, http.StatusOK, res)
	}
}

// Submit serves POST …/{uuid}/submit.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	h.transitionHandler("submitted", func(s *usecase.Service) func(*http.Request, uuid.UUID) (model.Reservation, error) {
		return func(r *http.Request, id uuid.UUID) (model.Reservation, error) {
			return s.Submit(r.Context(), orgID(r), id)
		}
	})(w, r)
}

// Approve serves POST …/{uuid}/approve.
func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.transitionHandler("approved", func(s *usecase.Service) func(*http.Request, uuid.UUID) (model.Reservation, error) {
		return func(r *http.Request, id uuid.UUID) (model.Reservation, error) {
			return s.Approve(r.Context(), orgID(r), id)
		}
	})(w, r)
}

// Confirm serves POST …/{uuid}/confirm.
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	h.transitionHandler("confirmed", func(s *usecase.Service) func(*http.Request, uuid.UUID) (model.Reservation, error) {
		return func(r *http.Request, id uuid.UUID) (model.Reservation, error) {
			return s.Confirm(r.Context(), orgID(r), id)
		}
	})(w, r)
}

// Cancel serves POST …/{uuid}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.reasonAction(w, r, "cancelled", func(r *http.Request, id uuid.UUID, reason string) (model.Reservation, error) {
		return h.svc.Cancel(r.Context(), orgID(r), id, reason)
	})
}

// NoShow serves POST …/{uuid}/no-show.
func (h *Handler) NoShow(w http.ResponseWriter, r *http.Request) {
	h.reasonAction(w, r, "no_show", func(r *http.Request, id uuid.UUID, reason string) (model.Reservation, error) {
		return h.svc.NoShow(r.Context(), orgID(r), id, reason)
	})
}

func (h *Handler) reasonAction(w http.ResponseWriter, r *http.Request, action string, fn func(*http.Request, uuid.UUID, string) (model.Reservation, error)) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ReasonInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := fn(r, id, in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceReservations, "reservations."+action, id, map[string]any{"reason": strings.TrimSpace(in.Reason)})
	response.JSON(w, r, http.StatusOK, res)
}

// ChangeQuote serves POST …/{uuid}/quotes.
func (h *Handler) ChangeQuote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ChangeQuoteInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	q, err := h.svc.ChangeQuote(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, q)
}

// ApplyChange serves POST …/{uuid}/change.
func (h *Handler) ApplyChange(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ApplyChangeInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.ApplyChange(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceReservations, "reservations.changed", id, map[string]any{"quote_uuid": in.QuoteUUID, "keep_price": in.KeepPrice})
	response.JSON(w, r, http.StatusOK, res)
}

// Extend serves POST …/{uuid}/extend.
func (h *Handler) Extend(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ExtendInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.Extend(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceReservations, "reservations.extended", id, map[string]any{"ends_at": in.EndsAt})
	response.JSON(w, r, http.StatusOK, res)
}

// PlateCandidates serves GET …/{uuid}/plate-candidates.
func (h *Handler) PlateCandidates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.PlateCandidates(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Plate serves POST …/{uuid}/plate.
func (h *Handler) Plate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.PlateInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.Plate(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceReservations, "reservations.plate_changed", id, map[string]any{"fix": in.Fix, "source": in.Source})
	response.JSON(w, r, http.StatusOK, res)
}

// Drivers serves PUT …/{uuid}/drivers.
func (h *Handler) Drivers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.DriversInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.UpdateDrivers(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceReservations, "reservations.drivers_changed", id, map[string]any{"count": len(in.Drivers)})
	response.JSON(w, r, http.StatusOK, res)
}

// Assign serves POST …/{uuid}/assign.
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AssignInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.Assign(r.Context(), orgID(r), id, in.UserUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceReservations, "reservations.assigned", id, map[string]any{"user_uuid": in.UserUUID})
	response.JSON(w, r, http.StatusOK, res)
}

// ------------------------------------------------------------------ quotes

// QuotesMeta serves GET /v1/tenant/quotes/meta.
func (h *Handler) QuotesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, QuotesMeta())
}

// ListQuotes serves GET /v1/tenant/quotes.
func (h *Handler) ListQuotes(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.QuoteSortable)
	if !ok {
		return
	}
	values := r.URL.Query()
	f := model.QuoteFilter{
		Purposes: allowed(splitList(joinValues(values, "purpose")), model.QuotePurposes),
		Statuses: allowed(splitList(joinValues(values, "status")), model.QuoteStatuses),
		Channels: splitList(joinValues(values, "channel")), ProductUUIDs: splitUUIDs(joinValues(values, "product")),
		ReservationUUIDs: splitUUIDs(joinValues(values, "reservation")), Q: q.Q, Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListQuotes(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateQuote serves POST /v1/tenant/quotes.
func (h *Handler) CreateQuote(w http.ResponseWriter, r *http.Request) {
	var in model.QuoteInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	q, err := h.svc.CreateQuote(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceQuotes, "quotes.created", q.UUID, map[string]any{"total": q.Total.String(), "product": q.Product.Code})
	response.JSON(w, r, http.StatusCreated, q)
}

// SearchQuotes serves POST /v1/tenant/quotes/search.
func (h *Handler) SearchQuotes(w http.ResponseWriter, r *http.Request) {
	var in model.QuoteSearchInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.Search(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// GetQuote serves GET /v1/tenant/quotes/{uuid}.
func (h *Handler) GetQuote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q, err := h.svc.GetQuote(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, q)
}

// -------------------------------------------------------- resolution tasks

// TasksMeta serves GET /v1/tenant/resolution-tasks/meta.
func (h *Handler) TasksMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, TasksMeta())
}

// ListTasks serves GET /v1/tenant/resolution-tasks.
func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.TaskSortable)
	if !ok {
		return
	}
	values := r.URL.Query()
	f := model.TaskFilter{
		Statuses:         allowed(splitList(joinValues(values, "status")), model.TaskStatuses),
		TaskTypes:        allowed(splitList(joinValues(values, "task_type")), model.TaskTypes),
		ReservationUUIDs: splitUUIDs(joinValues(values, "reservation")),
		AssigneeUUIDs:    splitUUIDs(joinValues(values, "assignee")),
		Q:                q.Q, Limit: q.Limit, Offset: q.Offset,
	}
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListTasks(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetTask serves GET /v1/tenant/resolution-tasks/{uuid}.
func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	t, err := h.svc.GetTask(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// ResolveTask serves POST /v1/tenant/resolution-tasks/{uuid}/resolve.
func (h *Handler) ResolveTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ResolveInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	t, err := h.svc.ResolveTask(r.Context(), orgID(r), id, in.Note)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceTasks, "resolution_tasks.resolved", id, map[string]any{"note": in.Note})
	response.JSON(w, r, http.StatusOK, t)
}

// AssignTask serves POST /v1/tenant/resolution-tasks/{uuid}/assign.
func (h *Handler) AssignTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AssignInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	t, err := h.svc.AssignTask(r.Context(), orgID(r), id, in.UserUUID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceTasks, "resolution_tasks.assigned", id, map[string]any{"user_uuid": in.UserUUID})
	response.JSON(w, r, http.StatusOK, t)
}

// ReplanTask serves POST /v1/tenant/resolution-tasks/{uuid}/replan.
func (h *Handler) ReplanTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in struct{}
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.ReplanTask(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceTasks, "resolution_tasks.replan", id, map[string]any{"replanned": out.Replanned})
	response.JSON(w, r, http.StatusOK, out)
}

// ----------------------------------------------------------------- helpers

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

func joinValues(values url.Values, key string) string {
	parts := []string{}
	for _, v := range values[key] {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ",")
}

func splitList(raw string) []string {
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func splitUUIDs(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range splitList(raw) {
		if id, err := uuid.Parse(v); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func allowed(values, set []string) []string {
	out := []string{}
	for _, v := range values {
		for _, a := range set {
			if v == a {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

// decodeJSON decodes a strict JSON body; optional bodies may be empty.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	if optional && len(bytes.TrimSpace(raw)) == 0 {
		return true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func details(ds []usecase.Detail) []response.Detail {
	out := make([]response.Detail, 0, len(ds))
	for _, d := range ds {
		out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
	}
	return out
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		out := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		response.ValidationError(w, r, details(ve.Details))
		return
	}
	if ce, ok := availabilityCapacity(err); ok {
		response.ErrorWithData(w, r, http.StatusConflict, response.CodeCapacityUnavailable,
			"No vehicle can serve this request for the whole range",
			[]response.Detail{{Code: ce.Reason, Message: ce.Reason}}, ce.Alternatives)
		return
	}
	var se *usecase.StateError
	if errors.As(err, &se) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
		return
	}
	var qe *usecase.QuoteError
	if errors.As(err, &qe) {
		response.ErrorWithData(w, r, http.StatusConflict, qe.Code, qe.Message, []response.Detail{{Field: "quote_uuid", Code: strings.ToLower(qe.Code), Message: qe.Message}}, qe.Replacement)
		return
	}
	var pe *usecase.PricingError
	if errors.As(err, &pe) {
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodePricing, pe.Message, []response.Detail{{Code: pe.Code, Message: pe.Message}})
		return
	}
	var ee *usecase.EligibilityError
	if errors.As(err, &ee) {
		out := []response.Detail{}
		for _, d := range ee.Drivers {
			for _, reason := range d.Reasons {
				out = append(out, response.Detail{Field: "drivers", Code: reason, Message: d.Name + ": " + reason})
			}
		}
		response.ErrorWithData(w, r, http.StatusUnprocessableEntity, CodeDriverNotEligible, ee.Error(), out, ee.Drivers)
		return
	}
	var xe *usecase.ExtensionConflictError
	if errors.As(err, &xe) {
		response.ErrorWithData(w, r, http.StatusConflict, CodeExtensionConflict, xe.Error(),
			[]response.Detail{{Code: "extension_conflict", Message: xe.Error()}}, xe.Alternatives)
		return
	}
	var eq *model.EquipmentError
	if errors.As(err, &eq) {
		response.ErrorWithData(w, r, http.StatusConflict, response.CodeEquipmentUnavailable, "Not enough equipment in stock",
			[]response.Detail{{Field: "extras", Code: "equipment_unavailable", Message: eq.ExtraUUID.String()}},
			map[string]any{"extra_uuid": eq.ExtraUUID, "stock": eq.Stock, "available": eq.Available, "requested": eq.Requested})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Reservation was not found")
	case errors.Is(err, usecase.ErrQuoteNotFound):
		response.NotFound(w, r, "Quote was not found")
	case errors.Is(err, usecase.ErrTaskNotFound):
		response.NotFound(w, r, "Resolution task was not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Missing permission for this action")
	case errors.Is(err, usecase.ErrConcurrent):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrExternalReferenceTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "external_reference", Code: "taken", Message: err.Error(),
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
