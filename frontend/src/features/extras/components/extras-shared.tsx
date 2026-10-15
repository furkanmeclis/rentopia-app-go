"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import {
  ALLOCATION_STATUS_LABEL_KEYS,
  ALLOCATION_STATUS_TONES,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
} from "@/features/extras/constants";
import type {
  AllocationStatus,
  ExtraActivityPage,
  ExtraStatus,
} from "@/features/extras/types";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function ExtraStatusChip({ status }: { status: ExtraStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(STATUS_LABEL_KEYS[status])}
      tone={STATUS_TONES[status]}
    />
  );
}

export function AllocationStatusChip({
  status,
  lapsed,
}: {
  status: AllocationStatus;
  lapsed?: boolean;
}) {
  const { t } = useLocale();
  if (lapsed) {
    return (
      <StatusChip
        label={t("extras.allocations.status.lapsed")}
        tone="warning"
      />
    );
  }
  return (
    <StatusChip
      label={t(ALLOCATION_STATUS_LABEL_KEYS[status])}
      tone={ALLOCATION_STATUS_TONES[status]}
    />
  );
}

type ActivityQuery = {
  data?: ExtraActivityPage;
  isLoading: boolean;
  isError: boolean;
  isFetching: boolean;
  refetch: () => unknown;
};

export const ACTIVITY_PAGE = 20;

/**
 * Audit timeline card shared by extras and campaigns. The caller owns the
 * query (its key carries the limit); "load more" raises the limit.
 */
export function ActivityFeedCard({
  query,
  onLoadMore,
}: {
  query: ActivityQuery;
  onLoadMore: () => void;
}) {
  const { t, locale } = useLocale();
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const describe = (action: string) => {
    const key = `activity.actions.${action}`;
    const text = t(key);
    return text === key ? action : text;
  };
  return (
    <EntitySectionCard
      title={t("extras.activity.title")}
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
        <EmptyState className="py-8" title={t("extras.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry.action),
              description: entry.actor
                ? `${entry.actor.name} ${entry.actor.surname}`.trim() ||
                  entry.actor.email
                : t("extras.activity.system_actor"),
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
              onClick={onLoadMore}
            >
              {t("extras.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/** Limit state for an activity feed ("load more" adds a page). */
export function useActivityLimit() {
  const [limit, setLimit] = useState(ACTIVITY_PAGE);
  return { limit, more: () => setLimit((current) => current + ACTIVITY_PAGE) };
}
