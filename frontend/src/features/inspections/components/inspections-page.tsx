"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

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
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  INSPECTIONS_RESOURCE,
  INSPECTION_KINDS,
  INSPECTION_STATUSES,
  KIND_LABEL_KEYS,
  REVIEW_LABEL_KEYS,
  REVIEW_STATUSES,
  REVIEW_TONES,
  SIGNATURE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/inspections/constants";
import {
  useInspectionFacets,
  useInspectionsList,
} from "@/features/inspections/hooks/use-inspections";
import type {
  InspectionFacetValue,
  InspectionListItem,
  ListInspectionsParams,
} from "@/features/inspections/types";
import { ReferenceText } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function countOf(facets: InspectionFacetValue[] | undefined, value: string) {
  return facets?.find((f) => f.value === value)?.count ?? 0;
}

/**
 * Handover / return forms; `review_status = pending` is the authorized
 * review queue of disputed or unsigned returns.
 */
export function InspectionsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    initialColumnFilters: [{ id: "review_status", value: ["pending"] }],
    realtime: tenantLiveList(organization, INSPECTIONS_RESOURCE),
  });
  const facetsQuery = useInspectionFacets();
  const facets = facetsQuery.data;
  const params = useMemo<ListInspectionsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      kind: joinFilter(column("kind")),
      status: joinFilter(column("status")),
      review_status: joinFilter(column("review_status")),
    };
  }, [listState.columnFilters, listState.params]);
  const query = useInspectionsList(params);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<InspectionListItem>({
          id: "rental",
          accessorFn: (row) => row.rental.reference,
          labelKey: "inspections.list.columns.rental",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <ReferenceText reference={row.original.rental.reference} />
              <span className="text-muted-foreground text-xs">
                {row.original.vehicle.plate} ·{" "}
                {row.original.customer.display_name}
              </span>
            </span>
          ),
        }),
        createColumn<InspectionListItem>({
          accessorKey: "kind",
          labelKey: "inspections.list.columns.kind",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: INSPECTION_KINDS.map((v) => ({
            value: v,
            labelKey: KIND_LABEL_KEYS[v],
            label: `${t(KIND_LABEL_KEYS[v])} (${countOf(facets?.kind, v)})`,
          })),
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5">
              <Badge variant="outline">
                {t(KIND_LABEL_KEYS[row.original.kind])}
              </Badge>
              {row.original.version > 1 ? (
                <span className="text-muted-foreground text-xs">
                  v{row.original.version}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<InspectionListItem>({
          accessorKey: "status",
          labelKey: "inspections.list.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: INSPECTION_STATUSES.map((v) => ({
            value: v,
            labelKey: STATUS_LABEL_KEYS[v],
            label: `${t(STATUS_LABEL_KEYS[v])} (${countOf(facets?.status, v)})`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[row.original.status])}
              tone={STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<InspectionListItem>({
          accessorKey: "review_status",
          labelKey: "inspections.list.columns.review",
          enableSorting: false,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: REVIEW_STATUSES.map((v) => ({
            value: v,
            labelKey: REVIEW_LABEL_KEYS[v],
            label: `${t(REVIEW_LABEL_KEYS[v])} (${countOf(facets?.review_status, v)})`,
          })),
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1.5">
              <StatusChip
                label={t(REVIEW_LABEL_KEYS[row.original.review_status])}
                tone={REVIEW_TONES[row.original.review_status]}
              />
              {row.original.disputed ? (
                <Badge variant="danger">{t("inspections.list.disputed")}</Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<InspectionListItem>({
          accessorKey: "signature_status",
          labelKey: "inspections.list.columns.signature",
          enableSorting: false,
          cell: ({ row }) =>
            t(SIGNATURE_LABEL_KEYS[row.original.signature_status]),
        }),
        createColumn<InspectionListItem>({
          id: "evidence",
          accessorFn: (row) => row.photos,
          labelKey: "inspections.list.columns.evidence",
          enableSorting: false,
          cell: ({ row }) =>
            t("inspections.list.evidence", {
              photos: row.original.photos,
              damages: row.original.damages,
            }),
        }),
        createColumn<InspectionListItem>({
          id: "finalized_at",
          accessorFn: (row) => row.finalized_at ?? "",
          labelKey: "inspections.list.columns.finalized_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.finalized_at
              ? datetime(row.original.finalized_at, "d MMM yyyy HH:mm", locale)
              : "—",
        }),
        createColumn<InspectionListItem>({
          accessorKey: "created_at",
          labelKey: "inspections.list.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
        createColumn<InspectionListItem>({
          id: "actions",
          labelKey: "inspections.list.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("inspections.list.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.inspections.detail(slug, row.original.uuid),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<InspectionListItem, unknown>[],
    [facets, locale, router, slug, t],
  );

  return (
    <EntityPage
      title={t("inspections.list.title")}
      description={t("inspections.list.description")}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("inspections.list.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("inspections.list.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([query.refetch(), facetsQuery.refetch()]),
        )}
        onRowClick={(row) =>
          router.push(routes.tenant.inspections.detail(slug, row.uuid))
        }
        emptyTitle={t("inspections.list.empty_title")}
        emptyDescription={t("inspections.list.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{ persistKey: "tenant-inspections-v1", rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void query.refetch();
              void facetsQuery.refetch();
            }}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
