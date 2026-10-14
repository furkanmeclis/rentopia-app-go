// Package fleetops is the operational side of the fleet: vehicle usage
// blocks (TEC-694, /v1/tenant/vehicles/{uuid}/blocks), transfers between
// branches (/v1/tenant/transfers) and one-way rules
// (/v1/tenant/one-way-rules), TEC-695. Blocks and transfers are inputs of
// the availability engine (vehicle_busy_intervals view,
// docs/design/fleet-availability-inputs.md).
package fleetops

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	fleetopshandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Engine resource slugs.
const (
	ResourceBlocks      = model.ResourceBlocks
	ResourceTransfers   = model.ResourceTransfers
	ResourceOneWayRules = model.ResourceOneWayRules
)

// Export query keys per resource (the list filters an export honours).
var (
	TransferExportKeys = model.TransferQueryKeys
	OneWayExportKeys   = model.OneWayQueryKeys
)

// EngineHandlers are the shared engine handlers the resources mount their
// export / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	ExportTransfers http.HandlerFunc
	ExportOneWay    http.HandlerFunc
	BulkOneWay      http.HandlerFunc
}

// RegisterRoutes mounts the fleet operations routes. A nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *fleetopshandler.Handler,
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
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	optional := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		if fn != nil {
			route(pattern, fn, guards...)
		}
	}

	// Blocks: children of a vehicle (the vehicle's branch scope applies).
	blocksRead := perm(rbac.PermTenantBlocksRead)
	blocksWrite := perm(rbac.PermTenantBlocksWrite)
	route("GET /v1/tenant/vehicles/{uuid}/blocks", h.ListBlocks, blocksRead)
	route("POST /v1/tenant/vehicles/{uuid}/blocks", h.CreateBlock, blocksWrite)
	route("PATCH /v1/tenant/vehicles/{uuid}/blocks/{blockUuid}", h.PatchBlock, blocksWrite)
	route("DELETE /v1/tenant/vehicles/{uuid}/blocks/{blockUuid}", h.DeleteBlock, blocksWrite)
	route("POST /v1/tenant/vehicles/{uuid}/blocks/{blockUuid}/release", h.ReleaseBlock, blocksWrite)

	// Fleet calendar (TEC-696): vehicles with their blocks, transfers and
	// maintenance days in a window. Block / transfer bars additionally need
	// their own read permission (the use case drops the layer otherwise).
	route("GET /v1/tenant/fleet/calendar", h.Calendar, perm(rbac.PermTenantVehiclesRead))

	// Transfers. Static segments before /{uuid}.
	trRead := perm(rbac.PermTenantTransfersRead)
	trWrite := perm(rbac.PermTenantTransfersWrite)
	route("GET /v1/tenant/transfers/meta", h.TransfersMeta, trRead)
	route("GET /v1/tenant/transfers/facets", h.TransferFacets, trRead)
	route("GET /v1/tenant/transfers/assignees", h.TransferAssignees, trWrite)
	route("GET /v1/tenant/transfers", h.ListTransfers, trRead)
	route("POST /v1/tenant/transfers", h.CreateTransfer, trWrite)
	optional("POST /v1/tenant/transfers/export", engines.ExportTransfers, trRead)
	route("GET /v1/tenant/transfers/{uuid}", h.GetTransfer, trRead)
	route("PATCH /v1/tenant/transfers/{uuid}", h.PatchTransfer, trWrite)
	route("POST /v1/tenant/transfers/{uuid}/start", h.StartTransfer, trWrite)
	route("POST /v1/tenant/transfers/{uuid}/complete", h.CompleteTransfer, trWrite)
	route("POST /v1/tenant/transfers/{uuid}/cancel", h.CancelTransfer, trWrite)
	route("GET /v1/tenant/transfers/{uuid}/activity", h.TransferActivity, trRead)

	// One-way rules.
	owRead := perm(rbac.PermTenantOneWayRead)
	owWrite := perm(rbac.PermTenantOneWayWrite)
	route("GET /v1/tenant/one-way-rules/meta", h.OneWayMeta, owRead)
	route("GET /v1/tenant/one-way-rules/facets", h.OneWayFacets, owRead)
	route("GET /v1/tenant/one-way-rules/resolve", h.ResolveOneWay, owRead)
	route("GET /v1/tenant/one-way-rules", h.ListOneWay, owRead)
	route("POST /v1/tenant/one-way-rules", h.CreateOneWay, owWrite)
	optional("POST /v1/tenant/one-way-rules/export", engines.ExportOneWay, owRead)
	// The bulk handler checks the action permission (tenant.one_way.write).
	optional("POST /v1/tenant/one-way-rules/bulk", engines.BulkOneWay, owRead)
	route("GET /v1/tenant/one-way-rules/{uuid}", h.GetOneWay, owRead)
	route("PATCH /v1/tenant/one-way-rules/{uuid}", h.PatchOneWay, owWrite)
	route("DELETE /v1/tenant/one-way-rules/{uuid}", h.DeleteOneWay, owWrite)
	route("GET /v1/tenant/one-way-rules/{uuid}/activity", h.OneWayActivity, owRead)
}

// RegisterLiveResources declares the live channels
// (org:{org_uuid}:resource:tenant.transfers / tenant.one_way_rules /
// tenant.vehicle_blocks) and their tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(ResourceTransfers, rbac.PermTenantTransfersRead)
	reg.Register(ResourceOneWayRules, rbac.PermTenantOneWayRead)
	reg.Register(ResourceBlocks, rbac.PermTenantBlocksRead)
}
