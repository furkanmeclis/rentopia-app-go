"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { supportAccessService } from "@/features/support-access/services/support-access.service";
import type {
  ListSupportGrantsParams,
  SupportGrantOpenRequest,
} from "@/features/support-access/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const supportAccessKeys = {
  all: (slug: string) => ["tenant", slug, "support-access"] as const,
  list: (slug: string, params: ListSupportGrantsParams) =>
    [...supportAccessKeys.all(slug), "list", params] as const,
};

function useInvalidate() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: supportAccessKeys.all(slug) });
}

export function useSupportGrants(params: ListSupportGrantsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: supportAccessKeys.list(slug, params),
    queryFn: () => supportAccessService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useOpenSupportGrant() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: SupportGrantOpenRequest) =>
      supportAccessService.open(slug, body),
    onSuccess: () => {
      appToast.success(t("support_access.toast.opened"));
      void invalidate();
    },
  });
}

export function useRevokeSupportGrant() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => supportAccessService.revoke(slug, uuid),
    onSuccess: () => {
      appToast.success(t("support_access.toast.revoked"));
      void invalidate();
    },
  });
}
