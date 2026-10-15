"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";

import { rentalKeys } from "@/features/rentals/hooks/use-rentals";
import {
  type QueueItem,
  deleteItem,
  listItems,
  newClientRef,
  putItem,
} from "@/features/inspections/lib/offline-store";
import { inspectionsService } from "@/features/inspections/services/inspections.service";
import type {
  Inspection,
  InspectionDamageRequest,
  InspectionDraftRequest,
  InspectionKind,
  InspectionPhotoRequest,
  InspectionReviewRequest,
  InspectionTemplateRequest,
  ListInspectionsParams,
} from "@/features/inspections/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const inspectionKeys = {
  all: (slug: string) => ["tenant", slug, "inspections"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...inspectionKeys.all(slug), "list", params] as const,
  facets: (slug: string) => [...inspectionKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...inspectionKeys.all(slug), "detail", uuid] as const,
  charges: (slug: string, uuid: string) =>
    [...inspectionKeys.all(slug), "charges", uuid] as const,
  media: (slug: string, uuid: string) =>
    [...inspectionKeys.all(slug), "media", uuid] as const,
  templates: (slug: string, params: Record<string, unknown>) =>
    [...inspectionKeys.all(slug), "templates", params] as const,
};

export function useInspectionsList(params: ListInspectionsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: inspectionKeys.list(slug, params),
    queryFn: () => inspectionsService.list(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useInspectionFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: inspectionKeys.facets(slug),
    queryFn: () => inspectionsService.facets(slug),
  });
}

export function useInspection(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: inspectionKeys.detail(slug, uuid ?? ""),
    queryFn: () => inspectionsService.get(slug, uuid as string),
    enabled: Boolean(uuid),
  });
}

/** Starts or resumes the form of a rental (POST is idempotent). */
export function useStartInspection(rentalUuid: string, kind: InspectionKind) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...inspectionKeys.all(slug), "start", rentalUuid, kind],
    queryFn: () => inspectionsService.start(slug, rentalUuid, kind),
    retry: false,
    staleTime: Infinity,
  });
}

export function useInspectionCharges(uuid: string, enabled: boolean) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: inspectionKeys.charges(slug, uuid),
    queryFn: () => inspectionsService.charges(slug, uuid),
    enabled,
  });
}

/** Short-lived media links (refreshed before they expire). */
export function useMediaLinks(uuid: string | null, version: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...inspectionKeys.media(slug, uuid ?? ""), version],
    queryFn: async () => {
      const out = await inspectionsService.mediaLinks(slug, uuid as string);
      const map: Record<string, string> = {};
      for (const item of out.items) map[item.uuid] = item.url;
      return map;
    },
    enabled: Boolean(uuid),
    staleTime: 5 * 60_000,
    refetchInterval: 8 * 60_000,
  });
}

/** Writes the fresh form into the caches the wizard reads. */
function useStoreInspection() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return useCallback(
    (inspection: Inspection) => {
      queryClient.setQueryData(
        inspectionKeys.detail(slug, inspection.uuid),
        inspection,
      );
      void queryClient.invalidateQueries({
        queryKey: inspectionKeys.charges(slug, inspection.uuid),
      });
    },
    [queryClient, slug],
  );
}

export function useInvalidateInspections() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: inspectionKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: rentalKeys.all(slug) }),
    ]);
}

export function useUpdateDraft(uuid: string) {
  const { slug } = useTenant();
  const store = useStoreInspection();
  return useMutation({
    mutationFn: (body: InspectionDraftRequest) =>
      inspectionsService.update(slug, uuid, body),
    onSuccess: store,
  });
}

export function useDamageActions(uuid: string) {
  const { slug } = useTenant();
  const store = useStoreInspection();
  const add = useMutation({
    mutationFn: (body: InspectionDamageRequest) =>
      inspectionsService.addDamage(slug, uuid, body),
    onSuccess: store,
  });
  const update = useMutation({
    mutationFn: ({
      damage,
      body,
    }: {
      damage: string;
      body: InspectionDamageRequest;
    }) => inspectionsService.updateDamage(slug, uuid, damage, body),
    onSuccess: store,
  });
  const remove = useMutation({
    mutationFn: (damage: string) =>
      inspectionsService.deleteDamage(slug, uuid, damage),
    onSuccess: store,
  });
  return { add, update, remove };
}

export function useDeletePhoto(uuid: string) {
  const { slug } = useTenant();
  const store = useStoreInspection();
  return useMutation({
    mutationFn: (photo: string) =>
      inspectionsService.deletePhoto(slug, uuid, photo),
    onSuccess: store,
  });
}

export function useSaveSignature(uuid: string) {
  const { slug } = useTenant();
  const store = useStoreInspection();
  return useMutation({
    mutationFn: (body: Parameters<typeof inspectionsService.signature>[2]) =>
      inspectionsService.signature(slug, uuid, body),
    onSuccess: store,
  });
}

export function useCompleteInspection(uuid: string) {
  const { slug } = useTenant();
  const store = useStoreInspection();
  const invalidate = useInvalidateInspections();
  return useMutation({
    mutationFn: (body: Parameters<typeof inspectionsService.complete>[2]) =>
      inspectionsService.complete(slug, uuid, body),
    onSuccess: (inspection) => {
      store(inspection);
      void invalidate();
    },
  });
}

export function useReviseInspection(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateInspections();
  return useMutation({
    mutationFn: (reason: string) =>
      inspectionsService.revise(slug, uuid, reason),
    onSuccess: () => {
      appToast.success(t("inspections.toast.revised"));
      void invalidate();
    },
  });
}

export function useReviewInspection(uuid: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateInspections();
  return useMutation({
    mutationFn: (body: InspectionReviewRequest) =>
      inspectionsService.review(slug, uuid, body),
    onSuccess: () => {
      appToast.success(t("inspections.toast.reviewed"));
      void invalidate();
    },
  });
}

export function useInspectionTemplates(params: {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
}) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: inspectionKeys.templates(slug, params),
    queryFn: () => inspectionsService.templates(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useSaveTemplate() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: InspectionTemplateRequest) =>
      inspectionsService.saveTemplate(slug, body),
    onSuccess: () => {
      appToast.success(t("inspections.toast.template_saved"));
      void queryClient.invalidateQueries({
        queryKey: inspectionKeys.all(slug),
      });
    },
  });
}

// ---------------------------------------------------------- upload queue

export type QueueView = Omit<QueueItem, "blob"> & { previewUrl?: string };

function errorText(error: unknown) {
  if (error instanceof ApiError) {
    return error.details[0]?.code ?? error.code;
  }
  return error instanceof Error ? error.message : String(error);
}

/**
 * Device upload queue of a form. A photo is stored on the device first
 * (IndexedDB), then announced (idempotent `client_ref`), uploaded to the
 * API-issued address and completed (server hash). Failed or offline items
 * stay queued and retry when the connection returns.
 */
export function useUploadQueue(inspectionUuid: string | null) {
  const { slug } = useTenant();
  const store = useStoreInspection();
  const queryClient = useQueryClient();
  const [items, setItems] = useState<QueueView[]>([]);
  const running = useRef(false);
  const previews = useRef(new Map<string, string>());

  const refresh = useCallback(async () => {
    if (!inspectionUuid) return;
    const list = await listItems(inspectionUuid);
    setItems(
      list.map(({ blob, ...rest }) => {
        let url = previews.current.get(rest.id);
        if (!url) {
          try {
            url = URL.createObjectURL(blob);
            previews.current.set(rest.id, url);
          } catch {
            url = undefined;
          }
        }
        return { ...rest, previewUrl: url };
      }),
    );
  }, [inspectionUuid]);

  const process = useCallback(async () => {
    if (!inspectionUuid || running.current) return;
    if (typeof navigator !== "undefined" && navigator.onLine === false) return;
    running.current = true;
    try {
      const list = await listItems(inspectionUuid);
      for (const item of list) {
        if (item.status === "done") continue;
        const current: QueueItem = {
          ...item,
          status: "uploading",
          error: undefined,
        };
        await putItem(current);
        await refresh();
        try {
          if (!current.photoUuid || !current.uploadPath) {
            const announced = await inspectionsService.addPhoto(
              slug,
              inspectionUuid,
              {
                slot: current.slot,
                damage_uuid: current.damageUuid,
                client_ref: current.id,
                content_type:
                  current.type as InspectionPhotoRequest["content_type"],
                size_bytes: current.size,
                device_captured_at: current.capturedAt,
              },
            );
            current.photoUuid = announced.photo.uuid;
            current.uploadPath = announced.upload.path;
            await putItem(current);
            if (announced.photo.status === "completed") {
              current.status = "done";
              await putItem(current);
              continue;
            }
          }
          await inspectionsService.uploadPhoto(
            slug,
            current.uploadPath as string,
            current.blob,
            current.name,
          );
          current.status = "completing";
          await putItem(current);
          await refresh();
          await inspectionsService.completePhoto(
            slug,
            inspectionUuid,
            current.photoUuid as string,
          );
          current.status = "done";
          await putItem(current);
        } catch (error) {
          current.status = "failed";
          current.error = errorText(error);
          await putItem(current);
        }
        await refresh();
      }
      const fresh = await inspectionsService
        .get(slug, inspectionUuid)
        .catch(() => null);
      if (fresh) store(fresh);
      void queryClient.invalidateQueries({
        queryKey: inspectionKeys.media(slug, inspectionUuid),
      });
    } finally {
      running.current = false;
      await refresh();
    }
  }, [inspectionUuid, queryClient, refresh, slug, store]);

  const add = useCallback(
    async (file: File, slot: string, damageUuid?: string) => {
      if (!inspectionUuid) return;
      const item: QueueItem = {
        id: newClientRef(),
        slug,
        inspectionUuid,
        slot,
        damageUuid,
        name: file.name || `${slot}.jpg`,
        type: file.type || "image/jpeg",
        size: file.size,
        capturedAt: file.lastModified
          ? new Date(file.lastModified).toISOString()
          : undefined,
        blob: file,
        status: "queued",
        createdAt: Date.now(),
      };
      await putItem(item);
      await refresh();
      void process();
    },
    [inspectionUuid, process, refresh, slug],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteItem(id);
      const url = previews.current.get(id);
      if (url) {
        URL.revokeObjectURL(url);
        previews.current.delete(id);
      }
      await refresh();
    },
    [refresh],
  );

  /** Forgets finished items (their photos are on the server). */
  const clearDone = useCallback(async () => {
    for (const item of items) {
      if (item.status === "done") await remove(item.id);
    }
  }, [items, remove]);

  useEffect(() => {
    void refresh().then(() => process());
    const online = () => void process();
    window.addEventListener("online", online);
    const urls = previews.current;
    return () => {
      window.removeEventListener("online", online);
      for (const url of urls.values()) URL.revokeObjectURL(url);
      urls.clear();
    };
  }, [process, refresh]);

  return { items, add, retry: process, remove, clearDone };
}
