"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Amount } from "@/features/finance/components/finance-cells";
import {
  REPORT_GROUPS,
  REPORT_GROUP_LABEL_KEYS,
  REPORT_VALUE_LABEL_KEYS,
} from "@/features/finance/constants";
import { useFinanceReport } from "@/features/finance/hooks/use-finance";
import { monthStartISO, todayISO } from "@/features/finance/lib/format";
import type { FinanceReportRow } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const EXPENSE_VALUES = ["net_amount", "vat_amount", "total_amount"] as const;
const CASH_VALUES = [
  "total_in",
  "total_out",
  "collections",
  "expenses",
  "refunds",
  "supplier_payments",
] as const;

type ReportKind = "expenses" | "cash-flow";

function ReportSection({
  kind,
  title,
  groups,
  valueKeys,
  from,
  to,
}: {
  kind: ReportKind;
  title: string;
  groups: readonly string[];
  valueKeys: readonly string[];
  from: string;
  to: string;
}) {
  const { t, locale } = useLocale();
  const [group, setGroup] = useState<string>(groups[0] ?? "month");
  const query = useFinanceReport(kind, {
    group_by: group,
    from: from || undefined,
    to: to || undefined,
  });
  const report = query.data;
  const unnamed = (row: FinanceReportRow) => {
    if (row.label) return row.label;
    if (group === "branch") return t("finance.head_office");
    if (group === "vehicle") return t("finance.reports.no_vehicle");
    return "—";
  };
  const columns = useMemo(
    () =>
      [
        createColumn<FinanceReportRow>({
          accessorKey: "label",
          labelKey:
            REPORT_GROUP_LABEL_KEYS[group] ?? "finance.reports.group_month",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{unnamed(row.original)}</span>
          ),
        }),
        createColumn<FinanceReportRow>({
          accessorKey: "count",
          labelKey: "finance.reports.count",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.count}</span>
          ),
        }),
        ...valueKeys.map((key) =>
          createColumn<FinanceReportRow>({
            id: key,
            accessorFn: (row) => Number(row.values[key] ?? 0),
            labelKey: REPORT_VALUE_LABEL_KEYS[key] ?? key,
            cell: ({ row }) => (
              <Amount
                value={row.original.values[key] ?? "0.00"}
                currency={row.original.currency}
                className={key === "total_amount" ? "font-semibold" : undefined}
              />
            ),
          }),
        ),
      ] as ColumnDef<FinanceReportRow, unknown>[],
    // eslint-disable-next-line react-hooks/exhaustive-deps -- unnamed depends on group / t only
    [group, t, valueKeys],
  );

  return (
    <EntitySectionCard
      title={title}
      badge={
        report
          ? t("finance.reports.period", {
              from: date(report.from, "dd.MM.yyyy", locale),
              to: date(report.to, "dd.MM.yyyy", locale),
            })
          : undefined
      }
    >
      <div className="space-y-4">
        <EntityTable
          columns={columns}
          data={report?.rows ?? []}
          getRowId={(row, i) => `${row.key}-${row.currency}-${i}`}
          isLoading={query.isLoading}
          isError={query.isError}
          errorDescription={t("finance.load_failed")}
          onRetry={() => void query.refetch()}
          emptyTitle={t("finance.reports.empty")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: `tenant-finance-report-${kind}-v1`,
            rowSelection: false,
            globalFilter: false,
          }}
          toolbarExtra={
            <>
              <Select value={group} onValueChange={setGroup}>
                <SelectTrigger
                  className="w-full sm:w-48"
                  aria-label={t("finance.reports.group_by")}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {groups.map((g) => (
                    <SelectItem key={g} value={g}>
                      {t(REPORT_GROUP_LABEL_KEYS[g] ?? g)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <EntityToolbar
                onRefresh={() => void query.refetch()}
                refreshDisabled={query.isFetching}
              />
            </>
          }
        />
        {report ? (
          <dl className="grid grid-cols-2 gap-4 border-t pt-4 sm:grid-cols-3 lg:grid-cols-6">
            {valueKeys.map((key) => (
              <div key={key} className="space-y-1">
                <dt className="text-muted-foreground text-xs">
                  {t(REPORT_VALUE_LABEL_KEYS[key] ?? key)}
                </dt>
                <dd>
                  <Amount
                    value={report.totals[key] ?? "0.00"}
                    className="font-semibold"
                  />
                </dd>
              </div>
            ))}
          </dl>
        ) : null}
      </div>
    </EntitySectionCard>
  );
}

/** "Finans raporları": expense and cash-flow aggregates for a period. */
export function ReportsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [from, setFrom] = useState(monthStartISO());
  const [to, setTo] = useState(todayISO());

  return (
    <EntityPage
      title={t("finance.reports.title")}
      description={t("finance.reports.description")}
      permission={permissions.tenant.finance.reports.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("finance.reports.title") },
      ]}
    >
      <div className="space-y-6">
        <div className="grid grid-cols-2 gap-2 sm:flex sm:items-center">
          <DatePicker
            value={from}
            onChange={setFrom}
            placeholder={t("finance.filters.from")}
          />
          <DatePicker
            value={to}
            onChange={setTo}
            placeholder={t("finance.filters.to")}
          />
        </div>
        <ReportSection
          kind="expenses"
          title={t("finance.reports.expenses_title")}
          groups={REPORT_GROUPS.expenses}
          valueKeys={EXPENSE_VALUES}
          from={from}
          to={to}
        />
        <ReportSection
          kind="cash-flow"
          title={t("finance.reports.cash_flow_title")}
          groups={REPORT_GROUPS.cashFlow}
          valueKeys={CASH_VALUES}
          from={from}
          to={to}
        />
      </div>
    </EntityPage>
  );
}
