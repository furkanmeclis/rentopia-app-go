import type {
  IntegrationJobStatus,
  PaymentLinkStatus,
  PaymentMethod,
  PaymentStatus,
  PublicPayState,
} from "@/features/payments/types";

/** Live list resources (backend payments/model). */
export const PAYMENTS_RESOURCE = "tenant.payments" as const;
export const PAYMENT_LINKS_RESOURCE = "tenant.payment_links" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const PAYMENT_STATUSES: PaymentStatus[] = [
  "pending",
  "succeeded",
  "partially_refunded",
  "refunded",
  "failed",
  "cancelled",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const PAYMENT_STATUS_LABEL_KEYS: Record<PaymentStatus, string> = {
  pending: "payments.statuses.pending",
  succeeded: "payments.statuses.succeeded",
  partially_refunded: "payments.statuses.partially_refunded",
  refunded: "payments.statuses.refunded",
  failed: "payments.statuses.failed",
  cancelled: "payments.statuses.cancelled",
};

export const PAYMENT_STATUS_TONES: Record<PaymentStatus, Tone> = {
  pending: "warning",
  succeeded: "success",
  partially_refunded: "warning",
  refunded: "default",
  failed: "danger",
  cancelled: "default",
};

export const METHOD_LABEL_KEYS: Record<PaymentMethod, string> = {
  checkout_form: "payments.methods.checkout_form",
  cash: "payments.methods.cash",
  card_pos: "payments.methods.card_pos",
  bank_transfer: "payments.methods.bank_transfer",
};

export const MANUAL_METHODS: Exclude<PaymentMethod, "checkout_form">[] = [
  "cash",
  "card_pos",
  "bank_transfer",
];

export const LINK_STATUS_LABEL_KEYS: Record<PaymentLinkStatus, string> = {
  active: "payments.links.statuses.active",
  paid: "payments.links.statuses.paid",
  expired: "payments.links.statuses.expired",
  cancelled: "payments.links.statuses.cancelled",
};

export const LINK_STATUS_TONES: Record<PaymentLinkStatus, Tone> = {
  active: "warning",
  paid: "success",
  expired: "default",
  cancelled: "default",
};

/** Reservation outcome of a verified payment. */
export const OUTCOME_LABEL_KEYS: Record<string, string> = {
  recorded: "payments.outcomes.recorded",
  confirmed: "payments.outcomes.confirmed",
  reinstated: "payments.outcomes.reinstated",
  late_payment_task: "payments.outcomes.late_payment_task",
  applying: "payments.outcomes.applying",
};

export const VERIFIED_VIA_LABEL_KEYS: Record<string, string> = {
  webhook: "payments.verified_via.webhook",
  retrieve: "payments.verified_via.retrieve",
  manual: "payments.verified_via.manual",
};

export const PUBLIC_STATE_TITLE_KEYS: Record<PublicPayState, string> = {
  ready: "payments.pay.states.ready_title",
  processing: "payments.pay.states.processing_title",
  paid: "payments.pay.states.paid_title",
  failed: "payments.pay.states.failed_title",
  expired: "payments.pay.states.expired_title",
  cancelled: "payments.pay.states.cancelled_title",
};

export const PUBLIC_STATE_BODY_KEYS: Record<PublicPayState, string> = {
  ready: "payments.pay.states.ready_body",
  processing: "payments.pay.states.processing_body",
  paid: "payments.pay.states.paid_body",
  failed: "payments.pay.states.failed_body",
  expired: "payments.pay.states.expired_body",
  cancelled: "payments.pay.states.cancelled_body",
};

export const PUBLIC_RESULT_KEYS: Record<string, string> = {
  confirmed: "payments.pay.results.confirmed",
  reinstated: "payments.pay.results.confirmed",
  recorded: "payments.pay.results.recorded",
  late_payment_task: "payments.pay.results.late_payment_task",
};

export const INTEGRATION_FIELD_LABEL_KEYS: Record<string, string> = {
  api_key: "payments.integrations.fields.api_key",
  secret_key: "payments.integrations.fields.secret_key",
  usercode: "payments.integrations.fields.usercode",
  password: "payments.integrations.fields.password",
  msgheader: "payments.integrations.fields.msgheader",
  customer_no: "payments.integrations.fields.customer_no",
};

export const INTEGRATION_PROVIDER_LABEL_KEYS: Record<string, string> = {
  iyzico: "payments.integrations.providers.iyzico",
  netgsm: "payments.integrations.providers.netgsm",
  log: "payments.integrations.providers.log",
  hgs_fake: "payments.integrations.providers.hgs_fake",
};

/** Connect form hint per provider (where the organization finds its keys). */
export const INTEGRATION_PROVIDER_HINT_KEYS: Record<string, string> = {
  iyzico: "payments.integrations.provider_hints.iyzico",
  netgsm: "payments.integrations.provider_hints.netgsm",
  log: "payments.integrations.provider_hints.log",
  hgs_fake: "payments.integrations.provider_hints.hgs_fake",
};

/** Live list resource of integration jobs (backend jobs usecase). */
export const INTEGRATION_JOBS_RESOURCE = "tenant.integration_jobs" as const;

export const INTEGRATION_JOB_STATUSES: IntegrationJobStatus[] = [
  "pending",
  "processing",
  "submitted",
  "succeeded",
  "failed",
  "needs_verification",
  "ignored",
];

export const INTEGRATION_JOB_STATUS_LABEL_KEYS: Record<
  IntegrationJobStatus,
  string
> = {
  pending: "payments.integrations.jobs.statuses.pending",
  processing: "payments.integrations.jobs.statuses.processing",
  submitted: "payments.integrations.jobs.statuses.submitted",
  succeeded: "payments.integrations.jobs.statuses.succeeded",
  failed: "payments.integrations.jobs.statuses.failed",
  needs_verification: "payments.integrations.jobs.statuses.needs_verification",
  ignored: "payments.integrations.jobs.statuses.ignored",
};

export const INTEGRATION_JOB_STATUS_TONES: Record<IntegrationJobStatus, Tone> =
  {
    pending: "warning",
    processing: "warning",
    submitted: "warning",
    succeeded: "success",
    failed: "danger",
    needs_verification: "danger",
    ignored: "default",
  };

export const INTEGRATION_JOB_KIND_LABEL_KEYS: Record<string, string> = {
  "sms.send": "payments.integrations.jobs.kinds.sms_send",
  "payment.refund": "payments.integrations.jobs.kinds.payment_refund",
  "payment.webhook": "payments.integrations.jobs.kinds.payment_webhook",
  "toll.fetch": "payments.integrations.jobs.kinds.toll_fetch",
};

export const ATTEMPT_OPERATION_LABEL_KEYS: Record<string, string> = {
  submit: "payments.integrations.jobs.operations.submit",
  check_status: "payments.integrations.jobs.operations.check_status",
};

export const ATTEMPT_TRIGGER_LABEL_KEYS: Record<string, string> = {
  worker: "payments.integrations.jobs.triggers.worker",
  sweep: "payments.integrations.jobs.triggers.sweep",
  retry: "payments.integrations.jobs.triggers.retry",
  inline: "payments.integrations.jobs.triggers.inline",
};

export const ATTEMPT_OUTCOME_LABEL_KEYS: Record<string, string> = {
  succeeded: "payments.integrations.jobs.outcomes.succeeded",
  accepted: "payments.integrations.jobs.outcomes.accepted",
  pending: "payments.integrations.jobs.outcomes.pending",
  failed: "payments.integrations.jobs.outcomes.failed",
  not_found: "payments.integrations.jobs.outcomes.not_found",
  uncertain: "payments.integrations.jobs.outcomes.uncertain",
  unsupported: "payments.integrations.jobs.outcomes.unsupported",
};

export const ATTEMPT_OUTCOME_TONES: Record<string, Tone> = {
  succeeded: "success",
  accepted: "success",
  pending: "warning",
  failed: "danger",
  not_found: "default",
  uncertain: "danger",
  unsupported: "danger",
};

export const CHECK_STATUS_LABEL_KEYS: Record<string, string> = {
  "": "payments.integrations.check.never",
  ok: "payments.integrations.check.ok",
  failed: "payments.integrations.check.failed",
};
