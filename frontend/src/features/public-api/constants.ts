import type {
  PublicApiChannel,
  PublicApiKeyKind,
  PublicApiScope,
  PublicWebhookDeliveryStatus,
  PublicWebhookEvent,
} from "@/features/public-api/types";

/** Live list resources (backend `publicapi.RegisterLiveResources`). */
export const PUBLIC_API_KEYS_RESOURCE = "tenant.public_api_keys";
export const PUBLIC_WEBHOOKS_RESOURCE = "tenant.public_webhooks";

export const KEY_KINDS: PublicApiKeyKind[] = ["secret", "publishable"];
export const KEY_STATUSES = ["active", "revoked"] as const;
export const CHANNELS: PublicApiChannel[] = ["web", "widget", "api"];
export const SCOPES: PublicApiScope[] = [
  "catalog.read",
  "quotes.write",
  "reservations.write",
  "reservations.read",
];
export const WEBHOOK_EVENTS: PublicWebhookEvent[] = [
  "reservation.created",
  "reservation.confirmed",
  "reservation.changed",
  "reservation.cancelled",
  "reservation.plate_fixed",
];
export const ENDPOINT_STATUSES = ["active", "disabled"] as const;
export const DELIVERY_STATUSES: PublicWebhookDeliveryStatus[] = [
  "pending",
  "succeeded",
  "failed",
];

export const KEY_KIND_LABEL_KEYS: Record<PublicApiKeyKind, string> = {
  secret: "public_api.kinds.secret",
  publishable: "public_api.kinds.publishable",
};

export const KEY_STATUS_LABEL_KEYS: Record<string, string> = {
  active: "public_api.key_statuses.active",
  revoked: "public_api.key_statuses.revoked",
};

export const CHANNEL_LABEL_KEYS: Record<PublicApiChannel, string> = {
  web: "public_api.channels.web",
  widget: "public_api.channels.widget",
  api: "public_api.channels.api",
};

export const SCOPE_LABEL_KEYS: Record<PublicApiScope, string> = {
  "catalog.read": "public_api.scopes.catalog_read",
  "quotes.write": "public_api.scopes.quotes_write",
  "reservations.write": "public_api.scopes.reservations_write",
  "reservations.read": "public_api.scopes.reservations_read",
};

export const EVENT_LABEL_KEYS: Record<string, string> = {
  "reservation.created": "public_api.events.reservation_created",
  "reservation.confirmed": "public_api.events.reservation_confirmed",
  "reservation.changed": "public_api.events.reservation_changed",
  "reservation.cancelled": "public_api.events.reservation_cancelled",
  "reservation.plate_fixed": "public_api.events.reservation_plate_fixed",
  ping: "public_api.events.ping",
};

export const ENDPOINT_STATUS_LABEL_KEYS: Record<string, string> = {
  active: "public_api.endpoint_statuses.active",
  disabled: "public_api.endpoint_statuses.disabled",
};

export const DELIVERY_STATUS_LABEL_KEYS: Record<
  PublicWebhookDeliveryStatus,
  string
> = {
  pending: "public_api.delivery_statuses.pending",
  succeeded: "public_api.delivery_statuses.succeeded",
  failed: "public_api.delivery_statuses.failed",
};

export const DELIVERY_STATUS_TONES: Record<
  PublicWebhookDeliveryStatus,
  "default" | "success" | "warning" | "danger"
> = {
  pending: "warning",
  succeeded: "success",
  failed: "danger",
};
