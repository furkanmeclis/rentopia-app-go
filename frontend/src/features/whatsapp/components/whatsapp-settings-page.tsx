"use client";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { MessagesTable } from "@/features/whatsapp/components/messages-table";
import { SessionCard } from "@/features/whatsapp/components/session-card";
import { SettingsCard } from "@/features/whatsapp/components/settings-card";
import { useWhatsAppSession } from "@/features/whatsapp/hooks/use-whatsapp";
import { useLocale } from "@/providers/locale-provider";

/**
 * Settings → WhatsApp (TEC-735): link the organization's own number (QR),
 * watch the session, the send limit and the message log with delivery and
 * read receipts. Customer notifications use it through the notification
 * rules (channel "WhatsApp").
 */
export function WhatsAppSettingsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const session = useWhatsAppSession();
  return (
    <EntityPage
      title={t("whatsapp.title")}
      description={t("whatsapp.description")}
      permission={permissions.tenant.integrations.manage}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("whatsapp.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.nav_tenant_settings_general"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("whatsapp.title") },
      ]}
    >
      <div className="space-y-6">
        {session.isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : session.isError || !session.data ? (
          <ErrorState
            title={t("whatsapp.errors.session")}
            onRetry={() => void session.refetch()}
          />
        ) : (
          <div className="grid gap-6 lg:grid-cols-3">
            <div className="lg:col-span-2">
              <SessionCard session={session.data} />
            </div>
            <SettingsCard
              key={`${session.data.account_uuid ?? "none"}-${session.data.rate_limit_per_minute}`}
              session={session.data}
            />
          </div>
        )}
        <EntitySectionCard title={t("whatsapp.messages.title")}>
          <MessagesTable />
        </EntitySectionCard>
      </div>
    </EntityPage>
  );
}
