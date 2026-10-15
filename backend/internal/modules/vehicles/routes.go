// Package vehicles is the fleet vehicles tenant resource (TEC-692,
// /v1/tenant/vehicles): physical vehicles, their operational status history
// and rental group memberships, plus the eligible vehicles of a product
// (/v1/tenant/products/{uuid}/eligible-vehicles).
package vehicles

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/adapters"
	vehiclehandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug of vehicles.
const Resource = adapters.Resource

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = model.QueryKeys

// EngineHandlers are the shared engine handlers the resource mounts its
// export / import / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/vehicles. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *vehiclehandler.Handler,
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
	read := perm(rbac.PermTenantVehiclesRead)
	write := perm(rbac.PermTenantVehiclesWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/vehicles/meta", h.Meta, read)
	route("GET /v1/tenant/vehicles/facets", h.Facets, read)
	route("GET /v1/tenant/vehicles", h.List, read)
	route("POST /v1/tenant/vehicles", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/vehicles/export", engines.Export, read, perm(rbac.PermTenantVehiclesExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/vehicles/import", engines.Import, write, perm(rbac.PermTenantVehiclesImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/vehicles/import/sample", engines.Sample, perm(rbac.PermTenantVehiclesImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the action permission (tenant.vehicles.bulk).
		route("POST /v1/tenant/vehicles/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/vehicles/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/vehicles/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/vehicles/{uuid}", h.Delete, write)
	route("POST /v1/tenant/vehicles/{uuid}/status", h.ChangeStatus, write)
	route("GET /v1/tenant/vehicles/{uuid}/events", h.Events, read)
	route("GET /v1/tenant/vehicles/{uuid}/rental-groups", h.Groups, read)
	route("POST /v1/tenant/vehicles/{uuid}/rental-groups", h.AddGroup, write)
	route("DELETE /v1/tenant/vehicles/{uuid}/rental-groups/{groupUuid}", h.RemoveGroup, write)
	route("GET /v1/tenant/vehicles/{uuid}/kasko", h.Kasko, read)
	route("GET /v1/tenant/vehicles/{uuid}/activity", h.Activity, read)

	// Products tab "Taahhüt & uygun araçlar": vehicle data, so vehicles.read.
	route("GET /v1/tenant/products/{uuid}/eligible-vehicles", h.ProductVehicles, read)
	// Rental group detail "Araçlar": explicit memberships with their current
	// eligibility, and "add all eligible vehicles" (memberships are vehicle
	// data, so vehicles.read / vehicles.write).
	route("GET /v1/tenant/rental-groups/{uuid}/members", h.GroupMembers, read)
	route("POST /v1/tenant/rental-groups/{uuid}/members/add-eligible", h.AddEligibleMembers, write)
}

// RegisterLiveResources declares the vehicles list channel
// (org:{org_uuid}:resource:tenant.vehicles) and its tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantVehiclesRead)
}
