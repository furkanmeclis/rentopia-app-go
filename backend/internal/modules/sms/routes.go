// Package sms mounts the tenant SMS routes (TEC-734, docs/design/sms.md):
//
//	POST /v1/tenant/integrations/{uuid}/test-sms   tenant.integrations.manage
//
// OTP issue / verify has no route of its own: features call
// usecase.Service.IssueOTP / VerifyOTP (TEC-711 contract approval).
package sms

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/features"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// RegisterRoutes mounts the routes (a nil resolver mounts nothing).
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	mux.Handle("POST /v1/tenant/integrations/{uuid}/test-sms", middleware.Chain(http.HandlerFunc(h.TestSend),
		middleware.Authenticate(tokens, loader), middleware.RequireOrganization(resolver),
		middleware.RequireFeature(features.SMS), // TEC-674
		middleware.RequireTenantPermission(rbac.PermTenantIntegrationsManage)))
}
