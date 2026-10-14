"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { GitMerge, ListChecks, ListTree, Pencil } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
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
  CatalogTabs,
  useCatalogBreadcrumbs,
} from "@/features/catalog/components/catalog-tabs";
import {
  ModelCreateDialog,
  ModelMergeDialog,
  ModelRenameDialog,
  ModelRulesConfirmDialog,
} from "@/features/catalog/components/model-dialogs";
import {
  CATALOG_MODEL_SOURCES,
  firstFilterValue,
  CATALOG_LIVE,
} from "@/features/catalog/constants";
import {
  useCatalogBrandOptions,
  useCatalogDisplayBrandOptions,
  useCatalogMeta,
  useCatalogModels,
} from "@/features/catalog/hooks/use-catalog";
import type {
  CatalogModel,
  ListModelsParams,
} from "@/features/catalog/services/catalog.service";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

type CatalogModelsPageProps = {
  initialBrand?: string;
  initialDisplayBrand?: string;
};

export function CatalogModelsPage({
  initialBrand,
  initialDisplayBrand,
}: CatalogModelsPageProps) {
  const { t } = useLocale();
  const router = useRouter();
  const { can } = usePermission();
  const canWrite = can(permissions.catalog.write);
  const breadcrumbs = useCatalogBreadcrumbs(t("catalog.tabs.models"));
  const [creating, setCreating] = useState(false);
  const [renaming, setRenaming] = useState<CatalogModel | null>(null);
  const [merging, setMerging] = useState<CatalogModel | null>(null);
  const [ruleTargets, setRuleTargets] = useState<{
    models: CatalogModel[];
    reset: () => void;
  } | null>(null);
  const listState = useServerListState({
    initialSort: "brand",
    initialPageSize: 50,
    initialColumnFilters: [
      ...(initialBrand ? [{ id: "brand", value: [initialBrand] }] : []),
      ...(initialDisplayBrand
        ? [{ id: "display_brand", value: [initialDisplayBrand] }]
        : []),
    ],
    realtime: { resource: CATALOG_LIVE.models },
  });
  const brands = useCatalogBrandOptions();
  const displayBrands = useCatalogDisplayBrandOptions();
  useCatalogMeta("models");

  const brandFilter = firstFilterValue(listState.columnFilters, "brand");
  const displayFilter = firstFilterValue(
    listState.columnFilters,
    "display_brand",
  );
  const params = useMemo<ListModelsParams>(
    () => ({
      ...listState.params,
      q:
        listState.params.q || firstFilterValue(listState.columnFilters, "name"),
      brand: brandFilter,
      display_brand: displayFilter,
      source: firstFilterValue(listState.columnFilters, "source"),
    }),
    [brandFilter, displayFilter, listState.columnFilters, listState.params],
  );
  const listQuery = useCatalogModels(params);

  const brandOptions = useMemo(
    () =>
      (brands.data ?? []).map((b) => ({
        value: b.uuid,
        label: `${b.display_name} (${b.tsb_name})`,
      })),
    [brands.data],
  );
  const displayOptions = useMemo(
    () =>
      (displayBrands.data ?? []).map((d) => ({
        value: d.uuid,
        label: d.name,
      })),
    [displayBrands.data],
  );

  const columns = useMemo(
    () =>
      [
        ...(canWrite ? [createSelectColumnDef<CatalogModel>()] : []),
        createColumn<CatalogModel>({
          accessorKey: "name",
          labelKey: "catalog.columns.model",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-2">
              <span className="font-medium">{row.original.name}</span>
              {row.original.group_size > 1 ? (
                <Badge
                  variant="secondary"
                  className="font-normal"
                  title={t("catalog.models.grouped_hint", {
                    count: row.original.group_size,
                  })}
                >
                  {t("catalog.models.grouped", {
                    count: row.original.group_size,
                  })}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<CatalogModel>({
          id: "display_brand",
          accessorFn: (row) => row.display_brand.uuid,
          labelKey: "catalog.columns.display_brand",
          filterVariant: "faceted",
          filterOptions: displayOptions,
          gridSecondary: true,
          cell: ({ row }) => row.original.display_brand.name,
        }),
        createColumn<CatalogModel>({
          id: "brand",
          accessorFn: (row) => row.brand.uuid,
          labelKey: "catalog.columns.tsb_brand",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: brandOptions,
          cell: ({ row }) => (
            <span className="text-muted-foreground font-mono text-xs">
              {row.original.brand.tsb_name ?? row.original.brand.display_name}
            </span>
          ),
        }),
        createColumn<CatalogModel>({
          accessorKey: "source",
          labelKey: "catalog.columns.source",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CATALOG_MODEL_SOURCES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.source.${value}`,
          })),
          cell: ({ row }) => (
            <Badge
              variant={row.original.source === "auto" ? "outline" : "secondary"}
              className="font-normal"
            >
              {t(`catalog.source.${row.original.source}`)}
            </Badge>
          ),
        }),
        createColumn<CatalogModel>({
          accessorKey: "types_count",
          labelKey: "catalog.columns.types_count",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.types_count}</span>
          ),
        }),
        createColumn<CatalogModel>({
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
                  label: t("catalog.models.view_types"),
                  icon: ListTree,
                  onSelect: () =>
                    router.push(
                      `${routes.platform.catalog.types}?model=${row.original.uuid}`,
                    ),
                },
                {
                  id: "rename",
                  label: t("catalog.actions.rename"),
                  icon: Pencil,
                  permission: permissions.catalog.write,
                  onSelect: () => setRenaming(row.original),
                },
                {
                  id: "merge",
                  label: t("catalog.actions.merge"),
                  icon: GitMerge,
                  permission: permissions.catalog.write,
                  onSelect: () => setMerging(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CatalogModel, unknown>[],
    [brandOptions, canWrite, displayOptions, router, t],
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
      actions={
        <EntityCreateButton
          onClick={() => setCreating(true)}
          label={t("catalog.actions.create_model")}
          permission={permissions.catalog.write}
        />
      }
    >
      <CatalogTabs active="models" />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(`${routes.platform.catalog.types}?model=${row.uuid}`)
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(listQuery.refetch)}
        emptyTitle={t("catalog.models.empty_title")}
        emptyDescription={t("catalog.empty_description")}
        pageCount={pageCount}
        pageSizeOptions={[20, 50, 100]}
        state={listState.tableState}
        features={{
          persistKey: "platform-catalog-models-v2",
          rowSelection: canWrite,
        }}
        bulkActions={
          canWrite
            ? [
                {
                  id: "rules",
                  label: t("catalog.models.rules_action"),
                  icon: ListChecks,
                  onClick: (selected, table) =>
                    setRuleTargets({
                      models: selected,
                      reset: () => table.resetRowSelection(),
                    }),
                },
              ]
            : undefined
        }
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
      <ModelCreateDialog
        open={creating}
        defaultBrand={brandFilter}
        onOpenChange={setCreating}
      />
      <ModelRenameDialog
        model={renaming}
        onOpenChange={(open) => {
          if (!open) setRenaming(null);
        }}
      />
      <ModelMergeDialog
        model={merging}
        onOpenChange={(open) => {
          if (!open) setMerging(null);
        }}
      />
      <ModelRulesConfirmDialog
        models={ruleTargets?.models ?? null}
        onDone={(applied) => {
          if (applied) ruleTargets?.reset();
          setRuleTargets(null);
        }}
      />
    </EntityPage>
  );
}
