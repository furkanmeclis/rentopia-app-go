"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, RotateCcw, XCircle } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityDrawer,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { AppForm, AppTextarea } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  MessageStatusChip,
  amountText,
  channelProblemText,
  errorCodeText,
} from "@/features/channels/components/channel-cells";
import {
  CHANNEL_MESSAGES_RESOURCE,
  COLLECTED_BY_LABEL_KEYS,
  MESSAGE_STATUS_LABEL_KEYS,
  MESSAGE_STATUSES,
  MESSAGE_TYPE_LABEL_KEYS,
  MESSAGE_TYPES,
} from "@/features/channels/constants";
import {
  useChannelMessage,
  useChannelMessageFacets,
  useChannelMessages,
  useResolveChannelMessage,
} from "@/features/channels/hooks/use-channels";
import type {
  ChannelMessage,
  ListChannelMessagesParams,
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
import { appToast } from "@/providers/toast-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

function summaryText(t: T, m: ChannelMessage, locale: "tr" | "en"): string {
  const s = m.summary as Record<string, unknown>;
  const parts: string[] = [];
  if (typeof s.product_code === "string" && s.product_code)
    parts.push(s.product_code);
  if (typeof s.pickup_location_code === "string" && s.pickup_location_code) {
    parts.push(s.pickup_location_code);
  }
  if (typeof s.starts_at === "string" && typeof s.ends_at === "string") {
    parts.push(
      `${datetime(s.starts_at, "d MMM HH:mm", locale)} → ${datetime(s.ends_at, "d MMM HH:mm", locale)}`,
    );
  }
  return parts.join(" · ") || t("channels.messages.no_summary");
}

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="min-w-0 space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{value}</dd>
    </div>
  );
}

const IGNORE_FORM = "channel-ignore-form";

/** Dismiss a failed message with a note (the channel is told). */
function IgnoreDialog({
  uuid,
  onClose,
}: {
  uuid: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const resolve = useResolveChannelMessage();
  const schema = useMemo(
    () =>
      z.object({
        note: z
          .string()
          .trim()
          .min(3, t("channels.validation.note"))
          .max(500, t("channels.validation.note")),
      }),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(uuid)}
      onOpenChange={(open) => (!open ? onClose() : undefined)}
      title={t("channels.messages.ignore_title")}
      description={t("channels.messages.ignore_description")}
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={IGNORE_FORM}
            variant="destructive"
            disabled={resolve.isPending}
          >
            {t("channels.actions.ignore")}
          </Button>
        </>
      }
    >
      {uuid ? (
        <AppForm<{ note: string }>
          id={IGNORE_FORM}
          schema={schema}
          defaultValues={{ note: "" }}
          onSubmit={(v) =>
            resolve.mutate(
              { uuid, action: "ignore", note: v.note.trim() },
              {
                onSuccess: () => {
                  appToast.success(t("channels.toast.ignored"));
                  onClose();
                },
                onError: (error) =>
                  appToast.error(channelProblemText(t, error)),
              },
            )
          }
        >
          <AppTextarea name="note" label={t("channels.messages.note")} />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/** A message with its channel booking and the retry / dismiss actions. */
function MessageDrawer({
  uuid,
  onClose,
  onRetry,
  onIgnore,
}: {
  uuid: string | null;
  onClose: () => void;
  onRetry: (uuid: string) => void;
  onIgnore: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = useChannelMessage(uuid);
  const d = query.data;
  const s = (d?.summary ?? {}) as Record<string, unknown>;
  const str = (v: unknown) => (typeof v === "string" && v ? v : "—");
  return (
    <EntityDrawer
      open={Boolean(uuid)}
      onOpenChange={(open) => (!open ? onClose() : undefined)}
      title={t("channels.messages.drawer_title")}
      description={d ? `${d.external_ref} · v${d.version}` : undefined}
      size="md"
      footer={
        d && (d.actions.retry || d.actions.ignore) ? (
          <div className="flex w-full justify-end gap-2">
            {d.actions.ignore ? (
              <Button variant="outline" onClick={() => onIgnore(d.uuid)}>
                {t("channels.actions.ignore")}
              </Button>
            ) : null}
            {d.actions.retry ? (
              <Button onClick={() => onRetry(d.uuid)}>
                {t("channels.actions.retry")}
              </Button>
            ) : null}
          </div>
        ) : undefined
      }
    >
      {!d ? (
        <div className="space-y-3 px-4">
          <Skeleton className="h-6 w-1/2" />
          <Skeleton className="h-32 w-full" />
        </div>
      ) : (
        <div className="space-y-4 px-4">
          <div className="flex flex-wrap items-center gap-2">
            <MessageStatusChip status={d.status} />
            <span className="text-muted-foreground text-sm">
              {t(MESSAGE_TYPE_LABEL_KEYS[d.type])}
            </span>
          </div>
          {d.status === "failed" ? (
            <Alert variant="destructive">
              <AlertDescription>
                <span className="font-medium">
                  {errorCodeText(t, d.error_code)}
                </span>
                {d.error_message ? ` — ${d.error_message}` : null}
              </AlertDescription>
            </Alert>
          ) : d.error_message ? (
            <p className="text-muted-foreground text-sm">{d.error_message}</p>
          ) : null}
          <dl className="grid grid-cols-2 gap-3">
            <Fact
              label={t("channels.messages.account")}
              value={d.account.name ?? "—"}
            />
            <Fact
              label={t("channels.messages.message_key")}
              value={<code className="text-xs">{d.message_key}</code>}
            />
            <Fact
              label={t("channels.messages.product_code")}
              value={str(s.product_code)}
            />
            <Fact
              label={t("channels.messages.offer_ref")}
              value={str(s.offer_ref)}
            />
            <Fact
              label={t("channels.messages.pickup")}
              value={str(s.pickup_location_code)}
            />
            <Fact
              label={t("channels.messages.return")}
              value={str(s.return_location_code)}
            />
            <Fact
              label={t("channels.messages.starts_at")}
              value={
                typeof s.starts_at === "string"
                  ? datetime(s.starts_at, "d MMM yyyy HH:mm", locale)
                  : "—"
              }
            />
            <Fact
              label={t("channels.messages.ends_at")}
              value={
                typeof s.ends_at === "string"
                  ? datetime(s.ends_at, "d MMM yyyy HH:mm", locale)
                  : "—"
              }
            />
            <Fact
              label={t("channels.messages.customer")}
              value={str(s.customer)}
            />
            <Fact
              label={t("channels.messages.total")}
              value={amountText(str(s.total), locale, str(s.currency))}
            />
            <Fact
              label={t("channels.messages.deliveries")}
              value={String(d.deliveries)}
            />
            <Fact
              label={t("channels.messages.attempts")}
              value={String(d.attempts)}
            />
            <Fact
              label={t("channels.messages.acked_at")}
              value={
                d.acked_at
                  ? datetime(d.acked_at, "d MMM HH:mm", locale)
                  : d.ack_error || "—"
              }
            />
            <Fact
              label={t("channels.messages.reservation")}
              value={
                d.reservation ? (
                  <Link
                    href={routes.tenant.reservations.detail(
                      slug,
                      d.reservation.uuid,
                    )}
                    className="font-mono text-xs underline-offset-4 hover:underline"
                  >
                    {d.reservation.reference}
                  </Link>
                ) : (
                  "—"
                )
              }
            />
          </dl>
          {d.booking ? (
            <div className="space-y-2 rounded-lg border p-3">
              <p className="text-sm font-medium">
                {t("channels.messages.booking_title")}
              </p>
              <dl className="grid grid-cols-2 gap-3">
                <Fact
                  label={t("channels.messages.applied_version")}
                  value={`v${d.booking.applied_version} / v${d.booking.last_version}`}
                />
                <Fact
                  label={t("channels.messages.collected_by")}
                  value={t(COLLECTED_BY_LABEL_KEYS[d.booking.collected_by])}
                />
                <Fact
                  label={t("channels.messages.gross")}
                  value={amountText(
                    d.booking.gross,
                    locale,
                    d.booking.currency,
                  )}
                />
                <Fact
                  label={t("channels.messages.commission")}
                  value={amountText(
                    d.booking.commission,
                    locale,
                    d.booking.currency,
                  )}
                />
                <Fact
                  label={t("channels.messages.net")}
                  value={amountText(d.booking.net, locale, d.booking.currency)}
                />
                <Fact
                  label={t("channels.messages.price_difference")}
                  value={amountText(
                    d.booking.price_difference,
                    locale,
                    d.booking.currency,
                  )}
                />
              </dl>
            </div>
          ) : null}
          {d.resolution_note ? (
            <p className="text-muted-foreground text-sm">
              {t("channels.messages.resolution", {
                note: d.resolution_note,
                by: d.resolved_by || "—",
              })}
            </p>
          ) : null}
        </div>
      )}
    </EntityDrawer>
  );
}

/**
 * Inbound channel messages and the error queue: every booking /
 * modification / cancellation once, with its result. Failed messages can
 * be applied again after a fix, or dismissed with a note.
 */
export function ChannelMessagesTable() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const canSync = can(permissions.tenant.channels.sync);
  const urlDefaults = useUrlListDefaults(
    ["status", "type"],
    ["created_at", "occurred_at"],
  );
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-created_at",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CHANNEL_MESSAGES_RESOURCE),
  });
  const [open, setOpen] = useState<string | null>(null);
  const [ignore, setIgnore] = useState<string | null>(null);
  const resolve = useResolveChannelMessage();
  const retry = (uuid: string) =>
    resolve.mutate(
      { uuid, action: "retry" },
      {
        onSuccess: (d) =>
          d.status === "failed"
            ? appToast.error(
                t("channels.toast.retry_failed", {
                  error: errorCodeText(t, d.error_code),
                }),
              )
            : appToast.success(
                t("channels.toast.retried", {
                  status: t(MESSAGE_STATUS_LABEL_KEYS[d.status]),
                }),
              ),
        onError: (error) => appToast.error(channelProblemText(t, error)),
      },
    );

  const listParams = useMemo<ListChannelMessagesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      type: joinFilter(column("type")),
    };
  }, [listState.columnFilters, listState.params]);
  const listQuery = useChannelMessages(listParams);
  const facetsQuery = useChannelMessageFacets();
  const facets = facetsQuery.data;
  const total = listQuery.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<ChannelMessage>({
          accessorKey: "created_at",
          labelKey: "channels.messages.columns.received_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-sm whitespace-nowrap tabular-nums">
              {datetime(row.original.created_at, "d MMM yyyy HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<ChannelMessage>({
          accessorKey: "type",
          labelKey: "channels.messages.columns.type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: MESSAGE_TYPES.map((v) => ({
            value: v,
            labelKey: MESSAGE_TYPE_LABEL_KEYS[v],
            label: `${t(MESSAGE_TYPE_LABEL_KEYS[v])} (${facets?.types[v] ?? 0})`,
          })),
          cell: ({ row }) => t(MESSAGE_TYPE_LABEL_KEYS[row.original.type]),
        }),
        createColumn<ChannelMessage>({
          accessorKey: "external_ref",
          labelKey: "channels.messages.columns.external_ref",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs font-medium">
              {row.original.external_ref}{" "}
              <span className="text-muted-foreground">
                v{row.original.version}
              </span>
            </span>
          ),
        }),
        createColumn<ChannelMessage>({
          accessorKey: "status",
          labelKey: "channels.messages.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: MESSAGE_STATUSES.map((v) => ({
            value: v,
            labelKey: MESSAGE_STATUS_LABEL_KEYS[v],
            label: `${t(MESSAGE_STATUS_LABEL_KEYS[v])} (${facets?.statuses[v] ?? 0})`,
          })),
          cell: ({ row }) => (
            <span className="flex flex-col items-start gap-1">
              <MessageStatusChip status={row.original.status} />
              {row.original.error_code ? (
                <span className="text-muted-foreground text-[11px]">
                  {errorCodeText(t, row.original.error_code)}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<ChannelMessage>({
          id: "summary",
          accessorFn: (row) => row.external_ref,
          labelKey: "channels.messages.columns.summary",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {summaryText(t, row.original, locale)}
            </span>
          ),
        }),
        createColumn<ChannelMessage>({
          id: "reservation",
          accessorFn: (row) => row.reservation?.reference ?? "",
          labelKey: "channels.messages.columns.reservation",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reservation ? (
              <Link
                href={routes.tenant.reservations.detail(
                  slug,
                  row.original.reservation.uuid,
                )}
                className="font-mono text-xs underline-offset-4 hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {row.original.reservation.reference}
              </Link>
            ) : (
              "—"
            ),
        }),
        createColumn<ChannelMessage>({
          id: "account",
          accessorFn: (row) => row.account.name ?? "",
          labelKey: "channels.messages.columns.account",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.account.name,
        }),
        createColumn<ChannelMessage>({
          accessorKey: "deliveries",
          labelKey: "channels.messages.columns.deliveries",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.deliveries,
        }),
        ...auditColumns<ChannelMessage>(
          {
            createdAt: "channels.messages.columns.created_at",
            uuid: "channels.messages.columns.uuid",
          },
          locale,
        ),
        createColumn<ChannelMessage>({
          id: "actions",
          labelKey: "channels.messages.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const m = row.original;
            const failed = m.status === "failed" && canSync;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: t("channels.actions.open"),
                    icon: Eye,
                    onSelect: () => setOpen(m.uuid),
                  },
                  ...(failed
                    ? [
                        {
                          id: "retry",
                          label: t("channels.actions.retry"),
                          icon: RotateCcw,
                          onSelect: () => retry(m.uuid),
                        },
                        {
                          id: "ignore",
                          label: t("channels.actions.ignore"),
                          icon: XCircle,
                          variant: "destructive" as const,
                          onSelect: () => setIgnore(m.uuid),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<ChannelMessage, unknown>[],
    // retry is recreated each render; it only closes over stable values.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [canSync, facets, locale, slug, t],
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
        onRowClick={(row) => setOpen(row.uuid)}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("channels.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("channels.messages.empty_filtered_title")
            : t("channels.messages.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("channels.messages.empty_filtered_description")
            : t("channels.messages.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-channel-messages-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void listQuery.refetch();
              void facetsQuery.refetch();
            }}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
      <MessageDrawer
        uuid={open}
        onClose={() => setOpen(null)}
        onRetry={retry}
        onIgnore={(uuid) => setIgnore(uuid)}
      />
      <IgnoreDialog uuid={ignore} onClose={() => setIgnore(null)} />
    </>
  );
}
