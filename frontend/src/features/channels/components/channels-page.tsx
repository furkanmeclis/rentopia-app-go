"use client";

import { Info } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ChannelAccountsPanel } from "@/features/channels/components/channel-accounts-panel";
import { ChannelMappingsTable } from "@/features/channels/components/channel-mappings-table";
import { ChannelMessagesTable } from "@/features/channels/components/channel-messages-table";
import { ChannelReconciliation } from "@/features/channels/components/channel-reconciliation";
import { ChannelSyncRunsTable } from "@/features/channels/components/channel-sync-runs-table";
import { CHANNEL_TABS, type ChannelTab } from "@/features/channels/constants";
import { useChannelNeedsAction } from "@/features/channels/hooks/use-channels";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const TAB_LABEL_KEYS: Record<ChannelTab, string> = {
  accounts: "channels.tabs.accounts",
  mappings: "channels.tabs.mappings",
  messages: "channels.tabs.messages",
  runs: "channels.tabs.runs",
  reconciliation: "channels.tabs.reconciliation",
};

/**
 * "Kanal yöneticisi": marketplaces selling the organization's products
 * (Yolcu360, later Obilet). Accounts and sync state, mappings, the message
 * / error queue, the sync log and the reconciliation report as tabs (the
 * tab is kept in the URL).
 */
export function ChannelsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const raw = searchParams.get("tab");
  const tab: ChannelTab = CHANNEL_TABS.includes(raw as ChannelTab)
    ? (raw as ChannelTab)
    : "accounts";
  const facets = useChannelNeedsAction();
  const setTab = (value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", value);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };
  return (
    <EntityPage
      title={t("channels.title")}
      description={t("channels.description")}
      permission={permissions.tenant.channels.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("channels.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("channels.title") },
      ]}
    >
      <div className="space-y-4">
        <Alert>
          <Info className="size-4" />
          <AlertTitle>{t("channels.note_title")}</AlertTitle>
          <AlertDescription>{t("channels.note")}</AlertDescription>
        </Alert>
        <Tabs value={tab} onValueChange={setTab} className="space-y-4">
          <TabsList className="flex h-auto flex-wrap">
            {CHANNEL_TABS.map((key) => (
              <TabsTrigger key={key} value={key}>
                {t(TAB_LABEL_KEYS[key])}
                {key === "messages" && (facets?.needs_action ?? 0) > 0 ? (
                  <Badge variant="danger" className="ml-1.5">
                    {facets?.needs_action}
                  </Badge>
                ) : null}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="accounts">
            <ChannelAccountsPanel />
          </TabsContent>
          <TabsContent value="mappings">
            <ChannelMappingsTable />
          </TabsContent>
          <TabsContent value="messages">
            <ChannelMessagesTable />
          </TabsContent>
          <TabsContent value="runs">
            <ChannelSyncRunsTable />
          </TabsContent>
          <TabsContent value="reconciliation">
            <ChannelReconciliation />
          </TabsContent>
        </Tabs>
      </div>
    </EntityPage>
  );
}
