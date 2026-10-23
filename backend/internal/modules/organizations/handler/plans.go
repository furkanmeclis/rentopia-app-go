package handler

import (
	"errors"
	"net/http"

	orgmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/model"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Activity actions of plan changes (TEC-674).
const (
	ActionPlanUpdated    = "organizations.plan_updated"
	ActionFeatureUpdated = "organizations.feature_updated"
)

// SetPlans attaches plan and feature management (TEC-674).
func (h *Handler) SetPlans(p *orgusecase.Plans) { h.plans = p }

func (h *Handler) plansReady(w http.ResponseWriter, r *http.Request) bool {
	if h.plans == nil {
		response.NotFound(w, r, "Plans are not available")
		return false
	}
	return true
}

func writePlanError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, orgusecase.ErrFeatureNotFound) {
		response.RecordFailure(w, err)
		response.NotFound(w, r, "Feature was not found")
		return
	}
	writeTenantError(w, r, err)
}

// ListPlans serves GET /v1/platform/plans.
func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	if !h.plansReady(w, r) {
		return
	}
	items, err := h.plans.ListPlans(r.Context())
	if err != nil {
		writePlanError(w, r, err)
		return
	}
	n := int32(len(items))
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, int64(n), n, 0))
}

// GetPlan serves GET /v1/platform/organizations/{uuid}/plan.
func (h *Handler) GetPlan(w http.ResponseWriter, r *http.Request) {
	if !h.plansReady(w, r) {
		return
	}
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	out, err := h.plans.Get(r.Context(), id)
	if err != nil {
		writePlanError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// PutPlan serves PUT /v1/platform/organizations/{uuid}/plan.
func (h *Handler) PutPlan(w http.ResponseWriter, r *http.Request) {
	if !h.plansReady(w, r) {
		return
	}
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in orgmodel.PlanInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	out, previous, err := h.plans.SetPlan(r.Context(), id, in, p.UserInternal)
	if err != nil {
		writePlanError(w, r, err)
		return
	}
	h.record(r, ActionPlanUpdated, &id, map[string]any{
		"plan": out.Plan.Code, "previous_plan": previous,
		"period_starts_at": out.PeriodStartsAt, "period_ends_at": out.PeriodEndsAt,
	})
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, out)
}

// PutFeature serves PUT /v1/platform/organizations/{uuid}/features/{key}.
func (h *Handler) PutFeature(w http.ResponseWriter, r *http.Request) {
	if !h.plansReady(w, r) {
		return
	}
	id, ok := parsePathUUID(w, r, "uuid", "organization uuid is invalid")
	if !ok {
		return
	}
	var in orgmodel.FeatureInput
	if err := decodeJSON(w, r, &in); err != nil {
		return
	}
	key := r.PathValue("key")
	p := authctx.MustPrincipal(r.Context())
	out, previous, err := h.plans.SetFeature(r.Context(), id, key, in.Enabled, p.UserInternal)
	if err != nil {
		writePlanError(w, r, err)
		return
	}
	h.record(r, ActionFeatureUpdated, &id, map[string]any{
		"feature": key, "enabled": in.Enabled, "previous": previous,
	})
	h.publishPlatform(r.Context(), resourceevents.ActionUpdated, id)
	response.JSON(w, r, http.StatusOK, out)
}
