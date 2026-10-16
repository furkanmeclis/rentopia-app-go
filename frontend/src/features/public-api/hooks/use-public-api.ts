"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { publicApiService } from "@/features/public-api/services/public-api.service";
import type {
  ListDeliveriesParams,
  ListKeysParams,
  ListWebhooksParams,
  PublicApiKeyCreateRequest,
  PublicApiKeyPatchRequest,
  PublicWebhookCreateRequest,
  PublicWebhookPatchRequest,
} from "@/features/public-api/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys carry the organization slug. */
export const publicApiKeys = {
  all: (slug: string) => ["tenant", slug, "public-api"] as const,
  overview: (slug: string) => [...publicApiKeys.all(slug), "overview"] as const,
  keys: (slug: string, params: Record<string, unknown>) =>
    [...publicApiKeys.all(slug), "keys", params] as const,
  webhooks: (slug: string, params: Record<string, unknown>) =>
    [...publicApiKeys.all(slug), "webhooks", params] as const,
  deliveries: (slug: string, params: Record<string, unknown>) =>
    [...publicApiKeys.all(slug), "deliveries", params] as const,
  delivery: (slug: string, uuid: string) =>
    [...publicApiKeys.all(slug), "delivery", uuid] as const,
};

function useInvalidate() {
  const { slug } = useTenant();
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({ queryKey: publicApiKeys.all(slug) });
}

export function usePublicApiOverview() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: publicApiKeys.overview(slug),
    queryFn: () => publicApiService.overview(slug),
  });
}

export function usePublicApiKeys(params: ListKeysParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: publicApiKeys.keys(slug, params),
    queryFn: () => publicApiService.keys(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCreatePublicApiKey() {
  const { slug } = useTenant();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: PublicApiKeyCreateRequest) =>
      publicApiService.createKey(slug, body),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function useUpdatePublicApiKey() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: PublicApiKeyPatchRequest;
    }) => publicApiService.patchKey(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("public_api.toast.key_updated"));
      void invalidate();
    },
  });
}

export function useRotatePublicApiKey() {
  const { slug } = useTenant();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ uuid, graceHours }: { uuid: string; graceHours: number }) =>
      publicApiService.rotateKey(slug, uuid, graceHours),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function useRevokePublicApiKey() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => publicApiService.revokeKey(slug, uuid),
    onSuccess: () => {
      appToast.success(t("public_api.toast.key_revoked"));
      void invalidate();
    },
  });
}

export function usePublicWebhooks(params: ListWebhooksParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: publicApiKeys.webhooks(slug, params),
    queryFn: () => publicApiService.webhooks(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useCreatePublicWebhook() {
  const { slug } = useTenant();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: PublicWebhookCreateRequest) =>
      publicApiService.createWebhook(slug, body),
    meta: { silent: true },
    onSuccess: () => void invalidate(),
  });
}

export function useUpdatePublicWebhook() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid: string;
      body: PublicWebhookPatchRequest;
    }) => publicApiService.patchWebhook(slug, uuid, body),
    meta: { silent: true },
    onSuccess: () => {
      appToast.success(t("public_api.toast.webhook_updated"));
      void invalidate();
    },
  });
}

export function useDeletePublicWebhook() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => publicApiService.deleteWebhook(slug, uuid),
    onSuccess: () => {
      appToast.success(t("public_api.toast.webhook_deleted"));
      void invalidate();
    },
  });
}

export function useRotatePublicWebhookSecret() {
  const { slug } = useTenant();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) =>
      publicApiService.rotateWebhookSecret(slug, uuid),
    onSuccess: () => void invalidate(),
  });
}

export function useTestPublicWebhook() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => publicApiService.testWebhook(slug, uuid),
    onSuccess: (delivery) => {
      if (delivery.status === "succeeded") {
        appToast.success(t("public_api.toast.test_ok"));
      } else {
        appToast.error(
          t("public_api.toast.test_failed", {
            error: delivery.last_error || String(delivery.last_status_code),
          }),
        );
      }
      void invalidate();
    },
  });
}

export function usePublicWebhookDeliveries(params: ListDeliveriesParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: publicApiKeys.deliveries(slug, params),
    queryFn: () => publicApiService.deliveries(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function usePublicWebhookDelivery(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: publicApiKeys.delivery(slug, uuid ?? ""),
    queryFn: () => publicApiService.delivery(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

export function useRetryPublicWebhookDelivery() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (uuid: string) => publicApiService.retryDelivery(slug, uuid),
    onSuccess: (delivery) => {
      appToast.success(
        t(
          delivery.status === "succeeded"
            ? "public_api.toast.retry_ok"
            : "public_api.toast.retry_queued",
        ),
      );
      void invalidate();
    },
  });
}
