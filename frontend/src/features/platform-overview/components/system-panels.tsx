"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { HardDrive } from "lucide-react";

import { Skeleton } from "@/components/ui/skeleton";
import { Progress } from "@/components/ui/progress";
import { routes } from "@/config/routes";
import { HealthStatusChip } from "@/features/platform-health/components/health-chips";
import {
  formatBytes,
  healthLabel,
} from "@/features/platform-health/lib/labels";
import type { HealthSummary } from "@/features/platform-health/types";
import {
  OverviewPanel,
  PanelError,
  PanelNote,
} from "@/features/platform-overview/components/overview-panel";
import type { StorageUsage } from "@/features/storage/types";
import { relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Overall connection health and one row per registered source. */
export function HealthPanel({
  query,
}: {
  query: UseQueryResult<HealthSummary>;
}) {
  const { t, locale } = useLocale();
  const data = query.data;

  return (
    <OverviewPanel
      title={t("dashboard.health.title")}
      description={
        data
          ? t("dashboard.health.checked", {
              time: relativeDatetime(data.checked_at, locale),
            })
          : undefined
      }
      link={{
        href: `${routes.platform.health}?tab=health`,
        label: t("dashboard.health.open"),
      }}
    >
      {query.isError && !data ? (
        <PanelError
          description={t("platform_health.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : !data ? (
        <div className="space-y-3" aria-busy>
          <Skeleton className="h-14 w-full rounded-xl" />
          <Skeleton className="h-9 w-full rounded-xl" />
          <Skeleton className="h-9 w-full rounded-xl" />
        </div>
      ) : (
        <div className="space-y-3">
          <div className="bg-muted/50 flex items-center justify-between gap-3 rounded-xl px-3 py-3">
            <div className="min-w-0">
              <p className="text-sm font-medium">
                {t("platform_health.overall")}
              </p>
              <p className="text-muted-foreground text-xs">
                {data.critical === 0 && data.warning === 0
                  ? t("platform_health.overall_ok")
                  : [
                      data.critical > 0
                        ? t("platform_health.overall_critical", {
                            count: data.critical,
                          })
                        : null,
                      data.warning > 0
                        ? t("platform_health.overall_warning", {
                            count: data.warning,
                          })
                        : null,
                    ]
                      .filter(Boolean)
                      .join(", ")}
              </p>
            </div>
            <HealthStatusChip status={data.status} />
          </div>
          {data.sources.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("dashboard.health.no_sources")}
            </p>
          ) : (
            <ul className="divide-border divide-y">
              {data.sources.map((source) => (
                <li
                  key={source.code}
                  className="flex items-center justify-between gap-3 py-2.5"
                >
                  <span className="min-w-0">
                    <span className="block truncate text-sm">
                      {healthLabel(locale, "sources", source.code)}
                    </span>
                    {source.issue_count > 0 ? (
                      <span className="text-muted-foreground block text-xs tabular-nums">
                        {t("dashboard.health.issues", {
                          count: source.issue_count,
                        })}
                      </span>
                    ) : null}
                  </span>
                  <HealthStatusChip status={source.status} />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </OverviewPanel>
  );
}

/** Platform file store usage against its quota. */
export function StoragePanel({
  query,
}: {
  query: UseQueryResult<StorageUsage>;
}) {
  const { t, locale } = useLocale();
  const data = query.data;
  const quota = data?.quota_bytes ?? 0;
  const used = data?.used_bytes ?? 0;
  const pct = quota > 0 ? Math.min(100, Math.round((used / quota) * 100)) : 0;

  return (
    <OverviewPanel
      title={t("layout.nav_storage")}
      link={{
        href: routes.platform.settings.storage,
        label: t("dashboard.storage.open"),
      }}
    >
      {query.isError && !data ? (
        <PanelError
          description={t("dashboard.storage.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : !data ? (
        <div className="space-y-2" aria-busy>
          <Skeleton className="h-6 w-28" />
          <Skeleton className="h-2 w-full rounded-full" />
          <Skeleton className="h-3 w-40" />
        </div>
      ) : data.total_files === 0 && used === 0 ? (
        <PanelNote
          icon={<HardDrive className="size-5" />}
          title={t("dashboard.storage.empty")}
        />
      ) : (
        <div className="space-y-2.5">
          <p className="flex items-baseline gap-1.5">
            <span className="text-xl font-semibold tracking-tight tabular-nums">
              {formatBytes(used, locale)}
            </span>
            {quota > 0 ? (
              <span className="text-muted-foreground text-sm">
                {t("dashboard.storage.of_quota", {
                  quota: formatBytes(quota, locale),
                })}
              </span>
            ) : null}
          </p>
          {quota > 0 ? (
            <Progress
              value={pct}
              aria-label={t("layout.nav_storage")}
              className={pct >= 90 ? "bg-destructive/20" : undefined}
            />
          ) : null}
          <p className="text-muted-foreground text-xs tabular-nums">
            {t("dashboard.storage.files", {
              files: data.total_files.toLocaleString(locale),
              folders: data.total_folders.toLocaleString(locale),
            })}
          </p>
        </div>
      )}
    </OverviewPanel>
  );
}
