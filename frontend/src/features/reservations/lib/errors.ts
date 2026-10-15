import type { AvailabilityAlternatives } from "@/features/availability/types";
import type {
  Quote,
  ReservationDriver,
  ReservationExtensionAlternatives,
} from "@/features/reservations/types";
import { ApiError } from "@/lib/api";

/** `data` of an error envelope (409 / 422 answers that carry a payload). */
export function errorData<T>(error: unknown): T | null {
  if (!(error instanceof ApiError)) return null;
  const body = error.body as { data?: T | null } | undefined;
  return body?.data ?? null;
}

/**
 * What a reservation create / change answered, sorted by what the screen
 * offers next.
 */
export type ReservationProblem =
  | { kind: "price_changed"; replacement: Quote | null; message: string }
  | { kind: "quote_expired"; message: string }
  | { kind: "quote_used"; message: string }
  | {
      kind: "capacity";
      reason: string;
      alternatives: AvailabilityAlternatives | null;
    }
  | { kind: "drivers"; drivers: ReservationDriver[]; message: string }
  | { kind: "equipment"; message: string }
  | { kind: "coupon"; reason: string; message: string }
  | {
      kind: "extension";
      alternatives: ReservationExtensionAlternatives | null;
      message: string;
    }
  | { kind: "other"; message: string };

export function reservationProblem(error: unknown): ReservationProblem {
  if (!(error instanceof ApiError)) {
    return { kind: "other", message: String(error) };
  }
  switch (error.code) {
    case "QUOTE_PRICE_CHANGED":
      return {
        kind: "price_changed",
        replacement: errorData<Quote>(error),
        message: error.message,
      };
    case "QUOTE_EXPIRED":
      return { kind: "quote_expired", message: error.message };
    case "QUOTE_USED":
      return { kind: "quote_used", message: error.message };
    case "CAPACITY_UNAVAILABLE":
      return {
        kind: "capacity",
        reason: error.details[0]?.code ?? "no_vehicle",
        alternatives: errorData<AvailabilityAlternatives>(error),
      };
    case "DRIVER_NOT_ELIGIBLE":
      return {
        kind: "drivers",
        drivers: errorData<ReservationDriver[]>(error) ?? [],
        message: error.message,
      };
    case "EQUIPMENT_UNAVAILABLE":
      return { kind: "equipment", message: error.message };
    case "EXTENSION_CONFLICT":
      return {
        kind: "extension",
        alternatives: errorData<ReservationExtensionAlternatives>(error),
        message: error.message,
      };
    default: {
      const coupon = error.details.find((d) => d.field === "coupon_code");
      if (coupon?.code?.startsWith("coupon_")) {
        return {
          kind: "coupon",
          reason: coupon.code.slice("coupon_".length),
          message: coupon.message ?? error.message,
        };
      }
      return { kind: "other", message: error.message };
    }
  }
}
