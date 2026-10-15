package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

func metaHelpers(base string) (
	col func(key string, t rm.ColumnType, sortable, visible bool) rm.Column,
	facet func(c rm.Column) rm.Column,
	filter func(key, lookup string) rm.Filter,
) {
	col = func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: base + ".columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	facet = func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	filter = func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: base + ".columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	return col, facet, filter
}

// RatePlansMeta is the table contract of GET /v1/tenant/rate-plans.
// Capabilities match the routes: export and import are mounted, bulk is not
// (activation is a deliberate per-plan or per-matrix action).
func RatePlansMeta() rm.ResourceMeta {
	col, facet, filter := metaHelpers("rate_plans")
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:      model.ResourceRatePlans,
		DefaultSort:   "rental_product",
		DefaultFields: []string{"uuid", "rental_product", "scope", "branch", "channel", "priority", "price_from", "status"},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: false,
		},
		SearchableFields: []string{"name", "rental_product", "branch"},
		SortableFields:   []string{"name", "rental_product", "scope", "status", "priority", "created_at", "updated_at"},
		FilterableFields: []string{"rental_product", "scope", "branch", "channel", "status", "has_draft"},
		Columns: []rm.Column{
			facet(col("rental_product", e, true, true)),
			col("name", s, true, false),
			facet(col("scope", e, true, true)),
			facet(col("branch", e, false, true)),
			facet(col("channel", e, false, true)),
			col("priority", s, true, true),
			col("bands", s, false, true),
			col("price_from", s, false, true),
			facet(col("status", e, true, true)),
			col("version", s, false, false),
			col("updated_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("rental_product", "rate_plans.facets.rental_product"),
			filter("scope", "rate_plans.scope"),
			filter("branch", "rate_plans.facets.branch"),
			filter("channel", "sales_channels"),
			filter("status", "pricing.status"),
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// DateRulesMeta is the table contract of GET /v1/tenant/date-rules.
func DateRulesMeta() rm.ResourceMeta {
	col, facet, filter := metaHelpers("date_rules")
	s, e, d, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeString, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:      model.ResourceDateRules,
		DefaultSort:   "start_date",
		DefaultFields: []string{"uuid", "name", "start_date", "end_date", "kind", "value", "priority", "status"},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: false, Bulk: false,
		},
		SearchableFields: []string{"name"},
		SortableFields:   []string{"name", "start_date", "priority", "status", "created_at", "updated_at"},
		FilterableFields: []string{"status", "kind", "has_draft", "date_from", "date_to"},
		Columns: []rm.Column{
			col("name", s, true, true),
			col("start_date", d, true, true),
			col("end_date", d, false, true),
			facet(col("kind", e, false, true)),
			col("value", s, false, true),
			col("scope", s, false, true),
			col("priority", s, true, true),
			facet(col("status", e, true, true)),
			col("version", s, false, false),
			col("updated_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("status", "pricing.status"),
			filter("kind", "date_rules.kind"),
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}
