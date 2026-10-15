// Package overview is the operations summary of the tenant panel home page
// (TEC-716, requirement §15.1 "Operasyon özeti"): one aggregated endpoint
// with KPIs and the sections that need attention today, plus the sidebar
// badge counts. docs/design/operations-overview.md is the design.
//
// Permissions: no slug of its own. The routes only need a membership;
// every KPI and section is computed only when the caller's tenant role
// grants the read permission of the module it comes from (nil otherwise),
// and every query is limited to the caller's branch scope.
package overview

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/overview/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

// RegisterRoutes mounts the overview routes. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	mux.Handle("GET /v1/tenant/overview", middleware.Chain(http.HandlerFunc(h.Overview), authn, org))
	mux.Handle("GET /v1/tenant/overview/badges", middleware.Chain(http.HandlerFunc(h.Badges), authn, org))
}
