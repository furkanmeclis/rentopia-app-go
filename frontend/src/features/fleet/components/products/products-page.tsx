"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, PencilLine, Power, PowerOff, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

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
import { GuaranteeBadgeChip } from "@/features/fleet/components/products/guarantee-badge";
import { mediaSrc } from "@/features/fleet/components/products/product-card-preview";
import { ProductQuickEditDrawer } from "@/features/fleet/components/products/product-quick-edit-drawer";
import {
  activeColumn,
  activeFilter,
  auditColumns,
  CodeBadge,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  GUARANTEE_LABEL_KEYS,
  GUARANTEE_TYPES,
  PRODUCT_STATUSES,
  RENTAL_PRODUCTS_RESOURCE,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/fleet/constants";
import {
  useDeleteProduct,
  useProductFacets,
  useProductsList,
  useProductsMeta,
  useUpdateProduct,
} from "@/features/fleet/hooks/use-fleet";
import type {
  FleetFacet,
  ListProductsParams,
  RentalProduct,
} from "@/features/fleet/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function countOf(facets: FleetFacet[] | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

/**
 * Rentable products: what channels sell, with their guarantee level and
 * current version. Full tenant list standard incl. bulk activate / deactivate.
 */
export function ProductsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RENTAL_PRODUCTS_RESOURCE),
  });
  const [quickEdit, setQuickEdit] = useState<RentalProduct | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<RentalProduct | null>(null);

  const listParams = useMemo<ListProductsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      segment: joinFilter(column("segment")),
      status: joinFilter(column("status")),
      guarantee_type: joinFilter(column("guarantee_type")),
      is_active: activeFilter(column("is_active")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useProductsList(listParams);
  const metaQuery = useProductsMeta();
  const facetsQuery = useProductFacets();
  const update = useUpdateProduct();
  const remove = useDeleteProduct();
  const write = permissions.tenant.fleet.write;
  const facets = facetsQuery.data;

  const openDetail = useCallback(
    (product: RentalProduct) =>
      router.push(routes.tenant.fleet.products.detail(slug, product.uuid)),
    [router, slug],
  );

  const baseColumns = useMemo(
    () =>
      [
        createColumn<RentalProduct>({
          id: "image",
          labelKey: "fleet.products.columns.image",
          enableSorting: false,
          cell: ({ row }) => {
            const cover = row.original.images[0];
            const src = cover ? mediaSrc(cover.public_url) : null;
            return src ? (
              // eslint-disable-next-line @next/next/no-img-element -- API-owned media stream
              <img
                src={src}
                alt=""
                className="h-9 w-14 rounded object-cover"
                loading="lazy"
              />
            ) : (
              <div className="bg-muted h-9 w-14 rounded" />
            );
          },
        }),
        createColumn<RentalProduct>({
          accessorKey: "code",
          labelKey: "fleet.products.columns.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => <CodeBadge code={row.original.code} />,
        }),
        createColumn<RentalProduct>({
          accessorKey: "title",
          labelKey: "fleet.products.columns.title",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.title}</span>
          ),
        }),
        createColumn<RentalProduct>({
          id: "segment",
          accessorFn: (row) => row.segment.uuid,
          labelKey: "fleet.products.columns.segment",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.segment ?? []).map((facet) => ({
            value: facet.value,
            label: `${facet.label ?? facet.value} (${facet.count})`,
          })),
          cell: ({ row }) => row.original.segment.name,
        }),
        createColumn<RentalProduct>({
          id: "guarantee_type",
          accessorFn: (row) => row.version.guarantee.type,
          labelKey: "fleet.products.columns.guarantee_type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: GUARANTEE_TYPES.map((value) => ({
            value,
            labelKey: GUARANTEE_LABEL_KEYS[value],
            label: `${t(GUARANTEE_LABEL_KEYS[value])} (${countOf(facets?.guarantee_type, value)})`,
          })),
          cell: ({ row }) => (
            <GuaranteeBadgeChip badge={row.original.guarantee_badge} />
          ),
        }),
        createColumn<RentalProduct>({
          accessorKey: "display_text",
          labelKey: "fleet.products.columns.display_text",
          enableSorting: false,
          cell: ({ row }) => row.original.display_text,
        }),
        createColumn<RentalProduct>({
          accessorKey: "status",
          labelKey: "fleet.products.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: PRODUCT_STATUSES.map((value) => ({
            value,
            labelKey: STATUS_LABEL_KEYS[value],
            label: `${t(STATUS_LABEL_KEYS[value])} (${countOf(facets?.status, value)})`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[row.original.status])}
              tone={STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<RentalProduct>({
          accessorKey: "current_version",
          labelKey: "fleet.products.columns.current_version",
          enableSorting: true,
          cell: ({ row }) => (
            <Badge variant="outline" className="tabular-nums">
              v{row.original.current_version}
            </Badge>
          ),
        }),
        activeColumn<RentalProduct>(t, "fleet.products.columns.is_active"),
        createColumn<RentalProduct>({
          accessorKey: "updated_at",
          labelKey: "fleet.products.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.updated_at, undefined, locale),
        }),
        ...auditColumns<RentalProduct>(
          {
            createdAt: "fleet.products.columns.created_at",
            uuid: "fleet.products.columns.uuid",
          },
          locale,
        ),
        createColumn<RentalProduct>({
          id: "actions",
          labelKey: "fleet.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const product = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "view",
                    label: t("fleet.actions.view"),
                    icon: Eye,
                    onSelect: () => openDetail(product),
                  },
                  {
                    id: "quick-edit",
                    label: t("fleet.actions.quick_edit"),
                    icon: PencilLine,
                    permission: write,
                    onSelect: () => setQuickEdit(product),
                  },
                  {
                    id: "edit",
                    label: t("fleet.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () =>
                      router.push(
                        routes.tenant.fleet.products.edit(slug, product.uuid),
                      ),
                  },
                  {
                    id: "toggle-active",
                    label: product.is_active
                      ? t("fleet.actions.deactivate")
                      : t("fleet.actions.activate"),
                    icon: product.is_active ? PowerOff : Power,
                    permission: write,
                    onSelect: () =>
                      update.mutate({
                        uuid: product.uuid,
                        body: { is_active: !product.is_active },
                      }),
                  },
                  {
                    id: "delete",
                    label: t("fleet.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(product),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<RentalProduct, unknown>[],
    [facets, locale, openDetail, router, slug, t, update, write],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<RentalProduct>(), ...baseColumns],
    [baseColumns],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      segment: listParams.segment,
      status: listParams.status,
      guarantee_type: listParams.guarantee_type,
      is_active: listParams.is_active,
      sort: listParams.sort,
    }),
    [
      listParams.guarantee_type,
      listParams.is_active,
      listParams.q,
      listParams.segment,
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
      resolveBulkActionsWithIcons(
        RENTAL_PRODUCTS_RESOURCE,
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
      title={t("fleet.products.title")}
      description={t("fleet.products.description")}
      permission={permissions.tenant.fleet.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("fleet.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleet.title") },
        { label: t("fleet.products.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.fleet.products.create(slug))}
          label={t("fleet.products.actions.create")}
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
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("fleet.products.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("fleet.empty_filtered_title")
            : t("fleet.products.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("fleet.empty_filtered_description")
            : t("fleet.products.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-fleet-products-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={RENTAL_PRODUCTS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={RENTAL_PRODUCTS_RESOURCE}
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

      <ProductQuickEditDrawer
        product={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget ? `${deleteTarget.code} · ${deleteTarget.title}` : ""
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
