"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { campaignsService } from "@/features/campaigns/services/campaigns.service";
import type {
  CampaignCreateRequest,
  CampaignPatchRequest,
  ListCampaignsParams,
} from "@/features/campaigns/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const campaignKeys = {
  all: (slug: string) => ["tenant", slug, "campaigns"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...campaignKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...campaignKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...campaignKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...campaignKeys.all(slug), "detail", uuid] as const,
  activity: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...campaignKeys.all(slug), "activity", uuid, params] as const,
  redemptions: (slug: string, uuid: string, params: Record<string, unknown>) =>
    [...campaignKeys.all(slug), "redemptions", uuid, params] as const,
};

function useInvalidate() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: campaignKeys.all(slug) });
}

export function useCampaignsList(params: ListCampaignsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: campaignKeys.list(slug, params),
    queryFn: () => campaignsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCampaignsMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: campaignKeys.meta(slug),
    queryFn: () => campaignsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useCampaignFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: campaignKeys.facets(slug),
    queryFn: () => campaignsService.facets(slug),
    staleTime: 60_000,
  });
}

export function useCampaign(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: campaignKeys.detail(slug, uuid),
    queryFn: () => campaignsService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useCampaignActivity(
  uuid: string,
  params: { limit: number; offset: number },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: campaignKeys.activity(slug, uuid, params),
    queryFn: () => campaignsService.activity(slug, uuid, params),
  });
}

export function useCampaignRedemptions(
  uuid: string,
  params: { limit: number; offset: number; status?: string },
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: campaignKeys.redemptions(slug, uuid, params),
    queryFn: () => campaignsService.redemptions(slug, uuid, params),
    placeholderData: (previous) => previous,
  });
}

export function useCreateCampaign() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: CampaignCreateRequest) =>
      campaignsService.create(slug, body),
    onSuccess: () => {
      appToast.success(t("campaigns.toast.created"));
      void invalidate();
    },
  });
}

export function useUpdateCampaign() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: CampaignPatchRequest;
    }) => campaignsService.update(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("campaigns.toast.updated"));
      void invalidate();
    },
  });
}

export function useDeleteCampaign() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => campaignsService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("campaigns.toast.deleted"));
      void invalidate();
    },
  });
}
