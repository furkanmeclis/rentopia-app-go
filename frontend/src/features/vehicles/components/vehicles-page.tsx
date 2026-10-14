"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Activity,
  Eye,
  Pencil,
  PencilLine,
  Power,
  PowerOff,
  Trash2,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import {
  activeColumn,
  activeFilter,
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useVehicleBulkParams } from "@/features/vehicles/components/vehicle-bulk-params-dialog";
import {
  GroupChips,
  KaskoValue,
  Km,
  PlateBadge,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import { VehicleQuickEditDrawer } from "@/features/vehicles/components/vehicle-quick-edit-drawer";
import { VehicleStatusDialog } from "@/features/vehicles/components/vehicle-status-dialog";
import {
  OWNERSHIPS,
  OWNERSHIP_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  VEHICLES_RESOURCE,
  VEHICLE_STATUSES,
} from "@/features/vehicles/constants";
import {
  useDeleteVehicle,
  useUpdateVehicle,
  useVehicleFacets,
  useVehiclesList,
  useVehiclesMeta,
} from "@/features/vehicles/hooks/use-vehicles";
import type {
  ListVehiclesParams,
  Vehicle,
  VehicleFacets,
} from "@/features/vehicles/types";
import { useLocale } from "@/providers/locale-provider";

type FacetList = VehicleFacets[keyof VehicleFacets];

function countOf(facets: FacetList | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

function referenceOptions(facets: FacetList | undefined) {
  return (facets ?? []).map((facet) => ({
    value: facet.value,
    label: `${facet.label ?? facet.value} (${facet.count})`,
  }));
}

/** Card of the grid view. */
function VehicleCard({ vehicle }: { vehicle: Vehicle }) {
  const { t } = useLocale();
  return (
    <div className="space-y-3">
      <div className="flex items-start justify-between gap-2">
        <PlateBadge plate={vehicle.plate} />
        <VehicleStatusChip status={vehicle.status} />
      </div>
      <div>
        <p className="font-medium">{vehicle.label || "—"}</p>
        <p className="text-muted-foreground text-sm">
          {vehicle.model_year} · {vehicle.color || t("vehicles.no_color")}
        </p>
      </div>
      <div className="text-muted-foreground grid grid-cols-2 gap-2 text-sm">
        <Km value={vehicle.odometer_km} />
        <span className="truncate text-end">{vehicle.current_branch.name}</span>
      </div>
      <div className="flex items-center justify-between gap-2 text-sm">
        <GroupChips vehicle={vehicle} />
        <KaskoValue value={vehicle.kasko_value} period={vehicle.kasko_period} />
      </div>
    </div>
  );
}

/**
 * Fleet vehicles: the full tenant list standard (faceted filters, grid view,
 * saved columns, export / İdes import, bulk actions with params, live
 * updates).
 */
export function VehiclesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, VEHICLES_RESOURCE),
  });
  const [quickEdit, setQuickEdit] = useState<Vehicle | null>(null);
  const [statusTarget, setStatusTarget] = useState<Vehicle | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Vehicle | null>(null);
  const bulkParams = useVehicleBulkParams();

  const listParams = useMemo<ListVehiclesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      rental_group: joinFilter(column("rental_group")),
      segment: joinFilter(column("segment")),
      home_branch: joinFilter(column("home_branch")),
      current_branch: joinFilter(column("current_branch")),
      ownership: joinFilter(column("ownership")),
      pool: joinFilter(column("pool")),
      is_active: activeFilter(column("is_active")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useVehiclesList(listParams);
  const metaQuery = useVehiclesMeta();
  const facetsQuery = useVehicleFacets();
  const update = useUpdateVehicle();
  const remove = useDeleteVehicle();
  const write = permissions.tenant.vehicles.write;
  const facets = facetsQuery.data;

  const openDetail = useCallback(
    (vehicle: Vehicle) =>
      router.push(routes.tenant.fleet.vehicles.detail(slug, vehicle.uuid)),
    [router, slug],
  );

  const baseColumns = useMemo(
    () =>
      [
        createColumn<Vehicle>({
          accessorKey: "plate",
          labelKey: "vehicles.columns.plate",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => <PlateBadge plate={row.original.plate} />,
        }),
        createColumn<Vehicle>({
          id: "vehicle",
          accessorFn: (row) => row.label,
          labelKey: "vehicles.columns.vehicle",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="truncate font-medium">
                {row.original.brand} {row.original.model}
              </p>
              <p className="text-muted-foreground truncate text-xs">
                {row.original.type_name ||
                  (row.original.manual_entry ? t("vehicles.manual_badge") : "")}
              </p>
            </div>
          ),
        }),
        createColumn<Vehicle>({
          accessorKey: "model_year",
          labelKey: "vehicles.columns.model_year",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.model_year}</span>
          ),
        }),
        createColumn<Vehicle>({
          accessorKey: "odometer_km",
          labelKey: "vehicles.columns.odometer_km",
          enableSorting: true,
          cell: ({ row }) => <Km value={row.original.odometer_km} />,
        }),
        createColumn<Vehicle>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "vehicles.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: VEHICLE_STATUSES.map((value) => ({
            value,
            labelKey: STATUS_LABEL_KEYS[value],
            label: `${t(STATUS_LABEL_KEYS[value])} (${countOf(facets?.status, value)})`,
          })),
          cell: ({ row }) => <VehicleStatusChip status={row.original.status} />,
        }),
        createColumn<Vehicle>({
          id: "current_branch",
          accessorFn: (row) => row.current_branch.uuid,
          labelKey: "vehicles.columns.current_branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.current_branch),
          cell: ({ row }) => row.original.current_branch.name,
        }),
        createColumn<Vehicle>({
          id: "home_branch",
          accessorFn: (row) => row.home_branch.uuid,
          labelKey: "vehicles.columns.home_branch",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.home_branch),
          cell: ({ row }) => row.original.home_branch.name,
        }),
        createColumn<Vehicle>({
          accessorKey: "kasko_value",
          labelKey: "vehicles.columns.kasko_value",
          enableSorting: true,
          cell: ({ row }) => (
            <KaskoValue
              value={row.original.kasko_value}
              period={row.original.kasko_period}
            />
          ),
        }),
        createColumn<Vehicle>({
          id: "rental_group",
          accessorFn: (row) => row.rental_groups.map((g) => g.uuid).join(","),
          labelKey: "vehicles.columns.rental_group",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.rental_group),
          cell: ({ row }) => <GroupChips vehicle={row.original} />,
        }),
        createColumn<Vehicle>({
          id: "segment",
          accessorFn: (row) =>
            row.rental_groups.map((g) => g.segment.uuid).join(","),
          labelKey: "vehicles.columns.segment",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.segment),
          cell: ({ row }) =>
            Array.from(
              new Set(row.original.rental_groups.map((g) => g.segment.name)),
            ).join(", ") || "—",
        }),
        createColumn<Vehicle>({
          id: "ownership",
          accessorFn: (row) => row.ownership,
          labelKey: "vehicles.columns.ownership",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: OWNERSHIPS.map((value) => ({
            value,
            labelKey: OWNERSHIP_LABEL_KEYS[value],
            label: `${t(OWNERSHIP_LABEL_KEYS[value])} (${countOf(facets?.ownership, value)})`,
          })),
          cell: ({ row }) =>
            row.original.ownership === "supplied"
              ? `${t(OWNERSHIP_LABEL_KEYS.supplied)} · ${row.original.supplier_name}`
              : t(OWNERSHIP_LABEL_KEYS.owned),
        }),
        createColumn<Vehicle>({
          id: "pool",
          accessorFn: (row) => row.fleet_pool,
          labelKey: "vehicles.columns.pool",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.pool),
          cell: ({ row }) => (
            <code className="text-xs">{row.original.fleet_pool}</code>
          ),
        }),
        createColumn<Vehicle>({
          accessorKey: "vin",
          labelKey: "vehicles.columns.vin",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.vin ? (
              <code className="text-xs">{row.original.vin}</code>
            ) : (
              "—"
            ),
        }),
        createColumn<Vehicle>({
          accessorKey: "color",
          labelKey: "vehicles.columns.color",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.color || "—",
        }),
        activeColumn<Vehicle>(t, "vehicles.columns.is_active"),
        ...auditColumns<Vehicle>(
          {
            createdAt: "vehicles.columns.created_at",
            uuid: "vehicles.columns.uuid",
          },
          locale,
        ),
        createColumn<Vehicle>({
          id: "actions",
          labelKey: "vehicles.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const vehicle = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "view",
                    label: t("vehicles.actions.view"),
                    icon: Eye,
                    onSelect: () => openDetail(vehicle),
                  },
                  {
                    id: "status",
                    label: t("vehicles.actions.change_status"),
                    icon: Activity,
                    permission: write,
                    onSelect: () => setStatusTarget(vehicle),
                  },
                  {
                    id: "quick-edit",
                    label: t("vehicles.actions.quick_edit"),
                    icon: PencilLine,
                    permission: write,
                    onSelect: () => setQuickEdit(vehicle),
                  },
                  {
                    id: "edit",
                    label: t("vehicles.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () =>
                      router.push(
                        routes.tenant.fleet.vehicles.edit(slug, vehicle.uuid),
                      ),
                  },
                  {
                    id: "toggle-active",
                    label: vehicle.is_active
                      ? t("vehicles.actions.deactivate")
                      : t("vehicles.actions.activate"),
                    icon: vehicle.is_active ? PowerOff : Power,
                    permission: write,
                    onSelect: () =>
                      update.mutate({
                        uuid: vehicle.uuid,
                        body: { is_active: !vehicle.is_active },
                      }),
                  },
                  {
                    id: "delete",
                    label: t("vehicles.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(vehicle),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<Vehicle, unknown>[],
    [facets, locale, openDetail, router, slug, t, update, write],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<Vehicle>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export and the "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      rental_group: listParams.rental_group,
      segment: listParams.segment,
      home_branch: listParams.home_branch,
      current_branch: listParams.current_branch,
      ownership: listParams.ownership,
      pool: listParams.pool,
      is_active: listParams.is_active,
      sort: listParams.sort,
    }),
    [
      listParams.current_branch,
      listParams.home_branch,
      listParams.is_active,
      listParams.ownership,
      listParams.pool,
      listParams.q,
      listParams.rental_group,
      listParams.segment,
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
      resolveBulkActionsWithIcons(VEHICLES_RESOURCE, meta?.bulk_actions ?? []),
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
      title={t("vehicles.title")}
      description={t("vehicles.description")}
      permission={permissions.tenant.vehicles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("vehicles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleet.title") },
        { label: t("vehicles.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.fleet.vehicles.create(slug))}
          label={t("vehicles.actions.create")}
          permission={write}
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
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("vehicles.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("vehicles.empty_filtered_title")
            : t("vehicles.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("vehicles.empty_filtered_description")
            : t("vehicles.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        renderGridItem={(vehicle) => <VehicleCard vehicle={vehicle} />}
        features={{
          persistKey: "tenant-fleet-vehicles-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={VEHICLES_RESOURCE}
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
              resource={VEHICLES_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
              onImportComplete={refresh}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      {bulkParams.dialog}

      <VehicleQuickEditDrawer
        vehicle={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <VehicleStatusDialog
        vehicle={statusTarget}
        onOpenChange={(open) => {
          if (!open) setStatusTarget(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget ? `${deleteTarget.plate} · ${deleteTarget.label}` : ""
        }
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
