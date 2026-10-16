"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { DatePicker } from "@/components/ui/date-picker";
import { DispatchDrawer } from "@/features/notification-engine/components/dispatch-drawer";
import {
  AUDIENCE_LABEL_KEYS,
  CHANNELS,
  CHANNEL_LABEL_KEYS,
  DELIVERY_STATUSES,
  DELIVERY_STATUS_LABEL_KEYS,
  DELIVERY_STATUS_TONES,
  EVENT_LABEL_KEYS,
} from "@/features/notification-engine/constants";
import { useDeliveries } from "@/features/notification-engine/hooks/use-notification-engine";
import type {
  DeliveryListParams,
  NotificationDelivery,
} from "@/features/notification-engine/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/** Istanbul day boundary of a yyyy-MM-dd picker value. */
function dayStart(day: string, offsetDays = 0): string | undefined {
  if (!day) return undefined;
  const d = new Date(`${day}T00:00:00+03:00`);
  d.setUTCDate(d.getUTCDate() + offsetDays);
  return d.toISOString();
}

/** "Gönderim geçmişi": one row per channel attempt. */
export function DeliveriesTable() {
  const { t, locale } = useLocale();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const params = useMemo<DeliveryListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      channel: join(col("channel")),
      status: join(col("status")),
      event: join(col("event")),
      audience: join(col("audience")),
      created_from: dayStart(from),
      created_to: dayStart(to, 1),
    };
  }, [from, listState.columnFilters, listState.params, to]);
  const query = useDeliveries(params);

  const columns = useMemo(
    () =>
      [
        createColumn<NotificationDelivery>({
          accessorKey: "created_at",
          labelKey: "notification_engine.history.columns.created_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<NotificationDelivery>({
          id: "event",
          accessorFn: (row) => row.event,
          labelKey: "notification_engine.history.columns.event",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: Object.entries(EVENT_LABEL_KEYS).map(([v, k]) => ({
            value: v,
            labelKey: k,
            label: t(k),
          })),
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-1">
              <span className="font-medium">
                {t(EVENT_LABEL_KEYS[row.original.event] ?? row.original.event)}
              </span>
              {row.original.test ? (
                <Badge variant="outline">
                  {t("notification_engine.history.test")}
                </Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<NotificationDelivery>({
          id: "recipient",
          accessorFn: (row) => row.recipient_name,
          labelKey: "notification_engine.history.columns.recipient",
          enableSorting: false,
          cell: ({ row }) => (
            <div>
              <p>{row.original.recipient_name || "—"}</p>
              <p className="text-muted-foreground text-xs">
                {row.original.recipient_masked}
              </p>
            </div>
          ),
        }),
        createColumn<NotificationDelivery>({
          accessorKey: "channel",
          labelKey: "notification_engine.history.columns.channel",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CHANNELS.map((c) => ({
            value: c,
            labelKey: CHANNEL_LABEL_KEYS[c],
            label: t(CHANNEL_LABEL_KEYS[c]),
          })),
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {row.original.attempt_no}.{" "}
              {t(CHANNEL_LABEL_KEYS[row.original.channel])}
            </span>
          ),
        }),
        createColumn<NotificationDelivery>({
          accessorKey: "status",
          labelKey: "notification_engine.history.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: DELIVERY_STATUSES.map((s) => ({
            value: s,
            labelKey: DELIVERY_STATUS_LABEL_KEYS[s],
            label: t(DELIVERY_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(DELIVERY_STATUS_LABEL_KEYS[row.original.status])}
              tone={DELIVERY_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<NotificationDelivery>({
          accessorKey: "provider",
          labelKey: "notification_engine.history.columns.provider",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.provider || "—",
        }),
        createColumn<NotificationDelivery>({
          accessorKey: "provider_ref",
          labelKey: "notification_engine.history.columns.provider_ref",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.provider_ref || "—"}
            </span>
          ),
        }),
        createColumn<NotificationDelivery>({
          accessorKey: "subject",
          labelKey: "notification_engine.history.columns.subject",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="block max-w-[16rem] truncate">
              {row.original.subject || "—"}
            </span>
          ),
        }),
        createColumn<NotificationDelivery>({
          accessorKey: "error",
          labelKey: "notification_engine.history.columns.error",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.error ? (
              <span className="text-destructive block max-w-[18rem] truncate text-xs">
                {row.original.error}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<NotificationDelivery>({
          accessorKey: "audience",
          labelKey: "notification_engine.history.columns.audience",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: Object.entries(AUDIENCE_LABEL_KEYS).map(([v, k]) => ({
            value: v,
            labelKey: k,
            label: t(k),
          })),
          cell: ({ row }) => t(AUDIENCE_LABEL_KEYS[row.original.audience]),
        }),
      ] as ColumnDef<NotificationDelivery, unknown>[],
    [locale, t],
  );

  const total = query.data?.total ?? 0;
  const hasFilters = Boolean(
    params.q ||
    params.channel ||
    params.status ||
    params.event ||
    params.audience ||
    from ||
    to,
  );
  const refresh = () => void query.refetch();

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("notification_engine.errors.load")}
        onRetry={refresh}
        onRowClick={(row) => setOpen(row.dispatch_uuid)}
        emptyTitle={
          hasFilters
            ? t("notification_engine.history.empty_filtered")
            : t("notification_engine.history.empty")
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-notification-deliveries-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <div className="grid w-full grid-cols-2 gap-2 sm:w-auto">
              <DatePicker
                value={from}
                onChange={setFrom}
                placeholder={t("notification_engine.history.from")}
              />
              <DatePicker
                value={to}
                onChange={setTo}
                placeholder={t("notification_engine.history.to")}
              />
            </div>
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
      <DispatchDrawer
        uuid={open}
        onOpenChange={(next) => {
          if (!next) setOpen(null);
        }}
      />
    </>
  );
}
