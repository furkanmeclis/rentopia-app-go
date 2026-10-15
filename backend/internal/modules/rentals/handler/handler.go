// Package handler is the HTTP layer of rentals and operation tasks. Every
// handler runs behind RequireOrganization and reads the organization from
// orgctx.Scope; the client never sends it. Path uuids are resolved before
// body validation, so another organization's uuid answers 404.
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeInvalidState answers an action that does not fit the state.
const CodeInvalidState = "INVALID_STATE"

// Handler serves /v1/tenant/rentals and /v1/tenant/operation-tasks.
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

// ---------------------------------------------------------------- rentals

// Meta serves GET /v1/tenant/rentals/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, RentalsMeta())
}

// Facets serves GET /v1/tenant/rentals/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/rentals.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.Sortable)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.QueryKeys {
		filters[key] = joinValues(r.URL.Query(), key)
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

// Create serves POST /v1/tenant/rentals.
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
	h.audit(r, model.ResourceRentals, "rentals.created", res.UUID, map[string]any{"reference": res.Reference})
	response.JSON(w, r, http.StatusCreated, res)
}

// Get serves GET /v1/tenant/rentals/{uuid}.
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

// Events serves GET /v1/tenant/rentals/{uuid}/events.
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

// action decodes the body of a rental action, runs it and audits it.
func action[T any](h *Handler, w http.ResponseWriter, r *http.Request, name string, fn func(id uuid.UUID, in T) (model.Rental, error)) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in T
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := fn(id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceRentals, "rentals."+name, id, map[string]any{"status": res.Status})
	response.JSON(w, r, http.StatusOK, res)
}

// Handover serves POST …/{uuid}/handover.
func (h *Handler) Handover(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "handed_over", func(id uuid.UUID, in model.HandoverInput) (model.Rental, error) {
		return h.svc.Handover(r.Context(), orgID(r), id, in)
	})
}

// Return serves POST …/{uuid}/return.
func (h *Handler) Return(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "returned", func(id uuid.UUID, in model.ReturnInput) (model.Rental, error) {
		return h.svc.Return(r.Context(), orgID(r), id, in)
	})
}

// CompleteReturn serves POST …/{uuid}/complete-return.
func (h *Handler) CompleteReturn(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "return_checked", func(id uuid.UUID, in model.CompleteReturnInput) (model.Rental, error) {
		return h.svc.CompleteReturn(r.Context(), orgID(r), id, in)
	})
}

// Cancel serves POST …/{uuid}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "cancelled", func(id uuid.UUID, in model.ReasonInput) (model.Rental, error) {
		return h.svc.Cancel(r.Context(), orgID(r), id, in.Reason)
	})
}

// Extend serves POST …/{uuid}/extend.
func (h *Handler) Extend(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "extended", func(id uuid.UUID, in model.ExtendInput) (model.Rental, error) {
		return h.svc.Extend(r.Context(), orgID(r), id, in)
	})
}

// Swap serves POST …/{uuid}/swap.
func (h *Handler) Swap(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "vehicle_swapped", func(id uuid.UUID, in model.SwapInput) (model.Rental, error) {
		return h.svc.Swap(r.Context(), orgID(r), id, in)
	})
}

// AddCharge serves POST …/{uuid}/charges.
func (h *Handler) AddCharge(w http.ResponseWriter, r *http.Request) {
	action(h, w, r, "charge_proposed", func(id uuid.UUID, in model.ChargeInput) (model.Rental, error) {
		return h.svc.AddCharge(r.Context(), orgID(r), id, in)
	})
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	chargeID, ok := pathUUID(w, r, "chargeUuid")
	if !ok {
		return
	}
	var in model.DecisionInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.DecideCharge(r.Context(), orgID(r), id, chargeID, approve, in.Note)
	if err != nil {
		writeError(w, r, err)
		return
	}
	name := "rentals.charge_waived"
	if approve {
		name = "rentals.charge_approved"
	}
	h.audit(r, model.ResourceRentals, name, id, map[string]any{"charge_uuid": chargeID})
	response.JSON(w, r, http.StatusOK, res)
}

// OverrideCharge serves PATCH …/{uuid}/charges/{chargeUuid}.
func (h *Handler) OverrideCharge(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	chargeID, ok := pathUUID(w, r, "chargeUuid")
	if !ok {
		return
	}
	var in model.ChargeOverrideInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	res, err := h.svc.OverrideCharge(r.Context(), orgID(r), id, chargeID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceRentals, "rentals.charge_overridden", id, map[string]any{"charge_uuid": chargeID})
	response.JSON(w, r, http.StatusOK, res)
}

// ApproveCharge serves POST …/{uuid}/charges/{chargeUuid}/approve.
func (h *Handler) ApproveCharge(w http.ResponseWriter, r *http.Request) { h.decide(w, r, true) }

// WaiveCharge serves POST …/{uuid}/charges/{chargeUuid}/waive.
func (h *Handler) WaiveCharge(w http.ResponseWriter, r *http.Request) { h.decide(w, r, false) }

// ---------------------------------------------------------------- tasks

// TasksMeta serves GET /v1/tenant/operation-tasks/meta.
func (h *Handler) TasksMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, TasksMeta())
}

// TaskFacets serves GET /v1/tenant/operation-tasks/facets.
func (h *Handler) TaskFacets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.TaskFacets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ListTasks serves GET /v1/tenant/operation-tasks.
func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, model.TaskSortable)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.TaskQueryKeys {
		filters[key] = joinValues(r.URL.Query(), key)
	}
	f := model.TaskFilterFromQuery(filters)
	f.Limit, f.Offset = q.Limit, q.Offset
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

// GetTask serves GET /v1/tenant/operation-tasks/{uuid}.
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

func taskAction[T any](h *Handler, w http.ResponseWriter, r *http.Request, name string, fn func(id uuid.UUID, in T) (model.Task, error)) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in T
	if !decodeJSON(w, r, &in, true) {
		return
	}
	t, err := fn(id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceOperationTasks, "operation_tasks."+name, id, map[string]any{"status": t.Status, "task_type": t.TaskType})
	response.JSON(w, r, http.StatusOK, t)
}

// AssignTask serves POST /v1/tenant/operation-tasks/{uuid}/assign.
func (h *Handler) AssignTask(w http.ResponseWriter, r *http.Request) {
	taskAction(h, w, r, "assigned", func(id uuid.UUID, in model.AssignInput) (model.Task, error) {
		return h.svc.AssignTask(r.Context(), orgID(r), id, in.UserUUID)
	})
}

// StartTask serves POST /v1/tenant/operation-tasks/{uuid}/start.
func (h *Handler) StartTask(w http.ResponseWriter, r *http.Request) {
	taskAction(h, w, r, "started", func(id uuid.UUID, _ struct{}) (model.Task, error) {
		return h.svc.StartTask(r.Context(), orgID(r), id)
	})
}

// CompleteTask serves POST /v1/tenant/operation-tasks/{uuid}/complete.
func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	taskAction(h, w, r, "completed", func(id uuid.UUID, in model.NoteInput) (model.Task, error) {
		return h.svc.FinishTask(r.Context(), orgID(r), id, true, in.Note)
	})
}

// CancelTask serves POST /v1/tenant/operation-tasks/{uuid}/cancel.
func (h *Handler) CancelTask(w http.ResponseWriter, r *http.Request) {
	taskAction(h, w, r, "cancelled", func(id uuid.UUID, in model.NoteInput) (model.Task, error) {
		return h.svc.FinishTask(r.Context(), orgID(r), id, false, in.Note)
	})
}

// ---------------------------------------------------------------- helpers

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
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	if ce, ok := availusecase.IsCapacity(err); ok {
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
	var ce *usecase.ConflictError
	if errors.As(err, &ce) {
		response.ErrorWithData(w, r, http.StatusConflict, ce.Code, ce.Message, []response.Detail{{Code: ce.Detail, Message: ce.Message}}, ce.Data)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Rental was not found")
	case errors.Is(err, usecase.ErrTaskNotFound):
		response.NotFound(w, r, "Operation task was not found")
	case errors.Is(err, usecase.ErrChargeNotFound):
		response.NotFound(w, r, "Charge was not found")
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Missing permission for this action")
	case errors.Is(err, usecase.ErrConcurrent):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
