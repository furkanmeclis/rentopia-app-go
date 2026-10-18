"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { permissions } from "@/config/permissions";
import { siteLeadsService } from "@/features/site-leads/services/site-leads.service";
import type {
  ListSiteLeadsParams,
  SiteLeadPatch,
} from "@/features/site-leads/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const siteLeadKeys = {
  all: (slug: string) => ["tenant", slug, "site-leads"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...siteLeadKeys.all(slug), "list", params] as const,
  detail: (slug: string, uuid: string) =>
    [...siteLeadKeys.all(slug), "detail", uuid] as const,
  newCount: (slug: string) => [...siteLeadKeys.all(slug), "new-count"] as const,
};

export function useSiteLeadsList(params: ListSiteLeadsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteLeadKeys.list(slug, params),
    queryFn: () => siteLeadsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useSiteLead(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteLeadKeys.detail(slug, uuid ?? ""),
    queryFn: () => siteLeadsService.get(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

/** Unhandled (`new`) leads for the sidebar badge: one row, the total. */
export function useNewSiteLeadsCount() {
  const { slug } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.siteLeads.read);
  const query = useQuery({
    queryKey: siteLeadKeys.newCount(slug),
    queryFn: () =>
      siteLeadsService.list(slug, { limit: 1, offset: 0, status: "new" }),
    enabled: Boolean(slug) && allowed,
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
  });
  return allowed ? (query.data?.total ?? null) : null;
}

export function useUpdateSiteLead() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: SiteLeadPatch }) =>
      siteLeadsService.update(slug, uuid, body),
    onSuccess: (lead) => {
      queryClient.setQueryData(siteLeadKeys.detail(slug, lead.uuid), lead);
      void queryClient.invalidateQueries({ queryKey: siteLeadKeys.all(slug) });
      appToast.success(t("site_leads.toast.updated"));
    },
  });
}
