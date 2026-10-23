// Package publicapi is the public booking API (TEC-728,
// docs/design/public-api.md): /v1/public/* for the branded site, the
// embeddable widget and an organization's own frontends or partners, and
// /v1/tenant/public-api/* where staff manage the keys and webhooks.
//
// Permissions (staff): tenant.public_api.read lists keys, webhooks and
// deliveries; tenant.public_api.manage creates, rotates and revokes keys
// and manages webhooks.
package publicapi

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterRoutes mounts the public and the staff routes. A nil resolver
// (tenancy off) mounts nothing: every key belongs to an organization.
func RegisterRoutes(mux *http.ServeMux, public *handler.Public, panel *handler.Panel, tokens *jwt.Manager,
	loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	public.Mount(mux)

	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	read, manage := perm(rbac.PermTenantPublicAPIRead), perm(rbac.PermTenantPublicAPIManage)
	// Key and webhook management is the public_api plan feature (TEC-674):
	// reads, revoking a key and deleting an endpoint (switching things off)
	// stay open on every plan; the public routes check each key's channel
	// feature themselves.
	feature := middleware.RequireFeature(features.PublicAPI)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	route("GET /v1/tenant/public-api/overview", panel.Overview, read)
	route("GET /v1/tenant/public-api/keys", panel.ListKeys, read)
	route("POST /v1/tenant/public-api/keys", panel.CreateKey, read, manage, feature)
	route("GET /v1/tenant/public-api/keys/{uuid}", panel.GetKey, read)
	route("PATCH /v1/tenant/public-api/keys/{uuid}", panel.PatchKey, read, manage, feature)
	route("POST /v1/tenant/public-api/keys/{uuid}/rotate", panel.RotateKey, read, manage, feature)
	route("POST /v1/tenant/public-api/keys/{uuid}/revoke", panel.RevokeKey, read, manage)
	route("GET /v1/tenant/public-api/webhooks", panel.ListEndpoints, read)
	route("POST /v1/tenant/public-api/webhooks", panel.CreateEndpoint, read, manage, feature)
	route("GET /v1/tenant/public-api/webhooks/{uuid}", panel.GetEndpoint, read)
	route("PATCH /v1/tenant/public-api/webhooks/{uuid}", panel.PatchEndpoint, read, manage, feature)
	route("DELETE /v1/tenant/public-api/webhooks/{uuid}", panel.DeleteEndpoint, read, manage)
	route("POST /v1/tenant/public-api/webhooks/{uuid}/rotate-secret", panel.RotateEndpointSecret, read, manage, feature)
	route("POST /v1/tenant/public-api/webhooks/{uuid}/test", panel.TestEndpoint, read, manage, feature)
	route("GET /v1/tenant/public-api/webhook-deliveries", panel.ListDeliveries, read)
	route("GET /v1/tenant/public-api/webhook-deliveries/{uuid}", panel.GetDelivery, read)
	route("POST /v1/tenant/public-api/webhook-deliveries/{uuid}/retry", panel.RetryDelivery, read, manage, feature)
}

// RegisterLiveResources declares the live channels of keys and webhooks.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceKeys, rbac.PermTenantPublicAPIRead)
	reg.Register(model.ResourceWebhooks, rbac.PermTenantPublicAPIRead)
}
