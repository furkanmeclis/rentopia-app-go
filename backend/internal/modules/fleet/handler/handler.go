// Package handler is the HTTP layer of the fleet tenant resources
// (/v1/tenant/segments, /v1/tenant/rental-groups, /v1/tenant/products).
// Every tenant handler runs behind RequireOrganization and reads the
// organization id from orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the fleet resources.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// listQuery parses limit/offset/q/sort and the resource filters. Repeated
// filter params are joined with commas (faceted filters).
func listQuery(w http.ResponseWriter, r *http.Request, sortColumns map[string]string, keys []string) (apiquery.Query, map[string]string, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return apiquery.Query{}, nil, false
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortColumns)); err != nil {
		writeError(w, r, err)
		return q, nil, false
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range keys {
		filters[key] = joinValues(values, key)
	}
	return q, filters, true
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

// record writes a fail-soft activity event; the recorder stamps the
// organization from the request scope.
func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func (h *Handler) activityList(w http.ResponseWriter, r *http.Request, resource string) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.Activity(r.Context(), scope.ID, resource, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func actorID(r *http.Request) int64 {
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return 0
	}
	return p.UserInternal
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
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
	switch {
	case errors.Is(err, usecase.ErrSegmentNotFound):
		response.NotFound(w, r, "Segment was not found")
	case errors.Is(err, usecase.ErrGroupNotFound):
		response.NotFound(w, r, "Rental group was not found")
	case errors.Is(err, usecase.ErrProductNotFound):
		response.NotFound(w, r, "Product was not found")
	case errors.Is(err, usecase.ErrVersionNotFound):
		response.NotFound(w, r, "Product version was not found")
	case errors.Is(err, usecase.ErrImageNotFound):
		response.NotFound(w, r, "Image was not found")
	case errors.Is(err, usecase.ErrCodeTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "code", Message: err.Error(), Code: "taken",
		}})
	case errors.Is(err, usecase.ErrSegmentInUse), errors.Is(err, usecase.ErrGroupInUse):
		response.Conflict(w, r, "IN_USE", err.Error())
	case errors.Is(err, usecase.ErrVersionConflict):
		response.Conflict(w, r, response.CodeConflict, err.Error())
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, "STORAGE_UNAVAILABLE", err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
