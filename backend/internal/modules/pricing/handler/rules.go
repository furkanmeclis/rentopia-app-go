package handler

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// RulesMeta serves GET /v1/tenant/date-rules/meta.
func (h *Handler) RulesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, DateRulesMeta())
}

// RuleFacets serves GET /v1/tenant/date-rules/facets.
func (h *Handler) RuleFacets(w http.ResponseWriter, r *http.Request) {
	facets, err := h.svc.RuleFacets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// ListRules serves GET /v1/tenant/date-rules.
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.DateRuleSortColumns)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.DateRuleQueryKeys {
		filters[key] = joinValues(r.URL.Query(), key)
	}
	f := model.DateRuleFilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField = q.Limit, q.Offset, "start_date"
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListRules(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateRule serves POST /v1/tenant/date-rules.
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var in model.DateRuleInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	rule, err := h.svc.CreateRule(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordRule(r, "date_rules.created", resourceevents.ActionCreated, rule)
	response.JSON(w, r, http.StatusCreated, rule)
}

// GetRule serves GET /v1/tenant/date-rules/{uuid}.
func (h *Handler) GetRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rule, err := h.svc.GetRule(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, rule)
}

// PatchRule serves PATCH /v1/tenant/date-rules/{uuid}.
func (h *Handler) PatchRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.DateRuleInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	rule, err := h.svc.PatchRule(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordRule(r, "date_rules.updated", resourceevents.ActionUpdated, rule)
	response.JSON(w, r, http.StatusOK, rule)
}

// DeleteRule serves DELETE /v1/tenant/date-rules/{uuid}.
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeleteRule(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "date_rules.deleted", model.ResourceDateRules, id, map[string]any{})
	h.publish(r, model.ResourceDateRules, resourceevents.ActionDeleted, id)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// DiscardRuleDraft serves DELETE /v1/tenant/date-rules/{uuid}/draft.
func (h *Handler) DiscardRuleDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rule, err := h.svc.DiscardRuleDraft(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordRule(r, "date_rules.draft_discarded", resourceevents.ActionUpdated, rule)
	response.JSON(w, r, http.StatusOK, rule)
}

// RuleActivationCheck serves GET /v1/tenant/date-rules/{uuid}/activation-check.
func (h *Handler) RuleActivationCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	check, err := h.svc.RuleActivationCheck(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, check)
}

// ActivateRule serves POST /v1/tenant/date-rules/{uuid}/activate.
func (h *Handler) ActivateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ActivateInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	rule, err := h.svc.ActivateRule(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordRule(r, "date_rules.activated", resourceevents.ActionUpdated, rule)
	response.JSON(w, r, http.StatusOK, rule)
}

// DeactivateRule serves POST /v1/tenant/date-rules/{uuid}/deactivate.
func (h *Handler) DeactivateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	rule, err := h.svc.DeactivateRule(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordRule(r, "date_rules.deactivated", resourceevents.ActionUpdated, rule)
	response.JSON(w, r, http.StatusOK, rule)
}

// RuleVersions serves GET /v1/tenant/date-rules/{uuid}/versions.
func (h *Handler) RuleVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.RuleVersions(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// RuleActivity serves GET /v1/tenant/date-rules/{uuid}/activity.
func (h *Handler) RuleActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.RuleActivity(r.Context(), orgctx.Must(r.Context()).ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) recordRule(r *http.Request, action, event string, rule model.DateRule) {
	payload := map[string]any{
		"name": rule.Name, "status": rule.Status, "version": rule.CurrentVersion, "kind": rule.Kind,
		"value": rule.Value, "priority": rule.Priority,
	}
	if rule.StartDate != nil && rule.EndDate != nil {
		payload["start_date"], payload["end_date"] = rule.StartDate.String(), rule.EndDate.String()
	}
	h.record(r, action, model.ResourceDateRules, rule.UUID, payload)
	h.publish(r, model.ResourceDateRules, event, rule.UUID)
}

// Preview serves POST /v1/tenant/pricing/preview.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	var in model.PreviewInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	out, err := h.svc.Preview(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
