"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useCustomerActivity } from "@/features/customers/hooks/use-customers";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE = 20;

/** "Aktivite" tab: changes, reveals (with reason), downloads, consents. */
export function CustomerActivityCard({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(PAGE);
  const query = useCustomerActivity(uuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const describe = (action: string) => {
    const key = `activity.actions.${action}`;
    const label = t(key);
    return label === key ? action : label;
  };
  const detail = (payload: Record<string, unknown>) => {
    const reason = typeof payload.reason === "string" ? payload.reason : "";
    return reason ? t("customers.activity.reason", { reason }) : null;
  };

  return (
    <EntitySectionCard
      title={t("customers.activity.title")}
      badge={query.data ? total : undefined}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <EmptyState className="py-8" title={t("customers.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => {
              const actor = entry.actor
                ? `${entry.actor.name} ${entry.actor.surname}`.trim() ||
                  entry.actor.email
                : t("customers.activity.system");
              const extra = detail(entry.payload ?? {});
              return {
                id: entry.uuid,
                title: describe(entry.action),
                description: extra ? `${actor} · ${extra}` : actor,
                meta: (
                  <time
                    dateTime={entry.created_at}
                    title={datetime(entry.created_at, undefined, locale)}
                  >
                    {relativeDatetime(entry.created_at, locale)}
                  </time>
                ),
              };
            })}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={query.isFetching}
              onClick={() => setLimit((current) => current + PAGE)}
            >
              {t("customers.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}
