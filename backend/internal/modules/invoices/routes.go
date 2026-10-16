// Package invoices is the e-invoice / e-archive tenant resource (TEC-722,
// RT-A22, docs/design/einvoice.md): drafts cut from a rental's or an
// account's live accruals (linked through ledger_entry_links, never a new
// ledger line), go-ubltr UBL-TR documents validated with the GİB XSD and
// Schematron, sending through the integration job runner, e-Arşiv
// cancellation and IADE returns, the tax mapping settings.
//
// Permissions: tenant.invoices.read (list, detail, preview, downloads),
// .write (drafts, validate, finalize, retry), .cancel (cancel, return),
// .export, .settings, .bulk.create_for_rentals, .bulk.send. The integrator
// account is an integration account behind tenant.integrations.manage.
package invoices

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/einvoice/einvoicefake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine / live list / activity resource.
const Resource = model.Resource

// ExportQueryKeys are the list filters the export accepts.
var ExportQueryKeys = append([]string{"q"}, handler.QueryKeys...)

// Providers builds the e-invoice adapters of the environment: the official
// integrator slot always (it reports "waiting on dependency" until TEC-685
// chooses the integrator), the fake (einvoice_fake) outside production
// only, so it is never selectable there.
func Providers(appEnv string, _ *slog.Logger) []einvoice.Provider {
	out := []einvoice.Provider{einvoice.Official{}}
	if !strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		out = append(out, einvoicefake.New())
	}
	return out
}

// Adapters lists providers as integration adapters (account registry).
func Adapters(providers []einvoice.Provider) []integrations.Adapter {
	out := make([]integrations.Adapter, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	return out
}

// Engines are the shared engine handlers (nil members skip the route).
type Engines struct {
	Export http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/invoices. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, engines Engines, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	read, write := perm(rbac.PermTenantInvoicesRead), perm(rbac.PermTenantInvoicesWrite)
	cancel := perm(rbac.PermTenantInvoicesCancel)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/invoices/meta", h.MetaH, read)
	route("GET /v1/tenant/invoices/facets", h.Facets, read)
	route("GET /v1/tenant/invoices/items", h.Items, read)
	route("GET /v1/tenant/invoices/uninvoiced-rentals", h.UninvoicedRentals, read)
	route("GET /v1/tenant/invoices/settings", h.Settings, read)
	route("PUT /v1/tenant/invoices/settings", h.PutSettings, read, perm(rbac.PermTenantInvoicesSettings))
	route("GET /v1/tenant/invoices", h.List, read)
	route("POST /v1/tenant/invoices", h.Create, read, write)
	if engines.Export != nil {
		route("POST /v1/tenant/invoices/export", engines.Export, read, perm(rbac.PermTenantInvoicesExport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the per-action permission in the scope.
		route("POST /v1/tenant/invoices/bulk", engines.Bulk, read)
	}
	route("GET /v1/tenant/invoices/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/invoices/{uuid}", h.Patch, read, write)
	route("POST /v1/tenant/invoices/{uuid}/validate", h.Validate, read, write)
	route("POST /v1/tenant/invoices/{uuid}/finalize", h.Finalize, read, write)
	route("POST /v1/tenant/invoices/{uuid}/retry", h.Retry, read, write)
	route("POST /v1/tenant/invoices/{uuid}/cancel", h.Cancel, read, cancel)
	route("POST /v1/tenant/invoices/{uuid}/return", h.Return, read, cancel)
	route("GET /v1/tenant/invoices/{uuid}/preview", h.Preview, read)
	route("GET /v1/tenant/invoices/{uuid}/xml", h.XML, read)
	route("GET /v1/tenant/invoices/{uuid}/pdf", h.PDF, read)
}

// RegisterLiveResources declares the live list channel
// (org:{org_uuid}:resource:tenant.invoices) and its read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantInvoicesRead)
}
