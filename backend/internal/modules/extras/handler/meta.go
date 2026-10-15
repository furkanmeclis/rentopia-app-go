package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/extras/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/extras. Capabilities match
// the routes: export and bulk are mounted by routes.go (no import).
func Meta() resourcemeta.ResourceMeta {
	return resourcemeta.ResourceMeta{
		Resource:      model.Resource,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "code", "name", "extra_type", "charge_type", "default_price", "status", "is_physical"},
		Capabilities: resourcemeta.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: false, Bulk: true,
		},
		SearchableFields: []string{"code", "name", "description"},
		SortableFields:   []string{"code", "name", "extra_type", "status", "default_price", "sort_order", "created_at", "updated_at"},
		FilterableFields: []string{"extra_type", "charge_type", "status", "is_physical", "channel"},
		Columns: []resourcemeta.Column{
			{Key: "code", LabelKey: "extras.columns.code", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "name", LabelKey: "extras.columns.name", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "extra_type", LabelKey: "extras.columns.extra_type", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "charge_type", LabelKey: "extras.columns.charge_type", Type: resourcemeta.ColumnTypeEnum, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "default_price", LabelKey: "extras.columns.default_price", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "product_price_count", LabelKey: "extras.columns.product_price_count", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "max_total", LabelKey: "extras.columns.max_total", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "is_physical", LabelKey: "extras.columns.is_physical", Type: resourcemeta.ColumnTypeBoolean, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "stock_total", LabelKey: "extras.columns.stock_total", Type: resourcemeta.ColumnTypeString, DefaultVisible: false},
			{Key: "status", LabelKey: "extras.columns.status", Type: resourcemeta.ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: resourcemeta.FilterVariantFaceted, DefaultVisible: true},
			{Key: "sort_order", LabelKey: "extras.columns.sort_order", Type: resourcemeta.ColumnTypeString, Sortable: true, DefaultVisible: false},
			{Key: "created_at", LabelKey: "extras.columns.created_at", Type: resourcemeta.ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
			{Key: "uuid", LabelKey: "extras.columns.uuid", Type: resourcemeta.ColumnTypeUUID, DefaultVisible: false},
		},
		Filters: []resourcemeta.Filter{
			{Key: "extra_type", LabelKey: "extras.columns.extra_type", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "extras.facets.extra_type"},
			{Key: "charge_type", LabelKey: "extras.columns.charge_type", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "extras.facets.charge_type"},
			{Key: "status", LabelKey: "extras.columns.status", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "extras.facets.status"},
			{Key: "is_physical", LabelKey: "extras.columns.is_physical", Variant: resourcemeta.FilterVariantFaceted, EnumLookup: "boolean"},
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}
