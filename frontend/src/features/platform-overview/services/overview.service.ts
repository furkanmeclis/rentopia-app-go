import type { SiteDomain } from "@/features/site/types";
import { usersService } from "@/features/users/services/users.service";
import { platformRequest } from "@/lib/api/platform-request";

/**
 * Small reads the platform overview needs that no feature hook exposes.
 * Everything else on the page reuses the owning feature's service and
 * query keys (organizations, platform health, storage, activity).
 */
export const overviewService = {
  /** Platform user count (`limit=1` + `total`). */
  async usersTotal(): Promise<number> {
    const page = await usersService.list({ limit: 1, offset: 0 });
    return page.total;
  },

  /** Same read as the platform domains page, so both share one cache entry. */
  siteDomains() {
    return platformRequest<SiteDomain[]>("GET", "/v1/platform/site-domains", {
      query: { limit: 200 },
    });
  },
};
