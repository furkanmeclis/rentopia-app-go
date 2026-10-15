"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { extrasService } from "@/features/extras/services/extras.service";
import type {
  EquipmentReserveRequest,
  ExtraCreateRequest,
  ExtraPatchRequest,
  ExtraPricesRequest,
  ExtraStockRequest,
  ExtrasAvailabilityRequest,
  ListAllocationsParams,
  ListExtrasParams,
} from "@/features/extras/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug: two organizations never share cache. */
export const extrasKeys = {
  all: (slug: string) => ["tenant", slug, "extras"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...extrasKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...extrasKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...extrasKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...extrasKeys.all(slug), "detail", uuid] as const,
  prices: (slug: string, uuid: string) =>
    [...extrasKeys.all(slug), "prices", uuid] as const,
  priceHistory: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...extrasKeys.all(slug), "price-history", uuid, params] as const,
  stock: (slug: string, uuid: string) =>
    [...extrasKeys.all(slug), "stock", uuid] as const,
  activity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...extrasKeys.all(slug), "activity", uuid, params] as const,
  allocations: (slug: string, params: Record<string, unknown>) =>
    [...extrasKeys.all(slug), "allocations", params] as const,
};

function useInvalidate() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: extrasKeys.all(slug) });
}

export function useExtrasList(params: ListExtrasParams, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.list(slug, params),
    queryFn: () => extrasService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useExtrasMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.meta(slug),
    queryFn: () => extrasService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useExtraFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.facets(slug),
    queryFn: () => extrasService.facets(slug),
    staleTime: 60_000,
  });
}

export function useExtra(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.detail(slug, uuid),
    queryFn: () => extrasService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useExtraPrices(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.prices(slug, uuid),
    queryFn: () => extrasService.prices(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useExtraPriceHistory(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.priceHistory(slug, uuid, params),
    queryFn: () => extrasService.priceHistory(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useExtraStock(uuid: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.stock(slug, uuid),
    queryFn: () => extrasService.stock(slug, uuid),
    enabled: Boolean(uuid) && enabled,
  });
}

export function useExtraActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.activity(slug, uuid, params),
    queryFn: () => extrasService.activity(slug, uuid, params),
  });
}

export function useEquipmentAllocations(params: ListAllocationsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: extrasKeys.allocations(slug, params),
    queryFn: () => extrasService.allocations(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCreateExtra() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: ExtraCreateRequest) => extrasService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("extras.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateExtra() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: ExtraPatchRequest }) =>
      extrasService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("extras.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteExtra() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => extrasService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("extras.toast.deleted"));
      void invalidate();
    },
  });
}

export function useSetExtraPrices() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: ExtraPricesRequest }) =>
      extrasService.setPrices(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("extras.toast.prices_saved"));
      void invalidate();
    },
  });
}

export function useSetExtraStock() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: ExtraStockRequest }) =>
      extrasService.setStock(slug, uuid, body),
    onSuccess: (result) => {
      if (result.warnings.length) {
        appToast.warning(
          t("extras.toast.stock_below", { count: result.warnings.length }),
        );
      } else {
        appToast.success(t("extras.toast.stock_saved"));
      }
      void invalidate();
    },
  });
}

export function useExtrasAvailability() {
  const { slug } = useTenant();
  return useMutation({
    mutationFn: (body: ExtrasAvailabilityRequest) =>
      extrasService.availability(slug, body),
  });
}

export function useReserveEquipment() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: EquipmentReserveRequest) =>
      extrasService.reserve(slug, body),
    onSuccess: () => {
      appToast.success(t("extras.toast.reserved"));
      void invalidate();
    },
  });
}

export function useConfirmAllocation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => extrasService.confirm(slug, uuid),
    onSuccess: () => {
      appToast.success(t("extras.toast.confirmed"));
      void invalidate();
    },
  });
}

export function useReleaseAllocation() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => extrasService.release(slug, uuid),
    onSuccess: () => {
      appToast.success(t("extras.toast.released"));
      void invalidate();
    },
  });
}
