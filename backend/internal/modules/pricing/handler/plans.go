package handler

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// PlansMeta serves GET /v1/tenant/rate-plans/meta.
func (h *Handler) PlansMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, RatePlansMeta())
}

// PlanFacets serves GET /v1/tenant/rate-plans/facets.
func (h *Handler) PlanFacets(w http.ResponseWriter, r *http.Request) {
	facets, err := h.svc.PlanFacets(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, facets)
}

// ListPlans serves GET /v1/tenant/rate-plans.
func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	scope := orgctx.Must(r.Context())
	q, ok := listQuery(w, r, usecase.RatePlanSortColumns)
	if !ok {
		return
	}
	filters := map[string]string{"q": q.Q}
	for _, key := range model.RatePlanQueryKeys {
		filters[key] = joinValues(r.URL.Query(), key)
	}
	f := model.RatePlanFilterFromQuery(filters)
	f.Limit, f.Offset, f.SortField = q.Limit, q.Offset, "rental_product"
	if len(q.Sort) > 0 {
		f.SortField, f.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	items, total, err := h.svc.ListPlans(r.Context(), scope.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreatePlan serves POST /v1/tenant/rate-plans.
func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	var in model.RatePlanCreateInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	plan, err := h.svc.CreatePlan(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPlan(r, "rate_plans.created", resourceevents.ActionCreated, plan)
	response.JSON(w, r, http.StatusCreated, plan)
}

// GetPlan serves GET /v1/tenant/rate-plans/{uuid}.
func (h *Handler) GetPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	plan, err := h.svc.GetPlan(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, plan)
}

// PatchPlan serves PATCH /v1/tenant/rate-plans/{uuid}.
func (h *Handler) PatchPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.RatePlanPatchInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	plan, err := h.svc.PatchPlan(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPlan(r, "rate_plans.updated", resourceevents.ActionUpdated, plan)
	response.JSON(w, r, http.StatusOK, plan)
}

// DeletePlan serves DELETE /v1/tenant/rate-plans/{uuid}.
func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	if err := h.svc.DeletePlan(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "rate_plans.deleted", model.ResourceRatePlans, id, map[string]any{})
	h.publish(r, model.ResourceRatePlans, resourceevents.ActionDeleted, id)
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "deleted"})
}

// DiscardPlanDraft serves DELETE /v1/tenant/rate-plans/{uuid}/draft.
func (h *Handler) DiscardPlanDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	plan, err := h.svc.DiscardPlanDraft(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPlan(r, "rate_plans.draft_discarded", resourceevents.ActionUpdated, plan)
	response.JSON(w, r, http.StatusOK, plan)
}

// PlanActivationCheck serves GET /v1/tenant/rate-plans/{uuid}/activation-check.
func (h *Handler) PlanActivationCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	check, err := h.svc.PlanActivationCheck(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, check)
}

// ActivatePlan serves POST /v1/tenant/rate-plans/{uuid}/activate.
func (h *Handler) ActivatePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ActivateInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	plan, err := h.svc.ActivatePlan(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPlan(r, "rate_plans.activated", resourceevents.ActionUpdated, plan)
	response.JSON(w, r, http.StatusOK, plan)
}

// DeactivatePlan serves POST /v1/tenant/rate-plans/{uuid}/deactivate.
func (h *Handler) DeactivatePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	plan, err := h.svc.DeactivatePlan(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.recordPlan(r, "rate_plans.deactivated", resourceevents.ActionUpdated, plan)
	response.JSON(w, r, http.StatusOK, plan)
}

// PlanVersions serves GET /v1/tenant/rate-plans/{uuid}/versions.
func (h *Handler) PlanVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	items, err := h.svc.PlanVersions(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// PlanActivity serves GET /v1/tenant/rate-plans/{uuid}/activity.
func (h *Handler) PlanActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	q := apiquery.Parse(r.URL.Query())
	items, total, err := h.svc.PlanActivity(r.Context(), orgctx.Must(r.Context()).ID, id, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

func (h *Handler) recordPlan(r *http.Request, action, event string, p model.RatePlan) {
	payload := map[string]any{
		"rental_product": p.Product.Code, "scope": p.Scope, "status": p.Status, "version": p.CurrentVersion,
		"priority": p.Priority,
	}
	if p.Branch != nil {
		payload["branch"] = p.Branch.Code
	}
	if p.Channel != nil {
		payload["channel"] = *p.Channel
	}
	if p.ActiveVersion != nil {
		payload["active_version"] = *p.ActiveVersion
	}
	h.record(r, action, model.ResourceRatePlans, p.UUID, payload)
	h.publish(r, model.ResourceRatePlans, event, p.UUID)
}

// ------------------------------------------------------------------ matrix

func matrixTarget(w http.ResponseWriter, r *http.Request) (model.MatrixTarget, bool) {
	values := r.URL.Query()
	t := model.MatrixTarget{Scope: strings.TrimSpace(values.Get("scope"))}
	if t.Scope == "" {
		t.Scope = "common"
	}
	details := []response.Detail{}
	if raw := strings.TrimSpace(values.Get("branch")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			details = append(details, response.Detail{Field: "branch", Code: "invalid", Message: "branch is invalid"})
		}
		t.BranchUUID = &id
	}
	if raw := strings.TrimSpace(values.Get("channel")); raw != "" {
		t.Channel = &raw
	}
	p, ok := queryInt(values, "priority", 0)
	if !ok {
		details = append(details, response.Detail{Field: "priority", Code: "invalid", Message: "priority is invalid"})
	}
	t.Priority = p
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return t, false
	}
	return t, true
}

// Matrix serves GET /v1/tenant/rate-plans/matrix?scope=&branch=&channel=&priority=.
func (h *Handler) Matrix(w http.ResponseWriter, r *http.Request) {
	t, ok := matrixTarget(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Matrix(r.Context(), orgctx.Must(r.Context()).ID, t)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, m)
}

// SaveMatrix serves PUT /v1/tenant/rate-plans/matrix.
func (h *Handler) SaveMatrix(w http.ResponseWriter, r *http.Request) {
	var in model.MatrixSaveInput
	if !decodeJSON(w, r, &in, false) {
		return
	}
	res, changed, err := h.svc.SaveMatrix(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	for _, id := range changed {
		h.record(r, "rate_plans.matrix_saved", model.ResourceRatePlans, id, map[string]any{"scope": in.Scope, "priority": in.Priority})
	}
	if len(changed) > 0 {
		h.publish(r, model.ResourceRatePlans, resourceevents.ActionBulk, changed...)
	}
	response.JSON(w, r, http.StatusOK, res)
}

// ActivateMatrix serves POST /v1/tenant/rate-plans/matrix/activate.
func (h *Handler) ActivateMatrix(w http.ResponseWriter, r *http.Request) {
	var in model.MatrixTarget
	if !decodeJSON(w, r, &in, false) {
		return
	}
	res, activated, err := h.svc.ActivateMatrix(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	for _, id := range activated {
		h.record(r, "rate_plans.activated", model.ResourceRatePlans, id, map[string]any{"scope": in.Scope, "via": "matrix"})
	}
	if len(activated) > 0 {
		h.publish(r, model.ResourceRatePlans, resourceevents.ActionBulk, activated...)
	}
	response.JSON(w, r, http.StatusOK, res)
}
