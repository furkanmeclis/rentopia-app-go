"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
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
import { useGroupsList } from "@/features/fleet/hooks/use-fleet";
import { ResourceIOToolbar } from "@/features/io";
import { useBranchOptions } from "@/features/pricing/hooks/use-pricing-options";
import {
  ProfitabilityFilters,
  useProfitabilityFilters,
} from "@/features/profitability/components/profitability-filters";
import {
  ContributionChart,
  MethodNotes,
  ProfitabilityKpis,
  ProfitabilityTrendChart,
  daysText,
  percentText,
} from "@/features/profitability/components/profitability-sections";
import { FIELD_LABEL_KEYS } from "@/features/profitability/constants";
import { useProfitabilityReport } from "@/features/profitability/hooks/use-profitability";
import { num } from "@/features/profitability/lib/period";
import type {
  ProfitabilityFigures,
  ProfitabilityRow,
} from "@/features/profitability/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const ALL = "__all__";

/** Money columns of the table (sortable, numeric). */
const MONEY_COLUMNS: {
  key: keyof ProfitabilityFigures;
  hidden?: boolean;
  strong?: boolean;
}[] = [
  { key: "gross_income" },
  { key: "commissions" },
  { key: "direct_costs" },
  { key: "overhead" },
  { key: "contribution", strong: true },
  { key: "rent", hidden: true },
  { key: "extras", hidden: true },
  { key: "charges", hidden: true },
  { key: "toll_fees", hidden: true },
  { key: "refunded", hidden: true },
  { key: "deposits_held", hidden: true },
  { key: "pass_through", hidden: true },
  { key: "capex", hidden: true },
];

/** The totals row under the table: the same figures for the whole page. */
function TotalsRow({ totals }: { totals: ProfitabilityFigures }) {
  const { t, locale } = useLocale();
  const items: { key: string; value: React.ReactNode }[] = [
    { key: "rental_days", value: daysText(totals.rental_days, locale) },
    { key: "occupancy", value: percentText(totals.occupancy, locale) },
    ...MONEY_COLUMNS.filter((c) => !c.hidden).map((c) => ({
      key: c.key,
      value: (
        <Amount
          value={String(totals[c.key])}
          className={
            c.key === "contribution" && num(totals.contribution) < 0
              ? "text-destructive font-semibold"
              : "font-semibold"
          }
        />
      ),
    })),
    { key: "margin", value: percentText(totals.margin, locale) },
  ];
  return (
    <div className="border-t pt-4">
      <p className="mb-2 text-sm font-semibold">{t("profitability.total")}</p>
      <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-9">
        {items.map((item) => (
          <div key={item.key} className="space-y-1">
            <dt className="text-muted-foreground text-xs">
              {t(FIELD_LABEL_KEYS[item.key] ?? item.key)}
            </dt>
            <dd className="tabular-nums">{item.value}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

/**
 * Finans → Araç kârlılığı (TEC-727): the fleet report for a period, basis
 * and overhead allocation method, with KPIs, charts, the per-vehicle
 * DataTable, its totals, the method notes and the export.
 */
export function ProfitabilityPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { state, setState, params } = useProfitabilityFilters("this_month");
  const [branch, setBranch] = useState(ALL);
  const [group, setGroup] = useState(ALL);
  const branches = useBranchOptions();
  const canGroups = can(permissions.tenant.fleet.read);
  const groups = useGroupsList(
    { limit: 100, offset: 0, sort: "code" },
    canGroups,
  );

  const query = useProfitabilityReport({
    ...params,
    branch: branch === ALL ? undefined : branch,
    rental_group: group === ALL ? undefined : group,
  });
  const report = query.data;
  const exportQuery = {
    from: params.from,
    to: params.to,
    basis: params.basis,
    allocation: params.allocation,
    branch: branch === ALL ? undefined : branch,
    rental_group: group === ALL ? undefined : group,
  };

  const columns = useMemo(
    () =>
      [
        createColumn<ProfitabilityRow>({
          id: "plate",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "profitability.fields.vehicle",
          gridPrimary: true,
          enableSorting: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <Link
                className="font-medium whitespace-nowrap underline-offset-2 hover:underline"
                href={`${routes.tenant.fleet.vehicles.detail(slug, row.original.vehicle.uuid)}?tab=profitability`}
              >
                {row.original.vehicle.plate}
              </Link>
              <span className="text-muted-foreground text-xs">
                {row.original.vehicle.label}
              </span>
            </div>
          ),
        }),
        createColumn<ProfitabilityRow>({
          id: "home_branch",
          accessorFn: (row) => row.vehicle.home_branch.name,
          labelKey: "profitability.fields.home_branch",
          enableSorting: true,
          defaultHidden: true,
        }),
        createColumn<ProfitabilityRow>({
          id: "rental_days",
          accessorFn: (row) => row.rental_days,
          labelKey: "profitability.fields.rental_days",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {daysText(row.original.rental_days, locale)}
            </span>
          ),
        }),
        createColumn<ProfitabilityRow>({
          id: "occupancy",
          accessorFn: (row) => row.occupancy ?? -1,
          labelKey: "profitability.fields.occupancy",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.occupancy, locale)}
            </span>
          ),
        }),
        ...MONEY_COLUMNS.map((c) =>
          createColumn<ProfitabilityRow>({
            id: c.key,
            accessorFn: (row) => num(String(row[c.key])),
            labelKey: FIELD_LABEL_KEYS[c.key] ?? c.key,
            enableSorting: true,
            defaultHidden: c.hidden,
            cell: ({ row }) => (
              <Amount
                value={String(row.original[c.key])}
                muted={c.hidden && c.key !== "rent"}
                className={
                  c.strong
                    ? num(row.original.contribution) < 0
                      ? "text-destructive font-semibold"
                      : "font-semibold"
                    : undefined
                }
              />
            ),
          }),
        ),
        createColumn<ProfitabilityRow>({
          id: "margin",
          accessorFn: (row) => row.margin ?? -Infinity,
          labelKey: "profitability.fields.margin",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {percentText(row.original.margin, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<ProfitabilityRow, unknown>[],
    [locale, slug],
  );

  return (
    <EntityPage
      title={t("profitability.title")}
      description={t("profitability.description")}
      permission={permissions.tenant.finance.reports.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("profitability.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("profitability.title") },
      ]}
    >
      <div className="space-y-6">
        <ProfitabilityFilters state={state} onChange={setState}>
          {branches.allowed ? (
            <Select value={branch} onValueChange={setBranch}>
              <SelectTrigger
                className="w-full sm:w-48"
                aria-label={t("profitability.filters.branch")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>
                  {t("profitability.filters.all_branches")}
                </SelectItem>
                {branches.options.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
          {canGroups ? (
            <Select value={group} onValueChange={setGroup}>
              <SelectTrigger
                className="w-full sm:w-48"
                aria-label={t("profitability.filters.rental_group")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>
                  {t("profitability.filters.all_groups")}
                </SelectItem>
                {(groups.data?.items ?? []).map((g) => (
                  <SelectItem key={g.uuid} value={g.uuid}>
                    {g.code} · {g.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
        </ProfitabilityFilters>

        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            description={t("profitability.load_failed")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : (
          <>
            <ProfitabilityKpis
              figures={report?.totals}
              loading={query.isLoading}
            />
            <div className="grid gap-6 xl:grid-cols-2">
              <ProfitabilityTrendChart
                trend={report?.trend ?? []}
                loading={query.isLoading}
              />
              <ContributionChart
                rows={report?.rows ?? []}
                loading={query.isLoading}
              />
            </div>
            <EntitySectionCard
              title={t("profitability.vehicles_title")}
              badge={report?.rows.length}
            >
              <div className="space-y-4">
                <EntityTable
                  columns={columns}
                  data={report?.rows ?? []}
                  getRowId={(row) => row.vehicle.uuid}
                  isLoading={query.isLoading}
                  emptyTitle={t("profitability.empty")}
                  emptyDescription={t("profitability.empty_description")}
                  manual={{
                    filtering: false,
                    sorting: false,
                    pagination: false,
                  }}
                  initialState={{
                    sorting: [{ id: "contribution", desc: true }],
                  }}
                  features={{
                    persistKey: "tenant-vehicle-profitability-v1",
                    rowSelection: false,
                    columnFilters: false,
                    facetedFilters: false,
                  }}
                  toolbarExtra={
                    <>
                      <ResourceIOToolbar
                        resource="tenant.vehicle_profitability"
                        query={exportQuery}
                        capabilities={{ export: true }}
                      />
                      <EntityToolbar
                        onRefresh={() => void query.refetch()}
                        refreshDisabled={query.isFetching}
                      />
                    </>
                  }
                />
                {report && report.rows.length > 0 ? (
                  <TotalsRow totals={report.totals} />
                ) : null}
              </div>
            </EntitySectionCard>
            <MethodNotes
              method={report?.method}
              from={report?.from}
              to={report?.to}
            />
          </>
        )}
      </div>
    </EntityPage>
  );
}
