"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Hammer, Pencil, Plus, Trash2, Unlock } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityDeleteDialog,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  BlockDialog,
  type BlockDialogMode,
  ReleaseBlockDialog,
} from "@/features/fleet-ops/components/block-dialogs";
import {
  BlockKindBadge,
  BlockStateChip,
  RangeCell,
} from "@/features/fleet-ops/components/fleet-ops-cells";
import {
  BLOCK_KINDS,
  BLOCK_KIND_LABEL_KEYS,
  BLOCK_STATES,
  BLOCK_STATE_LABEL_KEYS,
  VEHICLE_BLOCKS_RESOURCE,
} from "@/features/fleet-ops/constants";
import {
  useDeleteBlock,
  useVehicleBlocks,
} from "@/features/fleet-ops/hooks/use-fleet-ops";
import type { VehicleBlock } from "@/features/fleet-ops/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function actorName(actor: VehicleBlock["created_by"]) {
  if (!actor) return null;
  return `${actor.name} ${actor.surname}`.trim() || actor.email;
}

/**
 * "Engeller" tab of the vehicle page: usage blocks (maintenance, damage,
 * cleaning, …) as a DataTable with "Bakıma al", new block, edit, "Engeli
 * kapat" and delete.
 */
export function VehicleBlocksTab({ vehicleUuid }: { vehicleUuid: string }) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.blocks.read);
  const write = permissions.tenant.blocks.write;
  const listState = useServerListState({
    initialPageSize: 10,
    realtime: tenantLiveList(organization, VEHICLE_BLOCKS_RESOURCE),
  });
  const [dialog, setDialog] = useState<BlockDialogMode | null>(null);
  const [releaseTarget, setReleaseTarget] = useState<VehicleBlock | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VehicleBlock | null>(null);
  const remove = useDeleteBlock(vehicleUuid);

  const params = useMemo(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
      state: joinFilter(filterValues(listState.columnFilters, "state")),
      kind: joinFilter(filterValues(listState.columnFilters, "kind")),
    }),
    [listState.columnFilters, listState.params.limit, listState.params.offset],
  );
  const query = useVehicleBlocks(vehicleUuid, params, allowed);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<VehicleBlock>({
          id: "kind",
          accessorFn: (row) => row.kind,
          labelKey: "vehicle_blocks.columns.kind",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: BLOCK_KINDS.map((value) => ({
            value,
            labelKey: BLOCK_KIND_LABEL_KEYS[value],
            label: t(BLOCK_KIND_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => <BlockKindBadge kind={row.original.kind} />,
        }),
        createColumn<VehicleBlock>({
          id: "state",
          accessorFn: (row) => row.state,
          labelKey: "vehicle_blocks.columns.state",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: BLOCK_STATES.map((value) => ({
            value,
            labelKey: BLOCK_STATE_LABEL_KEYS[value],
            label: t(BLOCK_STATE_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => <BlockStateChip state={row.original.state} />,
        }),
        createColumn<VehicleBlock>({
          id: "period",
          accessorFn: (row) => row.starts_at,
          labelKey: "vehicle_blocks.columns.period",
          enableSorting: false,
          cell: ({ row }) => (
            <RangeCell
              start={row.original.starts_at}
              end={row.original.ends_at}
            />
          ),
        }),
        createColumn<VehicleBlock>({
          accessorKey: "reason",
          labelKey: "vehicle_blocks.columns.reason",
          enableSorting: false,
          cell: ({ row }) => row.original.reason || "—",
        }),
        createColumn<VehicleBlock>({
          id: "created_by",
          labelKey: "vehicle_blocks.columns.created_by",
          enableSorting: false,
          cell: ({ row }) => actorName(row.original.created_by) ?? "—",
        }),
        createColumn<VehicleBlock>({
          id: "released_by",
          labelKey: "vehicle_blocks.columns.released_by",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => actorName(row.original.released_by) ?? "—",
        }),
        createColumn<VehicleBlock>({
          id: "actions",
          labelKey: "vehicle_blocks.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const block = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("vehicle_blocks.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () => setDialog({ type: "edit", block }),
                  },
                  ...(block.state === "active"
                    ? [
                        {
                          id: "release",
                          label: t("vehicle_blocks.actions.release"),
                          icon: Unlock,
                          permission: write,
                          onSelect: () => setReleaseTarget(block),
                        },
                      ]
                    : []),
                  {
                    id: "delete",
                    label: t("vehicle_blocks.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive" as const,
                    onSelect: () => setDeleteTarget(block),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<VehicleBlock, unknown>[],
    [t, write],
  );

  if (!allowed) {
    return (
      <EmptyState
        className="py-8"
        title={t("vehicle_blocks.no_access_title")}
        description={t("vehicle_blocks.no_access_description")}
      />
    );
  }
  const hasFilters = Boolean(params.state || params.kind);

  return (
    <EntitySectionCard title={t("vehicle_blocks.title")} badge={total}>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("vehicle_blocks.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={
          hasFilters
            ? t("vehicle_blocks.empty_filtered_title")
            : t("vehicle_blocks.empty_title")
        }
        emptyDescription={t("vehicle_blocks.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / listState.pagination.pageSize),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-vehicle-blocks-v1",
          rowSelection: false,
          globalFilter: false,
        }}
        toolbarExtra={
          <PermissionGuard permission={write}>
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog({ type: "maintenance" })}
              >
                <Hammer className="size-4" />
                {t("vehicle_blocks.actions.start_maintenance")}
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog({ type: "create" })}
              >
                <Plus className="size-4" />
                {t("vehicle_blocks.actions.create")}
              </Button>
            </div>
          </PermissionGuard>
        }
      />

      <BlockDialog
        vehicleUuid={vehicleUuid}
        mode={dialog}
        onOpenChange={(open) => {
          if (!open) setDialog(null);
        }}
      />
      <ReleaseBlockDialog
        vehicleUuid={vehicleUuid}
        block={releaseTarget}
        onOpenChange={(open) => {
          if (!open) setReleaseTarget(null);
        }}
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget ? t(BLOCK_KIND_LABEL_KEYS[deleteTarget.kind]) : ""
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
    </EntitySectionCard>
  );
}
