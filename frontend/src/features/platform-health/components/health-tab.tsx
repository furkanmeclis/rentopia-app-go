"use client";

import { AlertTriangle, BellRing } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { HealthStatusChip } from "@/features/platform-health/components/health-chips";
import { QueuesCard } from "@/features/platform-health/components/queues-card";
import { useHealthSummary } from "@/features/platform-health/hooks/use-platform-health";
import { healthHint, healthLabel } from "@/features/platform-health/lib/labels";
import type {
  HealthMetric,
  HealthSource,
} from "@/features/platform-health/types";
import { cn } from "@/lib/utils";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function MetricTile({ metric }: { metric: HealthMetric }) {
  const { locale } = useLocale();
  const problem =
    metric.value > 0 &&
    (metric.severity === "critical" || metric.severity === "warning");
  return (
    <div
      className={cn(
        "rounded-md border px-3 py-2",
        problem && metric.severity === "critical"
          ? "border-destructive/40 bg-destructive/5"
          : problem
            ? "border-amber-500/40 bg-amber-500/5"
            : "border-border",
      )}
    >
      <div className="text-muted-foreground text-xs">
        {healthLabel(locale, "metrics", metric.key)}
      </div>
      <div className="text-lg font-semibold tabular-nums">
        {metric.value.toLocaleString(locale)}
      </div>
    </div>
  );
}

function SourceCard({
  source,
  onViewIssues,
}: {
  source: HealthSource;
  onViewIssues: (source: string) => void;
}) {
  const { t, locale } = useLocale();
  const hint = healthHint(locale, source.code);
  return (
    <EntitySectionCard
      title={healthLabel(locale, "sources", source.code)}
      actions={<HealthStatusChip status={source.status} />}
    >
      <div className="space-y-3">
        {hint ? <p className="text-muted-foreground text-sm">{hint}</p> : null}
        {source.error ? (
          <p className="text-destructive flex items-center gap-2 text-sm">
            <AlertTriangle className="size-4" />
            {t("platform_health.source_error")}
          </p>
        ) : null}
        {source.metrics.length > 0 ? (
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {source.metrics.map((m) => (
              <MetricTile key={m.key} metric={m} />
            ))}
          </div>
        ) : null}
        {source.issue_count > 0 ? (
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-muted-foreground text-sm">
              {[
                source.critical > 0
                  ? t("platform_health.overall_critical", {
                      count: source.critical,
                    })
                  : null,
                source.warning > 0
                  ? t("platform_health.overall_warning", {
                      count: source.warning,
                    })
                  : null,
              ]
                .filter(Boolean)
                .join(" · ")}
              {source.truncated ? ` · ${t("platform_health.truncated")}` : ""}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onViewIssues(source.code)}
            >
              {t("platform_health.view_issues")}
            </Button>
          </div>
        ) : null}
      </div>
    </EntitySectionCard>
  );
}

/**
 * Connection health: the overall status, one card per registered health
 * source with its metrics, and the failed-work alert state.
 */
export function HealthTab({
  onViewIssues,
}: {
  onViewIssues: (source: string) => void;
}) {
  const { t, locale } = useLocale();
  const query = useHealthSummary();
  const data = query.data;

  if (query.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("platform_health.load_failed")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (query.isLoading || !data) {
    return (
      <div className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-40 rounded-lg" />
        <Skeleton className="h-40 rounded-lg" />
      </div>
    );
  }

  const alertsBySource = new Map(data.alerts.map((a) => [a.source, a]));
  const lastAlert = data.alerts
    .filter((a) => a.last_alert_at)
    .sort((a, b) =>
      (b.last_alert_at ?? "").localeCompare(a.last_alert_at ?? ""),
    )[0];

  return (
    <div className="space-y-4">
      <EntitySectionCard
        title={t("platform_health.overall")}
        actions={
          <>
            <HealthStatusChip status={data.status} />
            <Button
              variant="outline"
              size="sm"
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
            >
              {t("platform_health.refresh")}
            </Button>
          </>
        }
      >
        <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span>
            {data.critical + data.warning === 0
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
                  .join(" · ")}
          </span>
          <span className="text-muted-foreground">
            {t("platform_health.checked_at", {
              time: datetime(data.checked_at, "dd.MM.yyyy HH:mm:ss", locale),
            })}
          </span>
        </div>
      </EntitySectionCard>

      <div className="grid gap-4 lg:grid-cols-2">
        {data.sources.map((source) => (
          <SourceCard
            key={source.code}
            source={source}
            onViewIssues={onViewIssues}
          />
        ))}
      </div>

      <QueuesCard
        queues={data.queues}
        queuesError={data.queues_error}
        jobs={data.integration_jobs}
      />

      <EntitySectionCard title={t("platform_health.alerts.title")}>
        <div className="space-y-2 text-sm">
          <p className="text-muted-foreground flex items-start gap-2">
            <BellRing className="mt-0.5 size-4 shrink-0" />
            {t("platform_health.alerts.description")}
          </p>
          <p>
            {lastAlert?.last_alert_at
              ? t("platform_health.alerts.last", {
                  time: relativeDatetime(lastAlert.last_alert_at, locale),
                  count: lastAlert.last_alert_count,
                })
              : t("platform_health.alerts.never")}
          </p>
          {data.sources.some(
            (s) => alertsBySource.get(s.code)?.last_alert_at,
          ) ? (
            <ul className="text-muted-foreground space-y-1">
              {data.sources.map((s) => {
                const a = alertsBySource.get(s.code);
                if (!a?.last_alert_at) return null;
                return (
                  <li key={s.code}>
                    {healthLabel(locale, "sources", s.code)}:{" "}
                    {t("platform_health.alerts.last", {
                      time: relativeDatetime(a.last_alert_at, locale),
                      count: a.last_alert_count,
                    })}
                  </li>
                );
              })}
            </ul>
          ) : null}
        </div>
      </EntitySectionCard>
    </div>
  );
}
