// Package availability is the availability and capacity engine (TEC-697):
// capacity search (/v1/tenant/availability/search), holds
// (/v1/tenant/availability/holds) and channel capacity rules
// (/v1/tenant/capacity-rules). docs/design/availability-engine.md.
package availability

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	availabilityhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterRoutes mounts the availability routes. A nil resolver (tenancy
// off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *availabilityhandler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	read := perm(rbac.PermTenantAvailabilityRead)
	hold := perm(rbac.PermTenantAvailabilityHold)
	route("POST /v1/tenant/availability/search", h.Search, read)
	route("GET /v1/tenant/availability/holds", h.ListHolds, read)
	route("POST /v1/tenant/availability/holds", h.CreateHold, hold)
	route("GET /v1/tenant/availability/holds/{uuid}", h.GetHold, read)
	route("DELETE /v1/tenant/availability/holds/{uuid}", h.ReleaseHold, hold)
	route("POST /v1/tenant/availability/holds/{uuid}/extend", h.ExtendHold, hold)

	rulesRead := perm(rbac.PermTenantCapacityRulesRead)
	rulesWrite := perm(rbac.PermTenantCapacityRulesWrite)
	route("GET /v1/tenant/capacity-rules", h.ListRules, rulesRead)
	route("POST /v1/tenant/capacity-rules", h.CreateRule, rulesWrite)
	route("GET /v1/tenant/capacity-rules/{uuid}", h.GetRule, rulesRead)
	route("PUT /v1/tenant/capacity-rules/{uuid}", h.UpdateRule, rulesWrite)
	route("DELETE /v1/tenant/capacity-rules/{uuid}", h.DeleteRule, rulesWrite)
}

// RegisterLiveResources declares the live channels of holds and rules.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceHolds, rbac.PermTenantAvailabilityRead)
	reg.Register(model.ResourceCapacityRules, rbac.PermTenantCapacityRulesRead)
}
