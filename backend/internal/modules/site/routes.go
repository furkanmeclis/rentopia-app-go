// Package site is the branded site (TEC-729, docs/design/branded-site.md):
// /v1/tenant/site holds an organization's site settings (theme, layout,
// content, SEO, analytics, subdomain) and /v1/public/site/* resolves a
// request host to its site for the `site` app, which then books through
// the public API (/v1/public/*) with the site's publishable key.
//
// Permissions (staff): tenant.site.read reads the settings;
// tenant.site.manage saves them (switching the site on creates its key).
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
	mux.HandleFunc("GET /v1/public/site/assets/{uuid}/{kind}", h.Asset)

	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSiteRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantSiteManage)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/site", h.Get, read)
	route("PUT /v1/tenant/site", h.Put, read, manage)
}
