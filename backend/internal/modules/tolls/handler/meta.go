package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/toll-passes (export and
// file import through routes.go; no bulk actions: every decision on a pass
// needs its own reason).
func Meta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "tolls.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	faceted := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "tolls.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:    model.Resource,
		DefaultSort: "-passed_at",
		DefaultFields: []string{
			"passed_at", "plate", "station", "amount", "status", "rental",
		},
		Capabilities:     rm.Capabilities{Read: true, Search: true, Filter: true, Sort: true, Export: true, Import: true},
		SearchableFields: []string{"plate", "tag_number", "station", "external_id", "rental"},
		SortableFields:   []string{"passed_at", "amount", "status", "plate", "created_at"},
		FilterableFields: model.QueryKeys,
		Columns: []rm.Column{
			col("passed_at", dt, true, true),
			col("plate", s, true, true),
			col("tag_number", s, false, false),
			col("station", s, false, true),
			col("amount", s, true, true),
			faceted(col("status", e, true, true)),
			faceted(col("review_reason", e, false, false)),
			col("rental", s, false, true),
			faceted(col("source", e, false, false)),
			faceted(col("provider", e, false, false)),
			col("external_id", s, false, false),
			col("created_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("status", "tolls.statuses"),
			filter("review_reason", "tolls.review_reasons"),
			filter("source", "tolls.sources"),
			filter("provider", "tolls.facets.provider"),
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}
