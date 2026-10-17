// Package campaigns is the campaigns / coupons resource of M2 (TEC-703,
// beta doc §6.4): /v1/tenant/campaigns. Coupons are checked by the price
// preview (coupon_code) and redeemed by reservations (TEC-706) through
// usecase.Service.Redeem / Consume / Release. docs/design/campaigns.md.
//
// Permissions: tenant.campaigns.read (lists, details, redemptions,
// export), tenant.campaigns.write (create, edit, delete, bulk). Using a
// coupon code at sale time needs no permission of its own; a manual
// discount stays tenant.discount.apply.
package campaigns

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	campaignhandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug.
const Resource = model.Resource

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = model.QueryKeys

// EngineHandlers are the shared engine handlers the resource mounts its
// export / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/campaigns. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *campaignhandler.Handler,
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
	read := perm(rbac.PermTenantCampaignsRead)
	write := perm(rbac.PermTenantCampaignsWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/campaigns/meta", h.Meta, read)
	route("GET /v1/tenant/campaigns/facets", h.Facets, read)
	route("GET /v1/tenant/campaigns", h.List, read)
	route("POST /v1/tenant/campaigns", h.Create, read, write)
	if engines.Export != nil {
		route("POST /v1/tenant/campaigns/export", engines.Export, read)
	}
	if engines.Bulk != nil {
		route("POST /v1/tenant/campaigns/bulk", engines.Bulk, read)
	}
	route("GET /v1/tenant/campaigns/{uuid}/storefront", h.GetStorefront, read)
	route("PATCH /v1/tenant/campaigns/{uuid}/storefront", h.PatchStorefront, read, write)
	route("GET /v1/tenant/campaigns/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/campaigns/{uuid}", h.Patch, read, write)
	route("DELETE /v1/tenant/campaigns/{uuid}", h.Delete, read, write)
	route("GET /v1/tenant/campaigns/{uuid}/activity", h.Activity, read)
	route("GET /v1/tenant/campaigns/{uuid}/redemptions", h.Redemptions, read)
}

// RegisterLiveResources declares the live channel
// (org:{org_uuid}:resource:tenant.campaigns).
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantCampaignsRead)
}
