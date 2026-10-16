// Package platformhealth is the platform usage and connection health
// dashboard (TEC-739, §2.2 / §15.1): usage per organization (active
// vehicles, monthly reservations, members, tracked file storage), the
// health of every registered health source across all organizations, and
// the failed-work alert sweep that notifies platform administrators.
//
// Health sources are pluggable (internal/platform/healthcheck): the
// integration account and integration job sources are built in
// (NewRegistry); other modules register theirs on the same registry at the
// composition root, e.g.
//
//	reg := platformhealth.NewRegistry(queries)
//	_ = reg.Register(channelmanager.NewHealthSource(...))
//
// Every route needs platform.health.read.
package platformhealth

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/platformhealth/sources"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/healthcheck"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// NewRegistry builds the health source registry with the built-in
// sources (integration accounts, integration jobs).
func NewRegistry(q *db.Queries) *healthcheck.Registry {
	return healthcheck.NewRegistry(
		sources.NewIntegrationAccounts(q),
		sources.NewIntegrationJobs(q),
	)
}

// RegisterRoutes mounts /v1/platform/usage* and /v1/platform/health*.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformHealthRead)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, read))
	}
	route("GET /v1/platform/usage/meta", h.UsageMeta)
	route("GET /v1/platform/usage/summary", h.UsageSummary)
	route("GET /v1/platform/usage", h.ListUsage)
	route("GET /v1/platform/health/issues/meta", h.IssuesMeta)
	route("GET /v1/platform/health/issues", h.ListIssues)
	route("GET /v1/platform/health", h.Health)
}
