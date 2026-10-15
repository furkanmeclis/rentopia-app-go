import { ApiError } from "@/lib/api";

/** State / conflict detail codes the rental API answers (409). */
const PROBLEM_KEYS: Record<string, string> = {
  contract_not_approved: "rentals.problems.contract_not_approved",
  invalid_transition: "rentals.problems.invalid_transition",
  allocation_not_active: "rentals.problems.allocation_not_active",
  reservation_not_confirmed: "rentals.problems.reservation_not_confirmed",
  not_ready: "rentals.problems.not_ready",
  rental_underway: "rentals.problems.rental_underway",
  already_decided: "rentals.problems.already_decided",
  task_closed: "rentals.problems.task_closed",
  record_handover: "rentals.problems.record_handover",
  record_return: "rentals.problems.record_return",
  extension_conflict: "rentals.problems.extension_conflict",
};

/**
 * Text of a failed rental action: a known detail code in the current
 * language, the capacity answer, validation details or the API message.
 */
export function rentalProblemText(
  t: (key: string) => string,
  error: unknown,
): string {
  if (!(error instanceof ApiError)) return String(error);
  if (error.code === "CAPACITY_UNAVAILABLE") {
    return t("rentals.problems.capacity");
  }
  const code = error.details[0]?.code;
  const key = code ? PROBLEM_KEYS[code] : undefined;
  if (key) return t(key);
  if (error.status === 422 && error.details.length > 0) {
    return error.details
      .map((d) => (d.field ? `${d.field}: ${d.message}` : d.message))
      .join(" · ");
  }
  return error.message;
}

/** True when the handover failed for a missing contract approval. */
export function isContractProblem(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    error.details[0]?.code === "contract_not_approved"
  );
}
