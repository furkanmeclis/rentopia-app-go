"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { RuleDialog } from "@/features/notification-engine/components/rule-dialog";
import {
  AUDIENCE_LABEL_KEYS,
  CHANNEL_LABEL_KEYS,
  EVENT_LABEL_KEYS,
  GROUP_LABEL_KEYS,
} from "@/features/notification-engine/constants";
import {
  useNotificationEvents,
  useResetRule,
} from "@/features/notification-engine/hooks/use-notification-engine";
import { offsetLabel } from "@/features/notification-engine/lib/format";
import type {
  NotificationChannel,
  NotificationEvent,
} from "@/features/notification-engine/types";
import { useLocale } from "@/providers/locale-provider";

/** "Kurallar": every catalog event with its channel order and reminders. */
export function RulesTable() {
  const { t } = useLocale();
  const query = useNotificationEvents();
  const reset = useResetRule();
  const [editing, setEditing] = useState<NotificationEvent | null>(null);
  const [resetting, setResetting] = useState<NotificationEvent | null>(null);
  const channels = useMemo(
    () => query.data?.channels ?? [],
    [query.data?.channels],
  );
  const available = useMemo(
    () => new Map(channels.map((c) => [c.name, c.available])),
    [channels],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<NotificationEvent>({
          id: "event",
          accessorFn: (row) => t(EVENT_LABEL_KEYS[row.code] ?? row.code),
          labelKey: "notification_engine.rules.columns.event",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-[12rem]">
              <p className="font-medium">
                {t(EVENT_LABEL_KEYS[row.original.code] ?? row.original.code)}
              </p>
              <p className="text-muted-foreground font-mono text-xs">
                {row.original.code}
              </p>
            </div>
          ),
        }),
        createColumn<NotificationEvent>({
          id: "group",
          accessorFn: (row) => row.group,
          labelKey: "notification_engine.rules.columns.group",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: Object.entries(GROUP_LABEL_KEYS).map(([v, k]) => ({
            value: v,
            labelKey: k,
            label: t(k),
          })),
          cell: ({ row }) => t(GROUP_LABEL_KEYS[row.original.group]),
        }),
        createColumn<NotificationEvent>({
          id: "audience",
          accessorFn: (row) => row.audience,
          labelKey: "notification_engine.rules.columns.audience",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: Object.entries(AUDIENCE_LABEL_KEYS).map(([v, k]) => ({
            value: v,
            labelKey: k,
            label: t(k),
          })),
          cell: ({ row }) => t(AUDIENCE_LABEL_KEYS[row.original.audience]),
        }),
        createColumn<NotificationEvent>({
          id: "channels",
          accessorFn: (row) => row.rule.channels.join(","),
          labelKey: "notification_engine.rules.columns.channels",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.rule.channels.length ? (
              <div className="flex flex-wrap items-center gap-1">
                {(row.original.rule.channels as NotificationChannel[]).map(
                  (c, i) => (
                    <Badge
                      key={c}
                      variant={
                        available.get(c) === false ? "outline" : "secondary"
                      }
                    >
                      {i + 1}. {t(CHANNEL_LABEL_KEYS[c])}
                    </Badge>
                  ),
                )}
              </div>
            ) : (
              "—"
            ),
        }),
        createColumn<NotificationEvent>({
          id: "fallback",
          accessorFn: (row) => (row.rule.fallback ? "yes" : "no"),
          labelKey: "notification_engine.rules.columns.fallback",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.rule.fallback
              ? t("notification_engine.yes")
              : t("notification_engine.no"),
        }),
        createColumn<NotificationEvent>({
          id: "reminders",
          accessorFn: (row) => row.rule.reminder_offsets.join(","),
          labelKey: "notification_engine.rules.columns.reminders",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reminder && row.original.rule.reminder_offsets.length
              ? row.original.rule.reminder_offsets
                  .map((o) => offsetLabel(t, o))
                  .join(", ")
              : "—",
        }),
        createColumn<NotificationEvent>({
          id: "status",
          accessorFn: (row) => (row.rule.enabled ? "on" : "off"),
          labelKey: "notification_engine.rules.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "on",
              labelKey: "notification_engine.rules.on",
              label: t("notification_engine.rules.on"),
            },
            {
              value: "off",
              labelKey: "notification_engine.rules.off",
              label: t("notification_engine.rules.off"),
            },
          ],
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-1">
              <StatusChip
                label={
                  row.original.rule.enabled
                    ? t("notification_engine.rules.on")
                    : t("notification_engine.rules.off")
                }
                tone={row.original.rule.enabled ? "success" : "default"}
              />
              {row.original.rule.custom ? (
                <Badge variant="outline">
                  {t("notification_engine.custom")}
                </Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<NotificationEvent>({
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
                  label: t("notification_engine.rules.edit"),
                  icon: Pencil,
                  permission: permissions.tenant.notificationSettings.write,
                  onSelect: () => setEditing(row.original),
                },
                {
                  id: "reset",
                  label: t("notification_engine.reset"),
                  icon: RotateCcw,
                  permission: permissions.tenant.notificationSettings.write,
                  disabled: !row.original.rule.custom,
                  onSelect: () => setResetting(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<NotificationEvent, unknown>[],
    [available, t],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.events ?? []}
        getRowId={(row) => row.code}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("notification_engine.errors.load")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("notification_engine.rules.empty")}
        onRowClick={(row) => setEditing(row)}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-notification-rules-v1",
          rowSelection: false,
          pagination: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <RuleDialog
        event={editing}
        channels={channels}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
      />
      <ConfirmDialog
        open={Boolean(resetting)}
        title={t("notification_engine.rules.reset_title")}
        description={t("notification_engine.rules.reset_description")}
        confirmLabel={t("notification_engine.reset")}
        isPending={reset.isPending}
        onConfirm={() => {
          if (!resetting) return;
          reset.mutate(resetting.code, {
            onSettled: () => setResetting(null),
          });
        }}
        onCancel={() => setResetting(null)}
      />
    </>
  );
}
