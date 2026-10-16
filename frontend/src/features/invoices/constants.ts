import type {
  InvoiceLineKind,
  InvoiceProfile,
  InvoiceStatus,
} from "@/features/invoices/types";

type Tone = "default" | "success" | "warning" | "danger";

/** Engine / live list resource (backend invoices module). */
export const INVOICES_RESOURCE = "tenant.invoices" as const;

export const INVOICE_STATUSES: InvoiceStatus[] = [
  "draft",
  "queued",
  "sent",
  "awaiting_result",
  "succeeded",
  "failed",
  "cancelled",
];

export const INVOICE_STATUS_LABEL_KEYS: Record<InvoiceStatus, string> = {
  draft: "invoices.status.draft",
  queued: "invoices.status.queued",
  sent: "invoices.status.sent",
  awaiting_result: "invoices.status.awaiting_result",
  succeeded: "invoices.status.succeeded",
  failed: "invoices.status.failed",
  cancelled: "invoices.status.cancelled",
};

export const INVOICE_STATUS_HINT_KEYS: Record<InvoiceStatus, string> = {
  draft: "invoices.status_hint.draft",
  queued: "invoices.status_hint.queued",
  sent: "invoices.status_hint.sent",
  awaiting_result: "invoices.status_hint.awaiting_result",
  succeeded: "invoices.status_hint.succeeded",
  failed: "invoices.status_hint.failed",
  cancelled: "invoices.status_hint.cancelled",
};

export const INVOICE_STATUS_TONES: Record<InvoiceStatus, Tone> = {
  draft: "default",
  queued: "warning",
  sent: "warning",
  awaiting_result: "warning",
  succeeded: "success",
  failed: "danger",
  cancelled: "default",
};

export const INVOICE_PROFILES: InvoiceProfile[] = [
  "TEMELFATURA",
  "TICARIFATURA",
  "EARSIVFATURA",
];

export const INVOICE_PROFILE_LABEL_KEYS: Record<InvoiceProfile, string> = {
  TEMELFATURA: "invoices.profile.temelfatura",
  TICARIFATURA: "invoices.profile.ticarifatura",
  EARSIVFATURA: "invoices.profile.earsivfatura",
};

export const INVOICE_TYPE_LABEL_KEYS: Record<string, string> = {
  SATIS: "invoices.type.satis",
  IADE: "invoices.type.iade",
};

export const LINE_KINDS: InvoiceLineKind[] = [
  "rent",
  "extra",
  "one_way",
  "charge",
  "toll",
];

export const LINE_KIND_LABEL_KEYS: Record<InvoiceLineKind, string> = {
  rent: "invoices.line_kind.rent",
  extra: "invoices.line_kind.extra",
  one_way: "invoices.line_kind.one_way",
  charge: "invoices.line_kind.charge",
  toll: "invoices.line_kind.toll",
};

export const HOLD_REASON_LABEL_KEYS: Record<string, string> = {
  no_account: "invoices.hold.no_account",
  adapter_unverified: "invoices.hold.adapter_unverified",
  adapter_unavailable: "invoices.hold.adapter_unavailable",
};

export const TAXPAYER_SOURCE_LABEL_KEYS: Record<string, string> = {
  provider: "invoices.taxpayer_source.provider",
  cache: "invoices.taxpayer_source.cache",
  customer_flag: "invoices.taxpayer_source.customer_flag",
};

export const RELEASE_REASON_LABEL_KEYS: Record<string, string> = {
  cancelled: "invoices.release.cancelled",
  returned: "invoices.release.returned",
  removed: "invoices.release.removed",
};

export const CONNECTION_STATE_LABEL_KEYS: Record<string, string> = {
  none: "invoices.connection.none",
  waiting: "invoices.connection.waiting",
  test: "invoices.connection.test",
  unavailable: "invoices.connection.unavailable",
  ready: "invoices.connection.ready",
};

export const CONNECTION_HINT_KEYS: Record<string, string> = {
  none: "invoices.settings.connection_hint.none",
  waiting: "invoices.settings.connection_hint.waiting",
  test: "invoices.settings.connection_hint.test",
  unavailable: "invoices.settings.connection_hint.unavailable",
  ready: "invoices.settings.connection_hint.ready",
};

export const RETRY_ACTION_KEYS: Record<string, string> = {
  rerouted: "invoices.toast.rerouted",
  already_done: "invoices.toast.already_done",
  in_progress: "invoices.toast.in_progress",
  resubmitted: "invoices.toast.resubmitted",
};

/** Form fields of the problems the API maps (receiver.*, lines.N.*, …). */
export const PROBLEM_FIELD_LABEL_KEYS: Record<string, string> = {
  "receiver.identity_number": "invoices.fields.identity_number",
  "receiver.name": "invoices.fields.receiver_name",
  "receiver.tax_office": "invoices.fields.tax_office",
  "receiver.district": "invoices.fields.district",
  "receiver.city": "invoices.fields.city",
  "receiver.postal_code": "invoices.fields.postal_code",
  "receiver.country": "invoices.fields.country",
  "receiver.email": "invoices.fields.email",
  receiver: "invoices.sections.receiver",
  "supplier.legal_name": "invoices.fields.supplier_legal_name",
  "supplier.tax_number": "invoices.fields.supplier_tax_number",
  "supplier.identity_number": "invoices.fields.supplier_tax_number",
  "supplier.tax_office": "invoices.fields.supplier_tax_office",
  "supplier.name": "invoices.fields.supplier_legal_name",
  "supplier.district": "invoices.fields.supplier_district",
  "supplier.city": "invoices.fields.supplier_city",
  supplier: "invoices.sections.supplier",
  profile: "invoices.fields.profile",
  issue_date: "invoices.fields.issue_date",
  number: "invoices.fields.number",
  returns: "invoices.fields.returns",
  lines: "invoices.sections.lines",
  document: "invoices.fields.document",
};

export const LINE_FIELD_LABEL_KEYS: Record<string, string> = {
  name: "invoices.fields.line_name",
  vat_rate: "invoices.fields.vat_rate",
  exemption_code: "invoices.fields.exemption_code",
  exemption_reason: "invoices.fields.exemption_reason",
  amount: "invoices.fields.amount",
};
