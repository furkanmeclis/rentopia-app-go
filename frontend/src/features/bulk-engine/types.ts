import type { LucideIcon } from "lucide-react";

/** Bulk action contract from resource meta API (no icon). */
export type BulkActionMeta = {
  id: string;
  label_key: string;
  permission: string;
  destructive?: boolean;
  reversible?: boolean;
  confirm_key?: string;
};

/** Bulk action for UI — icon is required. */
export type BulkActionDef = BulkActionMeta & {
  icon: LucideIcon;
};

export type BulkTarget = {
  scope: "ids" | "query";
  ids?: string[];
  query?: Record<string, string>;
  /** Arguments of parameterised actions (e.g. the new vehicle status). */
  params?: Record<string, string>;
};

export type BulkSummary = {
  total: number;
  succeeded: number;
  failed: number;
};

export type BulkJob = {
  uuid: string;
  resource: string;
  action: string;
  status: string;
  target?: BulkTarget;
  summary?: BulkSummary;
  error?: string | null;
  rollback_until?: string | null;
  applied_at?: string | null;
  created_at: string;
};

export type BulkExecuteSyncResult = {
  sync: true;
  summary: BulkSummary;
};

export type BulkResource =
  | "platform.users"
  | "platform.roles"
  | "platform.organizations"
  | "tenant.branches"
  | "tenant.rental_groups"
  | "tenant.rental_products"
  | "tenant.vehicles"
  | "tenant.one_way_rules"
  | "tenant.customers"
  | "tenant.extras"
  | "tenant.campaigns"
  | "tenant.reservations";

/**
 * Execute route per resource. `tenant.*` resources are organization-scoped:
 * call them with a tenant `ApiScope` (see `useApiScope`).
 */
export const BULK_PATHS: Record<BulkResource, string> = {
  "platform.users": "/v1/platform/users/bulk",
  "platform.roles": "/v1/platform/roles/bulk",
  "platform.organizations": "/v1/platform/organizations/bulk",
  "tenant.branches": "/v1/tenant/branches/bulk",
  "tenant.rental_groups": "/v1/tenant/rental-groups/bulk",
  "tenant.rental_products": "/v1/tenant/products/bulk",
  "tenant.vehicles": "/v1/tenant/vehicles/bulk",
  "tenant.one_way_rules": "/v1/tenant/one-way-rules/bulk",
  "tenant.customers": "/v1/tenant/customers/bulk",
  "tenant.extras": "/v1/tenant/extras/bulk",
  "tenant.campaigns": "/v1/tenant/campaigns/bulk",
  "tenant.reservations": "/v1/tenant/reservations/bulk",
};

export type SelectionScope =
  | { mode: "none" }
  | { mode: "page"; ids: string[] }
  | { mode: "all"; query: Record<string, string>; total: number };

export type ListJobsResult = {
  items: BulkJob[];
  total: number;
  limit: number;
  offset: number;
};
