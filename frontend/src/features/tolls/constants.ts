import type {
  TollEventAction,
  TollReviewReason,
  TollSource,
  TollStatus,
} from "@/features/tolls/types";

/** Engine / live resource slug. */
export const TOLLS_RESOURCE = "tenant.toll_passes" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const TOLL_STATUSES: TollStatus[] = [
  "needs_review",
  "unmatched",
  "charged",
  "matched",
  "ignored",
];

/** The review queue (unmatched + needs_review). */
export const REVIEW_STATUSES: TollStatus[] = ["needs_review", "unmatched"];

/** Static i18n key maps (the i18n checker verifies every key). */
export const TOLL_STATUS_LABEL_KEYS: Record<TollStatus, string> = {
  unmatched: "tolls.statuses.unmatched",
  needs_review: "tolls.statuses.needs_review",
  matched: "tolls.statuses.matched",
  charged: "tolls.statuses.charged",
  ignored: "tolls.statuses.ignored",
};

export const TOLL_STATUS_TONES: Record<TollStatus, Tone> = {
  unmatched: "warning",
  needs_review: "danger",
  matched: "success",
  charged: "success",
  ignored: "default",
};

export const REVIEW_REASONS: Exclude<TollReviewReason, "">[] = [
  "no_rental",
  "missing_actual_times",
  "outside_actual_window",
  "overlap",
  "in_transfer",
  "vehicle_unknown",
  "vehicle_conflict",
  "rental_cancelled",
  "unassigned",
];

export const REVIEW_REASON_LABEL_KEYS: Record<
  Exclude<TollReviewReason, "">,
  string
> = {
  vehicle_unknown: "tolls.review_reasons.vehicle_unknown",
  vehicle_conflict: "tolls.review_reasons.vehicle_conflict",
  no_rental: "tolls.review_reasons.no_rental",
  overlap: "tolls.review_reasons.overlap",
  missing_actual_times: "tolls.review_reasons.missing_actual_times",
  outside_actual_window: "tolls.review_reasons.outside_actual_window",
  in_transfer: "tolls.review_reasons.in_transfer",
  rental_cancelled: "tolls.review_reasons.rental_cancelled",
  unassigned: "tolls.review_reasons.unassigned",
};

export const SOURCE_LABEL_KEYS: Record<TollSource, string> = {
  api: "tolls.sources.api",
  file: "tolls.sources.file",
};

export const MATCH_METHOD_LABEL_KEYS: Record<string, string> = {
  plate: "tolls.match_methods.plate",
  tag: "tolls.match_methods.tag",
  manual: "tolls.match_methods.manual",
};

export const EVENT_LABEL_KEYS: Record<TollEventAction, string> = {
  received: "tolls.events.received",
  revised: "tolls.events.revised",
  matched: "tolls.events.matched",
  review: "tolls.events.review",
  assigned: "tolls.events.assigned",
  unassigned: "tolls.events.unassigned",
  ignored: "tolls.events.ignored",
  rematched: "tolls.events.rematched",
  charge_created: "tolls.events.charge_created",
  charge_reversed: "tolls.events.charge_reversed",
};

export const CHANGED_FIELD_LABEL_KEYS: Record<string, string> = {
  passed_at: "tolls.columns.passed_at",
  plate: "tolls.columns.plate",
  tag_number: "tolls.columns.tag_number",
  station: "tolls.columns.station",
  exit_station: "tolls.fields.exit_station",
  amount: "tolls.columns.amount",
  currency: "tolls.fields.currency",
};

export const TOLL_CHARGE_KIND_LABEL_KEYS: Record<string, string> = {
  toll: "tolls.charges.kinds.toll",
  toll_fee: "tolls.charges.kinds.toll_fee",
};

export const TAG_NETWORKS = ["hgs", "ogs"] as const;

export const TAG_NETWORK_LABEL_KEYS: Record<string, string> = {
  hgs: "tolls.tags.networks.hgs",
  ogs: "tolls.tags.networks.ogs",
};
