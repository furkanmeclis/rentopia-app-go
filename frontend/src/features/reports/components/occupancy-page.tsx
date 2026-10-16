"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { AppChart } from "@/components/charts";
import {
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { Amount } from "@/features/finance/components/finance-cells";
import { amountText } from "@/features/finance/lib/format";
import { useGroupsList } from "@/features/fleet/hooks/use-fleet";
import { ResourceIOToolbar } from "@/features/io";
import { monthLabel } from "@/features/profitability/lib/period";
import {
  ALL,
  MethodNotes,
  OptionSelect,
  ReportError,
  ReportFilters,
  ReportPage,
  useReportFilters,
} from "@/features/reports/components/report-shell";
import { ReportKpis } from "@/features/reports/components/report-kpis";
import { OCCUPANCY_GROUPS, SERIES_COLORS } from "@/features/reports/constants";
import { useReport } from "@/features/reports/hooks/use-reports";
import { numberText, percentText } from "@/features/reports/lib/format";
import type {
  OccupancyGroup,
  OccupancyReport,
  OccupancyRow,
} from "@/features/reports/types";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function groupLabel(
  row: OccupancyRow,
  report: OccupancyReport | undefined,
  t: (key: string) => string,
  locale: string,
): string {
  if (row.key === "ungrouped") return t("reports.occupancy.ungrouped");
  if (report?.group_by === "month") return monthLabel(row.key, locale);
  if (report?.group_by === "rental_group" && row.group.code) {
    return `${row.group.code} · ${row.group.name}`;
  }
  return row.group.name;
}

/** Raporlar → Doluluk (TEC-738). */
export function OccupancyReportPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const filters = useReportFilters();
  const [groupBy, setGroupBy] = useState<OccupancyGroup>("rental_group");
  const [group, setGroup] = useState(ALL);
  const canGroups = can(permissions.tenant.fleet.read);
  const groups = useGroupsList(
    { limit: 100, offset: 0, sort: "code" },
    canGroups,
  );
  const params = {
    ...filters.params,
    group_by: groupBy,
    rental_group: group === ALL ? undefined : group,
  };
  const query = useReport("occupancy", params);
  const report = query.data;
  const totals = report?.totals;

  const columns = useMemo(
    () =>
      [
        createColumn<OccupancyRow>({
          id: "group",
          accessorFn: (row) => groupLabel(row, report, t, locale),
          labelKey: "reports.fields.group",
          gridPrimary: true,
          enableSorting: true,
        }),
        createColumn<OccupancyRow>({
          id: "vehicles",
          accessorFn: (row) => row.vehicles,
          labelKey: "reports.fields.vehicles",
          enableSorting: true,
        }),
        createColumn<OccupancyRow>({
          id: "rental_days",
          accessorFn: (row) => row.rental_days,
          labelKey: "reports.fields.rental_days",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {numberText(row.original.rental_days, locale)}
            </span>
          ),
        }),
        createColumn<OccupancyRow>({
          id: "available_days",
          accessorFn: (row) => row.available_days,
          labelKey: "reports.fields.available_days",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {numberText(row.original.available_days, locale)}
            </span>
          ),
        }),
        createColumn<OccupancyRow>({
          id: "occupancy",
          accessorFn: (row) => row.occupancy ?? -1,
          labelKey: "reports.fields.occupancy",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {percentText(row.original.occupancy, locale)}
            </span>
          ),
        }),
        createColumn<OccupancyRow>({
          id: "rental_count",
          accessorFn: (row) => row.rental_count,
          labelKey: "reports.fields.rental_count",
          enableSorting: true,
          defaultHidden: true,
        }),
        createColumn<OccupancyRow>({
          id: "gross_income",
          accessorFn: (row) => Number(row.gross_income),
          labelKey: "reports.fields.gross_income",
          enableSorting: true,
          cell: ({ row }) => <Amount value={row.original.gross_income} />,
        }),
        createColumn<OccupancyRow>({
          id: "income_per_available_day",
          accessorFn: (row) => Number(row.income_per_available_day ?? 0),
          labelKey: "reports.fields.income_per_available_day",
          enableSorting: true,
          cell: ({ row }) => (
            <Amount value={row.original.income_per_available_day} />
          ),
        }),
      ] as ColumnDef<OccupancyRow, unknown>[],
    [report, t, locale],
  );

  const trendData = (report?.trend ?? []).map((p) => ({
    label: p.month,
    occupancy: p.occupancy ?? 0,
  }));
  const rowData = (report?.rows ?? []).map((r) => ({
    label: groupLabel(r, report, t, locale),
    occupancy: r.occupancy ?? 0,
  }));
  const occupancyConfig = {
    occupancy: {
      label: t("reports.fields.occupancy"),
      theme: SERIES_COLORS[0],
    },
  };

  return (
    <ReportPage
      titleKey="reports.occupancy.title"
      descriptionKey="reports.occupancy.description"
    >
      <ReportFilters
        period={filters.period}
        onPeriodChange={filters.setPeriod}
        branch={filters.branch}
        onBranchChange={filters.setBranch}
      >
        <OptionSelect
          value={groupBy}
          options={OCCUPANCY_GROUPS}
          labelKey={(g) => `reports.group.${g}`}
          ariaLabel={t("reports.filters.group_by")}
          onChange={setGroupBy}
        />
        {canGroups ? (
          <Select value={group} onValueChange={setGroup}>
            <SelectTrigger
              className="w-full sm:w-48"
              aria-label={t("reports.filters.rental_group")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>
                {t("reports.filters.all_groups")}
              </SelectItem>
              {(groups.data?.items ?? []).map((g) => (
                <SelectItem key={g.uuid} value={g.uuid}>
                  {g.code} · {g.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
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
                      key: "occupancy",
                      title: t("reports.fields.occupancy"),
                      value: percentText(totals.occupancy, locale),
                    },
                    {
                      key: "rental_days",
                      title: t("reports.fields.rental_days"),
                      value: numberText(totals.rental_days, locale),
                      hint: t("reports.kpi.rentals", {
                        count: totals.rental_count,
                      }),
                    },
                    {
                      key: "available_days",
                      title: t("reports.fields.available_days"),
                      value: numberText(totals.available_days, locale),
                    },
                    {
                      key: "vehicles",
                      title: t("reports.fields.vehicles"),
                      value: totals.vehicles,
                    },
                    {
                      key: "revpad",
                      title: t("reports.fields.income_per_available_day"),
                      value: amountText(
                        totals.income_per_available_day,
                        locale,
                      ),
                      hint: t("reports.kpi.gross_income", {
                        amount: amountText(totals.gross_income, locale),
                      }),
                    },
                  ]
                : []
            }
          />
          <div className="grid gap-6 xl:grid-cols-2">
            <AppChart
              type="line"
              title={t("reports.occupancy.trend_title")}
              description={t("reports.occupancy.trend_description")}
              data={trendData}
              config={occupancyConfig}
              valueFormatter={(v) => percentText(v, locale)}
              categoryFormatter={(v) => monthLabel(v, locale)}
              height={260}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
            <AppChart
              type="bar-horizontal"
              title={t("reports.occupancy.groups_title")}
              description={t(`reports.group.${groupBy}`)}
              data={rowData}
              config={occupancyConfig}
              valueFormatter={(v) => percentText(v, locale)}
              height={Math.max(200, rowData.length * 32 + 40)}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
          </div>
          <EntitySectionCard
            title={t("reports.occupancy.table_title")}
            badge={report?.rows.length}
          >
            <EntityTable
              columns={columns}
              data={report?.rows ?? []}
              getRowId={(row) => row.key}
              isLoading={query.isLoading}
              emptyTitle={t("reports.empty")}
              emptyDescription={t("reports.empty_description")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                persistKey: "tenant-report-occupancy-v1",
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
              }}
              toolbarExtra={
                <>
                  <ResourceIOToolbar
                    resource="tenant.report_occupancy"
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
                label: t("reports.filters.group_by"),
                value: t(`reports.group.${groupBy}`),
              },
            ]}
            notes={[
              t("reports.notes.occupancy"),
              ...(report && report.multi_group_vehicles > 0
                ? [
                    t("reports.notes.multi_group", {
                      count: report.multi_group_vehicles,
                    }),
                  ]
                : []),
            ]}
          />
        </>
      )}
    </ReportPage>
  );
}
