"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { customerRequestsService } from "@/features/customer-portal/services/portal.service";
import type {
  CustomerRequestResolve,
  ListCustomerRequestsParams,
} from "@/features/customer-portal/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const customerRequestKeys = {
  all: (slug: string) => ["tenant", slug, "customer-requests"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...customerRequestKeys.all(slug), "list", params] as const,
  stats: (slug: string) => [...customerRequestKeys.all(slug), "stats"] as const,
};

export function useCustomerRequests(
  params: ListCustomerRequestsParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerRequestKeys.list(slug, params),
    queryFn: () => customerRequestsService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useCustomerRequestStats(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: customerRequestKeys.stats(slug),
    queryFn: () => customerRequestsService.stats(slug),
    staleTime: 30_000,
    refetchInterval: 120_000,
    enabled: enabled && Boolean(slug),
  });
}

export function useResolveCustomerRequest() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: CustomerRequestResolve;
    }) => customerRequestsService.resolve(slug, uuid, body),
    onSuccess: (out) => {
      appToast.success(
        t(
          out.status === "rejected"
            ? "customer_portal.staff.toast.rejected"
            : "customer_portal.staff.toast.resolved",
        ),
      );
      void client.invalidateQueries({
        queryKey: customerRequestKeys.all(slug),
      });
      void client.invalidateQueries({
        queryKey: ["tenant", slug, "reservations"],
      });
    },
  });
}
