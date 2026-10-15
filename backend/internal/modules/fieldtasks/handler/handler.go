// Package handler is the HTTP layer of field tasks (/v1/tenant/field-tasks)
// and the delivery points picker (/v1/tenant/delivery-points). Every
// handler runs behind RequireOrganization and reads the organization from
// orgctx.Scope; path uuids of another organization answer 404.
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/usecase"
	rentalusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeInvalidState answers an action that does not fit the state.
const CodeInvalidState = "INVALID_STATE"

// Handler serves the field task routes.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

func (h *Handler) audit(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
}

// Meta serves GET /v1/tenant/field-tasks/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/field-tasks/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) filter(w http.ResponseWriter, r *http.Request) (model.Filter, apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return model.Filter{}, apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	sortable := map[string]string{}
	for k := range model.Sortable {
		sortable[k] = k
	}
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
		writeError(w, r, err)
		return model.Filter{}, q, false
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.QueryKeys {
		filters[key] = joinValues(values, key)
	}
	f := model.FilterFromQuery(filters)
	f.Limit, f.Offset = q.Limit, q.Offset
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	return f, q, true
}

// List serves GET /v1/tenant/field-tasks (the board).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	f, q, ok := h.filter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.List(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Mine serves GET /v1/tenant/field-tasks/mine (the caller's own tasks).
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	f, q, ok := h.filter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.Mine(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Assignees serves GET /v1/tenant/field-tasks/assignees.
func (h *Handler) Assignees(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Assignees(r.Context(), orgID(r), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items)
}

// Create serves POST /v1/tenant/field-tasks.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.CreateInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	t, err := h.svc.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "field_tasks.created", t.UUID, map[string]any{"task_type": t.TaskType, "status": t.Status})
	response.JSON(w, r, http.StatusCreated, t)
}

// Get serves GET /v1/tenant/field-tasks/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	t, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// Events serves GET /v1/tenant/field-tasks/{uuid}/events.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.Events(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items)
}

func act[T any](h *Handler, w http.ResponseWriter, r *http.Request, name string, fn func(id uuid.UUID, in T) (model.Task, error)) {
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
	payload := map[string]any{"status": t.Status, "task_type": t.TaskType}
	if t.Assignee != nil {
		payload["assignee_uuid"] = t.Assignee.UUID
	}
	h.audit(r, "field_tasks."+name, id, payload)
	response.JSON(w, r, http.StatusOK, t)
}

// Patch serves PATCH /v1/tenant/field-tasks/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	act(h, w, r, "updated", func(id uuid.UUID, in model.PatchInput) (model.Task, error) {
		return h.svc.Patch(r.Context(), orgID(r), id, in)
	})
}

// Assign serves POST /v1/tenant/field-tasks/{uuid}/assign.
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	act(h, w, r, "assigned", func(id uuid.UUID, in model.AssignInput) (model.Task, error) {
		return h.svc.Assign(r.Context(), orgID(r), id, in.UserUUID)
	})
}

// Start serves POST /v1/tenant/field-tasks/{uuid}/start.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	act(h, w, r, "started", func(id uuid.UUID, _ struct{}) (model.Task, error) {
		return h.svc.Start(r.Context(), orgID(r), id)
	})
}

// ReportIssue serves POST /v1/tenant/field-tasks/{uuid}/report-issue.
func (h *Handler) ReportIssue(w http.ResponseWriter, r *http.Request) {
	act(h, w, r, "issue_reported", func(id uuid.UUID, in model.NoteInput) (model.Task, error) {
		return h.svc.ReportIssue(r.Context(), orgID(r), id, in.Note)
	})
}

// Complete serves POST /v1/tenant/field-tasks/{uuid}/complete.
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	act(h, w, r, "completed", func(id uuid.UUID, in model.NoteInput) (model.Task, error) {
		return h.svc.Complete(r.Context(), orgID(r), id, in.Note)
	})
}

// Cancel serves POST /v1/tenant/field-tasks/{uuid}/cancel.
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	act(h, w, r, "cancelled", func(id uuid.UUID, in model.NoteInput) (model.Task, error) {
		return h.svc.Cancel(r.Context(), orgID(r), id, in.Note)
	})
}

// DeliveryPoints serves GET /v1/tenant/delivery-points[?branch=&product=].
func (h *Handler) DeliveryPoints(w http.ResponseWriter, r *http.Request) {
	var branch, product *uuid.UUID
	for key, dst := range map[string]**uuid.UUID{"branch": &branch, "product": &product} {
		raw := strings.TrimSpace(r.URL.Query().Get(key))
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: key, Message: key + " is invalid", Code: "invalid"}})
			return
		}
		*dst = &id
	}
	items, err := h.svc.DeliveryPoints(r.Context(), orgID(r), branch, product)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, items)
}

// ---------------------------------------------------------------- helpers

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
	var rve *rentalusecase.ValidationError
	if errors.As(err, &rve) {
		out := make([]response.Detail, 0, len(rve.Details))
		for _, d := range rve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var se *usecase.StateError
	if errors.As(err, &se) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
		return
	}
	var rse *rentalusecase.StateError
	if errors.As(err, &rse) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, rse.Message, []response.Detail{{Code: rse.Code, Message: rse.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound), errors.Is(err, rentalusecase.ErrTaskNotFound):
		response.NotFound(w, r, "Field task was not found")
	case errors.Is(err, usecase.ErrForbidden), errors.Is(err, rentalusecase.ErrForbidden):
		response.Forbidden(w, r, "Missing permission for this action")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
