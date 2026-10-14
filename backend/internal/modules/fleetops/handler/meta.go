package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fleetops/model"
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

// TransfersMeta is the table contract of GET /v1/tenant/transfers.
// Capabilities match the routes: export is mounted, import / bulk are not.
func TransfersMeta() rm.ResourceMeta {
	col, facet, filter := metaHelpers("transfers")
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:    model.ResourceTransfers,
		DefaultSort: "-planned_departure",
		DefaultFields: []string{
			"uuid", "plate", "route", "responsible", "planned_departure", "planned_arrival", "status", "cost",
		},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: false,
			Search: true, Filter: true, Sort: true, Export: true, Import: false, Bulk: false,
		},
		SearchableFields: []string{"plate", "note", "from_branch", "to_branch"},
		SortableFields: []string{
			"plate", "planned_departure", "planned_arrival", "status", "cost", "created_at", "updated_at",
		},
		FilterableFields: []string{"status", "from_branch", "to_branch", "vehicle"},
		Columns: []rm.Column{
			col("plate", s, true, true),
			col("route", s, false, true),
			facet(col("from_branch", e, false, false)),
			facet(col("to_branch", e, false, false)),
			col("responsible", s, false, true),
			col("planned_departure", dt, true, true),
			col("planned_arrival", dt, true, true),
			col("actual_departure", dt, false, false),
			col("actual_arrival", dt, false, false),
			facet(col("status", e, true, true)),
			col("cost", s, true, true),
			col("note", s, false, false),
			col("created_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("status", "transfers.status"),
			filter("from_branch", "transfers.facets.from_branch"),
			filter("to_branch", "transfers.facets.to_branch"),
		},
		Includes:    []string{},
		BulkActions: []bulkengine.BulkActionDef{},
	}
}

// OneWayMeta is the table contract of GET /v1/tenant/one-way-rules.
func OneWayMeta() rm.ResourceMeta {
	col, facet, filter := metaHelpers("one_way_rules")
	s, e, b := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeBoolean
	return rm.ResourceMeta{
		Resource:      model.ResourceOneWayRules,
		DefaultSort:   "-created_at",
		DefaultFields: []string{"uuid", "from_branch", "to_branch", "rental_product", "allowed", "fee", "is_active"},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: false, Bulk: true,
		},
		SearchableFields: []string{"from_branch", "to_branch", "rental_product", "note"},
		SortableFields:   []string{"from_branch", "to_branch", "fee", "is_active", "created_at", "updated_at"},
		FilterableFields: []string{"from_branch", "to_branch", "allowed", "is_active"},
		Columns: []rm.Column{
			facet(col("from_branch", e, true, true)),
			facet(col("to_branch", e, true, true)),
			col("rental_product", s, false, true),
			facet(col("allowed", b, false, true)),
			col("fee", s, true, true),
			col("note", s, false, false),
			facet(col("is_active", b, true, true)),
			col("created_at", rm.ColumnTypeDatetime, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("from_branch", "one_way_rules.facets.from_branch"),
			filter("to_branch", "one_way_rules.facets.to_branch"),
			filter("allowed", "boolean"),
			filter("is_active", "boolean"),
		},
		Includes:    []string{},
		BulkActions: adapters.NewOneWayBulk(nil).BulkActions(),
	}
}
