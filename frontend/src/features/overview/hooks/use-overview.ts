"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import {
  overviewKeys,
  overviewService,
} from "@/features/overview/services/overview.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { usePermission } from "@/providers/permission-provider";

/** Overview data refreshes on events; this is only a safety net. */
const OVERVIEW_POLL_MS = 5 * 60_000;

export function useOverview(branch: string | undefined) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: overviewKeys.page(slug, branch),
    queryFn: ({ signal }) => overviewService.get(slug, branch, signal),
    placeholderData: (previous) => previous,
    refetchInterval: OVERVIEW_POLL_MS,
  });
}

/**
 * Live resources behind the overview the member may read (the server
 * refuses the others): reservations, rentals, operation tasks, payments,
 * resolution tasks, integration jobs, documents and vehicles.
 */
export function useOverviewLiveResources(): string[] {
  const { can } = usePermission();
  return useMemo(() => {
    const p = permissions.tenant;
    const pairs: [string, string][] = [
      ["tenant.reservations", p.reservations.read],
      ["tenant.rentals", p.rentals.read],
      ["tenant.operation_tasks", p.rentals.read],
      ["tenant.vehicles", p.vehicles.read],
      ["tenant.payments", p.payments.read],
      ["tenant.resolution_tasks", p.resolutionTasks.read],
      ["tenant.integration_jobs", p.integrations.manage],
      ["tenant.documents", p.contracts.read],
    ];
    return pairs.filter(([, perm]) => can(perm)).map(([r]) => r);
  }, [can]);
}

/** Sidebar badge counts, live on reservations and resolution tasks. */
export function useOverviewBadges() {
  const { slug } = useTenant();
  const { can } = usePermission();
  const p = permissions.tenant;
  const approvals = can([p.reservations.read, p.reservations.approve]);
  const tasks = can(p.resolutionTasks.read);
  const query = useQuery({
    queryKey: overviewKeys.badges(slug),
    queryFn: ({ signal }) => overviewService.badges(slug, signal),
    enabled: Boolean(slug) && (approvals || tasks),
    staleTime: 30_000,
    refetchInterval: OVERVIEW_POLL_MS,
  });
  const resources = useMemo(
    () => [
      ...(approvals ? ["tenant.reservations"] : []),
      ...(tasks ? ["tenant.resolution_tasks"] : []),
    ],
    [approvals, tasks],
  );
  useTenantLiveRefetch(
    resources,
    () => query.refetch({ cancelRefetch: false }),
    {
      enabled: approvals || tasks,
    },
  );
  return query;
}
