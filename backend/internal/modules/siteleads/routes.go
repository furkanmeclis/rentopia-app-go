// Package siteleads exposes organization-scoped website enquiries.
package siteleads

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/siteleads/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"net/http"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantSiteLeadsRead)
	manage := middleware.RequireTenantPermission(rbac.PermTenantSiteLeadsManage)
	// Site enquiries come from the branded site, a plan feature (TEC-674):
	// reading them stays open on every plan, changing them needs it.
	feature := middleware.RequireFeature(features.Site)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org, read}, guards...)...))
	}
	route("GET /v1/tenant/site/leads/meta", h.Meta)
	route("GET /v1/tenant/site/leads", h.List)
	route("GET /v1/tenant/site/leads/{uuid}", h.Get)
	route("PATCH /v1/tenant/site/leads/{uuid}", h.Patch, manage, feature)
}
