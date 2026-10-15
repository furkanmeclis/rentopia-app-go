import type { ResourceMeta } from "@/features/io/types";
import type {
  ActivationCheck,
  DateRule,
  DateRuleCreateRequest,
  DateRuleFacets,
  DateRulePage,
  DateRulePatchRequest,
  DateRuleVersion,
  ListDateRulesParams,
  ListRatePlansParams,
  MatrixQuery,
  PricingActivateRequest,
  PricingActivityPage,
  PricingPreview,
  PricingPreviewRequest,
  RatePlan,
  RatePlanCreateRequest,
  RatePlanFacets,
  RatePlanMatrix,
  RatePlanMatrixActivateResult,
  RatePlanMatrixSaveRequest,
  RatePlanMatrixSaveResult,
  RatePlanMatrixTarget,
  RatePlanPage,
  RatePlanPatchRequest,
  RatePlanVersion,
} from "@/features/pricing/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const PLANS = "/v1/tenant/rate-plans";
const RULES = "/v1/tenant/date-rules";

type PageParams = { limit: number; offset: number };

/**
 * Rate plans, date rules and the preview of one organization. Every call
 * names the organization through `tenantRequest` (`X-Organization`).
 * Activation calls are silent: the caller shows the issue list.
 */
export const pricingService = {
  // --- rate plans ---
  plans(slug: string, params: ListRatePlansParams) {
    return tenantRequest<RatePlanPage>(slug, "GET", PLANS, { query: params });
  },
  plansMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${PLANS}/meta`);
  },
  planFacets(slug: string) {
    return tenantRequest<RatePlanFacets>(slug, "GET", `${PLANS}/facets`);
  },
  plan(slug: string, uuid: string) {
    return tenantRequest<RatePlan>(slug, "GET", `${PLANS}/${uuid}`);
  },
  createPlan(slug: string, body: RatePlanCreateRequest) {
    return tenantRequest<RatePlan>(slug, "POST", PLANS, { body });
  },
  updatePlan(slug: string, uuid: string, body: RatePlanPatchRequest) {
    return tenantRequest<RatePlan>(slug, "PATCH", `${PLANS}/${uuid}`, {
      body,
    });
  },
  removePlan(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${PLANS}/${uuid}`,
    );
  },
  discardPlanDraft(slug: string, uuid: string) {
    return tenantRequest<RatePlan>(slug, "DELETE", `${PLANS}/${uuid}/draft`);
  },
  planCheck(slug: string, uuid: string) {
    return tenantRequest<ActivationCheck>(
      slug,
      "GET",
      `${PLANS}/${uuid}/activation-check`,
    );
  },
  activatePlan(slug: string, uuid: string, body: PricingActivateRequest) {
    return tenantRequest<RatePlan>(slug, "POST", `${PLANS}/${uuid}/activate`, {
      body,
      silent: true,
    });
  },
  deactivatePlan(slug: string, uuid: string) {
    return tenantRequest<RatePlan>(slug, "POST", `${PLANS}/${uuid}/deactivate`);
  },
  planVersions(slug: string, uuid: string) {
    return tenantRequest<{ items: RatePlanVersion[] }>(
      slug,
      "GET",
      `${PLANS}/${uuid}/versions`,
    );
  },
  planActivity(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<PricingActivityPage>(
      slug,
      "GET",
      `${PLANS}/${uuid}/activity`,
      { query: params },
    );
  },

  // --- matrix ---
  matrix(slug: string, query: MatrixQuery) {
    return tenantRequest<RatePlanMatrix>(slug, "GET", `${PLANS}/matrix`, {
      query,
    });
  },
  saveMatrix(slug: string, body: RatePlanMatrixSaveRequest) {
    return tenantRequest<RatePlanMatrixSaveResult>(
      slug,
      "PUT",
      `${PLANS}/matrix`,
      { body },
    );
  },
  activateMatrix(slug: string, body: RatePlanMatrixTarget) {
    return tenantRequest<RatePlanMatrixActivateResult>(
      slug,
      "POST",
      `${PLANS}/matrix/activate`,
      { body, silent: true },
    );
  },

  // --- date rules ---
  rules(slug: string, params: ListDateRulesParams) {
    return tenantRequest<DateRulePage>(slug, "GET", RULES, { query: params });
  },
  rulesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${RULES}/meta`);
  },
  ruleFacets(slug: string) {
    return tenantRequest<DateRuleFacets>(slug, "GET", `${RULES}/facets`);
  },
  rule(slug: string, uuid: string) {
    return tenantRequest<DateRule>(slug, "GET", `${RULES}/${uuid}`);
  },
  createRule(slug: string, body: DateRuleCreateRequest) {
    return tenantRequest<DateRule>(slug, "POST", RULES, { body });
  },
  updateRule(slug: string, uuid: string, body: DateRulePatchRequest) {
    return tenantRequest<DateRule>(slug, "PATCH", `${RULES}/${uuid}`, {
      body,
    });
  },
  removeRule(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${RULES}/${uuid}`,
    );
  },
  discardRuleDraft(slug: string, uuid: string) {
    return tenantRequest<DateRule>(slug, "DELETE", `${RULES}/${uuid}/draft`);
  },
  ruleCheck(slug: string, uuid: string) {
    return tenantRequest<ActivationCheck>(
      slug,
      "GET",
      `${RULES}/${uuid}/activation-check`,
    );
  },
  activateRule(slug: string, uuid: string, body: PricingActivateRequest) {
    return tenantRequest<DateRule>(slug, "POST", `${RULES}/${uuid}/activate`, {
      body,
      silent: true,
    });
  },
  deactivateRule(slug: string, uuid: string) {
    return tenantRequest<DateRule>(slug, "POST", `${RULES}/${uuid}/deactivate`);
  },
  ruleVersions(slug: string, uuid: string) {
    return tenantRequest<{ items: DateRuleVersion[] }>(
      slug,
      "GET",
      `${RULES}/${uuid}/versions`,
    );
  },
  ruleActivity(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<PricingActivityPage>(
      slug,
      "GET",
      `${RULES}/${uuid}/activity`,
      { query: params },
    );
  },

  // --- preview ---
  preview(slug: string, body: PricingPreviewRequest) {
    return tenantRequest<PricingPreview>(
      slug,
      "POST",
      "/v1/tenant/pricing/preview",
      { body },
    );
  },
};
