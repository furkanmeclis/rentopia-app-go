// Package reservations is the reservation core of M2: quotes (TEC-704,
// /v1/tenant/quotes), reservations with their state machine, capacity
// commitment and plate plan (TEC-706, /v1/tenant/reservations) and
// resolution tasks (TEC-698, /v1/tenant/resolution-tasks).
// docs/design/reservations.md is the design.
//
// Permissions: tenant.reservations.read / .write (create, edit, change,
// extend, plate, drivers, staff assignment) / .cancel (cancel, no-show) /
// .confirm (confirm) / .approve (staff approval queue: approve, accept,
// reject; TEC-708) / .export / .override_eligibility (in the
// use case), tenant.quotes.read / .write, tenant.resolution_tasks.read /
// .write. A manual discount needs tenant.discount.apply, keeping the price
// on a change tenant.price.override (checked in the use case).
package reservations

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug of reservations.
const Resource = adapters.Resource

// ExportQueryKeys are the query keys an export honours (list filters).
var ExportQueryKeys = append([]string{"q"}, model.QueryKeys...)

// EngineHandlers are the shared engine handlers the resource mounts its
// export / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	Export http.HandlerFunc
	Bulk   http.HandlerFunc
}

// RegisterRoutes mounts the reservation routes. A nil resolver (tenancy
// off) mounts nothing.
func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, engines EngineHandlers, tokens *jwt.Manager, loader middleware.IdentityLoader, resolver orgctx.Resolver) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	perm := middleware.RequireTenantPermission
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	read := perm(rbac.PermTenantReservationsRead)
	write := perm(rbac.PermTenantReservationsWrite)
	cancel := perm(rbac.PermTenantReservationsCancel)
	confirm := perm(rbac.PermTenantReservationsConfirm)

	// Static segments before /{uuid}.
	route("GET /v1/tenant/reservations/meta", h.Meta, read)
	route("GET /v1/tenant/reservations/facets", h.Facets, read)
	route("GET /v1/tenant/reservations", h.List, read)
	route("POST /v1/tenant/reservations", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/reservations/export", engines.Export, read, perm(rbac.PermTenantReservationsExport))
	}
	if engines.Bulk != nil {
		// The bulk handler checks the action permission (cancel / write).
		route("POST /v1/tenant/reservations/bulk", engines.Bulk, read)
	}
	route("GET /v1/tenant/reservations/{uuid}", h.Get, read)
	route("GET /v1/tenant/reservations/{uuid}/events", h.Events, read)
	route("POST /v1/tenant/reservations/{uuid}/submit", h.Submit, write)
	approve := perm(rbac.PermTenantReservationsApprove)
	route("POST /v1/tenant/reservations/{uuid}/approve", h.Approve, approve)
	route("POST /v1/tenant/reservations/{uuid}/accept", h.Accept, approve)
	route("POST /v1/tenant/reservations/{uuid}/reject", h.Reject, approve)
	route("POST /v1/tenant/reservations/{uuid}/confirm", h.Confirm, confirm)
	route("POST /v1/tenant/reservations/{uuid}/cancel", h.Cancel, cancel)
	route("POST /v1/tenant/reservations/{uuid}/no-show", h.NoShow, cancel)
	route("POST /v1/tenant/reservations/{uuid}/quotes", h.ChangeQuote, write, perm(rbac.PermTenantQuotesWrite))
	route("POST /v1/tenant/reservations/{uuid}/change", h.ApplyChange, write)
	route("POST /v1/tenant/reservations/{uuid}/extend", h.Extend, write)
	route("GET /v1/tenant/reservations/{uuid}/plate-candidates", h.PlateCandidates, write)
	route("POST /v1/tenant/reservations/{uuid}/plate", h.Plate, write)
	route("PUT /v1/tenant/reservations/{uuid}/drivers", h.Drivers, write)
	route("POST /v1/tenant/reservations/{uuid}/assign", h.Assign, write)

	quotesRead := perm(rbac.PermTenantQuotesRead)
	quotesWrite := perm(rbac.PermTenantQuotesWrite)
	route("GET /v1/tenant/quotes/meta", h.QuotesMeta, quotesRead)
	route("GET /v1/tenant/quotes", h.ListQuotes, quotesRead)
	route("POST /v1/tenant/quotes", h.CreateQuote, quotesWrite)
	route("POST /v1/tenant/quotes/search", h.SearchQuotes, quotesRead)
	route("GET /v1/tenant/quotes/{uuid}", h.GetQuote, quotesRead)

	tasksRead := perm(rbac.PermTenantResolutionTasksRead)
	tasksWrite := perm(rbac.PermTenantResolutionTasksWrite)
	route("GET /v1/tenant/resolution-tasks/meta", h.TasksMeta, tasksRead)
	route("GET /v1/tenant/resolution-tasks", h.ListTasks, tasksRead)
	route("GET /v1/tenant/resolution-tasks/{uuid}", h.GetTask, tasksRead)
	route("POST /v1/tenant/resolution-tasks/{uuid}/resolve", h.ResolveTask, tasksWrite)
	route("POST /v1/tenant/resolution-tasks/{uuid}/assign", h.AssignTask, tasksWrite)
	route("POST /v1/tenant/resolution-tasks/{uuid}/replan", h.ReplanTask, tasksWrite)
}

// RegisterLiveResources declares the live list channels and their read
// permissions.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceReservations, rbac.PermTenantReservationsRead)
	reg.Register(model.ResourceQuotes, rbac.PermTenantQuotesRead)
	reg.Register(model.ResourceTasks, rbac.PermTenantResolutionTasksRead)
}
