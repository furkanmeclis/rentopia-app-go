import type {
  TenantOverview,
  TenantOverviewBadges,
} from "@/features/overview/types";
import { tenantRequest } from "@/lib/api/tenant-request";

export const overviewKeys = {
  all: (slug: string) => ["tenant", slug, "overview"] as const,
  page: (slug: string, branch: string | undefined) =>
    [...overviewKeys.all(slug), "page", branch ?? "all"] as const,
  badges: (slug: string) => [...overviewKeys.all(slug), "badges"] as const,
};

export const overviewService = {
  /** Operations overview (KPIs + sections), optionally for one branch. */
  get(slug: string, branch?: string, signal?: AbortSignal) {
    return tenantRequest<TenantOverview>(slug, "GET", "/v1/tenant/overview", {
      query: branch ? { branch } : undefined,
      signal,
    });
  },
  /** Sidebar counts: approval queue and open resolution tasks. */
  badges(slug: string, signal?: AbortSignal) {
    return tenantRequest<TenantOverviewBadges>(
      slug,
      "GET",
      "/v1/tenant/overview/badges",
      { silent: true, signal },
    );
  },
};
