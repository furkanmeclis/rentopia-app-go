import type { KabisEvent, KabisStatus } from "@/features/kabis/types";

/** Live list resource slug (backend kabis/model). */
export const KABIS_RESOURCE = "tenant.kabis_notifications" as const;

type Tone = "default" | "success" | "warning" | "danger";

/** Filter order of the screen: what needs a person first. */
export const KABIS_STATUSES: KabisStatus[] = [
  "blocked",
  "waiting_dependency",
  "needs_verification",
  "failed",
  "pending",
  "submitted",
  "succeeded",
  "reported_manually",
  "reconciled",
];

/** Need a person (nav badge). */
export const KABIS_NEEDS_ACTION: KabisStatus[] = [
  "blocked",
  "waiting_dependency",
  "failed",
  "needs_verification",
];

/** The only states in which the authority has the record. */
export const KABIS_OFFICIAL: KabisStatus[] = [
  "succeeded",
  "reported_manually",
  "reconciled",
];

/**
 * Static i18n key maps (the i18n checker verifies every key). The wording
 * never makes a local record look like an official notification.
 */
export const KABIS_STATUS_LABEL_KEYS: Record<KabisStatus, string> = {
  blocked: "kabis.statuses.blocked",
  waiting_dependency: "kabis.statuses.waiting_dependency",
  pending: "kabis.statuses.pending",
  processing: "kabis.statuses.processing",
  submitted: "kabis.statuses.submitted",
  needs_verification: "kabis.statuses.needs_verification",
  failed: "kabis.statuses.failed",
  succeeded: "kabis.statuses.succeeded",
  reported_manually: "kabis.statuses.reported_manually",
  reconciled: "kabis.statuses.reconciled",
  ignored: "kabis.statuses.ignored",
};

export const KABIS_STATUS_HINT_KEYS: Record<KabisStatus, string> = {
  blocked: "kabis.status_hints.blocked",
  waiting_dependency: "kabis.status_hints.waiting_dependency",
  pending: "kabis.status_hints.pending",
  processing: "kabis.status_hints.processing",
  submitted: "kabis.status_hints.submitted",
  needs_verification: "kabis.status_hints.needs_verification",
  failed: "kabis.status_hints.failed",
  succeeded: "kabis.status_hints.succeeded",
  reported_manually: "kabis.status_hints.reported_manually",
  reconciled: "kabis.status_hints.reconciled",
  ignored: "kabis.status_hints.ignored",
};

export const KABIS_STATUS_TONES: Record<KabisStatus, Tone> = {
  blocked: "danger",
  waiting_dependency: "warning",
  pending: "warning",
  processing: "warning",
  submitted: "warning",
  needs_verification: "danger",
  failed: "danger",
  succeeded: "success",
  reported_manually: "success",
  reconciled: "success",
  ignored: "default",
};

export const KABIS_EVENTS: KabisEvent[] = [
  "rental_start",
  "rental_end",
  "correction",
];

export const KABIS_EVENT_LABEL_KEYS: Record<KabisEvent, string> = {
  rental_start: "kabis.events.rental_start",
  rental_end: "kabis.events.rental_end",
  correction: "kabis.events.correction",
};

export const KABIS_CAUSE_LABEL_KEYS: Record<string, string> = {
  handover: "kabis.causes.handover",
  return: "kabis.causes.return",
  drivers_changed: "kabis.causes.drivers_changed",
  vehicle_swap: "kabis.causes.vehicle_swap",
};

export const KABIS_HOLD_LABEL_KEYS: Record<string, string> = {
  no_account: "kabis.holds.no_account",
  adapter_unverified: "kabis.holds.adapter_unverified",
  adapter_unavailable: "kabis.holds.adapter_unavailable",
};

export const KABIS_RETRY_ACTION_KEYS: Record<string, string> = {
  revalidated: "kabis.toast.revalidated",
  already_done: "kabis.toast.already_done",
  in_progress: "kabis.toast.in_progress",
  resubmitted: "kabis.toast.resubmitted",
};

export const KABIS_ROLE_LABEL_KEYS: Record<string, string> = {
  renter: "kabis.roles.renter",
  primary: "kabis.roles.primary",
  additional: "kabis.roles.additional",
};

/** Missing field paths ("drivers.0.identity_number") by their last part. */
export const KABIS_FIELD_LABEL_KEYS: Record<string, string> = {
  plate: "kabis.missing.plate",
  name: "kabis.missing.name",
  city: "kabis.missing.city",
  first_name: "kabis.missing.first_name",
  last_name: "kabis.missing.last_name",
  company_title: "kabis.missing.company_title",
  identity_number: "kabis.missing.identity_number",
  nationality: "kabis.missing.nationality",
  licence_number: "kabis.missing.licence_number",
  record: "kabis.missing.record",
  drivers: "kabis.missing.drivers",
};

export const KABIS_FIELD_SCOPE_KEYS: Record<string, string> = {
  vehicle: "kabis.missing_scope.vehicle",
  place: "kabis.missing_scope.place",
  renter: "kabis.missing_scope.renter",
  drivers: "kabis.missing_scope.driver",
};

/** Accepted attachment types and size (backend: PDF / JPEG / PNG / WebP, 10 MiB). */
export const KABIS_ATTACHMENT_ACCEPT =
  "application/pdf,image/jpeg,image/png,image/webp";
export const KABIS_ATTACHMENT_MAX_BYTES = 10 * 1024 * 1024;
