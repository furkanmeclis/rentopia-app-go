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
import { Amount } from "@/features/finance/components/finance-cells";
import { amountText } from "@/features/finance/lib/format";
import { ResourceIOToolbar } from "@/features/io";
import {
  ALLOCATIONS,
  ALLOCATION_LABEL_KEYS,
  BASES,
  BASIS_LABEL_KEYS,
} from "@/features/profitability/constants";
import type {
  ProfitabilityAllocation,
  ProfitabilityBasis,
} from "@/features/profitability/types";
import {
  MethodNotes,
  OptionSelect,
  ReportError,
  ReportFilters,
  ReportPage,
  useReportFilters,
} from "@/features/reports/components/report-shell";
import { ReportKpis } from "@/features/reports/components/report-kpis";
import { SERIES_COLORS } from "@/features/reports/constants";
import { useReport } from "@/features/reports/hooks/use-reports";
import {
  compactText,
  num,
  numberText,
  percentText,
} from "@/features/reports/lib/format";
import type { BranchRow } from "@/features/reports/types";
import { useLocale } from "@/providers/locale-provider";

const MONEY_KEYS = [
  "gross_income",
  "commissions",
  "direct_costs",
  "overhead",
  "contribution",
] as const;

/** Raporlar → Şube karşılaştırma (TEC-738). */
export function BranchesReportPage() {
  const { t, locale } = useLocale();
  const filters = useReportFilters();
  const [basis, setBasis] = useState<ProfitabilityBasis>("accrual");
  const [allocation, setAllocation] =
    useState<ProfitabilityAllocation>("rental_days");
  const params = { ...filters.params, basis, allocation };
  const query = useReport("branches", params);
  const report = query.data;
  const totals = report?.totals;

  const columns = useMemo(
    () =>
      [
        createColumn<BranchRow>({
          id: "branch",
          accessorFn: (row) => row.branch.name,
          labelKey: "reports.fields.branch",
          gridPrimary: true,
          enableSorting: true,
        }),
        createColumn<BranchRow>({
          id: "vehicles",
          accessorFn: (row) => row.vehicles,
          labelKey: "reports.fields.vehicles",
          enableSorting: true,
        }),
        createColumn<BranchRow>({
          id: "rental_count",
          accessorFn: (row) => row.rental_count,
          labelKey: "reports.fields.rental_count",
          enableSorting: true,
        }),
        createColumn<BranchRow>({
          id: "rental_days",
          accessorFn: (row) => row.rental_days,
          labelKey: "reports.fields.rental_days",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {numberText(row.original.rental_days, locale)}
            </span>
          ),
        }),
        createColumn<BranchRow>({
          id: "occupancy",
          accessorFn: (row) => row.occupancy ?? -1,
          labelKey: "reports.fields.occupancy",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.occupancy, locale)}
            </span>
          ),
        }),
        ...MONEY_KEYS.map((key) =>
          createColumn<BranchRow>({
            id: key,
            accessorFn: (row) => num(row[key]),
            labelKey: `reports.fields.${key}`,
            enableSorting: true,
            defaultHidden: key === "commissions",
            cell: ({ row }) => (
              <Amount
                value={row.original[key]}
                className={
                  key === "contribution"
                    ? num(row.original.contribution) < 0
                      ? "text-destructive font-semibold"
                      : "font-semibold"
                    : undefined
                }
              />
            ),
          }),
        ),
        createColumn<BranchRow>({
          id: "margin",
          accessorFn: (row) => row.margin ?? -Infinity,
          labelKey: "reports.fields.margin",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.margin, locale)}
            </span>
          ),
        }),
        createColumn<BranchRow>({
          id: "income_per_vehicle",
          accessorFn: (row) => num(row.income_per_vehicle),
          labelKey: "reports.fields.income_per_vehicle",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => <Amount value={row.original.income_per_vehicle} />,
        }),
        createColumn<BranchRow>({
          id: "contribution_share",
          accessorFn: (row) => row.contribution_share ?? -1,
          labelKey: "reports.fields.contribution_share",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.contribution_share, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<BranchRow, unknown>[],
    [locale],
  );

  const moneyData = (report?.rows ?? []).map((r) => ({
    label: r.branch.name,
    income: num(r.gross_income),
    contribution: num(r.contribution),
  }));
  const occupancyData = (report?.rows ?? []).map((r) => ({
    label: r.branch.name,
    occupancy: r.occupancy ?? 0,
  }));

  return (
    <ReportPage
      titleKey="reports.branches.title"
      descriptionKey="reports.branches.description"
    >
      <ReportFilters
        period={filters.period}
        onPeriodChange={filters.setPeriod}
        branch={filters.branch}
        onBranchChange={filters.setBranch}
      >
        <OptionSelect
          value={basis}
          options={BASES}
          labelKey={(b) => BASIS_LABEL_KEYS[b]}
          ariaLabel={t("reports.filters.basis")}
          onChange={setBasis}
          className="w-full sm:w-44"
        />
        <OptionSelect
          value={allocation}
          options={ALLOCATIONS}
          labelKey={(a) => ALLOCATION_LABEL_KEYS[a]}
          ariaLabel={t("reports.filters.allocation")}
          onChange={setAllocation}
          className="w-full sm:w-52"
        />
      </ReportFilters>

      {query.isError ? (
        <ReportError onRetry={() => void query.refetch()} />
      ) : (
        <>
          <ReportKpis
            loading={query.isLoading || !totals}
            count={4}
            items={
              totals
                ? [
                    {
                      key: "gross_income",
                      title: t("reports.fields.gross_income"),
                      value: amountText(totals.gross_income, locale),
                    },
                    {
                      key: "contribution",
                      title: t("reports.fields.contribution"),
                      value: (
                        <span
                          className={
                            num(totals.contribution) < 0
                              ? "text-destructive"
                              : undefined
                          }
                        >
                          {amountText(totals.contribution, locale)}
                        </span>
                      ),
                      hint: t("reports.kpi.margin", {
                        value: percentText(totals.margin, locale),
                      }),
                    },
                    {
                      key: "occupancy",
                      title: t("reports.fields.occupancy"),
                      value: percentText(totals.occupancy, locale),
                    },
                    {
                      key: "vehicles",
                      title: t("reports.fields.vehicles"),
                      value: totals.vehicles,
                      hint: t("reports.kpi.rentals", {
                        count: totals.rental_count,
                      }),
                    },
                  ]
                : []
            }
          />
          <div className="grid gap-6 xl:grid-cols-2">
            <AppChart
              type="bar"
              showLegend
              title={t("reports.branches.money_title")}
              data={moneyData}
              config={{
                income: {
                  label: t("reports.fields.gross_income"),
                  theme: SERIES_COLORS[0],
                },
                contribution: {
                  label: t("reports.fields.contribution"),
                  theme: SERIES_COLORS[2],
                },
              }}
              valueFormatter={(v) => compactText(v, locale)}
              height={260}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
            <AppChart
              type="bar-horizontal"
              title={t("reports.branches.occupancy_title")}
              data={occupancyData}
              config={{
                occupancy: {
                  label: t("reports.fields.occupancy"),
                  theme: SERIES_COLORS[0],
                },
              }}
              valueFormatter={(v) => percentText(v, locale)}
              height={Math.max(200, occupancyData.length * 36 + 40)}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
          </div>
          <EntitySectionCard
            title={t("reports.branches.table_title")}
            badge={report?.rows.length}
          >
            <EntityTable
              columns={columns}
              data={report?.rows ?? []}
              getRowId={(row) => row.branch.uuid ?? row.branch.code}
              isLoading={query.isLoading}
              emptyTitle={t("reports.empty")}
              emptyDescription={t("reports.empty_description")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              initialState={{ sorting: [{ id: "contribution", desc: true }] }}
              features={{
                persistKey: "tenant-report-branches-v1",
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
              }}
              toolbarExtra={
                <>
                  <ResourceIOToolbar
                    resource="tenant.report_branches"
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
            items={
              report
                ? [
                    {
                      label: t("reports.filters.basis"),
                      value: t(BASIS_LABEL_KEYS[report.method.basis]),
                    },
                    {
                      label: t("reports.filters.allocation"),
                      value: t(ALLOCATION_LABEL_KEYS[report.method.allocation]),
                    },
                  ]
                : []
            }
            notes={[
              t("reports.notes.branches"),
              ...(report && num(report.method.unallocated_overhead) !== 0
                ? [
                    t("reports.notes.unallocated", {
                      amount: amountText(
                        report.method.unallocated_overhead,
                        locale,
                      ),
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
