import type {
  DeductionTarget,
  DepositMethod,
  DepositStatus,
} from "@/features/deposits/types";

/** Engine / live list resource of security deposits (TEC-719). */
export const DEPOSITS_RESOURCE = "tenant.security_deposits";

type Tone = "default" | "success" | "warning" | "danger";

export const DEPOSIT_METHODS: DepositMethod[] = [
  "cash",
  "pos_preauth",
  "online_preauth",
  "bank_transfer",
];

export const DEPOSIT_METHOD_LABEL_KEYS: Record<DepositMethod, string> = {
  cash: "deposits.method.cash",
  pos_preauth: "deposits.method.pos_preauth",
  online_preauth: "deposits.method.online_preauth",
  bank_transfer: "deposits.method.bank_transfer",
};

export const DEPOSIT_STATUSES: DepositStatus[] = [
  "pending",
  "held",
  "partially_captured",
  "released",
  "refunded",
  "cancelled",
];

export const DEPOSIT_STATUS_LABEL_KEYS: Record<DepositStatus, string> = {
  pending: "deposits.status.pending",
  held: "deposits.status.held",
  partially_captured: "deposits.status.partially_captured",
  released: "deposits.status.released",
  refunded: "deposits.status.refunded",
  cancelled: "deposits.status.cancelled",
};

export const DEPOSIT_STATUS_TONES: Record<DepositStatus, Tone> = {
  pending: "warning",
  held: "default",
  partially_captured: "warning",
  released: "success",
  refunded: "success",
  cancelled: "danger",
};

export const DEDUCTION_TARGETS: DeductionTarget[] = [
  "charge",
  "rent",
  "general",
];

export const DEDUCTION_TARGET_LABEL_KEYS: Record<DeductionTarget, string> = {
  charge: "deposits.applies_to.charge",
  rent: "deposits.applies_to.rent",
  general: "deposits.applies_to.general",
};

/** Resolution hint per method (what happens with the money). */
export const RESOLVE_HINT_KEYS: Record<DepositMethod, string> = {
  cash: "deposits.resolve.hint_cash",
  bank_transfer: "deposits.resolve.hint_bank_transfer",
  pos_preauth: "deposits.resolve.hint_pos_preauth",
  online_preauth: "deposits.resolve.hint_online_preauth",
};

/** System cancel reasons (`system:*`). */
export const CANCEL_REASON_KEYS: Record<string, string> = {
  "system:preauth_failed": "deposits.cancel_reason.preauth_failed",
  "system:preauth_declined": "deposits.cancel_reason.preauth_declined",
};

/** "Held over N days" presets of the list filter. */
export const HELD_OVER_PRESETS = [3, 7, 14, 30] as const;
