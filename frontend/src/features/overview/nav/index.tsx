"use client";

import { createNavAdornment, type NavAdornment } from "@/features/nav-engine";
import { useOverviewBadges } from "@/features/overview/hooks/use-overview";
import { useLocale } from "@/providers/locale-provider";

/**
 * Approval queue item: requests and reviews waiting for staff, red when
 * one passed its SLA. Counts follow the member's branch scope.
 */
function useApprovalsNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const { data } = useOverviewBadges();
  const approvals = data?.approvals;
  if (!approvals) return { badges: [] };
  return {
    badges: [
      {
        kind: "count",
        value: approvals.pending,
        variant: approvals.overdue > 0 ? "danger" : "warning",
      },
    ],
    info:
      approvals.pending > 0
        ? {
            title: t("overview.nav.approvals_title"),
            rows: [
              {
                label: t("overview.nav.approvals_pending"),
                value: approvals.pending,
                tone: "warning",
              },
              {
                label: t("overview.nav.approvals_overdue"),
                value: approvals.overdue,
                tone: approvals.overdue > 0 ? "danger" : "secondary",
              },
            ],
          }
        : null,
  };
}

/** Resolution tasks item: open availability problems. */
function useResolutionTasksNavAdornment(): NavAdornment {
  const { data } = useOverviewBadges();
  const tasks = data?.resolution_tasks;
  if (!tasks) return { badges: [] };
  return {
    badges: [{ kind: "count", value: tasks.open, variant: "danger" }],
  };
}

export const ApprovalsNavAdornment = createNavAdornment(
  useApprovalsNavAdornment,
);
export const ResolutionTasksNavAdornment = createNavAdornment(
  useResolutionTasksNavAdornment,
);
