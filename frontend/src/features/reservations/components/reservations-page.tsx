"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, CheckCircle2, Eye } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import {
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { moneyText } from "@/features/pricing/lib/format";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import {
  PaymentStatusChip,
  ReferenceText,
  ReservationStatusChip,
  actorName,
} from "@/features/reservations/components/reservation-cells";
import { useReservationBulkParams } from "@/features/reservations/components/reservation-bulk-params";
import {
  PAYMENT_LABEL_KEYS,
  PAYMENT_STATUSES,
  RESERVATIONS_RESOURCE,
  RESERVATION_STATUSES,
  STATUS_LABEL_KEYS,
} from "@/features/reservations/constants";
import {
  useReservationFacets,
  useReservationTransition,
  useReservationsList,
  useReservationsMeta,
} from "@/features/reservations/hooks/use-reservations";
import type {
  ListReservationsParams,
  ReservationFacet,
  ReservationListItem,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function countOf(facets: ReservationFacet[] | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

function facetOptions(facets: ReservationFacet[] | undefined) {
  return (facets ?? []).map((f) => ({
    value: f.value,
    label: `${f.label || f.value} (${f.count})`,
  }));
}

/**
 * Reservations ("Rezervasyonlar"): every reservation whose pick-up or
 * return branch is in the member's branch scope. Facets, live updates,
 * export and bulk cancel / assign staff.
 */
export function ReservationsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const urlDefaults = useUrlListDefaults(
    ["status", "payment_status", "channel", "branch"],
    ["reference", "status", "starts_at", "ends_at", "created_at"],
  );
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-created_at",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RESERVATIONS_RESOURCE),
  });
  const [cancelTarget, setCancelTarget] = useState<ReservationListItem | null>(
    null,
  );
  const transition = useReservationTransition();
  const bulkParams = useReservationBulkParams();

  const listParams = useMemo<ListReservationsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      payment_status: joinFilter(column("payment_status")),
      channel: joinFilter(column("channel")),
      product: joinFilter(column("product")),
      branch: joinFilter(column("branch")),
      customer: joinFilter(column("customer")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useReservationsList(listParams);
  const metaQuery = useReservationsMeta();
  const facetsQuery = useReservationFacets();
  const facets = facetsQuery.data;

  const baseColumns = useMemo(
    () =>
      [
        createColumn<ReservationListItem>({
          accessorKey: "reference",
          labelKey: "reservations.columns.reference",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5">
              <ReferenceText reference={row.original.reference} />
              {row.original.open_tasks > 0 ? (
                <Badge variant="warning" className="px-1.5 py-0 text-[10px]">
                  {t("reservations.open_tasks_badge", {
                    count: row.original.open_tasks,
                  })}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "status",
          labelKey: "reservations.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: RESERVATION_STATUSES.map((v) => ({
            value: v,
            labelKey: STATUS_LABEL_KEYS[v],
            label: `${t(STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
          })),
          cell: ({ row }) => (
            <ReservationStatusChip status={row.original.status} />
          ),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "payment_status",
          labelKey: "reservations.columns.payment_status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: PAYMENT_STATUSES.map((v) => ({
            value: v,
            labelKey: PAYMENT_LABEL_KEYS[v],
            label: `${t(PAYMENT_LABEL_KEYS[v])} (${countOf(facets?.payment_status, v)})`,
          })),
          cell: ({ row }) => (
            <PaymentStatusChip status={row.original.payment_status} />
          ),
        }),
        createColumn<ReservationListItem>({
          id: "customer",
          accessorFn: (row) => row.customer.display_name,
          labelKey: "reservations.columns.customer",
          enableSorting: false,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: facetOptions(facets?.customer),
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.customer.display_name}
            </span>
          ),
        }),
        createColumn<ReservationListItem>({
          id: "product",
          accessorFn: (row) => row.product.code,
          labelKey: "reservations.columns.product",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: facetOptions(facets?.product),
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.product.code}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.product.title}
              </span>
            </span>
          ),
        }),
        createColumn<ReservationListItem>({
          id: "branch",
          accessorFn: (row) => row.pickup_branch.code,
          labelKey: "reservations.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: facetOptions(facets?.branch),
          cell: ({ row }) =>
            row.original.pickup_branch.uuid === row.original.return_branch.uuid
              ? row.original.pickup_branch.code
              : `${row.original.pickup_branch.code} → ${row.original.return_branch.code}`,
        }),
        createColumn<ReservationListItem>({
          accessorKey: "starts_at",
          labelKey: "reservations.columns.starts_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.starts_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "ends_at",
          labelKey: "reservations.columns.ends_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.ends_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "channel",
          labelKey: "reservations.columns.channel",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: (facets?.channel ?? []).map((f) => ({
            value: f.value,
            label: `${channelLabel(t, f.value)} (${f.count})`,
          })),
          cell: ({ row }) => channelLabel(t, row.original.channel),
        }),
        createColumn<ReservationListItem>({
          id: "plate",
          accessorFn: (row) => row.vehicle?.plate ?? "",
          labelKey: "reservations.columns.plate",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.vehicle ? (
              <span className="flex items-center gap-1 font-mono text-xs">
                {row.original.vehicle.plate}
                {row.original.plate_fixed ? (
                  <Badge variant="outline" className="px-1 py-0 text-[10px]">
                    {t("reservations.plan.fixed_short")}
                  </Badge>
                ) : null}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<ReservationListItem>({
          accessorKey: "total_amount",
          labelKey: "reservations.columns.total_amount",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.total_amount, locale)}
            </span>
          ),
        }),
        createColumn<ReservationListItem>({
          id: "assigned_user",
          accessorFn: (row) => row.assigned_user?.email ?? "",
          labelKey: "reservations.columns.assigned_user",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => actorName(row.original.assigned_user),
        }),
        createColumn<ReservationListItem>({
          id: "deadline",
          accessorFn: (row) => row.deadline ?? "",
          labelKey: "reservations.columns.deadline",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.deadline
              ? datetime(row.original.deadline, "d MMM HH:mm", locale)
              : "—",
        }),
        ...auditColumns<ReservationListItem>(
          {
            createdAt: "reservations.columns.created_at",
            uuid: "reservations.columns.uuid",
          },
          locale,
        ),
        createColumn<ReservationListItem>({
          id: "actions",
          labelKey: "reservations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const r = row.original;
            const active =
              r.status !== "cancelled" &&
              r.status !== "expired" &&
              r.status !== "no_show";
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "view",
                    label: t("reservations.actions.open"),
                    icon: Eye,
                    onSelect: () =>
                      router.push(
                        routes.tenant.reservations.detail(slug, r.uuid),
                      ),
                  },
                  ...(r.status === "awaiting_review" ||
                  r.status === "awaiting_payment"
                    ? [
                        {
                          id: "confirm",
                          label: t("reservations.actions.confirm"),
                          icon: CheckCircle2,
                          permission: permissions.tenant.reservations.confirm,
                          onSelect: () =>
                            transition.mutate({
                              uuid: r.uuid,
                              action: "confirm" as const,
                            }),
                        },
                      ]
                    : []),
                  ...(active
                    ? [
                        {
                          id: "cancel",
                          label: t("reservations.actions.cancel"),
                          icon: Ban,
                          permission: permissions.tenant.reservations.cancel,
                          variant: "destructive" as const,
                          onSelect: () => setCancelTarget(r),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<ReservationListItem, unknown>[],
    [facets, locale, router, slug, t, transition],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<ReservationListItem>(), ...baseColumns],
    [baseColumns],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      payment_status: listParams.payment_status,
      channel: listParams.channel,
      product: listParams.product,
      branch: listParams.branch,
      customer: listParams.customer,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.channel,
      listParams.customer,
      listParams.payment_status,
      listParams.product,
      listParams.q,
      listParams.sort,
      listParams.status,
    ],
  );
  const total = listQuery.data?.total ?? 0;
  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery: engineQuery,
    total,
  });
  const meta = metaQuery.data;
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(
        RESERVATIONS_RESOURCE,
        meta?.bulk_actions ?? [],
      ),
    [meta?.bulk_actions],
  );
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Object.entries(engineQuery).some(
    ([key, value]) => key !== "sort" && Boolean(value),
  );
  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("reservations.title")}
      description={t("reservations.description")}
      permission={permissions.tenant.reservations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reservations.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.reservations.create(slug))}
          label={t("reservations.actions.create")}
          permission={permissions.tenant.reservations.write}
        />
      }
    >
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(r) =>
          router.push(routes.tenant.reservations.detail(slug, r.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("reservations.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("reservations.empty_filtered_title")
            : t("reservations.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("reservations.empty_filtered_description")
            : t("reservations.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-reservations-v1",
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={RESERVATIONS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              resolveParams={bulkParams.resolveParams}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={RESERVATIONS_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      {bulkParams.dialog}
      <ReasonDialog
        open={Boolean(cancelTarget)}
        title={t("reservations.cancel.title")}
        description={
          cancelTarget
            ? t("reservations.cancel.description", {
                reference: cancelTarget.reference,
              })
            : undefined
        }
        label={t("reservations.fields.cancel_reason")}
        confirmLabel={t("reservations.actions.cancel")}
        destructive
        pending={transition.isPending}
        onCancel={() => setCancelTarget(null)}
        onConfirm={(reason) => {
          if (!cancelTarget) return;
          transition.mutate(
            { uuid: cancelTarget.uuid, action: "cancel", reason },
            { onSuccess: () => setCancelTarget(null) },
          );
        }}
      />
    </EntityPage>
  );
}
