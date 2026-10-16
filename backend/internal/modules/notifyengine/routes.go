// Package notifyengine mounts the notification engine routes (TEC-733,
// docs/design/notification-engine.md):
//
//	GET  /v1/tenant/notification-events                     read
//	PUT  /v1/tenant/notification-rules                      write
//	POST /v1/tenant/notification-rules/reset                write
//	GET  /v1/tenant/notification-templates                  read
//	PUT  /v1/tenant/notification-templates                  write
//	POST /v1/tenant/notification-templates/reset            write
//	POST /v1/tenant/notification-templates/preview          read
//	POST /v1/tenant/notification-tests                      write
//	GET  /v1/tenant/notification-deliveries[/meta|/facets]  read
//	GET  /v1/tenant/notification-dispatches/{uuid}          read
//
// Rules and templates are addressed by catalog codes in the body (event,
// channel, language), never by another organization's ids.
//
// read = tenant.notification_settings.read, write = …write. The customer
// portal's web push subscription lives under /v1/customer/{orgSlug}.
package notifyengine

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the staff routes (a nil resolver mounts nothing).
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantNotificationSettingsRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantNotificationSettingsWrite)
	route := func(pattern string, fn http.HandlerFunc, perm func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, perm))
	}
	route("GET /v1/tenant/notification-events", h.Events, read)
	route("PUT /v1/tenant/notification-rules", h.PutRule, write)
	route("POST /v1/tenant/notification-rules/reset", h.ResetRule, write)
	route("GET /v1/tenant/notification-templates", h.Templates, read)
	route("PUT /v1/tenant/notification-templates", h.PutTemplate, write)
	route("POST /v1/tenant/notification-templates/reset", h.ResetTemplate, write)
	route("POST /v1/tenant/notification-templates/preview", h.Preview, read)
	route("POST /v1/tenant/notification-tests", h.Test, write)
	route("GET /v1/tenant/notification-deliveries/meta", h.DeliveriesMeta, read)
	route("GET /v1/tenant/notification-deliveries/facets", h.DeliveryFacets, read)
	route("GET /v1/tenant/notification-deliveries", h.Deliveries, read)
	route("GET /v1/tenant/notification-dispatches/{uuid}", h.Dispatch, read)
}

// RegisterCustomerRoutes mounts the customer portal's web push routes
// behind the portal session middleware (customerportal handler
// RequireSession). A nil session mounts nothing.
func RegisterCustomerRoutes(mux *http.ServeMux, h *handler.Handler, session func(http.Handler) http.Handler) {
	if session == nil {
		return
	}
	mux.Handle("GET /v1/customer/{orgSlug}/push/vapid-public-key", session(http.HandlerFunc(h.CustomerVAPIDKey)))
	mux.Handle("POST /v1/customer/{orgSlug}/push-subscriptions", session(http.HandlerFunc(h.CustomerSubscribe)))
	mux.Handle("DELETE /v1/customer/{orgSlug}/push-subscriptions", session(http.HandlerFunc(h.CustomerUnsubscribe)))
}
