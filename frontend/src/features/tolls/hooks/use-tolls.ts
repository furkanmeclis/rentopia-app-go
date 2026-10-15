"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { rentalKeys } from "@/features/rentals/hooks/use-rentals";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { TOLLS_RESOURCE } from "@/features/tolls/constants";
import { tollsService } from "@/features/tolls/services/tolls.service";
import type {
  ListTollPassesParams,
  TollSettingsPatch,
  TollTagCreate,
  TollTagPatch,
} from "@/features/tolls/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const tollKeys = {
  all: (slug: string) => ["tenant", slug, "tolls"] as const,
  list: (slug: string, params: Record<string, unknown>) =>
    [...tollKeys.all(slug), "list", params] as const,
  meta: (slug: string) => [...tollKeys.all(slug), "meta"] as const,
  facets: (slug: string) => [...tollKeys.all(slug), "facets"] as const,
  detail: (slug: string, uuid: string) =>
    [...tollKeys.all(slug), "detail", uuid] as const,
  candidates: (slug: string, uuid: string) =>
    [...tollKeys.all(slug), "candidates", uuid] as const,
  settings: (slug: string) => [...tollKeys.all(slug), "settings"] as const,
  tags: (slug: string, vehicle: string) =>
    [...tollKeys.all(slug), "tags", vehicle] as const,
};

/** A decision moves charges on rentals: refresh passes and rentals. */
function useInvalidateTolls() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: tollKeys.all(slug) }),
      queryClient.invalidateQueries({ queryKey: rentalKeys.all(slug) }),
    ]);
}

export function useTollPassesList(
  params: ListTollPassesParams,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tollKeys.list(slug, params),
    queryFn: () => tollsService.list(slug, params),
    placeholderData: (previous) => previous,
    enabled,
  });
}

export function useTollPassesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tollKeys.meta(slug),
    queryFn: () => tollsService.meta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useTollFacets(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tollKeys.facets(slug),
    queryFn: () => tollsService.facets(slug),
    staleTime: 60_000,
    enabled: Boolean(slug) && enabled,
  });
}

/**
 * Review queue count for the sidebar badge: facets, refreshed live from
 * `tenant.toll_passes` and every five minutes.
 */
export function useTollReviewCount() {
  const { can } = usePermission();
  const allowed = can(permissions.tenant.tolls.read);
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: tollKeys.facets(slug),
    queryFn: () => tollsService.facets(slug),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    enabled: Boolean(slug) && allowed,
  });
  const resources = useMemo(() => [TOLLS_RESOURCE], []);
  useTenantLiveRefetch(
    resources,
    () => query.refetch({ cancelRefetch: false }),
    { enabled: allowed },
  );
  return allowed ? query.data : undefined;
}

export function useTollPass(uuid: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tollKeys.detail(slug, uuid),
    queryFn: () => tollsService.get(slug, uuid),
    enabled: Boolean(uuid),
  });
}

export function useTollCandidates(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tollKeys.candidates(slug, uuid ?? ""),
    queryFn: () => tollsService.candidates(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

type Decision =
  | { action: "assign"; uuid: string; rental_uuid: string; reason: string }
  | { action: "unassign" | "ignore" | "rematch"; uuid: string; reason: string };

const DECISION_TOASTS: Record<Decision["action"], string> = {
  assign: "tolls.toast.assigned",
  unassign: "tolls.toast.unassigned",
  ignore: "tolls.toast.ignored",
  rematch: "tolls.toast.rematched",
};

/** Assign / unassign / ignore / rematch (dialogs render the errors). */
export function useTollDecision() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateTolls();
  return useMutation({
    mutationFn: (d: Decision) => {
      switch (d.action) {
        case "assign":
          return tollsService.assign(slug, d.uuid, {
            rental_uuid: d.rental_uuid,
            reason: d.reason,
          });
        case "unassign":
          return tollsService.unassign(slug, d.uuid, d.reason);
        case "ignore":
          return tollsService.ignore(slug, d.uuid, d.reason);
        default:
          return tollsService.rematch(slug, d.uuid, d.reason);
      }
    },
    meta: { silent: true },
    onSuccess: (_data, d) => {
      appToast.success(t(DECISION_TOASTS[d.action]));
      void invalidate();
    },
  });
}

export function useFetchTollsNow() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateTolls();
  return useMutation({
    mutationFn: () => tollsService.fetchNow(slug),
    meta: { silent: true },
    onSuccess: (job) => {
      appToast.success(
        job.status === "succeeded"
          ? t("tolls.toast.fetched")
          : t("tolls.toast.fetch_queued"),
      );
      void invalidate();
    },
  });
}

export function useTollSettings(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tollKeys.settings(slug),
    queryFn: () => tollsService.settings(slug),
    enabled: Boolean(slug) && enabled,
  });
}

export function useUpdateTollSettings() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: TollSettingsPatch) =>
      tollsService.updateSettings(slug, body),
    meta: { silent: true },
    onSuccess: (data) => {
      queryClient.setQueryData(tollKeys.settings(slug), data);
      appToast.success(t("tolls.toast.settings_saved"));
    },
  });
}

export function useVehicleTollTags(vehicle: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: tollKeys.tags(slug, vehicle),
    queryFn: () => tollsService.tags(slug, vehicle),
    enabled: Boolean(vehicle) && enabled,
  });
}

type TagMutation =
  | { action: "create"; body: TollTagCreate }
  | { action: "update"; tag: string; body: TollTagPatch }
  | { action: "delete"; tag: string };

export function useTollTagMutation(vehicle: string) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateTolls();
  return useMutation({
    mutationFn: async (m: TagMutation): Promise<unknown> => {
      switch (m.action) {
        case "create":
          return tollsService.createTag(slug, vehicle, m.body);
        case "update":
          return tollsService.updateTag(slug, vehicle, m.tag, m.body);
        default:
          return tollsService.deleteTag(slug, vehicle, m.tag);
      }
    },
    meta: { silent: true },
    onSuccess: (_data, m) => {
      appToast.success(
        t(
          m.action === "create"
            ? "tolls.toast.tag_added"
            : m.action === "update"
              ? "tolls.toast.tag_updated"
              : "tolls.toast.tag_deleted",
        ),
      );
      void invalidate();
    },
  });
}
