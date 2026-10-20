"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { Building2, History, Plus } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import type { ActivityListResult } from "@/features/io/services/activity.service";
import { formatActivityResource } from "@/features/io/lib/display";
import {
  OrganizationAccessChip,
  OrganizationStatusChip,
  ownerLabel,
} from "@/features/organizations/components/organization-cells";
import type { OrganizationSummaryPage } from "@/features/organizations/types";
import {
  OverviewPanel,
  PanelError,
  PanelNote,
  PanelRowsSkeleton,
} from "@/features/platform-overview/components/overview-panel";
import { OrganizationAvatar } from "@/features/tenant/components/organization-avatar";
import { relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Newest organizations with owner, fleet size and access state. */
export function RecentOrganizationsPanel({
  query,
  canCreate,
}: {
  query: UseQueryResult<OrganizationSummaryPage>;
  canCreate: boolean;
}) {
  const { t, locale } = useLocale();
  const page = query.data;

  return (
    <OverviewPanel
      title={t("dashboard.recent_orgs.title")}
      link={
        page && page.total > 0
          ? {
              href: routes.platform.organizations.root,
              label: t("dashboard.see_all_count", { count: page.total }),
            }
          : undefined
      }
    >
      {query.isError && !page ? (
        <PanelError
          description={t("dashboard.recent_orgs.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : !page ? (
        <PanelRowsSkeleton rows={4} />
      ) : page.items.length === 0 ? (
        <PanelNote
          icon={<Building2 className="size-5" />}
          title={t("dashboard.recent_orgs.empty_title")}
          description={t("dashboard.recent_orgs.empty_description")}
          action={
            canCreate ? (
              <Button asChild size="sm">
                <Link href={routes.platform.organizations.create}>
                  <Plus className="size-4" />
                  {t("dashboard.actions.create_organization")}
                </Link>
              </Button>
            ) : undefined
          }
        />
      ) : (
        <ul className="divide-border -mx-2 divide-y">
          {page.items.map((org) => (
            <li key={org.uuid}>
              <Link
                href={routes.platform.organizations.detail(org.uuid)}
                className="hover:bg-muted/60 focus-visible:ring-ring flex items-center gap-3 rounded-xl px-2 py-2.5 outline-none focus-visible:ring-2"
              >
                <OrganizationAvatar
                  logoUrl={org.logo_url}
                  className="size-9 shrink-0 rounded-xl"
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {org.name}
                  </span>
                  <span className="text-muted-foreground block truncate text-xs">
                    {[
                      org.owner
                        ? ownerLabel(org.owner)
                        : t("dashboard.recent_orgs.owner_pending"),
                      org.vehicle_count === 1
                        ? t("dashboard.recent_orgs.vehicles_one")
                        : t("dashboard.recent_orgs.vehicles_other", {
                            count: org.vehicle_count,
                          }),
                      t("dashboard.recent_orgs.created", {
                        time: relativeDatetime(org.created_at, locale),
                      }),
                    ].join(" · ")}
                  </span>
                </span>
                <span className="shrink-0">
                  {org.status === "suspended" ? (
                    <OrganizationStatusChip status={org.status} />
                  ) : (
                    <OrganizationAccessChip state={org.access_state} />
                  )}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </OverviewPanel>
  );
}

/** Latest entries of the platform activity log. */
export function RecentActivityPanel({
  query,
}: {
  query: UseQueryResult<ActivityListResult>;
}) {
  const { t, locale } = useLocale();
  const page = query.data;

  const actionLabel = (action: string) => {
    const key = `activity.actions.${action}`;
    const label = t(key);
    return label === key ? action : label;
  };

  return (
    <OverviewPanel
      title={t("dashboard.activity.title")}
      link={{
        href: routes.platform.settings.activity,
        label: t("dashboard.see_all"),
      }}
    >
      {query.isError && !page ? (
        <PanelError
          description={t("dashboard.activity.load_failed")}
          onRetry={() => void query.refetch()}
        />
      ) : !page ? (
        <PanelRowsSkeleton rows={5} />
      ) : page.items.length === 0 ? (
        <PanelNote
          icon={<History className="size-5" />}
          title={t("dashboard.activity.empty_title")}
          description={t("dashboard.activity.empty_description")}
        />
      ) : (
        <ol className="relative space-y-0">
          {page.items.map((event, index) => (
            <li key={event.uuid} className="relative flex gap-3 pb-3 last:pb-0">
              <span
                aria-hidden
                className="relative flex w-3 shrink-0 justify-center pt-1.5"
              >
                <span className="bg-primary/70 ring-card relative z-10 size-2 rounded-full ring-4" />
                {index < page.items.length - 1 ? (
                  <span className="bg-border absolute top-3 bottom-[-0.75rem] w-px" />
                ) : null}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm">
                  {actionLabel(event.action)}
                </span>
                <span className="text-muted-foreground block truncate text-xs">
                  {formatActivityResource(t, event.resource)}
                  {" · "}
                  <time dateTime={event.created_at}>
                    {relativeDatetime(event.created_at, locale)}
                  </time>
                </span>
              </span>
            </li>
          ))}
        </ol>
      )}
    </OverviewPanel>
  );
}
