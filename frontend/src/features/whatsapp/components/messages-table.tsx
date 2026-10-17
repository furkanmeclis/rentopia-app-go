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
import { EVENT_LABEL_KEYS } from "@/features/notification-engine/constants";
import {
  MESSAGE_STATUSES,
  MESSAGE_STATUS_LABEL_KEYS,
  MESSAGE_STATUS_TONES,
} from "@/features/whatsapp/constants";
import { useWhatsAppMessages } from "@/features/whatsapp/hooks/use-whatsapp";
import type {
  MessageListParams,
  WhatsAppMessage,
} from "@/features/whatsapp/types";
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

/** "Mesaj kaydı": one row per WhatsApp message with its receipts. */
export function MessagesTable() {
  const { t, locale } = useLocale();
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const params = useMemo<MessageListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      status: join(col("status")),
      created_from: dayStart(from),
      created_to: dayStart(to, 1),
    };
  }, [from, listState.columnFilters, listState.params, to]);
  const query = useWhatsAppMessages(params);

  const columns = useMemo(() => {
    const purposeLabel = (purpose: string) => {
      if (purpose === "test") return t("whatsapp.messages.purpose_test");
      const event = purpose.startsWith("notify.") ? purpose.slice(7) : purpose;
      return EVENT_LABEL_KEYS[event] ? t(EVENT_LABEL_KEYS[event]) : purpose;
    };

    const when = (v: string | null | undefined) =>
      v ? (
        <span className="whitespace-nowrap tabular-nums">
          {datetime(v, undefined, locale)}
        </span>
      ) : (
        "—"
      );
    return [
      createColumn<WhatsAppMessage>({
        accessorKey: "created_at",
        labelKey: "whatsapp.messages.columns.created_at",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => when(row.original.created_at),
      }),
      createColumn<WhatsAppMessage>({
        accessorKey: "purpose",
        labelKey: "whatsapp.messages.columns.purpose",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <span className="font-medium">
              {purposeLabel(row.original.purpose)}
            </span>
            {row.original.test ? (
              <Badge variant="outline">{t("whatsapp.messages.test")}</Badge>
            ) : null}
            {row.original.kind === "document" ? (
              <Badge variant="outline">{t("whatsapp.messages.document")}</Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<WhatsAppMessage>({
        id: "recipient",
        accessorFn: (row) => row.recipient_masked,
        labelKey: "whatsapp.messages.columns.recipient",
        enableSorting: false,
        cell: ({ row }) => (
          <div>
            <p>{row.original.customer_name || "—"}</p>
            <p className="text-muted-foreground text-xs tabular-nums">
              {row.original.recipient_masked}
            </p>
          </div>
        ),
      }),
      createColumn<WhatsAppMessage>({
        accessorKey: "status",
        labelKey: "whatsapp.messages.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: MESSAGE_STATUSES.map((s) => ({
          value: s,
          labelKey: MESSAGE_STATUS_LABEL_KEYS[s],
          label: t(MESSAGE_STATUS_LABEL_KEYS[s]),
        })),
        cell: ({ row }) => (
          <StatusChip
            label={t(MESSAGE_STATUS_LABEL_KEYS[row.original.status])}
            tone={MESSAGE_STATUS_TONES[row.original.status]}
          />
        ),
      }),
      createColumn<WhatsAppMessage>({
        accessorKey: "delivered_at",
        labelKey: "whatsapp.messages.columns.delivered_at",
        enableSorting: false,
        cell: ({ row }) => when(row.original.delivered_at),
      }),
      createColumn<WhatsAppMessage>({
        accessorKey: "read_at",
        labelKey: "whatsapp.messages.columns.read_at",
        enableSorting: false,
        cell: ({ row }) => when(row.original.read_at),
      }),
      createColumn<WhatsAppMessage>({
        accessorKey: "error",
        labelKey: "whatsapp.messages.columns.error",
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
      createColumn<WhatsAppMessage>({
        accessorKey: "provider_message_id",
        labelKey: "whatsapp.messages.columns.provider_message_id",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.provider_message_id}
          </span>
        ),
      }),
      createColumn<WhatsAppMessage>({
        accessorKey: "sent_at",
        labelKey: "whatsapp.messages.columns.sent_at",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => when(row.original.sent_at),
      }),
    ] as ColumnDef<WhatsAppMessage, unknown>[];
  }, [locale, t]);

  const total = query.data?.total ?? 0;
  const hasFilters = Boolean(params.q || params.status || from || to);
  const refresh = () => void query.refetch();

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={query.isLoading}
      isError={query.isError}
      errorDescription={t("whatsapp.errors.messages")}
      onRetry={refresh}
      emptyTitle={
        hasFilters
          ? t("whatsapp.messages.empty_filtered")
          : t("whatsapp.messages.empty")
      }
      pageCount={Math.max(
        1,
        Math.ceil(total / (listState.pagination.pageSize || 20)),
      )}
      state={listState.tableState}
      features={{
        persistKey: "tenant-whatsapp-messages-v1",
        rowSelection: false,
      }}
      toolbarExtra={
        <>
          <div className="grid w-full grid-cols-2 gap-2 sm:w-auto">
            <DatePicker
              value={from}
              onChange={setFrom}
              placeholder={t("whatsapp.messages.from")}
            />
            <DatePicker
              value={to}
              onChange={setTo}
              placeholder={t("whatsapp.messages.to")}
            />
          </div>
          <EntityToolbar
            onRefresh={refresh}
            refreshDisabled={query.isFetching}
          />
        </>
      }
    />
  );
}
