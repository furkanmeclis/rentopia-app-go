"use client";

import { Pencil, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

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
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { EquipmentAllocationsTab } from "@/features/extras/components/equipment-allocations-tab";
import { ExtraDrawer } from "@/features/extras/components/extra-drawer";
import { ExtraPricesTab } from "@/features/extras/components/extra-prices-tab";
import { ExtraStockTab } from "@/features/extras/components/extra-stock-tab";
import {
  ActivityFeedCard,
  ExtraStatusChip,
  useActivityLimit,
} from "@/features/extras/components/extras-shared";
import {
  CHARGE_HINT_KEYS,
  CHARGE_LABEL_KEYS,
  TYPE_LABEL_KEYS,
} from "@/features/extras/constants";
import {
  useDeleteExtra,
  useExtra,
  useExtraActivity,
} from "@/features/extras/hooks/use-extras";
import { CodeBadge } from "@/features/fleet/components/shared/list-utils";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { moneyText } from "@/features/pricing/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function ExtraActivity({ uuid }: { uuid: string }) {
  const { limit, more } = useActivityLimit();
  const query = useExtraActivity(uuid, { limit, offset: 0 });
  return <ActivityFeedCard query={query} onLoadMore={more} />;
}

/**
 * Extra show page: summary, per-product price matrix with history, branch
 * stock, equipment allocations and activity.
 */
export function ExtraDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useExtra(uuid);
  const remove = useDeleteExtra();
  const extra = query.data;
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const title = extra?.name ?? t("extras.detail_title");

  return (
    <EntityPage
      title={title}
      description={t("extras.detail_description")}
      permission={permissions.tenant.extras.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("extras.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("extras.title"),
          href: routes.tenant.pricing.extras.root(slug),
        },
        { label: title },
      ]}
      actions={
        extra ? (
          <EntityActions>
            <PermissionGuard permission={permissions.tenant.extras.write}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setEditOpen(true)}
              >
                <Pencil className="size-4" />
                {t("extras.actions.edit")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 className="size-4" />
                {t("extras.actions.delete")}
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
          description={t("extras.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {extra ? (
        <div className="space-y-6">
          <EntityHeader
            title={extra.name}
            subtitle={t(TYPE_LABEL_KEYS[extra.extra_type])}
            badges={
              <>
                <CodeBadge code={extra.code} />
                <ExtraStatusChip status={extra.status} />
                {extra.is_physical ? (
                  <StatusChip
                    label={t("extras.physical.true")}
                    tone="success"
                  />
                ) : null}
              </>
            }
          />
          <EntitySectionCard title={t("extras.detail.details")}>
            <EntityDetail
              sections={[
                {
                  id: "extra",
                  fields: [
                    {
                      key: "charge",
                      label: t("extras.fields.charge_type"),
                      value: (
                        <span className="flex flex-col">
                          <span>{t(CHARGE_LABEL_KEYS[extra.charge_type])}</span>
                          <span className="text-muted-foreground text-xs">
                            {t(CHARGE_HINT_KEYS[extra.charge_type])}
                          </span>
                        </span>
                      ),
                    },
                    {
                      key: "default_price",
                      label: t("extras.fields.default_price"),
                      value: moneyText(extra.default_price, locale),
                    },
                    {
                      key: "quantity",
                      label: t("extras.detail.quantity"),
                      value: t("extras.detail.quantity_range", {
                        min: extra.min_quantity,
                        max: extra.max_quantity,
                      }),
                    },
                    {
                      key: "max_total",
                      label: t("extras.fields.max_total"),
                      value: extra.max_total
                        ? moneyText(extra.max_total, locale)
                        : t("extras.detail.no_cap"),
                    },
                    {
                      key: "channels",
                      label: t("extras.fields.channels"),
                      value:
                        extra.channels == null ? (
                          t("extras.detail.all_channels")
                        ) : (
                          <span className="flex flex-wrap gap-1">
                            {extra.channels.map((c) => (
                              <Badge key={c} variant="outline">
                                {channelLabel(t, c)}
                              </Badge>
                            ))}
                          </span>
                        ),
                    },
                    {
                      key: "description",
                      label: t("extras.fields.description"),
                      value: extra.description || "—",
                    },
                    {
                      key: "updated_at",
                      label: t("extras.detail.updated_at"),
                      value: datetime(extra.updated_at, undefined, locale),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>
          <Tabs defaultValue="prices" className="space-y-4">
            <TabsList className="flex h-auto flex-wrap">
              <TabsTrigger value="prices">
                {t("extras.tabs.prices")}
              </TabsTrigger>
              {extra.is_physical ? (
                <>
                  <TabsTrigger value="stock">
                    {t("extras.tabs.stock")}
                  </TabsTrigger>
                  <TabsTrigger value="allocations">
                    {t("extras.tabs.allocations")}
                  </TabsTrigger>
                </>
              ) : null}
              <TabsTrigger value="activity">
                {t("extras.tabs.activity")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="prices">
              <ExtraPricesTab extra={extra} />
            </TabsContent>
            {extra.is_physical ? (
              <>
                <TabsContent value="stock">
                  <ExtraStockTab extra={extra} />
                </TabsContent>
                <TabsContent value="allocations">
                  <EquipmentAllocationsTab extra={extra} />
                </TabsContent>
              </>
            ) : null}
            <TabsContent value="activity">
              <ExtraActivity uuid={extra.uuid} />
            </TabsContent>
          </Tabs>
          <ExtraDrawer
            open={editOpen}
            extra={extra}
            onOpenChange={setEditOpen}
          />
          <EntityDeleteDialog
            open={deleteOpen}
            entityLabel={`${extra.code} · ${extra.name}`}
            isPending={remove.isPending}
            onCancel={() => setDeleteOpen(false)}
            onConfirm={() =>
              remove.mutate(extra.uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.pricing.extras.root(slug)),
                onSettled: () => setDeleteOpen(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
