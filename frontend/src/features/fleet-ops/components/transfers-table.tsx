"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  CarFront,
  CircleCheck,
  CircleX,
  Pencil,
  Play,
  Plus,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { PermissionGuard } from "@/components/common/permission-guard";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  DateTimeCell,
  MoneyCell,
  RouteCell,
  TransferStatusChip,
} from "@/features/fleet-ops/components/fleet-ops-cells";
import {
  TransferActionDialog,
  TransferDialog,
} from "@/features/fleet-ops/components/transfer-dialogs";
import {
  TRANSFERS_RESOURCE,
  TRANSFER_STATUSES,
  TRANSFER_STATUS_LABEL_KEYS,
} from "@/features/fleet-ops/constants";
import {
  useTransferFacets,
  useTransfersList,
  useTransfersMeta,
} from "@/features/fleet-ops/hooks/use-fleet-ops";
import type {
  ListTransfersParams,
  Transfer,
  TransferAction,
  TransferFacets,
} from "@/features/fleet-ops/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { PlateBadge } from "@/features/vehicles/components/vehicle-cells";
import { useLocale } from "@/providers/locale-provider";

type FacetList = TransferFacets[keyof TransferFacets];

function countOf(facets: FacetList | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

function referenceOptions(facets: FacetList | undefined) {
  return (facets ?? []).map((facet) => ({
    value: facet.value,
    label: `${facet.label ?? facet.value} (${facet.count})`,
  }));
}

function personName(person: Transfer["responsible"]) {
  if (!person) return null;
  return `${person.name} ${person.surname}`.trim() || person.email;
}

/**
 * Transfers DataTable: the full list (faceted filters, export, live) or,
 * with `vehicleUuid`, the "Transferler" tab of a vehicle. Lifecycle
 * actions (start / complete / cancel) and edit live in the row menu; the
 * create dialog in the toolbar.
 */
export function TransfersTable({ vehicleUuid }: { vehicleUuid?: string }) {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const inVehicle = Boolean(vehicleUuid);
  const listState = useServerListState({
    initialSort: "-planned_departure",
    initialPageSize: inVehicle ? 10 : 20,
    realtime: tenantLiveList(organization, TRANSFERS_RESOURCE),
  });
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Transfer | null>(null);
  const [actionTarget, setActionTarget] = useState<{
    transfer: Transfer;
    action: TransferAction;
  } | null>(null);
  const write = permissions.tenant.transfers.write;

  const listParams = useMemo<ListTransfersParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      from_branch: joinFilter(column("from_branch")),
      to_branch: joinFilter(column("to_branch")),
      vehicle: vehicleUuid,
    };
  }, [listState.columnFilters, listState.params, vehicleUuid]);

  const listQuery = useTransfersList(listParams);
  const metaQuery = useTransfersMeta();
  const facetsQuery = useTransferFacets(!inVehicle);
  const facets = facetsQuery.data;

  const columns = useMemo(
    () =>
      [
        createColumn<Transfer>({
          id: "plate",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "transfers.columns.plate",
          enableSorting: true,
          enableHiding: !inVehicle,
          defaultHidden: inVehicle,
          cell: ({ row }) => (
            <div className="min-w-0">
              <PlateBadge plate={row.original.vehicle.plate} />
              <p className="text-muted-foreground mt-1 truncate text-xs">
                {row.original.vehicle.label}
              </p>
            </div>
          ),
        }),
        createColumn<Transfer>({
          id: "route",
          accessorFn: (row) => `${row.from_branch.code}-${row.to_branch.code}`,
          labelKey: "transfers.columns.route",
          enableSorting: false,
          cell: ({ row }) => (
            <RouteCell
              from={row.original.from_branch}
              to={row.original.to_branch}
            />
          ),
        }),
        createColumn<Transfer>({
          id: "from_branch",
          accessorFn: (row) => row.from_branch.uuid,
          labelKey: "transfers.columns.from_branch",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: inVehicle ? undefined : "faceted",
          filterOptions: referenceOptions(facets?.from_branch),
          cell: ({ row }) => row.original.from_branch.name,
        }),
        createColumn<Transfer>({
          id: "to_branch",
          accessorFn: (row) => row.to_branch.uuid,
          labelKey: "transfers.columns.to_branch",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: inVehicle ? undefined : "faceted",
          filterOptions: referenceOptions(facets?.to_branch),
          cell: ({ row }) => row.original.to_branch.name,
        }),
        createColumn<Transfer>({
          id: "responsible",
          labelKey: "transfers.columns.responsible",
          enableSorting: false,
          cell: ({ row }) => personName(row.original.responsible) ?? "—",
        }),
        createColumn<Transfer>({
          accessorKey: "planned_departure",
          labelKey: "transfers.columns.planned_departure",
          enableSorting: true,
          cell: ({ row }) => (
            <DateTimeCell value={row.original.planned_departure} />
          ),
        }),
        createColumn<Transfer>({
          accessorKey: "planned_arrival",
          labelKey: "transfers.columns.planned_arrival",
          enableSorting: true,
          cell: ({ row }) => (
            <DateTimeCell value={row.original.planned_arrival} />
          ),
        }),
        createColumn<Transfer>({
          accessorKey: "actual_departure",
          labelKey: "transfers.columns.actual_departure",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <DateTimeCell value={row.original.actual_departure} />
          ),
        }),
        createColumn<Transfer>({
          accessorKey: "actual_arrival",
          labelKey: "transfers.columns.actual_arrival",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <DateTimeCell value={row.original.actual_arrival} />
          ),
        }),
        createColumn<Transfer>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "transfers.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: TRANSFER_STATUSES.map((value) => ({
            value,
            labelKey: TRANSFER_STATUS_LABEL_KEYS[value],
            label: inVehicle
              ? t(TRANSFER_STATUS_LABEL_KEYS[value])
              : `${t(TRANSFER_STATUS_LABEL_KEYS[value])} (${countOf(facets?.status, value)})`,
          })),
          cell: ({ row }) => (
            <TransferStatusChip status={row.original.status} />
          ),
        }),
        createColumn<Transfer>({
          accessorKey: "cost",
          labelKey: "transfers.columns.cost",
          enableSorting: true,
          cell: ({ row }) => <MoneyCell value={row.original.cost} />,
        }),
        createColumn<Transfer>({
          accessorKey: "note",
          labelKey: "transfers.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<Transfer>({
          id: "uuid",
          accessorFn: (row) => row.uuid,
          labelKey: "transfers.columns.uuid",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.uuid}
            </code>
          ),
        }),
        createColumn<Transfer>({
          id: "actions",
          labelKey: "transfers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const transfer = row.original;
            const open =
              transfer.status === "planned" || transfer.status === "in_transit";
            return (
              <EntityRowActions
                actions={[
                  ...(inVehicle
                    ? []
                    : [
                        {
                          id: "vehicle",
                          label: t("transfers.actions.view_vehicle"),
                          icon: CarFront,
                          onSelect: () =>
                            router.push(
                              routes.tenant.fleet.vehicles.detail(
                                slug,
                                transfer.vehicle.uuid,
                              ),
                            ),
                        },
                      ]),
                  ...(transfer.status !== "cancelled"
                    ? [
                        {
                          id: "edit",
                          label: t("transfers.actions.edit"),
                          icon: Pencil,
                          permission: write,
                          onSelect: () => setEditTarget(transfer),
                        },
                      ]
                    : []),
                  ...(transfer.status === "planned"
                    ? [
                        {
                          id: "start",
                          label: t("transfers.actions.start"),
                          icon: Play,
                          permission: write,
                          onSelect: () =>
                            setActionTarget({ transfer, action: "start" }),
                        },
                      ]
                    : []),
                  ...(transfer.status === "in_transit"
                    ? [
                        {
                          id: "complete",
                          label: t("transfers.actions.complete"),
                          icon: CircleCheck,
                          permission: write,
                          onSelect: () =>
                            setActionTarget({ transfer, action: "complete" }),
                        },
                      ]
                    : []),
                  ...(open
                    ? [
                        {
                          id: "cancel",
                          label: t("transfers.actions.cancel"),
                          icon: CircleX,
                          permission: write,
                          variant: "destructive" as const,
                          onSelect: () =>
                            setActionTarget({ transfer, action: "cancel" }),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<Transfer, unknown>[],
    [facets, inVehicle, router, slug, t, write],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      from_branch: listParams.from_branch,
      to_branch: listParams.to_branch,
      vehicle: listParams.vehicle,
      sort: listParams.sort,
    }),
    [
      listParams.from_branch,
      listParams.q,
      listParams.sort,
      listParams.status,
      listParams.to_branch,
      listParams.vehicle,
    ],
  );
  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Object.entries(engineQuery).some(
    ([key, value]) => key !== "sort" && key !== "vehicle" && Boolean(value),
  );
  const refresh = () => {
    void listQuery.refetch();
    if (!inVehicle) void facetsQuery.refetch();
  };

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(transfer) => {
          if (transfer.status !== "cancelled") setEditTarget(transfer);
        }}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("transfers.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          inVehicle
            ? listQuery.refetch()
            : Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("transfers.empty_filtered_title")
            : t("transfers.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("transfers.empty_filtered_description")
            : t("transfers.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: inVehicle
            ? "tenant-vehicle-transfers-v1"
            : "tenant-fleet-transfers-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <PermissionGuard permission={write}>
              <Button
                type="button"
                size="sm"
                variant={inVehicle ? "default" : "outline"}
                onClick={() => setCreateOpen(true)}
              >
                <Plus className="size-4" />
                {t("transfers.actions.create")}
              </Button>
            </PermissionGuard>
            {inVehicle ? null : (
              <ResourceIOToolbar
                resource={TRANSFERS_RESOURCE}
                query={engineQuery}
                capabilities={metaQuery.data?.capabilities}
              />
            )}
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <TransferDialog
        open={createOpen}
        vehicleUuid={vehicleUuid}
        onOpenChange={setCreateOpen}
      />
      <TransferDialog
        open={Boolean(editTarget)}
        transfer={editTarget}
        onOpenChange={(open) => {
          if (!open) setEditTarget(null);
        }}
      />
      <TransferActionDialog
        target={actionTarget}
        onOpenChange={(open) => {
          if (!open) setActionTarget(null);
        }}
      />
    </>
  );
}
