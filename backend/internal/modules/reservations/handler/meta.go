package handler

import (
	availusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

func availabilityCapacity(err error) (*availusecase.CapacityError, bool) {
	return availusecase.IsCapacity(err)
}

func column(prefix, key string, t rm.ColumnType, sortable, visible bool) rm.Column {
	return rm.Column{Key: key, LabelKey: prefix + ".columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
}

func faceted(c rm.Column) rm.Column {
	c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
	return c
}

// ReservationsMeta is the table contract of GET /v1/tenant/reservations.
// Capabilities match the mounted routes (export / bulk via routes.go).
func ReservationsMeta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return column("reservations", key, t, sortable, visible)
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "reservations.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:    model.ResourceReservations,
		DefaultSort: "-created_at",
		DefaultFields: []string{
			"reference", "status", "payment_status", "customer", "product", "branch", "starts_at", "ends_at", "total_amount",
		},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true, Export: true, Bulk: true,
		},
		SearchableFields: []string{"reference", "external_reference", "customer", "plate"},
		SortableFields:   []string{"reference", "status", "starts_at", "ends_at", "total_amount", "created_at"},
		FilterableFields: []string{"status", "payment_status", "channel", "product", "branch", "customer", "starts_from", "starts_to"},
		Columns: []rm.Column{
			col("reference", s, true, true),
			faceted(col("status", e, true, true)),
			faceted(col("payment_status", e, false, true)),
			faceted(col("customer", e, false, true)),
			faceted(col("product", e, false, true)),
			faceted(col("branch", e, false, true)),
			col("starts_at", dt, true, true),
			col("ends_at", dt, true, true),
			faceted(col("channel", e, false, false)),
			col("plate", s, false, false),
			col("total_amount", s, true, true),
			col("assigned_user", s, false, false),
			col("created_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("status", "reservations.statuses"),
			filter("payment_status", "reservations.payment_statuses"),
			filter("channel", "reservations.facets.channel"),
			filter("product", "reservations.facets.product"),
			filter("branch", "reservations.facets.branch"),
			filter("customer", "reservations.facets.customer"),
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}

// QuotesMeta is the table contract of GET /v1/tenant/quotes.
func QuotesMeta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return column("quotes", key, t, sortable, visible)
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:         model.ResourceQuotes,
		DefaultSort:      "-created_at",
		DefaultFields:    []string{"product", "customer", "starts_at", "ends_at", "total", "purpose", "status", "valid_until"},
		Capabilities:     rm.Capabilities{Create: true, Read: true, Search: true, Filter: true, Sort: true},
		SearchableFields: []string{"product", "customer", "reservation"},
		SortableFields:   []string{"total", "starts_at", "valid_until", "created_at"},
		FilterableFields: []string{"purpose", "status", "channel"},
		Columns: []rm.Column{
			col("product", s, false, true), col("customer", s, false, true), col("reservation", s, false, true),
			col("starts_at", dt, true, true), col("ends_at", dt, false, true), col("total", s, true, true),
			col("difference", s, false, false),
			faceted(col("purpose", e, false, true)), faceted(col("status", e, false, true)), faceted(col("channel", e, false, false)),
			col("valid_until", dt, true, true), col("created_at", dt, true, false),
		},
		Filters: []rm.Filter{
			{Key: "purpose", LabelKey: "quotes.columns.purpose", Variant: rm.FilterVariantFaceted, EnumLookup: "quotes.purposes"},
			{Key: "status", LabelKey: "quotes.columns.status", Variant: rm.FilterVariantFaceted, EnumLookup: "quotes.statuses"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}

// TasksMeta is the table contract of GET /v1/tenant/resolution-tasks.
func TasksMeta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return column("resolution_tasks", key, t, sortable, visible)
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:         model.ResourceTasks,
		DefaultSort:      "-created_at",
		DefaultFields:    []string{"task_type", "status", "reservation", "reason", "assignee", "created_at"},
		Capabilities:     rm.Capabilities{Read: true, Update: true, Search: true, Filter: true, Sort: true},
		SearchableFields: []string{"reservation", "reason"},
		SortableFields:   []string{"created_at", "starts_at"},
		FilterableFields: []string{"status", "task_type"},
		Columns: []rm.Column{
			faceted(col("task_type", e, false, true)), faceted(col("status", e, false, true)), col("reservation", s, false, true),
			col("starts_at", dt, true, true), col("reason", s, false, true), col("assignee", s, false, true),
			col("created_at", dt, true, true),
		},
		Filters: []rm.Filter{
			{Key: "status", LabelKey: "resolution_tasks.columns.status", Variant: rm.FilterVariantFaceted, EnumLookup: "resolution_tasks.statuses"},
			{Key: "task_type", LabelKey: "resolution_tasks.columns.task_type", Variant: rm.FilterVariantFaceted, EnumLookup: "resolution_tasks.types"},
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}
