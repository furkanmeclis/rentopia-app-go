import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type KabisNotification = Schemas["KabisNotification"];
export type KabisNotificationDetail = Schemas["KabisNotificationDetail"];
export type KabisNotificationPage =
  Schemas["EnvelopeKabisNotificationPage"]["data"];
export type KabisFacets = Schemas["KabisFacets"];
export type KabisRetryResult = Schemas["EnvelopeKabisRetry"]["data"];
export type KabisStatus = KabisNotification["status"];
export type KabisEvent = KabisNotification["event"];

/** List query of `GET /v1/tenant/kabis-notifications` (multi-values comma-joined). */
export type ListKabisParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  event?: string;
  rental?: string;
  needs_action?: string;
  from?: string;
  to?: string;
};

/** "Reported manually" or "reconciled". */
export type KabisResolution = "mark-manual" | "reconcile";

export type KabisResolveInput = {
  reason: string;
  reference: string;
  file?: File | null;
};
