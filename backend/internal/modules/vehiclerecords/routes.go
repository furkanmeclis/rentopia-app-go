// Package vehiclerecords is vehicle documents, maintenance and expiry
// tracking (TEC-693): sub-resources of a vehicle under
// /v1/tenant/vehicles/{uuid}/documents|maintenance, the "Operasyon özeti"
// alerts list (/v1/tenant/fleet/alerts), document / maintenance exports and
// the daily reminder job (worker).
//
// Permissions: the vehicle ones (tenant.vehicles.read / write / export) plus
// tenant.document.download for document files. Branch scope: every route
// resolves the vehicle like GET /v1/tenant/vehicles/{uuid} (home or current
// branch in scope, else 404).
package vehiclerecords

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Engine slugs of the export-only resources.
const (
	ResourceDocuments   = model.ResourceDocuments
	ResourceMaintenance = model.ResourceMaintenance
)

// Export query keys of the two resources.
var (
	DocumentExportKeys    = model.DocumentExportKeys
	MaintenanceExportKeys = model.MaintenanceExportKeys
)

// EngineHandlers are the shared export handlers. Nil members skip the route.
type EngineHandlers struct {
	ExportDocuments   http.HandlerFunc
	ExportMaintenance http.HandlerFunc
}

// RegisterRoutes mounts the routes. A nil resolver (tenancy off) mounts
// nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
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
	export := perm(rbac.PermTenantVehiclesExport)
	download := perm(rbac.PermTenantDocumentDownload)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/fleet/alerts", h.Alerts, read)
	if engines.ExportDocuments != nil {
		route("POST /v1/tenant/vehicle-documents/export", engines.ExportDocuments, read, export)
	}
	if engines.ExportMaintenance != nil {
		route("POST /v1/tenant/vehicle-maintenance/export", engines.ExportMaintenance, read, export)
	}

	route("GET /v1/tenant/vehicles/{uuid}/documents", h.ListDocuments, read)
	route("POST /v1/tenant/vehicles/{uuid}/documents", h.CreateDocument, write)
	route("PATCH /v1/tenant/vehicles/{uuid}/documents/{documentUuid}", h.PatchDocument, write)
	route("DELETE /v1/tenant/vehicles/{uuid}/documents/{documentUuid}", h.DeleteDocument, write)
	route("PUT /v1/tenant/vehicles/{uuid}/documents/{documentUuid}/file", h.UploadDocumentFile, write)
	route("GET /v1/tenant/vehicles/{uuid}/documents/{documentUuid}/file", h.DownloadDocumentFile, read, download)

	route("GET /v1/tenant/vehicles/{uuid}/maintenance", h.ListMaintenance, read)
	route("POST /v1/tenant/vehicles/{uuid}/maintenance", h.CreateMaintenance, write)
	route("PATCH /v1/tenant/vehicles/{uuid}/maintenance/{itemUuid}", h.PatchMaintenance, write)
	route("DELETE /v1/tenant/vehicles/{uuid}/maintenance/{itemUuid}", h.DeleteMaintenance, write)
	route("POST /v1/tenant/vehicles/{uuid}/maintenance/{itemUuid}/complete", h.CompleteMaintenance, write)
}
