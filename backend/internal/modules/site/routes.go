// Package site is the branded site (TEC-729 / TEC-730,
// docs/design/branded-site.md): /v1/tenant/site holds an organization's
// site settings (live: on / off, subdomain, locale, key; draft: theme,
// layout, content, SEO, analytics, widget, images), its publish history
// and the online publish state of the catalog; /v1/public/site/* resolves
// a request host to its published site (or the draft for a preview token)
// for the `site` app, which then books through the public API
// (/v1/public/*) with the site's publishable key.
//
// Permissions (staff): tenant.site.read reads the settings, the history
// and the catalog state and opens preview links; tenant.site.manage saves,
// publishes, restores, uploads images, renews the key and switches
// catalog items.
package site

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/site/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the public and the staff routes. A nil resolver
// (tenancy off) mounts nothing: every site belongs to an organization.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader,
	resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	mux.HandleFunc("GET /v1/public/site/resolve", h.Resolve)
	mux.HandleFunc("GET /v1/public/site/media/{siteUuid}/{mediaUuid}", h.PublicMedia)
	mux.HandleFunc("GET /v1/public/site/assets/{uuid}/{kind}", h.Asset)

	mux.HandleFunc("GET /v1/internal/site/domain-allowed", h.DomainAllowed)
	authn := middleware.Authenticate(tokens, loader)
	mux.Handle("GET /v1/platform/site-domains", middleware.Chain(http.HandlerFunc(h.PlatformDomains), authn, middleware.RequirePermission(rbac.PermPlatformSiteDomainsRead)))
	mux.Handle("POST /v1/platform/site-domains/{uuid}/recheck", middleware.Chain(http.HandlerFunc(h.RecheckDomain), authn, middleware.RequirePermission(rbac.PermPlatformSiteDomainsManage)))
	mux.Handle("POST /v1/platform/site-domains/{uuid}/mark-tls-active", middleware.Chain(http.HandlerFunc(h.MarkDomainTLSActive), authn, middleware.RequirePermission(rbac.PermPlatformSiteDomainsManage)))
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSiteRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantSiteManage)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/site/domains", h.ListDomains, read)
	route("POST /v1/tenant/site/domains", h.CreateDomain, manage)
	route("GET /v1/tenant/site/domains/{uuid}", h.GetDomain, read)
	route("DELETE /v1/tenant/site/domains/{uuid}", h.DeleteDomain, manage)
	route("POST /v1/tenant/site/domains/{uuid}/verify", h.VerifyDomain, manage)
	route("GET /v1/tenant/site/media", h.ListMedia, read)
	route("POST /v1/tenant/site/media", h.UploadMedia, manage)
	route("PATCH /v1/tenant/site/media/{uuid}", h.PatchMedia, manage)
	route("DELETE /v1/tenant/site/media/{uuid}", h.DeleteMedia, manage)
	route("GET /v1/tenant/site", h.Get, read)
	route("PUT /v1/tenant/site", h.Put, read, manage)
	route("POST /v1/tenant/site/publish", h.Publish, read, manage)
	route("GET /v1/tenant/site/versions", h.Versions, read)
	route("POST /v1/tenant/site/versions/{revisionUuid}/restore", h.Restore, read, manage)
	route("POST /v1/tenant/site/preview-token", h.PreviewToken, read)
	route("POST /v1/tenant/site/renew-key", h.RenewKey, read, manage)
	// One literal route per image kind (the isolation test enumerates route
	// literals; the kind is not a record id).
	route("PUT /v1/tenant/site/assets/logo", h.PutImage("logo"), read, manage)
	route("DELETE /v1/tenant/site/assets/logo", h.DeleteImage("logo"), read, manage)
	route("PUT /v1/tenant/site/assets/favicon", h.PutImage("favicon"), read, manage)
	route("DELETE /v1/tenant/site/assets/favicon", h.DeleteImage("favicon"), read, manage)
	route("PUT /v1/tenant/site/assets/cover", h.PutImage("cover"), read, manage)
	route("DELETE /v1/tenant/site/assets/cover", h.DeleteImage("cover"), read, manage)
	route("GET /v1/tenant/site/catalog", h.Catalog, read)
	route("PATCH /v1/tenant/site/catalog/branches/{uuid}", h.PatchBranch, read, manage)
	route("PATCH /v1/tenant/site/catalog/products/{uuid}", h.PatchSubgroup, read, manage)
	route("PATCH /v1/tenant/site/catalog/extras/{uuid}", h.PatchExtra, read, manage)
}
