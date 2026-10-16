"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { KeyRound, ListChecks, Pencil, Send, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityCreateButton,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  ENDPOINT_STATUSES,
  ENDPOINT_STATUS_LABEL_KEYS,
  EVENT_LABEL_KEYS,
  PUBLIC_WEBHOOKS_RESOURCE,
} from "@/features/public-api/constants";
import {
  SecretDialog,
  type ShownSecret,
} from "@/features/public-api/components/secret-dialog";
import { WebhookFormDialog } from "@/features/public-api/components/webhook-dialogs";
import {
  useDeletePublicWebhook,
  usePublicWebhooks,
  useRotatePublicWebhookSecret,
  useTestPublicWebhook,
} from "@/features/public-api/hooks/use-public-api";
import type {
  IssuedPublicWebhook,
  ListWebhooksParams,
  PublicWebhook,
} from "@/features/public-api/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Webhook endpoints: signed reservation events of bookings made through the
 * public API. "Teslimatlar" opens the deliveries tab filtered to an
 * endpoint.
 */
export function WebhooksTable({
  onShowDeliveries,
}: {
  onShowDeliveries: (endpoint: PublicWebhook) => void;
}) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const manage = permissions.tenant.publicApi.manage;
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, PUBLIC_WEBHOOKS_RESOURCE),
  });
  const params = useMemo<ListWebhooksParams>(
    () => ({
      ...listState.params,
      status: joinFilter(filterValues(listState.columnFilters, "status")),
    }),
    [listState.columnFilters, listState.params],
  );
  const query = usePublicWebhooks(params);
  const test = useTestPublicWebhook();
  const rotate = useRotatePublicWebhookSecret();
  const remove = useDeletePublicWebhook();
  const total = query.data?.total ?? 0;
  const [formOpen, setFormOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<PublicWebhook | null>(null);
  const [rotateTarget, setRotateTarget] = useState<PublicWebhook | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PublicWebhook | null>(null);
  const [shown, setShown] = useState<ShownSecret | null>(null);

  const showIssued = (issued: IssuedPublicWebhook) =>
    setShown({
      title: t("public_api.secret.webhook_title"),
      description: t("public_api.secret.webhook_description"),
      secret: issued.secret,
      once: true,
    });

  const columns = useMemo(
    () =>
      [
        createColumn<PublicWebhook>({
          accessorKey: "name",
          labelKey: "public_api.webhooks.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex min-w-0 flex-col">
              <span className="font-medium">{row.original.name}</span>
              <span className="text-muted-foreground truncate font-mono text-xs">
                {row.original.url}
              </span>
            </div>
          ),
        }),
        createColumn<PublicWebhook>({
          accessorKey: "status",
          labelKey: "public_api.webhooks.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ENDPOINT_STATUSES.map((v) => ({
            value: v,
            labelKey: ENDPOINT_STATUS_LABEL_KEYS[v]!,
            label: t(ENDPOINT_STATUS_LABEL_KEYS[v]!),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(ENDPOINT_STATUS_LABEL_KEYS[row.original.status]!)}
              tone={row.original.status === "active" ? "success" : "default"}
            />
          ),
        }),
        createColumn<PublicWebhook>({
          id: "events",
          accessorFn: (row) => row.events.join(" "),
          labelKey: "public_api.webhooks.columns.events",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.events.map((e) => (
                <Badge key={e} variant="secondary">
                  {t(EVENT_LABEL_KEYS[e] ?? "public_api.events.unknown")}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<PublicWebhook>({
          id: "health",
          accessorFn: (row) => row.consecutive_failures,
          labelKey: "public_api.webhooks.columns.health",
          enableSorting: false,
          cell: ({ row }) => {
            const w = row.original;
            return (
              <div className="flex flex-col gap-0.5 text-xs">
                {w.last_success_at ? (
                  <span>
                    {t("public_api.webhooks.last_success", {
                      date: datetime(w.last_success_at, "d MMM HH:mm", locale),
                    })}
                  </span>
                ) : null}
                {w.consecutive_failures > 0 ? (
                  <span className="text-destructive">
                    {t("public_api.webhooks.failures", {
                      count: w.consecutive_failures,
                    })}
                  </span>
                ) : null}
                {!w.last_success_at && w.consecutive_failures === 0 ? (
                  <span className="text-muted-foreground">
                    {t("public_api.webhooks.no_deliveries")}
                  </span>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<PublicWebhook>({
          accessorKey: "secret_last4",
          labelKey: "public_api.webhooks.columns.secret",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              whsec_…{row.original.secret_last4}
            </span>
          ),
        }),
        createColumn<PublicWebhook>({
          accessorKey: "created_at",
          labelKey: "public_api.webhooks.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<PublicWebhook>({
          id: "actions",
          labelKey: "public_api.webhooks.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const w = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "deliveries",
                    label: t("public_api.webhooks.actions.deliveries"),
                    icon: ListChecks,
                    onSelect: () => onShowDeliveries(w),
                  },
                  {
                    id: "test",
                    label: t("public_api.webhooks.actions.test"),
                    icon: Send,
                    permission: manage,
                    disabled: test.isPending,
                    onSelect: () => test.mutate(w.uuid),
                  },
                  {
                    id: "edit",
                    label: t("public_api.webhooks.actions.edit"),
                    icon: Pencil,
                    permission: manage,
                    onSelect: () => setEditTarget(w),
                  },
                  {
                    id: "rotate",
                    label: t("public_api.webhooks.actions.rotate_secret"),
                    icon: KeyRound,
                    permission: manage,
                    onSelect: () => setRotateTarget(w),
                  },
                  {
                    id: "delete",
                    label: t("public_api.webhooks.actions.delete"),
                    icon: Trash2,
                    permission: manage,
                    variant: "destructive" as const,
                    separatorBefore: true,
                    onSelect: () => setDeleteTarget(w),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<PublicWebhook, unknown>[],
    [locale, manage, onShowDeliveries, t, test],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("public_api.webhooks.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("public_api.webhooks.empty_title")}
        emptyDescription={t("public_api.webhooks.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-public-webhooks-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <div className="flex items-center gap-2">
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
            <EntityCreateButton
              label={t("public_api.webhooks.create")}
              permission={manage}
              onClick={() => setFormOpen(true)}
            />
          </div>
        }
      />
      <WebhookFormDialog
        open={formOpen || Boolean(editTarget)}
        webhook={editTarget}
        onClose={() => {
          setFormOpen(false);
          setEditTarget(null);
        }}
        onIssued={showIssued}
      />
      <SecretDialog shown={shown} onClose={() => setShown(null)} />
      <ConfirmDialog
        open={Boolean(rotateTarget)}
        title={t("public_api.webhooks.rotate_title")}
        description={t("public_api.webhooks.rotate_description")}
        confirmLabel={t("public_api.webhooks.actions.rotate_secret")}
        isPending={rotate.isPending}
        onCancel={() => setRotateTarget(null)}
        onConfirm={() => {
          if (!rotateTarget) return;
          rotate.mutate(rotateTarget.uuid, {
            onSuccess: (issued) => {
              setRotateTarget(null);
              showIssued(issued);
            },
          });
        }}
      />
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t("public_api.webhooks.delete_title")}
        description={t("public_api.webhooks.delete_description")}
        confirmLabel={t("public_api.webhooks.actions.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSuccess: () => setDeleteTarget(null),
          });
        }}
      />
    </>
  );
}
