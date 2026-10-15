import type {
  ContractKind,
  DocumentStatus,
  TemplateStatus,
  VersionStatus,
} from "@/features/contracts/types";

/** Live list resources. */
export const TEMPLATES_RESOURCE = "tenant.contract_templates" as const;
export const DOCUMENTS_RESOURCE = "tenant.documents" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const CONTRACT_KINDS: ContractKind[] = [
  "rental_contract",
  "handover_form",
  "return_form",
  "terms",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const KIND_LABEL_KEYS: Record<ContractKind, string> = {
  rental_contract: "contracts.kinds.rental_contract",
  handover_form: "contracts.kinds.handover_form",
  return_form: "contracts.kinds.return_form",
  terms: "contracts.kinds.terms",
};

export const TEMPLATE_STATUSES: TemplateStatus[] = [
  "draft",
  "active",
  "archived",
];

export const TEMPLATE_STATUS_LABEL_KEYS: Record<TemplateStatus, string> = {
  draft: "contracts.template_statuses.draft",
  active: "contracts.template_statuses.active",
  archived: "contracts.template_statuses.archived",
};

export const TEMPLATE_STATUS_TONES: Record<TemplateStatus, Tone> = {
  draft: "warning",
  active: "success",
  archived: "default",
};

export const DOCUMENT_STATUS_LABEL_KEYS: Record<DocumentStatus, string> = {
  draft: "contracts.document_statuses.draft",
  approved: "contracts.document_statuses.approved",
  void: "contracts.document_statuses.void",
};

export const DOCUMENT_STATUS_TONES: Record<DocumentStatus, Tone> = {
  draft: "warning",
  approved: "success",
  void: "default",
};

export const VERSION_STATUS_LABEL_KEYS: Record<VersionStatus, string> = {
  draft: "contracts.version_statuses.draft",
  issued: "contracts.version_statuses.issued",
  approved: "contracts.version_statuses.approved",
  superseded: "contracts.version_statuses.superseded",
};

export const VERSION_STATUS_TONES: Record<VersionStatus, Tone> = {
  draft: "warning",
  issued: "default",
  approved: "success",
  superseded: "default",
};

export const PDF_STATUS_LABEL_KEYS: Record<string, string> = {
  none: "contracts.pdf_statuses.none",
  pending: "contracts.pdf_statuses.pending",
  ready: "contracts.pdf_statuses.ready",
  failed: "contracts.pdf_statuses.failed",
};

export const PLACEHOLDER_GROUP_LABEL_KEYS: Record<string, string> = {
  company: "contracts.placeholder_groups.company",
  reservation: "contracts.placeholder_groups.reservation",
  renter: "contracts.placeholder_groups.renter",
  invoice_customer: "contracts.placeholder_groups.invoice_customer",
  drivers: "contracts.placeholder_groups.drivers",
  vehicle: "contracts.placeholder_groups.vehicle",
  product: "contracts.placeholder_groups.product",
  dates: "contracts.placeholder_groups.dates",
  points: "contracts.placeholder_groups.points",
  amounts: "contracts.placeholder_groups.amounts",
  extras: "contracts.placeholder_groups.extras",
  policies: "contracts.placeholder_groups.policies",
  document: "contracts.placeholder_groups.document",
};

/** Label of every catalog placeholder (unknown keys show the token). */
export const PLACEHOLDER_LABEL_KEYS: Record<string, string> = {
  "company.legal_name": "contracts.placeholders.company.legal_name",
  "company.tax_number": "contracts.placeholders.company.tax_number",
  "company.tax_office": "contracts.placeholders.company.tax_office",
  "company.address": "contracts.placeholders.company.address",
  "company.phone": "contracts.placeholders.company.phone",
  "company.email": "contracts.placeholders.company.email",
  "reservation.reference": "contracts.placeholders.reservation.reference",
  "reservation.channel": "contracts.placeholders.reservation.channel",
  "rental.reference": "contracts.placeholders.rental.reference",
  "renter.full_name": "contracts.placeholders.renter.full_name",
  "renter.identity": "contracts.placeholders.renter.identity",
  "renter.phone": "contracts.placeholders.renter.phone",
  "renter.email": "contracts.placeholders.renter.email",
  "invoice_customer.full_name":
    "contracts.placeholders.invoice_customer.full_name",
  "drivers.primary_name": "contracts.placeholders.drivers.primary_name",
  "drivers.primary_licence": "contracts.placeholders.drivers.primary_licence",
  "drivers.table": "contracts.placeholders.drivers.table",
  "vehicle.plate": "contracts.placeholders.vehicle.plate",
  "vehicle.label": "contracts.placeholders.vehicle.label",
  "vehicle.km_out": "contracts.placeholders.vehicle.km_out",
  "vehicle.fuel_out": "contracts.placeholders.vehicle.fuel_out",
  "vehicle.km_in": "contracts.placeholders.vehicle.km_in",
  "vehicle.fuel_in": "contracts.placeholders.vehicle.fuel_in",
  "product.title": "contracts.placeholders.product.title",
  "product.code": "contracts.placeholders.product.code",
  "product.guarantee": "contracts.placeholders.product.guarantee",
  "product.deposit": "contracts.placeholders.product.deposit",
  "product.included_km_per_day":
    "contracts.placeholders.product.included_km_per_day",
  "product.extra_km_fee": "contracts.placeholders.product.extra_km_fee",
  "product.fuel_policy": "contracts.placeholders.product.fuel_policy",
  "dates.pickup_at": "contracts.placeholders.dates.pickup_at",
  "dates.return_at": "contracts.placeholders.dates.return_at",
  "dates.actual_pickup_at": "contracts.placeholders.dates.actual_pickup_at",
  "dates.actual_return_at": "contracts.placeholders.dates.actual_return_at",
  "dates.billing_days": "contracts.placeholders.dates.billing_days",
  "points.pickup_branch": "contracts.placeholders.points.pickup_branch",
  "points.return_branch": "contracts.placeholders.points.return_branch",
  "points.pickup_point": "contracts.placeholders.points.pickup_point",
  "points.return_point": "contracts.placeholders.points.return_point",
  "points.actual_return_branch":
    "contracts.placeholders.points.actual_return_branch",
  "amounts.total": "contracts.placeholders.amounts.total",
  "amounts.prepayment": "contracts.placeholders.amounts.prepayment",
  "amounts.paid": "contracts.placeholders.amounts.paid",
  "amounts.balance": "contracts.placeholders.amounts.balance",
  "amounts.extensions_total": "contracts.placeholders.amounts.extensions_total",
  "amounts.currency": "contracts.placeholders.amounts.currency",
  "extras.table": "contracts.placeholders.extras.table",
  "policies.cancellation": "contracts.placeholders.policies.cancellation",
  "policies.no_show": "contracts.placeholders.policies.no_show",
  "policies.early_return": "contracts.placeholders.policies.early_return",
  "policies.extension": "contracts.placeholders.policies.extension",
  "policies.fuel": "contracts.placeholders.policies.fuel",
  "policies.mileage": "contracts.placeholders.policies.mileage",
  "policies.late_return_grace_minutes":
    "contracts.placeholders.policies.late_return_grace_minutes",
  "policies.extra_hour_fee": "contracts.placeholders.policies.extra_hour_fee",
  "document.version": "contracts.placeholders.document.version",
  "document.date": "contracts.placeholders.document.date",
};

export function placeholderLabel(t: (key: string) => string, key: string) {
  const label = PLACEHOLDER_LABEL_KEYS[key];
  return label ? t(label) : key;
}
