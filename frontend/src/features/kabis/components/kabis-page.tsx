"use client";

import { Info, PlugZap } from "lucide-react";
import Link from "next/link";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { KabisTable } from "@/features/kabis/components/kabis-table";
import { useIntegrations } from "@/features/payments/hooks/use-payments";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * The organization's KABİS connection as the integrations page sets it:
 * none (notifications wait on a dependency), the official slot (method not
 * verified yet, TEC-682) or the development adapter.
 */
function ConnectionCard() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const accounts = useIntegrations({
    limit: 10,
    offset: 0,
    kind: "kabis",
    status: "active",
  });
  if (accounts.isLoading) return null;
  const active = accounts.data?.items?.[0];
  const state = !active
    ? {
        tone: "warning" as const,
        label: t("kabis.connection.none"),
        hint: t("kabis.connection.none_hint"),
      }
    : active.provider === "kabis"
      ? {
          tone: "warning" as const,
          label: t("kabis.connection.official"),
          hint: t("kabis.connection.official_hint"),
        }
      : {
          tone: "success" as const,
          label: t("kabis.connection.fake"),
          hint: t("kabis.connection.fake_hint"),
        };
  return (
    <div className="bg-card flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 items-start gap-3">
        <PlugZap className="text-muted-foreground mt-0.5 size-5 shrink-0" />
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-medium">
              {t("kabis.connection.title")}
            </span>
            <StatusChip label={state.label} tone={state.tone} />
            {active ? (
              <span className="text-muted-foreground text-xs">
                {active.display_name}
              </span>
            ) : null}
          </div>
          <p className="text-muted-foreground text-xs">{state.hint}</p>
        </div>
      </div>
      <Button asChild variant="outline" size="sm" className="shrink-0">
        <Link href={routes.tenant.settings.integrations(slug)}>
          {t("kabis.connection.manage")}
        </Link>
      </Button>
    </div>
  );
}

/**
 * "KABİS bildirimleri": one notification per actual handover / return
 * (and correction) of the member's branches, with what needs a person
 * first. Only "reported" states are official notifications.
 */
export function KabisPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  return (
    <EntityPage
      title={t("kabis.title")}
      description={t("kabis.description")}
      permission={permissions.tenant.kabis.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("kabis.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("kabis.title") },
      ]}
    >
      <div className="space-y-4">
        <Alert>
          <Info className="size-4" />
          <AlertTitle>{t("kabis.official_note_title")}</AlertTitle>
          <AlertDescription>{t("kabis.official_note")}</AlertDescription>
        </Alert>
        {can(permissions.tenant.integrations.manage) ? (
          <ConnectionCard />
        ) : null}
        <KabisTable persistKey="tenant-kabis-notifications-v1" />
      </div>
    </EntityPage>
  );
}
