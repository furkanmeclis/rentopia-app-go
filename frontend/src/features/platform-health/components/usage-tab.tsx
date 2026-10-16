"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { OrganizationStatusChip } from "@/features/organizations/components/organization-cells";
import {
  ORGANIZATION_STATUSES,
  STATUS_LABEL_KEYS,
} from "@/features/organizations/constants";
import { ReportKpis } from "@/features/reports/components/report-kpis";
import {
  useUsageList,
  useUsageSummary,
} from "@/features/platform-health/hooks/use-platform-health";
import {
  formatBytes,
  monthLabel,
  recentMonths,
} from "@/features/platform-health/lib/labels";
import type {
  ListUsageParams,
  OrganizationUsage,
} from "@/features/platform-health/types";
import { relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function Count({ value }: { value: number }) {
  return <span className="tabular-nums">{value.toLocaleString()}</span>;
}

/**
 * Usage per organization: platform totals for the month, then the
 * server-side DataTable (search, status facet, sort on every figure).
 */
export function UsageTab() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const months = useMemo(() => recentMonths(), []);
  const [month, setMonth] = useState(months[0]);
  const listState = useServerListState({
    initialSort: "-active_vehicles",
    initialPageSize: 20,
  });

  const params = useMemo<ListUsageParams>(() => {
    const status = filterValues(
      listState.columnFilters.find((f) => f.id === "status")?.value,
    );
    return {
      ...listState.params,
      status: status.length ? status.join(",") : undefined,
      month,
    };
  }, [listState.columnFilters, listState.params, month]);

  const listQuery = useUsageList(params);
  const summaryQuery = useUsageSummary(month);
  const summary = summaryQuery.data;

  const columns = useMemo<ColumnDef<OrganizationUsage>[]>(
    () => [
      createColumn<OrganizationUsage>({
        accessorKey: "name",
        labelKey: "platform_health.usage.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="flex min-w-0 flex-col">
            <span className="truncate font-medium">{row.original.name}</span>
            <code className="text-muted-foreground text-xs">
              {row.original.slug}
            </code>
          </span>
        ),
      }),
      createColumn<OrganizationUsage>({
        accessorKey: "status",
        labelKey: "platform_health.usage.columns.status",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: ORGANIZATION_STATUSES.map((value) => ({
          value,
          labelKey: STATUS_LABEL_KEYS[value],
          label: t(STATUS_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => (
          <OrganizationStatusChip status={row.original.status} />
        ),
      }),
      createColumn<OrganizationUsage>({
        accessorKey: "active_vehicles",
        labelKey: "platform_health.usage.columns.active_vehicles",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="flex items-baseline gap-1">
            <Count value={row.original.active_vehicles} />
            <span className="text-muted-foreground text-xs">
              {t("platform_health.usage.vehicles_of", {
                total: row.original.total_vehicles,
              })}
            </span>
          </span>
        ),
      }),
      createColumn<OrganizationUsage>({
        accessorKey: "reservations_month",
        labelKey: "platform_health.usage.columns.reservations_month",
        enableSorting: true,
        cell: ({ row }) => <Count value={row.original.reservations_month} />,
      }),
      createColumn<OrganizationUsage>({
        accessorKey: "member_count",
        labelKey: "platform_health.usage.columns.member_count",
        enableSorting: true,
        cell: ({ row }) => <Count value={row.original.member_count} />,
      }),
      createColumn<OrganizationUsage>({
        accessorKey: "storage_bytes",
        labelKey: "platform_health.usage.columns.storage_bytes",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="tabular-nums">
            {formatBytes(row.original.storage_bytes, locale)}
          </span>
        ),
      }),
      createColumn<OrganizationUsage>({
        accessorKey: "last_login_at",
        labelKey: "platform_health.usage.columns.last_login_at",
        enableSorting: true,
        cell: ({ row }) =>
          row.original.last_login_at ? (
            <span className="text-sm">
              {relativeDatetime(row.original.last_login_at, locale)}
            </span>
          ) : (
            <span className="text-muted-foreground text-sm">
              {t("platform_health.usage.never")}
            </span>
          ),
      }),
    ],
    [locale, t],
  );

  const openOrganization = useCallback(
    (row: OrganizationUsage) =>
      router.push(routes.platform.organizations.detail(row.uuid)),
    [router],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Boolean(params.q) || Boolean(params.status);

  return (
    <div className="space-y-6">
      <ReportKpis
        loading={summaryQuery.isLoading}
        count={5}
        items={[
          {
            key: "organizations",
            title: t("platform_health.kpi.organizations"),
            value: (summary?.organizations ?? 0).toLocaleString(locale),
            hint: t("platform_health.kpi.organizations_hint", {
              active: summary?.active_organizations ?? 0,
            }),
          },
          {
            key: "vehicles",
            title: t("platform_health.kpi.active_vehicles"),
            value: (summary?.active_vehicles ?? 0).toLocaleString(locale),
          },
          {
            key: "reservations",
            title: t("platform_health.kpi.reservations_month"),
            value: (summary?.reservations_month ?? 0).toLocaleString(locale),
            hint: monthLabel(month, locale),
          },
          {
            key: "members",
            title: t("platform_health.kpi.members"),
            value: (summary?.members ?? 0).toLocaleString(locale),
          },
          {
            key: "storage",
            title: t("platform_health.kpi.storage"),
            value: formatBytes(summary?.storage_bytes ?? 0, locale),
            hint: t("platform_health.kpi.storage_hint"),
          },
        ]}
      />

      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openOrganization}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("platform_health.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters
            ? t("platform_health.usage.empty_filtered_title")
            : t("platform_health.usage.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("platform_health.usage.empty_filtered_description")
            : t("platform_health.usage.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "platform-usage-v1", viewMode: true }}
        toolbarExtra={
          <>
            <Select value={month} onValueChange={setMonth}>
              <SelectTrigger
                className="h-8 w-[170px]"
                aria-label={t("platform_health.month")}
                title={t("platform_health.month_hint")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {months.map((m) => (
                  <SelectItem key={m} value={m}>
                    {monthLabel(m, locale)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <EntityToolbar
              onRefresh={() => {
                void listQuery.refetch();
                void summaryQuery.refetch();
              }}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
    </div>
  );
}
