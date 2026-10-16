"use client";

import {
  CloudDownload,
  CloudUpload,
  FlaskConical,
  PlugZap,
  Settings2,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { BrandLogo } from "@/components/brand/brand-logo";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  ChannelSettingsDrawer,
  ChannelSimulateDialog,
  toastRun,
} from "@/features/channels/components/channel-account-dialogs";
import {
  channelProblemText,
  providerText,
} from "@/features/channels/components/channel-cells";
import {
  COLLECTED_BY_LABEL_KEYS,
  HEALTH_LABEL_KEYS,
  HEALTH_TONES,
} from "@/features/channels/constants";
import {
  useChannelAccounts,
  useChannelSync,
} from "@/features/channels/hooks/use-channels";
import type { ChannelAccount } from "@/features/channels/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="truncate text-sm">{value}</dd>
    </div>
  );
}

/** One channel account: health, last push / pull, settings and actions. */
function AccountCard({
  account,
  onSettings,
  onSimulate,
}: {
  account: ChannelAccount;
  onSettings: () => void;
  onSimulate: () => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const sync = useChannelSync();
  const canSync = can(permissions.tenant.channels.sync);
  const canWrite = can(permissions.tenant.channels.write);
  const s = account.sync;
  const when = (v: string | null) =>
    v ? datetime(v, "d MMM yyyy HH:mm", locale) : "—";
  const run = (action: "push" | "pull") =>
    sync.mutate(
      { uuid: account.uuid, action },
      {
        onSuccess: (r) => toastRun(t, r),
        onError: (error) => appToast.error(channelProblemText(t, error)),
      },
    );
  const usable = account.status === "active" && account.available;
  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <BrandLogo provider={account.provider} />
          <div className="min-w-0 space-y-1">
            <CardTitle className="flex flex-wrap items-center gap-2 text-base">
              {account.display_name}
              <StatusChip
                label={t(HEALTH_LABEL_KEYS[s.health])}
                tone={HEALTH_TONES[s.health]}
              />
            </CardTitle>
            <CardDescription>
              {providerText(t, account.provider)} ·{" "}
              {account.mode === "live"
                ? t("channels.accounts.mode_live")
                : t("channels.accounts.mode_sandbox")}
              {account.status !== "active"
                ? ` · ${t("channels.accounts.disabled")}`
                : ""}
            </CardDescription>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {canSync && usable ? (
            <>
              <Button
                size="sm"
                variant="outline"
                disabled={sync.isPending}
                onClick={() => run("push")}
              >
                <CloudUpload className="size-4" />
                {t("channels.actions.push")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={sync.isPending}
                onClick={() => run("pull")}
              >
                <CloudDownload className="size-4" />
                {t("channels.actions.pull")}
              </Button>
              {account.simulator ? (
                <Button size="sm" variant="outline" onClick={onSimulate}>
                  <FlaskConical className="size-4" />
                  {t("channels.actions.simulate")}
                </Button>
              ) : null}
            </>
          ) : null}
          {canWrite ? (
            <Button size="sm" variant="secondary" onClick={onSettings}>
              <Settings2 className="size-4" />
              {t("channels.actions.settings")}
            </Button>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {!account.available ? (
          <Alert>
            <AlertDescription>
              {t("channels.accounts.adapter_unavailable")}
            </AlertDescription>
          </Alert>
        ) : null}
        {s.health === "expired" ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t("channels.accounts.expired_hint", {
                policy:
                  account.settings.offline_policy === "quota"
                    ? t("channels.accounts.offline_quota", {
                        quota: account.settings.offline_quota,
                      })
                    : t("channels.accounts.offline_close"),
              })}
            </AlertDescription>
          </Alert>
        ) : null}
        <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Fact
            label={t("channels.accounts.last_push")}
            value={when(s.last_push_at)}
          />
          <Fact
            label={t("channels.accounts.last_pull")}
            value={when(s.last_pull_at)}
          />
          <Fact
            label={t("channels.accounts.valid_until")}
            value={when(s.pushed_valid_until)}
          />
          <Fact
            label={t("channels.accounts.offline_policy")}
            value={
              account.settings.offline_policy === "quota"
                ? t("channels.accounts.offline_quota", {
                    quota: account.settings.offline_quota,
                  })
                : t("channels.accounts.offline_close")
            }
          />
          <Fact
            label={t("channels.accounts.open_errors")}
            value={String(account.open_errors)}
          />
          <Fact
            label={t("channels.accounts.active_mappings")}
            value={String(account.active_mappings)}
          />
          <Fact
            label={t("channels.accounts.horizon")}
            value={t("channels.accounts.horizon_days", {
              days: account.settings.horizon_days,
            })}
          />
          <Fact
            label={t("channels.accounts.collected_by")}
            value={t(COLLECTED_BY_LABEL_KEYS[account.settings.collected_by])}
          />
        </dl>
        {s.last_push_error || s.last_pull_error ? (
          <p className="text-destructive text-xs break-words">
            {s.last_push_error || s.last_pull_error}
          </p>
        ) : null}
        {account.settings.stop_sale ? (
          <StatusChip
            label={t("channels.accounts.stop_sale_on")}
            tone="danger"
          />
        ) : null}
      </CardContent>
    </Card>
  );
}

/**
 * Channel accounts ("Kanal hesapları"): the connection itself lives on the
 * integrations page; here are the sync state, settings and actions.
 */
export function ChannelAccountsPanel() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const accounts = useChannelAccounts();
  const [settings, setSettings] = useState<ChannelAccount | null>(null);
  const [simulate, setSimulate] = useState<ChannelAccount | null>(null);
  if (accounts.isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }
  if (accounts.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        description={t("channels.load_failed")}
        onRetry={() => void accounts.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const items = accounts.data ?? [];
  return (
    <div className="space-y-4">
      <div className="bg-card flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <PlugZap className="text-muted-foreground mt-0.5 size-5 shrink-0" />
          <div className="min-w-0 space-y-1">
            <p className="text-sm font-medium">
              {t("channels.accounts.connect_title")}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("channels.accounts.connect_hint")}
            </p>
          </div>
        </div>
        {can(permissions.tenant.integrations.manage) ? (
          <Button asChild variant="outline" size="sm" className="shrink-0">
            <Link href={routes.tenant.settings.integrations(slug)}>
              {t("channels.accounts.manage")}
            </Link>
          </Button>
        ) : null}
      </div>
      {items.length === 0 ? (
        <Alert>
          <AlertTitle>{t("channels.accounts.empty_title")}</AlertTitle>
          <AlertDescription>
            {t("channels.accounts.empty_description")}
          </AlertDescription>
        </Alert>
      ) : (
        items.map((a) => (
          <AccountCard
            key={a.uuid}
            account={a}
            onSettings={() => setSettings(a)}
            onSimulate={() => setSimulate(a)}
          />
        ))
      )}
      <ChannelSettingsDrawer
        account={settings}
        onClose={() => setSettings(null)}
      />
      <ChannelSimulateDialog
        account={simulate}
        onClose={() => setSimulate(null)}
      />
    </div>
  );
}
