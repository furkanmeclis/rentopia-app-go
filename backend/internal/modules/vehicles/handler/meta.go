package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/vehicles. Capabilities match
// the live routes: export / import / bulk are mounted by routes.go.
func Meta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "vehicles.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	facet := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "vehicles.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e := rm.ColumnTypeString, rm.ColumnTypeEnum
	return rm.ResourceMeta{
		Resource:    model.Resource,
		DefaultSort: "-created_at",
		DefaultFields: []string{
			"uuid", "plate", "vehicle", "model_year", "odometer_km", "status", "current_branch", "kasko_value", "is_active",
		},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		SearchableFields: []string{"plate", "vin", "brand", "model", "type_name", "color"},
		SortableFields: []string{
			"plate", "model_year", "odometer_km", "status", "kasko_value", "is_active", "created_at", "updated_at",
			"inspection_until", "insurance_until", "kasko_until", "document_expires_at",
			"next_maintenance_date", "next_maintenance_km",
		},
		FilterableFields: []string{
			"status", "rental_group", "segment", "home_branch", "current_branch", "ownership", "pool", "is_active",
			"document_status",
		},
		Columns: []rm.Column{
			col("plate", s, true, true),
			col("vehicle", s, false, true),
			col("model_year", s, true, true),
			col("odometer_km", s, true, true),
			facet(col("status", e, true, true)),
			facet(col("current_branch", e, false, true)),
			facet(col("home_branch", e, false, false)),
			col("kasko_value", s, true, true),
			facet(col("document_status", e, true, true)),
			col("inspection_until", s, true, false),
			col("insurance_until", s, true, false),
			col("kasko_until", s, true, false),
			col("next_maintenance_date", s, true, false),
			col("next_maintenance_km", s, true, false),
			facet(col("rental_group", e, false, false)),
			facet(col("segment", e, false, false)),
			facet(col("ownership", e, false, false)),
			facet(col("pool", e, false, false)),
			col("vin", s, false, false),
			col("color", s, false, false),
			facet(col("is_active", rm.ColumnTypeBoolean, true, true)),
			col("created_at", rm.ColumnTypeDatetime, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("status", "vehicles.status"),
			filter("rental_group", "vehicles.facets.rental_group"),
			filter("segment", "vehicles.facets.segment"),
			filter("home_branch", "vehicles.facets.home_branch"),
			filter("current_branch", "vehicles.facets.current_branch"),
			filter("ownership", "vehicles.ownership"),
			filter("pool", "vehicles.facets.pool"),
			filter("is_active", "boolean"),
			filter("document_status", "vehicles.document_status"),
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}
