"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, PencilLine, Power, PowerOff, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
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
import { GroupQuickEditDrawer } from "@/features/fleet/components/groups/group-quick-edit-drawer";
import {
  activeColumn,
  activeFilter,
  auditColumns,
  CodeBadge,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  BODY_TYPES,
  BODY_TYPE_LABEL_KEYS,
  FUELS,
  FUEL_LABEL_KEYS,
  RENTAL_GROUPS_RESOURCE,
  TRANSMISSIONS,
  TRANSMISSION_LABEL_KEYS,
} from "@/features/fleet/constants";
import {
  useDeleteGroup,
  useGroupFacets,
  useGroupsList,
  useGroupsMeta,
  useUpdateGroup,
} from "@/features/fleet/hooks/use-fleet";
import type {
  FleetFacet,
  ListGroupsParams,
  RentalGroup,
} from "@/features/fleet/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

function countOf(facets: FleetFacet[] | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

/**
 * Rental groups: the protected attributes an "X veya benzeri" product
 * guarantees. Full tenant list standard incl. bulk activate / deactivate.
 */
export function GroupsPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });
  const [quickEdit, setQuickEdit] = useState<RentalGroup | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<RentalGroup | null>(null);

  const listParams = useMemo<ListGroupsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      segment: joinFilter(column("segment")),
      transmission: joinFilter(column("transmission")),
      fuel: joinFilter(column("fuel")),
      body_type: joinFilter(column("body_type")),
      is_active: activeFilter(column("is_active")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useGroupsList(listParams);
  const metaQuery = useGroupsMeta();
  const facetsQuery = useGroupFacets();
  const update = useUpdateGroup();
  const remove = useDeleteGroup();
  const write = permissions.tenant.fleet.write;
  const facets = facetsQuery.data;

  const openDetail = useCallback(
    (group: RentalGroup) =>
      router.push(routes.tenant.fleet.groups.detail(slug, group.uuid)),
    [router, slug],
  );

  const baseColumns = useMemo(
    () =>
      [
        createColumn<RentalGroup>({
          accessorKey: "code",
          labelKey: "fleet.groups.columns.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => <CodeBadge code={row.original.code} />,
        }),
        createColumn<RentalGroup>({
          accessorKey: "name",
          labelKey: "fleet.groups.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<RentalGroup>({
          id: "segment",
          accessorFn: (row) => row.segment.uuid,
          labelKey: "fleet.groups.columns.segment",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.segment ?? []).map((facet) => ({
            value: facet.value,
            label: `${facet.label ?? facet.value} (${facet.count})`,
          })),
          cell: ({ row }) => row.original.segment.name,
        }),
        createColumn<RentalGroup>({
          id: "transmission",
          accessorFn: (row) => row.transmission ?? "",
          labelKey: "fleet.groups.columns.transmission",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: TRANSMISSIONS.map((value) => ({
            value,
            labelKey: TRANSMISSION_LABEL_KEYS[value],
            label: `${t(TRANSMISSION_LABEL_KEYS[value])} (${countOf(facets?.transmission, value)})`,
          })),
          cell: ({ row }) =>
            row.original.transmission
              ? t(TRANSMISSION_LABEL_KEYS[row.original.transmission])
              : t("fleet.groups.form.not_guaranteed"),
        }),
        createColumn<RentalGroup>({
          id: "fuel",
          accessorFn: (row) => row.fuel ?? "",
          labelKey: "fleet.groups.columns.fuel",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: FUELS.map((value) => ({
            value,
            labelKey: FUEL_LABEL_KEYS[value],
            label: `${t(FUEL_LABEL_KEYS[value])} (${countOf(facets?.fuel, value)})`,
          })),
          cell: ({ row }) =>
            row.original.fuel
              ? t(FUEL_LABEL_KEYS[row.original.fuel])
              : t("fleet.groups.form.not_guaranteed"),
        }),
        createColumn<RentalGroup>({
          id: "body_type",
          accessorFn: (row) => row.body_type ?? "",
          labelKey: "fleet.groups.columns.body_type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: BODY_TYPES.map((value) => ({
            value,
            labelKey: BODY_TYPE_LABEL_KEYS[value],
            label: `${t(BODY_TYPE_LABEL_KEYS[value])} (${countOf(facets?.body_type, value)})`,
          })),
          cell: ({ row }) =>
            row.original.body_type
              ? t(BODY_TYPE_LABEL_KEYS[row.original.body_type])
              : t("fleet.groups.form.not_guaranteed"),
        }),
        createColumn<RentalGroup>({
          accessorKey: "min_seats",
          labelKey: "fleet.groups.columns.min_seats",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => row.original.min_seats ?? "—",
        }),
        createColumn<RentalGroup>({
          accessorKey: "acriss_code",
          labelKey: "fleet.groups.columns.acriss_code",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.acriss_code ? (
              <code className="text-xs">{row.original.acriss_code}</code>
            ) : (
              "—"
            ),
        }),
        createColumn<RentalGroup>({
          id: "criteria",
          labelKey: "fleet.groups.columns.criteria",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            t("fleet.groups.criteria_summary", {
              models: row.original.catalog_models.length,
              types: row.original.catalog_types.length,
            }),
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
        ...auditColumns<RentalGroup>(
          {
            createdAt: "fleet.groups.columns.created_at",
            uuid: "fleet.groups.columns.uuid",
          },
          locale,
        ),
        createColumn<RentalGroup>({
          id: "actions",
          labelKey: "fleet.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const group = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "view",
                    label: t("fleet.actions.view"),
                    icon: Eye,
                    onSelect: () => openDetail(group),
                  },
                  {
                    id: "quick-edit",
                    label: t("fleet.actions.quick_edit"),
                    icon: PencilLine,
                    permission: write,
                    onSelect: () => setQuickEdit(group),
                  },
                  {
                    id: "edit",
                    label: t("fleet.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () =>
                      router.push(
                        routes.tenant.fleet.groups.edit(slug, group.uuid),
                      ),
                  },
                  {
                    id: "toggle-active",
                    label: group.is_active
                      ? t("fleet.actions.deactivate")
                      : t("fleet.actions.activate"),
                    icon: group.is_active ? PowerOff : Power,
                    permission: write,
                    onSelect: () =>
                      update.mutate({
                        uuid: group.uuid,
                        body: { is_active: !group.is_active },
                      }),
                  },
                  {
                    id: "delete",
                    label: t("fleet.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(group),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<RentalGroup, unknown>[],
    [facets, locale, openDetail, router, slug, t, update, write],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<RentalGroup>(), ...baseColumns],
    [baseColumns],
  );

  // Same filters for export and the "all matching" bulk scope.
  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      segment: listParams.segment,
      transmission: listParams.transmission,
      fuel: listParams.fuel,
      body_type: listParams.body_type,
      is_active: listParams.is_active,
      sort: listParams.sort,
    }),
    [
      listParams.body_type,
      listParams.fuel,
      listParams.is_active,
      listParams.q,
      listParams.segment,
      listParams.sort,
      listParams.transmission,
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
        RENTAL_GROUPS_RESOURCE,
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
      title={t("fleet.groups.title")}
      description={t("fleet.groups.description")}
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
        { label: t("fleet.groups.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.fleet.groups.create(slug))}
          label={t("fleet.groups.actions.create")}
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
        errorDescription={t("fleet.groups.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters
            ? t("fleet.empty_filtered_title")
            : t("fleet.groups.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("fleet.empty_filtered_description")
            : t("fleet.groups.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-fleet-groups-v1",
          rowSelection: true,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={RENTAL_GROUPS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={RENTAL_GROUPS_RESOURCE}
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

      <GroupQuickEditDrawer
        group={quickEdit}
        onOpenChange={(open) => {
          if (!open) setQuickEdit(null);
        }}
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
