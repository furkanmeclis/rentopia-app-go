import type {
  ChannelEntityType,
  ChannelHealth,
  ChannelMessageStatus,
  ChannelMessageType,
} from "@/features/channels/types";

/** Live list resources (backend channels/model). */
export const CHANNEL_ACCOUNTS_RESOURCE = "tenant.channel_accounts" as const;
export const CHANNEL_MAPPINGS_RESOURCE = "tenant.channel_mappings" as const;
export const CHANNEL_MESSAGES_RESOURCE = "tenant.channel_messages" as const;

type Tone = "default" | "success" | "warning" | "danger";

export const CHANNEL_TABS = [
  "accounts",
  "mappings",
  "messages",
  "runs",
  "reconciliation",
] as const;
export type ChannelTab = (typeof CHANNEL_TABS)[number];

export const ENTITY_TYPES: ChannelEntityType[] = ["branch", "product", "extra"];
export const ENTITY_TYPE_LABEL_KEYS: Record<ChannelEntityType, string> = {
  branch: "channels.entity_types.branch",
  product: "channels.entity_types.product",
  extra: "channels.entity_types.extra",
};

export const MESSAGE_STATUSES: ChannelMessageStatus[] = [
  "failed",
  "applied",
  "duplicate",
  "stale",
  "ignored",
  "received",
];
export const MESSAGE_STATUS_LABEL_KEYS: Record<ChannelMessageStatus, string> = {
  received: "channels.message_statuses.received",
  applied: "channels.message_statuses.applied",
  duplicate: "channels.message_statuses.duplicate",
  stale: "channels.message_statuses.stale",
  failed: "channels.message_statuses.failed",
  ignored: "channels.message_statuses.ignored",
};
export const MESSAGE_STATUS_TONES: Record<ChannelMessageStatus, Tone> = {
  received: "warning",
  applied: "success",
  duplicate: "default",
  stale: "default",
  failed: "danger",
  ignored: "default",
};

export const MESSAGE_TYPES: ChannelMessageType[] = [
  "booking",
  "modification",
  "cancellation",
];
export const MESSAGE_TYPE_LABEL_KEYS: Record<ChannelMessageType, string> = {
  booking: "channels.message_types.booking",
  modification: "channels.message_types.modification",
  cancellation: "channels.message_types.cancellation",
};

export const ERROR_CODE_LABEL_KEYS: Record<string, string> = {
  invalid_message: "channels.error_codes.invalid_message",
  unknown_location: "channels.error_codes.unknown_location",
  unknown_product: "channels.error_codes.unknown_product",
  ambiguous_product: "channels.error_codes.ambiguous_product",
  unknown_extra: "channels.error_codes.unknown_extra",
  capacity_unavailable: "channels.error_codes.capacity_unavailable",
  pricing_unavailable: "channels.error_codes.pricing_unavailable",
  reinstate_not_supported: "channels.error_codes.reinstate_not_supported",
  cancel_refused: "channels.error_codes.cancel_refused",
  change_refused: "channels.error_codes.change_refused",
  booking_refused: "channels.error_codes.booking_refused",
  customer_failed: "channels.error_codes.customer_failed",
  internal_error: "channels.error_codes.internal_error",
};

export const HEALTH_LABEL_KEYS: Record<ChannelHealth, string> = {
  ok: "channels.health.ok",
  pending: "channels.health.pending",
  failing: "channels.health.failing",
  expired: "channels.health.expired",
  disabled: "channels.health.disabled",
};
export const HEALTH_TONES: Record<ChannelHealth, Tone> = {
  ok: "success",
  pending: "warning",
  failing: "danger",
  expired: "danger",
  disabled: "default",
};

export const RUN_DIRECTIONS = ["push", "pull", "reconcile"] as const;
export const RUN_DIRECTION_LABEL_KEYS: Record<string, string> = {
  push: "channels.run_directions.push",
  pull: "channels.run_directions.pull",
  reconcile: "channels.run_directions.reconcile",
};
export const RUN_STATUSES = [
  "running",
  "succeeded",
  "partial",
  "failed",
] as const;
export const RUN_STATUS_LABEL_KEYS: Record<string, string> = {
  running: "channels.run_statuses.running",
  succeeded: "channels.run_statuses.succeeded",
  partial: "channels.run_statuses.partial",
  failed: "channels.run_statuses.failed",
};
export const RUN_STATUS_TONES: Record<string, Tone> = {
  running: "warning",
  succeeded: "success",
  partial: "warning",
  failed: "danger",
};
export const RUN_TRIGGER_LABEL_KEYS: Record<string, string> = {
  event: "channels.run_triggers.event",
  schedule: "channels.run_triggers.schedule",
  manual: "channels.run_triggers.manual",
};

export const MATCH_STATUSES = [
  "matched",
  "missing_local",
  "missing_channel",
  "status_mismatch",
  "amount_mismatch",
  "commission_mismatch",
] as const;
export const MATCH_LABEL_KEYS: Record<string, string> = {
  matched: "channels.matches.matched",
  missing_local: "channels.matches.missing_local",
  missing_channel: "channels.matches.missing_channel",
  status_mismatch: "channels.matches.status_mismatch",
  amount_mismatch: "channels.matches.amount_mismatch",
  commission_mismatch: "channels.matches.commission_mismatch",
};
export const MATCH_TONES: Record<string, Tone> = {
  matched: "success",
  missing_local: "danger",
  missing_channel: "warning",
  status_mismatch: "danger",
  amount_mismatch: "warning",
  commission_mismatch: "warning",
};

export const COLLECTED_BY_LABEL_KEYS: Record<string, string> = {
  channel: "channels.collected_by.channel",
  us: "channels.collected_by.us",
};
