// Package kabis is the KABİS rental notification tenant resource (TEC-726,
// RT-A21, docs/design/kabis.md): notifications of actual handovers and
// returns (and corrections) on the integration job runner, staff retry
// (status check first), manual reporting / reconciliation with a reason,
// reference and attachment.
//
// Permissions: tenant.kabis.read (screen, detail, rental chip,
// attachment), tenant.kabis.retry, tenant.kabis.reconcile (mark manual /
// reconcile). The KABİS account (enabled, adapter, credentials) is an
// integration account behind tenant.integrations.manage.
package kabis

import (
	"log/slog"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/kabis/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/envguard"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/kabis/kabisfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the live list / activity resource.
const Resource = model.Resource

// Providers builds the KABİS adapters of the environment: the official
// slot always (it reports "waiting on dependency" until TEC-682 verifies
// the method), the fake (kabis_fake) outside production only, so it is
// never selectable there. Outside production the official slot is also
// guarded (TEC-744): it never reaches the authority unless
// KABIS_ALLOW_PRODUCTION_IN_NON_PROD (allowProduction) is set.
func Providers(appEnv string, allowProduction bool, _ *slog.Logger) []kabis.Provider {
	refusal := envguard.Refusal(appEnv, "kabis", envguard.AllowKabisKey, allowProduction)
	out := []kabis.Provider{kabis.Guard(kabis.Official{}, refusal)}
	if !envguard.IsProduction(appEnv) {
		out = append(out, kabisfake.New())
	}
	return out
}

// Adapters lists providers as integration adapters (account registry).
func Adapters(providers []kabis.Provider) []integrations.Adapter {
	out := make([]integrations.Adapter, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	return out
}

// RegisterRoutes mounts /v1/tenant/kabis-notifications. A nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantKabisRead)
	// KABİS is a plan feature (TEC-674): the history, manual marking and
	// reconciliation stay open on every plan; a retry sends, so it needs it.
	feature := middleware.RequireFeature(features.Kabis)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/kabis-notifications/meta", h.MetaH, read)
	route("GET /v1/tenant/kabis-notifications/facets", h.Facets, read)
	route("GET /v1/tenant/kabis-notifications", h.List, read)
	route("GET /v1/tenant/kabis-notifications/{uuid}", h.Get, read)
	route("GET /v1/tenant/kabis-notifications/{uuid}/attachment", h.Attachment, read)
	route("POST /v1/tenant/kabis-notifications/{uuid}/retry", h.Retry, read, perm(rbac.PermTenantKabisRetry), feature)
	route("POST /v1/tenant/kabis-notifications/{uuid}/mark-manual", h.MarkManual, read, perm(rbac.PermTenantKabisReconcile))
	route("POST /v1/tenant/kabis-notifications/{uuid}/reconcile", h.Reconcile, read, perm(rbac.PermTenantKabisReconcile))
}

// RegisterLiveResources declares the live list channel
// (org:{org_uuid}:resource:tenant.kabis_notifications) and its read
// permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(Resource, rbac.PermTenantKabisRead)
}
