// Package customerportal is the customer portal (TEC-715,
// docs/design/customer-portal.md): /v1/customer/{orgSlug}/* for customers
// signed in with a reservation reference, their contact and a one-time
// code (own session middleware, never the staff JWT), and
// /v1/tenant/customer-requests where staff see and resolve the change /
// cancellation requests customers send.
//
// Permissions (staff): tenant.reservations.read lists / reads requests,
// tenant.reservations.write resolves them (requests belong to a
// reservation; acting on the reservation keeps its own permission).
package customerportal

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterRoutes mounts the portal routes. A nil resolver (tenancy off)
// mounts nothing: the portal is organization-scoped.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	// Customer routes: the organization comes from the URL.
	mux.HandleFunc("GET /v1/customer/{orgSlug}", h.Info)
	mux.HandleFunc("POST /v1/customer/{orgSlug}/auth/start", h.Start)
	mux.HandleFunc("POST /v1/customer/{orgSlug}/auth/verify", h.Verify)
	session := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, h.RequireSession(fn))
	}
	session("POST /v1/customer/{orgSlug}/auth/logout", h.Logout)
	session("GET /v1/customer/{orgSlug}/session", h.Session)
	session("GET /v1/customer/{orgSlug}/reservations", h.Reservations)
	session("GET /v1/customer/{orgSlug}/reservations/{uuid}", h.Reservation)
	session("GET /v1/customer/{orgSlug}/reservations/{uuid}/documents/{documentUuid}/pdf", h.DocumentPDF)
	session("GET /v1/customer/{orgSlug}/reservations/{uuid}/invoices/{invoiceUuid}/pdf", h.InvoicePDF)
	session("POST /v1/customer/{orgSlug}/reservations/{uuid}/pay", h.Pay)
	session("POST /v1/customer/{orgSlug}/reservations/{uuid}/requests", h.CreateRequest)

	// Staff routes.
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	read := perm(rbac.PermTenantReservationsRead)
	route("GET /v1/tenant/customer-requests", h.ListRequests, read)
	route("GET /v1/tenant/customer-requests/stats", h.RequestStats, read)
	route("GET /v1/tenant/customer-requests/{uuid}", h.GetRequest, read)
	route("POST /v1/tenant/customer-requests/{uuid}/resolve", h.ResolveRequest, read, perm(rbac.PermTenantReservationsWrite))
}

// Limiter adapts ratelimit.Limiter to the handler's RateLimiter.
type Limiter struct{ L *ratelimit.Limiter }

// Allow implements handler.RateLimiter.
func (l Limiter) Allow(r *http.Request, action, subject string, limit int, window time.Duration) bool {
	if l.L == nil {
		return true
	}
	ok, _ := l.L.Allow(r.Context(), action, subject, limit, window)
	return ok
}

// RegisterLiveResources declares the live channel of customer requests.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceRequests, rbac.PermTenantReservationsRead)
}
