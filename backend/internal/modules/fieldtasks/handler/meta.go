package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of the board (GET /v1/tenant/field-tasks).
func Meta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "field_tasks.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	faceted := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "field_tasks.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:         model.Resource,
		DefaultSort:      "due_at",
		DefaultFields:    []string{"task_type", "status", "due_at", "place", "vehicle", "reference", "branch", "assignee"},
		Capabilities:     rm.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true, Bulk: true},
		SearchableFields: []string{"reference", "vehicle", "address", "flight_number", "note"},
		SortableFields:   []string{"due_at", "status", "task_type", "created_at"},
		FilterableFields: []string{
			"status", "task_type", "location_kind", "branch", "assignee", "unassigned", "reservation", "due_from", "due_to",
		},
		Columns: []rm.Column{
			faceted(col("task_type", e, true, true)),
			faceted(col("status", e, true, true)),
			col("due_at", dt, true, true),
			faceted(col("location_kind", e, false, false)),
			col("place", s, false, true),
			col("vehicle", s, false, true),
			col("reference", s, false, true),
			faceted(col("branch", e, false, true)),
			faceted(col("assignee", e, false, true)),
			col("contact", s, false, false),
			col("flight_number", s, false, false),
			col("created_at", dt, true, false),
		},
		Filters: []rm.Filter{
			filter("status", "field_tasks.statuses"),
			filter("task_type", "field_tasks.types"),
			filter("location_kind", "field_tasks.location_kinds"),
			filter("branch", "field_tasks.facets.branch"),
			filter("assignee", "field_tasks.facets.assignee"),
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}
