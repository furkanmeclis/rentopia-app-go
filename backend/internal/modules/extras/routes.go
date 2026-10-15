// Package extras is the extras catalog of M2 (TEC-702, beta doc §7):
// /v1/tenant/extras (catalog, per-product prices, branch stock, equipment
// availability) and /v1/tenant/equipment-allocations (timed equipment
// holds / reservations). docs/design/extras.md.
//
// Permissions: tenant.extras.read (lists, details, prices, stock,
// availability, allocations), tenant.extras.write (catalog, prices, stock,
// bulk), tenant.availability.hold (create / confirm / release equipment
// allocations: the same sales action as a vehicle hold).
package extras

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	extrashandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Engine resource slugs.
const (
	Resource            = model.Resource
	ResourceAllocations = model.ResourceAllocations
)

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = model.QueryKeys

// EngineHandlers are the shared engine handlers the resource mounts its
// export / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts the extras routes. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *extrashandler.Handler,
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
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantExtrasRead)
	write := perm(rbac.PermTenantExtrasWrite)
	hold := perm(rbac.PermTenantAvailabilityHold)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/extras/meta", h.Meta, read)
	route("GET /v1/tenant/extras/facets", h.Facets, read)
	route("POST /v1/tenant/extras/availability", h.Availability, read)
	route("GET /v1/tenant/extras", h.List, read)
	route("POST /v1/tenant/extras", h.Create, read, write)
	if engines.Export != nil {
		route("POST /v1/tenant/extras/export", engines.Export, read)
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/extras/bulk", engines.Bulk, read)
	}
	route("GET /v1/tenant/extras/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/extras/{uuid}", h.Patch, read, write)
	route("DELETE /v1/tenant/extras/{uuid}", h.Delete, read, write)
	route("GET /v1/tenant/extras/{uuid}/activity", h.Activity, read)
	route("GET /v1/tenant/extras/{uuid}/prices", h.Prices, read)
	route("PUT /v1/tenant/extras/{uuid}/prices", h.SetPrices, read, write)
	route("GET /v1/tenant/extras/{uuid}/price-history", h.PriceHistory, read)
	route("GET /v1/tenant/extras/{uuid}/stock", h.Stock, read)
	route("PUT /v1/tenant/extras/{uuid}/stock", h.SetStock, read, write)

	// Equipment allocations (ReserveEquipment / ConfirmEquipment /
	// ReleaseEquipment over HTTP).
	route("GET /v1/tenant/equipment-allocations", h.ListAllocations, read)
	route("POST /v1/tenant/equipment-allocations", h.CreateAllocation, read, hold)
	route("GET /v1/tenant/equipment-allocations/{uuid}", h.GetAllocation, read)
	route("POST /v1/tenant/equipment-allocations/{uuid}/confirm", h.ConfirmAllocation, read, hold)
	route("DELETE /v1/tenant/equipment-allocations/{uuid}", h.ReleaseAllocation, read, hold)
}

// RegisterLiveResources declares the live channels
// (org:{org_uuid}:resource:tenant.extras / tenant.equipment_allocations).
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantExtrasRead)
	reg.Register(ResourceAllocations, rbac.PermTenantExtrasRead)
}
