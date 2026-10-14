package handler

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// OneWayMeta serves GET /v1/tenant/one-way-rules/meta.
func (h *Handler) OneWayMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, OneWayMeta())
}

// OneWayFacets serves GET /v1/tenant/one-way-rules/facets.
func (h *Handler) OneWayFacets(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	facets, err := h.svc.RuleFacets(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// ListOneWay serves GET /v1/tenant/one-way-rules.
func (h *Handler) ListOneWay(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q, ok := listQuery(w, r, usecase.OneWaySortColumns)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.OneWayQueryKeys {
		filters[key] = joinValues(r.URL.Query(), key)
	}
	f := model.OneWayFilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField, f.SortDesc = q.Limit, q.Offset, "created_at", true
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListRules(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// ResolveOneWay serves GET /v1/tenant/one-way-rules/resolve?from_branch=&to_branch=&rental_product=.
func (h *Handler) ResolveOneWay(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	values := r.URL.Query()
	from, errFrom := uuid.Parse(strings.TrimSpace(values.Get("from_branch")))
	to, errTo := uuid.Parse(strings.TrimSpace(values.Get("to_branch")))
	details := []response.Detail{}
	if errFrom != nil {
		details = append(details, response.Detail{Field: "from_branch", Code: "invalid", Message: "from_branch is invalid"})
	}
	if errTo != nil {
		details = append(details, response.Detail{Field: "to_branch", Code: "invalid", Message: "to_branch is invalid"})
	}
	var product *uuid.UUID
	if raw := strings.TrimSpace(values.Get("rental_product")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			details = append(details, response.Detail{Field: "rental_product", Code: "invalid", Message: "rental_product is invalid"})
		}
		product = &id
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return
	}
	quote, err := h.svc.ResolveOneWay(r.Context(), scope.ID, from, to, product)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, quote)
}

// CreateOneWay serves POST /v1/tenant/one-way-rules.
func (h *Handler) CreateOneWay(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	var in model.OneWayInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	rule, err := h.svc.CreateRule(r.Context(), scope.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordRule(r, "one_way_rules.created", resourceevents.ActionCreated, rule)
	response.JSON(w, r, http.StatusCreated, rule)
}

// GetOneWay serves GET /v1/tenant/one-way-rules/{uuid}.
func (h *Handler) GetOneWay(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rule, err := h.svc.GetRule(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rule)
}

// PatchOneWay serves PATCH /v1/tenant/one-way-rules/{uuid}.
func (h *Handler) PatchOneWay(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.OneWayInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	rule, err := h.svc.PatchRule(r.Context(), scope.ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordRule(r, "one_way_rules.updated", resourceevents.ActionUpdated, rule)
	response.JSON(w, r, http.StatusOK, rule)
}

// DeleteOneWay serves DELETE /v1/tenant/one-way-rules/{uuid}.
func (h *Handler) DeleteOneWay(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "one_way_rules.deleted", model.ResourceOneWayRules, id, map[string]any{})
	h.publish(r, model.ResourceOneWayRules, resourceevents.ActionDeleted, id)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// OneWayActivity serves GET /v1/tenant/one-way-rules/{uuid}/activity.
func (h *Handler) OneWayActivity(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.RuleActivity(r.Context(), scope.ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) recordRule(r *http.Request, action, event string, rule model.OneWayRule) {
	payload := map[string]any{
		"from": rule.FromBranch.Code, "to": rule.ToBranch.Code, "allowed": rule.Allowed, "fee": rule.Fee,
		"is_active": rule.IsActive,
	}
	if rule.Product != nil {
		payload["rental_product"] = rule.Product.Code
	}
	h.record(r, action, model.ResourceOneWayRules, rule.UUID, payload)
	h.publish(r, model.ResourceOneWayRules, event, rule.UUID)
}
