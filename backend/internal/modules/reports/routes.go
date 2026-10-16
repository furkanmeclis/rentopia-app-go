// Package reports is the reports section (TEC-738, §15.1):
// occupancy, income / expense, channel
// performance, receivable / payable aging and the branch comparison under
// /v1/tenant/reports/*, each exported through the io engine. The vehicle
// contribution is the vehicle profitability report (TEC-727).
//
// Permissions reuse the existing report slugs: tenant.finance.reports.read
// reads, tenant.report.export (with read) exports.
package reports

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Report describes one report: its path segment, io resource and the query
// keys its export accepts.
type Report struct {
	Path      string
	Resource  string
	QueryKeys []string
}

// Reports lists every report of the module.
var Reports = []Report{
	{Path: "occupancy", Resource: model.ResourceOccupancy, QueryKeys: usecase.OccupancyKeys},
	{Path: "income-expense", Resource: model.ResourceIncomeExpense, QueryKeys: usecase.IncomeExpenseKeys},
	{Path: "channels", Resource: model.ResourceChannels, QueryKeys: usecase.ChannelKeys},
	{Path: "aging", Resource: model.ResourceAging, QueryKeys: usecase.AgingKeys},
	{Path: "branches", Resource: model.ResourceBranches, QueryKeys: usecase.BranchKeys},
}

// ExportFactory builds the io engine export handler of one resource (nil:
// no export routes).
type ExportFactory func(resource string, queryKeys ...string) http.HandlerFunc

// RegisterRoutes mounts the reports. A nil resolver (tenancy off) mounts
// nothing.
func RegisterRoutes(
	mux *http.ServeMux,
	h *handler.Handler,
	export ExportFactory,
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
	route := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, middleware.Chain(h, authn, org, read))
	}
	exportRoute := func(pattern, resource string, keys []string) {
		if export != nil {
			mux.Handle(pattern, middleware.Chain(export(resource, keys...), authn, org, read, exp))
		}
	}
	route("GET /v1/tenant/reports/occupancy", h.Occupancy)
	exportRoute("POST /v1/tenant/reports/occupancy/export", model.ResourceOccupancy, usecase.OccupancyKeys)
	route("GET /v1/tenant/reports/income-expense", h.IncomeExpense)
	exportRoute("POST /v1/tenant/reports/income-expense/export", model.ResourceIncomeExpense, usecase.IncomeExpenseKeys)
	route("GET /v1/tenant/reports/channels", h.Channels)
	exportRoute("POST /v1/tenant/reports/channels/export", model.ResourceChannels, usecase.ChannelKeys)
	route("GET /v1/tenant/reports/aging", h.Aging)
	exportRoute("POST /v1/tenant/reports/aging/export", model.ResourceAging, usecase.AgingKeys)
	route("GET /v1/tenant/reports/branches", h.Branches)
	exportRoute("POST /v1/tenant/reports/branches/export", model.ResourceBranches, usecase.BranchKeys)
}
