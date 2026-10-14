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
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { SegmentQuickEditDrawer } from "@/features/fleet/components/segments/segment-quick-edit-drawer";
import {
  activeColumn,
  activeFilter,
  auditColumns,
  CodeBadge,
  filterValues,
} from "@/features/fleet/components/shared/list-utils";
import { SEGMENTS_RESOURCE } from "@/features/fleet/constants";
import {
  useDeleteSegment,
  useSegmentsList,
  useSegmentsMeta,
  useUpdateSegment,
} from "@/features/fleet/hooks/use-fleet";
import type { ListSegmentsParams, Segment } from "@/features/fleet/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";

/**
 * Vehicle segments (commercial classes). Tenant list standard: server-side
 * pagination / sort / search, faceted filter, column visibility, row actions,
 * row click → detail, quick-edit drawer, export.
 */
export function SegmentsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "sort_order",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, SEGMENTS_RESOURCE),
  });
  const [quickEdit, setQuickEdit] = useState<Segment | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Segment | null>(null);

  const listParams = useMemo<ListSegmentsParams>(
    () => ({
      ...listState.params,
      is_active: activeFilter(
        filterValues(listState.columnFilters, "is_active"),
      ),
    }),
    [listState.columnFilters, listState.params],
  );

  const listQuery = useSegmentsList(listParams);
  const metaQuery = useSegmentsMeta();
  const update = useUpdateSegment();
  const remove = useDeleteSegment();
  const write = permissions.tenant.fleet.write;

  const openDetail = useCallback(
    (segment: Segment) =>
      router.push(routes.tenant.fleet.segments.detail(slug, segment.uuid)),
    [router, slug],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Segment>({
          accessorKey: "code",
          labelKey: "fleet.segments.columns.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => <CodeBadge code={row.original.code} />,
        }),
        createColumn<Segment>({
          accessorKey: "name",
          labelKey: "fleet.segments.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<Segment>({
          accessorKey: "sort_order",
          labelKey: "fleet.segments.columns.sort_order",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.sort_order}</span>
          ),
        }),
        createColumn<Segment>({
          accessorKey: "groups_count",
          labelKey: "fleet.segments.columns.groups_count",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="secondary" className="tabular-nums">
              {row.original.groups_count}
            </Badge>
          ),
        }),
        createColumn<Segment>({
          accessorKey: "products_count",
          labelKey: "fleet.segments.columns.products_count",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="secondary" className="tabular-nums">
              {row.original.products_count}
            </Badge>
          ),
        }),
        activeColumn<Segment>(t, "fleet.segments.columns.is_active"),
        ...auditColumns<Segment>(
          {
            createdAt: "fleet.segments.columns.created_at",
            uuid: "fleet.segments.columns.uuid",
          },
          locale,
        ),
        createColumn<Segment>({
          id: "actions",
          labelKey: "fleet.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const segment = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "view",
                    label: t("fleet.actions.view"),
                    icon: Eye,
                    onSelect: () => openDetail(segment),
                  },
                  {
                    id: "quick-edit",
                    label: t("fleet.actions.quick_edit"),
                    icon: PencilLine,
                    permission: write,
                    onSelect: () => setQuickEdit(segment),
                  },
                  {
                    id: "edit",
                    label: t("fleet.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () =>
                      router.push(
                        routes.tenant.fleet.segments.edit(slug, segment.uuid),
                      ),
                  },
                  {
                    id: "toggle-active",
                    label: segment.is_active
                      ? t("fleet.actions.deactivate")
                      : t("fleet.actions.activate"),
                    icon: segment.is_active ? PowerOff : Power,
                    permission: write,
                    onSelect: () =>
                      update.mutate({
                        uuid: segment.uuid,
                        body: { is_active: !segment.is_active },
                      }),
                  },
                  {
                    id: "delete",
                    label: t("fleet.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(segment),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<Segment, unknown>[],
    [locale, openDetail, router, slug, t, update, write],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      is_active: listParams.is_active,
      sort: listParams.sort,
    }),
    [listParams.is_active, listParams.q, listParams.sort],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Boolean(listParams.q) || Boolean(listParams.is_active);

  return (
    <EntityPage
      title={t("fleet.segments.title")}
      description={t("fleet.segments.description")}
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
        { label: t("fleet.segments.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.tenant.fleet.segments.create(slug))}
          label={t("fleet.segments.actions.create")}
          permission={write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("fleet.segments.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() => listQuery.refetch())}
        emptyTitle={
          hasFilters
            ? t("fleet.empty_filtered_title")
            : t("fleet.segments.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("fleet.empty_filtered_description")
            : t("fleet.segments.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-fleet-segments-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={SEGMENTS_RESOURCE}
              query={engineQuery}
              capabilities={metaQuery.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <SegmentQuickEditDrawer
        segment={quickEdit}
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
