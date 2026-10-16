"use client";

import { Send } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DeliveriesTable } from "@/features/notification-engine/components/deliveries-table";
import { DispatchDrawer } from "@/features/notification-engine/components/dispatch-drawer";
import { RulesTable } from "@/features/notification-engine/components/rules-table";
import { TemplatesTable } from "@/features/notification-engine/components/templates-table";
import { TestDialog } from "@/features/notification-engine/components/test-dialog";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Bildirimler" (TEC-733): event rules (channel order, fallback,
 * reminders), message templates and the delivery history.
 */
export function NotificationSettingsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [tab, setTab] = useState("rules");
  const [testOpen, setTestOpen] = useState(false);
  const [sent, setSent] = useState<string | null>(null);
  return (
    <EntityPage
      title={t("notification_engine.title")}
      description={t("notification_engine.description")}
      permission={permissions.tenant.notificationSettings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("notification_engine.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.nav_tenant_settings_general"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("notification_engine.title") },
      ]}
      actions={
        <PermissionGuard
          permission={permissions.tenant.notificationSettings.write}
        >
          <Button type="button" onClick={() => setTestOpen(true)}>
            <Send className="size-4" />
            {t("notification_engine.test.open")}
          </Button>
        </PermissionGuard>
      }
    >
      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList>
          <TabsTrigger value="rules">
            {t("notification_engine.tabs.rules")}
          </TabsTrigger>
          <TabsTrigger value="templates">
            {t("notification_engine.tabs.templates")}
          </TabsTrigger>
          <TabsTrigger value="history">
            {t("notification_engine.tabs.history")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="rules">
          <RulesTable />
        </TabsContent>
        <TabsContent value="templates">
          <TemplatesTable />
        </TabsContent>
        <TabsContent value="history">
          <DeliveriesTable />
        </TabsContent>
      </Tabs>
      <TestDialog
        open={testOpen}
        onOpenChange={setTestOpen}
        onSent={(d) => setSent(d.uuid)}
      />
      <DispatchDrawer
        uuid={sent}
        onOpenChange={(next) => {
          if (!next) setSent(null);
        }}
      />
    </EntityPage>
  );
}
