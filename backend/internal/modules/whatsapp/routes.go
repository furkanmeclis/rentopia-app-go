// Package whatsapp mounts the WhatsApp channel routes (TEC-735,
// docs/design/whatsapp.md):
//
//	GET    /v1/tenant/whatsapp/session            session state
//	PATCH  /v1/tenant/whatsapp/session            settings (rate limit)
//	DELETE /v1/tenant/whatsapp/session            remove the session
//	POST   /v1/tenant/whatsapp/session/connect    connect (risk notice)
//	GET    /v1/tenant/whatsapp/session/qr         pairing QR code
//	POST   /v1/tenant/whatsapp/session/refresh    read the state
//	POST   /v1/tenant/whatsapp/session/logout     unlink the phone
//	POST   /v1/tenant/whatsapp/test-messages      test send
//	GET    /v1/tenant/whatsapp/messages           message log
//	POST   /v1/public/whatsapp/webhook/{token}    provider webhook
//
// Every tenant route needs tenant.integrations.manage (the WhatsApp
// session is an integration account).
package whatsapp

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/whatsappfake"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/whatsapp/wuzapi"
)

// Providers builds the WhatsApp providers of the environment: wuzapi
// always (unusable until WUZAPI_BASE_URL / WUZAPI_ADMIN_TOKEN are set) and
// the fake outside production only. The fake links a session ten seconds
// after the QR code is shown.
func Providers(cfg config.WhatsAppConfig, appEnv string, _ *slog.Logger) ([]whatsapp.Provider, *whatsappfake.Fake) {
	out := []whatsapp.Provider{wuzapi.New(wuzapi.Config{BaseURL: cfg.WuzapiBaseURL, AdminToken: cfg.WuzapiAdminToken})}
	var fake *whatsappfake.Fake
	if !strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		fake = whatsappfake.New()
		fake.AutoPair = 10 * time.Second
		out = append(out, fake)
	}
	return out, fake
}

// Adapters lists providers as integration adapters (account registry).
func Adapters(providers []whatsapp.Provider) []integrations.Adapter {
	out := make([]integrations.Adapter, 0, len(providers))
	for _, p := range providers {
		out = append(out, p)
	}
	return out
}

// RegisterRoutes mounts the routes (a nil resolver mounts nothing).
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	manage := middleware.RequireTenantPermission(rbac.PermTenantIntegrationsManage)
	route := func(pattern string, fn http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(fn, authn, org, manage))
	}
	route("GET /v1/tenant/whatsapp/session", h.Session)
	route("PATCH /v1/tenant/whatsapp/session", h.UpdateSettings)
	route("DELETE /v1/tenant/whatsapp/session", h.Remove)
	route("POST /v1/tenant/whatsapp/session/connect", h.Connect)
	route("GET /v1/tenant/whatsapp/session/qr", h.QR)
	route("POST /v1/tenant/whatsapp/session/refresh", h.Refresh)
	route("POST /v1/tenant/whatsapp/session/logout", h.Logout)
	route("POST /v1/tenant/whatsapp/test-messages", h.TestSend)
	route("GET /v1/tenant/whatsapp/messages", h.Messages)
	mux.HandleFunc("POST /v1/public/whatsapp/webhook/{token}", h.Webhook)
}
