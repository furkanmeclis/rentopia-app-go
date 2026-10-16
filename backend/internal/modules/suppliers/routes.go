// Package suppliers is the supplier card resource (/v1/tenant/suppliers,
// TEC-721, docs/design/suppliers.md). Supplier invoices and payments live
// in finance (/v1/tenant/finance/supplier-invoices, supplier-payments)
// because they post to the ledger and the cash book.
package suppliers

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/adapters"
	supplierhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/suppliers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug of suppliers.
const Resource = model.Resource

// QueryKeys are the list filters an export / bulk honours.
var QueryKeys = adapters.QueryKeys

// EngineHandlers are the shared engine handlers the resource mounts its
// export / import / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/suppliers. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *supplierhandler.Handler,
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
	read, write := perm(rbac.PermTenantSuppliersRead), perm(rbac.PermTenantSuppliersWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/suppliers/meta", h.Meta, read)
	route("GET /v1/tenant/suppliers/facets", h.Facets, read)
	route("GET /v1/tenant/suppliers", h.List, read)
	route("POST /v1/tenant/suppliers", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/suppliers/export", engines.Export, read, perm(rbac.PermTenantSuppliersExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/suppliers/import", engines.Import, write, perm(rbac.PermTenantSuppliersImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/suppliers/import/sample", engines.Sample, perm(rbac.PermTenantSuppliersImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/suppliers/bulk", engines.Bulk, read)
	}
	route("GET /v1/tenant/suppliers/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/suppliers/{uuid}", h.Patch, write)
	route("GET /v1/tenant/suppliers/{uuid}/activity", h.Activity, read)
}

// RegisterLiveResources declares the suppliers list channel
// (org:{org_uuid}:resource:tenant.suppliers) and its read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantSuppliersRead)
}
