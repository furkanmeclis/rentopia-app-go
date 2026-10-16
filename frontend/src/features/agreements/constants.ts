import type {
  AgreementStatus,
  AgreementValidity,
  ContactRole,
  InvoicePeriod,
  PeriodStatus,
} from "@/features/agreements/types";

/** Engine / live resource slug. */
export const AGREEMENTS_RESOURCE = "tenant.agreements" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const AGREEMENT_STATUSES: AgreementStatus[] = [
  "draft",
  "active",
  "suspended",
  "terminated",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const AGREEMENT_STATUS_LABEL_KEYS: Record<AgreementStatus, string> = {
  draft: "agreements.statuses.draft",
  active: "agreements.statuses.active",
  suspended: "agreements.statuses.suspended",
  terminated: "agreements.statuses.terminated",
};

export const AGREEMENT_STATUS_TONES: Record<AgreementStatus, Tone> = {
  draft: "default",
  active: "success",
  suspended: "warning",
  terminated: "danger",
};

/** Status changes the API accepts from each status. */
export const AGREEMENT_TRANSITIONS: Record<AgreementStatus, AgreementStatus[]> =
  {
    draft: ["active", "terminated"],
    active: ["suspended", "terminated"],
    suspended: ["active", "terminated"],
    terminated: [],
  };

export const AGREEMENT_VALIDITIES: AgreementValidity[] = [
  "upcoming",
  "current",
  "expired",
];

export const VALIDITY_LABEL_KEYS: Record<AgreementValidity, string> = {
  upcoming: "agreements.validities.upcoming",
  current: "agreements.validities.current",
  expired: "agreements.validities.expired",
};

export const VALIDITY_TONES: Record<AgreementValidity, Tone> = {
  upcoming: "warning",
  current: "success",
  expired: "default",
};

export const INVOICE_PERIODS: InvoicePeriod[] = ["monthly", "per_rental"];

export const INVOICE_PERIOD_LABEL_KEYS: Record<InvoicePeriod, string> = {
  monthly: "agreements.invoice_periods.monthly",
  per_rental: "agreements.invoice_periods.per_rental",
};

export const INVOICE_PERIOD_HINT_KEYS: Record<InvoicePeriod, string> = {
  monthly: "agreements.invoice_periods.monthly_hint",
  per_rental: "agreements.invoice_periods.per_rental_hint",
};

export const CONTACT_ROLES: ContactRole[] = [
  "signatory",
  "billing",
  "operations",
  "other",
];

export const CONTACT_ROLE_LABEL_KEYS: Record<ContactRole, string> = {
  signatory: "agreements.contact_roles.signatory",
  billing: "agreements.contact_roles.billing",
  operations: "agreements.contact_roles.operations",
  other: "agreements.contact_roles.other",
};

export const PERIOD_STATUS_LABEL_KEYS: Record<PeriodStatus, string> = {
  planned: "agreements.period_statuses.planned",
  posted: "agreements.period_statuses.posted",
  superseded: "agreements.period_statuses.superseded",
  reversed: "agreements.period_statuses.reversed",
};

export const PERIOD_STATUS_TONES: Record<PeriodStatus, Tone> = {
  planned: "default",
  posted: "success",
  superseded: "warning",
  reversed: "danger",
};

export const ENTRY_KIND_LABEL_KEYS: Record<string, string> = {
  accrual: "agreements.entry_kinds.accrual",
  reversal: "agreements.entry_kinds.reversal",
  collection: "agreements.entry_kinds.collection",
  refund: "agreements.entry_kinds.refund",
  adjustment: "agreements.entry_kinds.adjustment",
  offset: "agreements.entry_kinds.offset",
  payment: "agreements.entry_kinds.payment",
};
