import type {
  NotificationChannel,
  NotificationDeliveryStatus,
  NotificationDispatchStatus,
  NotificationLanguage,
} from "@/features/notification-engine/types";

/** Resource of the delivery history table (meta, persist keys). */
export const DELIVERIES_RESOURCE = "tenant.notification_deliveries";

export const CHANNELS: NotificationChannel[] = [
  "sms",
  "email",
  "whatsapp",
  "push",
];

export const CHANNEL_LABEL_KEYS: Record<NotificationChannel, string> = {
  sms: "notification_engine.channels.sms",
  email: "notification_engine.channels.email",
  whatsapp: "notification_engine.channels.whatsapp",
  push: "notification_engine.channels.push",
};

export const LANGUAGES: NotificationLanguage[] = ["tr", "en"];

export const LANGUAGE_LABEL_KEYS: Record<NotificationLanguage, string> = {
  tr: "notification_engine.languages.tr",
  en: "notification_engine.languages.en",
};

export const DELIVERY_STATUSES: NotificationDeliveryStatus[] = [
  "sending",
  "sent",
  "delivered",
  "failed",
  "skipped",
];

export const DELIVERY_STATUS_LABEL_KEYS: Record<
  NotificationDeliveryStatus,
  string
> = {
  sending: "notification_engine.delivery_statuses.sending",
  sent: "notification_engine.delivery_statuses.sent",
  delivered: "notification_engine.delivery_statuses.delivered",
  failed: "notification_engine.delivery_statuses.failed",
  skipped: "notification_engine.delivery_statuses.skipped",
};

export const DISPATCH_STATUS_LABEL_KEYS: Record<
  NotificationDispatchStatus,
  string
> = {
  pending: "notification_engine.dispatch_statuses.pending",
  sending: "notification_engine.dispatch_statuses.sending",
  delivered: "notification_engine.dispatch_statuses.delivered",
  failed: "notification_engine.dispatch_statuses.failed",
  skipped: "notification_engine.dispatch_statuses.skipped",
  reconciled: "notification_engine.dispatch_statuses.reconciled",
};

export type Tone = "default" | "success" | "warning" | "danger";

export const DELIVERY_STATUS_TONES: Record<NotificationDeliveryStatus, Tone> = {
  sending: "default",
  sent: "success",
  delivered: "success",
  failed: "danger",
  skipped: "warning",
};

export const DISPATCH_STATUS_TONES: Record<NotificationDispatchStatus, Tone> = {
  pending: "default",
  sending: "default",
  delivered: "success",
  failed: "danger",
  skipped: "warning",
  reconciled: "default",
};

/** Event labels (static keys; the catalog lives in the backend). */
export const EVENT_LABEL_KEYS: Record<string, string> = {
  "reservation.confirmed": "notification_engine.events.reservation_confirmed",
  "reservation.rejected": "notification_engine.events.reservation_rejected",
  "reservation.cancelled": "notification_engine.events.reservation_cancelled",
  "reservation.payment_pending":
    "notification_engine.events.reservation_payment_pending",
  "rental.pickup_upcoming": "notification_engine.events.rental_pickup_upcoming",
  "rental.return_upcoming": "notification_engine.events.rental_return_upcoming",
  "rental.overdue": "notification_engine.events.rental_overdue",
  "invoice.issued": "notification_engine.events.invoice_issued",
  "account.payment_due": "notification_engine.events.account_payment_due",
  "field_task.assigned": "notification_engine.events.field_task_assigned",
  "field_task.overdue": "notification_engine.events.field_task_overdue",
  "site_lead.created": "notification_engine.events.site_lead_created",
};

export const GROUP_LABEL_KEYS: Record<string, string> = {
  reservations: "notification_engine.groups.reservations",
  rentals: "notification_engine.groups.rentals",
  finance: "notification_engine.groups.finance",
  operations: "notification_engine.groups.operations",
};

export const AUDIENCE_LABEL_KEYS: Record<string, string> = {
  customer: "notification_engine.audiences.customer",
  staff: "notification_engine.audiences.staff",
};

/** Reminder offset presets (minutes before the deadline). */
export const OFFSET_PRESETS = [
  0, 30, 60, 120, 360, 720, 1440, 2880, 4320, 10080,
];
