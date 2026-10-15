import type {
  CampaignScope,
  CampaignState,
  CampaignType,
  DiscountKind,
  RedemptionStatus,
} from "@/features/campaigns/types";

/** Engine / live resource slug. */
export const CAMPAIGNS_RESOURCE = "tenant.campaigns" as const;

export const CAMPAIGN_TYPES: CampaignType[] = ["coupon", "repeat_customer"];
export const DISCOUNT_KINDS: DiscountKind[] = ["percent", "amount"];
export const CAMPAIGN_STATES: CampaignState[] = [
  "active",
  "scheduled",
  "exhausted",
  "expired",
  "inactive",
];
export const CAMPAIGN_SCOPES: CampaignScope[] = ["all", "segments", "products"];
export const REDEMPTION_STATUSES: RedemptionStatus[] = [
  "reserved",
  "consumed",
  "released",
];

/** Static i18n key maps (the i18n checker verifies every key). */
export const TYPE_LABEL_KEYS: Record<CampaignType, string> = {
  coupon: "campaigns.type.coupon",
  repeat_customer: "campaigns.type.repeat_customer",
};

export const KIND_LABEL_KEYS: Record<DiscountKind, string> = {
  percent: "campaigns.kind.percent",
  amount: "campaigns.kind.amount",
};

export const STATE_LABEL_KEYS: Record<CampaignState, string> = {
  active: "campaigns.state.active",
  scheduled: "campaigns.state.scheduled",
  exhausted: "campaigns.state.exhausted",
  expired: "campaigns.state.expired",
  inactive: "campaigns.state.inactive",
};

export const STATE_TONES: Record<
  CampaignState,
  "default" | "success" | "warning" | "danger"
> = {
  active: "success",
  scheduled: "warning",
  exhausted: "danger",
  expired: "default",
  inactive: "default",
};

export const SCOPE_LABEL_KEYS: Record<CampaignScope, string> = {
  all: "campaigns.scope.all",
  segments: "campaigns.scope.segments",
  products: "campaigns.scope.products",
};

export const REDEMPTION_STATUS_LABEL_KEYS: Record<RedemptionStatus, string> = {
  reserved: "campaigns.redemptions.status.reserved",
  consumed: "campaigns.redemptions.status.consumed",
  released: "campaigns.redemptions.status.released",
};

export const REDEMPTION_STATUS_TONES: Record<
  RedemptionStatus,
  "default" | "success" | "warning" | "danger"
> = {
  reserved: "warning",
  consumed: "success",
  released: "default",
};

/** Coupon rejection reasons of the price preview. */
export const COUPON_REASON_KEYS: Record<string, string> = {
  not_found: "campaigns.reason.not_found",
  inactive: "campaigns.reason.inactive",
  not_started: "campaigns.reason.not_started",
  expired: "campaigns.reason.expired",
  channel: "campaigns.reason.channel",
  scope: "campaigns.reason.scope",
  customer_scope: "campaigns.reason.customer_scope",
  customer_required: "campaigns.reason.customer_required",
  customer_limit: "campaigns.reason.customer_limit",
  limit: "campaigns.reason.limit",
  min_amount: "campaigns.reason.min_amount",
  corporate: "campaigns.reason.corporate",
  not_eligible: "campaigns.reason.not_eligible",
};
