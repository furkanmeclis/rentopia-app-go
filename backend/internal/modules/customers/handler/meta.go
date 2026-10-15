package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/model"
	rm "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

// Meta is the table contract of GET /v1/tenant/customers. Identity, phone
// and e-mail columns are the masked values. Capabilities match the live
// routes (export / import / bulk are mounted by routes.go).
func Meta() rm.ResourceMeta {
	col := func(key string, t rm.ColumnType, sortable, visible bool) rm.Column {
		return rm.Column{Key: key, LabelKey: "customers.columns." + key, Type: t, Sortable: sortable, DefaultVisible: visible}
	}
	facet := func(c rm.Column) rm.Column {
		c.Filterable, c.FilterVariant = true, rm.FilterVariantFaceted
		return c
	}
	filter := func(key, lookup string) rm.Filter {
		return rm.Filter{Key: key, LabelKey: "customers.columns." + key, Variant: rm.FilterVariantFaceted, EnumLookup: lookup}
	}
	s, e := rm.ColumnTypeString, rm.ColumnTypeEnum
	return rm.ResourceMeta{
		Resource:    model.Resource,
		DefaultSort: "-created_at",
		DefaultFields: []string{
			"uuid", "display_name", "customer_type", "identity_masked", "phone_masked", "nationality", "tag", "consent",
		},
		Capabilities: rm.Capabilities{
			Create: true, Read: true, Update: true, Delete: true,
			Search: true, Filter: true, Sort: true, Export: true, Import: true, Bulk: true,
		},
		// Free text matches names, company title and e-mail; digits match the
		// phone; a full TCKN / VKN / passport matches exactly (blind index).
		SearchableFields: []string{"first_name", "last_name", "company_title", "email", "phone", "identity_number"},
		SortableFields:   []string{"display_name", "customer_type", "nationality", "created_at", "updated_at"},
		FilterableFields: []string{"customer_type", "nationality", "tag", "consent", "anonymized"},
		Columns: []rm.Column{
			col("display_name", s, true, true),
			facet(col("customer_type", e, true, true)),
			col("identity_masked", s, false, true),
			col("phone_masked", s, false, true),
			col("email_masked", s, false, false),
			facet(col("nationality", e, true, true)),
			facet(col("tag", e, false, true)),
			facet(col("consent", e, false, true)),
			col("drivers_count", s, false, false),
			col("billing_city", s, false, false),
			facet(col("anonymized", rm.ColumnTypeBoolean, false, false)),
			col("created_at", rm.ColumnTypeDatetime, true, false),
			col("updated_at", rm.ColumnTypeDatetime, true, false),
			col("uuid", rm.ColumnTypeUUID, false, false),
		},
		Filters: []rm.Filter{
			filter("customer_type", "customers.types"),
			filter("nationality", "customers.facets.nationality"),
			filter("tag", "customers.facets.tag"),
			filter("consent", "customers.consent_channels"),
			filter("anonymized", "boolean"),
		},
		Includes:    []string{},
		BulkActions: adapters.NewBulk(nil).BulkActions(),
	}
}
