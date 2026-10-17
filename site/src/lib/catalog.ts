import "server-only";

import { bookingApi } from "./server";
import type { Location } from "./types";

/** Online-selectable branches and points, or null when booking is unavailable. */
export async function loadLocations(): Promise<Location[] | null> {
  try {
    return await bookingApi<Location[]>("/v1/public/locations");
  } catch {
    return null;
  }
}

/** "branch" / "branch:point" → the API's uuid pair. */
export function splitPlace(
  v: string | undefined,
): { branch: string; point?: string } | null {
  const uuid =
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  if (!v) return null;
  const [branch, point] = v.split(":");
  if (
    !branch ||
    !uuid.test(branch) ||
    (point !== undefined && !uuid.test(point))
  )
    return null;
  return { branch, point };
}
