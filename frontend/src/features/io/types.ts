export type ExportFormat = "pdf" | "xlsx" | "csv" | "json";
export type ImportFormat = "json" | "xlsx" | "csv" | "tsv";

import type { BulkActionMeta } from "@/features/bulk-engine/types";

export type ResourceCapabilities = {
  create?: boolean;
  read?: boolean;
  update?: boolean;
  delete?: boolean;
  search?: boolean;
  filter?: boolean;
  sort?: boolean;
  export?: boolean;
  import?: boolean;
  bulk?: boolean;
};

export type ResourceMeta = {
  resource?: string;
  capabilities?: ResourceCapabilities;
  sortable?: string[];
  filterable?: string[];
  bulk_actions?: BulkActionMeta[];
};

export type ExportJob = {
  uuid: string;
  resource: string;
  format: string;
  status: string;
  row_count: number;
  error?: string | null;
  download_url?: string | null;
  created_at: string;
};

export type ImportJob = {
  uuid: string;
  resource: string;
  format: string;
  status: string;
  mapping?: Record<string, string>;
  defaults?: Record<string, string>;
  preview_summary?: ImportPreviewSummary;
  error?: string | null;
  rollback_until?: string | null;
  applied_at?: string | null;
  created_at: string;
};

/** One field an import row would change (dry-run diff adapters). */
export type ImportFieldChange = { field: string; from: string; to: string };

export type ImportPreviewRow = {
  index: number;
  data: Record<string, unknown>;
  /** Dry-run diff adapters only: what the row would do. */
  op?: "create" | "update";
  changes?: ImportFieldChange[];
};

export type ImportPreviewSummary = {
  total: number;
  valid: number;
  invalid: number;
  /** Dry-run diff adapters only. */
  creates?: number;
  updates?: number;
  rows?: ImportPreviewRow[] | null;
  errors?: { index: number; field?: string; error: string }[] | null;
};

export type AppSettings = {
  company_name: string;
  tagline: string;
  primary_color: string;
  address: string;
  phone: string;
  email: string;
  website: string;
  footer_text: string;
  paper_size: string;
  logo_url?: string | null;
};

export type ActivityEvent = {
  uuid: string;
  actor_user_id?: number | null;
  action: string;
  resource: string;
  resource_uuid?: string | null;
  payload: Record<string, unknown>;
  created_at: string;
};

/**
 * Export / import resources. `tenant.*` resources are organization-scoped and
 * go through `/v1/tenant/*` with the tenant `ApiScope` (see `useApiScope`).
 */
export type IoResource =
  | "platform.users"
  | "platform.roles"
  | "platform.notifications"
  | "platform.activity"
  | "platform.organizations"
  | "tenant.branches"
  | "tenant.segments"
  | "tenant.rental_groups"
  | "tenant.rental_products"
  | "tenant.vehicles"
  | "tenant.vehicle_documents"
  | "tenant.vehicle_maintenance"
  | "tenant.transfers"
  | "tenant.one_way_rules"
  | "tenant.rate_plans"
  | "tenant.date_rules"
  | "tenant.customers"
  | "tenant.extras"
  | "tenant.campaigns"
  | "tenant.reservations"
  | "tenant.rentals"
  | "tenant.finance_accounts"
  | "tenant.ledger_entries"
  | "tenant.cash_accounts"
  | "tenant.cash_movements"
  | "tenant.expenses"
  | "tenant.security_deposits"
  | "tenant.toll_passes"
  | "tenant.finance_collections";

export const EXPORT_PATHS: Record<IoResource, string> = {
  "platform.users": "/v1/platform/users/export",
  "platform.roles": "/v1/platform/roles/export",
  "platform.notifications": "/v1/platform/notifications/export",
  "platform.activity": "/v1/platform/activity/export",
  "platform.organizations": "/v1/platform/organizations/export",
  "tenant.branches": "/v1/tenant/branches/export",
  "tenant.segments": "/v1/tenant/segments/export",
  "tenant.rental_groups": "/v1/tenant/rental-groups/export",
  "tenant.rental_products": "/v1/tenant/products/export",
  "tenant.vehicles": "/v1/tenant/vehicles/export",
  "tenant.vehicle_documents": "/v1/tenant/vehicle-documents/export",
  "tenant.vehicle_maintenance": "/v1/tenant/vehicle-maintenance/export",
  "tenant.transfers": "/v1/tenant/transfers/export",
  "tenant.one_way_rules": "/v1/tenant/one-way-rules/export",
  "tenant.rate_plans": "/v1/tenant/rate-plans/export",
  "tenant.date_rules": "/v1/tenant/date-rules/export",
  "tenant.customers": "/v1/tenant/customers/export",
  "tenant.extras": "/v1/tenant/extras/export",
  "tenant.campaigns": "/v1/tenant/campaigns/export",
  "tenant.reservations": "/v1/tenant/reservations/export",
  "tenant.rentals": "/v1/tenant/rentals/export",
  "tenant.finance_accounts": "/v1/tenant/finance/accounts/export",
  "tenant.ledger_entries": "/v1/tenant/finance/entries/export",
  "tenant.cash_accounts": "/v1/tenant/finance/cash-accounts/export",
  "tenant.cash_movements": "/v1/tenant/finance/movements/export",
  "tenant.expenses": "/v1/tenant/finance/expenses/export",
  "tenant.security_deposits": "/v1/tenant/finance/deposits/export",
  "tenant.toll_passes": "/v1/tenant/toll-passes/export",
  "tenant.finance_collections": "/v1/tenant/finance/collections/export",
};

export const IMPORT_PATHS: Partial<
  Record<IoResource, { upload: string; sample: string }>
> = {
  "platform.users": {
    upload: "/v1/platform/users/import",
    sample: "/v1/platform/users/import/sample",
  },
  "platform.roles": {
    upload: "/v1/platform/roles/import",
    sample: "/v1/platform/roles/import/sample",
  },
  "tenant.branches": {
    upload: "/v1/tenant/branches/import",
    sample: "/v1/tenant/branches/import/sample",
  },
  "tenant.vehicles": {
    upload: "/v1/tenant/vehicles/import",
    sample: "/v1/tenant/vehicles/import/sample",
  },
  "tenant.rate_plans": {
    upload: "/v1/tenant/rate-plans/import",
    sample: "/v1/tenant/rate-plans/import/sample",
  },
  "tenant.customers": {
    upload: "/v1/tenant/customers/import",
    sample: "/v1/tenant/customers/import/sample",
  },
  "tenant.cash_accounts": {
    upload: "/v1/tenant/finance/cash-accounts/import",
    sample: "/v1/tenant/finance/cash-accounts/import/sample",
  },
  "tenant.cash_movements": {
    upload: "/v1/tenant/finance/movements/import",
    sample: "/v1/tenant/finance/movements/import/sample",
  },
  "tenant.expenses": {
    upload: "/v1/tenant/finance/expenses/import",
    sample: "/v1/tenant/finance/expenses/import/sample",
  },
  "tenant.toll_passes": {
    upload: "/v1/tenant/toll-passes/import",
    sample: "/v1/tenant/toll-passes/import/sample",
  },
};

export const IMPORT_SCHEMA: Partial<
  Record<IoResource, { key: string; labelKey: string; required?: boolean }[]>
> = {
  "platform.users": [
    { key: "email", labelKey: "users.fields.email", required: true },
    { key: "name", labelKey: "users.fields.name", required: true },
    { key: "surname", labelKey: "users.fields.surname", required: true },
    { key: "status", labelKey: "users.fields.status" },
    { key: "locale", labelKey: "users.fields.locale" },
    { key: "role_slugs", labelKey: "users.fields.roles" },
  ],
  "platform.roles": [
    { key: "name", labelKey: "roles.fields.name", required: true },
    { key: "slug", labelKey: "roles.fields.slug", required: true },
    { key: "description", labelKey: "roles.fields.description" },
    { key: "permission_slugs", labelKey: "roles.fields.permissions" },
  ],
  "tenant.branches": [
    { key: "code", labelKey: "branches.fields.code", required: true },
    { key: "name", labelKey: "branches.fields.name", required: true },
    { key: "city", labelKey: "branches.fields.city" },
    { key: "district", labelKey: "branches.fields.district" },
    { key: "address", labelKey: "branches.fields.address" },
    { key: "phone", labelKey: "branches.fields.phone" },
    { key: "email", labelKey: "branches.fields.email" },
    { key: "latitude", labelKey: "branches.fields.latitude" },
    { key: "longitude", labelKey: "branches.fields.longitude" },
    { key: "prep_minutes", labelKey: "branches.fields.prep_minutes" },
    { key: "is_active", labelKey: "branches.fields.is_active" },
  ],
  // İdes fleet XLSX: plaka, şasi, marka, model, tip / kasko kodu, model yılı,
  // renk, vites, yakıt, şube, mülkiyet, km. type_code before type_name so a
  // "Tip/Kasko Kodu" header maps to the code.
  "tenant.toll_passes": [
    {
      key: "external_id",
      labelKey: "tolls.import.external_id",
      required: true,
    },
    { key: "passed_at", labelKey: "tolls.import.passed_at", required: true },
    { key: "plate", labelKey: "tolls.import.plate" },
    { key: "tag_number", labelKey: "tolls.import.tag_number" },
    { key: "station", labelKey: "tolls.import.station", required: true },
    { key: "exit_station", labelKey: "tolls.import.exit_station" },
    { key: "amount", labelKey: "tolls.import.amount", required: true },
    { key: "currency", labelKey: "tolls.import.currency" },
    { key: "provider", labelKey: "tolls.import.provider" },
  ],
  "tenant.vehicles": [
    { key: "plate", labelKey: "vehicles.fields.plate", required: true },
    { key: "vin", labelKey: "vehicles.fields.vin" },
    { key: "brand", labelKey: "vehicles.fields.brand" },
    { key: "model", labelKey: "vehicles.fields.model" },
    { key: "type_code", labelKey: "vehicles.fields.type_code" },
    { key: "type_name", labelKey: "vehicles.fields.type" },
    {
      key: "model_year",
      labelKey: "vehicles.fields.model_year",
      required: true,
    },
    { key: "color", labelKey: "vehicles.fields.color" },
    { key: "transmission", labelKey: "vehicles.fields.transmission" },
    { key: "fuel", labelKey: "vehicles.fields.fuel" },
    { key: "branch", labelKey: "vehicles.fields.branch", required: true },
    { key: "ownership", labelKey: "vehicles.fields.ownership" },
    { key: "supplier_name", labelKey: "vehicles.fields.supplier_name" },
    { key: "fleet_pool", labelKey: "vehicles.fields.fleet_pool" },
    { key: "odometer_km", labelKey: "vehicles.fields.odometer_km" },
  ],
  // İdes price list: ürün kodu, bant (1-3, 15+), günlük fiyat; optional
  // branch code / channel / priority target an override plan.
  "tenant.rate_plans": [
    {
      key: "product_code",
      labelKey: "pricing.import.product_code",
      required: true,
    },
    { key: "band", labelKey: "pricing.import.band", required: true },
    {
      key: "daily_price",
      labelKey: "pricing.import.daily_price",
      required: true,
    },
    { key: "branch_code", labelKey: "pricing.import.branch_code" },
    { key: "channel", labelKey: "pricing.import.channel" },
    { key: "priority", labelKey: "pricing.import.priority" },
  ],
  // İdes customer list: müşteri tipi, ad, soyad, unvan, TC kimlik, vergi no,
  // pasaport, uyruk, telefon, e-posta, vergi dairesi, adres, il, ilçe,
  // etiketler, not. Rows match existing customers by identity number.
  "tenant.customers": [
    { key: "customer_type", labelKey: "customers.fields.customer_type" },
    { key: "first_name", labelKey: "customers.fields.first_name" },
    { key: "last_name", labelKey: "customers.fields.last_name" },
    { key: "company_title", labelKey: "customers.fields.company_title" },
    { key: "tckn", labelKey: "customers.import.tckn" },
    { key: "vkn", labelKey: "customers.import.vkn" },
    { key: "passport_no", labelKey: "customers.import.passport_no" },
    { key: "nationality", labelKey: "customers.fields.nationality" },
    { key: "phone", labelKey: "customers.fields.phone" },
    { key: "email", labelKey: "customers.fields.email" },
    { key: "tax_office", labelKey: "customers.fields.tax_office" },
    { key: "billing_address", labelKey: "customers.fields.billing_address" },
    { key: "billing_city", labelKey: "customers.fields.billing_city" },
    { key: "billing_district", labelKey: "customers.fields.billing_district" },
    { key: "tags", labelKey: "customers.fields.tags" },
    { key: "note", labelKey: "customers.fields.note" },
  ],
  "tenant.cash_accounts": [
    { key: "code", labelKey: "finance.io.code", required: true },
    { key: "name", labelKey: "finance.io.name", required: true },
    { key: "kind", labelKey: "finance.io.kind", required: true },
    { key: "branch_code", labelKey: "finance.io.branch_code" },
    { key: "currency", labelKey: "finance.io.currency" },
    { key: "opening_balance", labelKey: "finance.io.opening_balance" },
    { key: "opening_date", labelKey: "finance.io.opening_date" },
    { key: "bank_name", labelKey: "finance.io.bank_name" },
    { key: "iban", labelKey: "finance.io.iban" },
    { key: "note", labelKey: "finance.io.note" },
  ],
  "tenant.cash_movements": [
    {
      key: "account_code",
      labelKey: "finance.io.account_code",
      required: true,
    },
    { key: "movement_date", labelKey: "finance.io.movement_date" },
    { key: "kind", labelKey: "finance.io.kind", required: true },
    { key: "direction", labelKey: "finance.io.direction" },
    { key: "amount", labelKey: "finance.io.amount", required: true },
    { key: "description", labelKey: "finance.io.description" },
    { key: "reason", labelKey: "finance.io.reason" },
    { key: "category_code", labelKey: "finance.io.category_code" },
  ],
  "tenant.expenses": [
    { key: "expense_date", labelKey: "finance.io.expense_date" },
    {
      key: "category_code",
      labelKey: "finance.io.category_code",
      required: true,
    },
    { key: "total_amount", labelKey: "finance.io.total_amount" },
    { key: "net_amount", labelKey: "finance.io.net_amount" },
    { key: "vat_rate", labelKey: "finance.io.vat_rate" },
    { key: "description", labelKey: "finance.io.description" },
    { key: "supplier_name", labelKey: "finance.io.supplier_name" },
    { key: "document_no", labelKey: "finance.io.document_no" },
    { key: "branch_code", labelKey: "finance.io.branch_code" },
    { key: "plate", labelKey: "finance.io.plate" },
    { key: "cash_account_code", labelKey: "finance.io.cash_account_code" },
  ],
};
