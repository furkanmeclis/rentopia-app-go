// Package handler is the HTTP layer of the campaigns resource (TEC-703).
// Every handler runs behind RequireOrganization and reads the organization
// id from orgctx.Scope; the client never sends it.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves the campaigns routes.
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

// Meta is the table contract of GET /v1/tenant/campaigns. Capabilities
// match the routes: export and bulk are mounted, import is not.
func Meta() resourcemeta.ResourceMeta {
	col := func(key string, typ resourcemeta.ColumnType, sortable, visible bool) resourcemeta.Column {
		return resourcemeta.Column{Key: key, LabelKey: "campaigns.columns." + key, Type: typ, Sortable: sortable, DefaultVisible: visible}
	}
	facet := func(c resourcemeta.Column) resourcemeta.Column {
		c.Filterable, c.FilterVariant = true, resourcemeta.FilterVariantFaceted
		return c
	}
	return resourcemeta.ResourceMeta{
		Resource:      model.Resource,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "name", "code", "campaign_type", "discount_value", "state", "used_count"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: false, Bulk: true,
		},
		SearchableFields: []string{"name", "code", "description"},
		SortableFields:   []string{"name", "code", "start_date", "end_date", "used_count", "created_at", "updated_at"},
		FilterableFields: []string{"campaign_type", "discount_kind", "state", "channel"},
		Columns: []resourcemeta.Column{
			col("name", resourcemeta.ColumnTypeString, true, true),
			col("code", resourcemeta.ColumnTypeString, true, true),
			facet(col("campaign_type", resourcemeta.ColumnTypeEnum, false, true)),
			facet(col("discount_kind", resourcemeta.ColumnTypeEnum, false, false)),
			col("discount_value", resourcemeta.ColumnTypeString, false, true),
			col("start_date", resourcemeta.ColumnTypeDatetime, true, true),
			col("end_date", resourcemeta.ColumnTypeDatetime, true, true),
			col("used_count", resourcemeta.ColumnTypeString, true, true),
			facet(col("state", resourcemeta.ColumnTypeEnum, false, true)),
			col("created_at", resourcemeta.ColumnTypeDatetime, true, false),
			col("uuid", resourcemeta.ColumnTypeUUID, false, false),
		},
		Filters: []resourcemeta.Filter{
			{Key: "campaign_type", LabelKey: "campaigns.columns.campaign_type", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "campaigns.facets.campaign_type"},
			{Key: "discount_kind", LabelKey: "campaigns.columns.discount_kind", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "campaigns.facets.discount_kind"},
			{Key: "state", LabelKey: "campaigns.columns.state", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "campaigns.facets.state"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

// Meta serves GET /v1/tenant/campaigns/meta.
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, Meta())
}

// Facets serves GET /v1/tenant/campaigns/facets.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.Facets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, f)
}

// List serves GET /v1/tenant/campaigns.
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
	f := model.FilterFromQuery(map[string]string{
		"campaign_type": joinValues(values, "campaign_type"), "discount_kind": joinValues(values, "discount_kind"),
		"state": joinValues(values, "state"), "channel": values.Get("channel"),
	})
	f.Q, f.Limit, f.Offset = q.Q, q.Limit, q.Offset
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

// Create serves POST /v1/tenant/campaigns.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in model.CampaignInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.Create(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "campaigns.created", c.UUID, resourceevents.ActionCreated, map[string]any{"name": c.Name, "code": c.Code})
	response.JSON(w, r, http.StatusCreated, c)
}

// Get serves GET /v1/tenant/campaigns/{uuid}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// Patch serves PATCH /v1/tenant/campaigns/{uuid}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.CampaignInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.Patch(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "campaigns.updated", c.UUID, resourceevents.ActionUpdated, map[string]any{"name": c.Name})
	response.JSON(w, r, http.StatusOK, c)
}

// Delete serves DELETE /v1/tenant/campaigns/{uuid}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "campaigns.deleted", id, resourceevents.ActionDeleted, map[string]any{})
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// Activity serves GET /v1/tenant/campaigns/{uuid}/activity.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
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

// Redemptions serves GET /v1/tenant/campaigns/{uuid}/redemptions?status=.
func (h *Handler) Redemptions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	statuses := model.SplitList(joinValues(r.URL.Query(), "status"))
	items, total, err := h.svc.Redemptions(r.Context(), orgctx.Must(r.Context()).ID, id, statuses, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) record(r *http.Request, action string, id uuid.UUID, ev string, payload map[string]any) {
	if h.events != nil {
		h.events.Publish(r.Context(), model.Resource, ev, id.String())
	}
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, model.Resource, &id, payload, r)
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

func pathUUID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "uuid", Message: "uuid is invalid", Code: "invalid"}})
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
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Campaign was not found")
	case errors.Is(err, usecase.ErrCodeTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{
			Field: "code", Message: err.Error(), Code: "taken",
		}})
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}
