// Package fieldtasks is the field task module (TEC-724, RT-A29): delivery
// to an address or an airport, collection, transfers and other jobs of
// the field staff, on the operation_tasks rows rentals open. Routes:
// /v1/tenant/field-tasks (the board, "my tasks", the state machine, bulk
// assign) and /v1/tenant/delivery-points (the wizard's delivery picker).
// docs/design/field-tasks.md is the design.
//
// Permissions: tenant.field_tasks.read (board, facets, timeline) /
// .assign (create, re-plan, assign, bulk assign, cancel) / .execute ("my
// tasks": start, report an issue, complete the caller's own tasks). A task
// route that needs a task (GET, timeline, start, report, complete) admits
// any of the three; the use case narrows executors to their own tasks.
package fieldtasks

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// Resource is the engine slug of field tasks (bulk).
const Resource = model.Resource

// EngineHandlers are the shared engine handlers the module mounts.
type EngineHandlers struct {
	Bulk http.HandlerFunc
}

// RegisterRoutes mounts the field task routes. A nil resolver (tenancy off)
// mounts nothing.
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
	read := perm(rbac.PermTenantFieldTasksRead)
	assign := perm(rbac.PermTenantFieldTasksAssign)
	execute := perm(rbac.PermTenantFieldTasksExecute)
	anyRole := middleware.RequireAnyTenantPermission(
		rbac.PermTenantFieldTasksRead, rbac.PermTenantFieldTasksAssign, rbac.PermTenantFieldTasksExecute,
	)

	// Static segments before /{uuid}.
	route("GET /v1/tenant/field-tasks/meta", h.Meta, read)
	route("GET /v1/tenant/field-tasks/facets", h.Facets, read)
	route("GET /v1/tenant/field-tasks/assignees", h.Assignees, anyRole)
	route("GET /v1/tenant/field-tasks/mine", h.Mine, execute)
	route("GET /v1/tenant/field-tasks", h.List, read)
	route("POST /v1/tenant/field-tasks", h.Create, assign)
	if engines.Bulk != nil {
		route("POST /v1/tenant/field-tasks/bulk", engines.Bulk, read)
	}
	route("GET /v1/tenant/field-tasks/{uuid}", h.Get, anyRole)
	route("GET /v1/tenant/field-tasks/{uuid}/events", h.Events, anyRole)
	route("PATCH /v1/tenant/field-tasks/{uuid}", h.Patch, assign)
	route("POST /v1/tenant/field-tasks/{uuid}/assign", h.Assign, assign)
	route("POST /v1/tenant/field-tasks/{uuid}/start", h.Start, anyRole)
	route("POST /v1/tenant/field-tasks/{uuid}/report-issue", h.ReportIssue, anyRole)
	route("POST /v1/tenant/field-tasks/{uuid}/complete", h.Complete, anyRole)
	route("POST /v1/tenant/field-tasks/{uuid}/cancel", h.Cancel, assign)

	route("GET /v1/tenant/delivery-points", h.DeliveryPoints, perm(rbac.PermTenantQuotesRead))
}

// RegisterLiveResources declares the live list channels and their read
// permissions (the board; the executors' own list).
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(model.Resource, rbac.PermTenantFieldTasksRead)
	reg.Register(model.ResourceMine, rbac.PermTenantFieldTasksExecute)
}
