"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ListPlus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  KaskoValue,
  Km,
  PlateBadge,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import { REASON_LABEL_KEYS } from "@/features/vehicles/constants";
import {
  useAddEligibleMembers,
  useGroupMembers,
  useProductVehicles,
} from "@/features/vehicles/hooks/use-vehicles";
import type { RentalGroupMember, Vehicle } from "@/features/vehicles/types";
import { datetime } from "@/lib/utils/format";
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

function useMemberColumns() {
  const { t, locale } = useLocale();
  return useMemo(
    () =>
      [
        createColumn<RentalGroupMember>({
          id: "plate",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "vehicles.columns.plate",
          enableSorting: false,
          cell: ({ row }) => <PlateBadge plate={row.original.vehicle.plate} />,
        }),
        createColumn<RentalGroupMember>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle.label,
          labelKey: "vehicles.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.vehicle.label || "—"}
            </span>
          ),
        }),
        createColumn<RentalGroupMember>({
          id: "eligible",
          accessorFn: (row) => String(row.eligible),
          labelKey: "vehicles.groups.columns.eligible",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.eligible ? (
              <StatusChip
                label={t("vehicles.groups.eligible")}
                tone="success"
              />
            ) : (
              <div className="space-y-1">
                <StatusChip
                  label={t("vehicles.groups.not_eligible_badge")}
                  tone="danger"
                />
                <p className="text-muted-foreground text-xs">
                  {row.original.reasons
                    .map((reason) =>
                      REASON_LABEL_KEYS[reason]
                        ? t(REASON_LABEL_KEYS[reason])
                        : reason,
                    )
                    .join(", ")}
                </p>
              </div>
            ),
        }),
        createColumn<RentalGroupMember>({
          id: "status",
          accessorFn: (row) => row.vehicle.status,
          labelKey: "vehicles.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <VehicleStatusChip status={row.original.vehicle.status} />
          ),
        }),
        createColumn<RentalGroupMember>({
          id: "current_branch",
          accessorFn: (row) => row.vehicle.current_branch.name,
          labelKey: "vehicles.columns.current_branch",
          enableSorting: false,
        }),
        createColumn<RentalGroupMember>({
          id: "added_at",
          accessorFn: (row) => row.added_at,
          labelKey: "vehicles.groups.columns.added_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => datetime(row.original.added_at, undefined, locale),
        }),
      ] as ColumnDef<RentalGroupMember, unknown>[],
    [locale, t],
  );
}

/**
 * Member vehicles of a rental group (server-side DataTable sub-list) with
 * their current eligibility. Membership is explicit: only members that
 * still meet the criteria count for the group's products. "Add all
 * eligible vehicles" puts every matching active vehicle in the group.
 */
export function GroupVehiclesTable({ groupUuid }: { groupUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.vehicles.read);
  const canWrite = can(permissions.tenant.vehicles.write);
  const [confirmAdd, setConfirmAdd] = useState(false);
  const listState = useServerListState({ initialPageSize: 10 });
  const params = useMemo(
    () => ({ limit: listState.params.limit, offset: listState.params.offset }),
    [listState.params.limit, listState.params.offset],
  );
  const query = useGroupMembers(groupUuid, params, allowed);
  const addEligible = useAddEligibleMembers();
  const columns = useMemberColumns();
  if (!allowed) return <NoVehicleAccess />;
  const total = query.data?.total ?? 0;
  const counting = query.data?.eligible_total ?? 0;
  const candidates = query.data?.candidates_total ?? 0;

  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary" className="text-base tabular-nums">
            {counting}
          </Badge>
          <span className="text-muted-foreground text-sm">
            {t("vehicles.sub.group_counting", { total })}
          </span>
        </div>
        {canWrite && candidates > 0 ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={addEligible.isPending}
            onClick={() => setConfirmAdd(true)}
          >
            <ListPlus className="size-4" />
            {t("vehicles.sub.add_eligible", { count: candidates })}
          </Button>
        ) : null}
      </div>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.vehicle.uuid}
        onRowClick={(member) =>
          router.push(
            routes.tenant.fleet.vehicles.detail(slug, member.vehicle.uuid),
          )
        }
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("vehicles.sub.group_empty_title")}
        emptyDescription={t("vehicles.sub.group_empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / listState.pagination.pageSize),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-fleet-group-members-v1",
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
        }}
      />
      <ConfirmDialog
        open={confirmAdd}
        title={t("vehicles.sub.add_eligible_title")}
        description={t("vehicles.sub.add_eligible_description", {
          count: candidates,
        })}
        confirmLabel={t("vehicles.sub.add_eligible_confirm")}
        isPending={addEligible.isPending}
        onCancel={() => setConfirmAdd(false)}
        onConfirm={() =>
          addEligible.mutate(groupUuid, {
            onSettled: () => setConfirmAdd(false),
          })
        }
      />
    </div>
  );
}
