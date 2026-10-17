"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  RunStatusChip,
  runCountsText,
} from "@/features/channels/components/channel-cells";
import {
  CHANNEL_ACCOUNTS_RESOURCE,
  RUN_DIRECTION_LABEL_KEYS,
  RUN_DIRECTIONS,
  RUN_STATUS_LABEL_KEYS,
  RUN_STATUSES,
  RUN_TRIGGER_LABEL_KEYS,
} from "@/features/channels/constants";
import {
  useChannelAccounts,
  useChannelSyncRuns,
} from "@/features/channels/hooks/use-channels";
import type {
  ChannelSyncRun,
  ListChannelSyncRunsParams,
} from "@/features/channels/types";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** The sync log: every push, pull and reconciliation with its result. */
export function ChannelSyncRunsTable() {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const accounts = useChannelAccounts();
  const listState = useServerListState({
    initialSort: "-started_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CHANNEL_ACCOUNTS_RESOURCE),
  });
  const listParams = useMemo<ListChannelSyncRunsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    const account = column("account");
    return {
      limit: listState.params.limit,
      offset: listState.params.offset,
      sort: listState.params.sort,
      direction: joinFilter(column("direction")),
      status: joinFilter(column("status")),
      account: account.length === 1 ? account[0] : undefined,
    };
  }, [listState.columnFilters, listState.params]);
  const listQuery = useChannelSyncRuns(listParams);
  const total = listQuery.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<ChannelSyncRun>({
          accessorKey: "started_at",
          labelKey: "channels.runs.columns.started_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-sm whitespace-nowrap tabular-nums">
              {datetime(row.original.started_at, "d MMM yyyy HH:mm:ss", locale)}
            </span>
          ),
        }),
        createColumn<ChannelSyncRun>({
          accessorKey: "direction",
          labelKey: "channels.runs.columns.direction",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: RUN_DIRECTIONS.map((v) => ({
            value: v,
            labelKey: RUN_DIRECTION_LABEL_KEYS[v],
            label: t(RUN_DIRECTION_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {t(RUN_DIRECTION_LABEL_KEYS[row.original.direction])}
            </span>
          ),
        }),
        createColumn<ChannelSyncRun>({
          accessorKey: "trigger",
          labelKey: "channels.runs.columns.trigger",
          enableSorting: false,
          cell: ({ row }) => t(RUN_TRIGGER_LABEL_KEYS[row.original.trigger]),
        }),
        createColumn<ChannelSyncRun>({
          accessorKey: "status",
          labelKey: "channels.runs.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: RUN_STATUSES.map((v) => ({
            value: v,
            labelKey: RUN_STATUS_LABEL_KEYS[v],
            label: t(RUN_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => <RunStatusChip status={row.original.status} />,
        }),
        createColumn<ChannelSyncRun>({
          id: "counts",
          accessorFn: (row) => row.items_total,
          labelKey: "channels.runs.columns.counts",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-sm tabular-nums">
              {runCountsText(t, row.original)}
            </span>
          ),
        }),
        createColumn<ChannelSyncRun>({
          id: "account",
          accessorFn: (row) => row.account.uuid,
          labelKey: "channels.runs.columns.account",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (accounts.data ?? []).map((a) => ({
            value: a.uuid,
            label: a.display_name,
          })),
          cell: ({ row }) => row.original.account.name,
        }),
        createColumn<ChannelSyncRun>({
          accessorKey: "error",
          labelKey: "channels.runs.columns.error",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground line-clamp-2 text-xs break-words">
              {row.original.error ||
                (typeof row.original.details.availability_error === "string"
                  ? row.original.details.availability_error
                  : "—")}
            </span>
          ),
        }),
        createColumn<ChannelSyncRun>({
          accessorKey: "actor",
          labelKey: "channels.runs.columns.actor",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.actor || t("channels.runs.system"),
        }),
        createColumn<ChannelSyncRun>({
          accessorKey: "finished_at",
          labelKey: "channels.runs.columns.finished_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.finished_at
              ? datetime(row.original.finished_at, "d MMM HH:mm:ss", locale)
              : "—",
        }),
      ] as ColumnDef<ChannelSyncRun, unknown>[],
    [accounts.data, locale, t],
  );
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  return (
    <EntityTable
      columns={columns}
      data={listQuery.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={listQuery.isLoading}
      isError={listQuery.isError}
      errorDescription={t("channels.load_failed")}
      onRetry={() => void listQuery.refetch()}
      realtime={listState.realtimeFor(() => listQuery.refetch())}
      emptyTitle={t("channels.runs.empty_title")}
      emptyDescription={t("channels.runs.empty_description")}
      pageCount={pageCount}
      state={listState.tableState}
      features={{
        persistKey: "tenant-channel-runs-v1",
        rowSelection: false,
        globalFilter: false,
      }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void listQuery.refetch()}
          refreshDisabled={listQuery.isFetching}
        />
      }
    />
  );
}
