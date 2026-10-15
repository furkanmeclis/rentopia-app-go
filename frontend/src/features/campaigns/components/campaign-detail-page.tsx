"use client";

import type { ColumnDef, ColumnFiltersState } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CampaignStateChip,
  DiscountText,
  UsageText,
} from "@/features/campaigns/components/campaign-cells";
import { CampaignDrawer } from "@/features/campaigns/components/campaign-drawer";
import {
  REDEMPTION_STATUSES,
  REDEMPTION_STATUS_LABEL_KEYS,
  REDEMPTION_STATUS_TONES,
  SCOPE_LABEL_KEYS,
  TYPE_LABEL_KEYS,
} from "@/features/campaigns/constants";
import {
  useCampaign,
  useCampaignActivity,
  useCampaignRedemptions,
  useDeleteCampaign,
} from "@/features/campaigns/hooks/use-campaigns";
import type { Campaign, CampaignRedemption } from "@/features/campaigns/types";
import {
  ActivityFeedCard,
  useActivityLimit,
} from "@/features/extras/components/extras-shared";
import {
  CodeBadge,
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { moneyText } from "@/features/pricing/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function RedemptionsCard({ campaign }: { campaign: Campaign }) {
  const { t, locale } = useLocale();
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 });
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const query = useCampaignRedemptions(campaign.uuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
    status: joinFilter(filterValues(columnFilters, "status")),
  });
  const total = query.data?.total ?? 0;
  const columns = useMemo(
    () =>
      [
        createColumn<CampaignRedemption>({
          accessorKey: "reservation_uuid",
          labelKey: "campaigns.redemptions.columns.reservation",
          enableSorting: false,
          cell: ({ row }) => (
            <code className="text-xs">
              {row.original.reservation_uuid.slice(0, 8)}
            </code>
          ),
        }),
        createColumn<CampaignRedemption>({
          id: "customer",
          accessorFn: (row) => row.customer_uuid ?? "",
          labelKey: "campaigns.redemptions.columns.customer",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.customer_uuid ? (
              <code className="text-xs">
                {row.original.customer_uuid.slice(0, 8)}
              </code>
            ) : (
              "—"
            ),
        }),
        createColumn<CampaignRedemption>({
          accessorKey: "subtotal",
          labelKey: "campaigns.redemptions.columns.subtotal",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.subtotal, locale)}
            </span>
          ),
        }),
        createColumn<CampaignRedemption>({
          accessorKey: "discount_amount",
          labelKey: "campaigns.redemptions.columns.discount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              −{moneyText(row.original.discount_amount, locale)}
            </span>
          ),
        }),
        createColumn<CampaignRedemption>({
          accessorKey: "status",
          labelKey: "campaigns.redemptions.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: REDEMPTION_STATUSES.map((v) => ({
            value: v,
            labelKey: REDEMPTION_STATUS_LABEL_KEYS[v],
            label: t(REDEMPTION_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1">
              <StatusChip
                label={t(REDEMPTION_STATUS_LABEL_KEYS[row.original.status])}
                tone={REDEMPTION_STATUS_TONES[row.original.status]}
              />
              {row.original.status === "released" ? (
                <StatusChip
                  label={
                    row.original.usage_returned
                      ? t("campaigns.redemptions.usage_returned")
                      : t("campaigns.redemptions.usage_kept")
                  }
                />
              ) : null}
            </span>
          ),
        }),
        createColumn<CampaignRedemption>({
          accessorKey: "release_reason",
          labelKey: "campaigns.redemptions.columns.release_reason",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.release_reason || "—",
        }),
        createColumn<CampaignRedemption>({
          accessorKey: "created_at",
          labelKey: "campaigns.redemptions.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<CampaignRedemption>({
          id: "actor",
          accessorFn: (row) => row.created_by?.email ?? "",
          labelKey: "campaigns.redemptions.columns.actor",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.created_by
              ? `${row.original.created_by.name} ${row.original.created_by.surname}`.trim() ||
                row.original.created_by.email
              : "—",
        }),
      ] as ColumnDef<CampaignRedemption, unknown>[],
    [locale, t],
  );
  return (
    <EntitySectionCard
      title={t("campaigns.redemptions.title")}
      badge={query.data ? total : undefined}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("campaigns.redemptions.help")}
      </p>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("campaigns.redemptions.empty")}
        emptyDescription={t("campaigns.redemptions.empty_description")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        state={{
          pagination,
          onPaginationChange: setPagination,
          columnFilters,
          onColumnFiltersChange: setColumnFilters,
        }}
        features={{
          persistKey: "tenant-campaign-redemptions-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}

function CampaignActivity({ uuid }: { uuid: string }) {
  const { limit, more } = useActivityLimit();
  const query = useCampaignActivity(uuid, { limit, offset: 0 });
  return <ActivityFeedCard query={query} onLoadMore={more} />;
}

/** Campaign show page: summary, redemptions, activity. */
export function CampaignDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useCampaign(uuid);
  const remove = useDeleteCampaign();
  const campaign = query.data;
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const title = campaign?.name ?? t("campaigns.detail_title");
  const dateText = (value: string | null) =>
    value ? date(value, "d MMM yyyy", locale) : t("campaigns.detail.open");

  return (
    <EntityPage
      title={title}
      description={t("campaigns.detail_description")}
      permission={permissions.tenant.campaigns.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("campaigns.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("campaigns.title"),
          href: routes.tenant.pricing.campaigns.root(slug),
        },
        { label: title },
      ]}
      actions={
        campaign ? (
          <EntityActions>
            <PermissionGuard permission={permissions.tenant.campaigns.write}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setEditOpen(true)}
              >
                <Pencil className="size-4" />
                {t("campaigns.actions.edit")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 className="size-4" />
                {t("campaigns.actions.delete")}
              </Button>
            </PermissionGuard>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("campaigns.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {campaign ? (
        <div className="space-y-6">
          <EntityHeader
            title={campaign.name}
            subtitle={t(TYPE_LABEL_KEYS[campaign.campaign_type])}
            badges={
              <>
                {campaign.code ? <CodeBadge code={campaign.code} /> : null}
                <CampaignStateChip state={campaign.state} />
              </>
            }
          />
          <EntitySectionCard title={t("campaigns.detail.details")}>
            <EntityDetail
              sections={[
                {
                  id: "discount",
                  title: t("campaigns.drawer.section_discount"),
                  fields: [
                    {
                      key: "discount",
                      label: t("campaigns.fields.discount_value"),
                      value: <DiscountText campaign={campaign} />,
                    },
                    {
                      key: "min_subtotal",
                      label: t("campaigns.fields.min_subtotal"),
                      value: campaign.min_subtotal
                        ? moneyText(campaign.min_subtotal, locale)
                        : "—",
                    },
                    {
                      key: "usage",
                      label: t("campaigns.fields.usage_limit"),
                      value: <UsageText campaign={campaign} />,
                    },
                    {
                      key: "per_customer",
                      label: t("campaigns.fields.per_customer_limit"),
                      value:
                        campaign.per_customer_limit != null
                          ? String(campaign.per_customer_limit)
                          : "—",
                    },
                    ...(campaign.campaign_type === "repeat_customer"
                      ? [
                          {
                            key: "min_rentals",
                            label: t("campaigns.fields.min_completed_rentals"),
                            value: String(campaign.min_completed_rentals),
                          },
                        ]
                      : []),
                  ],
                },
                {
                  id: "validity",
                  title: t("campaigns.drawer.section_validity"),
                  fields: [
                    {
                      key: "dates",
                      label: t("campaigns.detail.validity"),
                      value:
                        campaign.start_date || campaign.end_date
                          ? `${dateText(campaign.start_date)} – ${dateText(campaign.end_date)}`
                          : t("campaigns.detail.open"),
                    },
                    {
                      key: "channels",
                      label: t("campaigns.fields.channels"),
                      value:
                        campaign.channels == null ? (
                          t("campaigns.detail.all_channels")
                        ) : (
                          <span className="flex flex-wrap gap-1">
                            {campaign.channels.map((c) => (
                              <Badge key={c} variant="outline">
                                {channelLabel(t, c)}
                              </Badge>
                            ))}
                          </span>
                        ),
                    },
                    {
                      key: "scope",
                      label: t("campaigns.fields.product_scope"),
                      value:
                        campaign.product_scope === "segments"
                          ? `${t(SCOPE_LABEL_KEYS.segments)}: ${campaign.segments.map((s) => s.name).join(", ")}`
                          : campaign.product_scope === "products"
                            ? `${t(SCOPE_LABEL_KEYS.products)}: ${campaign.products.map((p) => p.code).join(", ")}`
                            : t("campaigns.detail.all_products"),
                    },
                    {
                      key: "customers",
                      label: t("campaigns.detail.customers"),
                      value:
                        campaign.customer_uuids == null
                          ? t("campaigns.detail.all_customers")
                          : t("campaigns.detail.selected_customers", {
                              count: campaign.customer_uuids.length,
                            }),
                    },
                    {
                      key: "release",
                      label: t("campaigns.fields.release_on_cancel"),
                      value: campaign.release_on_cancel
                        ? t("campaigns.detail.yes")
                        : t("campaigns.detail.no"),
                    },
                    {
                      key: "corporate",
                      label: t("campaigns.fields.corporate_eligible"),
                      value: campaign.corporate_eligible
                        ? t("campaigns.detail.yes")
                        : t("campaigns.detail.no"),
                    },
                  ],
                },
              ]}
            />
            {campaign.description ? (
              <p className="text-muted-foreground mt-4 text-sm whitespace-pre-line">
                {campaign.description}
              </p>
            ) : null}
          </EntitySectionCard>
          <Tabs defaultValue="redemptions" className="space-y-4">
            <TabsList className="flex h-auto flex-wrap">
              <TabsTrigger value="redemptions">
                {t("campaigns.tabs.redemptions")}
              </TabsTrigger>
              <TabsTrigger value="activity">
                {t("campaigns.tabs.activity")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="redemptions">
              <RedemptionsCard campaign={campaign} />
            </TabsContent>
            <TabsContent value="activity">
              <CampaignActivity uuid={campaign.uuid} />
            </TabsContent>
          </Tabs>
          <CampaignDrawer
            open={editOpen}
            campaign={campaign}
            onOpenChange={setEditOpen}
          />
          <EntityDeleteDialog
            open={deleteOpen}
            entityLabel={campaign.name}
            isPending={remove.isPending}
            onCancel={() => setDeleteOpen(false)}
            onConfirm={() =>
              remove.mutate(campaign.uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.pricing.campaigns.root(slug)),
                onSettled: () => setDeleteOpen(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
