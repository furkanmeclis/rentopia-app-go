"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { KABIS_RESOURCE } from "@/features/kabis/constants";
import { kabisService } from "@/features/kabis/services/kabis.service";
import type {
  KabisResolution,
  KabisResolveInput,
  ListKabisParams,
} from "@/features/kabis/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { usePermission } from "@/providers/permission-provider";

/** Query keys carry the organization slug. */
export const kabisKeys = {
  all: (slug: string) => ["tenant", slug, "kabis"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...kabisKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...kabisKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...kabisKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...kabisKeys.all(slug), "detail", uuid] as const,
};

export function useKabisList(params: ListKabisParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: kabisKeys.list(slug, params),
    queryFn: () => kabisService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug) && enabled,
  });
}

export function useKabisMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: kabisKeys.meta(slug),
    queryFn: () => kabisService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useKabisFacets(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: kabisKeys.facets(slug),
    queryFn: () => kabisService.facets(slug),
    staleTime: 60_000,
    enabled: Boolean(slug) && enabled,
  });
}

/**
 * "Needs action" count for the sidebar badge: facets, refreshed live from
 * `tenant.kabis_notifications` and every five minutes.
 */
export function useKabisNeedsAction() {
  const { can } = usePermission();
  const allowed = can(permissions.tenant.kabis.read);
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: kabisKeys.facets(slug),
    queryFn: () => kabisService.facets(slug),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    enabled: Boolean(slug) && allowed,
  });
  const resources = useMemo(() => [KABIS_RESOURCE], []);
  useTenantLiveRefetch(
    resources,
    () => query.refetch({ cancelRefetch: false }),
    { enabled: allowed },
  );
  return allowed ? query.data : undefined;
}

export function useKabisNotification(uuid: string) {
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: kabisKeys.detail(slug, uuid),
    queryFn: () => kabisService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
  const resources = useMemo(() => [KABIS_RESOURCE], []);
  useTenantLiveRefetch(resources, () =>
    query.refetch({ cancelRefetch: false }),
  );
  return query;
}

function useInvalidateKabis() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: kabisKeys.all(slug) });
}

/** Retry: callers render the result / error (status check first). */
export function useRetryKabis() {
  const { slug } = useTenant();
  const invalidate = useInvalidateKabis();
  return useMutation({
    mutationFn: (uuid: string) => kabisService.retry(slug, uuid),
    meta: { silent: true },
    onSettled: () => void invalidate(),
  });
}

export function useResolveKabis() {
  const { slug } = useTenant();
  const invalidate = useInvalidateKabis();
  return useMutation({
    mutationFn: (v: {
      uuid: string;
      action: KabisResolution;
      input: KabisResolveInput;
    }) => kabisService.resolve(slug, v.uuid, v.action, v.input),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function useDownloadKabisAttachment() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: async (v: { uuid: string; fallbackName: string }) => {
      const { blob, filename } = await kabisService.attachment(slug, v.uuid);
      const href = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = href;
      link.download = filename ?? v.fallbackName;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(href);
    },
  });
}
