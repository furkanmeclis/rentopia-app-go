"use client";

import { Copy, ExternalLink, KeyRound } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  AppInput,
  AppSelect,
  AppSwitch,
  FormSection,
} from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useRenewSiteKey } from "@/features/site/hooks/use-site";
import type { SiteSettings } from "@/features/site/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const BLOCKER_KEYS: Record<SiteSettings["booking_blockers"][number], string> = {
  site_disabled: "site.general.blockers.site_disabled",
  key_missing: "site.general.blockers.key_missing",
  key_inactive: "site.general.blockers.key_inactive",
  channel_off: "site.general.blockers.channel_off",
};

/**
 * Genel: the live settings (on / off, address, default language; no
 * publish needed) and the booking state with the site's publishable key.
 */
export function SiteGeneralTab({
  settings,
  canManage,
}: {
  settings: SiteSettings;
  canManage: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const renew = useRenewSiteKey();
  const [confirmRenew, setConfirmRenew] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(settings.url);
      appToast.success(t("site.general.copied"));
    } catch {
      appToast.error(t("common.error_generic"));
    }
  };

  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.general.live_title")}
        description={t("site.general.live_description")}
        columns={2}
      >
        <AppSwitch
          name="enabled"
          label={t("site.general.enabled")}
          description={t("site.general.enabled_hint")}
          className="sm:col-span-2"
          disabled={!canManage}
        />
        <AppInput
          name="subdomain"
          label={t("site.general.subdomain")}
          description={t("site.general.subdomain_hint")}
          autoComplete="off"
          spellCheck={false}
          maxLength={63}
          disabled={!canManage}
        />
        <AppSelect
          name="default_locale"
          label={t("site.general.default_locale")}
          options={[
            { value: "tr", label: t("site.locales.tr") },
            { value: "en", label: t("site.locales.en") },
          ]}
          disabled={!canManage}
        />
        <div className="space-y-2 sm:col-span-2">
          <p className="text-sm font-medium">{t("site.general.url")}</p>
          <div className="flex flex-wrap items-center gap-2">
            <code className="bg-muted rounded px-2 py-1 font-mono text-sm break-all">
              {settings.url}
            </code>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void copy()}
            >
              <Copy className="size-4" />
              {t("site.general.copy")}
            </Button>
            {settings.live ? (
              <Button type="button" size="sm" variant="ghost" asChild>
                <a href={settings.url} target="_blank" rel="noreferrer">
                  <ExternalLink className="size-4" />
                  {t("site.general.open")}
                </a>
              </Button>
            ) : null}
          </div>
          {settings.hosts.length > 1 ? (
            <ul className="text-muted-foreground text-xs">
              {settings.hosts.slice(1).map((h) => (
                <li key={h.host}>{h.origin}</li>
              ))}
            </ul>
          ) : null}
        </div>
      </FormSection>

      <section className="border-border bg-card space-y-4 rounded-xl border p-5 sm:p-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="font-display text-base font-semibold tracking-tight">
              {t("site.general.status_title")}
            </h2>
            <p className="text-muted-foreground mt-1 text-sm">
              {settings.published_revision
                ? t("site.general.published_at", {
                    revision: settings.published_revision,
                    date: settings.published_at
                      ? datetime(settings.published_at, undefined, locale)
                      : "—",
                  })
                : t("site.general.never_published")}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <StatusChip
              label={t(settings.live ? "site.status.live" : "site.status.off")}
              tone={settings.live ? "success" : "default"}
            />
            <StatusChip
              label={t(
                settings.booking_enabled
                  ? "site.status.booking_on"
                  : "site.status.booking_off",
              )}
              tone={settings.booking_enabled ? "success" : "warning"}
            />
          </div>
        </div>
        {!settings.booking_enabled && settings.booking_blockers.length ? (
          <Alert>
            <AlertTitle>{t("site.general.blockers_title")}</AlertTitle>
            <AlertDescription>
              <ul className="list-disc space-y-1 pl-4">
                {settings.booking_blockers.map((b) => (
                  <li key={b}>{t(BLOCKER_KEYS[b])}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
          <div className="flex items-center gap-2 text-sm">
            <KeyRound className="text-muted-foreground size-4" />
            <span>{t("site.general.key")}</span>
            {settings.api_key ? (
              <StatusChip
                label={t(
                  settings.api_key.status === "active"
                    ? "site.general.key_active"
                    : "site.general.key_revoked",
                )}
                tone={
                  settings.api_key.status === "active" ? "success" : "danger"
                }
              />
            ) : (
              <StatusChip label={t("site.general.key_none")} />
            )}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" variant="ghost" asChild>
              <Link href={routes.tenant.settings.publicApi(slug)}>
                {t("site.general.keys_link")}
              </Link>
            </Button>
            {canManage ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={renew.isPending}
                onClick={() => setConfirmRenew(true)}
              >
                {t("site.general.renew_key")}
              </Button>
            ) : null}
          </div>
        </div>
      </section>

      <ConfirmDialog
        open={confirmRenew}
        title={t("site.general.renew_title")}
        description={t("site.general.renew_description")}
        confirmLabel={t("site.general.renew_key")}
        cancelLabel={t("common.cancel")}
        isPending={renew.isPending}
        onCancel={() => setConfirmRenew(false)}
        onConfirm={() =>
          renew.mutate(settings.version, {
            onSettled: () => setConfirmRenew(false),
          })
        }
      />
    </div>
  );
}
