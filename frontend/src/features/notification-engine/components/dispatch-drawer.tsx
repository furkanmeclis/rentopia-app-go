"use client";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { EntityDrawer } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AUDIENCE_LABEL_KEYS,
  CHANNEL_LABEL_KEYS,
  DELIVERY_STATUS_LABEL_KEYS,
  DELIVERY_STATUS_TONES,
  DISPATCH_STATUS_LABEL_KEYS,
  DISPATCH_STATUS_TONES,
  EVENT_LABEL_KEYS,
} from "@/features/notification-engine/constants";
import { useDispatch } from "@/features/notification-engine/hooks/use-notification-engine";
import type { NotificationChannel } from "@/features/notification-engine/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** One dispatch: the event, its recipient and every channel attempt. */
export function DispatchDrawer({
  uuid,
  onOpenChange,
}: {
  uuid: string | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const query = useDispatch(uuid);
  const d = query.data;
  return (
    <EntityDrawer
      open={Boolean(uuid)}
      onOpenChange={onOpenChange}
      title={
        d
          ? t(EVENT_LABEL_KEYS[d.event] ?? d.event)
          : t("notification_engine.history.dispatch")
      }
      description={t("notification_engine.history.dispatch_description")}
    >
      {query.isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-6 w-1/2" />
          <Skeleton className="h-24 w-full" />
        </div>
      ) : query.isError || !d ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("notification_engine.errors.load")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <div className="space-y-5">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
            <dt className="text-muted-foreground">
              {t("notification_engine.history.columns.recipient")}
            </dt>
            <dd>
              {d.recipient_name || "—"}{" "}
              <span className="text-muted-foreground text-xs">
                ({t(AUDIENCE_LABEL_KEYS[d.audience])})
              </span>
            </dd>
            <dt className="text-muted-foreground">
              {t("notification_engine.history.result")}
            </dt>
            <dd className="flex flex-wrap items-center gap-1">
              <StatusChip
                label={t(DISPATCH_STATUS_LABEL_KEYS[d.status])}
                tone={DISPATCH_STATUS_TONES[d.status]}
              />
              {d.test ? (
                <Badge variant="outline">
                  {t("notification_engine.history.test")}
                </Badge>
              ) : null}
            </dd>
            <dt className="text-muted-foreground">
              {t("notification_engine.rules.columns.channels")}
            </dt>
            <dd>
              {d.channels
                .map((c) => t(CHANNEL_LABEL_KEYS[c as NotificationChannel]))
                .join(" → ") || "—"}
              {d.fallback
                ? ""
                : ` (${t("notification_engine.history.no_fallback")})`}
            </dd>
            <dt className="text-muted-foreground">
              {t("notification_engine.history.columns.created_at")}
            </dt>
            <dd>{datetime(d.created_at, undefined, locale)}</dd>
            {d.last_error ? (
              <>
                <dt className="text-muted-foreground">
                  {t("notification_engine.history.columns.error")}
                </dt>
                <dd className="text-destructive text-xs">{d.last_error}</dd>
              </>
            ) : null}
          </dl>
          <div className="space-y-3">
            <p className="text-sm font-medium">
              {t("notification_engine.history.attempts")}
            </p>
            {d.attempts.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("notification_engine.history.no_attempts")}
              </p>
            ) : (
              <ol className="space-y-3">
                {d.attempts.map((a) => (
                  <li key={a.uuid} className="space-y-2 rounded-md border p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="text-sm font-medium">
                        {a.attempt_no}. {t(CHANNEL_LABEL_KEYS[a.channel])}
                      </span>
                      <StatusChip
                        label={t(DELIVERY_STATUS_LABEL_KEYS[a.status])}
                        tone={DELIVERY_STATUS_TONES[a.status]}
                      />
                    </div>
                    <p className="text-muted-foreground text-xs">
                      {[a.recipient_masked, a.provider, a.provider_ref]
                        .filter(Boolean)
                        .join(" · ") || "—"}
                    </p>
                    {a.subject ? (
                      <p className="text-sm font-semibold">{a.subject}</p>
                    ) : null}
                    <p className="text-sm whitespace-pre-wrap">{a.body}</p>
                    {a.error ? (
                      <p className="text-destructive text-xs">{a.error}</p>
                    ) : null}
                    <p className="text-muted-foreground text-xs">
                      {datetime(a.created_at, undefined, locale)}
                    </p>
                  </li>
                ))}
              </ol>
            )}
          </div>
        </div>
      )}
    </EntityDrawer>
  );
}
