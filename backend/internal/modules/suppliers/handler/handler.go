// Package handler is the HTTP layer of suppliers (/v1/tenant/suppliers).
// Every handler runs behind RequireOrganization; the organization comes
// from orgctx, never the client.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/suppliers.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

func flat(values url.Values) map[string]string {
	out := map[string]string{}
	for k, vs := range values {
		parts := []string{}
		for _, v := range vs {
			if v = strings.TrimSpace(v); v != "" {
				parts = append(parts, v)
			}
		}
		if len(parts) > 0 {
			out[k] = strings.Join(parts, ",")
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

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
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
			out = append(out, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Supplier was not found")
	case errors.Is(err, usecase.ErrCodeTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{Field: "code", Message: err.Error(), Code: "taken"}})
	case errors.Is(err, usecase.ErrTaxNumberTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{Field: "tax_number", Message: err.Error(), Code: "taken"}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

// Meta serves GET /v1/tenant/suppliers/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/suppliers/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// List serves GET /v1/tenant/suppliers.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return
	}
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(usecase.SortColumns)); err != nil {
		writeError(w, r, err)
		return
	}
	f := adapters.FilterFromQuery(flat(values))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
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

// Create serves POST /v1/tenant/suppliers.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	sp, err := h.svc.Create(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "suppliers.created", sp.UUID, map[string]any{"code": sp.Code, "title": sp.Title})
	response.JSON(w, r, http.StatusCreated, sp)
}

// Get serves GET /v1/tenant/suppliers/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	sp, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, sp)
}

// Patch serves PATCH /v1/tenant/suppliers/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	sp, err := h.svc.Patch(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	fields := []string{}
	raw, _ := json.Marshal(in)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for k, v := range m {
		if v != nil {
			fields = append(fields, k)
		}
	}
	h.record(r, "suppliers.updated", sp.UUID, map[string]any{"code": sp.Code, "fields": fields})
	response.JSON(w, r, http.StatusOK, sp)
}

// Activity serves GET /v1/tenant/suppliers/{uuid}/activity.
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

func col(key string, t resourcemeta.ColumnType, sortable, filterable, visible bool) resourcemeta.Column {
	c := resourcemeta.Column{Key: key, LabelKey: "suppliers.columns." + key, Type: t, Sortable: sortable, Filterable: filterable, DefaultVisible: visible}
	if filterable {
		c.FilterVariant = resourcemeta.FilterVariantFaceted
	}
	return c
}

// Meta is the table contract of GET /v1/tenant/suppliers.
func Meta() resourcemeta.ResourceMeta {
	str, enum, dt, id := resourcemeta.ColumnTypeString, resourcemeta.ColumnTypeEnum, resourcemeta.ColumnTypeDatetime, resourcemeta.ColumnTypeUUID
	facet := func(key, lookup string) resourcemeta.Filter {
		return resourcemeta.Filter{Key: key, LabelKey: "suppliers.columns." + key, Variant: resourcemeta.FilterVariantFaceted, EnumLookup: lookup}
	}
	return resourcemeta.ResourceMeta{
		Resource: model.Resource, DefaultSort: "title",
		DefaultFields: []string{"code", "title", "supplier_type", "tax_number", "phone", "payable", "overdue_amount", "next_due_date", "status"},
		Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true,
			Export: true, Import: true, Bulk: true},
		SearchableFields: []string{"title", "code", "tax_number", "email", "phone", "city", "contacts"},
		SortableFields:   []string{"code", "title", "balance", "payable", "open_amount", "overdue_amount", "next_due_date", "created_at"},
		FilterableFields: []string{"supplier_type", "status", "city", "due_state"},
		Columns: []resourcemeta.Column{
			col("code", str, true, false, true), col("title", str, true, false, true), col("supplier_type", enum, false, true, true),
			col("tax_number", str, false, false, true), col("tax_office", str, false, false, false), col("phone", str, false, false, true),
			col("email", str, false, false, false), col("city", enum, false, true, false), col("payment_term_days", str, false, false, false),
			col("payable", str, true, false, true), col("open_amount", str, true, false, false), col("overdue_amount", str, true, false, true),
			col("due_soon_amount", str, false, false, false), col("next_due_date", dt, true, false, true),
			col("due_state", enum, false, true, false), col("vehicle_count", str, false, false, false),
			col("status", enum, false, true, true), col("created_at", dt, true, false, false), col("uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet("supplier_type", "suppliers.type"), facet("status", "suppliers.status"), facet("city", "suppliers.city"),
			facet("due_state", "suppliers.due_state"),
		},
		Includes: []string{}, BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}
