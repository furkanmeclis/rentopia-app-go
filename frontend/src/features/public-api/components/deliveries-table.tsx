"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, RotateCcw, X } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { AppDrawer } from "@/components/dialogs/app-drawer";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  DELIVERY_STATUSES,
  DELIVERY_STATUS_LABEL_KEYS,
  DELIVERY_STATUS_TONES,
  EVENT_LABEL_KEYS,
  PUBLIC_WEBHOOKS_RESOURCE,
  WEBHOOK_EVENTS,
} from "@/features/public-api/constants";
import {
  usePublicWebhookDeliveries,
  usePublicWebhookDelivery,
  useRetryPublicWebhookDelivery,
} from "@/features/public-api/hooks/use-public-api";
import type {
  ListDeliveriesParams,
  PublicWebhook,
  PublicWebhookDelivery,
} from "@/features/public-api/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function eventLabel(t: (key: string) => string, event: string) {
  const key = EVENT_LABEL_KEYS[event];
  return key ? t(key) : event;
}

/** One delivery: attempts, last answer and the signed payload. */
function DeliveryDrawer({
  uuid,
  onClose,
}: {
  uuid: string | null;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const query = usePublicWebhookDelivery(uuid);
  const retry = useRetryPublicWebhookDelivery();
  const d = query.data;
  return (
    <AppDrawer
      open={Boolean(uuid)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("public_api.deliveries.detail_title")}
      description={
        d ? `${eventLabel(t, d.event_type)} · ${d.endpoint.name}` : undefined
      }
      size="lg"
      footer={
        d?.status === "failed" ? (
          <Button
            type="button"
            disabled={retry.isPending}
            onClick={() => retry.mutate(d.uuid)}
          >
            {t("public_api.deliveries.actions.retry")}
          </Button>
        ) : undefined
      }
    >
      {query.isLoading || !d ? (
        <div className="space-y-3">
          <Skeleton className="h-5 w-1/2" />
          <Skeleton className="h-40 w-full" />
        </div>
      ) : (
        <div className="space-y-4 text-sm">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2">
            <dt className="text-muted-foreground">
              {t("public_api.deliveries.columns.status")}
            </dt>
            <dd>
              <StatusChip
                label={t(DELIVERY_STATUS_LABEL_KEYS[d.status])}
                tone={DELIVERY_STATUS_TONES[d.status]}
              />
            </dd>
            <dt className="text-muted-foreground">
              {t("public_api.deliveries.columns.attempts")}
            </dt>
            <dd>{d.attempts}</dd>
            <dt className="text-muted-foreground">
              {t("public_api.deliveries.columns.last_status_code")}
            </dt>
            <dd>{d.last_status_code ?? "—"}</dd>
            {d.next_attempt_at ? (
              <>
                <dt className="text-muted-foreground">
                  {t("public_api.deliveries.next_attempt")}
                </dt>
                <dd>
                  {datetime(d.next_attempt_at, "d MMM yyyy HH:mm", locale)}
                </dd>
              </>
            ) : null}
            {d.delivered_at ? (
              <>
                <dt className="text-muted-foreground">
                  {t("public_api.deliveries.columns.delivered_at")}
                </dt>
                <dd>{datetime(d.delivered_at, "d MMM yyyy HH:mm", locale)}</dd>
              </>
            ) : null}
            <dt className="text-muted-foreground">
              {t("public_api.deliveries.event_id")}
            </dt>
            <dd className="font-mono text-xs">{d.event_uuid}</dd>
          </dl>
          {d.last_error ? (
            <p className="text-destructive text-xs break-words">
              {d.last_error}
            </p>
          ) : null}
          <Textarea
            readOnly
            rows={16}
            aria-label={t("public_api.deliveries.payload")}
            value={JSON.stringify(d.payload ?? {}, null, 2)}
            className="font-mono text-xs"
          />
        </div>
      )}
    </AppDrawer>
  );
}

/** Webhook deliveries (newest first), optionally of one endpoint. */
export function DeliveriesTable({
  endpoint,
  onClearEndpoint,
}: {
  endpoint: PublicWebhook | null;
  onClearEndpoint: () => void;
}) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const manage = permissions.tenant.publicApi.manage;
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, PUBLIC_WEBHOOKS_RESOURCE),
  });
  const params = useMemo<ListDeliveriesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    const { q: _q, ...rest } = listState.params;
    return {
      ...rest,
      endpoint: endpoint?.uuid,
      status: joinFilter(column("status")),
      event_type: joinFilter(column("event_type")),
    };
  }, [endpoint, listState.columnFilters, listState.params]);
  const query = usePublicWebhookDeliveries(params);
  const retry = useRetryPublicWebhookDelivery();
  const [openUuid, setOpenUuid] = useState<string | null>(null);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<PublicWebhookDelivery>({
          accessorKey: "event_type",
          labelKey: "public_api.deliveries.columns.event_type",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: [...WEBHOOK_EVENTS, "ping"].map((v) => ({
            value: v,
            labelKey: EVENT_LABEL_KEYS[v]!,
            label: t(EVENT_LABEL_KEYS[v]!),
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {eventLabel(t, row.original.event_type)}
            </Badge>
          ),
        }),
        createColumn<PublicWebhookDelivery>({
          id: "endpoint",
          accessorFn: (row) => row.endpoint.name,
          labelKey: "public_api.deliveries.columns.endpoint",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex min-w-0 flex-col">
              <span>{row.original.endpoint.name}</span>
              <span className="text-muted-foreground truncate font-mono text-xs">
                {row.original.endpoint.url}
              </span>
            </div>
          ),
        }),
        createColumn<PublicWebhookDelivery>({
          accessorKey: "status",
          labelKey: "public_api.deliveries.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: DELIVERY_STATUSES.map((v) => ({
            value: v,
            labelKey: DELIVERY_STATUS_LABEL_KEYS[v],
            label: t(DELIVERY_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <div className="flex flex-col gap-0.5">
              <StatusChip
                label={t(DELIVERY_STATUS_LABEL_KEYS[row.original.status])}
                tone={DELIVERY_STATUS_TONES[row.original.status]}
              />
              {row.original.last_error ? (
                <span className="text-destructive line-clamp-1 text-xs">
                  {row.original.last_error}
                </span>
              ) : null}
            </div>
          ),
        }),
        createColumn<PublicWebhookDelivery>({
          accessorKey: "attempts",
          labelKey: "public_api.deliveries.columns.attempts",
          enableSorting: false,
          cell: ({ row }) => row.original.attempts,
        }),
        createColumn<PublicWebhookDelivery>({
          accessorKey: "last_status_code",
          labelKey: "public_api.deliveries.columns.last_status_code",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.last_status_code ?? "—",
        }),
        createColumn<PublicWebhookDelivery>({
          accessorKey: "delivered_at",
          labelKey: "public_api.deliveries.columns.delivered_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.delivered_at
              ? datetime(row.original.delivered_at, "d MMM yyyy HH:mm", locale)
              : "—",
        }),
        createColumn<PublicWebhookDelivery>({
          accessorKey: "created_at",
          labelKey: "public_api.deliveries.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm:ss", locale),
        }),
        createColumn<PublicWebhookDelivery>({
          id: "actions",
          labelKey: "public_api.deliveries.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const d = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: t("public_api.deliveries.actions.open"),
                    icon: Eye,
                    onSelect: () => setOpenUuid(d.uuid),
                  },
                  ...(d.status === "failed"
                    ? [
                        {
                          id: "retry",
                          label: t("public_api.deliveries.actions.retry"),
                          icon: RotateCcw,
                          permission: manage,
                          disabled: retry.isPending,
                          onSelect: () => retry.mutate(d.uuid),
                        },
                      ]
                    : []),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<PublicWebhookDelivery, unknown>[],
    [locale, manage, retry, t],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => setOpenUuid(row.uuid)}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("public_api.deliveries.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("public_api.deliveries.empty_title")}
        emptyDescription={t("public_api.deliveries.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-public-webhook-deliveries-v1",
          rowSelection: false,
          globalFilter: false,
        }}
        toolbarExtra={
          <div className="flex items-center gap-2">
            {endpoint ? (
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={onClearEndpoint}
                aria-label={t("public_api.deliveries.clear_endpoint")}
              >
                {t("public_api.deliveries.endpoint_filter", {
                  name: endpoint.name,
                })}
                <X className="size-3" />
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
          </div>
        }
      />
      <DeliveryDrawer uuid={openUuid} onClose={() => setOpenUuid(null)} />
    </>
  );
}
