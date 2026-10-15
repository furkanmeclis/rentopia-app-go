"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

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
import {
  CampaignStateChip,
  DiscountText,
  UsageText,
} from "@/features/campaigns/components/campaign-cells";
import { CampaignDrawer } from "@/features/campaigns/components/campaign-drawer";
import {
  CAMPAIGNS_RESOURCE,
  CAMPAIGN_STATES,
  CAMPAIGN_TYPES,
  DISCOUNT_KINDS,
  KIND_LABEL_KEYS,
  STATE_LABEL_KEYS,
  TYPE_LABEL_KEYS,
} from "@/features/campaigns/constants";
import {
  useCampaignFacets,
  useCampaignsList,
  useCampaignsMeta,
  useDeleteCampaign,
  useUpdateCampaign,
} from "@/features/campaigns/hooks/use-campaigns";
import type {
  Campaign,
  CampaignFacets,
  ListCampaignsParams,
} from "@/features/campaigns/types";
import {
  auditColumns,
  CodeBadge,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type FacetList = CampaignFacets[keyof CampaignFacets];

function countOf(facets: FacetList | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

/**
 * Campaigns and coupons ("Kampanyalar"): coupon codes and repeat-customer
 * campaigns with their discount, validity, usage and computed state
 * (active, scheduled, exhausted, expired, inactive). Facets, live updates,
 * export and bulk activate / deactivate.
 */
export function CampaignsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, CAMPAIGNS_RESOURCE),
  });
  const [drawer, setDrawer] = useState<{ campaign: Campaign | null } | null>(
    null,
  );
  const [deleteTarget, setDeleteTarget] = useState<Campaign | null>(null);
  const write = permissions.tenant.campaigns.write;

  const listParams = useMemo<ListCampaignsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      campaign_type: joinFilter(column("campaign_type")),
      discount_kind: joinFilter(column("discount_kind")),
      state: joinFilter(column("state")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useCampaignsList(listParams);
  const metaQuery = useCampaignsMeta();
  const facetsQuery = useCampaignFacets();
  const update = useUpdateCampaign();
  const remove = useDeleteCampaign();
  const facets = facetsQuery.data;

  const baseColumns = useMemo(
    () =>
      [
        createColumn<Campaign>({
          accessorKey: "name",
          labelKey: "campaigns.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<Campaign>({
          id: "code",
          accessorFn: (row) => row.code ?? "",
          labelKey: "campaigns.columns.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.code ? <CodeBadge code={row.original.code} /> : "—",
        }),
        createColumn<Campaign>({
          accessorKey: "campaign_type",
          labelKey: "campaigns.columns.campaign_type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: CAMPAIGN_TYPES.map((v) => ({
            value: v,
            labelKey: TYPE_LABEL_KEYS[v],
            label: `${t(TYPE_LABEL_KEYS[v])} (${countOf(facets?.campaign_type, v)})`,
          })),
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(TYPE_LABEL_KEYS[row.original.campaign_type])}
            </Badge>
          ),
        }),
        createColumn<Campaign>({
          accessorKey: "discount_kind",
          labelKey: "campaigns.columns.discount_kind",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: DISCOUNT_KINDS.map((v) => ({
            value: v,
            labelKey: KIND_LABEL_KEYS[v],
            label: `${t(KIND_LABEL_KEYS[v])} (${countOf(facets?.discount_kind, v)})`,
          })),
          cell: ({ row }) => t(KIND_LABEL_KEYS[row.original.discount_kind]),
        }),
        createColumn<Campaign>({
          id: "discount_value",
          accessorFn: (row) => row.discount_value,
          labelKey: "campaigns.columns.discount_value",
          enableSorting: false,
          cell: ({ row }) => <DiscountText campaign={row.original} />,
        }),
        createColumn<Campaign>({
          id: "start_date",
          accessorFn: (row) => row.start_date ?? "",
          labelKey: "campaigns.columns.start_date",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.start_date
              ? date(row.original.start_date, "d MMM yyyy", locale)
              : "—",
        }),
        createColumn<Campaign>({
          id: "end_date",
          accessorFn: (row) => row.end_date ?? "",
          labelKey: "campaigns.columns.end_date",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.end_date
              ? date(row.original.end_date, "d MMM yyyy", locale)
              : "—",
        }),
        createColumn<Campaign>({
          id: "used_count",
          accessorFn: (row) => row.used_count,
          labelKey: "campaigns.columns.used_count",
          enableSorting: true,
          cell: ({ row }) => <UsageText campaign={row.original} />,
        }),
        createColumn<Campaign>({
          accessorKey: "state",
          labelKey: "campaigns.columns.state",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: CAMPAIGN_STATES.map((v) => ({
            value: v,
            labelKey: STATE_LABEL_KEYS[v],
            label: `${t(STATE_LABEL_KEYS[v])} (${countOf(facets?.state, v)})`,
          })),
          cell: ({ row }) => <CampaignStateChip state={row.original.state} />,
        }),
        ...auditColumns<Campaign>(
          {
            createdAt: "campaigns.columns.created_at",
            uuid: "campaigns.columns.uuid",
          },
          locale,
        ),
        createColumn<Campaign>({
          id: "actions",
          labelKey: "campaigns.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const c = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "view",
                    label: t("campaigns.actions.open"),
                    icon: Eye,
                    onSelect: () =>
                      router.push(
                        routes.tenant.pricing.campaigns.detail(slug, c.uuid),
                      ),
                  },
                  {
                    id: "edit",
                    label: t("campaigns.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () => setDrawer({ campaign: c }),
                  },
                  {
                    id: "toggle-active",
                    label: c.is_active
                      ? t("campaigns.actions.deactivate")
                      : t("campaigns.actions.activate"),
                    icon: c.is_active ? PowerOff : Power,
                    permission: write,
                    onSelect: () =>
                      update.mutate({
                        uuid: c.uuid,
                        body: { is_active: !c.is_active },
                      }),
                  },
                  {
                    id: "delete",
                    label: t("campaigns.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(c),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<Campaign, unknown>[],
    [facets, locale, router, slug, t, update, write],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<Campaign>(), ...baseColumns],
    [baseColumns],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      campaign_type: listParams.campaign_type,
      discount_kind: listParams.discount_kind,
      state: listParams.state,
      sort: listParams.sort,
    }),
    [
      listParams.campaign_type,
      listParams.discount_kind,
      listParams.q,
      listParams.sort,
      listParams.state,
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
      resolveBulkActionsWithIcons(CAMPAIGNS_RESOURCE, meta?.bulk_actions ?? []),
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
      title={t("campaigns.title")}
      description={t("campaigns.description")}
      permission={permissions.tenant.campaigns.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("campaigns.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("layout.section_pricing") },
        { label: t("campaigns.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setDrawer({ campaign: null })}
          label={t("campaigns.actions.create")}
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
        onRowClick={(c) =>
          router.push(routes.tenant.pricing.campaigns.detail(slug, c.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("campaigns.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("campaigns.empty_filtered_title")
            : t("campaigns.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("campaigns.empty_filtered_description")
            : t("campaigns.empty_description")
        }
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "tenant-pricing-campaigns-v1",
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource={CAMPAIGNS_RESOURCE}
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                refresh();
              }}
            />
            <ResourceIOToolbar
              resource={CAMPAIGNS_RESOURCE}
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

      <CampaignDrawer
        open={Boolean(drawer)}
        campaign={drawer?.campaign ?? null}
        onOpenChange={(open) => {
          if (!open) setDrawer(null);
        }}
        onCreated={(c) =>
          router.push(routes.tenant.pricing.campaigns.detail(slug, c.uuid))
        }
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={deleteTarget?.name ?? ""}
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
