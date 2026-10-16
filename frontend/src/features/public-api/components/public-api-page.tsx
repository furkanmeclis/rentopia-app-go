"use client";

import { useCallback, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CHANNEL_LABEL_KEYS } from "@/features/public-api/constants";
import { DeliveriesTable } from "@/features/public-api/components/deliveries-table";
import { KeysTable } from "@/features/public-api/components/keys-table";
import { WebhooksTable } from "@/features/public-api/components/webhooks-table";
import { usePublicApiOverview } from "@/features/public-api/hooks/use-public-api";
import type { PublicWebhook } from "@/features/public-api/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type Tab = "keys" | "webhooks" | "deliveries";

/**
 * Settings → Public API (TEC-728): the keys the branded site, the widget
 * and integrations use (`/v1/public/*`), webhook endpoints for reservation
 * events and their deliveries. The header shows which sales channels the
 * keys can sell on (switched in the organization settings).
 */
export function PublicApiPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const overview = usePublicApiOverview();
  const [tab, setTab] = useState<Tab>("keys");
  const [endpoint, setEndpoint] = useState<PublicWebhook | null>(null);
  const showDeliveries = useCallback((w: PublicWebhook) => {
    setEndpoint(w);
    setTab("deliveries");
  }, []);
  const data = overview.data;
  const baseUrl = data?.base_url ?? "";

  return (
    <EntityPage
      title={t("public_api.title")}
      description={t("public_api.description")}
      permission={permissions.tenant.publicApi.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("public_api.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("public_api.title") },
      ]}
    >
      <div className="mb-6 grid gap-4 md:grid-cols-3">
        <StatsCard
          title={t("public_api.overview.channels")}
          value={
            <div className="flex flex-wrap gap-2 text-sm font-normal">
              {(data?.channels ?? []).map((c) => (
                <StatusChip
                  key={c.code}
                  label={t(CHANNEL_LABEL_KEYS[c.code])}
                  tone={c.active ? "success" : "default"}
                />
              ))}
            </div>
          }
          hint={t("public_api.overview.channels_hint")}
        />
        <StatsCard
          title={t("public_api.overview.failed_deliveries")}
          value={data?.deliveries.failed ?? "—"}
          hint={t("public_api.overview.pending_deliveries", {
            count: data?.deliveries.pending ?? 0,
          })}
        />
        <StatsCard
          title={t("public_api.overview.base_url")}
          value={
            <span className="font-mono text-sm font-normal break-all">
              {baseUrl ? `${baseUrl}/v1/public` : "—"}
            </span>
          }
          hint={t("public_api.overview.docs_hint")}
        />
      </div>
      <Tabs
        value={tab}
        onValueChange={(v) => setTab(v as Tab)}
        className="space-y-4"
      >
        <TabsList>
          <TabsTrigger value="keys">{t("public_api.tabs.keys")}</TabsTrigger>
          <TabsTrigger value="webhooks">
            {t("public_api.tabs.webhooks")}
          </TabsTrigger>
          <TabsTrigger value="deliveries">
            {t("public_api.tabs.deliveries")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="keys">
          <KeysTable baseUrl={baseUrl} />
        </TabsContent>
        <TabsContent value="webhooks">
          <WebhooksTable onShowDeliveries={showDeliveries} />
        </TabsContent>
        <TabsContent value="deliveries">
          <DeliveriesTable
            endpoint={endpoint}
            onClearEndpoint={() => setEndpoint(null)}
          />
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
