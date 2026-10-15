"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ExternalLink, Eye } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  useUrlListDefaults,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  auditColumns,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import { RentalStatusChip } from "@/features/rentals/components/rental-cells";
import {
  RENTALS_RESOURCE,
  RENTAL_STATUSES,
  RENTAL_STATUS_LABEL_KEYS,
} from "@/features/rentals/constants";
import {
  useRentalFacets,
  useRentalsList,
  useRentalsMeta,
} from "@/features/rentals/hooks/use-rentals";
import type {
  ListRentalsParams,
  RentalFacetValue,
  RentalListItem,
} from "@/features/rentals/types";
import { ReferenceText } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function countOf(facets: RentalFacetValue[] | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

function facetOptions(facets: RentalFacetValue[] | undefined) {
  return (facets ?? []).map((f) => ({
    value: f.value,
    label: `${f.label || f.value} (${f.count})`,
  }));
}

/**
 * Rentals ("Kiralamalar"): every rental whose pick-up, return or actual
 * return branch is in the member's branch scope. Facets, live updates,
 * export; rentals are created from a confirmed reservation.
 */
export function RentalsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const urlDefaults = useUrlListDefaults(
    ["status", "branch", "product", "overdue"],
    ["reference", "status", "planned_pickup_at", "planned_return_at"],
  );
  const listState = useServerListState({
    initialSort: urlDefaults.initialSort ?? "-created_at",
    initialColumnFilters: urlDefaults.initialColumnFilters,
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RENTALS_RESOURCE),
  });

  const listParams = useMemo<ListRentalsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      status: joinFilter(column("status")),
      branch: joinFilter(column("branch")),
      product: joinFilter(column("product")),
      overdue: joinFilter(column("overdue")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useRentalsList(listParams);
  const metaQuery = useRentalsMeta();
  const facetsQuery = useRentalFacets();
  const facets = facetsQuery.data;

  const columns = useMemo(
    () =>
      [
        createColumn<RentalListItem>({
          accessorKey: "reference",
          labelKey: "rentals.columns.reference",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5">
              <ReferenceText reference={row.original.reference} />
              {row.original.open_charges > 0 ? (
                <Badge variant="warning" className="px-1.5 py-0 text-[10px]">
                  {t("rentals.open_charges_badge", {
                    count: row.original.open_charges,
                  })}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<RentalListItem>({
          accessorKey: "status",
          labelKey: "rentals.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: RENTAL_STATUSES.map((v) => ({
            value: v,
            labelKey: RENTAL_STATUS_LABEL_KEYS[v],
            label: `${t(RENTAL_STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
          })),
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5">
              <RentalStatusChip status={row.original.status} />
              {row.original.overdue_at ? (
                <Badge variant="danger">{t("rentals.overdue_badge")}</Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<RentalListItem>({
          id: "customer",
          accessorFn: (row) => row.customer.display_name,
          labelKey: "rentals.columns.customer",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.customer.display_name}
            </span>
          ),
        }),
        createColumn<RentalListItem>({
          id: "plate",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "rentals.columns.plate",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-mono text-xs font-medium">
                {row.original.vehicle.plate}
              </span>
              <span className="text-muted-foreground text-xs">
                {row.original.vehicle.label}
              </span>
            </span>
          ),
        }),
        createColumn<RentalListItem>({
          id: "product",
          accessorFn: (row) => row.product.code ?? "",
          labelKey: "rentals.columns.product",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: facetOptions(facets?.product),
          cell: ({ row }) => row.original.product.code ?? "—",
        }),
        createColumn<RentalListItem>({
          id: "branch",
          accessorFn: (row) => row.pickup_branch.code ?? "",
          labelKey: "rentals.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: facetOptions(facets?.branch),
          cell: ({ row }) =>
            row.original.pickup_branch.uuid === row.original.return_branch.uuid
              ? (row.original.pickup_branch.code ?? "")
              : `${row.original.pickup_branch.code ?? ""} → ${row.original.return_branch.code ?? ""}`,
        }),
        createColumn<RentalListItem>({
          accessorKey: "planned_pickup_at",
          labelKey: "rentals.columns.planned_pickup_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(
              row.original.planned_pickup_at,
              "d MMM yyyy HH:mm",
              locale,
            ),
        }),
        createColumn<RentalListItem>({
          accessorKey: "planned_return_at",
          labelKey: "rentals.columns.planned_return_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(
              row.original.planned_return_at,
              "d MMM yyyy HH:mm",
              locale,
            ),
        }),
        createColumn<RentalListItem>({
          id: "actual_pickup_at",
          accessorFn: (row) => row.actual_pickup_at ?? "",
          labelKey: "rentals.columns.actual_pickup_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.actual_pickup_at
              ? datetime(row.original.actual_pickup_at, "d MMM HH:mm", locale)
              : "—",
        }),
        createColumn<RentalListItem>({
          id: "actual_return_at",
          accessorFn: (row) => row.actual_return_at ?? "",
          labelKey: "rentals.columns.actual_return_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.actual_return_at
              ? datetime(row.original.actual_return_at, "d MMM HH:mm", locale)
              : "—",
        }),
        createColumn<RentalListItem>({
          id: "overdue",
          accessorFn: (row) => (row.overdue_at ? "true" : "false"),
          labelKey: "rentals.columns.late",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [
            { value: "true", label: t("rentals.filters.overdue") },
          ],
          cell: ({ row }) =>
            row.original.late_minutes > 0
              ? t("rentals.late_minutes", {
                  count: row.original.late_minutes,
                })
              : "—",
        }),
        ...auditColumns<RentalListItem>(
          {
            createdAt: "rentals.columns.created_at",
            uuid: "rentals.columns.uuid",
          },
          locale,
        ),
        createColumn<RentalListItem>({
          id: "actions",
          labelKey: "rentals.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("rentals.actions.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.rentals.detail(slug, row.original.uuid),
                    ),
                },
                {
                  id: "reservation",
                  label: t("rentals.actions.open_reservation"),
                  icon: ExternalLink,
                  permission: permissions.tenant.reservations.read,
                  onSelect: () =>
                    router.push(
                      routes.tenant.reservations.detail(
                        slug,
                        row.original.reservation_uuid,
                      ),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<RentalListItem, unknown>[],
    [facets, locale, router, slug, t],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      branch: listParams.branch,
      product: listParams.product,
      overdue: listParams.overdue,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.overdue,
      listParams.product,
      listParams.q,
      listParams.sort,
      listParams.status,
    ],
  );
  const total = listQuery.data?.total ?? 0;
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
      title={t("rentals.title")}
      description={t("rentals.description")}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("rentals.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(r) =>
          router.push(routes.tenant.rentals.detail(slug, r.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("rentals.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("rentals.empty_filtered_title")
            : t("rentals.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("rentals.empty_filtered_description")
            : t("rentals.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "tenant-rentals-v1", rowSelection: false }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={RENTALS_RESOURCE}
              query={engineQuery}
              capabilities={metaQuery.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
