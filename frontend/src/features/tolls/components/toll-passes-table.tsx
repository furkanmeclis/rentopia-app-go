"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Eye, Link2, RotateCcw, Unlink } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type ReactNode, useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import {
  ReviewReasonBadge,
  TollStatusChip,
  stationText,
  tollMoney,
} from "@/features/tolls/components/toll-cells";
import {
  AssignTollDialog,
  type ReasonAction,
  TollReasonDialog,
} from "@/features/tolls/components/toll-decision-dialogs";
import {
  REVIEW_REASONS,
  REVIEW_REASON_LABEL_KEYS,
  REVIEW_STATUSES,
  SOURCE_LABEL_KEYS,
  TOLLS_RESOURCE,
  TOLL_STATUSES,
  TOLL_STATUS_LABEL_KEYS,
} from "@/features/tolls/constants";
import {
  useTollFacets,
  useTollPassesList,
} from "@/features/tolls/hooks/use-tolls";
import type {
  ListTollPassesParams,
  TollFacet,
  TollPass,
} from "@/features/tolls/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type TollTableMode = "all" | "review" | "vehicle";

function countOf(facets: TollFacet[] | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

/**
 * Toll passes as a server DataTable: the full list, the review queue
 * (unmatched + needs review, oldest first) or one vehicle's passes. Date
 * range, status / reason / source / rental facets, live updates and the
 * review actions (assign, unassign, ignore, rematch — each with a reason).
 */
export function TollPassesTable({
  mode,
  vehicleUuid,
  persistKey,
  toolbarExtra,
}: {
  mode: TollTableMode;
  vehicleUuid?: string;
  persistKey: string;
  toolbarExtra?: (query: Record<string, string | undefined>) => ReactNode;
}) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const canReview = can(permissions.tenant.tolls.review);
  const urlDefaults = useUrlListDefaults(
    mode === "vehicle"
      ? []
      : ["status", "review_reason", "source", "has_rental", "rental"],
    ["passed_at", "amount", "status", "plate", "created_at"],
  );
  const listState = useServerListState({
    initialSort:
      urlDefaults.initialSort ??
      (mode === "review" ? "passed_at" : "-passed_at"),
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: mode === "vehicle" ? 10 : 20,
    realtime: tenantLiveList(organization, TOLLS_RESOURCE),
  });
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [assign, setAssign] = useState<TollPass | null>(null);
  const [reasonTarget, setReasonTarget] = useState<{
    pass: TollPass;
    action: ReasonAction;
  } | null>(null);

  const listParams = useMemo<ListTollPassesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    const statuses = column("status");
    return {
      ...listState.params,
      status: joinFilter(
        mode === "review" && statuses.length === 0
          ? [...REVIEW_STATUSES]
          : statuses,
      ),
      review_reason: joinFilter(column("review_reason")),
      source: joinFilter(column("source")),
      has_rental:
        column("has_rental").length === 1 ? column("has_rental")[0] : undefined,
      rental: joinFilter(column("rental")),
      vehicle: vehicleUuid,
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params, mode, to, vehicleUuid]);

  const listQuery = useTollPassesList(listParams);
  const facetsQuery = useTollFacets(mode !== "vehicle");
  const facets = facetsQuery.data;
  const total = listQuery.data?.total ?? 0;

  const statusOptions = (
    mode === "review" ? REVIEW_STATUSES : TOLL_STATUSES
  ).map((v) => ({
    value: v,
    labelKey: TOLL_STATUS_LABEL_KEYS[v],
    label:
      mode === "vehicle"
        ? t(TOLL_STATUS_LABEL_KEYS[v])
        : `${t(TOLL_STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
  }));

  const columns = useMemo(
    () =>
      [
        createColumn<TollPass>({
          accessorKey: "passed_at",
          labelKey: "tolls.columns.passed_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-sm whitespace-nowrap tabular-nums">
              {datetime(row.original.passed_at, "d MMM yyyy HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<TollPass>({
          accessorKey: "plate",
          labelKey: "tolls.columns.plate",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-mono text-xs font-medium">
                {row.original.vehicle?.plate || row.original.plate || "—"}
              </span>
              {row.original.tag_number ? (
                <span className="text-muted-foreground font-mono text-[11px]">
                  {row.original.tag_number}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<TollPass>({
          accessorKey: "station",
          labelKey: "tolls.columns.station",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-sm">{stationText(row.original)}</span>
          ),
        }),
        createColumn<TollPass>({
          accessorKey: "amount",
          labelKey: "tolls.columns.amount",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="flex flex-col items-start">
              <span className="font-medium tabular-nums">
                {tollMoney(row.original.amount, row.original.currency, locale)}
              </span>
              {row.original.fee_amount ? (
                <span className="text-muted-foreground text-[11px] tabular-nums">
                  {t("tolls.fee_short", {
                    amount: tollMoney(
                      row.original.fee_amount,
                      row.original.currency,
                      locale,
                    ),
                  })}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<TollPass>({
          accessorKey: "status",
          labelKey: "tolls.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: statusOptions,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1.5">
              <TollStatusChip status={row.original.status} />
              {row.original.revision > 1 ? (
                <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                  {t("tolls.revision_badge", {
                    revision: row.original.revision,
                  })}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<TollPass>({
          accessorKey: "review_reason",
          labelKey: "tolls.columns.review_reason",
          enableSorting: false,
          defaultHidden: mode !== "review",
          filterVariant: mode === "vehicle" ? undefined : "faceted",
          filterOptions: REVIEW_REASONS.map((v) => ({
            value: v,
            labelKey: REVIEW_REASON_LABEL_KEYS[v],
            label: `${t(REVIEW_REASON_LABEL_KEYS[v])} (${countOf(facets?.review_reason, v)})`,
          })),
          cell: ({ row }) => (
            <ReviewReasonBadge reason={row.original.review_reason} />
          ),
        }),
        createColumn<TollPass>({
          id: "rental",
          accessorFn: (row) => row.rental?.reference ?? "",
          labelKey: "tolls.columns.rental",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.rental ? (
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
            ) : (
              <span className="text-muted-foreground text-xs">
                {t("tolls.no_rental_yet")}
              </span>
            ),
        }),
        createColumn<TollPass>({
          id: "has_rental",
          accessorFn: (row) => String(Boolean(row.rental)),
          labelKey: "tolls.columns.has_rental",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: mode === "vehicle" ? undefined : "faceted",
          filterOptions: [
            { value: "true", label: t("tolls.filters.with_rental") },
            { value: "false", label: t("tolls.filters.without_rental") },
          ],
          cell: ({ row }) =>
            row.original.rental
              ? t("tolls.filters.with_rental")
              : t("tolls.filters.without_rental"),
        }),
        createColumn<TollPass>({
          accessorKey: "source",
          labelKey: "tolls.columns.source",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: mode === "vehicle" ? undefined : "faceted",
          filterOptions: (["api", "file"] as const).map((v) => ({
            value: v,
            labelKey: SOURCE_LABEL_KEYS[v],
            label: `${t(SOURCE_LABEL_KEYS[v])} (${countOf(facets?.source, v)})`,
          })),
          cell: ({ row }) => t(SOURCE_LABEL_KEYS[row.original.source]),
        }),
        createColumn<TollPass>({
          accessorKey: "external_id",
          labelKey: "tolls.columns.external_id",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.external_id}
            </span>
          ),
        }),
        ...auditColumns<TollPass>(
          {
            createdAt: "tolls.columns.created_at",
            uuid: "tolls.columns.uuid",
          },
          locale,
        ),
        createColumn<TollPass>({
          id: "actions",
          labelKey: "tolls.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const p = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: t("tolls.actions.open"),
                    icon: Eye,
                    onSelect: () =>
                      router.push(routes.tenant.tolls.detail(slug, p.uuid)),
                  },
                  ...(canReview
                    ? [
                        {
                          id: "assign",
                          label: p.rental
                            ? t("tolls.actions.reassign")
                            : t("tolls.actions.assign"),
                          icon: Link2,
                          onSelect: () => setAssign(p),
                        },
                        ...(p.rental
                          ? [
                              {
                                id: "unassign",
                                label: t("tolls.actions.unassign"),
                                icon: Unlink,
                                onSelect: () =>
                                  setReasonTarget({
                                    pass: p,
                                    action: "unassign",
                                  }),
                              },
                            ]
                          : []),
                        ...(p.status !== "ignored"
                          ? [
                              {
                                id: "ignore",
                                label: t("tolls.actions.ignore"),
                                icon: Ban,
                                onSelect: () =>
                                  setReasonTarget({
                                    pass: p,
                                    action: "ignore",
                                  }),
                              },
                            ]
                          : []),
                        {
                          id: "rematch",
                          label: t("tolls.actions.rematch"),
                          icon: RotateCcw,
                          onSelect: () =>
                            setReasonTarget({ pass: p, action: "rematch" }),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<TollPass, unknown>[],
    [canReview, facets, locale, mode, router, slug, statusOptions, t],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      review_reason: listParams.review_reason,
      source: listParams.source,
      has_rental: listParams.has_rental,
      rental: listParams.rental,
      vehicle: listParams.vehicle,
      from: listParams.from,
      to: listParams.to,
      sort: listParams.sort,
    }),
    [listParams],
  );
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters =
    Boolean(listParams.q || from || to) ||
    listState.columnFilters.some((f) => filterValues([f], f.id).length > 0);
  const refresh = () => {
    void listQuery.refetch();
    if (mode !== "vehicle") void facetsQuery.refetch();
  };

  const empty =
    mode === "review"
      ? {
          title: hasFilters
            ? t("tolls.empty_filtered_title")
            : t("tolls.review.empty_title"),
          description: hasFilters
            ? t("tolls.empty_filtered_description")
            : t("tolls.review.empty_description"),
        }
      : {
          title: hasFilters
            ? t("tolls.empty_filtered_title")
            : t("tolls.empty_title"),
          description: hasFilters
            ? t("tolls.empty_filtered_description")
            : mode === "vehicle"
              ? t("tolls.vehicle.empty_description")
              : t("tolls.empty_description"),
        };

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(r) =>
          router.push(routes.tenant.tolls.detail(slug, r.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("tolls.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([
            listQuery.refetch(),
            mode !== "vehicle" ? facetsQuery.refetch() : Promise.resolve(),
          ]),
        )}
        emptyTitle={empty.title}
        emptyDescription={empty.description}
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey, rowSelection: false }}
        toolbarExtra={
          <>
            <DatePicker
              value={from}
              onChange={(v) => setFrom(v ?? "")}
              placeholder={t("tolls.filters.from")}
              className="w-full sm:w-40"
            />
            <DatePicker
              value={to}
              onChange={(v) => setTo(v ?? "")}
              placeholder={t("tolls.filters.to")}
              className="w-full sm:w-40"
            />
            {toolbarExtra?.(engineQuery)}
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <AssignTollDialog pass={assign} onClose={() => setAssign(null)} />
      <TollReasonDialog
        target={reasonTarget}
        onClose={() => setReasonTarget(null)}
      />
    </>
  );
}
