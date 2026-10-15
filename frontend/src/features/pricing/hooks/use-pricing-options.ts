"use client";

import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useSegmentsList } from "@/features/fleet/hooks/use-fleet";
import { useRatePlanMatrix } from "@/features/pricing/hooks/use-pricing";
import {
  SALES_CHANNELS,
  SALES_CHANNEL_LABEL_KEYS,
} from "@/features/tenant-settings/constants";
import type { SalesChannelCode } from "@/features/tenant-settings/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type Option = { value: string; label: string };

/**
 * Rental products for pricing pickers. They come from the common matrix
 * (every live product, `tenant.pricing.read` only), so pricing users do
 * not need the fleet permission.
 */
export function useProductOptions() {
  const query = useRatePlanMatrix({ scope: "common", priority: 0 });
  const options = useMemo<Option[]>(
    () =>
      (query.data?.rows ?? []).map((row) => ({
        value: row.rental_product.uuid,
        label: `${row.rental_product.code} · ${row.rental_product.title}`,
      })),
    [query.data],
  );
  const segments = useMemo<Option[]>(() => {
    const seen = new Map<string, string>();
    for (const row of query.data?.rows ?? []) {
      seen.set(row.segment.uuid, `${row.segment.code} · ${row.segment.name}`);
    }
    return [...seen].map(([value, label]) => ({ value, label }));
  }, [query.data]);
  return { options, segments, isLoading: query.isLoading };
}

/** Branches of the organization (needs `tenant.branches.read`). */
export function useBranchOptions() {
  const { can } = usePermission();
  const allowed = can(permissions.tenant.branches.read);
  const query = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    allowed,
  );
  const options = useMemo<Option[]>(
    () =>
      (query.data?.items ?? []).map((b) => ({
        value: b.uuid,
        label: `${b.code} · ${b.name}`,
      })),
    [query.data],
  );
  return {
    options,
    isLoading: query.isLoading,
    allowed,
  };
}

/** Vehicle segments (needs `tenant.fleet.read`; falls back to the matrix). */
export function useSegmentOptions(fallback: Option[]) {
  const { can } = usePermission();
  const allowed = can(permissions.tenant.fleet.read);
  const query = useSegmentsList(
    { limit: 100, offset: 0, sort: "code" },
    allowed,
  );
  return useMemo<Option[]>(() => {
    if (!allowed || !query.data) return fallback;
    return query.data.items.map((s) => ({
      value: s.uuid,
      label: `${s.code} · ${s.name}`,
    }));
  }, [allowed, fallback, query.data]);
}

/** Sales channels of the settings, labelled. */
export function useChannelOptions(): Option[] {
  const { t } = useLocale();
  return useMemo(
    () =>
      SALES_CHANNELS.map((code) => ({
        value: code,
        label: t(SALES_CHANNEL_LABEL_KEYS[code]),
      })),
    [t],
  );
}

export function channelLabel(
  t: (key: string) => string,
  code: string | null | undefined,
): string {
  if (!code) return "—";
  const key = SALES_CHANNEL_LABEL_KEYS[code as SalesChannelCode];
  return key ? t(key) : code;
}
