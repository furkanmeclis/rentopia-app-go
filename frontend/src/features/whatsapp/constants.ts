import type {
  WhatsAppMessageStatus,
  WhatsAppSessionStatus,
} from "@/features/whatsapp/types";

type Tone = "default" | "success" | "warning" | "danger";

export const SESSION_STATUS_LABEL_KEYS: Record<WhatsAppSessionStatus, string> =
  {
    not_connected: "whatsapp.statuses.not_connected",
    created: "whatsapp.statuses.created",
    pairing: "whatsapp.statuses.pairing",
    connected: "whatsapp.statuses.connected",
    disconnected: "whatsapp.statuses.disconnected",
    logged_out: "whatsapp.statuses.logged_out",
  };

export const SESSION_STATUS_TONES: Record<WhatsAppSessionStatus, Tone> = {
  not_connected: "default",
  created: "warning",
  pairing: "warning",
  connected: "success",
  disconnected: "danger",
  logged_out: "default",
};

export const MESSAGE_STATUSES: WhatsAppMessageStatus[] = [
  "queued",
  "sent",
  "delivered",
  "read",
  "failed",
];

export const MESSAGE_STATUS_LABEL_KEYS: Record<WhatsAppMessageStatus, string> =
  {
    queued: "whatsapp.message_statuses.queued",
    sent: "whatsapp.message_statuses.sent",
    delivered: "whatsapp.message_statuses.delivered",
    read: "whatsapp.message_statuses.read",
    failed: "whatsapp.message_statuses.failed",
  };

export const MESSAGE_STATUS_TONES: Record<WhatsAppMessageStatus, Tone> = {
  queued: "default",
  sent: "warning",
  delivered: "success",
  read: "success",
  failed: "danger",
};

export const PROVIDER_LABEL_KEYS: Record<string, string> = {
  wuzapi: "whatsapp.providers.wuzapi",
  whatsapp_fake: "whatsapp.providers.whatsapp_fake",
};

/** Rate limit bounds (messages per minute), as the API enforces them. */
export const RATE_LIMIT_MIN = 1;
export const RATE_LIMIT_MAX = 60;
