import type {
  SupportGrantScope,
  SupportGrantStatus,
} from "@/features/support-access/types";

export const SUPPORT_SCOPES: SupportGrantScope[] = ["read_only", "full"];
export const SUPPORT_STATUSES: SupportGrantStatus[] = [
  "active",
  "expired",
  "revoked",
];

/** Durations offered in the form (backend accepts 1 h – 7 d). */
export const SUPPORT_DURATIONS = [1, 4, 8, 24, 72, 168] as const;

export const SCOPE_LABEL_KEYS: Record<SupportGrantScope, string> = {
  read_only: "support_access.scopes.read_only",
  full: "support_access.scopes.full",
};

export const SCOPE_HINT_KEYS: Record<SupportGrantScope, string> = {
  read_only: "support_access.scopes.read_only_hint",
  full: "support_access.scopes.full_hint",
};

export const STATUS_LABEL_KEYS: Record<SupportGrantStatus, string> = {
  active: "support_access.statuses.active",
  expired: "support_access.statuses.expired",
  revoked: "support_access.statuses.revoked",
};

export const STATUS_TONES: Record<
  SupportGrantStatus,
  "success" | "default" | "danger"
> = {
  active: "success",
  expired: "default",
  revoked: "danger",
};

export const DURATION_LABEL_KEYS: Record<
  (typeof SUPPORT_DURATIONS)[number],
  string
> = {
  1: "support_access.durations.h1",
  4: "support_access.durations.h4",
  8: "support_access.durations.h8",
  24: "support_access.durations.d1",
  72: "support_access.durations.d3",
  168: "support_access.durations.d7",
};
