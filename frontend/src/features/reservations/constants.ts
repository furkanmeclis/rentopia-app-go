import { PREVIEW_WARNING_KEYS } from "@/features/pricing/constants";
import type {
  CapacityCommitment,
  PaymentStatus,
  QuotePurpose,
  QuoteStatus,
  ReservationStatus,
  TaskStatus,
  TaskType,
} from "@/features/reservations/types";

/** Engine / live resource slugs. */
export const RESERVATIONS_RESOURCE = "tenant.reservations" as const;
export const QUOTES_RESOURCE = "tenant.quotes" as const;
export const TASKS_RESOURCE = "tenant.resolution_tasks" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const RESERVATION_STATUSES: ReservationStatus[] = [
  "request",
  "awaiting_review",
  "awaiting_payment",
  "confirmed",
  "cancelled",
  "expired",
  "no_show",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const STATUS_LABEL_KEYS: Record<ReservationStatus, string> = {
  request: "reservations.statuses.request",
  awaiting_review: "reservations.statuses.awaiting_review",
  awaiting_payment: "reservations.statuses.awaiting_payment",
  confirmed: "reservations.statuses.confirmed",
  cancelled: "reservations.statuses.cancelled",
  expired: "reservations.statuses.expired",
  no_show: "reservations.statuses.no_show",
};

export const STATUS_TONES: Record<ReservationStatus, Tone> = {
  request: "default",
  awaiting_review: "warning",
  awaiting_payment: "warning",
  confirmed: "success",
  cancelled: "danger",
  expired: "default",
  no_show: "danger",
};

export const PAYMENT_STATUSES: PaymentStatus[] = [
  "unpaid",
  "partially_paid",
  "paid",
  "refunded",
];

export const PAYMENT_LABEL_KEYS: Record<PaymentStatus, string> = {
  unpaid: "reservations.payment_statuses.unpaid",
  partially_paid: "reservations.payment_statuses.partially_paid",
  paid: "reservations.payment_statuses.paid",
  refunded: "reservations.payment_statuses.refunded",
};

export const PAYMENT_TONES: Record<PaymentStatus, Tone> = {
  unpaid: "warning",
  partially_paid: "warning",
  paid: "success",
  refunded: "default",
};

export const COMMITMENT_LABEL_KEYS: Record<CapacityCommitment, string> = {
  none: "reservations.commitment.none",
  hold: "reservations.commitment.hold",
  firm: "reservations.commitment.firm",
};

export const QUOTE_PURPOSES: QuotePurpose[] = [
  "new",
  "change",
  "extension",
  "upgrade",
];

export const PURPOSE_LABEL_KEYS: Record<QuotePurpose, string> = {
  new: "reservations.quotes.purposes.new",
  change: "reservations.quotes.purposes.change",
  extension: "reservations.quotes.purposes.extension",
  upgrade: "reservations.quotes.purposes.upgrade",
};

export const QUOTE_STATUSES: QuoteStatus[] = ["open", "used", "superseded"];

export const QUOTE_STATUS_LABEL_KEYS: Record<QuoteStatus, string> = {
  open: "reservations.quotes.statuses.open",
  used: "reservations.quotes.statuses.used",
  superseded: "reservations.quotes.statuses.superseded",
};

export const QUOTE_STATUS_TONES: Record<QuoteStatus, Tone> = {
  open: "success",
  used: "default",
  superseded: "default",
};

export const TASK_TYPES: TaskType[] = [
  "block_conflict",
  "release_conflict",
  "extension_conflict",
  "replan_failed",
  "late_return",
  "late_payment",
];

export const TASK_TYPE_LABEL_KEYS: Record<TaskType, string> = {
  block_conflict: "reservations.tasks.types.block_conflict",
  release_conflict: "reservations.tasks.types.release_conflict",
  extension_conflict: "reservations.tasks.types.extension_conflict",
  replan_failed: "reservations.tasks.types.replan_failed",
  late_return: "reservations.tasks.types.late_return",
  late_payment: "reservations.tasks.types.late_payment",
};

export const TASK_STATUSES: TaskStatus[] = ["open", "resolved"];

export const TASK_STATUS_LABEL_KEYS: Record<TaskStatus, string> = {
  open: "reservations.tasks.statuses.open",
  resolved: "reservations.tasks.statuses.resolved",
};

export const TASK_STATUS_TONES: Record<TaskStatus, Tone> = {
  open: "warning",
  resolved: "success",
};

/** Timeline actions of `GET /reservations/{uuid}/events`. */
export const EVENT_LABEL_KEYS: Record<string, string> = {
  created: "reservations.events.created",
  submit: "reservations.events.submit",
  approve: "reservations.events.approve",
  approved: "reservations.events.approve",
  confirm: "reservations.events.confirm",
  cancel: "reservations.events.cancel",
  expire: "reservations.events.expire",
  no_show: "reservations.events.no_show",
  changed: "reservations.events.changed",
  extended: "reservations.events.extended",
  upgraded: "reservations.events.upgraded",
  plate_changed: "reservations.events.plate_changed",
  plan_moved: "reservations.events.plan_moved",
  replanned: "reservations.events.replanned",
  plan_broken: "reservations.events.plan_broken",
  drivers_changed: "reservations.events.drivers_changed",
  assigned: "reservations.events.assigned",
  payment_recorded: "reservations.events.payment_recorded",
  equipment_warning: "reservations.events.equipment_warning",
  coupon_warning: "reservations.events.coupon_warning",
  task_resolved: "reservations.events.task_resolved",
  reject: "reservations.events.reject",
  reinstate: "reservations.events.reinstate",
  late_payment: "reservations.events.late_payment",
  payment_link_created: "reservations.events.payment_link_created",
  payment_link_cancelled: "reservations.events.payment_link_cancelled",
  payment_refunded: "reservations.events.payment_refunded",
  customer_request: "reservations.events.customer_request",
  customer_request_resolved: "reservations.events.customer_request_resolved",
  customer_request_rejected: "reservations.events.customer_request_rejected",
};

export const ASSIGNMENT_SOURCE_LABEL_KEYS: Record<string, string> = {
  engine: "reservations.plan.sources.engine",
  staff: "reservations.plan.sources.staff",
  drag: "reservations.plan.sources.drag",
  replan: "reservations.plan.sources.replan",
  upgrade: "reservations.plan.sources.upgrade",
  extension: "reservations.plan.sources.extension",
};

/** Driver eligibility reasons (customers driver eligibility codes). */
export const DRIVER_REASON_KEYS: Record<string, string> = {
  birth_date_missing: "reservations.driver_reasons.birth_date_missing",
  under_min_age: "reservations.driver_reasons.under_min_age",
  licence_missing: "reservations.driver_reasons.licence_missing",
  licence_issue_date_missing:
    "reservations.driver_reasons.licence_issue_date_missing",
  under_min_licence_years:
    "reservations.driver_reasons.under_min_licence_years",
  licence_expired: "reservations.driver_reasons.licence_expired",
};

/** Pricing / quote warnings (pricing preview codes + reservation ones). */
export const WARNING_KEYS: Record<string, string> = {
  ...PREVIEW_WARNING_KEYS,
  equipment_unavailable: "reservations.warnings.equipment_unavailable",
};

/** Translates a reason through a static map; unknown codes stay as is. */
export function reasonText(
  t: (key: string) => string,
  map: Record<string, string>,
  code: string,
): string {
  const key = map[code];
  return key ? t(key) : code;
}
