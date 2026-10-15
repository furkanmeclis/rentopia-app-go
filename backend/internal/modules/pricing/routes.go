// Package pricing is the tenant pricing domain of M2: rate plans with
// duration bands and branch / channel overrides (TEC-700,
// /v1/tenant/rate-plans), named date rules (TEC-701, /v1/tenant/date-rules)
// and the price preview (/v1/tenant/pricing/preview) over the pure engine
// (internal/platform/pricing, TEC-699).
//
// Permissions: tenant.pricing.read (lists, details, matrix, preview,
// export), tenant.pricing.write (drafts, matrix save, import, delete),
// tenant.pricing.activate (activate / deactivate; separate so staff who
// prepare prices cannot put them live).
package pricing

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	pricinghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Engine resource slugs.
const (
	ResourceRatePlans = model.ResourceRatePlans
	ResourceDateRules = model.ResourceDateRules
)

// Export query keys per resource (the list filters an export honours).
var (
	RatePlanExportKeys = model.RatePlanQueryKeys
	DateRuleExportKeys = model.DateRuleQueryKeys
)

// EngineHandlers are the shared engine handlers the resources mount their
// export / import routes on. Nil members skip those routes.
type EngineHandlers struct {
	ExportPlans http.HandlerFunc
	ImportPlans http.HandlerFunc
	SamplePlans http.HandlerFunc
	ExportRules http.HandlerFunc
}

// RegisterRoutes mounts the pricing routes. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *pricinghandler.Handler,
	engines EngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantPricingRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantPricingWrite)
	activate := middleware.RequireTenantPermission(rbac.PermTenantPricingActivate)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	optional := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		if fn != nil {
			route(pattern, fn, guards...)
		}
	}

	// Rate plans. Static segments before /{uuid}.
	route("GET /v1/tenant/rate-plans/meta", h.PlansMeta, read)
	route("GET /v1/tenant/rate-plans/facets", h.PlanFacets, read)
	route("GET /v1/tenant/rate-plans/matrix", h.Matrix, read)
	route("PUT /v1/tenant/rate-plans/matrix", h.SaveMatrix, read, write)
	route("POST /v1/tenant/rate-plans/matrix/activate", h.ActivateMatrix, read, activate)
	route("GET /v1/tenant/rate-plans", h.ListPlans, read)
	route("POST /v1/tenant/rate-plans", h.CreatePlan, read, write)
	optional("POST /v1/tenant/rate-plans/export", engines.ExportPlans, read)
	optional("POST /v1/tenant/rate-plans/import", engines.ImportPlans, read, write)
	optional("GET /v1/tenant/rate-plans/import/sample", engines.SamplePlans, read, write)
	route("GET /v1/tenant/rate-plans/{uuid}", h.GetPlan, read)
	route("PATCH /v1/tenant/rate-plans/{uuid}", h.PatchPlan, read, write)
	route("DELETE /v1/tenant/rate-plans/{uuid}", h.DeletePlan, read, write)
	route("DELETE /v1/tenant/rate-plans/{uuid}/draft", h.DiscardPlanDraft, read, write)
	route("GET /v1/tenant/rate-plans/{uuid}/activation-check", h.PlanActivationCheck, read)
	route("POST /v1/tenant/rate-plans/{uuid}/activate", h.ActivatePlan, read, activate)
	route("POST /v1/tenant/rate-plans/{uuid}/deactivate", h.DeactivatePlan, read, activate)
	route("GET /v1/tenant/rate-plans/{uuid}/versions", h.PlanVersions, read)
	route("GET /v1/tenant/rate-plans/{uuid}/activity", h.PlanActivity, read)

	// Date rules.
	route("GET /v1/tenant/date-rules/meta", h.RulesMeta, read)
	route("GET /v1/tenant/date-rules/facets", h.RuleFacets, read)
	route("GET /v1/tenant/date-rules", h.ListRules, read)
	route("POST /v1/tenant/date-rules", h.CreateRule, read, write)
	optional("POST /v1/tenant/date-rules/export", engines.ExportRules, read)
	route("GET /v1/tenant/date-rules/{uuid}", h.GetRule, read)
	route("PATCH /v1/tenant/date-rules/{uuid}", h.PatchRule, read, write)
	route("DELETE /v1/tenant/date-rules/{uuid}", h.DeleteRule, read, write)
	route("DELETE /v1/tenant/date-rules/{uuid}/draft", h.DiscardRuleDraft, read, write)
	route("GET /v1/tenant/date-rules/{uuid}/activation-check", h.RuleActivationCheck, read)
	route("POST /v1/tenant/date-rules/{uuid}/activate", h.ActivateRule, read, activate)
	route("POST /v1/tenant/date-rules/{uuid}/deactivate", h.DeactivateRule, read, activate)
	route("GET /v1/tenant/date-rules/{uuid}/versions", h.RuleVersions, read)
	route("GET /v1/tenant/date-rules/{uuid}/activity", h.RuleActivity, read)

	// Preview: old vs new price through the engine, before saving.
	route("POST /v1/tenant/pricing/preview", h.Preview, read)
}

// RegisterLiveResources declares the live channels
// (org:{org_uuid}:resource:tenant.rate_plans / tenant.date_rules) and their
// tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(ResourceRatePlans, rbac.PermTenantPricingRead)
	reg.Register(ResourceDateRules, rbac.PermTenantPricingRead)
}
