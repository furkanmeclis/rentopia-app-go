"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ListTree, Pencil } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
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
import { BrandEditDialog } from "@/features/catalog/components/brand-edit-dialog";
import {
  CatalogTabs,
  useCatalogBreadcrumbs,
} from "@/features/catalog/components/catalog-tabs";
import {
  CATALOG_CATEGORIES,
  firstFilterValue,
  CATALOG_LIVE,
} from "@/features/catalog/constants";
import {
  useCatalogBrands,
  useCatalogMeta,
} from "@/features/catalog/hooks/use-catalog";
import type {
  CatalogBrand,
  ListBrandsParams,
} from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

export function CatalogBrandsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const breadcrumbs = useCatalogBreadcrumbs(t("catalog.tabs.brands"));
  const [editing, setEditing] = useState<CatalogBrand | null>(null);
  const listState = useServerListState({
    initialSort: "display_name",
    initialPageSize: 50,
    realtime: { resource: CATALOG_LIVE.brands },
  });

  const params = useMemo<ListBrandsParams>(
    () => ({
      ...listState.params,
      q:
        listState.params.q ||
        firstFilterValue(listState.columnFilters, "display_name"),
      category: firstFilterValue(listState.columnFilters, "category"),
      is_active: firstFilterValue(listState.columnFilters, "is_active"),
    }),
    [listState.columnFilters, listState.params],
  );
  const listQuery = useCatalogBrands(params);
  useCatalogMeta("brands");

  const columns = useMemo(
    () =>
      [
        createColumn<CatalogBrand>({
          accessorKey: "display_name",
          labelKey: "catalog.columns.display_name",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.display_name}</span>
          ),
        }),
        createColumn<CatalogBrand>({
          accessorKey: "tsb_name",
          labelKey: "catalog.columns.tsb_name",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground font-mono text-xs">
              {row.original.tsb_name}
            </span>
          ),
        }),
        createColumn<CatalogBrand>({
          accessorKey: "tsb_code",
          labelKey: "catalog.columns.tsb_code",
          enableSorting: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.tsb_code}</code>
          ),
        }),
        createColumn<CatalogBrand>({
          accessorKey: "category",
          labelKey: "catalog.columns.category",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CATALOG_CATEGORIES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.category.${value}`,
          })),
          cell: ({ row }) => (
            <Badge variant="secondary" className="font-normal">
              {t(`catalog.category.${row.original.category}`)}
            </Badge>
          ),
        }),
        createColumn<CatalogBrand>({
          id: "is_active",
          accessorFn: (row) => String(row.is_active),
          labelKey: "catalog.columns.is_active",
          filterVariant: "faceted",
          filterOptions: [
            { value: "true", label: "true", labelKey: "catalog.active.true" },
            {
              value: "false",
              label: "false",
              labelKey: "catalog.active.false",
            },
          ],
          cell: ({ row }) => (
            <StatusChip
              label={t(`catalog.active.${String(row.original.is_active)}`)}
              tone={row.original.is_active ? "success" : "default"}
            />
          ),
        }),
        createColumn<CatalogBrand>({
          accessorKey: "types_count",
          labelKey: "catalog.columns.types_count",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.types_count}</span>
          ),
        }),
        createColumn<CatalogBrand>({
          accessorKey: "models_count",
          labelKey: "catalog.columns.models_count",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.models_count}</span>
          ),
        }),
        createColumn<CatalogBrand>({
          accessorKey: "unresolved_count",
          labelKey: "catalog.columns.unresolved_count",
          cell: ({ row }) =>
            row.original.unresolved_count > 0 ? (
              <Badge variant="warning" className="tabular-nums">
                {row.original.unresolved_count}
              </Badge>
            ) : (
              <span className="text-muted-foreground">0</span>
            ),
        }),
        createColumn<CatalogBrand>({
          id: "actions",
          labelKey: "catalog.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "types",
                  label: t("catalog.brands.view_types"),
                  icon: ListTree,
                  onSelect: () =>
                    router.push(
                      `${routes.platform.catalog.types}?brand=${row.original.uuid}`,
                    ),
                },
                {
                  id: "edit",
                  label: t("catalog.actions.edit"),
                  icon: Pencil,
                  permission: permissions.catalog.write,
                  onSelect: () => setEditing(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CatalogBrand, unknown>[],
    [router, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil(
      (listQuery.data?.total ?? 0) / (listState.pagination.pageSize || 50),
    ),
  );

  return (
    <EntityPage
      title={t("catalog.title")}
      description={t("catalog.description")}
      permission={permissions.catalog.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.forbidden")}
        />
      }
      breadcrumbs={breadcrumbs}
    >
      <CatalogTabs active="brands" />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(`${routes.platform.catalog.models}?brand=${row.uuid}`)
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(listQuery.refetch)}
        emptyTitle={t("catalog.brands.empty_title")}
        emptyDescription={t("catalog.empty_description")}
        pageCount={pageCount}
        pageSizeOptions={[20, 50, 100]}
        state={listState.tableState}
        features={{
          persistKey: "platform-catalog-brands-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
      <BrandEditDialog
        brand={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      />
    </EntityPage>
  );
}
