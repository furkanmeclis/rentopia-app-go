package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
)

func col(prefix, key string, t resourcemeta.ColumnType, sortable, filterable, visible bool) resourcemeta.Column {
	c := resourcemeta.Column{Key: key, LabelKey: prefix + ".columns." + key, Type: t, Sortable: sortable, Filterable: filterable, DefaultVisible: visible}
	if filterable {
		c.FilterVariant = resourcemeta.FilterVariantFaceted
	}
	return c
}

func facet(prefix, key, lookup string) resourcemeta.Filter {
	return resourcemeta.Filter{Key: key, LabelKey: prefix + ".columns." + key, Variant: resourcemeta.FilterVariantFaceted, EnumLookup: lookup}
}

const (
	str  = resourcemeta.ColumnTypeString
	enum = resourcemeta.ColumnTypeEnum
	dt   = resourcemeta.ColumnTypeDatetime
	id   = resourcemeta.ColumnTypeUUID
)

// AccountsMeta is the table contract of GET /v1/tenant/finance/accounts.
func AccountsMeta() resourcemeta.ResourceMeta {
	p := "finance_accounts"
	return resourcemeta.ResourceMeta{
		Resource: model.ResourceAccounts, DefaultSort: "-balance",
		DefaultFields:    []string{"code", "name", "branch", "balance", "overdue", "due_soon", "balance_status"},
		Capabilities:     resourcemeta.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true, Export: true},
		SearchableFields: []string{"code", "name"},
		SortableFields:   []string{"balance", "overdue", "name", "code", "last_entry_at", "created_at"},
		FilterableFields: []string{"balance_status", "status", "party_kind", "branch", "due_before", "min_balance", "max_balance"},
		Columns: []resourcemeta.Column{
			col(p, "code", str, true, false, true), col(p, "name", str, true, false, true),
			col(p, "party_kind", enum, false, true, false), col(p, "branch", enum, false, true, true),
			col(p, "debit", str, false, false, false), col(p, "credit", str, false, false, false),
			col(p, "balance", str, true, false, true), col(p, "overdue", str, true, false, true),
			col(p, "due_soon", str, false, false, true), col(p, "deposit_held", str, false, false, false),
			col(p, "balance_status", enum, false, true, true), col(p, "payment_term_days", str, false, false, false),
			col(p, "status", enum, false, true, false), col(p, "last_entry_at", dt, true, false, false),
			col(p, "created_at", dt, true, false, false), col(p, "uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet(p, "balance_status", "finance.balance_status"), facet(p, "status", "finance.account_status"),
			facet(p, "party_kind", "finance.party"), facet(p, "branch", "branches"),
		},
		Includes: []string{}, BulkActions: []bulkengine.BulkActionDef{},
	}
}

// EntriesMeta is the table contract of GET /v1/tenant/finance/entries.
func EntriesMeta() resourcemeta.ResourceMeta {
	p := "ledger_entries"
	return resourcemeta.ResourceMeta{
		Resource: model.ResourceEntries, DefaultSort: "-entry_date",
		DefaultFields:    []string{"entry_date", "account", "kind", "description", "debit", "credit", "running_balance"},
		Capabilities:     resourcemeta.Capabilities{Read: true, Search: true, Filter: true, Sort: true, Export: true},
		SearchableFields: []string{"description", "account", "reference"},
		SortableFields:   []string{"entry_date"},
		FilterableFields: []string{"account", "rental", "kind", "purpose", "direction", "branch", "from", "to"},
		Columns: []resourcemeta.Column{
			col(p, "entry_date", dt, true, false, true), col(p, "due_date", dt, false, false, false),
			col(p, "account", str, false, false, true), col(p, "kind", enum, false, true, true),
			col(p, "purpose", enum, false, true, false), col(p, "reference", str, false, false, true),
			col(p, "description", str, false, false, true), col(p, "debit", str, false, false, true),
			col(p, "credit", str, false, false, true), col(p, "running_balance", str, false, false, true),
			col(p, "branch", enum, false, true, false), col(p, "reason", str, false, false, false),
			col(p, "uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet(p, "kind", "finance.kind"), facet(p, "purpose", "finance.purpose"), facet(p, "direction", "finance.direction_ledger"),
			facet(p, "branch", "branches"),
		},
		Includes: []string{}, BulkActions: []bulkengine.BulkActionDef{},
	}
}

// CashAccountsMeta is the table contract of GET /v1/tenant/finance/cash-accounts.
func CashAccountsMeta() resourcemeta.ResourceMeta {
	p := "cash_accounts"
	return resourcemeta.ResourceMeta{
		Resource: model.ResourceCashAccounts, DefaultSort: "code",
		DefaultFields: []string{"code", "name", "kind", "branch", "balance", "status"},
		Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true,
			Export: true, Import: true},
		SearchableFields: []string{"code", "name", "bank_name"},
		SortableFields:   []string{"code", "name", "balance", "created_at"},
		FilterableFields: []string{"kind", "status", "branch", "head_office"},
		Columns: []resourcemeta.Column{
			col(p, "code", str, true, false, true), col(p, "name", str, true, false, true), col(p, "kind", enum, false, true, true),
			col(p, "branch", enum, false, true, true), col(p, "currency", str, false, false, false),
			col(p, "opening_balance", str, false, false, false), col(p, "total_in", str, false, false, false),
			col(p, "total_out", str, false, false, false), col(p, "balance", str, true, false, true),
			col(p, "bank_name", str, false, false, false), col(p, "iban", str, false, false, false),
			col(p, "status", enum, false, true, true), col(p, "last_movement_at", dt, false, false, false),
			col(p, "created_at", dt, true, false, false), col(p, "uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet(p, "kind", "finance.cash_kind"), facet(p, "status", "finance.account_status"), facet(p, "branch", "branches"),
		},
		Includes: []string{}, BulkActions: []bulkengine.BulkActionDef{},
	}
}

// MovementsMeta is the table contract of GET /v1/tenant/finance/movements.
func MovementsMeta() resourcemeta.ResourceMeta {
	p := "cash_movements"
	return resourcemeta.ResourceMeta{
		Resource: model.ResourceMovements, DefaultSort: "-movement_date",
		DefaultFields: []string{"movement_date", "account", "kind", "description", "category", "amount", "running_balance"},
		Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Search: true, Filter: true, Sort: true, Export: true,
			Import: true, Bulk: true},
		SearchableFields: []string{"description", "account"},
		SortableFields:   []string{"movement_date", "amount", "created_at"},
		FilterableFields: []string{"account", "kind", "direction", "category", "branch", "from", "to"},
		Columns: []resourcemeta.Column{
			col(p, "movement_date", dt, true, false, true), col(p, "account", enum, false, true, true),
			col(p, "kind", enum, false, true, true), col(p, "direction", enum, false, true, false),
			col(p, "description", str, false, false, true), col(p, "category", enum, false, true, true),
			col(p, "amount", str, true, false, true), col(p, "running_balance", str, false, false, true),
			col(p, "counter_account", str, false, false, false), col(p, "branch", enum, false, true, false),
			col(p, "reason", str, false, false, false), col(p, "created_at", dt, true, false, false),
			col(p, "uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet(p, "kind", "finance.movement_kind"), facet(p, "direction", "finance.direction"),
			facet(p, "category", "finance.expense_categories"), facet(p, "account", "finance.cash_accounts"), facet(p, "branch", "branches"),
		},
		Includes: []string{}, BulkActions: adapters.NewMovementsBulk(nil).BulkActions(),
	}
}

// ExpensesMeta is the table contract of GET /v1/tenant/finance/expenses.
func ExpensesMeta() resourcemeta.ResourceMeta {
	p := "expenses"
	return resourcemeta.ResourceMeta{
		Resource: model.ResourceExpenses, DefaultSort: "-expense_date",
		DefaultFields: []string{"expense_date", "category", "description", "branch", "vehicle", "total_amount", "attachment"},
		Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true,
			Export: true, Import: true, Bulk: true},
		SearchableFields: []string{"description", "supplier_name", "document_no", "plate"},
		SortableFields:   []string{"expense_date", "total_amount", "created_at"},
		FilterableFields: []string{"category", "branch", "vehicle", "status", "from", "to"},
		Columns: []resourcemeta.Column{
			col(p, "expense_date", dt, true, false, true), col(p, "category", enum, false, true, true),
			col(p, "description", str, false, false, true), col(p, "supplier_name", str, false, false, false),
			col(p, "document_no", str, false, false, false), col(p, "branch", enum, false, true, true),
			col(p, "vehicle", str, false, false, true), col(p, "cash_account", str, false, false, false),
			col(p, "net_amount", str, false, false, false), col(p, "vat_amount", str, false, false, false),
			col(p, "total_amount", str, true, false, true), col(p, "attachment", str, false, false, true),
			col(p, "status", enum, false, true, false), col(p, "created_at", dt, true, false, false),
			col(p, "uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet(p, "category", "finance.expense_categories"), facet(p, "status", "finance.expense_status"), facet(p, "branch", "branches"),
		},
		Includes: []string{}, BulkActions: adapters.NewExpensesBulk(nil).BulkActions(),
	}
}
