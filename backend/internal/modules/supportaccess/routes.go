// Package supportaccess is time-boxed, recorded support access (TEC-678):
// an organization's admins open a grant (/v1/tenant/support-access) that
// lets platform support staff enter the organization through impersonation
// for 1 hour to 7 days, read-only or full, and revoke it at any time.
// Impersonated sessions are refused everywhere in the tenant API without an
// active grant (GuardResolver) and cannot open or revoke grants themselves.
package supportaccess

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the tenant routes (nil resolver: tenancy off,
// nothing mounted) and the platform overview of active grants.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	authn := middleware.Authenticate(tokens, loader)
	mux.Handle("GET /v1/platform/support-grants", middleware.Chain(http.HandlerFunc(h.PlatformActive),
		authn, middleware.RequirePermission(rbac.PermPlatformUsersImpersonate)))
	if resolver == nil {
		return
	}
	org := middleware.RequireOrganization(resolver)
	manage := middleware.RequireTenantPermission(rbac.PermTenantSupportAccessManage)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, manage))
	}
	route("GET /v1/tenant/support-access", h.List)
	route("POST /v1/tenant/support-access", h.Open)
	route("POST /v1/tenant/support-access/{uuid}/revoke", h.Revoke)
}
