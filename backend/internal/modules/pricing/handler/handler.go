// Package handler is the HTTP layer of the pricing resources (rate plans,
// matrix, date rules, preview). Every handler runs behind
// RequireOrganization and reads the organization id from orgctx.Scope; the
// client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeActivationBlocked is the error code of a refused activation (422);
// details list the issues (field, code, message).
const CodeActivationBlocked = "ACTIVATION_BLOCKED"

// Handler serves the pricing routes.
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

func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func (h *Handler) publish(r *http.Request, resource, action string, ids ...uuid.UUID) {
	if h.events == nil {
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	h.events.Publish(r.Context(), resource, action, out...)
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

// decodeJSON decodes a body; an empty body is allowed when optional.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, optional bool) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
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

func queryInt(values url.Values, key string, def int) (int, bool) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil
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
	var ae *usecase.ActivationError
	if errors.As(err, &ae) {
		details := make([]response.Detail, 0, len(ae.Issues))
		for _, i := range ae.Issues {
			details = append(details, response.Detail{Field: i.Field, Message: i.Message, Code: i.Code})
		}
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, CodeActivationBlocked, "Activation was refused", details)
		return
	}
	var se *usecase.StateError
	if errors.As(err, &se) {
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrPlanNotFound):
		response.NotFound(w, r, "Rate plan was not found")
	case errors.Is(err, usecase.ErrRuleNotFound):
		response.NotFound(w, r, "Date rule was not found")
	case errors.Is(err, usecase.ErrNameTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "name", Message: err.Error(), Code: "taken",
		}})
	case errors.Is(err, usecase.ErrConcurrent):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Code: "concurrent", Message: err.Error(),
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
