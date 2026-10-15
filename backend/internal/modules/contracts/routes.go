// Package contracts is contract templates and documents (TEC-710):
// /v1/tenant/contract-templates (versioned rich text templates with
// placeholders, preview) and /v1/tenant/documents (rendered, hashed,
// immutable versions of a reservation's / rental's contract, forms and
// terms; approvals bound to the content hash; private PDFs).
// docs/design/contracts.md is the design.
//
// Permissions: tenant.contracts.read (templates, documents) / .write
// (template drafts, new versions) / .activate (activate a version);
// preparing documents and recording approvals needs tenant.contracts.write
// or tenant.rentals.write (use case); PDFs need tenant.document.download.
package contracts

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterRoutes mounts the routes. A nil resolver mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	// Public remote approval page (TEC-711): the token names the link and
	// its organization; per-IP / per-token limits in the handler.
	mux.HandleFunc("GET /v1/public/documents/{token}", h.PublicDocument)
	mux.HandleFunc("POST /v1/public/documents/{token}/code", h.PublicRequestCode)
	mux.HandleFunc("POST /v1/public/documents/{token}/approve", h.PublicApprove)
	mux.HandleFunc("GET /v1/public/documents/{token}/pdf", h.PublicPDF)
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	read := perm(rbac.PermTenantContractsRead)
	write := perm(rbac.PermTenantContractsWrite)
	activate := perm(rbac.PermTenantContractsActivate)

	route("GET /v1/tenant/contract-templates/meta", h.TemplatesMeta, read)
	route("GET /v1/tenant/contract-templates/placeholders", h.Placeholders, read)
	route("GET /v1/tenant/contract-templates", h.ListTemplates, read)
	route("POST /v1/tenant/contract-templates", h.CreateTemplate, write)
	route("POST /v1/tenant/contract-templates/preview", h.Preview, read)
	route("GET /v1/tenant/contract-templates/{uuid}", h.GetTemplate, read)
	route("PATCH /v1/tenant/contract-templates/{uuid}", h.UpdateTemplate, write)
	route("DELETE /v1/tenant/contract-templates/{uuid}", h.DeleteTemplate, write)
	route("POST /v1/tenant/contract-templates/{uuid}/activate", h.ActivateTemplate, activate)
	route("POST /v1/tenant/contract-templates/{uuid}/new-version", h.NewVersion, write)

	// Prepare / refresh / issue / approve check contracts.write or
	// rentals.write in the use case.
	route("GET /v1/tenant/documents", h.ListDocuments, read)
	route("POST /v1/tenant/documents", h.PrepareDocument, read)
	route("GET /v1/tenant/documents/{uuid}", h.GetDocument, read)
	route("POST /v1/tenant/documents/{uuid}/refresh", h.RefreshDocument, read)
	route("GET /v1/tenant/documents/{uuid}/versions/{documentVersionUuid}", h.GetVersion, read)
	route("POST /v1/tenant/documents/{uuid}/versions/{documentVersionUuid}/issue", h.IssueVersion, read)
	route("POST /v1/tenant/documents/{uuid}/versions/{documentVersionUuid}/approve", h.ApproveVersion, read)
	route("GET /v1/tenant/documents/{uuid}/versions/{documentVersionUuid}/pdf", h.DownloadPDF, read, perm(rbac.PermTenantDocumentDownload))

	// Remote approval links (TEC-711): create / send / revoke check
	// contracts.write or rentals.write in the use case, as approvals do.
	route("GET /v1/tenant/documents/{uuid}/signers", h.Signers, read)
	route("GET /v1/tenant/documents/{uuid}/approval-links", h.ListApprovalLinks, read)
	route("POST /v1/tenant/documents/{uuid}/versions/{documentVersionUuid}/approval-links", h.CreateApprovalLink, read)
	route("POST /v1/tenant/documents/{uuid}/approval-links/{linkUuid}/send-sms", h.SendApprovalLink, read)
	route("POST /v1/tenant/documents/{uuid}/approval-links/{linkUuid}/revoke", h.RevokeApprovalLink, read)
	route("GET /v1/tenant/documents/{uuid}/approvals/{approvalUuid}/pdf", h.DownloadApprovalPDF, read, perm(rbac.PermTenantDocumentDownload))
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

// RegisterLiveResources declares the live channels.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceTemplates, rbac.PermTenantContractsRead)
	reg.Register(model.ResourceDocuments, rbac.PermTenantContractsRead)
}
