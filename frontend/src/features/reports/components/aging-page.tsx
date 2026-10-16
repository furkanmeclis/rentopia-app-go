"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo, useState } from "react";

import { AppChart, type ChartConfig } from "@/components/charts";
import {
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { DatePicker } from "@/components/ui/date-picker";
import { routes } from "@/config/routes";
import { Amount } from "@/features/finance/components/finance-cells";
import { amountText, todayISO } from "@/features/finance/lib/format";
import { ResourceIOToolbar } from "@/features/io";
import {
  ALL,
  MethodNotes,
  OptionSelect,
  ReportError,
  ReportFilters,
  ReportPage,
} from "@/features/reports/components/report-shell";
import { ReportKpis } from "@/features/reports/components/report-kpis";
import {
  AGING_BUCKETS,
  AGING_COLORS,
  PARTIES,
} from "@/features/reports/constants";
import { useReport } from "@/features/reports/hooks/use-reports";
import { compactText, num } from "@/features/reports/lib/format";
import type { AgingParty, AgingRow } from "@/features/reports/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Raporlar → Cari / vade yaşlandırma (TEC-738). */
export function AgingReportPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [asOf, setAsOf] = useState(todayISO());
  const [party, setParty] = useState<AgingParty>("customer");
  const [branch, setBranch] = useState(ALL);
  const params = {
    as_of: asOf || undefined,
    party,
    branch: branch === ALL ? undefined : branch,
  };
  const query = useReport("aging", params);
  const report = query.data;
  const totals = report?.totals;

  const columns = useMemo(
    () =>
      [
        createColumn<AgingRow>({
          id: "name",
          accessorFn: (row) => row.name,
          labelKey: "reports.fields.account",
          gridPrimary: true,
          enableSorting: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <Link
                className="font-medium underline-offset-2 hover:underline"
                href={routes.tenant.finance.account(slug, row.original.uuid)}
              >
                {row.original.name}
              </Link>
              <span className="text-muted-foreground text-xs">
                {row.original.code}
              </span>
            </div>
          ),
        }),
        createColumn<AgingRow>({
          id: "payment_term_days",
          accessorFn: (row) => row.payment_term_days,
          labelKey: "reports.fields.payment_term_days",
          enableSorting: true,
          defaultHidden: true,
        }),
        ...AGING_BUCKETS.map((key) =>
          createColumn<AgingRow>({
            id: key,
            accessorFn: (row) => num(row[key]),
            labelKey: `reports.aging.bucket.${key}`,
            enableSorting: true,
            cell: ({ row }) => (
              <Amount
                value={row.original[key]}
                muted={num(row.original[key]) === 0}
                className={
                  key === "days_90_plus" && num(row.original[key]) > 0
                    ? "text-destructive font-medium"
                    : undefined
                }
              />
            ),
          }),
        ),
        ...(["overdue", "open", "credit", "balance"] as const).map((key) =>
          createColumn<AgingRow>({
            id: key,
            accessorFn: (row) => num(row[key]),
            labelKey: `reports.fields.${key}`,
            enableSorting: true,
            defaultHidden: key === "credit" || key === "balance",
            cell: ({ row }) => (
              <Amount
                value={row.original[key]}
                className={key === "open" ? "font-medium" : undefined}
              />
            ),
          }),
        ),
        createColumn<AgingRow>({
          id: "last_entry",
          accessorFn: (row) => row.last_entry ?? "",
          labelKey: "reports.fields.last_entry",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.last_entry
              ? date(row.original.last_entry, "dd.MM.yyyy", locale)
              : "—",
        }),
      ] as ColumnDef<AgingRow, unknown>[],
    [slug, locale],
  );

  const bucketConfig: ChartConfig = {};
  AGING_BUCKETS.forEach((key, i) => {
    bucketConfig[key] = {
      label: t(`reports.aging.bucket.${key}`),
      theme: AGING_COLORS[i],
    };
  });
  const bucketRow: Record<string, number | string> = {
    label: t(`reports.party.${party}`),
  };
  for (const b of report?.buckets ?? []) bucketRow[b.key] = num(b.amount);
  const hasBuckets = (report?.buckets ?? []).some((b) => num(b.amount) !== 0);

  return (
    <ReportPage
      titleKey="reports.aging.title"
      descriptionKey="reports.aging.description"
    >
      <ReportFilters
        showPeriod={false}
        branch={branch}
        onBranchChange={setBranch}
      >
        <DatePicker
          value={asOf}
          onChange={setAsOf}
          placeholder={t("reports.filters.as_of")}
        />
        <OptionSelect
          value={party}
          options={PARTIES}
          labelKey={(p) => `reports.party.${p}`}
          ariaLabel={t("reports.filters.party")}
          onChange={setParty}
          className="w-full sm:w-64"
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
                      key: "open",
                      title: t("reports.fields.open"),
                      value: amountText(totals.open, locale),
                      hint: t("reports.kpi.accounts", {
                        count: report?.rows.length ?? 0,
                      }),
                    },
                    {
                      key: "overdue",
                      title: t("reports.fields.overdue"),
                      value: amountText(totals.overdue, locale),
                    },
                    {
                      key: "days_90_plus",
                      title: t("reports.aging.bucket.days_90_plus"),
                      value: (
                        <span
                          className={
                            num(totals.days_90_plus) > 0
                              ? "text-destructive"
                              : undefined
                          }
                        >
                          {amountText(totals.days_90_plus, locale)}
                        </span>
                      ),
                    },
                    {
                      key: "credit",
                      title: t("reports.fields.credit"),
                      value: amountText(totals.credit, locale),
                    },
                  ]
                : []
            }
          />
          <AppChart
            type="bar-horizontal"
            stacked
            showLegend
            title={t("reports.aging.chart_title")}
            description={t("reports.aging.chart_description")}
            data={hasBuckets ? [bucketRow] : []}
            config={bucketConfig}
            series={AGING_BUCKETS.map((key) => ({ key, stackId: "aging" }))}
            valueFormatter={(v) => compactText(v, locale)}
            height={140}
            loading={query.isLoading}
            emptyTitle={t("reports.aging.empty")}
          />
          <EntitySectionCard
            title={t("reports.aging.table_title")}
            badge={report?.rows.length}
          >
            <EntityTable
              columns={columns}
              data={report?.rows ?? []}
              getRowId={(row) => row.uuid}
              isLoading={query.isLoading}
              emptyTitle={t("reports.aging.empty")}
              emptyDescription={t("reports.aging.empty_description")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                persistKey: "tenant-report-aging-v1",
                rowSelection: false,
                columnFilters: false,
                facetedFilters: false,
              }}
              toolbarExtra={
                <>
                  <ResourceIOToolbar
                    resource="tenant.report_aging"
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
            asOf={report?.as_of}
            items={[
              {
                label: t("reports.filters.party"),
                value: t(`reports.party.${party}`),
              },
            ]}
            notes={[t("reports.notes.aging")]}
          />
        </>
      )}
    </ReportPage>
  );
}
