"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
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
import { ExtraDrawer } from "@/features/extras/components/extra-drawer";
import { ExtraStatusChip } from "@/features/extras/components/extras-shared";
import {
  CHARGE_LABEL_KEYS,
  CHARGE_TYPES,
  EXTRAS_RESOURCE,
  EXTRA_STATUSES,
  EXTRA_TYPES,
  STATUS_LABEL_KEYS,
  TYPE_LABEL_KEYS,
} from "@/features/extras/constants";
import {
  useDeleteExtra,
  useExtraFacets,
  useExtrasList,
  useExtrasMeta,
} from "@/features/extras/hooks/use-extras";
import type {
  Extra,
  ExtraFacets,
  ListExtrasParams,
} from "@/features/extras/types";
import {
  activeFilter,
  auditColumns,
  CodeBadge,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import { moneyText } from "@/features/pricing/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

type FacetList = ExtraFacets[keyof ExtraFacets];

function countOf(facets: FacetList | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

/**
 * Extras catalog ("Ek hizmetler"): additional driver, child seat, km
 * package, delivery … with the default price, charge type and publish
 * status. Full tenant list standard: facets, live updates, export, bulk
 * publish / archive.
 */
export function ExtrasPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, EXTRAS_RESOURCE),
  });
  const [drawer, setDrawer] = useState<{ extra: Extra | null } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Extra | null>(null);
  const write = permissions.tenant.extras.write;

  const listParams = useMemo<ListExtrasParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      extra_type: joinFilter(column("extra_type")),
      charge_type: joinFilter(column("charge_type")),
      status: joinFilter(column("status")),
      is_physical: activeFilter(column("is_physical")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useExtrasList(listParams);
  const metaQuery = useExtrasMeta();
  const facetsQuery = useExtraFacets();
  const remove = useDeleteExtra();
  const facets = facetsQuery.data;

  const baseColumns = useMemo(
    () =>
      [
        createColumn<Extra>({
          accessorKey: "code",
          labelKey: "extras.columns.code",
          enableSorting: true,
          cell: ({ row }) => <CodeBadge code={row.original.code} />,
        }),
        createColumn<Extra>({
          accessorKey: "name",
          labelKey: "extras.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<Extra>({
          accessorKey: "extra_type",
          labelKey: "extras.columns.extra_type",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: EXTRA_TYPES.map((v) => ({
            value: v,
            labelKey: TYPE_LABEL_KEYS[v],
            label: `${t(TYPE_LABEL_KEYS[v])} (${countOf(facets?.extra_type, v)})`,
          })),
          cell: ({ row }) => t(TYPE_LABEL_KEYS[row.original.extra_type]),
        }),
        createColumn<Extra>({
          accessorKey: "charge_type",
          labelKey: "extras.columns.charge_type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: CHARGE_TYPES.map((v) => ({
            value: v,
            labelKey: CHARGE_LABEL_KEYS[v],
            label: `${t(CHARGE_LABEL_KEYS[v])} (${countOf(facets?.charge_type, v)})`,
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(CHARGE_LABEL_KEYS[row.original.charge_type])}
            </Badge>
          ),
        }),
        createColumn<Extra>({
          id: "default_price",
          accessorFn: (row) => row.default_price ?? "",
          labelKey: "extras.columns.default_price",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.default_price, locale)}
            </span>
          ),
        }),
        createColumn<Extra>({
          accessorKey: "product_price_count",
          labelKey: "extras.columns.product_price_count",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.product_price_count}
            </span>
          ),
        }),
        createColumn<Extra>({
          id: "max_total",
          accessorFn: (row) => row.max_total ?? "",
          labelKey: "extras.columns.max_total",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => moneyText(row.original.max_total, locale),
        }),
        createColumn<Extra>({
          id: "is_physical",
          accessorFn: (row) => String(row.is_physical),
          labelKey: "extras.columns.is_physical",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "true",
              labelKey: "extras.physical.true",
              label: `${t("extras.physical.true")} (${countOf(facets?.is_physical, "true")})`,
            },
            {
              value: "false",
              labelKey: "extras.physical.false",
              label: `${t("extras.physical.false")} (${countOf(facets?.is_physical, "false")})`,
            },
          ],
          cell: ({ row }) =>
            row.original.is_physical ? (
              <StatusChip label={t("extras.physical.true")} tone="success" />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Extra>({
          accessorKey: "stock_total",
          labelKey: "extras.columns.stock_total",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.is_physical ? (
              <span className="tabular-nums">{row.original.stock_total}</span>
            ) : (
              "—"
            ),
        }),
        createColumn<Extra>({
          accessorKey: "status",
          labelKey: "extras.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: EXTRA_STATUSES.map((v) => ({
            value: v,
            labelKey: STATUS_LABEL_KEYS[v],
            label: `${t(STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
          })),
          cell: ({ row }) => <ExtraStatusChip status={row.original.status} />,
        }),
        createColumn<Extra>({
          accessorKey: "sort_order",
          labelKey: "extras.columns.sort_order",
          enableSorting: true,
          defaultHidden: true,
        }),
        ...auditColumns<Extra>(
          {
            createdAt: "extras.columns.created_at",
            uuid: "extras.columns.uuid",
          },
          locale,
        ),
        createColumn<Extra>({
          id: "actions",
          labelKey: "extras.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("extras.actions.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.pricing.extras.detail(
                        slug,
                        row.original.uuid,
                      ),
                    ),
                },
                {
                  id: "edit",
                  label: t("extras.actions.edit"),
                  icon: Pencil,
                  permission: write,
                  onSelect: () => setDrawer({ extra: row.original }),
                },
                {
                  id: "delete",
                  label: t("extras.actions.delete"),
                  icon: Trash2,
                  permission: write,
                  variant: "destructive",
                  onSelect: () => setDeleteTarget(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<Extra, unknown>[],
    [facets, locale, router, slug, t, write],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<Extra>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export and the "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      extra_type: listParams.extra_type,
      charge_type: listParams.charge_type,
      status: listParams.status,
      is_physical: listParams.is_physical,
      sort: listParams.sort,
    }),
    [
      listParams.charge_type,
      listParams.extra_type,
      listParams.is_physical,
      listParams.q,
      listParams.sort,
      listParams.status,
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
      resolveBulkActionsWithIcons(EXTRAS_RESOURCE, meta?.bulk_actions ?? []),
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
      title={t("extras.title")}
      description={t("extras.description")}
      permission={permissions.tenant.extras.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("extras.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("layout.section_pricing") },
        { label: t("extras.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setDrawer({ extra: null })}
          label={t("extras.actions.create")}
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
        onRowClick={(extra) =>
          router.push(routes.tenant.pricing.extras.detail(slug, extra.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("extras.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("extras.empty_filtered_title")
            : t("extras.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("extras.empty_filtered_description")
            : t("extras.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-pricing-extras-v1",
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={EXTRAS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={EXTRAS_RESOURCE}
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

      <ExtraDrawer
        open={Boolean(drawer)}
        extra={drawer?.extra ?? null}
        onOpenChange={(open) => {
          if (!open) setDrawer(null);
        }}
        onCreated={(extra) =>
          router.push(routes.tenant.pricing.extras.detail(slug, extra.uuid))
        }
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget ? `${deleteTarget.code} · ${deleteTarget.name}` : ""
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
