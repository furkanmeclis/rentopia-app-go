"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import {
  CHANNEL_ACCOUNTS_RESOURCE,
  CHANNEL_MESSAGES_RESOURCE,
} from "@/features/channels/constants";
import { channelsService } from "@/features/channels/services/channels.service";
import type {
  ChannelMappingInput,
  ChannelSettingsInput,
  ChannelSimulateInput,
  ListChannelMappingsParams,
  ListChannelMessagesParams,
  ListChannelSyncRunsParams,
} from "@/features/channels/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const channelKeys = {
  all: (slug: string) => ["tenant", slug, "channels"] as const,
  accounts: (slug: string) => [...channelKeys.all(slug), "accounts"] as const,
  mappings: (slug: string, params: Record<string, unknown>) =>
    [...channelKeys.all(slug), "mappings", params] as const,
  mappingFacets: (slug: string) =>
    [...channelKeys.all(slug), "mapping-facets"] as const,
  messages: (slug: string, params: Record<string, unknown>) =>
    [...channelKeys.all(slug), "messages", params] as const,
  messageFacets: (slug: string) =>
    [...channelKeys.all(slug), "message-facets"] as const,
  message: (slug: string, uuid: string) =>
    [...channelKeys.all(slug), "message", uuid] as const,
  runs: (slug: string, params: Record<string, unknown>) =>
    [...channelKeys.all(slug), "runs", params] as const,
  reconciliation: (slug: string, uuid: string, from: string, to: string) =>
    [...channelKeys.all(slug), "reconciliation", uuid, from, to] as const,
};

function useInvalidateChannels() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: channelKeys.all(slug) });
}

/** Channel accounts with their sync state, refreshed live. */
export function useChannelAccounts() {
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: channelKeys.accounts(slug),
    queryFn: () => channelsService.accounts(slug),
    enabled: Boolean(slug),
  });
  const resources = useMemo(
    () => [CHANNEL_ACCOUNTS_RESOURCE, CHANNEL_MESSAGES_RESOURCE],
    [],
  );
  useTenantLiveRefetch(resources, () =>
    query.refetch({ cancelRefetch: false }),
  );
  return query;
}

export function useChannelMappings(params: ListChannelMappingsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelKeys.mappings(slug, params),
    queryFn: () => channelsService.mappings(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug),
  });
}

export function useChannelMappingFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelKeys.mappingFacets(slug),
    queryFn: () => channelsService.mappingFacets(slug),
    staleTime: 60_000,
    enabled: Boolean(slug),
  });
}

export function useChannelMessages(params: ListChannelMessagesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelKeys.messages(slug, params),
    queryFn: () => channelsService.messages(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug),
  });
}

export function useChannelMessageFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelKeys.messageFacets(slug),
    queryFn: () => channelsService.messageFacets(slug),
    staleTime: 60_000,
    enabled: Boolean(slug),
  });
}

/**
 * Failed messages (the error queue) for the sidebar badge, refreshed live
 * from `tenant.channel_messages` and every five minutes.
 */
export function useChannelNeedsAction() {
  const { can } = usePermission();
  const allowed = can(permissions.tenant.channels.read);
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: channelKeys.messageFacets(slug),
    queryFn: () => channelsService.messageFacets(slug),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
    enabled: Boolean(slug) && allowed,
  });
  const resources = useMemo(() => [CHANNEL_MESSAGES_RESOURCE], []);
  useTenantLiveRefetch(
    resources,
    () => query.refetch({ cancelRefetch: false }),
    { enabled: allowed },
  );
  return allowed ? query.data : undefined;
}

export function useChannelMessage(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelKeys.message(slug, uuid ?? ""),
    queryFn: () => channelsService.message(slug, uuid ?? ""),
    enabled: Boolean(slug && uuid),
  });
}

export function useChannelSyncRuns(params: ListChannelSyncRunsParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelKeys.runs(slug, params),
    queryFn: () => channelsService.runs(slug, params),
    placeholderData: (previous) => previous,
    enabled: Boolean(slug),
  });
}

export function useChannelReconciliation(
  uuid: string,
  from: string,
  to: string,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: channelKeys.reconciliation(slug, uuid, from, to),
    queryFn: () => channelsService.reconciliation(slug, uuid, from, to),
    enabled: Boolean(slug && uuid && from && to),
  });
}

export function useUpdateChannelAccount() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (v: { uuid: string; body: ChannelSettingsInput }) =>
      channelsService.updateAccount(slug, v.uuid, v.body),
    onSuccess: () => {
      appToast.success(t("channels.toast.settings_saved"));
      void invalidate();
    },
  });
}

/** Push / pull / simulate: callers render the run or the error. */
export function useChannelSync() {
  const { slug } = useTenant();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (v: {
      uuid: string;
      action: "push" | "pull" | "simulate";
      body?: ChannelSimulateInput;
    }) =>
      v.action === "simulate"
        ? channelsService.simulate(slug, v.uuid, v.body ?? {})
        : v.action === "push"
          ? channelsService.push(slug, v.uuid)
          : channelsService.pull(slug, v.uuid),
    meta: { silent: true },
    onSettled: () => void invalidate(),
  });
}

export function useSaveChannelMapping() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (v: { uuid: string | null; body: ChannelMappingInput }) =>
      v.uuid
        ? channelsService.updateMapping(slug, v.uuid, v.body)
        : channelsService.createMapping(slug, v.body),
    onSuccess: (_, v) => {
      appToast.success(
        v.uuid
          ? t("channels.toast.mapping_updated")
          : t("channels.toast.mapping_created"),
      );
      void invalidate();
    },
  });
}

export function useDeleteChannelMapping() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (uuid: string) => channelsService.deleteMapping(slug, uuid),
    onSuccess: () => {
      appToast.success(t("channels.toast.mapping_deleted"));
      void invalidate();
    },
  });
}

/** Retry / dismiss a failed message: callers render the result. */
export function useResolveChannelMessage() {
  const { slug } = useTenant();
  const invalidate = useInvalidateChannels();
  return useMutation({
    mutationFn: (v: {
      uuid: string;
      action: "retry" | "ignore";
      note?: string;
    }) =>
      v.action === "retry"
        ? channelsService.retryMessage(slug, v.uuid)
        : channelsService.ignoreMessage(slug, v.uuid, v.note ?? ""),
    meta: { silent: true },
    onSettled: () => void invalidate(),
  });
}
