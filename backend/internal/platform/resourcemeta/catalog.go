package resourcemeta

import "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"

// PlatformCatalogBrands returns meta for GET /v1/platform/catalog/brands.
func PlatformCatalogBrands() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.catalog.brands",
		DefaultSort:   "display_name",
		DefaultFields: []string{"display_name", "tsb_name", "tsb_code", "category", "is_active", "types_count"},
		Capabilities: Capabilities{
			Read: true, Update: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"display_name", "tsb_name", "tsb_code"},
		SortableFields:   []string{"display_name", "tsb_name", "tsb_code", "category", "types_count", "created_at"},
		FilterableFields: []string{"category", "is_active"},
		Columns: []Column{
			{Key: "display_name", LabelKey: "catalog.columns.display_name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "tsb_name", LabelKey: "catalog.columns.tsb_name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "tsb_code", LabelKey: "catalog.columns.tsb_code", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "category", LabelKey: "catalog.columns.category", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "is_active", LabelKey: "catalog.columns.is_active", Type: ColumnTypeBoolean, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "types_count", LabelKey: "catalog.columns.types_count", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "catalog.columns.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []Filter{
			{Key: "category", LabelKey: "catalog.columns.category", Variant: FilterVariantFaceted, EnumLookup: "catalog_brand_category"},
			{Key: "is_active", LabelKey: "catalog.columns.is_active", Variant: FilterVariantFaceted},
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// PlatformCatalogDisplayBrands returns meta for GET /v1/platform/catalog/display-brands.
func PlatformCatalogDisplayBrands() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.catalog.display_brands",
		DefaultSort:   "name",
		DefaultFields: []string{"name", "tsb_names", "brands_count", "types_count", "models_count", "is_active"},
		Capabilities: Capabilities{
			Read: true, Update: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"name", "tsb_name", "tsb_code"},
		SortableFields:   []string{"name", "brands_count", "types_count", "created_at"},
		FilterableFields: []string{"is_active", "merged"},
		Columns: []Column{
			{Key: "name", LabelKey: "catalog.columns.display_name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "tsb_names", LabelKey: "catalog.columns.tsb_brands", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "brands_count", LabelKey: "catalog.columns.brands_count", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "types_count", LabelKey: "catalog.columns.types_count", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "models_count", LabelKey: "catalog.columns.models_count", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "is_active", LabelKey: "catalog.columns.is_active", Type: ColumnTypeBoolean, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "catalog.columns.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []Filter{
			{Key: "is_active", LabelKey: "catalog.columns.is_active", Variant: FilterVariantFaceted},
			{Key: "merged", LabelKey: "catalog.display_brands.merged_only", Variant: FilterVariantFaceted},
		},
		Includes:    []string{"brands"},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// PlatformCatalogModels returns meta for GET /v1/platform/catalog/models.
func PlatformCatalogModels() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.catalog.models",
		DefaultSort:   "brand",
		DefaultFields: []string{"name", "brand", "source", "types_count"},
		Capabilities: Capabilities{
			Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"name"},
		SortableFields:   []string{"name", "brand", "source", "types_count", "created_at"},
		FilterableFields: []string{"brand", "source"},
		Columns: []Column{
			{Key: "name", LabelKey: "catalog.columns.model", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "brand", LabelKey: "catalog.columns.brand", Type: ColumnTypeString, Sortable: true, Filterable: true, DefaultVisible: true},
			{Key: "source", LabelKey: "catalog.columns.source", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "types_count", LabelKey: "catalog.columns.types_count", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "created_at", LabelKey: "catalog.columns.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: false},
		},
		Filters: []Filter{
			{Key: "brand", LabelKey: "catalog.columns.brand", Variant: FilterVariantFaceted},
			{Key: "source", LabelKey: "catalog.columns.source", Variant: FilterVariantFaceted, EnumLookup: "catalog_model_source"},
		},
		Includes:    []string{"rules"},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// PlatformCatalogTypes returns meta for GET /v1/platform/catalog/types.
func PlatformCatalogTypes() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.catalog.types",
		DefaultSort:   "brand",
		DefaultFields: []string{"raw_name", "brand", "model", "years", "suggested_fuel", "suggested_transmission"},
		Capabilities: Capabilities{
			Read: true, Update: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"raw_name", "tsb_type_code"},
		SortableFields:   []string{"raw_name", "tsb_type_code", "brand", "model", "last_seen_period", "created_at"},
		FilterableFields: []string{"brand", "model", "category", "year", "unresolved", "is_active", "fuel", "transmission"},
		Columns: []Column{
			{Key: "raw_name", LabelKey: "catalog.columns.raw_name", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "tsb_type_code", LabelKey: "catalog.columns.tsb_type_code", Type: ColumnTypeString, Sortable: true, DefaultVisible: false},
			{Key: "brand", LabelKey: "catalog.columns.brand", Type: ColumnTypeString, Sortable: true, Filterable: true, DefaultVisible: true},
			{Key: "model", LabelKey: "catalog.columns.model", Type: ColumnTypeString, Sortable: true, Filterable: true, DefaultVisible: true},
			{Key: "years", LabelKey: "catalog.columns.years", Type: ColumnTypeString, Filterable: true, DefaultVisible: true},
			{Key: "suggested_fuel", LabelKey: "catalog.columns.fuel", Type: ColumnTypeEnum, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "suggested_transmission", LabelKey: "catalog.columns.transmission", Type: ColumnTypeEnum, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "is_active", LabelKey: "catalog.columns.is_active", Type: ColumnTypeBoolean, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: false},
			{Key: "last_seen_period", LabelKey: "catalog.columns.last_seen_period", Type: ColumnTypeString, Sortable: true, DefaultVisible: false},
		},
		Filters: []Filter{
			{Key: "brand", LabelKey: "catalog.columns.brand", Variant: FilterVariantFaceted},
			{Key: "model", LabelKey: "catalog.columns.model", Variant: FilterVariantFaceted},
			{Key: "category", LabelKey: "catalog.columns.category", Variant: FilterVariantFaceted, EnumLookup: "catalog_brand_category"},
			{Key: "year", LabelKey: "catalog.columns.year", Variant: FilterVariantText},
			{Key: "unresolved", LabelKey: "catalog.filters.unresolved", Variant: FilterVariantFaceted},
			{Key: "is_active", LabelKey: "catalog.columns.is_active", Variant: FilterVariantFaceted},
			{Key: "fuel", LabelKey: "catalog.columns.fuel", Variant: FilterVariantFaceted, EnumLookup: "catalog_fuel"},
			{Key: "transmission", LabelKey: "catalog.columns.transmission", Variant: FilterVariantFaceted, EnumLookup: "catalog_transmission"},
		},
		Includes:    []string{"valuations"},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// PlatformCatalogImports returns meta for GET /v1/platform/catalog/imports.
func PlatformCatalogImports() ResourceMeta {
	return ResourceMeta{
		Resource:      "platform.catalog.imports",
		DefaultSort:   "-created_at",
		DefaultFields: []string{"period", "filename", "status", "created_at"},
		Capabilities: Capabilities{
			Create: true, Read: true, Search: true, Filter: true, Sort: true,
		},
		SearchableFields: []string{"filename", "period"},
		SortableFields:   []string{"period", "status", "created_at"},
		FilterableFields: []string{"status"},
		Columns: []Column{
			{Key: "period", LabelKey: "catalog.columns.period", Type: ColumnTypeString, Sortable: true, DefaultVisible: true},
			{Key: "filename", LabelKey: "catalog.columns.filename", Type: ColumnTypeString, DefaultVisible: true},
			{Key: "status", LabelKey: "catalog.columns.status", Type: ColumnTypeEnum, Sortable: true, Filterable: true, FilterVariant: FilterVariantFaceted, DefaultVisible: true},
			{Key: "created_at", LabelKey: "catalog.columns.created_at", Type: ColumnTypeDatetime, Sortable: true, DefaultVisible: true},
		},
		Filters: []Filter{
			{Key: "status", LabelKey: "catalog.columns.status", Variant: FilterVariantFaceted, EnumLookup: "catalog_import_status"},
		},
		Includes:    []string{"diff"},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}
