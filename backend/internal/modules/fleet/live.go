package fleet

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterLiveResources declares the fleet list channels
// (org:{org_uuid}:resource:tenant.segments, tenant.rental_groups,
// tenant.rental_products) and their tenant read permission. reg is the
// tenant registry checked against the organization membership.
func RegisterLiveResources(reg *resourceevents.Registry) {
	reg.Register(ResourceSegments, rbac.PermTenantFleetRead)
	reg.Register(ResourceRentalGroups, rbac.PermTenantFleetRead)
	reg.Register(ResourceRentalProducts, rbac.PermTenantFleetRead)
}
