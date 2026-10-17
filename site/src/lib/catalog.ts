import "server-only";

import { cache } from "react";

import { bookingApi } from "./server";
import type { Location, Subgroup } from "./types";

/**
 * Online-selectable branches and points, or null when booking is
 * unavailable. One API call per request (header, footer and page share it).
 */
export const loadLocations = cache(async (): Promise<Location[] | null> => {
  try {
    return await bookingApi<Location[]>("/v1/public/locations");
  } catch {
    return null;
  }
});

/**
 * The storefront's live vehicle subgroups (the fleet), or null when the
 * catalog cannot be read (booking off, API error). Never demo vehicles.
 */
export const loadSubgroups = cache(async (): Promise<Subgroup[] | null> => {
  try {
    return await bookingApi<Subgroup[]>("/v1/public/vehicle-subgroups");
  } catch {
    return null;
  }
});

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
