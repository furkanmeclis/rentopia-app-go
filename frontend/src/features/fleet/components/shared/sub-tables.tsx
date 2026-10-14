"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import { GuaranteeBadgeChip } from "@/features/fleet/components/products/guarantee-badge";
import {
  activeColumn,
  CodeBadge,
} from "@/features/fleet/components/shared/list-utils";
import {
  STATUS_LABEL_KEYS,
  STATUS_TONES,
  TRANSMISSION_LABEL_KEYS,
  FUEL_LABEL_KEYS,
  BODY_TYPE_LABEL_KEYS,
} from "@/features/fleet/constants";
import {
  useGroupsList,
  useProductsList,
} from "@/features/fleet/hooks/use-fleet";
import type { RentalGroup, RentalProduct } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/** Rental groups of one segment (server-side DataTable sub-list). */
export function GroupsSubTable({ segmentUuid }: { segmentUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "code",
    initialPageSize: 10,
  });
  const params = useMemo(
    () => ({ ...listState.params, segment: segmentUuid }),
    [listState.params, segmentUuid],
  );
  const query = useGroupsList(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<RentalGroup>({
          accessorKey: "code",
          labelKey: "fleet.groups.columns.code",
          enableSorting: true,
          cell: ({ row }) => <CodeBadge code={row.original.code} />,
        }),
        createColumn<RentalGroup>({
          accessorKey: "name",
          labelKey: "fleet.groups.columns.name",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<RentalGroup>({
          id: "attributes",
          labelKey: "fleet.groups.columns.attributes",
          enableSorting: false,
          cell: ({ row }) => {
            const g = row.original;
            const chips = [
              g.transmission ? t(TRANSMISSION_LABEL_KEYS[g.transmission]) : "",
              g.fuel ? t(FUEL_LABEL_KEYS[g.fuel]) : "",
              g.body_type ? t(BODY_TYPE_LABEL_KEYS[g.body_type]) : "",
            ].filter(Boolean);
            return chips.length ? (
              <div className="flex flex-wrap gap-1">
                {chips.map((chip) => (
                  <Badge key={chip} variant="outline">
                    {chip}
                  </Badge>
                ))}
              </div>
            ) : (
              "—"
            );
          },
        }),
        createColumn<RentalGroup>({
          accessorKey: "products_count",
          labelKey: "fleet.groups.columns.products_count",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="secondary" className="tabular-nums">
              {row.original.products_count}
            </Badge>
          ),
        }),
        activeColumn<RentalGroup>(t, "fleet.groups.columns.is_active"),
      ] as ColumnDef<RentalGroup, unknown>[],
    [t],
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(group) =>
        router.push(routes.tenant.fleet.groups.detail(slug, group.uuid))
      }
      isLoading={query.isLoading}
      isError={query.isError}
      onRetry={() => void query.refetch()}
      emptyTitle={t("fleet.segments.detail.groups_empty")}
      emptyDescription={t("fleet.segments.detail.groups_empty_description")}
      pageCount={Math.max(1, Math.ceil(total / listState.pagination.pageSize))}
      state={listState.tableState}
      features={{
        persistKey: "tenant-fleet-segment-groups-v1",
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
      }}
    />
  );
}

/**
 * Products of a segment or guaranteeing a rental group (server-side
 * DataTable sub-list).
 */
export function ProductsSubTable({
  segmentUuid,
  rentalGroupUuid,
  persistKey,
}: {
  segmentUuid?: string;
  rentalGroupUuid?: string;
  persistKey: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "code",
    initialPageSize: 10,
  });
  const params = useMemo(
    () => ({
      ...listState.params,
      segment: segmentUuid,
      rental_group: rentalGroupUuid,
    }),
    [listState.params, rentalGroupUuid, segmentUuid],
  );
  const query = useProductsList(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<RentalProduct>({
          accessorKey: "code",
          labelKey: "fleet.products.columns.code",
          enableSorting: true,
          cell: ({ row }) => <CodeBadge code={row.original.code} />,
        }),
        createColumn<RentalProduct>({
          accessorKey: "title",
          labelKey: "fleet.products.columns.title",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.title}</span>
          ),
        }),
        createColumn<RentalProduct>({
          accessorKey: "display_text",
          labelKey: "fleet.products.columns.display_text",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-2">
              <span>{row.original.display_text}</span>
              <GuaranteeBadgeChip badge={row.original.guarantee_badge} />
            </div>
          ),
        }),
        createColumn<RentalProduct>({
          accessorKey: "status",
          labelKey: "fleet.products.columns.status",
          enableSorting: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[row.original.status])}
              tone={STATUS_TONES[row.original.status]}
            />
          ),
        }),
        activeColumn<RentalProduct>(t, "fleet.products.columns.is_active"),
      ] as ColumnDef<RentalProduct, unknown>[],
    [t],
  );

  return (
    <EntityTable
      columns={columns}
      data={query.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(product) =>
        router.push(routes.tenant.fleet.products.detail(slug, product.uuid))
      }
      isLoading={query.isLoading}
      isError={query.isError}
      onRetry={() => void query.refetch()}
      emptyTitle={t("fleet.products.sub_empty_title")}
      emptyDescription={t("fleet.products.sub_empty_description")}
      pageCount={Math.max(1, Math.ceil(total / listState.pagination.pageSize))}
      state={listState.tableState}
      features={{
        persistKey,
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
      }}
    />
  );
}
