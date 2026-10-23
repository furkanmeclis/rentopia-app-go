// Package tolls is the HGS / OGS toll pass tenant resource (TEC-725,
// RT-A20, docs/design/tolls.md): passes from the provider fetch or a file
// import, matching to rentals, the review queue, vehicle tags and the
// processing fee settings.
package tolls

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tolls"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/tolls/tollfake"
)

// Resource is the engine slug of toll passes.
const Resource = model.Resource

// ExportQueryKeys are the list filters an export honours.
var ExportQueryKeys = model.QueryKeys

// Providers builds the toll providers of the environment. No real provider
// exists yet (TEC-683); the fake (hgs_fake) is registered outside
// production only, so it is never selectable there.
func Providers(appEnv string, _ *slog.Logger) []tolls.Provider {
	out := []tolls.Provider{}
	if !strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		out = append(out, tollfake.New())
	}
	return out
}

// Adapters lists providers as integration adapters (account registry).
func Adapters(providers []tolls.Provider) []integrations.Adapter {
	out := make([]integrations.Adapter, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	return out
}

// EngineHandlers are the shared engine handlers the resource mounts its
// export / import routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Import http.HandlerFunc
	Sample http.HandlerFunc
}

// RegisterRoutes mounts /v1/tenant/toll-passes, /v1/tenant/toll-settings
// and /v1/tenant/vehicles/{uuid}/toll-tags. A nil resolver (tenancy off)
// mounts nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
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
	read := perm(rbac.PermTenantTollsRead)
	importP := perm(rbac.PermTenantTollsImport)
	review := perm(rbac.PermTenantTollsReview)
	// Toll passes (HGS) are a plan feature (TEC-674): reads and the export
	// stay open on every plan; fetching, importing, reviewing (which posts
	// charges), settings and tags need it.
	feature := middleware.RequireFeature(features.HGS)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/toll-passes/meta", h.MetaH, read)
	route("GET /v1/tenant/toll-passes/facets", h.Facets, read)
	route("GET /v1/tenant/toll-passes", h.List, read)
	if engines.Export != nil {
		route("POST /v1/tenant/toll-passes/export", engines.Export, read, perm(rbac.PermTenantTollsExport))
	}
	if engines.Import != nil {
		route("POST /v1/tenant/toll-passes/import", engines.Import, read, importP, feature)
	}
	if engines.Sample != nil {
		route("GET /v1/tenant/toll-passes/import/sample", engines.Sample, importP)
	}
	route("POST /v1/tenant/toll-passes/fetch", h.Fetch, read, importP, feature)

	route("GET /v1/tenant/toll-passes/{uuid}", h.Get, read)
	route("GET /v1/tenant/toll-passes/{uuid}/candidates", h.Candidates, read, review)
	route("POST /v1/tenant/toll-passes/{uuid}/assign", h.Assign, read, review, feature)
	route("POST /v1/tenant/toll-passes/{uuid}/unassign", h.Unassign, read, review, feature)
	route("POST /v1/tenant/toll-passes/{uuid}/ignore", h.Ignore, read, review, feature)
	route("POST /v1/tenant/toll-passes/{uuid}/rematch", h.Rematch, read, review, feature)

	route("GET /v1/tenant/toll-settings", h.Settings, read)
	route("PATCH /v1/tenant/toll-settings", h.UpdateSettings, read, perm(rbac.PermTenantSettingsWrite), feature)

	// Vehicle tags are vehicle data: reading needs the toll read
	// permission, changes the vehicle write permission.
	route("GET /v1/tenant/vehicles/{uuid}/toll-tags", h.Tags, read)
	route("POST /v1/tenant/vehicles/{uuid}/toll-tags", h.CreateTag, read, perm(rbac.PermTenantVehiclesWrite), feature)
	route("PATCH /v1/tenant/vehicles/{uuid}/toll-tags/{tagUuid}", h.UpdateTag, read, perm(rbac.PermTenantVehiclesWrite), feature)
	route("DELETE /v1/tenant/vehicles/{uuid}/toll-tags/{tagUuid}", h.DeleteTag, read, perm(rbac.PermTenantVehiclesWrite), feature)
}

// RegisterLiveResources declares the toll pass list channel
// (org:{org_uuid}:resource:tenant.toll_passes) and its read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantTollsRead)
}
