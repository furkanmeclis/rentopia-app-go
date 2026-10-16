"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, CircleCheck, Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { ChannelMappingDrawer } from "@/features/channels/components/channel-mapping-drawer";
import {
  CHANNEL_MAPPINGS_RESOURCE,
  ENTITY_TYPE_LABEL_KEYS,
  ENTITY_TYPES,
} from "@/features/channels/constants";
import {
  useChannelAccounts,
  useChannelMappingFacets,
  useChannelMappings,
  useDeleteChannelMapping,
  useSaveChannelMapping,
} from "@/features/channels/hooks/use-channels";
import type {
  ChannelMapping,
  ListChannelMappingsParams,
} from "@/features/channels/types";
import {
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function internalRecord(m: ChannelMapping) {
  const ref =
    m.branch && m.entity_type === "branch" ? m.branch : (m.product ?? m.extra);
  return ref ? { code: ref.code ?? "", name: ref.name ?? "" } : null;
}

/**
 * Mappings of branches (locations), rental products and extras to the
 * channel's codes, per channel account: DataTable with entity / account
 * facets, live updates, a create / edit drawer, stop-sale toggle and delete.
 */
export function ChannelMappingsTable() {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.channels.write);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CHANNEL_MAPPINGS_RESOURCE),
  });
  const [drawer, setDrawer] = useState<{
    mapping: ChannelMapping | null;
  } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ChannelMapping | null>(null);
  const accounts = useChannelAccounts();
  const facets = useChannelMappingFacets();
  const save = useSaveChannelMapping();
  const remove = useDeleteChannelMapping();

  const listParams = useMemo<ListChannelMappingsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    const account = column("account");
    return {
      ...listState.params,
      entity_type: joinFilter(column("entity_type")),
      account: account.length === 1 ? account[0] : undefined,
    };
  }, [listState.columnFilters, listState.params]);
  const listQuery = useChannelMappings(listParams);
  const total = listQuery.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<ChannelMapping>({
          id: "entity_type",
          accessorFn: (row) => row.entity_type,
          labelKey: "channels.mappings.columns.entity_type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ENTITY_TYPES.map((v) => ({
            value: v,
            labelKey: ENTITY_TYPE_LABEL_KEYS[v],
            label: `${t(ENTITY_TYPE_LABEL_KEYS[v])} (${facets.data?.entity_types[v] ?? 0})`,
          })),
          cell: ({ row }) => (
            <Badge variant="secondary">
              {t(ENTITY_TYPE_LABEL_KEYS[row.original.entity_type])}
            </Badge>
          ),
        }),
        createColumn<ChannelMapping>({
          id: "record",
          accessorFn: (row) => internalRecord(row)?.code ?? "",
          labelKey: "channels.mappings.columns.record",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const r = internalRecord(row.original);
            return r ? (
              <span>
                <code className="text-xs">{r.code}</code> {r.name}
              </span>
            ) : (
              "—"
            );
          },
        }),
        createColumn<ChannelMapping>({
          id: "location",
          accessorFn: (row) =>
            row.entity_type === "product" ? (row.branch?.code ?? "") : "",
          labelKey: "channels.mappings.columns.location",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.entity_type !== "product" ? (
              "—"
            ) : row.original.branch ? (
              <span className="text-sm">{row.original.branch.name}</span>
            ) : (
              <Badge variant="outline">
                {t("channels.mappings.fields.all_locations")}
              </Badge>
            ),
        }),
        createColumn<ChannelMapping>({
          accessorKey: "external_code",
          labelKey: "channels.mappings.columns.external_code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="text-xs font-medium">
              {row.original.external_code}
            </code>
          ),
        }),
        createColumn<ChannelMapping>({
          accessorKey: "external_name",
          labelKey: "channels.mappings.columns.external_name",
          enableSorting: false,
          cell: ({ row }) => row.original.external_name || "—",
        }),
        createColumn<ChannelMapping>({
          id: "account",
          accessorFn: (row) => row.account.uuid,
          labelKey: "channels.mappings.columns.account",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (accounts.data ?? []).map((a) => ({
            value: a.uuid,
            label: a.display_name,
          })),
          cell: ({ row }) => (
            <span className="text-sm">{row.original.account.name}</span>
          ),
        }),
        createColumn<ChannelMapping>({
          id: "sale",
          accessorFn: (row) => (row.stop_sale ? "stop" : "open"),
          labelKey: "channels.mappings.columns.sale",
          enableSorting: false,
          cell: ({ row }) => {
            const m = row.original;
            if (m.entity_type !== "product") return "—";
            return (
              <span className="flex flex-wrap items-center gap-1.5">
                {m.stop_sale ? (
                  <StatusChip
                    label={t("channels.mappings.stop_sale")}
                    tone="danger"
                  />
                ) : (
                  <StatusChip
                    label={t("channels.mappings.on_sale")}
                    tone="success"
                  />
                )}
                {m.max_quota !== null ? (
                  <Badge variant="outline">
                    {t("channels.mappings.quota", { quota: m.max_quota })}
                  </Badge>
                ) : null}
              </span>
            );
          },
        }),
        createColumn<ChannelMapping>({
          accessorKey: "priority",
          labelKey: "channels.mappings.columns.priority",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.entity_type === "product"
              ? row.original.priority
              : "—",
        }),
        createColumn<ChannelMapping>({
          accessorKey: "is_active",
          labelKey: "channels.mappings.columns.is_active",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.is_active ? (
              <StatusChip
                label={t("channels.mappings.active")}
                tone="success"
              />
            ) : (
              <StatusChip label={t("channels.mappings.inactive")} />
            ),
        }),
        createColumn<ChannelMapping>({
          accessorKey: "updated_at",
          labelKey: "channels.mappings.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.updated_at, "d MMM yyyy HH:mm", locale),
        }),
        ...auditColumns<ChannelMapping>(
          {
            createdAt: "channels.mappings.columns.created_at",
            uuid: "channels.mappings.columns.uuid",
          },
          locale,
        ),
        createColumn<ChannelMapping>({
          id: "actions",
          labelKey: "channels.mappings.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const m = row.original;
            if (!canWrite) return null;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("channels.actions.edit"),
                    icon: Pencil,
                    onSelect: () => setDrawer({ mapping: m }),
                  },
                  ...(m.entity_type === "product"
                    ? [
                        {
                          id: "stop-sale",
                          label: m.stop_sale
                            ? t("channels.actions.resume_sale")
                            : t("channels.actions.stop_sale"),
                          icon: m.stop_sale ? CircleCheck : Ban,
                          onSelect: () =>
                            save.mutate({
                              uuid: m.uuid,
                              body: { stop_sale: !m.stop_sale },
                            }),
                        },
                      ]
                    : []),
                  {
                    id: "delete",
                    label: t("channels.actions.delete"),
                    icon: Trash2,
                    variant: "destructive" as const,
                    onSelect: () => setDeleteTarget(m),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<ChannelMapping, unknown>[],
    [accounts.data, canWrite, facets.data, locale, save, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters =
    Boolean(listParams.q) ||
    listState.columnFilters.some((f) => filterValues([f], f.id).length > 0);

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={canWrite ? (row) => setDrawer({ mapping: row }) : undefined}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("channels.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facets.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("channels.mappings.empty_filtered_title")
            : t("channels.mappings.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("channels.mappings.empty_filtered_description")
            : t("channels.mappings.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-channel-mappings-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            {canWrite && (accounts.data?.length ?? 0) > 0 ? (
              <EntityCreateButton
                label={t("channels.mappings.create")}
                onClick={() => setDrawer({ mapping: null })}
                permission={permissions.tenant.channels.write}
              />
            ) : null}
            <EntityToolbar
              onRefresh={() => {
                void listQuery.refetch();
                void facets.refetch();
              }}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <ChannelMappingDrawer
        open={drawer !== null}
        mapping={drawer?.mapping ?? null}
        accounts={accounts.data ?? []}
        onOpenChange={(open) => (!open ? setDrawer(null) : undefined)}
      />
      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.external_code ?? ""}
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </>
  );
}
