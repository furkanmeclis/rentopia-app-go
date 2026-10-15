import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type RatePlan = Schemas["RatePlan"];
export type RatePlanPage = Schemas["RatePlanPage"];
export type RatePlanVersion = Schemas["RatePlanVersion"];
export type RatePlanFacets = Schemas["RatePlanFacets"];
export type RatePlanCreateRequest = Schemas["RatePlanCreateRequest"];
export type RatePlanPatchRequest = Schemas["RatePlanPatchRequest"];
export type RateBand = Schemas["RateBand"];
export type RateBandInput = Schemas["RateBandInput"];
export type RatePlanScope = RatePlan["scope"];
export type PricingStatus = RatePlan["status"];
export type VersionStatus = RatePlanVersion["status"];

export type RatePlanMatrix = Schemas["RatePlanMatrix"];
export type RatePlanMatrixBand = Schemas["RatePlanMatrixBand"];
export type RatePlanMatrixRow = Schemas["RatePlanMatrixRow"];
export type RatePlanMatrixTarget = Schemas["RatePlanMatrixTarget"];
export type RatePlanMatrixSaveRequest = Schemas["RatePlanMatrixSaveRequest"];
export type RatePlanMatrixSaveResult = Schemas["RatePlanMatrixSaveResult"];
export type RatePlanMatrixActivateResult =
  Schemas["RatePlanMatrixActivateResult"];

export type DateRule = Schemas["DateRule"];
export type DateRulePage = Schemas["DateRulePage"];
export type DateRuleVersion = Schemas["DateRuleVersion"];
export type DateRuleFacets = Schemas["DateRuleFacets"];
export type DateRuleCreateRequest = Schemas["DateRuleCreateRequest"];
export type DateRulePatchRequest = Schemas["DateRulePatchRequest"];
export type DateRuleKind = DateRuleVersion["kind"];
export type ProductScope = DateRuleVersion["product_scope"];

export type ActivationCheck = Schemas["ActivationCheck"];
export type PricingIssue = Schemas["PricingIssue"];
export type PricingActivateRequest = Schemas["PricingActivateRequest"];

export type PricingPreviewRequest = Schemas["PricingPreviewRequest"];
export type PricingPreview = Schemas["PricingPreview"];
export type PricingPreviewSide = Schemas["PricingPreviewSide"];
export type PricingBreakdown = Schemas["PricingBreakdown"];
export type PricingDayLine = PricingBreakdown["days"][number];
export type PricingLine = PricingBreakdown["lines"][number];

export type PricingActivityPage = Schemas["FleetActivityPage"];

/** List query of `GET /v1/tenant/rate-plans` (multi-values comma-joined). */
export type ListRatePlansParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  rental_product?: string;
  branch?: string;
  scope?: string;
  channel?: string;
  status?: string;
  has_draft?: "true" | "false";
};

/** List query of `GET /v1/tenant/date-rules`. */
export type ListDateRulesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  kind?: string;
  has_draft?: "true" | "false";
  date_from?: string;
  date_to?: string;
};

/** Query of `GET /v1/tenant/rate-plans/matrix`. */
export type MatrixQuery = {
  scope: RatePlanScope;
  branch?: string;
  channel?: string;
  priority?: number;
};
