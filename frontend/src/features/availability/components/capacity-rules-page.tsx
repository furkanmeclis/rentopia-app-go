"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CapacityRuleDrawer } from "@/features/availability/components/capacity-rule-drawer";
import {
  CAPACITY_RULES_RESOURCE,
  RULE_TYPE_LABEL_KEYS,
  RULE_TYPES,
} from "@/features/availability/constants";
import {
  useCapacityRulesList,
  useDeleteCapacityRule,
  useSaveCapacityRule,
} from "@/features/availability/hooks/use-availability";
import type {
  CapacityRule,
  ListCapacityRulesParams,
} from "@/features/availability/types";
import {
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  SALES_CHANNEL_LABEL_KEYS,
  SALES_CHANNELS,
} from "@/features/tenant-settings/constants";
import type { SalesChannelCode } from "@/features/tenant-settings/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

/**
 * Channel capacity rules ("Kanal kapasite kuralları"): without rules every
 * channel sells from the shared pool; safety margins, guaranteed quotas and
 * sales limits narrow what a channel may sell. Small DataTable page with a
 * create / edit drawer.
 */
export function CapacityRulesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CAPACITY_RULES_RESOURCE),
  });
  const [drawer, setDrawer] = useState<{ rule: CapacityRule | null } | null>(
    null,
  );
  const [deleteTarget, setDeleteTarget] = useState<CapacityRule | null>(null);
  const write = permissions.tenant.capacityRules.write;

  const listParams = useMemo<ListCapacityRulesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      rule_type: joinFilter(column("rule_type")),
      channel: joinFilter(column("channel")),
    };
  }, [listState.columnFilters, listState.params]);
  const listQuery = useCapacityRulesList(listParams);
  const save = useSaveCapacityRule();
  const remove = useDeleteCapacityRule();

  const columns = useMemo(
    () =>
      [
        createColumn<CapacityRule>({
          id: "rule_type",
          accessorFn: (row) => row.rule_type,
          labelKey: "availability.rules.columns.rule_type",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: RULE_TYPES.map((type) => ({
            value: type,
            labelKey: RULE_TYPE_LABEL_KEYS[type],
            label: t(RULE_TYPE_LABEL_KEYS[type]),
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {t(RULE_TYPE_LABEL_KEYS[row.original.rule_type])}
            </span>
          ),
        }),
        createColumn<CapacityRule>({
          id: "channel",
          accessorFn: (row) => row.channel ?? "",
          labelKey: "availability.rules.columns.channel",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: SALES_CHANNELS.map((code) => ({
            value: code,
            labelKey: SALES_CHANNEL_LABEL_KEYS[code],
            label: t(SALES_CHANNEL_LABEL_KEYS[code]),
          })),
          cell: ({ row }) =>
            row.original.channel ? (
              (t(
                SALES_CHANNEL_LABEL_KEYS[
                  row.original.channel as SalesChannelCode
                ],
              ) ?? row.original.channel)
            ) : (
              <Badge variant="secondary">
                {t("availability.rules.fields.all_channels")}
              </Badge>
            ),
        }),
        createColumn<CapacityRule>({
          id: "product",
          accessorFn: (row) => row.product?.code ?? "",
          labelKey: "availability.rules.columns.product",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.product ? (
              <span>
                <code className="text-xs">{row.original.product.code}</code>{" "}
                {row.original.product.title}
              </span>
            ) : (
              <Badge variant="secondary">
                {t("availability.rules.fields.all_products")}
              </Badge>
            ),
        }),
        createColumn<CapacityRule>({
          id: "branch",
          accessorFn: (row) => row.branch?.code ?? "",
          labelKey: "availability.rules.columns.branch",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.branch ? (
              `${row.original.branch.code} · ${row.original.branch.name}`
            ) : (
              <Badge variant="secondary">
                {t("availability.rules.fields.all_branches")}
              </Badge>
            ),
        }),
        createColumn<CapacityRule>({
          id: "quantity",
          accessorFn: (row) => row.quantity,
          labelKey: "availability.rules.columns.quantity",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.quantity}</span>
          ),
        }),
        createColumn<CapacityRule>({
          id: "is_active",
          accessorFn: (row) => String(row.is_active),
          labelKey: "availability.rules.columns.is_active",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.is_active
                  ? t("fleet.active.true")
                  : t("fleet.active.false")
              }
              tone={row.original.is_active ? "success" : "default"}
            />
          ),
        }),
        createColumn<CapacityRule>({
          id: "note",
          accessorFn: (row) => row.note,
          labelKey: "availability.rules.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        ...auditColumns<CapacityRule>(
          {
            createdAt: "availability.rules.columns.created_at",
            uuid: "availability.rules.columns.uuid",
          },
          locale,
        ),
        createColumn<CapacityRule>({
          id: "actions",
          labelKey: "availability.rules.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const rule = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("availability.rules.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () => setDrawer({ rule }),
                  },
                  {
                    id: "toggle-active",
                    label: rule.is_active
                      ? t("availability.rules.actions.deactivate")
                      : t("availability.rules.actions.activate"),
                    icon: rule.is_active ? PowerOff : Power,
                    permission: write,
                    onSelect: () =>
                      save.mutate({
                        uuid: rule.uuid,
                        body: {
                          rule_type: rule.rule_type,
                          channel: rule.channel,
                          product_uuid: rule.product?.uuid ?? null,
                          branch_uuid: rule.branch?.uuid ?? null,
                          quantity: rule.quantity,
                          note: rule.note,
                          is_active: !rule.is_active,
                        },
                      }),
                  },
                  {
                    id: "delete",
                    label: t("availability.rules.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(rule),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<CapacityRule, unknown>[],
    [locale, save, t, write],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Boolean(
    listParams.q || listParams.rule_type || listParams.channel,
  );

  return (
    <EntityPage
      title={t("availability.rules.title")}
      description={t("availability.rules.description")}
      permission={permissions.tenant.capacityRules.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("availability.rules.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleet.title") },
        { label: t("availability.rules.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setDrawer({ rule: null })}
          label={t("availability.rules.actions.create")}
          permission={write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(rule) => setDrawer({ rule })}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("availability.rules.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() => listQuery.refetch())}
        emptyTitle={
          hasFilters
            ? t("availability.rules.empty_filtered_title")
            : t("availability.rules.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("availability.rules.empty_filtered_description")
            : t("availability.rules.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-fleet-capacity-rules-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />

      <CapacityRuleDrawer
        open={Boolean(drawer)}
        rule={drawer?.rule ?? null}
        onOpenChange={(open) => {
          if (!open) setDrawer(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget ? t(RULE_TYPE_LABEL_KEYS[deleteTarget.rule_type]) : ""
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
    </EntityPage>
  );
}
