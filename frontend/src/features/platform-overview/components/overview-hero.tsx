"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { Plus, RefreshCw } from "lucide-react";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";
import { Bar, BarChart, Cell, LabelList, XAxis } from "recharts";

import { Button } from "@/components/ui/button";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { formatBytes, monthLabel } from "@/features/platform-health/lib/labels";
import type { UsageSummary } from "@/features/platform-health/types";
import type { OverviewScopes } from "@/features/platform-overview/hooks/use-overview-query";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type HeroProps = {
  scopes: OverviewScopes;
  name: string;
  months: string[];
  trend: UseQueryResult<UsageSummary>[];
  usersTotal: UseQueryResult<number>;
  organizationsTotal: number | undefined;
  organizationsLoading: boolean;
  attention: { total: number; isLoading: boolean; hasFailures: boolean };
  refreshing: boolean;
  onRefresh: () => void;
};

type Stat = {
  key: string;
  label: string;
  value: ReactNode;
  hint?: string;
  href?: string;
};

function greetingKey(hour: number) {
  if (hour >= 5 && hour < 12) return "dashboard.greeting.morning";
  if (hour >= 12 && hour < 18) return "dashboard.greeting.day";
  if (hour >= 18 && hour < 23) return "dashboard.greeting.evening";
  return "dashboard.greeting.night";
}

function shortMonth(month: string, locale: string) {
  const [y, m] = month.split("-").map(Number);
  return new Date(y, m - 1, 1).toLocaleDateString(locale, { month: "short" });
}

/**
 * The page's one loud element: an ink panel (the sidebar's colour) with
 * the greeting, what needs attention, the platform's live totals and the
 * reservation trend. Rendered in the dark token set in both colour modes.
 */
export function OverviewHero({
  scopes,
  name,
  months,
  trend,
  usersTotal,
  organizationsTotal,
  organizationsLoading,
  attention,
  refreshing,
  onRefresh,
}: HeroProps) {
  const { t, locale } = useLocale();
  const [opened] = useState(() => new Date());
  const intlLocale = locale === "tr" ? "tr-TR" : "en-US";
  const number = (value: number) => value.toLocaleString(intlLocale);

  const current = trend[0];
  const previous = trend[1];
  const summary = current?.data;

  const stats: Stat[] = [];
  if (scopes.health && summary) {
    stats.push(
      {
        key: "organizations",
        label: t("dashboard.pulse.organizations"),
        value: number(summary.organizations),
        hint: t("dashboard.pulse.organizations_hint", {
          active: number(summary.active_organizations),
        }),
        href: scopes.organizations
          ? routes.platform.organizations.root
          : undefined,
      },
      {
        key: "vehicles",
        label: t("dashboard.pulse.active_vehicles"),
        value: number(summary.active_vehicles),
        hint: t("dashboard.pulse.active_vehicles_hint"),
        href: routes.platform.health,
      },
      {
        key: "reservations",
        label: t("dashboard.pulse.reservations", {
          month: monthLabel(summary.month, locale),
        }),
        value: number(summary.reservations_month),
        hint: previous?.data
          ? t("dashboard.pulse.reservations_prev", {
              month: monthLabel(previous.data.month, locale),
              count: number(previous.data.reservations_month),
            })
          : undefined,
        href: routes.platform.health,
      },
      {
        key: "members",
        label: t("dashboard.pulse.members"),
        value: number(summary.members),
        hint: t("dashboard.pulse.members_hint"),
      },
      {
        key: "storage",
        label: t("dashboard.pulse.storage"),
        value: formatBytes(summary.storage_bytes, locale),
        hint: t("dashboard.pulse.storage_hint"),
      },
    );
  } else if (
    !scopes.health &&
    scopes.organizations &&
    organizationsTotal != null
  ) {
    stats.push({
      key: "organizations",
      label: t("dashboard.pulse.organizations"),
      value: number(organizationsTotal),
      href: routes.platform.organizations.root,
    });
  }
  if (scopes.users && usersTotal.data != null) {
    stats.push({
      key: "users",
      label: t("dashboard.pulse.platform_users"),
      value: number(usersTotal.data),
      hint: t("dashboard.pulse.platform_users_hint"),
      href: routes.platform.users.root,
    });
  }

  const statsLoading =
    (scopes.health && current?.isLoading) ||
    (!scopes.health && scopes.organizations && organizationsLoading) ||
    (scopes.users && usersTotal.isLoading);
  const statsError = scopes.health && current?.isError;
  const showStats = scopes.health || scopes.organizations || scopes.users;

  let summaryLine: string;
  if (attention.isLoading) summaryLine = t("dashboard.summary.checking");
  else if (attention.total > 0)
    summaryLine =
      attention.total === 1
        ? t("dashboard.summary.attention_one")
        : t("dashboard.summary.attention_other", { count: attention.total });
  else if (attention.hasFailures) summaryLine = t("dashboard.summary.partial");
  else summaryLine = t("dashboard.summary.clear");

  return (
    <section
      aria-labelledby="overview-greeting"
      className="sidebar-ink dark bg-sidebar text-foreground relative overflow-hidden rounded-[1.5rem] border border-white/5 shadow-[0_24px_48px_-24px_rgb(20_21_39/0.55)]"
    >
      <div className="relative space-y-6 p-5 sm:p-7">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0 space-y-1.5">
            <p className="text-muted-foreground text-sm">
              {datetime(opened, "d MMMM yyyy, EEEE", locale)}
            </p>
            <h1
              id="overview-greeting"
              className="font-display text-2xl font-semibold tracking-tight text-balance sm:text-[1.75rem]"
            >
              {name
                ? t(greetingKey(opened.getHours()), { name })
                : t(`${greetingKey(opened.getHours())}_plain`)}
            </h1>
            <p
              className={cn(
                "flex items-center gap-2 text-sm",
                attention.total > 0
                  ? "text-amber-200"
                  : "text-muted-foreground",
              )}
              aria-live="polite"
            >
              <span
                aria-hidden
                className={cn(
                  "size-2 shrink-0 rounded-full",
                  attention.isLoading
                    ? "bg-muted-foreground/60 animate-pulse motion-reduce:animate-none"
                    : attention.total > 0 || attention.hasFailures
                      ? "bg-amber-400"
                      : "bg-emerald-400",
                )}
              />
              {summaryLine}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {scopes.createOrganization ? (
              <Button asChild>
                <Link href={routes.platform.organizations.create}>
                  <Plus className="size-4" />
                  {t("dashboard.actions.create_organization")}
                </Link>
              </Button>
            ) : null}
            <Button
              type="button"
              variant="outline"
              size="icon"
              onClick={onRefresh}
              disabled={refreshing}
              aria-label={t("dashboard.actions.refresh")}
              title={t("dashboard.actions.refresh")}
            >
              <RefreshCw
                className={cn(
                  "size-4",
                  refreshing && "animate-spin motion-reduce:animate-none",
                )}
              />
            </Button>
          </div>
        </div>

        {showStats ? (
          <div
            className={cn(
              "grid gap-6",
              scopes.health && "lg:grid-cols-[minmax(0,1fr)_minmax(0,18rem)]",
            )}
          >
            {statsError ? (
              <div className="flex flex-wrap items-center gap-3 border-t border-white/10 pt-4 text-sm">
                <span className="text-muted-foreground">
                  {t("dashboard.pulse.load_failed")}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void current?.refetch()}
                >
                  {t("common.retry")}
                </Button>
              </div>
            ) : statsLoading && stats.length === 0 ? (
              <StatsSkeleton />
            ) : (
              <dl className="grid grid-cols-2 gap-x-5 gap-y-5 sm:grid-cols-3 xl:grid-cols-3 2xl:grid-cols-6">
                {stats.map((stat) => (
                  <StatCell key={stat.key} stat={stat} />
                ))}
              </dl>
            )}
            {scopes.health ? (
              <ReservationTrend months={months} trend={trend} />
            ) : null}
          </div>
        ) : null}
      </div>
    </section>
  );
}

function StatCell({ stat }: { stat: Stat }) {
  const body = (
    <>
      <dt className="text-muted-foreground text-xs font-medium">
        {stat.label}
      </dt>
      <dd className="mt-1.5 text-2xl font-semibold tracking-tight tabular-nums sm:text-[1.75rem]">
        {stat.value}
      </dd>
      {stat.hint ? (
        <dd className="text-muted-foreground mt-0.5 truncate text-xs">
          {stat.hint}
        </dd>
      ) : null}
    </>
  );
  return (
    <div className="min-w-0 border-t border-white/10 pt-3">
      {stat.href ? (
        <Link
          href={stat.href}
          className="focus-visible:ring-ring -m-1 block rounded-lg p-1 transition-colors outline-none hover:bg-white/5 focus-visible:ring-2"
        >
          {body}
        </Link>
      ) : (
        body
      )}
    </div>
  );
}

function StatsSkeleton() {
  return (
    <div className="grid grid-cols-2 gap-5 sm:grid-cols-3" aria-busy>
      {[0, 1, 2, 3, 4, 5].map((i) => (
        <div key={i} className="space-y-2 border-t border-white/10 pt-3">
          <Skeleton className="h-3 w-20 bg-white/10" />
          <Skeleton className="h-7 w-16 bg-white/10" />
          <Skeleton className="h-3 w-24 bg-white/10" />
        </div>
      ))}
    </div>
  );
}

/** Reservations created per month, the last six months (oldest left). */
function ReservationTrend({
  months,
  trend,
}: {
  months: string[];
  trend: UseQueryResult<UsageSummary>[];
}) {
  const { t, locale } = useLocale();
  const intlLocale = locale === "tr" ? "tr-TR" : "en-US";

  const data = useMemo(
    () =>
      months
        .map((month, i) => ({
          month,
          label: shortMonth(month, intlLocale),
          full: monthLabel(month, locale),
          value: trend[i]?.data?.reservations_month,
          current: i === 0,
        }))
        .reverse(),
    [months, trend, intlLocale, locale],
  );

  const loading = trend.some((q) => q.isLoading);
  const failed = trend.filter((q) => q.isError).length;
  const known = data.filter((d) => d.value != null);
  const allZero = known.length > 0 && known.every((d) => d.value === 0);

  const config: ChartConfig = {
    value: { label: t("dashboard.trend.series"), color: "var(--primary)" },
  };

  return (
    <figure className="min-w-0 border-t border-white/10 pt-3">
      <figcaption className="flex items-baseline justify-between gap-2">
        <span className="text-muted-foreground text-xs font-medium">
          {t("dashboard.trend.title")}
        </span>
        <span className="text-muted-foreground/80 text-xs">
          {t("dashboard.trend.range", { count: months.length })}
        </span>
      </figcaption>
      {loading && known.length === 0 ? (
        <Skeleton className="mt-3 h-[124px] w-full rounded-xl bg-white/10" />
      ) : known.length === 0 ? (
        <p className="text-muted-foreground flex h-[124px] items-center justify-center text-center text-sm">
          {t("dashboard.trend.failed")}
        </p>
      ) : (
        <div className="relative mt-3">
          {allZero ? (
            <p className="text-muted-foreground pointer-events-none absolute inset-x-0 top-6 z-10 text-center text-sm">
              {t("dashboard.trend.empty")}
            </p>
          ) : null}
          <ChartContainer
            config={config}
            className="aspect-auto h-[124px] w-full"
          >
            <BarChart
              data={data}
              margin={{ top: 18, right: 0, bottom: 0, left: 0 }}
              barCategoryGap="22%"
            >
              <XAxis
                dataKey="label"
                tickLine={false}
                axisLine={false}
                tickMargin={6}
                fontSize={11}
              />
              <ChartTooltip
                cursor={{ fill: "rgb(255 255 255 / 0.05)" }}
                content={
                  <ChartTooltipContent
                    hideIndicator
                    labelFormatter={(_, payload) =>
                      (payload?.[0]?.payload as { full?: string } | undefined)
                        ?.full ?? ""
                    }
                    formatter={(value) =>
                      Number(value).toLocaleString(intlLocale)
                    }
                  />
                }
              />
              <Bar
                dataKey="value"
                radius={[4, 4, 0, 0]}
                maxBarSize={28}
                isAnimationActive={false}
              >
                {data.map((d) => (
                  <Cell
                    key={d.month}
                    fill="var(--color-value)"
                    fillOpacity={d.current ? 1 : 0.4}
                  />
                ))}
                <LabelList
                  dataKey="value"
                  position="top"
                  className="fill-foreground"
                  fontSize={11}
                  content={(props) => {
                    const { x, y, width, value, index } = props as {
                      x?: number | string;
                      y?: number | string;
                      width?: number | string;
                      value?: number | string;
                      index?: number;
                    };
                    if (index !== data.length - 1 || value == null) return null;
                    return (
                      <text
                        x={Number(x) + Number(width) / 2}
                        y={Number(y) - 6}
                        textAnchor="middle"
                        className="fill-foreground"
                        fontSize={11}
                        fontWeight={600}
                      >
                        {Number(value).toLocaleString(intlLocale)}
                      </text>
                    );
                  }}
                />
              </Bar>
            </BarChart>
          </ChartContainer>
        </div>
      )}
      {failed > 0 && known.length > 0 ? (
        <p className="text-muted-foreground mt-1 text-xs">
          {t("dashboard.trend.partial")}
        </p>
      ) : null}
    </figure>
  );
}
