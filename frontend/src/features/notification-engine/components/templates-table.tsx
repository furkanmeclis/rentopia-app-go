"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { TemplateDialog } from "@/features/notification-engine/components/template-dialog";
import {
  CHANNELS,
  CHANNEL_LABEL_KEYS,
  EVENT_LABEL_KEYS,
  LANGUAGES,
  LANGUAGE_LABEL_KEYS,
} from "@/features/notification-engine/constants";
import {
  useNotificationTemplates,
  useResetTemplate,
} from "@/features/notification-engine/hooks/use-notification-engine";
import type { NotificationTemplate } from "@/features/notification-engine/types";
import { useLocale } from "@/providers/locale-provider";

/** "Şablonlar": every event / channel / language message. */
export function TemplatesTable() {
  const { t } = useLocale();
  const query = useNotificationTemplates();
  const reset = useResetTemplate();
  const [editing, setEditing] = useState<NotificationTemplate | null>(null);
  const [resetting, setResetting] = useState<NotificationTemplate | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<NotificationTemplate>({
          id: "event",
          accessorFn: (row) => row.event,
          labelKey: "notification_engine.templates.columns.event",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: Object.entries(EVENT_LABEL_KEYS).map(([v, k]) => ({
            value: v,
            labelKey: k,
            label: t(k),
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {t(EVENT_LABEL_KEYS[row.original.event] ?? row.original.event)}
            </span>
          ),
        }),
        createColumn<NotificationTemplate>({
          id: "channel",
          accessorFn: (row) => row.channel,
          labelKey: "notification_engine.templates.columns.channel",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: CHANNELS.map((c) => ({
            value: c,
            labelKey: CHANNEL_LABEL_KEYS[c],
            label: t(CHANNEL_LABEL_KEYS[c]),
          })),
          cell: ({ row }) => t(CHANNEL_LABEL_KEYS[row.original.channel]),
        }),
        createColumn<NotificationTemplate>({
          id: "language",
          accessorFn: (row) => row.language,
          labelKey: "notification_engine.templates.columns.language",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: LANGUAGES.map((l) => ({
            value: l,
            labelKey: LANGUAGE_LABEL_KEYS[l],
            label: t(LANGUAGE_LABEL_KEYS[l]),
          })),
          cell: ({ row }) => t(LANGUAGE_LABEL_KEYS[row.original.language]),
        }),
        createColumn<NotificationTemplate>({
          id: "subject",
          accessorFn: (row) => row.subject,
          labelKey: "notification_engine.templates.columns.subject",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="block max-w-[16rem] truncate">
              {row.original.subject || "—"}
            </span>
          ),
        }),
        createColumn<NotificationTemplate>({
          id: "body",
          accessorFn: (row) => row.body,
          labelKey: "notification_engine.templates.columns.body",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground block max-w-[28rem] truncate text-xs">
              {row.original.body}
            </span>
          ),
        }),
        createColumn<NotificationTemplate>({
          id: "custom",
          accessorFn: (row) => (row.custom ? "custom" : "default"),
          labelKey: "notification_engine.templates.columns.source",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "custom",
              labelKey: "notification_engine.custom",
              label: t("notification_engine.custom"),
            },
            {
              value: "default",
              labelKey: "notification_engine.default",
              label: t("notification_engine.default"),
            },
          ],
          cell: ({ row }) =>
            row.original.custom ? (
              <Badge variant="secondary">
                {t("notification_engine.custom")}
              </Badge>
            ) : (
              <Badge variant="outline">
                {t("notification_engine.default")}
              </Badge>
            ),
        }),
        createColumn<NotificationTemplate>({
          id: "actions",
          labelKey: "notification_engine.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("notification_engine.templates.edit"),
                  icon: Pencil,
                  permission: permissions.tenant.notificationSettings.write,
                  onSelect: () => setEditing(row.original),
                },
                {
                  id: "reset",
                  label: t("notification_engine.reset"),
                  icon: RotateCcw,
                  permission: permissions.tenant.notificationSettings.write,
                  disabled: !row.original.custom,
                  onSelect: () => setResetting(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<NotificationTemplate, unknown>[],
    [t],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data ?? []}
        getRowId={(row) => `${row.event}|${row.channel}|${row.language}`}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("notification_engine.errors.load")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("notification_engine.templates.empty")}
        onRowClick={(row) => setEditing(row)}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-notification-templates-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <TemplateDialog
        template={editing}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
      />
      <ConfirmDialog
        open={Boolean(resetting)}
        title={t("notification_engine.templates.reset_title")}
        description={t("notification_engine.templates.reset_description")}
        confirmLabel={t("notification_engine.reset")}
        isPending={reset.isPending}
        onConfirm={() => {
          if (!resetting) return;
          reset.mutate(
            {
              event: resetting.event,
              channel: resetting.channel,
              language: resetting.language,
            },
            { onSettled: () => setResetting(null) },
          );
        }}
        onCancel={() => setResetting(null)}
      />
    </>
  );
}
