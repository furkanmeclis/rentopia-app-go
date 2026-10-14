"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, TriangleAlert } from "lucide-react";
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
import { Toggle } from "@/components/ui/toggle";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CatalogTabs,
  useCatalogBreadcrumbs,
} from "@/features/catalog/components/catalog-tabs";
import { TypeModelAssign } from "@/features/catalog/components/type-model-assign";
import {
  CATALOG_CATEGORIES,
  CATALOG_FUELS,
  CATALOG_TRANSMISSIONS,
  firstFilterValue,
  formatYears,
} from "@/features/catalog/constants";
import {
  catalogKeys,
  useCatalogBrandOptions,
  useCatalogMeta,
  useCatalogTypes,
} from "@/features/catalog/hooks/use-catalog";
import {
  catalogService,
  type CatalogType,
  type ListTypesParams,
} from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

type CatalogTypesPageProps = {
  initialBrand?: string;
  initialModel?: string;
  initialUnresolved?: boolean;
};

export function CatalogTypesPage({
  initialBrand,
  initialModel,
  initialUnresolved,
}: CatalogTypesPageProps) {
  const { t } = useLocale();
  const router = useRouter();
  const breadcrumbs = useCatalogBreadcrumbs(t("catalog.tabs.types"));
  const [unresolved, setUnresolved] = useState(Boolean(initialUnresolved));
  const listState = useServerListState({
    initialSort: "brand",
    initialPageSize: 50,
    initialColumnFilters: [
      ...(initialBrand ? [{ id: "brand", value: [initialBrand] }] : []),
      ...(initialModel ? [{ id: "model", value: [initialModel] }] : []),
    ],
  });
  const brands = useCatalogBrandOptions();
  useCatalogMeta("types");

  const filters = listState.columnFilters;
  const brandFilter = firstFilterValue(filters, "brand");
  const modelFilter = firstFilterValue(filters, "model");
  const yearRaw = firstFilterValue(filters, "years");
  const year = yearRaw && /^\d{4}$/.test(yearRaw) ? Number(yearRaw) : undefined;

  const params = useMemo<ListTypesParams>(
    () => ({
      ...listState.params,
      q: listState.params.q || firstFilterValue(filters, "raw_name"),
      brand: brandFilter,
      model: modelFilter,
      category: firstFilterValue(filters, "category"),
      fuel: firstFilterValue(filters, "suggested_fuel"),
      transmission: firstFilterValue(filters, "suggested_transmission"),
      is_active: firstFilterValue(filters, "is_active"),
      year,
      unresolved: unresolved ? "true" : undefined,
    }),
    [brandFilter, filters, listState.params, modelFilter, unresolved, year],
  );
  const listQuery = useCatalogTypes(params);

  // Model facet: models of the selected brand, plus the model from the URL.
  const brandModels = useQuery({
    queryKey: [...catalogKeys.models(), "facet", brandFilter ?? ""],
    queryFn: () =>
      catalogService.listModels({
        brand: brandFilter,
        limit: 100,
        offset: 0,
        sort: "name",
      }),
    enabled: Boolean(brandFilter),
    staleTime: 60_000,
  });
  const urlModel = useQuery({
    queryKey: [...catalogKeys.models(), "detail", initialModel ?? ""],
    queryFn: () => catalogService.getModel(initialModel!),
    enabled: Boolean(initialModel),
    staleTime: 60_000,
  });

  const brandOptions = useMemo(
    () =>
      (brands.data ?? []).map((b) => ({
        value: b.uuid,
        label: `${b.display_name} (${b.tsb_name})`,
      })),
    [brands.data],
  );
  const modelOptions = useMemo(() => {
    const opts = (brandModels.data?.items ?? []).map((m) => ({
      value: m.uuid,
      label: m.name,
    }));
    if (urlModel.data && !opts.some((o) => o.value === urlModel.data.uuid)) {
      opts.unshift({
        value: urlModel.data.uuid,
        label: `${urlModel.data.brand.display_name} · ${urlModel.data.name}`,
      });
    }
    return opts;
  }, [brandModels.data, urlModel.data]);

  const columns = useMemo(
    () =>
      [
        createColumn<CatalogType>({
          accessorKey: "raw_name",
          labelKey: "catalog.columns.raw_name",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          size: 340,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.raw_name}</span>
          ),
        }),
        createColumn<CatalogType>({
          accessorKey: "tsb_type_code",
          labelKey: "catalog.columns.tsb_type_code",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.tsb_type_code}</code>
          ),
        }),
        createColumn<CatalogType>({
          id: "brand",
          accessorFn: (row) => row.brand.uuid,
          labelKey: "catalog.columns.brand",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: brandOptions,
          gridSecondary: true,
          cell: ({ row }) => row.original.brand.display_name,
        }),
        createColumn<CatalogType>({
          id: "model",
          accessorFn: (row) => row.model?.uuid ?? "",
          labelKey: "catalog.columns.model",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: modelOptions,
          cell: ({ row }) => <TypeModelAssign type={row.original} compact />,
        }),
        createColumn<CatalogType>({
          id: "years",
          accessorFn: (row) => formatYears(row.years),
          labelKey: "catalog.columns.years",
          filterVariant: "text",
          cell: ({ row }) => (
            <span className="tabular-nums">
              {formatYears(row.original.years)}
            </span>
          ),
        }),
        createColumn<CatalogType>({
          id: "category",
          accessorFn: () => "",
          labelKey: "catalog.columns.category",
          filterVariant: "faceted",
          // Filter-only column (brand category): never shown.
          defaultHidden: true,
          enableHiding: false,
          filterOptions: CATALOG_CATEGORIES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.category.${value}`,
          })),
          cell: () => null,
        }),
        createColumn<CatalogType>({
          id: "suggested_fuel",
          accessorFn: (row) => row.suggested_fuel ?? "",
          labelKey: "catalog.columns.fuel",
          filterVariant: "faceted",
          filterOptions: CATALOG_FUELS.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.fuel.${value}`,
          })),
          cell: ({ row }) =>
            row.original.suggested_fuel
              ? t(`catalog.fuel.${row.original.suggested_fuel}`)
              : "—",
        }),
        createColumn<CatalogType>({
          id: "suggested_transmission",
          accessorFn: (row) => row.suggested_transmission ?? "",
          labelKey: "catalog.columns.transmission",
          filterVariant: "faceted",
          filterOptions: CATALOG_TRANSMISSIONS.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.transmission.${value}`,
          })),
          cell: ({ row }) =>
            row.original.suggested_transmission
              ? t(`catalog.transmission.${row.original.suggested_transmission}`)
              : "—",
        }),
        createColumn<CatalogType>({
          id: "is_active",
          accessorFn: (row) => String(row.is_active),
          labelKey: "catalog.columns.is_active",
          filterVariant: "faceted",
          defaultHidden: true,
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
        createColumn<CatalogType>({
          accessorKey: "last_seen_period",
          labelKey: "catalog.columns.last_seen_period",
          enableSorting: true,
          defaultHidden: true,
        }),
        createColumn<CatalogType>({
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
                      routes.platform.catalog.typeDetail(row.original.uuid),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CatalogType, unknown>[],
    [brandOptions, modelOptions, router, t],
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
      <CatalogTabs active="types" />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.platform.catalog.typeDetail(row.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("catalog.types.empty_title")}
        emptyDescription={t("catalog.empty_description")}
        pageCount={pageCount}
        pageSizeOptions={[20, 50, 100]}
        state={listState.tableState}
        features={{
          persistKey: "platform-catalog-types-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <Toggle
              variant="outline"
              size="sm"
              pressed={unresolved}
              onPressedChange={(pressed) => {
                setUnresolved(pressed);
                listState.setPagination((prev) => ({ ...prev, pageIndex: 0 }));
              }}
              aria-label={t("catalog.filters.unresolved")}
            >
              <TriangleAlert className="size-4" />
              {t("catalog.filters.unresolved")}
            </Toggle>
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
