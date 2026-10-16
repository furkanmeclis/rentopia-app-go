package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/agreements/model"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// AgreementsMeta is the table contract of GET /v1/tenant/agreements
// (export via routes.go; no bulk actions).
func AgreementsMeta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "agreements.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	faceted := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "agreements.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e, dt := rm.ColumnTypeString, rm.ColumnTypeEnum, rm.ColumnTypeDatetime
	d := s // calendar dates (Europe/Istanbul) travel as strings
	return rm.ResourceMeta{
		Resource:         model.Resource,
		DefaultSort:      "-created_at",
		DefaultFields:    []string{"code", "name", "customer", "status", "validity", "valid_from", "valid_to", "invoice_period"},
		Capabilities:     rm.Capabilities{Create: true, Read: true, Update: true, Delete: true, Search: true, Filter: true, Sort: true, Export: true},
		SearchableFields: []string{"code", "name", "customer"},
		SortableFields:   []string{"code", "name", "customer", "status", "valid_from", "valid_to", "created_at"},
		FilterableFields: []string{"status", "customer", "branch", "invoice_period", "validity", "valid_from", "valid_to"},
		Columns: []rm.Column{
			col("code", s, true, true),
			col("name", s, true, true),
			faceted(col("customer", e, true, true)),
			faceted(col("status", e, true, true)),
			faceted(col("validity", e, false, true)),
			col("valid_from", d, true, true),
			col("valid_to", d, true, true),
			faceted(col("invoice_period", e, false, true)),
			col("payment_term_days", s, false, false),
			col("open_account", rm.ColumnTypeBoolean, false, false),
			col("credit_limit", s, false, false),
			col("coupon_eligible", rm.ColumnTypeBoolean, false, false),
			faceted(col("branch", e, false, false)),
			col("reservation_count", s, false, true),
			col("tariff_count", s, false, false),
			col("created_at", dt, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("status", "agreements.statuses"),
			filter("customer", "agreements.facets.customer"),
			filter("branch", "agreements.facets.branch"),
			filter("invoice_period", "agreements.invoice_periods"),
			filter("validity", "agreements.validities"),
		},
		Includes:    []string{},
		BulkActions: nil,
	}
}
