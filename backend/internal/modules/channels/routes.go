// Package channels is the channel manager tenant resource (TEC-736,
// RT-A25, docs/design/channel-manager.md): channel accounts with their
// sync settings and state, mappings of branches / products / extras to the
// channel's codes, the inbound message and error queue, the sync log and
// the channel vs Rentopia reconciliation report.
//
// Permissions: tenant.channels.read (everything visible),
// tenant.channels.write (mappings, sync settings), tenant.channels.sync
// (push / pull now, retry / dismiss a message, simulate a booking). The
// channel account itself (enable, adapter, credentials) is an integration
// account behind tenant.integrations.manage.
package channels

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels/channelfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels/obilet"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels/yolcu360"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Providers builds the channel adapters of the environment: Yolcu360
// (documented supplier contract, TEC-680) and Obilet (documented client API
// conventions, supplier operations assumed until confirmed, TEC-737) always,
// their fakes (yolcu360_fake, obilet_fake) outside production only, so they
// are never selectable there.
func Providers(appEnv string, _ *slog.Logger) []channels.Provider {
	out := []channels.Provider{yolcu360.New(nil), obilet.New(nil)}
	if !strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		out = append(out, channelfake.New(), channelfake.NewObilet())
	}
	return out
}

// Adapters lists providers as integration adapters (account registry).
func Adapters(providers []channels.Provider) []integrations.Adapter {
	out := make([]integrations.Adapter, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	return out
}

// RegisterRoutes mounts the channel manager routes. A nil resolver
// (tenancy off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	read := perm(rbac.PermTenantChannelsRead)
	write := perm(rbac.PermTenantChannelsWrite)
	sync := perm(rbac.PermTenantChannelsSync)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}

	route("GET /v1/tenant/channel-accounts", h.ListAccounts, read)
	route("GET /v1/tenant/channel-accounts/{uuid}", h.GetAccount, read)
	route("PATCH /v1/tenant/channel-accounts/{uuid}", h.UpdateAccount, read, write)
	route("POST /v1/tenant/channel-accounts/{uuid}/push", h.Push, read, sync)
	route("POST /v1/tenant/channel-accounts/{uuid}/pull", h.Pull, read, sync)
	route("POST /v1/tenant/channel-accounts/{uuid}/simulate", h.Simulate, read, sync)
	route("GET /v1/tenant/channel-accounts/{uuid}/reconciliation", h.Reconciliation, read)

	// Static segments before /{uuid}.
	route("GET /v1/tenant/channel-mappings/facets", h.MappingFacets, read)
	route("GET /v1/tenant/channel-mappings", h.ListMappings, read)
	route("POST /v1/tenant/channel-mappings", h.CreateMapping, read, write)
	route("GET /v1/tenant/channel-mappings/{uuid}", h.GetMapping, read)
	route("PATCH /v1/tenant/channel-mappings/{uuid}", h.UpdateMapping, read, write)
	route("DELETE /v1/tenant/channel-mappings/{uuid}", h.DeleteMapping, read, write)

	route("GET /v1/tenant/channel-messages/facets", h.MessageFacets, read)
	route("GET /v1/tenant/channel-messages", h.ListMessages, read)
	route("GET /v1/tenant/channel-messages/{uuid}", h.GetMessage, read)
	route("POST /v1/tenant/channel-messages/{uuid}/retry", h.RetryMessage, read, sync)
	route("POST /v1/tenant/channel-messages/{uuid}/ignore", h.IgnoreMessage, read, sync)

	route("GET /v1/tenant/channel-sync-runs", h.ListSyncRuns, read)
}

// RegisterLiveResources declares the live list channels of accounts,
// mappings and messages and their read permission.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceAccounts, rbac.PermTenantChannelsRead)
	reg.Register(model.ResourceMappings, rbac.PermTenantChannelsRead)
	reg.Register(model.ResourceMessages, rbac.PermTenantChannelsRead)
}
