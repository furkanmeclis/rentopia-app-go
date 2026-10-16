"use client";

import { Bell, BellOff } from "lucide-react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { platformRequest } from "@/lib/api/platform-request";
import { notificationPermission } from "@/lib/web-push/register";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const SW_URL = "/app-push-sw.js";

const base = (orgSlug: string) => `/v1/customer/${encodeURIComponent(orgSlug)}`;

function urlBase64ToUint8Array(value: string): Uint8Array {
  const padding = "=".repeat((4 - (value.length % 4)) % 4);
  const raw = atob((value + padding).replace(/-/g, "+").replace(/_/g, "/"));
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

function supported(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

/**
 * Customer portal web push (TEC-733): subscribes this browser to the
 * organization's notifications for the signed-in customer. Customer
 * subscriptions are stored apart from staff subscriptions.
 */
export function CustomerPushToggle({ orgSlug }: { orgSlug: string }) {
  const { t } = useLocale();
  const [key, setKey] = useState<string | null>(null);
  const [subscribed, setSubscribed] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!supported()) return;
    let cancelled = false;
    void (async () => {
      try {
        const res = await platformRequest<{ vapid_public_key: string }>(
          "GET",
          `${base(orgSlug)}/push/vapid-public-key`,
          { silent: true },
        );
        if (cancelled || !res.vapid_public_key) return;
        setKey(res.vapid_public_key);
        const reg = await navigator.serviceWorker.getRegistration(SW_URL);
        const sub = await reg?.pushManager.getSubscription();
        if (!cancelled) {
          setSubscribed(Boolean(sub) && notificationPermission() === "granted");
        }
      } catch {
        // No session or push off: the button stays hidden.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [orgSlug]);

  if (!key) return null;

  const enable = async () => {
    setBusy(true);
    try {
      const permission = await window.Notification.requestPermission();
      if (permission !== "granted") {
        appToast.error(t("notification_engine.portal.denied"));
        return;
      }
      const reg = await navigator.serviceWorker.register(SW_URL);
      await navigator.serviceWorker.ready;
      const sub =
        (await reg.pushManager.getSubscription()) ??
        (await reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(key) as BufferSource,
        }));
      const json = sub.toJSON();
      await platformRequest("POST", `${base(orgSlug)}/push-subscriptions`, {
        body: {
          endpoint: json.endpoint,
          key_p256dh: json.keys?.p256dh ?? "",
          key_auth: json.keys?.auth ?? "",
        },
        silent: true,
      });
      setSubscribed(true);
      appToast.success(t("notification_engine.portal.enabled"));
    } catch {
      appToast.error(t("notification_engine.portal.failed"));
    } finally {
      setBusy(false);
    }
  };

  const disable = async () => {
    setBusy(true);
    try {
      const reg = await navigator.serviceWorker.getRegistration(SW_URL);
      const sub = await reg?.pushManager.getSubscription();
      if (sub) {
        await platformRequest("DELETE", `${base(orgSlug)}/push-subscriptions`, {
          body: { endpoint: sub.endpoint },
          silent: true,
        });
        await sub.unsubscribe();
      }
      setSubscribed(false);
      appToast.success(t("notification_engine.portal.disabled"));
    } catch {
      appToast.error(t("notification_engine.portal.failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      disabled={busy}
      onClick={() => void (subscribed ? disable() : enable())}
      aria-label={
        subscribed
          ? t("notification_engine.portal.disable")
          : t("notification_engine.portal.enable")
      }
    >
      {subscribed ? (
        <BellOff className="size-4" />
      ) : (
        <Bell className="size-4" />
      )}
      <span className="hidden sm:inline">
        {subscribed
          ? t("notification_engine.portal.disable")
          : t("notification_engine.portal.enable")}
      </span>
    </Button>
  );
}
