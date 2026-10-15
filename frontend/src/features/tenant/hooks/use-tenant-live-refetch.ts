"use client";

import { useEffect, useRef } from "react";

import { realtimeConfig } from "@/config/realtime";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantResourceChannel } from "@/features/tenant/lib/live";
import { parseResourceChanged, realtimeManager } from "@/lib/realtime";

const DEFAULT_DEBOUNCE_MS = 600;

/**
 * Calls `refetch` (debounced) whenever one of `resources` of the current
 * organization changes (`org:{uuid}:resource:{name}`). Pass only resources
 * the member may read: the server refuses the subscription token
 * otherwise. A burst of events (a bulk job, a payment that confirms a
 * reservation and closes a task) causes one refetch.
 */
export function useTenantLiveRefetch(
  resources: readonly string[],
  refetch: () => unknown,
  { enabled = true, debounceMs = DEFAULT_DEBOUNCE_MS } = {},
) {
  const { organization } = useTenant();
  const latest = useRef(refetch);
  useEffect(() => {
    latest.current = refetch;
  });

  const orgUuid = organization?.uuid;
  const key = [...resources].sort().join(",");
  useEffect(() => {
    if (!enabled || !orgUuid || !key || !realtimeConfig.enabled) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const unsubscribers: Array<() => void> = [];
    const schedule = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        timer = null;
        void Promise.resolve(latest.current()).catch(() => undefined);
      }, debounceMs);
    };
    for (const resource of key.split(",")) {
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
          // Realtime off or channel refused: the page still works.
        });
    }
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      for (const unsubscribe of unsubscribers) unsubscribe();
    };
  }, [enabled, orgUuid, key, debounceMs]);
}
