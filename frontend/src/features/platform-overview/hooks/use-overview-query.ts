"use client";

import { useQueries, useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { ioKeys } from "@/features/io/hooks/query-keys";
import {
  activityService,
  type ListActivityParams,
} from "@/features/io/services/activity.service";
import { organizationKeys } from "@/features/organizations/hooks/use-organizations";
import { organizationsService } from "@/features/organizations/services/organizations.service";
import type { ListOrganizationsParams } from "@/features/organizations/types";
import { platformHealthKeys } from "@/features/platform-health/hooks/use-platform-health";
import { recentMonths } from "@/features/platform-health/lib/labels";
import { platformHealthService } from "@/features/platform-health/services/platform-health.service";
import type { ListIssuesParams } from "@/features/platform-health/types";
import {
  ACCESS_ENDING_SCAN,
  OVERVIEW_STALE_MS,
  RECENT_LIMIT,
  TREND_MONTHS,
} from "@/features/platform-overview/constants";
import { overviewKeys } from "@/features/platform-overview/hooks/query-keys";
import { overviewService } from "@/features/platform-overview/services/overview.service";
import { useStorageUsage } from "@/features/storage/hooks/use-storage";
import { usePermission } from "@/providers/permission-provider";

/** What the signed-in platform user may read; each widget gates on one. */
export function useOverviewScopes() {
  const { can } = usePermission();
  return useMemo(
    () => ({
      organizations: can(permissions.organizations.read),
      createOrganization: can(permissions.organizations.write),
      health: can(permissions.platformHealth.read),
      siteDomains: can(permissions.siteDomains.read),
      users: can(permissions.users.read),
      roles: can(permissions.roles.read),
      catalog: can(permissions.catalog.read),
      notifications: can(permissions.notifications.platformRead),
      activity: can(permissions.activity.read),
      storage: can(permissions.storage.read),
    }),
    [can],
  );
}

export type OverviewScopes = ReturnType<typeof useOverviewScopes>;

/** Health page hooks always fetch; the overview only fetches when allowed. */
const HEALTH_REFRESH_MS = 60_000;

function useOrganizationPage(
  params: ListOrganizationsParams,
  enabled: boolean,
) {
  return useQuery({
    queryKey: organizationKeys.list(params),
    queryFn: () => organizationsService.list(params),
    enabled,
    staleTime: OVERVIEW_STALE_MS,
  });
}

/** Usage totals of the last months, newest first (shared with the usage tab). */
export function useUsageTrend(enabled: boolean) {
  const months = useMemo(() => recentMonths(new Date(), TREND_MONTHS), []);
  const results = useQueries({
    queries: months.map((month) => ({
      queryKey: platformHealthKeys.usageSummary(month),
      queryFn: () => platformHealthService.usageSummary(month),
      enabled,
      staleTime: 5 * 60_000,
    })),
  });
  return { months, results };
}

export function useHealthOverview(enabled: boolean) {
  return useQuery({
    queryKey: platformHealthKeys.summary(),
    queryFn: () => platformHealthService.health(),
    enabled,
    refetchInterval: HEALTH_REFRESH_MS,
  });
}

const OPEN_ISSUES: ListIssuesParams = {
  limit: 5,
  offset: 0,
  sort: "-occurred_at",
  severity: "critical,warning",
};

export function useOpenIssues(enabled: boolean) {
  return useQuery({
    queryKey: platformHealthKeys.issues(OPEN_ISSUES),
    queryFn: () => platformHealthService.issues(OPEN_ISSUES),
    enabled,
    refetchInterval: HEALTH_REFRESH_MS,
  });
}

const EXPIRED: ListOrganizationsParams = {
  limit: 5,
  offset: 0,
  access: "expired",
  sort: "-access_ends_at",
};
const ENDING: ListOrganizationsParams = {
  limit: ACCESS_ENDING_SCAN,
  offset: 0,
  access: "open",
  window: "limited",
  sort: "access_ends_at",
};
const SUSPENDED: ListOrganizationsParams = {
  limit: 1,
  offset: 0,
  status: "suspended",
};
const RECENT: ListOrganizationsParams = {
  limit: RECENT_LIMIT,
  offset: 0,
  sort: "-created_at",
};

export function useOrganizationSignals(enabled: boolean) {
  return {
    expired: useOrganizationPage(EXPIRED, enabled),
    ending: useOrganizationPage(ENDING, enabled),
    suspended: useOrganizationPage(SUSPENDED, enabled),
  };
}

export function useRecentOrganizations(enabled: boolean) {
  return useOrganizationPage(RECENT, enabled);
}

export function useSiteDomains(enabled: boolean) {
  return useQuery({
    queryKey: overviewKeys.siteDomains(),
    queryFn: () => overviewService.siteDomains(),
    enabled,
    staleTime: OVERVIEW_STALE_MS,
  });
}

export function useUsersTotal(enabled: boolean) {
  return useQuery({
    queryKey: overviewKeys.usersTotal(),
    queryFn: () => overviewService.usersTotal(),
    enabled,
    staleTime: OVERVIEW_STALE_MS,
  });
}

const RECENT_ACTIVITY: ListActivityParams = {
  limit: 6,
  offset: 0,
  sort: "-created_at",
};

export function useRecentActivity(enabled: boolean) {
  return useQuery({
    queryKey: ioKeys.activity.list(RECENT_ACTIVITY),
    queryFn: () => activityService.list(RECENT_ACTIVITY),
    enabled,
    staleTime: OVERVIEW_STALE_MS,
  });
}

export function useStorageOverview(enabled: boolean) {
  return useStorageUsage(enabled);
}
