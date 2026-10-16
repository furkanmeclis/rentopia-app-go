// Package agreements is the corporate agreement resource (TEC-723,
// RT-A24): /v1/tenant/agreements. docs/design/corporate-agreements.md is
// the design.
//
// Permissions: tenant.agreements.read (list, detail, tariffs, rentals,
// billing plan, statement, activity) / .write (create, edit, status,
// contacts, permitted drivers, delete a draft) / .export (list export).
// Agreement tariffs are edited through the rate plan API
// (tenant.pricing.write / activate).
package agreements

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug of agreements.
const Resource = model.Resource

// ExportQueryKeys are the query keys an export honours (list filters).
var ExportQueryKeys = append([]string{"q"}, model.QueryKeys...)

// EngineHandlers are the shared engine handlers the resource mounts its
// export route on. A nil member skips the route.
type EngineHandlers struct {
	Export http.HandlerFunc
}

// RegisterRoutes mounts the agreement routes. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, engines EngineHandlers, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	read := perm(rbac.PermTenantAgreementsRead)
	write := perm(rbac.PermTenantAgreementsWrite)

	// Static segments before /{uuid}.
	route("GET /v1/tenant/agreements/meta", h.Meta, read)
	route("GET /v1/tenant/agreements/facets", h.Facets, read)
	route("GET /v1/tenant/agreements", h.List, read)
	route("POST /v1/tenant/agreements", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/agreements/export", engines.Export, perm(rbac.PermTenantAgreementsExport))
	}
	route("GET /v1/tenant/agreements/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/agreements/{uuid}", h.Update, write)
	route("DELETE /v1/tenant/agreements/{uuid}", h.Delete, write)
	route("POST /v1/tenant/agreements/{uuid}/status", h.SetStatus, write)
	route("POST /v1/tenant/agreements/{uuid}/contacts", h.AddContact, write)
	route("DELETE /v1/tenant/agreements/{uuid}/contacts/{contactUuid}", h.RemoveContact, write)
	route("POST /v1/tenant/agreements/{uuid}/drivers", h.AddDriver, write)
	route("DELETE /v1/tenant/agreements/{uuid}/drivers/{driverUuid}", h.RemoveDriver, write)
	route("GET /v1/tenant/agreements/{uuid}/tariffs", h.Tariffs, read)
	route("GET /v1/tenant/agreements/{uuid}/rentals", h.Rentals, read)
	route("GET /v1/tenant/agreements/{uuid}/billing", h.Billing, read)
	route("GET /v1/tenant/agreements/{uuid}/statement", h.Statement, read)
	route("GET /v1/tenant/agreements/{uuid}/activity", h.Activity, read)
}

// RegisterLiveResources declares the live list channel and its read
// permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.Resource, rbac.PermTenantAgreementsRead)
}
