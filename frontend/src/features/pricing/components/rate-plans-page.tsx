"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Grid3x3, Trash2 } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

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
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { ResourceIOToolbar } from "@/features/io";
import { VersionBadge } from "@/features/pricing/components/pricing-shared";
import { RatePlanCreateDrawer } from "@/features/pricing/components/rate-plan-drawer";
import {
  PLAN_SCOPES,
  PRICING_STATUSES,
  RATE_PLANS_RESOURCE,
  SCOPE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/pricing/constants";
import {
  useDeleteRatePlan,
  useRatePlanFacets,
  useRatePlansList,
  useRatePlansMeta,
} from "@/features/pricing/hooks/use-pricing";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { moneyText } from "@/features/pricing/lib/format";
import type {
  ListRatePlansParams,
  RatePlan,
  RatePlanFacets,
} from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type FacetList = RatePlanFacets[keyof RatePlanFacets];

function countOf(facets: FacetList | undefined, value: string) {
  return facets?.find((facet) => facet.value === value)?.count ?? 0;
}

function referenceOptions(facets: FacetList | undefined) {
  return (facets ?? []).map((facet) => ({
    value: facet.value,
    label: `${facet.label ?? facet.value} (${facet.count})`,
  }));
}

/**
 * Rate plans ("Tarifeler"): a rental product's daily price by rental length,
 * overridden per branch / channel. Full tenant list standard with live
 * updates; import / export of the İdes price list; the matrix editor for
 * fast bulk entry.
 */
export function RatePlansPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const listState = useServerListState({
    initialSort: "rental_product",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, RATE_PLANS_RESOURCE),
  });
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<RatePlan | null>(null);
  const write = permissions.tenant.pricing.write;

  const listParams = useMemo<ListRatePlansParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      rental_product: joinFilter(column("rental_product")),
      scope: joinFilter(column("scope")),
      branch: joinFilter(column("branch")),
      channel: joinFilter(column("channel")),
      status: joinFilter(column("status")),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useRatePlansList(listParams);
  const metaQuery = useRatePlansMeta();
  const facetsQuery = useRatePlanFacets();
  const remove = useDeleteRatePlan();
  const facets = facetsQuery.data;

  const columns = useMemo(
    () =>
      [
        createColumn<RatePlan>({
          id: "rental_product",
          accessorFn: (row) => row.rental_product.uuid,
          labelKey: "pricing.columns.rental_product",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.rental_product),
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">
                <code className="text-xs">
                  {row.original.rental_product.code}
                </code>{" "}
                {row.original.rental_product.title}
              </span>
              {row.original.name ? (
                <span className="text-muted-foreground text-xs">
                  {row.original.name}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<RatePlan>({
          accessorKey: "name",
          labelKey: "pricing.columns.name",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => row.original.name || "—",
        }),
        createColumn<RatePlan>({
          accessorKey: "scope",
          labelKey: "pricing.columns.scope",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: PLAN_SCOPES.map((s) => ({
            value: s,
            labelKey: SCOPE_LABEL_KEYS[s],
            label: `${t(SCOPE_LABEL_KEYS[s])} (${countOf(facets?.scope, s)})`,
          })),
          cell: ({ row }) => (
            <Badge variant="secondary">
              {t(SCOPE_LABEL_KEYS[row.original.scope])}
            </Badge>
          ),
        }),
        createColumn<RatePlan>({
          id: "branch",
          accessorFn: (row) => row.branch?.uuid ?? "",
          labelKey: "pricing.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: referenceOptions(facets?.branch),
          cell: ({ row }) =>
            row.original.branch
              ? `${row.original.branch.code} · ${row.original.branch.name}`
              : t("pricing.all_branches"),
        }),
        createColumn<RatePlan>({
          id: "channel",
          accessorFn: (row) => row.channel ?? "",
          labelKey: "pricing.columns.channel",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.channel ?? []).map((f) => ({
            value: f.value,
            label: `${channelLabel(t, f.value)} (${f.count})`,
          })),
          cell: ({ row }) =>
            row.original.channel
              ? channelLabel(t, row.original.channel)
              : t("pricing.all_channels"),
        }),
        createColumn<RatePlan>({
          accessorKey: "priority",
          labelKey: "pricing.columns.priority",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.priority}</span>
          ),
        }),
        createColumn<RatePlan>({
          id: "bands",
          accessorFn: (row) => row.band_count,
          labelKey: "pricing.columns.bands",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.band_count}</span>
          ),
        }),
        createColumn<RatePlan>({
          id: "price_from",
          accessorFn: (row) => row.price_from ?? "",
          labelKey: "pricing.columns.price_from",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.price_from, locale)}
            </span>
          ),
        }),
        createColumn<RatePlan>({
          accessorKey: "status",
          labelKey: "pricing.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: PRICING_STATUSES.map((s) => ({
            value: s,
            labelKey: STATUS_LABEL_KEYS[s],
            label: `${t(STATUS_LABEL_KEYS[s])} (${countOf(facets?.status, s)})`,
          })),
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1">
              <StatusChip
                label={t(STATUS_LABEL_KEYS[row.original.status])}
                tone={STATUS_TONES[row.original.status]}
              />
              {row.original.has_draft && row.original.status !== "draft" ? (
                <StatusChip label={t("pricing.pending_draft")} tone="warning" />
              ) : null}
            </span>
          ),
        }),
        createColumn<RatePlan>({
          id: "version",
          accessorFn: (row) => row.current_version,
          labelKey: "pricing.columns.version",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <VersionBadge version={row.original.current_version} />
          ),
        }),
        createColumn<RatePlan>({
          accessorKey: "updated_at",
          labelKey: "pricing.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.updated_at, undefined, locale),
        }),
        createColumn<RatePlan>({
          accessorKey: "uuid",
          labelKey: "pricing.columns.uuid",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-xs">{row.original.uuid}</code>
          ),
        }),
        createColumn<RatePlan>({
          id: "actions",
          labelKey: "pricing.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("pricing.actions.open"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.tenant.pricing.plans.detail(
                        slug,
                        row.original.uuid,
                      ),
                    ),
                },
                {
                  id: "delete",
                  label: t("pricing.actions.delete"),
                  icon: Trash2,
                  permission: write,
                  variant: "destructive",
                  onSelect: () => setDeleteTarget(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<RatePlan, unknown>[],
    [facets, locale, router, slug, t, write],
  );

  const engineQuery = useMemo(
    () => ({
      q: listParams.q,
      rental_product: listParams.rental_product,
      scope: listParams.scope,
      branch: listParams.branch,
      channel: listParams.channel,
      status: listParams.status,
      sort: listParams.sort,
    }),
    [
      listParams.branch,
      listParams.channel,
      listParams.q,
      listParams.rental_product,
      listParams.scope,
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
      title={t("pricing.plans.title")}
      description={t("pricing.plans.description")}
      permission={permissions.tenant.pricing.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("pricing.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("layout.section_pricing") },
        { label: t("pricing.plans.title") },
      ]}
      actions={
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline" size="sm">
            <Link href={routes.tenant.pricing.plans.matrix(slug)}>
              <Grid3x3 className="size-4" />
              {can(write)
                ? t("pricing.matrix.open_edit")
                : t("pricing.matrix.open_view")}
            </Link>
          </Button>
          <EntityCreateButton
            onClick={() => setCreateOpen(true)}
            label={t("pricing.plans.create")}
            permission={write}
          />
        </div>
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(plan) =>
          router.push(routes.tenant.pricing.plans.detail(slug, plan.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("pricing.plans.load_failed")}
        onRetry={() => void listQuery.refetch()}
        realtime={listState.realtimeFor(() =>
          Promise.all([listQuery.refetch(), facetsQuery.refetch()]),
        )}
        emptyTitle={
          hasFilters
            ? t("pricing.empty_filtered_title")
            : t("pricing.plans.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("pricing.empty_filtered_description")
            : t("pricing.plans.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-pricing-rate-plans-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={RATE_PLANS_RESOURCE}
              query={engineQuery}
              capabilities={metaQuery.data?.capabilities}
              onImportComplete={refresh}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <RatePlanCreateDrawer open={createOpen} onOpenChange={setCreateOpen} />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        entityLabel={
          deleteTarget
            ? `${deleteTarget.rental_product.code} · ${t(SCOPE_LABEL_KEYS[deleteTarget.scope])}`
            : ""
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
