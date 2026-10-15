import type {
  DateRuleKind,
  PricingStatus,
  ProductScope,
  RatePlanScope,
  VersionStatus,
} from "@/features/pricing/types";

/** Engine / live resource slugs. */
export const RATE_PLANS_RESOURCE = "tenant.rate_plans" as const;
export const DATE_RULES_RESOURCE = "tenant.date_rules" as const;

export const PLAN_SCOPES: RatePlanScope[] = [
  "common",
  "branch",
  "channel",
  "branch_channel",
];

export const PRICING_STATUSES: PricingStatus[] = [
  "draft",
  "active",
  "inactive",
];

export const RULE_KINDS: DateRuleKind[] = ["percent", "amount"];

export const PRODUCT_SCOPES: ProductScope[] = ["all", "segments", "products"];

/** Channels that are not online (the counter); see backend orgsettings. */
export const OFFLINE_CHANNELS = ["branch"];

/** Scopes that need a branch / a channel. */
export const BRANCH_SCOPES: RatePlanScope[] = ["branch", "branch_channel"];
export const CHANNEL_SCOPES: RatePlanScope[] = ["channel", "branch_channel"];

/** Static i18n key maps (the i18n checker verifies every key). */
export const SCOPE_LABEL_KEYS: Record<RatePlanScope, string> = {
  common: "pricing.scope.common",
  branch: "pricing.scope.branch",
  channel: "pricing.scope.channel",
  branch_channel: "pricing.scope.branch_channel",
};

export const SCOPE_HINT_KEYS: Record<RatePlanScope, string> = {
  common: "pricing.scope.common_hint",
  branch: "pricing.scope.branch_hint",
  channel: "pricing.scope.channel_hint",
  branch_channel: "pricing.scope.branch_channel_hint",
};

export const STATUS_LABEL_KEYS: Record<PricingStatus, string> = {
  draft: "pricing.status.draft",
  active: "pricing.status.active",
  inactive: "pricing.status.inactive",
};

export const STATUS_TONES: Record<
  PricingStatus,
  "default" | "success" | "warning" | "danger"
> = {
  draft: "warning",
  active: "success",
  inactive: "default",
};

export const VERSION_STATUS_LABEL_KEYS: Record<VersionStatus, string> = {
  draft: "pricing.version_status.draft",
  active: "pricing.version_status.active",
  superseded: "pricing.version_status.superseded",
};

export const VERSION_STATUS_TONES: Record<
  VersionStatus,
  "default" | "success" | "warning" | "danger"
> = {
  draft: "warning",
  active: "success",
  superseded: "default",
};

export const KIND_LABEL_KEYS: Record<DateRuleKind, string> = {
  percent: "pricing.kind.percent",
  amount: "pricing.kind.amount",
};

export const PRODUCT_SCOPE_LABEL_KEYS: Record<ProductScope, string> = {
  all: "pricing.product_scope.all",
  segments: "pricing.product_scope.segments",
  products: "pricing.product_scope.products",
};

/** Activation issue codes → message keys (params interpolated). */
export const ISSUE_LABEL_KEYS: Record<string, string> = {
  no_draft: "pricing.issues.no_draft",
  no_bands: "pricing.issues.no_bands",
  band_range: "pricing.issues.band_range",
  band_price: "pricing.issues.band_price",
  band_overlap: "pricing.issues.band_overlap",
  coverage_start: "pricing.issues.coverage_start",
  coverage_gap: "pricing.issues.coverage_gap",
  coverage_short: "pricing.issues.coverage_short",
  priority_conflict: "pricing.issues.priority_conflict",
};

/** Preview engine error codes → message keys. */
export const PREVIEW_ERROR_KEYS: Record<string, string> = {
  no_plan: "pricing.preview.errors.no_plan",
  ambiguous_plan: "pricing.preview.errors.ambiguous_plan",
  no_band: "pricing.preview.errors.no_band",
  ambiguous_band: "pricing.preview.errors.ambiguous_band",
  ambiguous_rule: "pricing.preview.errors.ambiguous_rule",
  invalid_input: "pricing.preview.errors.invalid_input",
};

export const PREVIEW_WARNING_KEYS: Record<string, string> = {
  vat_rate_missing: "pricing.preview.warnings.vat_rate_missing",
  one_way_not_allowed: "pricing.preview.warnings.one_way_not_allowed",
  one_way_unavailable: "pricing.preview.warnings.one_way_unavailable",
  no_changes: "pricing.preview.warnings.no_changes",
  coupons_unavailable: "pricing.preview.warnings.coupons_unavailable",
};

export const LINE_LABEL_KEYS: Record<string, string> = {
  rental: "pricing.preview.lines.rental",
  extra: "pricing.preview.lines.extra",
  one_way: "pricing.preview.lines.one_way",
  delivery: "pricing.preview.lines.delivery",
};

export const PREPAYMENT_LABEL_KEYS: Record<string, string> = {
  none: "pricing.preview.prepayment.none",
  fixed: "pricing.preview.prepayment.fixed",
  percent: "pricing.preview.prepayment.percent",
  full: "pricing.preview.prepayment.full",
};

/** Default İdes-style band layout for a new plan / matrix. */
export const DEFAULT_BANDS: { min_days: number; max_days: number | null }[] = [
  { min_days: 1, max_days: 3 },
  { min_days: 4, max_days: 7 },
  { min_days: 8, max_days: 14 },
  { min_days: 15, max_days: 29 },
  { min_days: 30, max_days: null },
];
