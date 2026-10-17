"use client";

import type { ColumnDef, ColumnFiltersState } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useCallback, useMemo } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { routes } from "@/config/routes";
import {
  HEALTH_SEVERITIES,
  SEVERITY_LABEL_KEYS,
  SeverityChip,
} from "@/features/platform-health/components/health-chips";
import {
  useHealthIssues,
  useHealthSummary,
} from "@/features/platform-health/hooks/use-platform-health";
import { healthLabel } from "@/features/platform-health/lib/labels";
import type {
  HealthIssue,
  ListIssuesParams,
} from "@/features/platform-health/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Every problem the health sources report across organizations:
 * severity / source facets, search, sort; a row opens its organization.
 */
export function IssuesTab({ initialSource }: { initialSource?: string }) {
  const { t, locale } = useLocale();
  const router = useRouter();
  const initialColumnFilters = useMemo<ColumnFiltersState>(
    () => (initialSource ? [{ id: "source", value: [initialSource] }] : []),
    [initialSource],
  );
  const listState = useServerListState({
    initialSort: "-occurred_at",
    initialPageSize: 20,
    initialColumnFilters,
  });
  const summary = useHealthSummary();
  const sourceCodes = useMemo(
    () => (summary.data?.sources ?? []).map((s) => s.code),
    [summary.data?.sources],
  );

  const params = useMemo<ListIssuesParams>(() => {
    const joined = (id: string) => {
      const values = filterValues(
        listState.columnFilters.find((f) => f.id === id)?.value,
      );
      return values.length ? values.join(",") : undefined;
    };
    return {
      ...listState.params,
      source: joined("source"),
      severity: joined("severity"),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useHealthIssues(params);

  const columns = useMemo<ColumnDef<HealthIssue>[]>(
    () => [
      createColumn<HealthIssue>({
        accessorKey: "severity",
        labelKey: "platform_health.issues.columns.severity",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: HEALTH_SEVERITIES.map((value) => ({
          value,
          labelKey: SEVERITY_LABEL_KEYS[value],
          label: t(SEVERITY_LABEL_KEYS[value]),
        })),
        cell: ({ row }) => <SeverityChip severity={row.original.severity} />,
      }),
      createColumn<HealthIssue>({
        accessorKey: "source",
        labelKey: "platform_health.issues.columns.source",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: sourceCodes.map((value) => ({
          value,
          label: healthLabel(locale, "sources", value),
        })),
        cell: ({ row }) => (
          <span className="flex min-w-0 flex-col">
            <span className="truncate">
              {healthLabel(locale, "sources", row.original.source)}
            </span>
            <span className="text-muted-foreground truncate text-xs">
              {healthLabel(locale, "kinds", row.original.kind)}
            </span>
          </span>
        ),
      }),
      createColumn<HealthIssue>({
        id: "organization",
        accessorFn: (row) => row.organization?.name ?? "",
        labelKey: "platform_health.issues.columns.organization",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) =>
          row.original.organization ? (
            <span className="flex min-w-0 flex-col">
              <span className="truncate font-medium">
                {row.original.organization.name}
              </span>
              <code className="text-muted-foreground text-xs">
                {row.original.organization.slug}
              </code>
            </span>
          ) : (
            <span className="text-muted-foreground">
              {t("platform_health.issues.platform")}
            </span>
          ),
      }),
      createColumn<HealthIssue>({
        accessorKey: "provider",
        labelKey: "platform_health.issues.columns.provider",
        enableSorting: false,
        cell: ({ row }) => (
          <code className="text-xs">{row.original.provider || "—"}</code>
        ),
      }),
      createColumn<HealthIssue>({
        accessorKey: "subject",
        labelKey: "platform_health.issues.columns.subject",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="truncate">{row.original.subject || "—"}</span>
        ),
      }),
      createColumn<HealthIssue>({
        accessorKey: "message",
        labelKey: "platform_health.issues.columns.message",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.message ? (
            <span
              className="line-clamp-2 max-w-md text-sm break-words"
              title={row.original.message}
            >
              {row.original.message}
            </span>
          ) : (
            <span className="text-muted-foreground text-sm">
              {t("platform_health.issues.no_message")}
            </span>
          ),
      }),
      createColumn<HealthIssue>({
        accessorKey: "occurred_at",
        labelKey: "platform_health.issues.columns.occurred_at",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="text-sm whitespace-nowrap tabular-nums">
            {datetime(row.original.occurred_at, undefined, locale)}
          </span>
        ),
      }),
    ],
    [locale, sourceCodes, t],
  );

  const openOrganization = useCallback(
    (row: HealthIssue) => {
      if (row.organization) {
        router.push(
          routes.platform.organizations.detail(row.organization.uuid),
        );
      }
    },
    [router],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters =
    Boolean(params.q) || Boolean(params.source) || Boolean(params.severity);

  return (
    <EntityTable
      columns={columns}
      data={listQuery.data?.items ?? []}
      getRowId={(row) => row.key}
      onRowClick={openOrganization}
      isLoading={listQuery.isLoading}
      isError={listQuery.isError}
      errorDescription={t("platform_health.load_failed")}
      onRetry={() => void listQuery.refetch()}
      emptyTitle={
        hasFilters
          ? t("platform_health.issues.empty_filtered_title")
          : t("platform_health.issues.empty_title")
      }
      emptyDescription={
        hasFilters
          ? t("platform_health.issues.empty_filtered_description")
          : t("platform_health.issues.empty_description")
      }
      pageCount={pageCount}
      state={listState.tableState}
      features={{ persistKey: "platform-health-issues-v1", viewMode: true }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void listQuery.refetch()}
          refreshDisabled={listQuery.isFetching}
        />
      }
    />
  );
}
