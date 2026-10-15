package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/rentals/model"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

func column(prefix, key string, t rm.ColumnType, sortable, visible bool) rm.Column {
	return rm.Column{Key: key, LabelKey: prefix + ".columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
}

func faceted(c rm.Column) rm.Column {
	c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
	return c
}

// RentalsMeta is the table contract of GET /v1/tenant/rentals (export via
// routes.go; no bulk actions).
func RentalsMeta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return column("rentals", key, t, sortable, visible)
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "rentals.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:    model.ResourceRentals,
		DefaultSort: "-created_at",
		DefaultFields: []string{
			"reference", "status", "customer", "plate", "branch", "planned_pickup_at", "planned_return_at", "actual_return_at",
		},
		Capabilities:     rm.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true, Export: true},
		SearchableFields: []string{"reference", "plate", "customer"},
		SortableFields:   []string{"reference", "status", "planned_pickup_at", "planned_return_at", "created_at"},
		FilterableFields: []string{"status", "branch", "product", "vehicle", "customer", "reservation", "overdue"},
		Columns: []rm.Column{
			col("reference", s, true, true),
			faceted(col("status", e, true, true)),
			col("customer", s, false, true),
			col("plate", s, false, true),
			faceted(col("product", e, false, false)),
			faceted(col("branch", e, false, true)),
			col("planned_pickup_at", dt, true, true),
			col("planned_return_at", dt, true, true),
			col("actual_pickup_at", dt, false, false),
			col("actual_return_at", dt, false, true),
			col("late_minutes", s, false, false),
			col("open_charges", s, false, false),
			col("created_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("status", "rentals.statuses"),
			filter("branch", "rentals.facets.branch"),
			filter("product", "rentals.facets.product"),
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// TasksMeta is the table contract of GET /v1/tenant/operation-tasks.
func TasksMeta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return column("rentals.tasks", key, t, sortable, visible)
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "rentals.tasks.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:         model.ResourceOperationTasks,
		DefaultSort:      "due_at",
		DefaultFields:    []string{"task_type", "status", "branch", "rental", "plate", "due_at", "assignee"},
		Capabilities:     rm.Capabilities{Read: true, Update: true, Search: true, Filter: true, Sort: true},
		SearchableFields: []string{"rental", "plate", "note"},
		SortableFields:   []string{"due_at", "status", "created_at"},
		FilterableFields: []string{"status", "task_type", "branch", "rental", "vehicle", "assignee", "due_from", "due_to"},
		Columns: []rm.Column{
			faceted(col("task_type", e, false, true)),
			faceted(col("status", e, true, true)),
			faceted(col("branch", e, false, true)),
			col("rental", s, false, true),
			col("plate", s, false, true),
			col("due_at", dt, true, true),
			col("assignee", s, false, true),
			col("note", s, false, false),
			col("completed_at", dt, false, false),
			col("created_at", dt, true, false),
		},
		Filters: []rm.Filter{
			filter("status", "rentals.tasks.statuses"),
			filter("task_type", "rentals.tasks.types"),
			filter("branch", "rentals.tasks.facets.branch"),
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}
