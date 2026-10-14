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
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import {
  activeColumn,
  activeFilter,
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  MoneyCell,
  RouteCell,
} from "@/features/fleet-ops/components/fleet-ops-cells";
import { OneWayRuleDrawer } from "@/features/fleet-ops/components/one-way-rule-drawer";
import { ONE_WAY_RULES_RESOURCE } from "@/features/fleet-ops/constants";
import {
  useDeleteOneWayRule,
  useOneWayRuleFacets,
  useOneWayRulesList,
  useOneWayRulesMeta,
  useUpdateOneWayRule,
} from "@/features/fleet-ops/hooks/use-fleet-ops";
import type {
  ListOneWayRulesParams,
  OneWayRule,
  OneWayRuleFacets,
} from "@/features/fleet-ops/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

type FacetList = OneWayRuleFacets[keyof OneWayRuleFacets];

function countOf(facets: FacetList | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

function referenceOptions(facets: FacetList | undefined) {
  return (facets ?? []).map((facet) => ({
    value: facet.value,
    label: `${facet.label ?? facet.value} (${facet.count})`,
  }));
}

/**
 * One-way rules ("tek yön"): may a rental picked up at A be returned at B,
 * for which fee; a rental product rule overrides the generic one. Full
 * tenant list standard incl. bulk activate / deactivate and live updates.
 */
export function OneWayRulesPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, ONE_WAY_RULES_RESOURCE),
  });
  const [drawer, setDrawer] = useState<{ rule: OneWayRule | null } | null>(
    null,
  );
  const [deleteTarget, setDeleteTarget] = useState<OneWayRule | null>(null);
  const write = permissions.tenant.oneWay.write;

  const listParams = useMemo<ListOneWayRulesParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      from_branch: joinFilter(column("from_branch")),
      to_branch: joinFilter(column("to_branch")),
      allowed: activeFilter(column("allowed")),
      is_active: activeFilter(column("is_active")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useOneWayRulesList(listParams);
  const metaQuery = useOneWayRulesMeta();
  const facetsQuery = useOneWayRuleFacets();
  const update = useUpdateOneWayRule();
  const remove = useDeleteOneWayRule();
  const facets = facetsQuery.data;

  const baseColumns = useMemo(
    () =>
      [
        createColumn<OneWayRule>({
          id: "route",
          accessorFn: (row) => `${row.from_branch.code}-${row.to_branch.code}`,
          labelKey: "one_way_rules.columns.route",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <RouteCell
              from={row.original.from_branch}
              to={row.original.to_branch}
            />
          ),
        }),
        createColumn<OneWayRule>({
          id: "from_branch",
          accessorFn: (row) => row.from_branch.uuid,
          labelKey: "one_way_rules.columns.from_branch",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.from_branch),
          cell: ({ row }) => row.original.from_branch.name,
        }),
        createColumn<OneWayRule>({
          id: "to_branch",
          accessorFn: (row) => row.to_branch.uuid,
          labelKey: "one_way_rules.columns.to_branch",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.to_branch),
          cell: ({ row }) => row.original.to_branch.name,
        }),
        createColumn<OneWayRule>({
          id: "rental_product",
          accessorFn: (row) => row.rental_product?.uuid ?? "",
          labelKey: "one_way_rules.columns.rental_product",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.rental_product ? (
              <span>
                <code className="text-xs">
                  {row.original.rental_product.code}
                </code>{" "}
                {row.original.rental_product.title}
              </span>
            ) : (
              <Badge variant="secondary">
                {t("one_way_rules.fields.all_products")}
              </Badge>
            ),
        }),
        createColumn<OneWayRule>({
          id: "allowed",
          accessorFn: (row) => String(row.allowed),
          labelKey: "one_way_rules.columns.allowed",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "true",
              labelKey: "one_way_rules.allowed.true",
              label: `${t("one_way_rules.allowed.true")} (${countOf(facets?.allowed, "true")})`,
            },
            {
              value: "false",
              labelKey: "one_way_rules.allowed.false",
              label: `${t("one_way_rules.allowed.false")} (${countOf(facets?.allowed, "false")})`,
            },
          ],
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.allowed
                  ? t("one_way_rules.allowed.true")
                  : t("one_way_rules.allowed.false")
              }
              tone={row.original.allowed ? "success" : "danger"}
            />
          ),
        }),
        createColumn<OneWayRule>({
          accessorKey: "fee",
          labelKey: "one_way_rules.columns.fee",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.allowed ? <MoneyCell value={row.original.fee} /> : "—",
        }),
        createColumn<OneWayRule>({
          accessorKey: "note",
          labelKey: "one_way_rules.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        activeColumn<OneWayRule>(t, "one_way_rules.columns.is_active"),
        ...auditColumns<OneWayRule>(
          {
            createdAt: "one_way_rules.columns.created_at",
            uuid: "one_way_rules.columns.uuid",
          },
          locale,
        ),
        createColumn<OneWayRule>({
          id: "actions",
          labelKey: "one_way_rules.columns.actions",
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
                    label: t("one_way_rules.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () => setDrawer({ rule }),
                  },
                  {
                    id: "toggle-active",
                    label: rule.is_active
                      ? t("one_way_rules.actions.deactivate")
                      : t("one_way_rules.actions.activate"),
                    icon: rule.is_active ? PowerOff : Power,
                    permission: write,
                    onSelect: () =>
                      update.mutate({
                        uuid: rule.uuid,
                        body: { is_active: !rule.is_active },
                      }),
                  },
                  {
                    id: "delete",
                    label: t("one_way_rules.actions.delete"),
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
      ] as ColumnDef<OneWayRule, unknown>[],
    [facets, locale, t, update, write],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<OneWayRule>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export and the "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      from_branch: listParams.from_branch,
      to_branch: listParams.to_branch,
      allowed: listParams.allowed,
      is_active: listParams.is_active,
      sort: listParams.sort,
    }),
    [
      listParams.allowed,
      listParams.from_branch,
      listParams.is_active,
      listParams.q,
      listParams.sort,
      listParams.to_branch,
    ],
  );

  const total = listQuery.data?.total ?? 0;
  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery: engineQuery,
    total,
  });
  const meta = metaQuery.data;
  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(
        ONE_WAY_RULES_RESOURCE,
        meta?.bulk_actions ?? [],
      ),
    [meta?.bulk_actions],
  );
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Object.entries(engineQuery).some(
    ([key, value]) => key !== "sort" && Boolean(value),
  );
  const refresh = () => {
    void listQuery.refetch();
    void facetsQuery.refetch();
  };

  return (
    <EntityPage
      title={t("one_way_rules.title")}
      description={t("one_way_rules.description")}
      permission={permissions.tenant.oneWay.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("one_way_rules.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleet.title") },
        { label: t("one_way_rules.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setDrawer({ rule: null })}
          label={t("one_way_rules.actions.create")}
          permission={write}
        />
      }
    >
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(rule) => setDrawer({ rule })}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("one_way_rules.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("one_way_rules.empty_filtered_title")
            : t("one_way_rules.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("one_way_rules.empty_filtered_description")
            : t("one_way_rules.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-fleet-one-way-rules-v1",
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={ONE_WAY_RULES_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={ONE_WAY_RULES_RESOURCE}
              query={engineQuery}
              capabilities={meta?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <OneWayRuleDrawer
        open={Boolean(drawer)}
        rule={drawer?.rule ?? null}
        onOpenChange={(open) => {
          if (!open) setDrawer(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget
            ? `${deleteTarget.from_branch.code} → ${deleteTarget.to_branch.code}`
            : ""
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
