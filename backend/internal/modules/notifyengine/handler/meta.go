package handler

import (
	engine "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/usecase"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// ResourceDeliveries is the table resource of the delivery history.
const ResourceDeliveries = "tenant.notification_deliveries"

// DeliveriesMeta is the table contract of GET /v1/tenant/notification-deliveries
// (read only: no export, no bulk actions).
func DeliveriesMeta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "notification_engine.history.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	faceted := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "notification_engine.history.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	return rm.ResourceMeta{
		Resource:         ResourceDeliveries,
		DefaultSort:      "-created_at",
		DefaultFields:    []string{"created_at", "event", "recipient", "channel", "status", "error"},
		Capabilities:     rm.Capabilities{Read: true, Search: true, Filter: true, Sort: true},
		SearchableFields: []string{"recipient", "subject", "provider_ref"},
		SortableFields:   engine.Sortable,
		FilterableFields: []string{"channel", "status", "event", "audience", "created_at"},
		Columns: []rm.Column{
			col("created_at", dt, true, true),
			faceted(col("event", e, true, true)),
			col("recipient", s, false, true),
			faceted(col("channel", e, true, true)),
			col("attempt_no", s, false, false),
			faceted(col("status", e, true, true)),
			col("provider", s, false, false),
			col("provider_ref", s, false, false),
			col("subject", s, false, false),
			col("error", s, false, true),
			faceted(col("audience", e, false, false)),
		},
		Filters: []rm.Filter{
			filter("event", "notification_engine.events"),
			filter("channel", "notification_engine.channels"),
			filter("status", "notification_engine.delivery_statuses"),
			filter("audience", "notification_engine.audiences"),
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}
