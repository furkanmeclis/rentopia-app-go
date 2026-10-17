// Package handler serves staff lead lists and handling actions.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/jackc/pgx/v5/pgtype"
)

type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

func New(s *usecase.Service, a *activity.Recorder) *Handler { return &Handler{svc: s, activity: a} }
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Site lead not found")
	case errors.As(err, &ve):
		response.ValidationError(w, r, ve.Details)
	default:
		response.Internal(w, r, "Site lead operation failed")
	}
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.Get(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	id, ok := httpkit.PathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.Patch
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return
	}
	identity, _ := authctx.PrincipalFrom(r.Context())
	out, err := h.svc.Patch(r.Context(), orgctx.Must(r.Context()).ID, id, identity.UserInternal, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if h.activity != nil {
		h.activity.Record(r.Context(), &identity.UserInternal, "site_leads.updated", model.Resource, &id, map[string]any{"status": out.Status}, r)
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	q := apiquery.Parse(v)
	ve := []response.Detail{}
	for _, key := range apiquery.ForbiddenParams(v) {
		ve = append(ve, response.Detail{Field: key, Code: "forbidden", Message: "use limit/offset/q"})
	}
	if raw := v.Get("sort"); raw != "" && raw != "-created_at" && raw != "created_at" {
		ve = append(ve, response.Detail{Field: "sort", Code: "invalid", Message: "sort by created_at or -created_at"})
	}
	p := db.ListSiteLeadsParams{OrganizationID: orgctx.Must(r.Context()).ID, PageLimit: q.Limit, PageOffset: q.Offset, Q: q.Q, SortDesc: v.Get("sort") != "created_at"}
	for _, f := range []struct {
		key     string
		allowed []string
		target  *pgtype.Text
	}{{"kind", model.Kinds, &p.Kind}, {"status", model.Statuses, &p.Status}} {
		value := strings.TrimSpace(v.Get(f.key))
		if value == "" {
			continue
		}
		if !slices.Contains(f.allowed, value) {
			ve = append(ve, response.Detail{Field: f.key, Code: "invalid", Message: "invalid filter"})
		}
		*f.target = pgtype.Text{String: value, Valid: true}
	}
	for _, f := range []struct {
		key    string
		target *pgtype.Timestamptz
	}{{"date_from", &p.DateFrom}, {"date_to", &p.DateTo}} {
		raw := v.Get(f.key)
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			ve = append(ve, response.Detail{Field: f.key, Code: "invalid", Message: "RFC3339 required"})
			continue
		}
		*f.target = pgtype.Timestamptz{Time: t, Valid: true}
	}
	if p.DateFrom.Valid && p.DateTo.Valid && !p.DateTo.Time.After(p.DateFrom.Time) {
		ve = append(ve, response.Detail{Field: "date_to", Code: "invalid", Message: "must follow date_from"})
	}
	if len(ve) > 0 {
		response.ValidationError(w, r, ve)
		return
	}
	items, total, err := h.svc.List(r.Context(), p)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}
func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	columns := []resourcemeta.Column{}
	for _, key := range []string{"kind", "status", "name", "email", "phone", "company", "city", "created_at"} {
		typ := resourcemeta.ColumnTypeString
		if key == "created_at" {
			typ = resourcemeta.ColumnTypeDatetime
		}
		columns = append(columns, resourcemeta.Column{Key: key, LabelKey: "site_leads.columns." + key, Type: typ, DefaultVisible: true, Sortable: key == "created_at"})
	}
	out := resourcemeta.ResourceMeta{Resource: model.Resource, DefaultSort: "-created_at", DefaultFields: []string{"uuid", "kind", "status", "name", "email", "created_at"},
		Capabilities: resourcemeta.Capabilities{Read: true, Update: true, Filter: true, Search: true, Sort: true}, Columns: columns,
		SearchableFields: []string{"name", "email", "company"}, SortableFields: []string{"created_at"}, FilterableFields: []string{"kind", "status", "date_from", "date_to"},
		Filters: []resourcemeta.Filter{
			{Key: "kind", LabelKey: "site_leads.columns.kind", Variant: resourcemeta.FilterVariantFaceted},
			{Key: "status", LabelKey: "site_leads.columns.status", Variant: resourcemeta.FilterVariantFaceted},
		}, Includes: []string{}, BulkActions: []bulkengine.BulkActionDef{}}
	response.JSON(w, r, http.StatusOK, out)
}
