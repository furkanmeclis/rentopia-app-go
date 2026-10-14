"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  KaskoValue,
  Km,
  PlateBadge,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import {
  useProductVehicles,
  useVehiclesList,
} from "@/features/vehicles/hooks/use-vehicles";
import type { Vehicle } from "@/features/vehicles/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function useVehicleColumns() {
  return useMemo(
    () =>
      [
        createColumn<Vehicle>({
          accessorKey: "plate",
          labelKey: "vehicles.columns.plate",
          enableSorting: false,
          cell: ({ row }) => <PlateBadge plate={row.original.plate} />,
        }),
        createColumn<Vehicle>({
          id: "vehicle",
          accessorFn: (row) => row.label,
          labelKey: "vehicles.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.label || "—"}</span>
          ),
        }),
        createColumn<Vehicle>({
          accessorKey: "model_year",
          labelKey: "vehicles.columns.model_year",
          enableSorting: false,
        }),
        createColumn<Vehicle>({
          accessorKey: "odometer_km",
          labelKey: "vehicles.columns.odometer_km",
          enableSorting: false,
          cell: ({ row }) => <Km value={row.original.odometer_km} />,
        }),
        createColumn<Vehicle>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "vehicles.columns.status",
          enableSorting: false,
          cell: ({ row }) => <VehicleStatusChip status={row.original.status} />,
        }),
        createColumn<Vehicle>({
          id: "current_branch",
          accessorFn: (row) => row.current_branch.name,
          labelKey: "vehicles.columns.current_branch",
          enableSorting: false,
        }),
        createColumn<Vehicle>({
          accessorKey: "kasko_value",
          labelKey: "vehicles.columns.kasko_value",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <KaskoValue
              value={row.original.kasko_value}
              period={row.original.kasko_period}
            />
          ),
        }),
      ] as ColumnDef<Vehicle, unknown>[],
    [],
  );
}

/** Shown instead of a vehicle sub-list to members without vehicle access. */
function NoVehicleAccess() {
  const { t } = useLocale();
  return (
    <EmptyState
      className="py-8"
      title={t("vehicles.sub.no_access_title")}
      description={t("vehicles.sub.no_access_description")}
    />
  );
}

/**
 * Vehicles that keep the promise of a product's current version
 * (`SatisfiesGuarantee`), with their count. Capacity is never stored: it is
 * computed from these vehicles.
 */
export function ProductVehiclesTable({ productUuid }: { productUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.vehicles.read);
  const listState = useServerListState({ initialPageSize: 10 });
  const params = useMemo(
    () => ({ limit: listState.params.limit, offset: listState.params.offset }),
    [listState.params.limit, listState.params.offset],
  );
  const query = useProductVehicles(productUuid, params, allowed);
  const columns = useVehicleColumns();
  if (!allowed) return <NoVehicleAccess />;
  const total = query.data?.total ?? 0;

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <Badge variant="secondary" className="text-base tabular-nums">
          {total}
        </Badge>
        <span className="text-muted-foreground text-sm">
          {t("vehicles.sub.eligible_count")}
        </span>
      </div>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(vehicle) =>
          router.push(routes.tenant.fleet.vehicles.detail(slug, vehicle.uuid))
        }
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("vehicles.sub.eligible_empty_title")}
        emptyDescription={t("vehicles.sub.eligible_empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / listState.pagination.pageSize),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-fleet-product-vehicles-v1",
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
        }}
      />
    </div>
  );
}

/** Member vehicles of a rental group (server-side DataTable sub-list). */
export function GroupVehiclesTable({ groupUuid }: { groupUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.vehicles.read);
  const listState = useServerListState({
    initialSort: "plate",
    initialPageSize: 10,
  });
  const params = useMemo(
    () => ({ ...listState.params, rental_group: groupUuid }),
    [groupUuid, listState.params],
  );
  const query = useVehiclesList(params, allowed);
  const columns = useVehicleColumns();
  if (!allowed) return <NoVehicleAccess />;
  const total = query.data?.total ?? 0;

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(vehicle) =>
        router.push(routes.tenant.fleet.vehicles.detail(slug, vehicle.uuid))
      }
      isLoading={query.isLoading}
      isError={query.isError}
      onRetry={() => void query.refetch()}
      emptyTitle={t("vehicles.sub.group_empty_title")}
      emptyDescription={t("vehicles.sub.group_empty_description")}
      pageCount={Math.max(1, Math.ceil(total / listState.pagination.pageSize))}
      state={listState.tableState}
      features={{
        persistKey: "tenant-fleet-group-vehicles-v1",
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
      }}
    />
  );
}
