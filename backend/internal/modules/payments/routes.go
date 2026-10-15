// Package payments mounts the payment routes (TEC-707,
// docs/design/payments.md):
//
//	tenant (RequireOrganization):
//	  GET  /v1/tenant/payments                        tenant.payments.read
//	  GET  /v1/tenant/payments/{uuid}                 tenant.payments.read
//	  POST /v1/tenant/payments/{uuid}/verify          tenant.payments.create_link
//	  POST /v1/tenant/payments/{uuid}/refund          tenant.payment.refund (TEC-689 slug)
//	  GET  /v1/tenant/payment-links                   tenant.payments.read
//	  GET  /v1/tenant/payment-links/{uuid}            tenant.payments.read
//	  POST /v1/tenant/payment-links/{uuid}/cancel     tenant.payments.create_link
//	  POST /v1/tenant/reservations/{uuid}/payment-links tenant.payments.create_link
//	  POST /v1/tenant/reservations/{uuid}/collections   tenant.payment.collect (TEC-689 slug)
//	public (no auth, rate-limited):
//	  GET  /v1/public/pay/{token}                     pay page (organization, reference, amount, state)
//	  POST /v1/public/pay/{token}/checkout            opens the provider's hosted checkout
//	  POST /v1/public/payments/iyzico/webhook         signature-verified notification
package payments

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/handler"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterRoutes mounts the tenant routes (a nil resolver mounts none)
// and the public routes.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	mux.HandleFunc("GET /v1/public/pay/{token}", h.PublicLink)
	mux.HandleFunc("POST /v1/public/pay/{token}/checkout", h.StartCheckout)
	mux.HandleFunc("POST /v1/public/payments/iyzico/webhook", h.Webhook("iyzico"))
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	read := perm(rbac.PermTenantPaymentsRead)
	link := perm(rbac.PermTenantPaymentsCreateLink)
	route("GET /v1/tenant/payments", h.List, read)
	route("GET /v1/tenant/payments/{uuid}", h.Get, read)
	route("POST /v1/tenant/payments/{uuid}/verify", h.Verify, read, link)
	route("POST /v1/tenant/payments/{uuid}/refund", h.Refund, read, perm(rbac.PermTenantPaymentRefund))
	route("GET /v1/tenant/payment-links", h.ListLinks, read)
	route("GET /v1/tenant/payment-links/{uuid}", h.GetLink, read)
	route("POST /v1/tenant/payment-links/{uuid}/cancel", h.CancelLink, link)
	route("POST /v1/tenant/reservations/{uuid}/payment-links", h.CreateLink, link)
	route("POST /v1/tenant/reservations/{uuid}/collections", h.Collect, perm(rbac.PermTenantPaymentCollect))
}

// RegisterLiveResources declares the live list channels.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(paymodel.ResourcePayments, rbac.PermTenantPaymentsRead)
	reg.Register(paymodel.ResourceLinks, rbac.PermTenantPaymentsRead)
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
