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
import { Badge } from "@/components/ui/badge";
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
  EXPENSE_COLOR,
  INCOME_EXPENSE_GROUPS,
  incomeExpenseChartConfig,
} from "@/features/reports/constants";
import { useReport } from "@/features/reports/hooks/use-reports";
import {
  compactText,
  incomeCategoryLabel,
  num,
  percentText,
} from "@/features/reports/lib/format";
import type {
  IncomeExpenseGroup,
  IncomeExpenseRow,
} from "@/features/reports/types";
import { useLocale } from "@/providers/locale-provider";

function rowLabel(
  row: IncomeExpenseRow,
  groupBy: IncomeExpenseGroup,
  t: (key: string) => string,
  locale: string,
): string {
  if (row.kind === "income" || row.kind === "pass_through") {
    return incomeCategoryLabel(row.code, t);
  }
  if (row.key === "head_office") return t("reports.income_expense.head_office");
  if (groupBy === "month") return monthLabel(row.key, locale);
  return row.name || row.code;
}

const MONEY_KEYS = [
  "income",
  "expense",
  "net",
  "pass_through",
  "capex",
] as const;

/** Raporlar → Gelir / gider (TEC-738). */
export function IncomeExpenseReportPage() {
  const { t, locale } = useLocale();
  const filters = useReportFilters();
  const [groupBy, setGroupBy] = useState<IncomeExpenseGroup>("category");
  const params = { ...filters.params, group_by: groupBy };
  const query = useReport("income-expense", params);
  const report = query.data;
  const totals = report?.totals;

  const columns = useMemo(
    () =>
      [
        createColumn<IncomeExpenseRow>({
          id: "name",
          accessorFn: (row) => rowLabel(row, groupBy, t, locale),
          labelKey: "reports.fields.item",
          gridPrimary: true,
          enableSorting: true,
        }),
        ...(groupBy === "category"
          ? [
              createColumn<IncomeExpenseRow>({
                id: "kind",
                accessorFn: (row) => row.kind,
                labelKey: "reports.fields.kind",
                enableSorting: true,
                cell: ({ row }) => (
                  <Badge
                    variant={
                      row.original.kind === "income" ? "secondary" : "outline"
                    }
                  >
                    {t(`reports.kind.${row.original.kind}`)}
                  </Badge>
                ),
              }),
            ]
          : []),
        ...MONEY_KEYS.map((key) =>
          createColumn<IncomeExpenseRow>({
            id: key,
            accessorFn: (row) => num(row[key]),
            labelKey: `reports.fields.${key}`,
            enableSorting: true,
            defaultHidden:
              (key === "net" && groupBy === "category") ||
              key === "pass_through" ||
              key === "capex",
            cell: ({ row }) => (
              <Amount
                value={row.original[key]}
                muted={num(row.original[key]) === 0}
                className={
                  key === "net" && num(row.original.net) < 0
                    ? "text-destructive font-medium"
                    : undefined
                }
              />
            ),
          }),
        ),
        createColumn<IncomeExpenseRow>({
          id: "share",
          accessorFn: (row) => row.share ?? -1,
          labelKey: "reports.fields.share",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.share, locale)}
            </span>
          ),
        }),
        ...(groupBy !== "category"
          ? [
              createColumn<IncomeExpenseRow>({
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
            ]
          : []),
      ] as ColumnDef<IncomeExpenseRow, unknown>[],
    [groupBy, t, locale],
  );

  const trend = (report?.trend ?? []).map((p) => ({
    label: p.month,
    income: num(p.income),
    expense: num(p.expense),
  }));
  const breakdown =
    groupBy === "category"
      ? (report?.rows ?? [])
          .filter((r) => r.kind === "expense")
          .map((r) => ({
            label: rowLabel(r, groupBy, t, locale),
            expense: num(r.expense),
          }))
      : (report?.rows ?? []).map((r) => ({
          label: rowLabel(r, groupBy, t, locale),
          income: num(r.income),
          expense: num(r.expense),
        }));

  return (
    <ReportPage
      titleKey="reports.income_expense.title"
      descriptionKey="reports.income_expense.description"
    >
      <ReportFilters
        period={filters.period}
        onPeriodChange={filters.setPeriod}
        branch={filters.branch}
        onBranchChange={filters.setBranch}
      >
        <OptionSelect
          value={groupBy}
          options={INCOME_EXPENSE_GROUPS}
          labelKey={(g) => `reports.group.${g}`}
          ariaLabel={t("reports.filters.group_by")}
          onChange={setGroupBy}
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
                      key: "income",
                      title: t("reports.fields.income"),
                      value: amountText(totals.income, locale),
                    },
                    {
                      key: "expense",
                      title: t("reports.fields.expense"),
                      value: amountText(totals.expense, locale),
                    },
                    {
                      key: "net",
                      title: t("reports.fields.net"),
                      value: (
                        <span
                          className={
                            num(totals.net) < 0 ? "text-destructive" : undefined
                          }
                        >
                          {amountText(totals.net, locale)}
                        </span>
                      ),
                      hint: t("reports.kpi.margin", {
                        value: percentText(totals.margin, locale),
                      }),
                    },
                    {
                      key: "pass_through",
                      title: t("reports.fields.pass_through"),
                      value: amountText(totals.pass_through, locale),
                    },
                    {
                      key: "capex",
                      title: t("reports.fields.capex"),
                      value: amountText(totals.capex, locale),
                    },
                  ]
                : []
            }
          />
          <div className="grid gap-6 xl:grid-cols-2">
            <AppChart
              type="bar"
              title={t("reports.income_expense.trend_title")}
              description={t("reports.income_expense.trend_description")}
              data={trend}
              config={incomeExpenseChartConfig(t)}
              valueFormatter={(v) => compactText(v, locale)}
              categoryFormatter={(v) => monthLabel(v, locale)}
              height={260}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
            <AppChart
              type="bar-horizontal"
              title={
                groupBy === "category"
                  ? t("reports.income_expense.expense_breakdown")
                  : t("reports.income_expense.group_breakdown")
              }
              data={breakdown}
              config={
                groupBy === "category"
                  ? {
                      expense: {
                        label: t("reports.fields.expense"),
                        theme: EXPENSE_COLOR,
                      },
                    }
                  : incomeExpenseChartConfig(t)
              }
              valueFormatter={(v) => compactText(v, locale)}
              height={Math.max(200, breakdown.length * 36 + 40)}
              loading={query.isLoading}
              emptyTitle={t("reports.empty")}
            />
          </div>
          <EntitySectionCard
            title={t("reports.income_expense.table_title")}
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
                persistKey: `tenant-report-income-expense-${groupBy}-v1`,
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
              }}
              toolbarExtra={
                <>
                  <ResourceIOToolbar
                    resource="tenant.report_income_expense"
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
              t("reports.notes.income_expense"),
              t("reports.notes.kept_apart"),
            ]}
          />
        </>
      )}
    </ReportPage>
  );
}
