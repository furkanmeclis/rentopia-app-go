import type {
  HealthIssuePage,
  HealthSummary,
  ListIssuesParams,
  ListUsageParams,
  OrganizationUsagePage,
  UsageSummary,
} from "@/features/platform-health/types";
import { platformRequest } from "@/lib/api/platform-request";

/** Platform usage and connection health (`platform.health.read`). */
export const platformHealthService = {
  usage(params: ListUsageParams) {
    return platformRequest<OrganizationUsagePage>("GET", "/v1/platform/usage", {
      query: params,
    });
  },

  usageSummary(month?: string) {
    return platformRequest<UsageSummary>("GET", "/v1/platform/usage/summary", {
      query: { month },
    });
  },

  health() {
    return platformRequest<HealthSummary>("GET", "/v1/platform/health");
  },

  issues(params: ListIssuesParams) {
    return platformRequest<HealthIssuePage>(
      "GET",
      "/v1/platform/health/issues",
      { query: params },
    );
  },
};
