"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCheck, Eye, FileCheck2, RotateCcw } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  KabisResolveDialog,
  useKabisRetryAction,
} from "@/features/kabis/components/kabis-actions";
import {
  KabisOfficialBadge,
  KabisStatusChip,
  holdText,
  kabisEventText,
  missingFieldText,
} from "@/features/kabis/components/kabis-cells";
import {
  KABIS_EVENTS,
  KABIS_EVENT_LABEL_KEYS,
  KABIS_RESOURCE,
  KABIS_STATUSES,
  KABIS_STATUS_LABEL_KEYS,
} from "@/features/kabis/constants";
import { useKabisFacets, useKabisList } from "@/features/kabis/hooks/use-kabis";
import type {
  KabisNotification,
  KabisResolution,
  ListKabisParams,
} from "@/features/kabis/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useFeature } from "@/providers/feature-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const RETRYABLE = new Set([
  "blocked",
  "waiting_dependency",
  "pending",
  "failed",
  "needs_verification",
  "submitted",
]);
const MANUAL_FROM = new Set([
  "blocked",
  "waiting_dependency",
  "pending",
  "failed",
  "needs_verification",
]);
const RECONCILE_FROM = new Set(["needs_verification", "submitted", "failed"]);

/**
 * KABİS notifications as a server DataTable: status / event facets with
 * counts, date range, live updates and the row actions (open, retry with a
 * status check first, mark manual, reconcile).
 */
export function KabisTable({ persistKey }: { persistKey: string }) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const kabis = useFeature("kabis");
  const canRetry = kabis && can(permissions.tenant.kabis.retry);
  const canReconcile = can(permissions.tenant.kabis.reconcile);
  const urlDefaults = useUrlListDefaults(
    ["status", "event", "rental"],
    ["occurred_at", "created_at", "updated_at"],
  );
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-occurred_at",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 20,
    realtime: tenantLiveList(organization, KABIS_RESOURCE),
  });
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [resolve, setResolve] = useState<{
    uuid: string;
    action: KabisResolution;
  } | null>(null);
  const retry = useKabisRetryAction();

  const listParams = useMemo<ListKabisParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      event: joinFilter(column("event")),
      rental: joinFilter(column("rental")),
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params, to]);

  const listQuery = useKabisList(listParams);
  const facetsQuery = useKabisFacets();
  const facets = facetsQuery.data;
  const total = listQuery.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<KabisNotification>({
          accessorKey: "occurred_at",
          labelKey: "kabis.columns.occurred_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-sm whitespace-nowrap tabular-nums">
              {datetime(row.original.occurred_at, "d MMM yyyy HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<KabisNotification>({
          accessorKey: "event",
          labelKey: "kabis.columns.event",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: KABIS_EVENTS.map((v) => ({
            value: v,
            labelKey: KABIS_EVENT_LABEL_KEYS[v],
            label: `${t(KABIS_EVENT_LABEL_KEYS[v])} (${facets?.events[v] ?? 0})`,
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {kabisEventText(t, row.original.event, row.original.sequence)}
            </span>
          ),
        }),
        createColumn<KabisNotification>({
          id: "rental",
          accessorFn: (row) => row.rental.reference ?? "",
          labelKey: "kabis.columns.rental",
          enableSorting: false,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.rentals.detail(
                slug,
                row.original.rental.uuid,
              )}
              className="font-mono text-xs font-medium underline-offset-4 hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              {row.original.rental.reference}
            </Link>
          ),
        }),
        createColumn<KabisNotification>({
          accessorKey: "plate",
          labelKey: "kabis.columns.plate",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.plate || "—"}
            </span>
          ),
        }),
        createColumn<KabisNotification>({
          accessorKey: "status",
          labelKey: "kabis.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: KABIS_STATUSES.map((v) => ({
            value: v,
            labelKey: KABIS_STATUS_LABEL_KEYS[v],
            label: `${t(KABIS_STATUS_LABEL_KEYS[v])} (${facets?.statuses[v] ?? 0})`,
          })),
          cell: ({ row }) => {
            const n = row.original;
            const note =
              n.status === "blocked" && n.missing_fields.length
                ? missingFieldText(t, n.missing_fields[0]) +
                  (n.missing_fields.length > 1
                    ? ` +${n.missing_fields.length - 1}`
                    : "")
                : n.status === "waiting_dependency"
                  ? holdText(t, n.hold_reason)
                  : "";
            return (
              <span className="flex flex-col items-start gap-1">
                <span className="flex flex-wrap items-center gap-1.5">
                  <KabisStatusChip status={n.status} />
                  <KabisOfficialBadge status={n.status} />
                </span>
                {note ? (
                  <span className="text-muted-foreground text-[11px]">
                    {note}
                  </span>
                ) : null}
              </span>
            );
          },
        }),
        createColumn<KabisNotification>({
          accessorKey: "attempts",
          labelKey: "kabis.columns.attempts",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {row.original.attempts} / {row.original.max_attempts}
            </span>
          ),
        }),
        createColumn<KabisNotification>({
          id: "branch",
          accessorFn: (row) => row.branch.code ?? "",
          labelKey: "kabis.columns.branch",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-sm">{row.original.branch.name}</span>
          ),
        }),
        createColumn<KabisNotification>({
          accessorKey: "external_reference",
          labelKey: "kabis.columns.external_reference",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.external_reference ||
                row.original.resolution_reference ||
                "—"}
            </span>
          ),
        }),
        createColumn<KabisNotification>({
          accessorKey: "last_verified_at",
          labelKey: "kabis.columns.last_verified_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.last_verified_at
              ? datetime(row.original.last_verified_at, "d MMM HH:mm", locale)
              : "—",
        }),
        createColumn<KabisNotification>({
          accessorKey: "last_error",
          labelKey: "kabis.columns.last_error",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground line-clamp-2 text-xs break-words">
              {row.original.last_error || "—"}
            </span>
          ),
        }),
        createColumn<KabisNotification>({
          accessorKey: "updated_at",
          labelKey: "kabis.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.updated_at, "d MMM yyyy HH:mm", locale),
        }),
        ...auditColumns<KabisNotification>(
          { createdAt: "kabis.columns.created_at", uuid: "kabis.columns.uuid" },
          locale,
        ),
        createColumn<KabisNotification>({
          id: "actions",
          labelKey: "kabis.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const n = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: t("kabis.actions.open"),
                    icon: Eye,
                    onSelect: () =>
                      router.push(routes.tenant.kabis.detail(slug, n.uuid)),
                  },
                  ...(canRetry && RETRYABLE.has(n.status)
                    ? [
                        {
                          id: "retry",
                          label: t("kabis.actions.retry"),
                          icon: RotateCcw,
                          onSelect: () => retry.run(n.uuid),
                        },
                      ]
                    : []),
                  ...(canReconcile && MANUAL_FROM.has(n.status)
                    ? [
                        {
                          id: "mark-manual",
                          label: t("kabis.actions.mark_manual"),
                          icon: FileCheck2,
                          onSelect: () =>
                            setResolve({ uuid: n.uuid, action: "mark-manual" }),
                        },
                      ]
                    : []),
                  ...(canReconcile && RECONCILE_FROM.has(n.status)
                    ? [
                        {
                          id: "reconcile",
                          label: t("kabis.actions.reconcile"),
                          icon: CheckCheck,
                          onSelect: () =>
                            setResolve({ uuid: n.uuid, action: "reconcile" }),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<KabisNotification, unknown>[],
    [canReconcile, canRetry, facets, locale, retry, router, slug, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters =
    Boolean(listParams.q || from || to) ||
    listState.columnFilters.some((f) => filterValues([f], f.id).length > 0);

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(r) =>
          router.push(routes.tenant.kabis.detail(slug, r.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("kabis.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters ? t("kabis.empty_filtered_title") : t("kabis.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("kabis.empty_filtered_description")
            : t("kabis.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey, rowSelection: false }}
        toolbarExtra={
          <>
            <DatePicker
              value={from}
              onChange={(v) => setFrom(v ?? "")}
              placeholder={t("kabis.filters.from")}
              className="w-full sm:w-40"
            />
            <DatePicker
              value={to}
              onChange={(v) => setTo(v ?? "")}
              placeholder={t("kabis.filters.to")}
              className="w-full sm:w-40"
            />
            <EntityToolbar
              onRefresh={() => {
                void listQuery.refetch();
                void facetsQuery.refetch();
              }}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <KabisResolveDialog target={resolve} onClose={() => setResolve(null)} />
    </>
  );
}
