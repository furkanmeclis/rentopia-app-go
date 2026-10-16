"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { AppChart, type ChartConfig } from "@/components/charts";
import {
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Amount } from "@/features/finance/components/finance-cells";
import { amountText } from "@/features/finance/lib/format";
import { ResourceIOToolbar } from "@/features/io";
import { monthLabel } from "@/features/profitability/lib/period";
import {
  MethodNotes,
  OptionSelect,
  ReportError,
  ReportFilters,
  ReportPage,
  useReportFilters,
} from "@/features/reports/components/report-shell";
import { ReportKpis } from "@/features/reports/components/report-kpis";
import {
  DATE_BASES,
  KNOWN_CHANNELS,
  OTHER_COLOR,
  SERIES_COLORS,
} from "@/features/reports/constants";
import { useReport } from "@/features/reports/hooks/use-reports";
import {
  channelLabel,
  compactText,
  num,
  numberText,
  percentText,
} from "@/features/reports/lib/format";
import type { ChannelDateBasis, ChannelRow } from "@/features/reports/types";
import { useLocale } from "@/providers/locale-provider";

/** Fixed colour of a channel: known channels keep their slot. */
function channelColor(code: string, extras: string[]) {
  const known = (KNOWN_CHANNELS as readonly string[]).indexOf(code);
  if (known >= 0) return SERIES_COLORS[known];
  const extra = extras.indexOf(code);
  return extra === 0 ? SERIES_COLORS[6] : OTHER_COLOR;
}

/** Series key of a channel in the chart rows (codes may hold any char). */
const seriesKey = (code: string) => `ch_${code.replace(/[^a-z0-9_]/gi, "_")}`;

const COUNT_KEYS = [
  "reservations",
  "confirmed",
  "pending",
  "cancelled",
  "expired",
  "no_show",
] as const;

/** Raporlar → Kanal performansı (TEC-738). */
export function ChannelsReportPage() {
  const { t, locale } = useLocale();
  const filters = useReportFilters();
  const [basis, setBasis] = useState<ChannelDateBasis>("booking");
  const params = { ...filters.params, date_basis: basis };
  const query = useReport("channels", params);
  const report = query.data;
  const totals = report?.totals;

  const columns = useMemo(
    () =>
      [
        createColumn<ChannelRow>({
          id: "channel",
          accessorFn: (row) => channelLabel(row.channel, t),
          labelKey: "reports.fields.channel",
          gridPrimary: true,
          enableSorting: true,
        }),
        ...COUNT_KEYS.map((key) =>
          createColumn<ChannelRow>({
            id: key,
            accessorFn: (row) => row[key],
            labelKey: `reports.fields.${key}`,
            enableSorting: true,
            defaultHidden: key === "expired" || key === "no_show",
          }),
        ),
        createColumn<ChannelRow>({
          id: "cancellation_rate",
          accessorFn: (row) => row.cancellation_rate ?? -1,
          labelKey: "reports.fields.cancellation_rate",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.cancellation_rate, locale)}
            </span>
          ),
        }),
        ...(["revenue", "commission", "net_revenue"] as const).map((key) =>
          createColumn<ChannelRow>({
            id: key,
            accessorFn: (row) => num(row[key]),
            labelKey: `reports.fields.${key}`,
            enableSorting: true,
            cell: ({ row }) => (
              <Amount
                value={row.original[key]}
                className={key === "net_revenue" ? "font-medium" : undefined}
              />
            ),
          }),
        ),
        createColumn<ChannelRow>({
          id: "commission_rate",
          accessorFn: (row) => row.commission_rate ?? -1,
          labelKey: "reports.fields.commission_rate",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.commission_rate, locale)}
            </span>
          ),
        }),
        createColumn<ChannelRow>({
          id: "average_revenue",
          accessorFn: (row) => num(row.average_revenue),
          labelKey: "reports.fields.average_revenue",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => <Amount value={row.original.average_revenue} />,
        }),
        createColumn<ChannelRow>({
          id: "average_days",
          accessorFn: (row) => row.average_days,
          labelKey: "reports.fields.average_days",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {numberText(row.original.average_days, locale, 2)}
            </span>
          ),
        }),
        createColumn<ChannelRow>({
          id: "channel_collected",
          accessorFn: (row) => num(row.channel_collected),
          labelKey: "reports.fields.channel_collected",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => <Amount value={row.original.channel_collected} />,
        }),
        createColumn<ChannelRow>({
          id: "revenue_share",
          accessorFn: (row) => row.revenue_share ?? -1,
          labelKey: "reports.fields.revenue_share",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.revenue_share, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<ChannelRow, unknown>[],
    [t, locale],
  );

  // Monthly revenue stacked by channel; channels in a fixed colour order.
  const { trend, trendConfig, series } = useMemo(() => {
    const codes = Array.from(
      new Set((report?.rows ?? []).map((r) => r.channel)),
    );
    const extras = codes.filter(
      (c) => !(KNOWN_CHANNELS as readonly string[]).includes(c),
    );
    const config: ChartConfig = {};
    for (const c of codes) {
      config[seriesKey(c)] = {
        label: channelLabel(c, t),
        theme: channelColor(c, extras),
      };
    }
    const byMonth = new Map<string, Record<string, number | string>>();
    for (const p of report?.trend ?? []) {
      const row = byMonth.get(p.month) ?? { label: p.month };
      row[seriesKey(p.channel)] = num(p.revenue);
      byMonth.set(p.month, row);
    }
    return {
      trend: Array.from(byMonth.values()),
      trendConfig: config,
      series: codes.map((c) => ({ key: seriesKey(c), stackId: "revenue" })),
    };
  }, [report, t]);

  const cancelData = (report?.rows ?? []).map((r) => ({
    label: channelLabel(r.channel, t),
    rate: r.cancellation_rate ?? 0,
  }));

  return (
    <ReportPage
      titleKey="reports.channels.title"
      descriptionKey="reports.channels.description"
    >
      <ReportFilters
        period={filters.period}
        onPeriodChange={filters.setPeriod}
        branch={filters.branch}
        onBranchChange={filters.setBranch}
      >
        <OptionSelect
          value={basis}
          options={DATE_BASES}
          labelKey={(b) => `reports.date_basis.${b}`}
          ariaLabel={t("reports.filters.date_basis")}
          onChange={setBasis}
        />
      </ReportFilters>

      {query.isError ? (
        <ReportError onRetry={() => void query.refetch()} />
      ) : (
        <>
          <ReportKpis
            loading={query.isLoading || !totals}
            count={5}
            items={
              totals
                ? [
                    {
                      key: "reservations",
                      title: t("reports.fields.reservations"),
                      value: totals.reservations,
                      hint: t("reports.kpi.confirmed", {
                        count: totals.confirmed,
                      }),
                    },
                    {
                      key: "revenue",
                      title: t("reports.fields.revenue"),
                      value: amountText(totals.revenue, locale),
                    },
                    {
                      key: "commission",
                      title: t("reports.fields.commission"),
                      value: amountText(totals.commission, locale),
                      hint: t("reports.kpi.rate", {
                        value: percentText(totals.commission_rate, locale),
                      }),
                    },
                    {
                      key: "net_revenue",
                      title: t("reports.fields.net_revenue"),
                      value: amountText(totals.net_revenue, locale),
                    },
                    {
                      key: "cancellation_rate",
                      title: t("reports.fields.cancellation_rate"),
                      value: percentText(totals.cancellation_rate, locale),
                      hint: t("reports.kpi.cancelled", {
                        count: totals.cancelled,
                      }),
                    },
                  ]
                : []
            }
          />
          <div className="grid gap-6 xl:grid-cols-2">
            <AppChart
              type="bar"
              stacked
              showLegend
              title={t("reports.channels.trend_title")}
              description={t("reports.channels.trend_description")}
              data={trend}
              config={trendConfig}
              series={series}
              valueFormatter={(v) => compactText(v, locale)}
              categoryFormatter={(v) => monthLabel(v, locale)}
              height={260}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
            <AppChart
              type="bar-horizontal"
              title={t("reports.channels.cancel_title")}
              data={cancelData}
              config={{
                rate: {
                  label: t("reports.fields.cancellation_rate"),
                  theme: SERIES_COLORS[1],
                },
              }}
              valueFormatter={(v) => percentText(v, locale)}
              height={Math.max(200, cancelData.length * 36 + 40)}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
          </div>
          <EntitySectionCard
            title={t("reports.channels.table_title")}
            badge={report?.rows.length}
          >
            <EntityTable
              columns={columns}
              data={report?.rows ?? []}
              getRowId={(row) => row.channel}
              isLoading={query.isLoading}
              emptyTitle={t("reports.empty")}
              emptyDescription={t("reports.empty_description")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                persistKey: "tenant-report-channels-v1",
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
              }}
              toolbarExtra={
                <>
                  <ResourceIOToolbar
                    resource="tenant.report_channels"
                    query={params}
                    capabilities={{ export: true }}
                  />
                  <EntityToolbar
                    onRefresh={() => void query.refetch()}
                    refreshDisabled={query.isFetching}
                  />
                </>
              }
            />
          </EntitySectionCard>
          <MethodNotes
            method={report?.method}
            from={report?.from}
            to={report?.to}
            items={[
              {
                label: t("reports.filters.date_basis"),
                value: t(`reports.date_basis.${basis}`),
              },
            ]}
            notes={[t("reports.notes.channels"), t("reports.notes.source")]}
          />
        </>
      )}
    </ReportPage>
  );
}
