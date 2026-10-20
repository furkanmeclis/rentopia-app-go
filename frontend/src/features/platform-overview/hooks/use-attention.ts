"use client";

import { differenceInCalendarDays, parseISO } from "date-fns";
import { useMemo, useState } from "react";

import { routes } from "@/config/routes";
import { ACCESS_LABEL_KEYS } from "@/features/organizations/constants";
import { SEVERITY_LABEL_KEYS } from "@/features/platform-health/components/health-chips";
import { healthLabel } from "@/features/platform-health/lib/labels";
import { ACCESS_ENDING_DAYS } from "@/features/platform-overview/constants";
import {
  useOpenIssues,
  useOrganizationSignals,
  useSiteDomains,
  type OverviewScopes,
} from "@/features/platform-overview/hooks/use-overview-query";
import type { SiteDomain } from "@/features/site/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type AttentionTone = "danger" | "warning";

export type AttentionItem = {
  key: string;
  tone: AttentionTone;
  title: string;
  meta: string;
  badge: string;
  href: string;
  /** Newest first inside a tone. */
  at: string;
};

export type AttentionSource = "organizations" | "issues" | "domains";

export type AttentionFailure = {
  source: AttentionSource;
  retry: () => void;
};

export type AttentionCategory = {
  source: AttentionSource;
  count: number;
  href: string;
};

/** A domain the platform team has to act on, or `null` when it is fine. */
function domainProblem(
  d: SiteDomain,
): { tone: AttentionTone; labelKey: string } | null {
  if (d.status === "disabled") return null;
  if (d.status === "failed")
    return { tone: "danger", labelKey: "dashboard.attention.domain_failed" };
  if (d.status === "pending")
    return { tone: "warning", labelKey: "dashboard.attention.domain_pending" };
  if (d.tls_status === "failed")
    return {
      tone: "danger",
      labelKey: "dashboard.attention.domain_tls_failed",
    };
  if (d.tls_status !== "active")
    return { tone: "warning", labelKey: "dashboard.attention.domain_tls" };
  return null;
}

/**
 * Everything on the platform that needs a person: expired or ending
 * access windows, suspended organizations, open connection issues and
 * custom domains stuck in verification or TLS. Each source is read only
 * with its permission; a failed source is reported, the rest still show.
 */
export function useAttention(scopes: OverviewScopes) {
  const { t, locale } = useLocale();
  const orgs = useOrganizationSignals(scopes.organizations);
  const issues = useOpenIssues(scopes.health);
  const domains = useSiteDomains(scopes.siteDomains);

  // Day the page was opened; the attention list is about calendar days.
  const [today] = useState(() => new Date().toDateString());

  return useMemo(() => {
    const items: AttentionItem[] = [];
    const failures: AttentionFailure[] = [];
    const categories: AttentionCategory[] = [];
    let isLoading = false;

    if (scopes.organizations) {
      const { expired, ending, suspended } = orgs;
      isLoading ||=
        expired.isLoading || ending.isLoading || suspended.isLoading;
      if (expired.isError || ending.isError || suspended.isError) {
        failures.push({
          source: "organizations",
          retry: () => {
            void expired.refetch();
            void ending.refetch();
            void suspended.refetch();
          },
        });
      }
      for (const org of expired.data?.items ?? []) {
        items.push({
          key: `expired:${org.uuid}`,
          tone: "danger",
          title: org.name,
          meta: org.access_ends_at
            ? t("dashboard.attention.expired_meta", {
                date: datetime(org.access_ends_at, "d MMMM yyyy", locale),
              })
            : "",
          badge: t(ACCESS_LABEL_KEYS.expired),
          href: routes.platform.organizations.detail(org.uuid),
          at: org.access_ends_at ?? org.updated_at,
        });
      }
      const endingSoon = (ending.data?.items ?? []).filter((org) => {
        if (!org.access_ends_at) return false;
        const days = differenceInCalendarDays(
          parseISO(org.access_ends_at),
          new Date(today),
        );
        return days >= 0 && days <= ACCESS_ENDING_DAYS;
      });
      for (const org of endingSoon) {
        const ends = org.access_ends_at as string;
        const days = differenceInCalendarDays(parseISO(ends), new Date(today));
        items.push({
          key: `ending:${org.uuid}`,
          tone: "warning",
          title: org.name,
          meta: t("dashboard.attention.ending_meta", {
            date: datetime(ends, "d MMMM yyyy", locale),
          }),
          badge:
            days === 0
              ? t("dashboard.attention.ending_today")
              : days === 1
                ? t("dashboard.attention.ending_badge_one")
                : t("dashboard.attention.ending_badge_other", { count: days }),
          href: routes.platform.organizations.detail(org.uuid),
          at: ends,
        });
      }
      const suspendedCount = suspended.data?.total ?? 0;
      if (suspendedCount > 0) {
        items.push({
          key: "suspended",
          tone: "warning",
          title:
            suspendedCount === 1
              ? t("dashboard.attention.suspended_title_one")
              : t("dashboard.attention.suspended_title_other", {
                  count: suspendedCount,
                }),
          meta: t("dashboard.attention.suspended_meta"),
          badge: t("organizations.status.suspended"),
          href: `${routes.platform.organizations.root}?status=suspended`,
          at: "",
        });
      }
      const expiredTotal = expired.data?.total ?? 0;
      const orgCount = expiredTotal + endingSoon.length + suspendedCount;
      if (orgCount > 0) {
        categories.push({
          source: "organizations",
          count: orgCount,
          href:
            expiredTotal > 0
              ? `${routes.platform.organizations.root}?access=expired`
              : `${routes.platform.organizations.root}?window=limited&sort=access_ends_at`,
        });
      }
    }

    if (scopes.health) {
      isLoading ||= issues.isLoading;
      if (issues.isError) {
        failures.push({ source: "issues", retry: () => void issues.refetch() });
      }
      for (const issue of issues.data?.items ?? []) {
        items.push({
          key: `issue:${issue.key}`,
          tone: issue.severity === "critical" ? "danger" : "warning",
          title: [
            healthLabel(locale, "kinds", issue.kind),
            issue.organization?.name ?? t("platform_health.issues.platform"),
          ].join(" · "),
          meta: issue.message || issue.subject,
          badge: t(SEVERITY_LABEL_KEYS[issue.severity]),
          href: issue.organization
            ? routes.platform.organizations.detail(issue.organization.uuid)
            : `${routes.platform.health}?tab=issues`,
          at: issue.occurred_at,
        });
      }
      const total = issues.data?.total ?? 0;
      if (total > 0) {
        categories.push({
          source: "issues",
          count: total,
          href: `${routes.platform.health}?tab=issues`,
        });
      }
    }

    if (scopes.siteDomains) {
      isLoading ||= domains.isLoading;
      if (domains.isError) {
        failures.push({
          source: "domains",
          retry: () => void domains.refetch(),
        });
      }
      let count = 0;
      for (const d of domains.data ?? []) {
        const problem = domainProblem(d);
        if (!problem) continue;
        count += 1;
        items.push({
          key: `domain:${d.uuid}`,
          tone: problem.tone,
          title: d.host,
          meta:
            d.last_error ||
            (d.last_checked_at
              ? t("dashboard.attention.domain_checked", {
                  date: datetime(d.last_checked_at, "d MMM yyyy HH:mm", locale),
                })
              : t("dashboard.attention.domain_never_checked")),
          badge: t(problem.labelKey),
          href: routes.platform.siteDomains,
          at: d.updated_at,
        });
      }
      if (count > 0) {
        categories.push({
          source: "domains",
          count,
          href: routes.platform.siteDomains,
        });
      }
    }

    items.sort((a, b) => {
      if (a.tone !== b.tone) return a.tone === "danger" ? -1 : 1;
      return b.at.localeCompare(a.at);
    });
    const total = categories.reduce((sum, c) => sum + c.count, 0);
    return { items, total, categories, failures, isLoading };
  }, [scopes, orgs, issues, domains, t, locale, today]);
}
