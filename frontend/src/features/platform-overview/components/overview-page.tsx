"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { organizationKeys } from "@/features/organizations/hooks/use-organizations";
import { platformHealthKeys } from "@/features/platform-health/hooks/use-platform-health";
import { AttentionPanel } from "@/features/platform-overview/components/attention-panel";
import { OverviewHero } from "@/features/platform-overview/components/overview-hero";
import { OverviewShortcuts } from "@/features/platform-overview/components/overview-shortcuts";
import {
  RecentActivityPanel,
  RecentOrganizationsPanel,
} from "@/features/platform-overview/components/recent-panels";
import {
  HealthPanel,
  StoragePanel,
} from "@/features/platform-overview/components/system-panels";
import { overviewKeys } from "@/features/platform-overview/hooks/query-keys";
import { useAttention } from "@/features/platform-overview/hooks/use-attention";
import {
  useHealthOverview,
  useOverviewScopes,
  useRecentActivity,
  useRecentOrganizations,
  useStorageOverview,
  useUsageTrend,
  useUsersTotal,
} from "@/features/platform-overview/hooks/use-overview-query";
import { storageKeys } from "@/features/storage/hooks/query-keys";
import { isPlatformUser } from "@/lib/auth/types";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Platform home (`/platform`): an operator's view of the whole SaaS.
 * Live totals and the reservation trend, what needs attention (access
 * windows, connection issues, custom domains), connection health, file
 * store, newest organizations and recent activity. Every widget reads an
 * existing endpoint and is shown only with its permission.
 */
export function OverviewPage() {
  const { t } = useLocale();
  const { user } = useAuth();
  const queryClient = useQueryClient();
  const scopes = useOverviewScopes();
  const platformUser = isPlatformUser(user);

  const trend = useUsageTrend(platformUser && scopes.health);
  const usersTotal = useUsersTotal(platformUser && scopes.users);
  const recentOrgs = useRecentOrganizations(
    platformUser && scopes.organizations,
  );
  const health = useHealthOverview(platformUser && scopes.health);
  const storage = useStorageOverview(platformUser && scopes.storage);
  const activity = useRecentActivity(platformUser && scopes.activity);
  const attention = useAttention(scopes);

  const [refreshing, setRefreshing] = useState(false);
  const refresh = async () => {
    setRefreshing(true);
    try {
      await Promise.all(
        [
          platformHealthKeys.all,
          organizationKeys.all,
          overviewKeys.all,
          overviewKeys.siteDomains(),
          ioKeys.activity.lists(),
          storageKeys.usage(),
        ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      );
    } finally {
      setRefreshing(false);
    }
  };

  if (!platformUser) {
    return (
      <EmptyState
        title={t("common.error_forbidden")}
        description={t("dashboard.forbidden_description")}
      />
    );
  }

  const showAttention =
    scopes.organizations || scopes.health || scopes.siteDomains;
  // Main column: what to act on; side column: system state and the log.
  const hasMain = showAttention || scopes.organizations;
  const hasSide = scopes.health || scopes.storage || scopes.activity;

  return (
    <div className="space-y-5">
      <OverviewHero
        scopes={scopes}
        name={user?.name ?? ""}
        months={trend.months}
        trend={trend.results}
        usersTotal={usersTotal}
        organizationsTotal={recentOrgs.data?.total}
        organizationsLoading={recentOrgs.isLoading}
        attention={{
          total: attention.total,
          isLoading: showAttention && attention.isLoading,
          hasFailures: attention.failures.length > 0,
        }}
        refreshing={refreshing}
        onRefresh={() => void refresh()}
      />

      <OverviewShortcuts scopes={scopes} />

      {hasMain || hasSide ? (
        <div
          className={cn(
            "grid items-start gap-5",
            hasMain && hasSide && "xl:grid-cols-3",
          )}
        >
          {hasMain ? (
            <div className={cn("grid gap-5", hasSide && "xl:col-span-2")}>
              {showAttention ? <AttentionPanel {...attention} /> : null}
              {scopes.organizations ? (
                <RecentOrganizationsPanel
                  query={recentOrgs}
                  canCreate={scopes.createOrganization}
                />
              ) : null}
            </div>
          ) : null}
          {hasSide ? (
            <div
              className={cn(
                "grid items-start gap-5 md:grid-cols-2",
                hasMain ? "xl:grid-cols-1" : "xl:grid-cols-3",
              )}
            >
              {scopes.health ? <HealthPanel query={health} /> : null}
              {scopes.storage ? <StoragePanel query={storage} /> : null}
              {scopes.activity ? (
                <RecentActivityPanel query={activity} />
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
