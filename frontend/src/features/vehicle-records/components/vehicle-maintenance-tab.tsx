"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CalendarPlus, CheckCircle2, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ResourceIOToolbar } from "@/features/io";
import {
  CompleteMaintenanceDialog,
  MaintenanceDialog,
  type MaintenanceDialogMode,
} from "@/features/vehicle-records/components/maintenance-dialogs";
import {
  DaysLeft,
  ExpiryDate,
  KmValue,
  MaintenanceStatusChip,
} from "@/features/vehicle-records/components/record-cells";
import {
  MAINTENANCE_RESOURCE,
  MAINTENANCE_STATUSES,
  MAINTENANCE_STATUS_LABEL_KEYS,
  MAINTENANCE_TYPES,
  MAINTENANCE_TYPE_LABEL_KEYS,
} from "@/features/vehicle-records/constants";
import {
  useDeleteVehicleMaintenance,
  useVehicleMaintenance,
} from "@/features/vehicle-records/hooks/use-vehicle-records";
import type { VehicleMaintenance } from "@/features/vehicle-records/types";
import { currency, date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Bakım" tab of a vehicle: open plans (due by date or km) and the done
 * history in one DataTable; plan, record, complete, edit and delete through
 * dialogs.
 */
export function VehicleMaintenanceTab({
  vehicleUuid,
}: {
  vehicleUuid: string;
}) {
  const { t, locale } = useLocale();
  const query = useVehicleMaintenance(vehicleUuid);
  const remove = useDeleteVehicleMaintenance(vehicleUuid);
  const [mode, setMode] = useState<MaintenanceDialogMode | null>(null);
  const [completeTarget, setCompleteTarget] =
    useState<VehicleMaintenance | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VehicleMaintenance | null>(
    null,
  );
  const write = permissions.tenant.vehicles.write;

  const columns = useMemo(
    () =>
      [
        createColumn<VehicleMaintenance>({
          id: "maintenance_type",
          accessorFn: (row) => row.maintenance_type,
          labelKey: "vehicle_records.columns.maintenance_type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: MAINTENANCE_TYPES.map((value) => ({
            value,
            labelKey: MAINTENANCE_TYPE_LABEL_KEYS[value],
            label: t(MAINTENANCE_TYPE_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {t(MAINTENANCE_TYPE_LABEL_KEYS[row.original.maintenance_type])}
            </span>
          ),
        }),
        createColumn<VehicleMaintenance>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "vehicle_records.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: MAINTENANCE_STATUSES.map((value) => ({
            value,
            labelKey: MAINTENANCE_STATUS_LABEL_KEYS[value],
            label: t(MAINTENANCE_STATUS_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <MaintenanceStatusChip status={row.original.status} />
          ),
        }),
        createColumn<VehicleMaintenance>({
          id: "planned",
          accessorFn: (row) => row.planned_date ?? "",
          labelKey: "vehicle_records.columns.planned",
          enableSorting: true,
          cell: ({ row }) => {
            const m = row.original;
            if (!m.planned_date && m.planned_km == null) {
              return <span className="text-muted-foreground">—</span>;
            }
            const open = m.kind === "planned";
            return (
              <div className="flex flex-col gap-0.5">
                {m.planned_date ? (
                  open ? (
                    <ExpiryDate value={m.planned_date} horizonDays={15} />
                  ) : (
                    <span className="tabular-nums">
                      {date(m.planned_date, undefined, locale)}
                    </span>
                  )
                ) : null}
                {m.planned_km != null ? <KmValue value={m.planned_km} /> : null}
                {open ? <DaysLeft days={m.days_left} /> : null}
                {open && m.km_left != null ? (
                  <span
                    className={
                      m.km_left <= 0
                        ? "text-destructive text-xs"
                        : "text-muted-foreground text-xs"
                    }
                  >
                    {m.km_left <= 0
                      ? t("vehicle_records.km_overdue", { count: -m.km_left })
                      : t("vehicle_records.km_left", { count: m.km_left })}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<VehicleMaintenance>({
          id: "done",
          accessorFn: (row) => row.done_date ?? "",
          labelKey: "vehicle_records.columns.done",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.done_date ? (
              <div className="flex flex-col gap-0.5">
                <span className="tabular-nums">
                  {date(row.original.done_date, undefined, locale)}
                </span>
                {row.original.done_km != null ? (
                  <KmValue value={row.original.done_km} />
                ) : null}
              </div>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<VehicleMaintenance>({
          accessorKey: "service_provider",
          labelKey: "vehicle_records.columns.service_provider",
          enableSorting: true,
          cell: ({ row }) => row.original.service_provider || "—",
        }),
        createColumn<VehicleMaintenance>({
          id: "cost",
          accessorFn: (row) => (row.cost ? Number(row.cost) : -1),
          labelKey: "vehicle_records.columns.cost",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.cost ? (
              <span className="tabular-nums">
                {currency(
                  Number(row.original.cost),
                  row.original.currency,
                  locale,
                )}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<VehicleMaintenance>({
          accessorKey: "note",
          labelKey: "vehicle_records.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<VehicleMaintenance>({
          id: "actions",
          labelKey: "vehicle_records.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const m = row.original;
            return (
              <EntityRowActions
                actions={[
                  ...(m.kind === "planned"
                    ? [
                        {
                          id: "complete",
                          label: t("vehicle_records.maintenance.complete"),
                          icon: CheckCircle2,
                          permission: write,
                          onSelect: () => setCompleteTarget(m),
                        },
                      ]
                    : []),
                  {
                    id: "edit",
                    label: t("vehicle_records.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () => setMode({ kind: m.kind, item: m }),
                  },
                  {
                    id: "delete",
                    label: t("vehicle_records.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(m),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<VehicleMaintenance, unknown>[],
    [locale, t, write],
  );

  const items = query.data?.items ?? [];

  return (
    <EntitySectionCard
      title={t("vehicle_records.maintenance.title")}
      badge={query.data ? items.length : undefined}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("vehicle_records.maintenance.help")}
      </p>
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("vehicle_records.maintenance.empty_title")}
        emptyDescription={t("vehicle_records.maintenance.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-vehicle-maintenance-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={MAINTENANCE_RESOURCE}
              query={{ vehicle: vehicleUuid }}
              capabilities={{ export: true }}
            />
            <PermissionGuard permission={write}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setMode({ kind: "done", item: null })}
              >
                <Plus className="size-4" />
                {t("vehicle_records.maintenance.record")}
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => setMode({ kind: "planned", item: null })}
              >
                <CalendarPlus className="size-4" />
                {t("vehicle_records.maintenance.plan")}
              </Button>
            </PermissionGuard>
          </>
        }
      />

      <MaintenanceDialog
        vehicleUuid={vehicleUuid}
        mode={mode}
        onOpenChange={(open) => {
          if (!open) setMode(null);
        }}
      />

      <CompleteMaintenanceDialog
        vehicleUuid={vehicleUuid}
        item={completeTarget}
        onOpenChange={(open) => {
          if (!open) setCompleteTarget(null);
        }}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t("vehicle_records.maintenance.delete_title")}
        description={
          deleteTarget
            ? t("vehicle_records.maintenance.delete_description", {
                type: t(
                  MAINTENANCE_TYPE_LABEL_KEYS[deleteTarget.maintenance_type],
                ),
              })
            : undefined
        }
        confirmLabel={t("vehicle_records.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntitySectionCard>
  );
}
