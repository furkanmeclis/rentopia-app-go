"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";

import { siteKeys } from "@/features/site/hooks/use-site";
import { MEDIA_RULES } from "@/features/site/lib/site-options";
import { siteService } from "@/features/site/services/site.service";
import type { SiteMedia, SiteMediaList } from "@/features/site/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError } from "@/lib/api";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const siteMediaKeys = {
  list: (slug: string) => [...siteKeys.all(slug), "media"] as const,
};

export function useSiteMedia() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteMediaKeys.list(slug),
    queryFn: () => siteService.media(slug),
  });
}

/**
 * uuid → image src for thumbnails: every library item carries `url`, the
 * API-owned same-origin file route (`/api/v1/tenant/site/media/{uuid}/file`,
 * TEC-785), so no draft preview or local object URL is needed.
 */
export function useSiteMediaUrls() {
  const media = useSiteMedia();
  return useMemo(() => {
    const map: Record<string, string> = {};
    for (const item of media.data?.items ?? []) {
      try {
        const src = assertServiceMediaURL(item.url);
        if (src) map[item.uuid] = src;
      } catch {
        // Not an API-owned URL: never rendered.
      }
    }
    return map;
  }, [media.data]);
}

/** Client-side precheck; the server stays authoritative (type is sniffed). */
export function precheckMediaFile(
  file: File,
): "invalid_type" | "too_large" | null {
  if (!(MEDIA_RULES.accept as readonly string[]).includes(file.type)) {
    return "invalid_type";
  }
  if (file.size > MEDIA_RULES.maxBytes) return "too_large";
  return null;
}

export function mediaErrorMessage(
  error: unknown,
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  if (isApiError(error)) {
    if (error.code === "MEDIA_IN_USE") return t("site.media.errors.in_use");
    if (error.code === "MEDIA_QUOTA_EXCEEDED") {
      return t("site.media.errors.quota");
    }
    if (error.isValidation) {
      const first = Object.values(error.fieldErrors())[0];
      return first || error.message;
    }
    if (error.message) return error.message;
  }
  return t("common.error_generic");
}

/** Uploads several files one by one; returns the created items. */
export function useUploadSiteMedia() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: async (files: File[]) => {
      const list = queryClient.getQueryData<SiteMediaList>(
        siteMediaKeys.list(slug),
      );
      let count = list?.total ?? 0;
      let bytes = (list?.items ?? []).reduce((sum, m) => sum + m.size, 0);
      const created: SiteMedia[] = [];
      for (const file of files) {
        const problem = precheckMediaFile(file);
        if (problem) {
          appToast.error(
            t(`site.media.errors.${problem}`, {
              name: file.name,
              size: MEDIA_RULES.maxBytes / 1024 / 1024,
            }),
          );
          continue;
        }
        if (
          count + 1 > MEDIA_RULES.maxFiles ||
          bytes + file.size > MEDIA_RULES.maxTotalBytes
        ) {
          appToast.error(t("site.media.errors.quota"));
          break;
        }
        try {
          const item = await siteService.uploadMedia(slug, file);
          created.push(item);
          count += 1;
          bytes += item.size;
        } catch (error) {
          appToast.error(
            t("site.media.errors.upload_failed", {
              name: file.name,
              reason: mediaErrorMessage(error, t),
            }),
          );
          if (isApiError(error) && error.code === "MEDIA_QUOTA_EXCEEDED") {
            break;
          }
        }
      }
      return created;
    },
    onSuccess: (created) => {
      if (created.length) {
        appToast.success(
          t("site.media.toast.uploaded", { count: created.length }),
        );
      }
    },
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: siteMediaKeys.list(slug),
      }),
  });
}

export function usePatchSiteMedia() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: ({
      uuid,
      alt,
    }: {
      uuid: string;
      alt: { tr: string; en: string };
    }) => siteService.patchMedia(slug, uuid, alt),
    onSuccess: (item) => {
      queryClient.setQueryData<SiteMediaList>(
        siteMediaKeys.list(slug),
        (prev) =>
          prev
            ? {
                ...prev,
                items: prev.items.map((m) => (m.uuid === item.uuid ? item : m)),
              }
            : prev,
      );
      appToast.success(t("site.media.toast.alt_saved"));
    },
    onError: (error) => appToast.error(mediaErrorMessage(error, t)),
  });
}

export function useDeleteSiteMedia() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (uuid: string) => siteService.deleteMedia(slug, uuid),
    onSuccess: () => {
      appToast.success(t("site.media.toast.deleted"));
    },
    onError: (error) => appToast.error(mediaErrorMessage(error, t)),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: siteMediaKeys.list(slug),
      }),
  });
}
