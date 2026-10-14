// Package catalog is the platform vehicle catalog (TSB kasko list): brand /
// model / type / model-year kasko values, imported monthly.
package catalog

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	cataloghandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts /v1/platform/catalog/* and, when tenancy is on
// (resolver != nil), the read-only /v1/tenant/catalog/* lookups.
func RegisterRoutes(
	mux *http.ServeMux,
	h *cataloghandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformCatalogRead)
	write := middleware.RequirePermission(rbac.PermPlatformCatalogWrite)
	route := func(pattern string, fn http.HandlerFunc, mws ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn}, mws...)...))
	}

	// /meta before /{uuid}.
	route("GET /v1/platform/catalog/brands/meta", h.BrandsMeta, read)
	route("GET /v1/platform/catalog/brands", h.ListBrands, read)
	route("GET /v1/platform/catalog/brands/{uuid}", h.GetBrand, read)
	route("PATCH /v1/platform/catalog/brands/{uuid}", h.PatchBrand, write)

	route("GET /v1/platform/catalog/models/meta", h.ModelsMeta, read)
	route("GET /v1/platform/catalog/models", h.ListModels, read)
	route("POST /v1/platform/catalog/models", h.CreateModel, write)
	route("GET /v1/platform/catalog/models/{uuid}", h.GetModel, read)
	route("PATCH /v1/platform/catalog/models/{uuid}", h.PatchModel, write)
	route("POST /v1/platform/catalog/models/{uuid}/merge", h.MergeModel, write)

	route("GET /v1/platform/catalog/types/meta", h.TypesMeta, read)
	route("GET /v1/platform/catalog/types", h.ListTypes, read)
	route("GET /v1/platform/catalog/types/{uuid}", h.GetType, read)
	route("PATCH /v1/platform/catalog/types/{uuid}", h.PatchType, write)

	route("GET /v1/platform/catalog/imports/meta", h.ImportsMeta, read)
	route("GET /v1/platform/catalog/imports", h.ListImports, read)
	route("POST /v1/platform/catalog/imports", h.UploadImport, write)
	route("GET /v1/platform/catalog/imports/{uuid}", h.GetImport, read)
	route("POST /v1/platform/catalog/imports/{uuid}/apply", h.ApplyImport, write)

	if resolver == nil {
		return
	}
	org := middleware.RequireOrganization(resolver)
	route("GET /v1/tenant/catalog/brands", h.TenantBrands, org)
	route("GET /v1/tenant/catalog/models", h.TenantModels, org)
	route("GET /v1/tenant/catalog/types", h.TenantTypes, org)
}
