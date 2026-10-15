// Package rentals is the rental record of M3 (TEC-709): rentals born from
// confirmed reservations (/v1/tenant/rentals: handover, return, return
// check, extension, vehicle swap, charges) and the operation tasks of the
// branches (/v1/tenant/operation-tasks). docs/design/rentals.md is the
// design.
//
// Permissions: tenant.rentals.read (lists, details, tasks) / .write
// (create, handover, return, extend, swap, propose charges, work tasks) /
// .close (finish the return check, cancel before the handover, waive or
// approve charges — damage approvals need tenant.damage.approve, checked
// in the use case). A handover without an approved contract needs
// tenant.signature.waive (use case).
package rentals

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug of rentals.
const Resource = model.ResourceRentals

// ExportQueryKeys are the query keys an export honours (list filters).
var ExportQueryKeys = append([]string{"q"}, model.QueryKeys...)

// EngineHandlers are the shared engine handlers the resource mounts its
// export route on. A nil member skips the route.
type EngineHandlers struct {
	Export http.HandlerFunc
}

// RegisterRoutes mounts the rental and operation task routes. A nil
// resolver (tenancy off) mounts nothing.
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
	read := perm(rbac.PermTenantRentalsRead)
	write := perm(rbac.PermTenantRentalsWrite)
	closePerm := perm(rbac.PermTenantRentalsClose)

	// Static segments before /{uuid}.
	route("GET /v1/tenant/rentals/meta", h.Meta, read)
	route("GET /v1/tenant/rentals/facets", h.Facets, read)
	route("GET /v1/tenant/rentals", h.List, read)
	route("POST /v1/tenant/rentals", h.Create, write)
	if engines.Export != nil {
		route("POST /v1/tenant/rentals/export", engines.Export, read)
	}
	route("GET /v1/tenant/rentals/{uuid}", h.Get, read)
	route("GET /v1/tenant/rentals/{uuid}/events", h.Events, read)
	route("POST /v1/tenant/rentals/{uuid}/handover", h.Handover, write)
	route("POST /v1/tenant/rentals/{uuid}/return", h.Return, write)
	route("POST /v1/tenant/rentals/{uuid}/extend", h.Extend, write)
	route("POST /v1/tenant/rentals/{uuid}/swap", h.Swap, write)
	route("POST /v1/tenant/rentals/{uuid}/charges", h.AddCharge, write)
	route("POST /v1/tenant/rentals/{uuid}/complete-return", h.CompleteReturn, closePerm)
	route("POST /v1/tenant/rentals/{uuid}/cancel", h.Cancel, closePerm)
	// Approve / waive check close or damage.approve in the use case.
	route("POST /v1/tenant/rentals/{uuid}/charges/{chargeUuid}/approve", h.ApproveCharge, read)
	route("POST /v1/tenant/rentals/{uuid}/charges/{chargeUuid}/waive", h.WaiveCharge, read)

	route("GET /v1/tenant/operation-tasks/meta", h.TasksMeta, read)
	route("GET /v1/tenant/operation-tasks/facets", h.TaskFacets, read)
	route("GET /v1/tenant/operation-tasks", h.ListTasks, read)
	route("GET /v1/tenant/operation-tasks/{uuid}", h.GetTask, read)
	route("POST /v1/tenant/operation-tasks/{uuid}/assign", h.AssignTask, write)
	route("POST /v1/tenant/operation-tasks/{uuid}/start", h.StartTask, write)
	route("POST /v1/tenant/operation-tasks/{uuid}/complete", h.CompleteTask, write)
	route("POST /v1/tenant/operation-tasks/{uuid}/cancel", h.CancelTask, write)
}

// RegisterLiveResources declares the live list channels and their read
// permissions.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.ResourceRentals, rbac.PermTenantRentalsRead)
	reg.Register(model.ResourceOperationTasks, rbac.PermTenantRentalsRead)
}
