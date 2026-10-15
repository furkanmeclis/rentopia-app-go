import type {
  AvailabilityReason,
  CapacityRuleType,
} from "@/features/availability/types";

/** Live channels (org:{uuid}:resource:…). */
export const HOLDS_RESOURCE = "tenant.availability_holds";
export const CAPACITY_RULES_RESOURCE = "tenant.capacity_rules";

/** Panel requests sell through the branch channel unless told otherwise. */
export const DEFAULT_CHANNEL = "branch";

/** Default option time offered in the hold dialog (minutes). */
export const DEFAULT_HOLD_MINUTES = 30;
export const MAX_HOLD_MINUTES = 1440;

export const RULE_TYPES: CapacityRuleType[] = [
  "safety_margin",
  "guaranteed_quota",
  "sales_limit",
];

/** Static label keys (no template keys, see check-i18n). */
export const RULE_TYPE_LABEL_KEYS: Record<CapacityRuleType, string> = {
  sales_limit: "availability.rule_type.sales_limit",
  guaranteed_quota: "availability.rule_type.guaranteed_quota",
  safety_margin: "availability.rule_type.safety_margin",
};

export const RULE_TYPE_HINT_KEYS: Record<CapacityRuleType, string> = {
  sales_limit: "availability.rule_type.sales_limit_hint",
  guaranteed_quota: "availability.rule_type.guaranteed_quota_hint",
  safety_margin: "availability.rule_type.safety_margin_hint",
};

export const REASON_LABEL_KEYS: Record<AvailabilityReason, string> = {
  one_way_not_allowed: "availability.reason.one_way_not_allowed",
  no_eligible_vehicle: "availability.reason.no_eligible_vehicle",
};

/** CAPACITY_UNAVAILABLE detail codes. */
export const CAPACITY_REASON_LABEL_KEYS: Record<string, string> = {
  no_vehicle: "availability.capacity_reason.no_vehicle",
  no_eligible_vehicle: "availability.capacity_reason.no_eligible_vehicle",
  one_way_not_allowed: "availability.capacity_reason.one_way_not_allowed",
  channel_rules: "availability.capacity_reason.channel_rules",
  product_not_sellable: "availability.capacity_reason.product_not_sellable",
};
