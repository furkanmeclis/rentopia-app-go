"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { siteService } from "@/features/site/services/site.service";
import type {
  SiteCatalog,
  SiteCatalogKind,
  SiteImageKind,
  SiteSaveInput,
  SiteSettings,
} from "@/features/site/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const siteKeys = {
  all: (slug: string) => ["tenant", slug, "site"] as const,
  settings: (slug: string) => [...siteKeys.all(slug), "settings"] as const,
  versions: (slug: string) => [...siteKeys.all(slug), "versions"] as const,
  preview: (slug: string) => [...siteKeys.all(slug), "preview"] as const,
  catalog: (slug: string) => [...siteKeys.all(slug), "catalog"] as const,
  domains: (slug: string) => [...siteKeys.all(slug), "domains"] as const,
};

export function useSiteSettings() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteKeys.settings(slug),
    queryFn: () => siteService.get(slug),
  });
}

/**
 * Stores the answer of a write (the new version) and refreshes what
 * depends on it (history, preview link and draft image paths).
 */
function useApplySettings() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return (settings: SiteSettings) => {
    queryClient.setQueryData(siteKeys.settings(slug), settings);
    void queryClient.invalidateQueries({ queryKey: siteKeys.versions(slug) });
    void queryClient.invalidateQueries({ queryKey: siteKeys.preview(slug) });
  };
}

/**
 * Toast for a failed site write: a stale version reloads the settings, an
 * unchanged draft is a hint, anything else names the problem.
 */
export function useSiteErrorHandler() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return (error: unknown) => {
    if (isApiError(error) && error.code === "VERSION_CONFLICT") {
      appToast.error(t("site.conflict.toast"));
      void queryClient.invalidateQueries({ queryKey: siteKeys.all(slug) });
      return;
    }
    if (isApiError(error) && error.code === "NOTHING_TO_PUBLISH") {
      appToast.info(t("site.publish.nothing"));
      return;
    }
    if (isApiError(error) && error.isValidation) {
      const first = Object.values(error.fieldErrors())[0];
      appToast.error(first || error.message);
      return;
    }
    appToast.error(
      isApiError(error) && error.message
        ? error.message
        : t("common.error_generic"),
    );
  };
}

export function useSaveSite() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const apply = useApplySettings();
  return useMutation({
    mutationFn: (body: SiteSaveInput) => siteService.save(slug, body),
    meta: { silent: true },
    onSuccess: (settings) => {
      apply(settings);
      appToast.success(t("site.toast.saved"));
    },
  });
}

export function usePublishSite() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const apply = useApplySettings();
  const onError = useSiteErrorHandler();
  return useMutation({
    mutationFn: (version: number) => siteService.publish(slug, version),
    meta: { silent: true },
    onSuccess: (settings) => {
      apply(settings);
      appToast.success(
        t("site.toast.published", {
          revision: settings.published_revision ?? "",
        }),
      );
    },
    onError,
  });
}

export function useSiteVersions() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteKeys.versions(slug),
    queryFn: () => siteService.versions(slug),
  });
}

export function useRestoreSiteVersion() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const apply = useApplySettings();
  const onError = useSiteErrorHandler();
  return useMutation({
    mutationFn: ({
      revision,
      version,
    }: {
      revision: string;
      version: number;
    }) => siteService.restore(slug, revision, version),
    meta: { silent: true },
    onSuccess: (settings) => {
      apply(settings);
      appToast.success(t("site.toast.restored"));
    },
    onError,
  });
}

/** Preview link (30 minutes); refreshed after every write. */
export function useSitePreview(enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteKeys.preview(slug),
    queryFn: () => siteService.previewToken(slug),
    enabled,
    staleTime: 20 * 60 * 1000,
    refetchInterval: 20 * 60 * 1000,
  });
}

export function useRenewSiteKey() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const apply = useApplySettings();
  const onError = useSiteErrorHandler();
  return useMutation({
    mutationFn: (version: number) => siteService.renewKey(slug, version),
    meta: { silent: true },
    onSuccess: (settings) => {
      apply(settings);
      appToast.success(t("site.toast.key_renewed"));
    },
    onError,
  });
}

export function useUploadSiteImage() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const apply = useApplySettings();
  const onError = useSiteErrorHandler();
  return useMutation({
    mutationFn: ({
      kind,
      file,
      version,
    }: {
      kind: SiteImageKind;
      file: File;
      version: number;
    }) => siteService.uploadImage(slug, kind, file, version),
    meta: { silent: true },
    onSuccess: (settings) => {
      apply(settings);
      appToast.success(t("site.toast.image_uploaded"));
    },
    onError,
  });
}

export function useRemoveSiteImage() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const apply = useApplySettings();
  const onError = useSiteErrorHandler();
  return useMutation({
    mutationFn: ({ kind, version }: { kind: SiteImageKind; version: number }) =>
      siteService.removeImage(slug, kind, version),
    meta: { silent: true },
    onSuccess: (settings) => {
      apply(settings);
      appToast.success(t("site.toast.image_removed"));
    },
    onError,
  });
}

export function useSiteCatalog() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteKeys.catalog(slug),
    queryFn: () => siteService.catalog(slug),
  });
}

/** Switches one catalog item online / offline (optimistic). */
export function useSetCatalogOnline() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const key = siteKeys.catalog(slug);
  const field = {
    branches: "branches",
    products: "subgroups",
    extras: "extras",
  } as const;
  return useMutation({
    mutationFn: ({
      kind,
      uuid,
      showOnline,
    }: {
      kind: SiteCatalogKind;
      uuid: string;
      showOnline: boolean;
    }) => siteService.setOnline(slug, kind, uuid, showOnline),
    onMutate: async ({ kind, uuid, showOnline }) => {
      await queryClient.cancelQueries({ queryKey: key });
      const previous = queryClient.getQueryData<SiteCatalog>(key);
      if (previous) {
        const list = field[kind];
        queryClient.setQueryData<SiteCatalog>(key, {
          ...previous,
          [list]: previous[list].map((row) =>
            row.uuid === uuid ? { ...row, show_online: showOnline } : row,
          ),
        });
      }
      return { previous };
    },
    onError: (_error, _vars, context) => {
      if (context?.previous) queryClient.setQueryData(key, context.previous);
    },
    onSuccess: (_row, { showOnline }) => {
      appToast.success(
        t(showOnline ? "site.toast.shown_online" : "site.toast.hidden_online"),
      );
    },
    onSettled: () => void queryClient.invalidateQueries({ queryKey: key }),
  });
}

/** Custom domains (TEC-731). Pending ones are polled while the tab is open. */
export function useSiteDomains() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteKeys.domains(slug),
    queryFn: () => siteService.domains(slug),
    refetchInterval: (query) =>
      query.state.data?.some((d) => d.status === "pending") ? 30_000 : false,
  });
}

/** Add, re-check and remove a custom domain; every change refreshes the list and the settings (hosts). */
export function useSiteDomainActions() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: siteKeys.domains(slug) });
    void queryClient.invalidateQueries({ queryKey: siteKeys.settings(slug) });
  };
  const add = useMutation({
    mutationFn: (host: string) => siteService.addDomain(slug, host),
    onSuccess: () => {
      appToast.success(t("site.domains.toast.added"));
      refresh();
    },
  });
  const verify = useMutation({
    mutationFn: (uuid: string) => siteService.verifyDomain(slug, uuid),
    onSuccess: () => {
      appToast.success(t("site.domains.toast.verify_queued"));
      refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => siteService.removeDomain(slug, uuid),
    onSuccess: () => {
      appToast.success(t("site.domains.toast.removed"));
      refresh();
    },
  });
  return { add, verify, remove };
}
