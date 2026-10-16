"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { useTenant } from "@/features/tenant/components/tenant-provider";
import { whatsappService } from "@/features/whatsapp/services/whatsapp.service";
import type {
  MessageListParams,
  WhatsAppConnectInput,
  WhatsAppSession,
  WhatsAppSettingsInput,
  WhatsAppTestInput,
} from "@/features/whatsapp/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** Query keys; every key starts with the organization slug. */
export const whatsappKeys = {
  all: (slug: string) => ["whatsapp", slug] as const,
  session: (slug: string) => ["whatsapp", slug, "session"] as const,
  qr: (slug: string) => ["whatsapp", slug, "qr"] as const,
  messages: (slug: string, p: MessageListParams) =>
    ["whatsapp", slug, "messages", p] as const,
};

export function useWhatsAppSession() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: whatsappKeys.session(slug),
    queryFn: () => whatsappService.session(slug),
  });
}

/**
 * The pairing QR code, polled every 3 seconds while the session waits for
 * a phone (the code rotates). A linked answer refreshes the session.
 */
export function useWhatsAppQR(enabled: boolean) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useQuery({
    queryKey: whatsappKeys.qr(slug),
    queryFn: async () => {
      const out = await whatsappService.qr(slug);
      if (out.status === "connected") {
        void qc.invalidateQueries({ queryKey: whatsappKeys.all(slug) });
      }
      return out;
    },
    enabled,
    refetchInterval: enabled ? 3000 : false,
    refetchIntervalInBackground: false,
    retry: false,
    gcTime: 0,
  });
}

export function useWhatsAppMessages(params: MessageListParams) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: whatsappKeys.messages(slug, params),
    queryFn: () => whatsappService.messages(slug, params),
    placeholderData: (previous) => previous,
  });
}

function useSessionMutation<V>(
  fn: (slug: string, v: V) => Promise<WhatsAppSession>,
  toastKey?: string,
) {
  const { slug } = useTenant();
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: V) => fn(slug, v),
    onSuccess: (session) => {
      qc.setQueryData(whatsappKeys.session(slug), session);
      void qc.invalidateQueries({ queryKey: whatsappKeys.all(slug) });
      if (toastKey) appToast.success(t(toastKey));
    },
  });
}

export function useConnectWhatsApp() {
  return useSessionMutation((slug, body: WhatsAppConnectInput) =>
    whatsappService.connect(slug, body),
  );
}

export function useRefreshWhatsApp() {
  return useSessionMutation((slug) => whatsappService.refresh(slug));
}

export function useLogoutWhatsApp() {
  return useSessionMutation(
    (slug) => whatsappService.logout(slug),
    "whatsapp.toasts.logged_out",
  );
}

export function useRemoveWhatsApp() {
  return useSessionMutation(
    (slug) => whatsappService.remove(slug),
    "whatsapp.toasts.removed",
  );
}

export function useUpdateWhatsAppSettings() {
  return useSessionMutation(
    (slug, body: WhatsAppSettingsInput) =>
      whatsappService.updateSettings(slug, body),
    "whatsapp.toasts.settings_saved",
  );
}

export function useSendWhatsAppTest() {
  const { slug } = useTenant();
  const { t } = useLocale();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: WhatsAppTestInput) => whatsappService.test(slug, body),
    onSuccess: () => {
      appToast.success(t("whatsapp.toasts.test_sent"));
      void qc.invalidateQueries({ queryKey: whatsappKeys.all(slug) });
    },
  });
}
