"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

import { realtimeConfig } from "@/config/realtime";
import { CALENDAR_LIVE_RESOURCES } from "@/features/fleet-calendar/constants";
import { fleetCalendarService } from "@/features/fleet-calendar/services/fleet-calendar.service";
import type { FleetCalendarParams } from "@/features/fleet-calendar/types";
import { fleetOpsKeys } from "@/features/fleet-ops/hooks/use-fleet-ops";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantResourceChannel } from "@/features/tenant/lib/live";
import { parseResourceChanged, realtimeManager } from "@/lib/realtime";

/**
 * Calendar data lives under the fleet-ops keys: every block / transfer
 * mutation already invalidates `fleetOpsKeys.all(slug)`.
 */
export const fleetCalendarKeys = {
  all: (slug: string) => [...fleetOpsKeys.all(slug), "calendar"] as const,
  window: (slug: string, params: FleetCalendarParams) =>
    [...fleetCalendarKeys.all(slug), params] as const,
};

export function useFleetCalendar(params: FleetCalendarParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: fleetCalendarKeys.window(slug, params),
    queryFn: () => fleetCalendarService.get(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

const LIVE_DEBOUNCE_MS = 500;

/**
 * Refetches the visible window when vehicles, transfers or blocks of the
 * organization change (`org:{uuid}:resource:{name}`; channels the member
 * may not read are refused by the server and simply stay silent).
 * Debounced: a bulk job or a transfer touching several resources causes
 * one refetch.
 */
export function useFleetCalendarLive(refetch: () => unknown, enabled = true) {
  const { organization } = useTenant();
  const latest = useRef(refetch);
  useEffect(() => {
    latest.current = refetch;
  });

  const orgUuid = organization?.uuid;
  useEffect(() => {
    if (!enabled || !orgUuid || !realtimeConfig.enabled) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const unsubscribers: Array<() => void> = [];
    const schedule = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        timer = null;
        void Promise.resolve(latest.current()).catch(() => undefined);
      }, LIVE_DEBOUNCE_MS);
    };

    for (const resource of CALENDAR_LIVE_RESOURCES) {
      realtimeManager
        .subscribe(tenantResourceChannel(orgUuid, resource), (message) => {
          const ev = parseResourceChanged(message);
          if (ev && ev.resource === resource) schedule();
        })
        .then((unsubscribe) => {
          if (cancelled) unsubscribe();
          else unsubscribers.push(unsubscribe);
        })
        .catch(() => {
          // Forbidden channel / realtime off: the calendar still works.
        });
    }
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      for (const unsubscribe of unsubscribers) unsubscribe();
    };
  }, [enabled, orgUuid]);
}
