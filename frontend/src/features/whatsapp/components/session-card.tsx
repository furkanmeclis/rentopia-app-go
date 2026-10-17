"use client";

import {
  LogOut,
  MessageCircle,
  RefreshCw,
  Send,
  Smartphone,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useState } from "react";

import { BrandLogo } from "@/components/brand/brand-logo";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ConnectDialog } from "@/features/whatsapp/components/connect-dialog";
import { QRPanel } from "@/features/whatsapp/components/qr-panel";
import { TestDialog } from "@/features/whatsapp/components/test-dialog";
import {
  PROVIDER_LABEL_KEYS,
  SESSION_STATUS_LABEL_KEYS,
  SESSION_STATUS_TONES,
} from "@/features/whatsapp/constants";
import {
  useLogoutWhatsApp,
  useRefreshWhatsApp,
  useRemoveWhatsApp,
} from "@/features/whatsapp/hooks/use-whatsapp";
import type { WhatsAppSession } from "@/features/whatsapp/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * The organization's WhatsApp session: state, linked number, the QR code
 * while pairing, and the actions (connect, refresh, test, logout, remove).
 */
export function SessionCard({ session }: { session: WhatsAppSession }) {
  const { t, locale } = useLocale();
  const refresh = useRefreshWhatsApp();
  const logout = useLogoutWhatsApp();
  const remove = useRemoveWhatsApp();
  const [connectOpen, setConnectOpen] = useState(false);
  const [testOpen, setTestOpen] = useState(false);
  const [confirm, setConfirm] = useState<"logout" | "remove" | null>(null);
  const status = session.status;
  const linked = status === "connected";
  const pairing = status === "pairing" || status === "created";
  const hasAccount = Boolean(session.account_uuid);

  const row = (label: string, value: React.ReactNode) => (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-4">
      <dt className="text-muted-foreground w-40 shrink-0 text-sm">{label}</dt>
      <dd className="text-sm">{value}</dd>
    </div>
  );

  return (
    <EntitySectionCard
      title={t("whatsapp.session.title")}
      actions={
        <StatusChip
          label={t(SESSION_STATUS_LABEL_KEYS[status])}
          tone={SESSION_STATUS_TONES[status]}
        />
      }
    >
      <div className="space-y-4">
        {status === "disconnected" ? (
          <Alert variant="destructive">
            <TriangleAlert />
            <AlertTitle>{t("whatsapp.session.dropped_title")}</AlertTitle>
            <AlertDescription>
              {t("whatsapp.session.dropped_description")}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex items-start gap-3">
          <BrandLogo provider="whatsapp" />
          <dl className="min-w-0 flex-1 space-y-2">
            {row(
              t("whatsapp.session.provider"),
              session.provider
                ? t(PROVIDER_LABEL_KEYS[session.provider] ?? session.provider)
                : "—",
            )}
            {row(
              t("whatsapp.session.phone"),
              <span className="inline-flex items-center gap-1 tabular-nums">
                <Smartphone className="size-3.5" />
                {session.phone_masked || "—"}
              </span>,
            )}
            {row(
              t("whatsapp.session.connected_at"),
              session.connected_at && linked
                ? datetime(session.connected_at, undefined, locale)
                : "—",
            )}
            {session.disconnected_at && !linked
              ? row(
                  t("whatsapp.session.disconnected_at"),
                  datetime(session.disconnected_at, undefined, locale),
                )
              : null}
            {session.last_status_at
              ? row(
                  t("whatsapp.session.last_status_at"),
                  datetime(session.last_status_at, undefined, locale),
                )
              : null}
            {session.last_error
              ? row(
                  t("whatsapp.session.last_error"),
                  <span className="text-destructive text-xs">
                    {session.last_error}
                  </span>,
                )
              : null}
          </dl>
        </div>
        {pairing && hasAccount ? <QRPanel /> : null}
        {status === "not_connected" ? (
          <p className="text-muted-foreground text-sm">
            {t("whatsapp.session.not_connected_help")}
          </p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {!linked ? (
            <Button type="button" onClick={() => setConnectOpen(true)}>
              <MessageCircle className="size-4" />
              {status === "not_connected"
                ? t("whatsapp.actions.connect")
                : t("whatsapp.actions.reconnect")}
            </Button>
          ) : (
            <Button type="button" onClick={() => setTestOpen(true)}>
              <Send className="size-4" />
              {t("whatsapp.actions.test")}
            </Button>
          )}
          {hasAccount ? (
            <Button
              type="button"
              variant="outline"
              disabled={refresh.isPending}
              onClick={() => refresh.mutate(undefined)}
            >
              <RefreshCw className="size-4" />
              {t("whatsapp.actions.refresh")}
            </Button>
          ) : null}
          {linked ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => setConfirm("logout")}
            >
              <LogOut className="size-4" />
              {t("whatsapp.actions.logout")}
            </Button>
          ) : null}
          {hasAccount ? (
            <Button
              type="button"
              variant="destructive"
              onClick={() => setConfirm("remove")}
            >
              <Trash2 className="size-4" />
              {t("whatsapp.actions.remove")}
            </Button>
          ) : null}
        </div>
      </div>
      <ConnectDialog
        open={connectOpen}
        onOpenChange={setConnectOpen}
        session={session}
      />
      <TestDialog open={testOpen} onOpenChange={setTestOpen} />
      <ConfirmDialog
        open={confirm !== null}
        title={
          confirm === "remove"
            ? t("whatsapp.confirm.remove_title")
            : t("whatsapp.confirm.logout_title")
        }
        description={
          confirm === "remove"
            ? t("whatsapp.confirm.remove_description")
            : t("whatsapp.confirm.logout_description")
        }
        confirmLabel={
          confirm === "remove"
            ? t("whatsapp.actions.remove")
            : t("whatsapp.actions.logout")
        }
        variant="destructive"
        isPending={logout.isPending || remove.isPending}
        onCancel={() => setConfirm(null)}
        onConfirm={() => {
          const m = confirm === "remove" ? remove : logout;
          m.mutate(undefined, { onSuccess: () => setConfirm(null) });
        }}
      />
    </EntitySectionCard>
  );
}
