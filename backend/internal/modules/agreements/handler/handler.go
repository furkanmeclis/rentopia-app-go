// Package handler is the HTTP layer of corporate agreements
// (/v1/tenant/agreements). Every handler runs behind RequireOrganization
// and reads the organization from orgctx; path uuids are resolved before
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

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeInvalidState answers an action that does not fit the state.
const CodeInvalidState = "INVALID_STATE"

// Handler serves /v1/tenant/agreements.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func (h *Handler) audit(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
}

func orgID(r *http.Request) int64 { return orgctx.Must(r.Context()).ID }

// Meta serves GET /v1/tenant/agreements/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, AgreementsMeta())
}

// Facets serves GET /v1/tenant/agreements/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/agreements.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(model.Sortable)); err != nil {
		writeError(w, r, err)
		return
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
	items, total, err := h.svc.List(r.Context(), orgID(r), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Create serves POST /v1/tenant/agreements.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.Input
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.created", out.UUID, map[string]any{"code": out.Code, "status": out.Status, "customer": out.Customer.UUID})
	response.JSON(w, r, http.StatusCreated, out)
}

// Get serves GET /v1/tenant/agreements/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Update serves PATCH /v1/tenant/agreements/{uuid}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.Input
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, changed, err := h.svc.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.updated", id, map[string]any{"fields": changed})
	response.JSON(w, r, http.StatusOK, out)
}

// Delete serves DELETE /v1/tenant/agreements/{uuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.deleted", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// SetStatus serves POST /v1/tenant/agreements/{uuid}/status.
func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.StatusInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, from, err := h.svc.SetStatus(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.status_changed", id, map[string]any{"from": from, "to": out.Status, "reason": strings.TrimSpace(in.Reason)})
	response.JSON(w, r, http.StatusOK, out)
}

// AddContact serves POST /v1/tenant/agreements/{uuid}/contacts.
func (h *Handler) AddContact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ContactInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, created, err := h.svc.AddContact(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.contact_added", id, map[string]any{"contact": created, "role": in.Role})
	response.JSON(w, r, http.StatusCreated, out)
}

// RemoveContact serves DELETE /v1/tenant/agreements/{uuid}/contacts/{agreementContactUuid}.
func (h *Handler) RemoveContact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	contact, ok := pathUUID(w, r, "agreementContactUuid")
	if !ok {
		return
	}
	out, err := h.svc.RemoveContact(r.Context(), orgID(r), id, contact)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.contact_removed", id, map[string]any{"contact": contact})
	response.JSON(w, r, http.StatusOK, out)
}

// AddDriver serves POST /v1/tenant/agreements/{uuid}/drivers.
func (h *Handler) AddDriver(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.DriverInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, created, err := h.svc.AddDriver(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.driver_added", id, map[string]any{"driver": created})
	response.JSON(w, r, http.StatusCreated, out)
}

// RemoveDriver serves DELETE /v1/tenant/agreements/{uuid}/drivers/{agreementDriverUuid}.
func (h *Handler) RemoveDriver(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	driver, ok := pathUUID(w, r, "agreementDriverUuid")
	if !ok {
		return
	}
	out, err := h.svc.RemoveDriver(r.Context(), orgID(r), id, driver)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, "agreements.driver_removed", id, map[string]any{"driver": driver})
	response.JSON(w, r, http.StatusOK, out)
}

// Tariffs serves GET /v1/tenant/agreements/{uuid}/tariffs.
func (h *Handler) Tariffs(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Tariffs(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// Rentals serves GET /v1/tenant/agreements/{uuid}/rentals.
func (h *Handler) Rentals(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Rentals(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// Billing serves GET /v1/tenant/agreements/{uuid}/billing.
func (h *Handler) Billing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var rental *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("rental")); raw != "" {
		v, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "rental", Code: "invalid", Message: "rental is invalid"}})
			return
		}
		rental = &v
	}
	out, err := h.svc.Billing(r.Context(), orgID(r), id, rental)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Statement serves GET /v1/tenant/agreements/{uuid}/statement.
func (h *Handler) Statement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Statement(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Activity serves GET /v1/tenant/agreements/{uuid}/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), orgID(r), id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
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
	var se *usecase.StateError
	if errors.As(err, &se) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Agreement was not found")
	case errors.Is(err, usecase.ErrContactNotFound):
		response.NotFound(w, r, "Agreement contact was not found")
	case errors.Is(err, usecase.ErrDriverNotFound):
		response.NotFound(w, r, "Agreement driver was not found")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
