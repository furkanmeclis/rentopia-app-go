"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { siteArticlesService } from "@/features/site-articles/services/site-articles.service";
import type {
  ListSiteArticlesParams,
  SiteArticle,
  SiteArticleInput,
  SiteArticleUpdate,
} from "@/features/site-articles/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";

/** Query keys carry the organization slug. */
export const siteArticleKeys = {
  all: (slug: string) => ["tenant", slug, "site-articles"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...siteArticleKeys.all(slug), "list", params] as const,
  detail: (slug: string, uuid: string) =>
    [...siteArticleKeys.all(slug), "detail", uuid] as const,
};

export function useSiteArticlesList(params: ListSiteArticlesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteArticleKeys.list(slug, params),
    queryFn: () => siteArticlesService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useSiteArticle(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteArticleKeys.detail(slug, uuid ?? ""),
    queryFn: () => siteArticlesService.get(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

/** Keeps the detail cache on the server's answer and refreshes lists. */
function useStore() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return (article: SiteArticle) => {
    queryClient.setQueryData(
      siteArticleKeys.detail(slug, article.uuid),
      article,
    );
    void queryClient.invalidateQueries({
      queryKey: [...siteArticleKeys.all(slug), "list"],
    });
  };
}

export function useCreateSiteArticle() {
  const { slug } = useTenant();
  const store = useStore();
  return useMutation({
    meta: { silent: true },
    mutationFn: (body: SiteArticleInput) =>
      siteArticlesService.create(slug, body),
    onSuccess: store,
  });
}

export function useUpdateSiteArticle(uuid: string) {
  const { slug } = useTenant();
  const store = useStore();
  return useMutation({
    meta: { silent: true },
    mutationFn: (body: SiteArticleUpdate) =>
      siteArticlesService.update(slug, uuid, body),
    onSuccess: store,
  });
}

export function useSetSiteArticleStatus(uuid: string) {
  const { slug } = useTenant();
  const store = useStore();
  return useMutation({
    meta: { silent: true },
    mutationFn: ({
      publish,
      version,
    }: {
      publish: boolean;
      version: number;
    }) =>
      publish
        ? siteArticlesService.publish(slug, uuid, version)
        : siteArticlesService.unpublish(slug, uuid, version),
    onSuccess: store,
  });
}

export function useDeleteSiteArticle() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: ({ uuid, version }: { uuid: string; version: number }) =>
      siteArticlesService.remove(slug, uuid, version),
    onSuccess: (_res, { uuid }) => {
      queryClient.removeQueries({
        queryKey: siteArticleKeys.detail(slug, uuid),
      });
      void queryClient.invalidateQueries({
        queryKey: [...siteArticleKeys.all(slug), "list"],
      });
    },
  });
}
