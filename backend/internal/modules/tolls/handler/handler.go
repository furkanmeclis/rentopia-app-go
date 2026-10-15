// Package handler is the HTTP layer of toll passes (TEC-725). Every
// handler runs behind RequireOrganization and reads the organization from
// orgctx.Scope; path uuids are resolved inside the organization (and the
// branch scope) first, so another organization's uuid answers 404.
package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes.
const (
	CodeInvalidState = "INVALID_STATE"
	CodeNoProvider   = "TOLL_PROVIDER_NOT_CONNECTED"
)

// Handler serves /v1/tenant/toll-passes, /v1/tenant/toll-settings and the
// vehicle toll tags.
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

// MetaH serves GET /v1/tenant/toll-passes/meta.
func (h *Handler) MetaH(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/toll-passes/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/toll-passes.
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
	f, err := model.FilterFromQuery(filters)
	if err != nil {
		writeError(w, r, err)
		return
	}
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

// Get serves GET /v1/tenant/toll-passes/{uuid}.
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

// Candidates serves GET /v1/tenant/toll-passes/{uuid}/candidates.
func (h *Handler) Candidates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Candidates(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// decide runs a manual decision (assign / unassign / ignore / rematch).
func decide[T any](h *Handler, w http.ResponseWriter, r *http.Request, action string, fn func(id uuid.UUID, in T) (model.PassDetail, error), reason func(T) string) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	// The pass must exist in the organization and scope before the body
	// is looked at (another organization's uuid → 404).
	if _, err := h.svc.Get(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	var in T
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := fn(id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"reason": strings.TrimSpace(reason(in)), "status": out.Status, "external_id": out.ExternalID}
	if out.Rental != nil {
		payload["rental_uuid"], payload["rental_reference"] = out.Rental.UUID, out.Rental.Reference
	}
	h.audit(r, model.Resource, action, id, payload)
	response.JSON(w, r, http.StatusOK, out)
}

// Assign serves POST /v1/tenant/toll-passes/{uuid}/assign.
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	decide(h, w, r, "tolls.assigned", func(id uuid.UUID, in model.AssignInput) (model.PassDetail, error) {
		return h.svc.Assign(r.Context(), orgID(r), id, in)
	}, func(in model.AssignInput) string { return in.Reason })
}

// Unassign serves POST /v1/tenant/toll-passes/{uuid}/unassign.
func (h *Handler) Unassign(w http.ResponseWriter, r *http.Request) {
	decide(h, w, r, "tolls.unassigned", func(id uuid.UUID, in model.ReasonInput) (model.PassDetail, error) {
		return h.svc.Unassign(r.Context(), orgID(r), id, in)
	}, func(in model.ReasonInput) string { return in.Reason })
}

// Ignore serves POST /v1/tenant/toll-passes/{uuid}/ignore.
func (h *Handler) Ignore(w http.ResponseWriter, r *http.Request) {
	decide(h, w, r, "tolls.ignored", func(id uuid.UUID, in model.ReasonInput) (model.PassDetail, error) {
		return h.svc.Ignore(r.Context(), orgID(r), id, in)
	}, func(in model.ReasonInput) string { return in.Reason })
}

// Rematch serves POST /v1/tenant/toll-passes/{uuid}/rematch.
func (h *Handler) Rematch(w http.ResponseWriter, r *http.Request) {
	decide(h, w, r, "tolls.rematched", func(id uuid.UUID, in model.ReasonInput) (model.PassDetail, error) {
		return h.svc.Rematch(r.Context(), orgID(r), id, in)
	}, func(in model.ReasonInput) string { return in.Reason })
}

// Fetch serves POST /v1/tenant/toll-passes/fetch ("Şimdi sorgula").
func (h *Handler) Fetch(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.FetchNow(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.Resource, "tolls.fetch_requested", out.JobUUID, map[string]any{"from": out.From, "to": out.To})
	response.JSON(w, r, http.StatusAccepted, out)
}

// Settings serves GET /v1/tenant/toll-settings.
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Settings(r.Context(), orgID(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UpdateSettings serves PATCH /v1/tenant/toll-settings.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in model.SettingsInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.UpdateSettings(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.Resource, "tolls.settings_updated", orgctx.Must(r.Context()).UUID, map[string]any{
		"processing_fee": out.ProcessingFee.String(), "auto_fetch": out.AutoFetch, "lookback_days": out.LookbackDays,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// Tags serves GET /v1/tenant/vehicles/{uuid}/toll-tags.
func (h *Handler) Tags(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Tags(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}

// CreateTag serves POST /v1/tenant/vehicles/{uuid}/toll-tags.
func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if _, err := h.svc.Tags(r.Context(), orgID(r), id); err != nil {
		writeError(w, r, err)
		return
	}
	var in model.TagInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.CreateTag(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceVehicles, "vehicles.toll_tag_created", id, map[string]any{
		"tag_uuid": out.UUID, "tag_number": out.TagNumber, "provider": out.Provider,
	})
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdateTag serves PATCH /v1/tenant/vehicles/{uuid}/toll-tags/{tagUuid}.
func (h *Handler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	tagID, ok := pathUUID(w, r, "tagUuid")
	if !ok {
		return
	}
	var in model.TagInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.UpdateTag(r.Context(), orgID(r), id, tagID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceVehicles, "vehicles.toll_tag_updated", id, map[string]any{
		"tag_uuid": out.UUID, "tag_number": out.TagNumber, "valid_to": out.ValidTo,
	})
	response.JSON(w, r, http.StatusOK, out)
}

// DeleteTag serves DELETE /v1/tenant/vehicles/{uuid}/toll-tags/{tagUuid}.
func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	tagID, ok := pathUUID(w, r, "tagUuid")
	if !ok {
		return
	}
	out, err := h.svc.DeleteTag(r.Context(), orgID(r), id, tagID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.audit(r, model.ResourceVehicles, "vehicles.toll_tag_deleted", id, map[string]any{
		"tag_uuid": out.UUID, "tag_number": out.TagNumber,
	})
	w.WriteHeader(http.StatusNoContent)
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
	var qe *model.QueryError
	if errors.As(err, &qe) {
		response.ValidationError(w, r, []response.Detail{{Field: qe.Field, Message: qe.Error(), Code: "invalid"}})
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
	var ce *usecase.ConflictError
	if errors.As(err, &ce) {
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, ce.Message,
			[]response.Detail{{Field: ce.Field, Code: ce.Code, Message: ce.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Toll pass was not found")
	case errors.Is(err, usecase.ErrVehicleNotFound):
		response.NotFound(w, r, "Vehicle was not found")
	case errors.Is(err, usecase.ErrTagNotFound):
		response.NotFound(w, r, "Toll tag was not found")
	case errors.Is(err, usecase.ErrNoAccount):
		response.Conflict(w, r, CodeNoProvider, "No active toll provider account is connected")
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
