import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type AvailabilitySearchRequest = Schemas["AvailabilitySearchRequest"];
export type AvailabilitySearchResult = Schemas["AvailabilitySearchResult"];
export type AvailabilityProduct = Schemas["AvailabilityProduct"];
export type AvailabilityBreakdown = Schemas["AvailabilityBreakdown"];
export type AvailabilityReason = AvailabilityProduct["reasons"][number];
export type AvailabilityHold = Schemas["AvailabilityHold"];
export type AvailabilityHoldPage = Schemas["AvailabilityHoldPage"];
export type AvailabilityHoldRequest = Schemas["AvailabilityHoldRequest"];
export type AvailabilityPlanResult = Schemas["AvailabilityPlanResult"];
export type AvailabilityReleaseResult = Schemas["AvailabilityReleaseResult"];
export type AvailabilityAlternatives = Schemas["AvailabilityAlternatives"];

export type CapacityRule = Schemas["CapacityRule"];
export type CapacityRulePage = Schemas["CapacityRulePage"];
export type CapacityRuleRequest = Schemas["CapacityRuleRequest"];
export type CapacityRuleType = CapacityRule["rule_type"];

/** List query of `GET /v1/tenant/availability/holds`. */
export type ListHoldsParams = {
  limit: number;
  offset: number;
  status?: string;
};

/** List query of `GET /v1/tenant/capacity-rules` (multi values comma-joined). */
export type ListCapacityRulesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  rule_type?: string;
  channel?: string;
};
