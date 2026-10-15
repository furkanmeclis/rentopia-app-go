// Package inspections is the mobile handover / return flow of M3 (TEC-712,
// TEC-713) with its evidence storage (TEC-714): inspection templates,
// handover and return forms (versioned, immutable once finalized),
// damages on the vehicle diagram, photos (API-issued upload, server-side
// SHA-256, short-lived media links), signatures and the return charges.
// docs/design/inspections.md is the design.
//
// Permissions (no new slugs): tenant.rentals.read (forms, templates, media
// links) / .write (fill, photograph, sign, complete) / .close (save a
// template version, revise a finalized form, resolve the review queue).
// Checked in the use case: tenant.price.override (change a computed
// charge), tenant.signature.waive (hand over without a signature, accept
// an unsigned return), tenant.damage.approve (approve a disputed charge,
// rentals).
package inspections

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/inspections/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterRoutes mounts the inspection routes. A nil resolver (tenancy
// off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	read := perm(rbac.PermTenantRentalsRead)
	write := perm(rbac.PermTenantRentalsWrite)
	closePerm := perm(rbac.PermTenantRentalsClose)

	route("GET /v1/tenant/inspection-templates", h.ListTemplates, read)
	route("POST /v1/tenant/inspection-templates", h.SaveTemplate, closePerm)
	route("GET /v1/tenant/inspection-templates/active", h.ActiveTemplate, read)
	route("GET /v1/tenant/inspection-templates/{uuid}", h.GetTemplate, read)

	route("GET /v1/tenant/inspections/facets", h.Facets, read)
	route("GET /v1/tenant/inspections", h.List, read)
	route("POST /v1/tenant/inspections", h.Start, write)
	route("GET /v1/tenant/inspections/{uuid}", h.Get, read)
	route("PATCH /v1/tenant/inspections/{uuid}", h.Update, write)
	route("GET /v1/tenant/inspections/{uuid}/charges", h.Charges, read)
	route("POST /v1/tenant/inspections/{uuid}/media-links", h.MediaLinks, read)
	route("POST /v1/tenant/inspections/{uuid}/damages", h.AddDamage, write)
	route("PATCH /v1/tenant/inspections/{uuid}/damages/{damageUuid}", h.UpdateDamage, write)
	route("DELETE /v1/tenant/inspections/{uuid}/damages/{damageUuid}", h.DeleteDamage, write)
	route("POST /v1/tenant/inspections/{uuid}/photos", h.AddPhoto, write)
	route("PUT /v1/tenant/inspections/{uuid}/photos/{photoUuid}/file", h.UploadPhoto, write)
	route("POST /v1/tenant/inspections/{uuid}/photos/{photoUuid}/complete", h.CompletePhoto, write)
	route("DELETE /v1/tenant/inspections/{uuid}/photos/{photoUuid}", h.DeletePhoto, write)
	route("POST /v1/tenant/inspections/{uuid}/signature", h.Signature, write)
	route("POST /v1/tenant/inspections/{uuid}/complete", h.Complete, write)
	route("POST /v1/tenant/inspections/{uuid}/revise", h.Revise, closePerm)
	route("POST /v1/tenant/inspections/{uuid}/review", h.Review, closePerm)

	// Short-lived signed media links (the token is the authorization).
	mux.HandleFunc("GET /v1/public/media/inspections/{token}", h.Media)
}

// RegisterLiveResources declares the live list channels.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceInspections, rbac.PermTenantRentalsRead)
	reg.Register(model.ResourceTemplates, rbac.PermTenantRentalsRead)
}
