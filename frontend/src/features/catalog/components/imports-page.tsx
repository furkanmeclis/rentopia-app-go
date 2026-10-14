"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Upload } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CatalogTabs,
  useCatalogBreadcrumbs,
} from "@/features/catalog/components/catalog-tabs";
import { ImportUploadDialog } from "@/features/catalog/components/import-upload-dialog";
import {
  CATALOG_IMPORT_STATUSES,
  CATALOG_IMPORT_TONE,
  firstFilterValue,
} from "@/features/catalog/constants";
import {
  useCatalogImports,
  useCatalogMeta,
} from "@/features/catalog/hooks/use-catalog";
import type {
  CatalogImport,
  ListImportsParams,
} from "@/features/catalog/services/catalog.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function CatalogImportsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const breadcrumbs = useCatalogBreadcrumbs(t("catalog.tabs.imports"));
  const [uploadOpen, setUploadOpen] = useState(false);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  useCatalogMeta("imports");

  const params = useMemo<ListImportsParams>(
    () => ({
      ...listState.params,
      status: firstFilterValue(listState.columnFilters, "status"),
    }),
    [listState.columnFilters, listState.params],
  );
  const listQuery = useCatalogImports(params);

  const columns = useMemo(
    () =>
      [
        createColumn<CatalogImport>({
          accessorKey: "period",
          labelKey: "catalog.columns.period",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {row.original.period ?? "—"}
            </span>
          ),
        }),
        createColumn<CatalogImport>({
          accessorKey: "filename",
          labelKey: "catalog.columns.filename",
          gridSecondary: true,
        }),
        createColumn<CatalogImport>({
          accessorKey: "status",
          labelKey: "catalog.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CATALOG_IMPORT_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.status.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`catalog.status.${row.original.status}`)}
              tone={CATALOG_IMPORT_TONE[row.original.status]}
            />
          ),
        }),
        createColumn<CatalogImport>({
          id: "types_new",
          accessorFn: (row) => row.counts.types_new ?? 0,
          labelKey: "catalog.counts.types_new",
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.counts.types_new ?? 0}
            </span>
          ),
        }),
        createColumn<CatalogImport>({
          id: "valuations_upserted",
          accessorFn: (row) => row.counts.valuations_upserted ?? 0,
          labelKey: "catalog.counts.valuations_upserted",
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.counts.valuations_upserted ?? 0}
            </span>
          ),
        }),
        createColumn<CatalogImport>({
          id: "created_by_email",
          accessorFn: (row) => row.created_by_email ?? "",
          labelKey: "catalog.columns.created_by",
          defaultHidden: true,
        }),
        createColumn<CatalogImport>({
          accessorKey: "created_at",
          labelKey: "catalog.columns.created_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.created_at),
        }),
        createColumn<CatalogImport>({
          id: "actions",
          labelKey: "catalog.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("catalog.actions.view"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.platform.catalog.importDetail(row.original.uuid),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CatalogImport, unknown>[],
    [router, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil(
      (listQuery.data?.total ?? 0) / (listState.pagination.pageSize || 20),
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
        <PermissionGuard permission={permissions.catalog.write}>
          <Button size="sm" onClick={() => setUploadOpen(true)}>
            <Upload className="size-4" />
            {t("catalog.imports.upload")}
          </Button>
        </PermissionGuard>
      }
    >
      <CatalogTabs active="imports" />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.platform.catalog.importDetail(row.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("catalog.imports.empty_title")}
        emptyDescription={t("catalog.imports.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "platform-catalog-imports-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
      <ImportUploadDialog open={uploadOpen} onOpenChange={setUploadOpen} />
    </EntityPage>
  );
}
