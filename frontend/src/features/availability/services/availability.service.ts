import type {
  AvailabilityHold,
  AvailabilityHoldPage,
  AvailabilityHoldRequest,
  AvailabilityPlanResult,
  AvailabilityReleaseResult,
  AvailabilitySearchRequest,
  AvailabilitySearchResult,
  CapacityRule,
  CapacityRulePage,
  CapacityRuleRequest,
  ListCapacityRulesParams,
  ListHoldsParams,
} from "@/features/availability/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const AVAILABILITY = "/v1/tenant/availability";
const RULES = "/v1/tenant/capacity-rules";

/**
 * Availability engine of one organization: capacity search, holds
 * ("opsiyon") and channel capacity rules. Every call names the organization
 * through `tenantRequest` (`X-Organization`).
 */
export const availabilityService = {
  search(slug: string, body: AvailabilitySearchRequest) {
    return tenantRequest<AvailabilitySearchResult>(
      slug,
      "POST",
      `${AVAILABILITY}/search`,
      { body },
    );
  },
  holds(slug: string, params: ListHoldsParams) {
    return tenantRequest<AvailabilityHoldPage>(
      slug,
      "GET",
      `${AVAILABILITY}/holds`,
      { query: params },
    );
  },
  /** Silent: the hold dialog shows CAPACITY_UNAVAILABLE with alternatives. */
  createHold(slug: string, body: AvailabilityHoldRequest) {
    return tenantRequest<AvailabilityPlanResult>(
      slug,
      "POST",
      `${AVAILABILITY}/holds`,
      { body, silent: true },
    );
  },
  releaseHold(slug: string, uuid: string) {
    return tenantRequest<AvailabilityReleaseResult>(
      slug,
      "DELETE",
      `${AVAILABILITY}/holds/${uuid}`,
    );
  },
  extendHold(slug: string, uuid: string, minutes: number) {
    return tenantRequest<AvailabilityHold>(
      slug,
      "POST",
      `${AVAILABILITY}/holds/${uuid}/extend`,
      { body: { minutes } },
    );
  },

  rules(slug: string, params: ListCapacityRulesParams) {
    return tenantRequest<CapacityRulePage>(slug, "GET", RULES, {
      query: params,
    });
  },
  createRule(slug: string, body: CapacityRuleRequest) {
    return tenantRequest<CapacityRule>(slug, "POST", RULES, { body });
  },
  updateRule(slug: string, uuid: string, body: CapacityRuleRequest) {
    return tenantRequest<CapacityRule>(slug, "PUT", `${RULES}/${uuid}`, {
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
};
