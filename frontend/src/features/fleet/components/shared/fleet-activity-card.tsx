"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useFleetActivity } from "@/features/fleet/hooks/use-fleet";
import type {
  FleetActivityEntry,
  FleetResourceKind,
} from "@/features/fleet/types";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 10;

function actorName(entry: FleetActivityEntry) {
  if (!entry.actor) return null;
  const full = `${entry.actor.name} ${entry.actor.surname}`.trim();
  return full || entry.actor.email;
}

/** Audit trail of a segment, rental group or product. */
export function FleetActivityCard({
  kind,
  uuid,
  bare = false,
}: {
  kind: FleetResourceKind;
  uuid: string;
  /** Render without the surrounding card (inside a tab). */
  bare?: boolean;
}) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(PAGE_SIZE);
  const query = useFleetActivity(kind, uuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  const describe = (entry: FleetActivityEntry) => {
    const actionKey = `activity.actions.${entry.action}`;
    const action = t(actionKey);
    const payload = entry.payload as Record<string, unknown>;
    const version =
      typeof payload.version === "number" &&
      entry.action.endsWith("version_created")
        ? ` · v${payload.version}`
        : "";
    return (action === actionKey ? entry.action : action) + version;
  };

  const body = (
    <>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <EmptyState className="py-8" title={t("fleet.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry),
              description: actorName(entry) ?? t("fleet.activity.system_actor"),
              meta: (
                <time
                  dateTime={entry.created_at}
                  title={datetime(entry.created_at, undefined, locale)}
                >
                  {relativeDatetime(entry.created_at, locale)}
                </time>
              ),
            }))}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={query.isFetching}
              onClick={() => setLimit((current) => current + PAGE_SIZE)}
            >
              {t("fleet.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </>
  );

  if (bare) return <div className="space-y-4">{body}</div>;
  return (
    <EntitySectionCard
      title={t("fleet.activity.title")}
      badge={query.data ? total : undefined}
    >
      {body}
    </EntitySectionCard>
  );
}
