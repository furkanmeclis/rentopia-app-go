"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useSyncExternalStore } from "react";

import { siteKeys, useSitePreview } from "@/features/site/hooks/use-site";
import { MEDIA_RULES } from "@/features/site/lib/site-options";
import { siteService } from "@/features/site/services/site.service";
import type { SiteMedia, SiteMediaList } from "@/features/site/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError } from "@/lib/api";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const siteMediaKeys = {
  list: (slug: string) => [...siteKeys.all(slug), "media"] as const,
  urls: (slug: string, token: string | undefined) =>
    [...siteKeys.all(slug), "media-urls", token ?? ""] as const,
};

/*
 * Local previews of files uploaded in this browser session (object URLs of
 * the picked File, never a storage URL). The API has no tenant stream for
 * library items, so until an item is used by the draft this is the only
 * thumbnail the panel can show.
 */
const localPreviews = new Map<string, string>();
const listeners = new Set<() => void>();
let localVersion = 0;

function rememberLocalPreview(uuid: string, file: File) {
  if (typeof URL === "undefined" || !URL.createObjectURL) return;
  localPreviews.set(uuid, URL.createObjectURL(file));
  localVersion += 1;
  listeners.forEach((l) => l());
}

function forgetLocalPreview(uuid: string) {
  const url = localPreviews.get(uuid);
  if (!url) return;
  URL.revokeObjectURL(url);
  localPreviews.delete(uuid);
  localVersion += 1;
  listeners.forEach((l) => l());
}

function useLocalPreviewVersion() {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => localVersion,
    () => 0,
  );
}

function hostOf(url: string | undefined) {
  if (!url) return undefined;
  try {
    return new URL(url).host;
  } catch {
    return undefined;
  }
}

export function useSiteMedia() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: siteMediaKeys.list(slug),
    queryFn: () => siteService.media(slug),
  });
}

/**
 * uuid → image src for thumbnails: the draft's API-owned media stream URLs
 * (resolve with the preview token), else a local preview of a file
 * uploaded in this session. Missing entries render a placeholder.
 */
export function useSiteMediaUrls() {
  const { slug } = useTenant();
  const preview = useSitePreview(true);
  const token = preview.data?.token;
  const host = hostOf(preview.data?.url);
  const resolved = useQuery({
    queryKey: siteMediaKeys.urls(slug, token),
    queryFn: () => siteService.resolvePreview(host!, token!),
    enabled: Boolean(host && token),
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
  const version = useLocalPreviewVersion();
  return useMemo(() => {
    const map: Record<string, string> = {};
    for (const [uuid, url] of localPreviews) map[uuid] = url;
    for (const [uuid, url] of Object.entries(resolved.data?.media ?? {})) {
      try {
        const src = resolveLogoSrc(assertServiceMediaURL(url));
        if (src) map[uuid] = src;
      } catch {
        // Not an API-owned URL: never rendered.
      }
    }
    return map;
    // `version` re-reads the local previews after an upload.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resolved.data, version]);
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
          rememberLocalPreview(item.uuid, file);
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
    onSuccess: (_res, uuid) => {
      forgetLocalPreview(uuid);
      appToast.success(t("site.media.toast.deleted"));
    },
    onError: (error) => appToast.error(mediaErrorMessage(error, t)),
    onSettled: () =>
      void queryClient.invalidateQueries({
        queryKey: siteMediaKeys.list(slug),
      }),
  });
}
