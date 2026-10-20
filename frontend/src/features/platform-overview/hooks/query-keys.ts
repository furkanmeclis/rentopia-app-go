export const overviewKeys = {
  all: ["platform-overview"] as const,
  usersTotal: () => [...overviewKeys.all, "users-total"] as const,
  /** Shared with `PlatformSiteDomainsPage` (same request, same key). */
  siteDomains: () => ["platform", "site-domains"] as const,
};
