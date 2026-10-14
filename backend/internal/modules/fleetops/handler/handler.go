// Package handler is the HTTP layer of the fleet operations resources
// (vehicle blocks, transfers, one-way rules). Every handler runs behind
// RequireOrganization and reads the organization id from orgctx.Scope; the
// client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the fleet operations routes.
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
func (h *Handler) SetResourceEvents(pub resourceevents.Publisher) {
	h.events = pub
}

// record writes a fail-soft activity event on resource / id; the recorder
// stamps the organization from the request scope.
func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

// publish tells open lists of the organization what changed.
func (h *Handler) publish(r *http.Request, resource, action string, id uuid.UUID) {
	if h.events == nil {
		return
	}
	h.events.Publish(r.Context(), resource, action, id.String())
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

func listQuery(w http.ResponseWriter, r *http.Request, sortable map[string]string) (apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	if sortable != nil {
		if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
			writeError(w, r, err)
			return apiquery.Query{}, false
		}
	}
	return q, true
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
	var vve *vehicleusecase.ValidationError
	if errors.As(err, &vve) {
		details := make([]response.Detail, 0, len(vve.Details))
		for _, d := range vve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	if ce, ok := usecase.IsConflict(err); ok {
		details := make([]response.Detail, 0, len(ce.Items)+1)
		for _, item := range ce.Items {
			details = append(details, response.Detail{Field: item.Kind, Code: ce.Code, Message: conflictMessage(item)})
		}
		if len(details) == 0 {
			details = append(details, response.Detail{Code: ce.Code, Message: ce.Message})
		}
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, ce.Message, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrVehicleNotFound), errors.Is(err, vehicleusecase.ErrNotFound):
		response.NotFound(w, r, "Vehicle was not found")
	case errors.Is(err, usecase.ErrBlockNotFound):
		response.NotFound(w, r, "Block was not found")
	case errors.Is(err, usecase.ErrTransferNotFound):
		response.NotFound(w, r, "Transfer was not found")
	case errors.Is(err, usecase.ErrRuleNotFound):
		response.NotFound(w, r, "One-way rule was not found")
	case errors.Is(err, usecase.ErrRouteTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "to_branch_uuid", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

// conflictMessage is "<uuid> <start>/<end|open>" (RFC 3339, UTC).
func conflictMessage(c usecase.Conflict) string {
	end := "open"
	if c.EndsAt != nil {
		end = c.EndsAt.UTC().Format(time.RFC3339)
	}
	return c.UUID.String() + " " + c.StartsAt.UTC().Format(time.RFC3339) + "/" + end
}
