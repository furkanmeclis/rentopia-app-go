// Package profitability is the vehicle profitability report (TEC-727,
// §11.5, docs/design/vehicle-profitability.md): the fleet report
// (/v1/tenant/reports/vehicle-profitability), the vehicle detail tab
// (…/vehicles/{uuid}) and the export through the io engine.
//
// Permissions reuse the existing report slugs: tenant.finance.reports.read
// reads, tenant.report.export (with read) exports.
package profitability

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Resource is the io engine slug of the export.
const Resource = model.Resource

// ExportQueryKeys are the query keys the export accepts.
var ExportQueryKeys = usecase.QueryKeys

// RegisterRoutes mounts the report. A nil resolver (tenancy off) mounts
// nothing; a nil export handler skips the export route.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	export http.HandlerFunc,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantFinanceReportsRead)
	exp := middleware.RequireTenantPermission(rbac.PermTenantReportExport)
	mux.Handle("GET /v1/tenant/reports/vehicle-profitability", middleware.Chain(http.HandlerFunc(h.Report), authn, org, read))
	if export != nil {
		mux.Handle("POST /v1/tenant/reports/vehicle-profitability/export", middleware.Chain(export, authn, org, read, exp))
	}
	mux.Handle("GET /v1/tenant/reports/vehicle-profitability/vehicles/{uuid}", middleware.Chain(http.HandlerFunc(h.Vehicle), authn, org, read))
}
