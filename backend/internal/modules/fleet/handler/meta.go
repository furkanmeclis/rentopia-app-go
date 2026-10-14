package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleet/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Capabilities match the live routes: export and bulk (groups, products)
// are mounted by routes.go; import is not offered.

// SegmentsMeta is the table contract of GET /v1/tenant/segments.
func SegmentsMeta() rm.ResourceMeta {
	return rm.ResourceMeta{
		Resource:      model.ResourceSegments,
		DefaultSort:   "sort_order",
		DefaultFields: []string{"uuid", "code", "name", "sort_order", "groups_count", "products_count", "is_active"},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true,
		},
		SearchableFields: []string{"code", "name"},
		SortableFields:   []string{"code", "name", "sort_order", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"is_active"},
		Columns: []rm.Column{
			{Key: "code", LabelKey: "fleet.segments.columns.code", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "name", LabelKey: "fleet.segments.columns.name", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "sort_order", LabelKey: "fleet.segments.columns.sort_order", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "groups_count", LabelKey: "fleet.segments.columns.groups_count", Type: rm.ColumnTypeString, DefaultVisible: true},
			{Key: "products_count", LabelKey: "fleet.segments.columns.products_count", Type: rm.ColumnTypeString, DefaultVisible: true},
			{Key: "is_active", LabelKey: "fleet.segments.columns.is_active", Type: rm.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "fleet.segments.columns.created_at", Type: rm.ColumnTypeDatetime, Sortable: true},
			{Key: "uuid", LabelKey: "fleet.segments.columns.uuid", Type: rm.ColumnTypeUUID},
		},
		Filters: []rm.Filter{
			{Key: "is_active", LabelKey: "fleet.segments.columns.is_active", Variant: rm.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// GroupsMeta is the table contract of GET /v1/tenant/rental-groups.
func GroupsMeta() rm.ResourceMeta {
	return rm.ResourceMeta{
		Resource:      model.ResourceRentalGroups,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "code", "name", "segment", "transmission", "fuel", "body_type", "acriss_code", "products_count", "is_active"},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Bulk: true,
		},
		SearchableFields: []string{"code", "name", "acriss_code"},
		SortableFields:   []string{"code", "name", "acriss_code", "min_seats", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"is_active", "segment", "transmission", "fuel", "body_type"},
		Columns: []rm.Column{
			{Key: "code", LabelKey: "fleet.groups.columns.code", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "name", LabelKey: "fleet.groups.columns.name", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "segment", LabelKey: "fleet.groups.columns.segment", Type: rm.ColumnTypeEnum, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "transmission", LabelKey: "fleet.groups.columns.transmission", Type: rm.ColumnTypeEnum, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "fuel", LabelKey: "fleet.groups.columns.fuel", Type: rm.ColumnTypeEnum, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "body_type", LabelKey: "fleet.groups.columns.body_type", Type: rm.ColumnTypeEnum, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "min_seats", LabelKey: "fleet.groups.columns.min_seats", Type: rm.ColumnTypeString, Sortable: true},
			{Key: "acriss_code", LabelKey: "fleet.groups.columns.acriss_code", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "products_count", LabelKey: "fleet.groups.columns.products_count", Type: rm.ColumnTypeString, DefaultVisible: true},
			{Key: "is_active", LabelKey: "fleet.groups.columns.is_active", Type: rm.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "fleet.groups.columns.created_at", Type: rm.ColumnTypeDatetime, Sortable: true},
			{Key: "uuid", LabelKey: "fleet.groups.columns.uuid", Type: rm.ColumnTypeUUID},
		},
		Filters: []rm.Filter{
			{Key: "is_active", LabelKey: "fleet.groups.columns.is_active", Variant: rm.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "segment", LabelKey: "fleet.groups.columns.segment", Variant: rm.FilterVariantFaceted, EnumLookup: "rental_groups.facets.segment"},
			{Key: "transmission", LabelKey: "fleet.groups.columns.transmission", Variant: rm.FilterVariantFaceted, EnumLookup: "fleet.transmission"},
			{Key: "fuel", LabelKey: "fleet.groups.columns.fuel", Variant: rm.FilterVariantFaceted, EnumLookup: "fleet.fuel"},
			{Key: "body_type", LabelKey: "fleet.groups.columns.body_type", Variant: rm.FilterVariantFaceted, EnumLookup: "fleet.body_type"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewGroupsBulk(nil).BulkActions(),
	}
}

// ProductsMeta is the table contract of GET /v1/tenant/products.
func ProductsMeta() rm.ResourceMeta {
	return rm.ResourceMeta{
		Resource:      model.ResourceRentalProducts,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "code", "title", "segment", "guarantee_type", "display_text", "status", "current_version", "is_active"},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Bulk: true,
		},
		SearchableFields: []string{"code", "title"},
		SortableFields:   []string{"code", "title", "status", "current_version", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"is_active", "segment", "status", "guarantee_type", "rental_group"},
		Columns: []rm.Column{
			{Key: "code", LabelKey: "fleet.products.columns.code", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "title", LabelKey: "fleet.products.columns.title", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "segment", LabelKey: "fleet.products.columns.segment", Type: rm.ColumnTypeEnum, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "guarantee_type", LabelKey: "fleet.products.columns.guarantee_type", Type: rm.ColumnTypeEnum, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "display_text", LabelKey: "fleet.products.columns.display_text", Type: rm.ColumnTypeString, DefaultVisible: true},
			{Key: "status", LabelKey: "fleet.products.columns.status", Type: rm.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "current_version", LabelKey: "fleet.products.columns.current_version", Type: rm.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "is_active", LabelKey: "fleet.products.columns.is_active", Type: rm.ColumnTypeBoolean, Sortable: true, Filterable: true, FilterVariant: rm.FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "fleet.products.columns.created_at", Type: rm.ColumnTypeDatetime, Sortable: true},
			{Key: "updated_at", LabelKey: "fleet.products.columns.updated_at", Type: rm.ColumnTypeDatetime, Sortable: true},
			{Key: "uuid", LabelKey: "fleet.products.columns.uuid", Type: rm.ColumnTypeUUID},
		},
		Filters: []rm.Filter{
			{Key: "is_active", LabelKey: "fleet.products.columns.is_active", Variant: rm.FilterVariantFaceted, EnumLookup: "boolean"},
			{Key: "segment", LabelKey: "fleet.products.columns.segment", Variant: rm.FilterVariantFaceted, EnumLookup: "products.facets.segment"},
			{Key: "status", LabelKey: "fleet.products.columns.status", Variant: rm.FilterVariantFaceted, EnumLookup: "fleet.product_status"},
			{Key: "guarantee_type", LabelKey: "fleet.products.columns.guarantee_type", Variant: rm.FilterVariantFaceted, EnumLookup: "fleet.guarantee_type"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewProductsBulk(nil).BulkActions(),
	}
}
