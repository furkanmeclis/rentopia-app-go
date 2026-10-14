package catalog

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/catalog/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
)

// RegisterLiveResources declares the catalog list channels
// (resource:platform.catalog.*); watching them needs platform.catalog.read.
func RegisterLiveResources(reg *resourceevents.Registry) {
	for _, resource := range []string{
		usecase.LiveBrands, usecase.LiveModels, usecase.LiveTypes, usecase.LiveImports,
	} {
		reg.Register(resource, rbac.PermPlatformCatalogRead)
	}
}
