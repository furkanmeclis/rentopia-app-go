// Package customers is the customers and drivers tenant resource (TEC-705,
// /v1/tenant/customers): individual / corporate customers, corporate
// contacts, marketing consents, drivers with the rental condition check,
// and identity / licence documents in private storage.
//
// Customers are organization-wide (not branch-bound) and never shared
// between organizations. Permissions: tenant.customers.read (masked
// values), .write, .reveal (plaintext identity / licence / contact data,
// always with a reason and an activity event), .export, .import,
// .anonymize; document files also need tenant.document.download.
//
// Step-up: the reveal and anonymize routes do not require a step-up grant.
// The step-up engine's policy (TTL, methods) is a platform setting
// (/platform/settings/access); there is no organization-level policy yet,
// so tenant routes rely on the dedicated permissions plus the audit trail.
// Adding middleware.RequireStepUp here is a one-line change once a
// tenant-capable policy exists.
package customers

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug of customers.
const Resource = adapters.Resource

// ExportQueryKeys are the query keys an export honours: the list filters and
// include_sensitive.
var ExportQueryKeys = append(append([]string{}, model.QueryKeys...), model.ExportOnlyKeys...)

// EngineHandlers are the shared engine handlers the resource mounts its
// export / import / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/customers. A nil resolver (tenancy off)
// mounts nothing.
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
	read := perm(rbac.PermTenantCustomersRead)
	write := perm(rbac.PermTenantCustomersWrite)
	reveal := perm(rbac.PermTenantCustomersReveal)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/customers/meta", h.Meta, read)
	route("GET /v1/tenant/customers/facets", h.Facets, read)
	route("GET /v1/tenant/customers", h.List, read)
	route("POST /v1/tenant/customers", h.Create, write)
	if engines.Export != nil {
		// include_sensitive additionally needs tenant.customers.reveal
		// (checked and recorded by ExportGuard, again by the adapter).
		route("POST /v1/tenant/customers/export", h.ExportGuard(engines.Export), read, perm(rbac.PermTenantCustomersExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/customers/import", engines.Import, write, perm(rbac.PermTenantCustomersImport))
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/customers/import/sample", engines.Sample, perm(rbac.PermTenantCustomersImport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the action permission (tenant.customers.write).
		route("POST /v1/tenant/customers/bulk", engines.Bulk, read)
	}

	route("GET /v1/tenant/customers/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/customers/{uuid}", h.Patch, write)
	route("DELETE /v1/tenant/customers/{uuid}", h.Delete, write)
	route("POST /v1/tenant/customers/{uuid}/reveal", h.Reveal, read, reveal)
	route("POST /v1/tenant/customers/{uuid}/anonymize", h.Anonymize, read, perm(rbac.PermTenantCustomersAnonymize))
	route("GET /v1/tenant/customers/{uuid}/activity", h.Activity, read)

	route("GET /v1/tenant/customers/{uuid}/contacts", h.ListContacts, read)
	route("POST /v1/tenant/customers/{uuid}/contacts", h.CreateContact, write)
	route("PATCH /v1/tenant/customers/{uuid}/contacts/{contactUuid}", h.PatchContact, write)
	route("DELETE /v1/tenant/customers/{uuid}/contacts/{contactUuid}", h.DeleteContact, write)

	route("GET /v1/tenant/customers/{uuid}/consents", h.Consents, read)
	route("POST /v1/tenant/customers/{uuid}/consents", h.RecordConsent, write)

	route("GET /v1/tenant/customers/{uuid}/drivers", h.ListDrivers, read)
	route("POST /v1/tenant/customers/{uuid}/drivers", h.CreateDriver, write)
	route("GET /v1/tenant/customers/{uuid}/drivers/{driverUuid}", h.GetDriver, read)
	route("PATCH /v1/tenant/customers/{uuid}/drivers/{driverUuid}", h.PatchDriver, write)
	route("DELETE /v1/tenant/customers/{uuid}/drivers/{driverUuid}", h.DeleteDriver, write)
	route("GET /v1/tenant/customers/{uuid}/drivers/{driverUuid}/eligibility", h.DriverEligibility, read)

	route("GET /v1/tenant/customers/{uuid}/documents", h.ListDocuments, read)
	route("POST /v1/tenant/customers/{uuid}/documents", h.CreateDocument, write)
	route("DELETE /v1/tenant/customers/{uuid}/documents/{docUuid}", h.DeleteDocument, write)
	route("PUT /v1/tenant/customers/{uuid}/documents/{docUuid}/file", h.UploadDocumentFile, write)
	route("GET /v1/tenant/customers/{uuid}/documents/{docUuid}/file", h.DownloadDocumentFile,
		read, perm(rbac.PermTenantDocumentDownload), reveal)
}

// RegisterLiveResources declares the customers list channel
// (org:{org_uuid}:resource:tenant.customers) and its tenant read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantCustomersRead)
}
