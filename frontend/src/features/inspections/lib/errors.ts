import { rentalProblemText } from "@/features/rentals/lib/errors";
import { ApiError } from "@/lib/api";

const PROBLEM_KEYS: Record<string, string> = {
  inspection_incomplete: "inspections.problems.incomplete",
  signature_required: "inspections.problems.signature_required",
  finalized: "inspections.problems.finalized",
  review_first: "inspections.problems.review_first",
  signature_bound: "inspections.problems.signature_bound",
  content_changed: "inspections.problems.content_changed",
  not_uploaded: "inspections.problems.not_uploaded",
  object_missing: "inspections.problems.object_missing",
  size_mismatch: "inspections.problems.object_missing",
  invalid_type: "inspections.photos.invalid_type",
  type_mismatch: "inspections.photos.invalid_type",
  too_large: "inspections.photos.too_large",
  invalid_state: "inspections.problems.invalid_state",
  review_pending: "inspections.problems.review_pending",
  no_review: "inspections.problems.no_review",
};

/** Text of a failed inspection action in the current language. */
export function inspectionProblemText(
  t: (key: string) => string,
  error: unknown,
): string {
  if (error instanceof ApiError) {
    if (error.status === 403) return t("inspections.problems.forbidden");
    const code = error.details[0]?.code;
    const key = code ? PROBLEM_KEYS[code] : undefined;
    if (key) return t(key);
  }
  return rentalProblemText(t, error);
}

/** True when the request never reached the API (offline, network down). */
export function isNetworkError(error: unknown): boolean {
  if (typeof navigator !== "undefined" && navigator.onLine === false) {
    return true;
  }
  return error instanceof TypeError;
}
