// Package fleet is the fleet classification of an organization (TEC-691):
// vehicle segments (/v1/tenant/segments), rental groups
// (/v1/tenant/rental-groups) and versioned rentable products
// (/v1/tenant/products). Physical vehicles come later and are matched to
// groups and guarantees with package eligibility.
package fleet

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/middleware"
	fleethandler "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/handler"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/jwt"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
)

// Engine resource slugs.
const (
	ResourceSegments       = model.ResourceSegments
	ResourceRentalGroups   = model.ResourceRentalGroups
	ResourceRentalProducts = model.ResourceRentalProducts
)

// Export query keys per resource (the list filters an export honours).
var (
	SegmentExportKeys = model.SegmentQueryKeys
	GroupExportKeys   = model.GroupQueryKeys
	ProductExportKeys = model.ProductQueryKeys
)

// EngineHandlers are the shared engine handlers the resources mount their
// export / bulk routes on. Nil members skip those routes.
type EngineHandlers struct {
	ExportSegments http.HandlerFunc
	ExportGroups   http.HandlerFunc
	ExportProducts http.HandlerFunc
	BulkGroups     http.HandlerFunc
	BulkProducts   http.HandlerFunc
}

// RegisterRoutes mounts the fleet routes. The public image stream is always
// mounted; the tenant routes only when tenancy is on (resolver != nil).
func RegisterRoutes(
	mux *http.ServeMux,
	h *fleethandler.Handler,
	engines EngineHandlers,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
	resolver orgctx.Resolver,
) {
	// Product images are public media (websites and channels show them).
	mux.HandleFunc("GET /v1/public/media/rental-products/{orgUuid}/images/{imageUuid}", h.PublicImage)

	if resolver == nil {
		return
	}
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(resolver)
	read := middleware.RequireTenantPermission(rbac.PermTenantFleetRead)
	write := middleware.RequireTenantPermission(rbac.PermTenantFleetWrite)
	route := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		mux.Handle(pattern, middleware.Chain(fn, append([]func(http.Handler) http.Handler{authn, org}, guards...)...))
	}
	optional := func(pattern string, fn http.HandlerFunc, guards ...func(http.Handler) http.Handler) {
		if fn != nil {
			route(pattern, fn, guards...)
		}
	}

	// Static segments before /{uuid}.
	route("GET /v1/tenant/segments/meta", h.SegmentsMeta, read)
	route("GET /v1/tenant/segments/facets", h.SegmentFacets, read)
	route("GET /v1/tenant/segments", h.ListSegments, read)
	route("POST /v1/tenant/segments", h.CreateSegment, write)
	optional("POST /v1/tenant/segments/export", engines.ExportSegments, read)
	route("GET /v1/tenant/segments/{uuid}", h.GetSegment, read)
	route("PATCH /v1/tenant/segments/{uuid}", h.PatchSegment, write)
	route("DELETE /v1/tenant/segments/{uuid}", h.DeleteSegment, write)
	route("GET /v1/tenant/segments/{uuid}/activity", h.SegmentActivity, read)

	route("GET /v1/tenant/rental-groups/meta", h.GroupsMeta, read)
	route("GET /v1/tenant/rental-groups/facets", h.GroupFacets, read)
	route("GET /v1/tenant/rental-groups", h.ListGroups, read)
	route("POST /v1/tenant/rental-groups", h.CreateGroup, write)
	optional("POST /v1/tenant/rental-groups/export", engines.ExportGroups, read)
	// The bulk handler checks the per-action permission in the scope.
	optional("POST /v1/tenant/rental-groups/bulk", engines.BulkGroups, read)
	route("GET /v1/tenant/rental-groups/{uuid}", h.GetGroup, read)
	route("PATCH /v1/tenant/rental-groups/{uuid}", h.PatchGroup, write)
	route("DELETE /v1/tenant/rental-groups/{uuid}", h.DeleteGroup, write)
	route("GET /v1/tenant/rental-groups/{uuid}/activity", h.GroupActivity, read)

	route("GET /v1/tenant/products/meta", h.ProductsMeta, read)
	route("GET /v1/tenant/products/facets", h.ProductFacets, read)
	route("GET /v1/tenant/products", h.ListProducts, read)
	route("POST /v1/tenant/products", h.CreateProduct, write)
	optional("POST /v1/tenant/products/export", engines.ExportProducts, read)
	optional("POST /v1/tenant/products/bulk", engines.BulkProducts, read)
	route("GET /v1/tenant/products/{uuid}", h.GetProduct, read)
	route("PATCH /v1/tenant/products/{uuid}", h.PatchProduct, write)
	route("DELETE /v1/tenant/products/{uuid}", h.DeleteProduct, write)
	route("GET /v1/tenant/products/{uuid}/activity", h.ProductActivity, read)
	route("GET /v1/tenant/products/{uuid}/versions", h.ListVersions, read)
	route("GET /v1/tenant/products/{uuid}/versions/{versionUuid}", h.GetVersion, read)
	route("POST /v1/tenant/products/{uuid}/images", h.UploadImage, write)
	route("PUT /v1/tenant/products/{uuid}/images/order", h.ReorderImages, write)
	route("DELETE /v1/tenant/products/{uuid}/images/{imageUuid}", h.DeleteImage, write)
}
