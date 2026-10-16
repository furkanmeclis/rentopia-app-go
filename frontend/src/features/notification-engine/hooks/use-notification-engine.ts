"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { notificationEngineService } from "@/features/notification-engine/services/notification-engine.service";
import type {
  DeliveryListParams,
  NotificationRuleInput,
  NotificationTemplateInput,
  NotificationTemplateKey,
  NotificationTestInput,
} from "@/features/notification-engine/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys; every key starts with the organization slug. */
export const notificationEngineKeys = {
  all: (slug: string) => ["notification-engine", slug] as const,
  events: (slug: string) => ["notification-engine", slug, "events"] as const,
  templates: (slug: string) =>
    ["notification-engine", slug, "templates"] as const,
  meta: (slug: string) => ["notification-engine", slug, "meta"] as const,
  facets: (slug: string) => ["notification-engine", slug, "facets"] as const,
  deliveries: (slug: string, p: DeliveryListParams) =>
    ["notification-engine", slug, "deliveries", p] as const,
  dispatch: (slug: string, uuid: string) =>
    ["notification-engine", slug, "dispatch", uuid] as const,
};

export function useNotificationEvents() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: notificationEngineKeys.events(slug),
    queryFn: () => notificationEngineService.events(slug),
  });
}

export function useNotificationTemplates() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: notificationEngineKeys.templates(slug),
    queryFn: () => notificationEngineService.templates(slug),
  });
}

export function useDeliveriesMeta() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: notificationEngineKeys.meta(slug),
    queryFn: () => notificationEngineService.deliveriesMeta(slug),
    staleTime: 5 * 60_000,
  });
}

export function useDeliveryFacets() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: notificationEngineKeys.facets(slug),
    queryFn: () => notificationEngineService.deliveryFacets(slug),
    staleTime: 60_000,
  });
}

export function useDeliveries(params: DeliveryListParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: notificationEngineKeys.deliveries(slug, params),
    queryFn: () => notificationEngineService.deliveries(slug, params),
    placeholderData: (previous) => previous,
  });
}

export function useDispatch(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: notificationEngineKeys.dispatch(slug, uuid ?? ""),
    queryFn: () => notificationEngineService.dispatch(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

function useInvalidate() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return () =>
    qc.invalidateQueries({ queryKey: notificationEngineKeys.all(slug) });
}

export function useSaveRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: NotificationRuleInput) =>
      notificationEngineService.putRule(slug, body),
    onSuccess: () => {
      appToast.success(t("notification_engine.toasts.rule_saved"));
      void invalidate();
    },
  });
}

export function useResetRule() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (event: string) =>
      notificationEngineService.resetRule(slug, event),
    onSuccess: () => {
      appToast.success(t("notification_engine.toasts.rule_reset"));
      void invalidate();
    },
  });
}

export function useSaveTemplate() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: NotificationTemplateInput) =>
      notificationEngineService.putTemplate(slug, body),
    onSuccess: () => {
      appToast.success(t("notification_engine.toasts.template_saved"));
      void invalidate();
    },
  });
}

export function useResetTemplate() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: NotificationTemplateKey) =>
      notificationEngineService.resetTemplate(slug, body),
    onSuccess: () => {
      appToast.success(t("notification_engine.toasts.template_reset"));
      void invalidate();
    },
  });
}

export function useSendTest() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (body: NotificationTestInput) =>
      notificationEngineService.test(slug, body),
    onSuccess: () => {
      appToast.success(t("notification_engine.toasts.test_sent"));
      void invalidate();
    },
  });
}
