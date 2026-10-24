"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Boxes,
  GitMerge,
  ImageIcon,
  Layers,
  Link2,
  Pencil,
  Split,
  Tags,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { BrandMark } from "@/components/common/brand-mark";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Toggle } from "@/components/ui/toggle";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  DisplayBrandEditDialog,
  DisplayBrandLogoDialog,
  DisplayBrandMergeDialog,
  DisplayBrandUnmergeDialog,
} from "@/features/catalog/components/display-brand-dialogs";
import { DisplayBrandMotomarksDialog } from "@/features/catalog/components/display-brand-motomarks-dialog";
import { CATALOG_LIVE, firstFilterValue } from "@/features/catalog/constants";
import {
  useCatalogDisplayBrands,
  useCatalogMeta,
} from "@/features/catalog/hooks/use-catalog";
import type {
  CatalogDisplayBrand,
  ListDisplayBrandsParams,
} from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

type Dialog = "edit" | "merge" | "unmerge" | "logo" | "motomarks";

/** "Görünen markalar": display brands with merge / unmerge / rename / logo. */
export function DisplayBrandsTable() {
  const { t } = useLocale();
  const router = useRouter();
  const [mergedOnly, setMergedOnly] = useState(false);
  const [dialog, setDialog] = useState<{
    kind: Dialog;
    brand: CatalogDisplayBrand;
  } | null>(null);
  const listState = useServerListState({
    initialSort: "name",
    initialPageSize: 50,
    realtime: { resource: CATALOG_LIVE.brands },
  });
  useCatalogMeta("display-brands");

  const params = useMemo<ListDisplayBrandsParams>(
    () => ({
      ...listState.params,
      q:
        listState.params.q || firstFilterValue(listState.columnFilters, "name"),
      is_active: firstFilterValue(listState.columnFilters, "is_active"),
      merged: mergedOnly ? "true" : undefined,
    }),
    [listState.columnFilters, listState.params, mergedOnly],
  );
  const listQuery = useCatalogDisplayBrands(params);
  const open = (kind: Dialog, brand: CatalogDisplayBrand) =>
    setDialog({ kind, brand });
  const close = (open: boolean) => {
    if (!open) setDialog(null);
  };

  const columns = useMemo(
    () =>
      [
        createColumn<CatalogDisplayBrand>({
          accessorKey: "name",
          labelKey: "catalog.columns.display_name",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          size: 240,
          cell: ({ row }) => {
            return (
              <div className="flex items-center gap-2">
                <BrandMark
                  name={row.original.name}
                  logoUrl={row.original.logo_badge_url ?? row.original.logo_url}
                  size="md"
                />
                <div className="min-w-0">
                  <p className="truncate font-medium">{row.original.name}</p>
                  <p className="text-muted-foreground truncate font-mono text-xs">
                    {row.original.slug}
                  </p>
                </div>
              </div>
            );
          },
        }),
        createColumn<CatalogDisplayBrand>({
          id: "tsb_names",
          accessorFn: (row) => row.tsb_names.join(", "),
          labelKey: "catalog.columns.tsb_brands",
          gridSecondary: true,
          size: 260,
          cell: ({ row }) => (
            <div className="flex flex-wrap gap-1">
              {row.original.tsb_names.map((name) => (
                <Badge
                  key={name}
                  variant="outline"
                  className="font-mono text-xs font-normal"
                >
                  {name}
                </Badge>
              ))}
            </div>
          ),
        }),
        createColumn<CatalogDisplayBrand>({
          accessorKey: "brands_count",
          labelKey: "catalog.columns.brands_count",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.brands_count > 1 ? (
              <Badge variant="secondary" className="gap-1 tabular-nums">
                <Layers className="size-3" />
                {row.original.brands_count}
              </Badge>
            ) : (
              <span className="text-muted-foreground tabular-nums">1</span>
            ),
        }),
        createColumn<CatalogDisplayBrand>({
          accessorKey: "models_count",
          labelKey: "catalog.columns.models_count",
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.models_count}</span>
          ),
        }),
        createColumn<CatalogDisplayBrand>({
          accessorKey: "types_count",
          labelKey: "catalog.columns.types_count",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.types_count}</span>
          ),
        }),
        createColumn<CatalogDisplayBrand>({
          id: "logo_source",
          accessorFn: (row) => row.motomarks?.match_source ?? "none",
          labelKey: "catalog.columns.logo_source",
          cell: ({ row }) => {
            const mm = row.original.motomarks;
            const source = mm?.match_source ?? "none";
            return (
              <div className="flex flex-wrap items-center gap-1">
                {row.original.logo_uploaded ? (
                  <Badge variant="secondary">
                    {t("catalog.logos.uploaded")}
                  </Badge>
                ) : null}
                <StatusChip
                  label={
                    t(`catalog.logos.match_source.${source}`) +
                    (mm?.match_score != null && source === "auto"
                      ? ` · ${Math.round(mm.match_score)}`
                      : "")
                  }
                  tone={
                    source === "auto" || source === "manual"
                      ? "success"
                      : source === "skip"
                        ? "default"
                        : "warning"
                  }
                />
              </div>
            );
          },
        }),
        createColumn<CatalogDisplayBrand>({
          id: "is_active",
          accessorFn: (row) => String(row.is_active),
          labelKey: "catalog.columns.is_active",
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "true",
              label: "true",
              labelKey: "catalog.active.true",
            },
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
        createColumn<CatalogDisplayBrand>({
          id: "actions",
          labelKey: "catalog.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "models",
                  label: t("catalog.display_brands.view_models"),
                  icon: Boxes,
                  onSelect: () =>
                    router.push(
                      `${routes.platform.catalog.models}?display_brand=${row.original.uuid}`,
                    ),
                },
                {
                  id: "tsb",
                  label: t("catalog.display_brands.view_tsb_brands"),
                  icon: Tags,
                  onSelect: () =>
                    router.push(
                      `${routes.platform.catalog.brands}?display_brand=${row.original.uuid}`,
                    ),
                },
                {
                  id: "edit",
                  label: t("catalog.actions.edit"),
                  icon: Pencil,
                  permission: permissions.catalog.write,
                  onSelect: () => open("edit", row.original),
                },
                {
                  id: "logo",
                  label: t("catalog.display_brands.logo_action"),
                  icon: ImageIcon,
                  permission: permissions.catalog.write,
                  onSelect: () => open("logo", row.original),
                },
                {
                  id: "motomarks",
                  label: t("catalog.logos.link_title"),
                  icon: Link2,
                  permission: permissions.catalog.write,
                  onSelect: () => open("motomarks", row.original),
                },
                {
                  id: "merge",
                  label: t("catalog.display_brands.merge_action"),
                  icon: GitMerge,
                  permission: permissions.catalog.write,
                  onSelect: () => open("merge", row.original),
                },
                ...(row.original.brands_count > 1
                  ? [
                      {
                        id: "unmerge",
                        label: t("catalog.actions.unmerge"),
                        icon: Split,
                        permission: permissions.catalog.write,
                        onSelect: () => open("unmerge", row.original),
                      },
                    ]
                  : []),
              ]}
            />
          ),
        }),
      ] as ColumnDef<CatalogDisplayBrand, unknown>[],
    [router, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil(
      (listQuery.data?.total ?? 0) / (listState.pagination.pageSize || 50),
    ),
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(
            `${routes.platform.catalog.models}?display_brand=${row.uuid}`,
          )
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("catalog.display_brands.empty_title")}
        emptyDescription={t("catalog.empty_description")}
        pageCount={pageCount}
        pageSizeOptions={[20, 50, 100]}
        state={listState.tableState}
        realtime={listState.realtimeFor(listQuery.refetch)}
        features={{
          persistKey: "platform-catalog-display-brands-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <Toggle
              variant="outline"
              size="sm"
              pressed={mergedOnly}
              onPressedChange={(pressed) => {
                setMergedOnly(pressed);
                listState.setPagination((prev) => ({ ...prev, pageIndex: 0 }));
              }}
              aria-label={t("catalog.display_brands.merged_only")}
            >
              <Layers className="size-4" />
              {t("catalog.display_brands.merged_only")}
            </Toggle>
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <DisplayBrandEditDialog
        brand={dialog?.kind === "edit" ? dialog.brand : null}
        onOpenChange={close}
      />
      <DisplayBrandMergeDialog
        brand={dialog?.kind === "merge" ? dialog.brand : null}
        onOpenChange={close}
      />
      <DisplayBrandUnmergeDialog
        brand={dialog?.kind === "unmerge" ? dialog.brand : null}
        onOpenChange={close}
      />
      <DisplayBrandLogoDialog
        brand={dialog?.kind === "logo" ? dialog.brand : null}
        onOpenChange={close}
      />
      <DisplayBrandMotomarksDialog
        brand={
          dialog?.kind === "motomarks"
            ? {
                uuid: dialog.brand.uuid,
                name: dialog.brand.name,
                logo_url: dialog.brand.logo_url,
                motomarks: dialog.brand.motomarks,
                suggestion: dialog.brand.motomarks?.suggested_id
                  ? {
                      id: dialog.brand.motomarks.suggested_id,
                      name: dialog.brand.motomarks.suggested_id,
                      score: dialog.brand.motomarks.suggested_score,
                    }
                  : null,
              }
            : null
        }
        onOpenChange={close}
      />
    </>
  );
}
