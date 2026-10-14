import type { FleetResourceKind } from "@/features/fleet/types";

/** Query keys carry the organization slug: tabs on two organizations never share cache. */
export const fleetKeys = {
  all: (slug: string) => ["tenant", slug, "fleet"] as const,
  resource: (slug: string, kind: FleetResourceKind) =>
    [...fleetKeys.all(slug), kind] as const,
  lists: (slug: string, kind: FleetResourceKind) =>
    [...fleetKeys.resource(slug, kind), "list"] as const,
  list: (
    slug: string,
    kind: FleetResourceKind,
    params: Record<string, unknown>,
  ) => [...fleetKeys.lists(slug, kind), params] as const,
  meta: (slug: string, kind: FleetResourceKind) =>
    [...fleetKeys.resource(slug, kind), "meta"] as const,
  facets: (slug: string, kind: FleetResourceKind) =>
    [...fleetKeys.resource(slug, kind), "facets"] as const,
  detail: (slug: string, kind: FleetResourceKind, uuid: string) =>
    [...fleetKeys.resource(slug, kind), "detail", uuid] as const,
  activity: (slug: string, kind: FleetResourceKind, uuid: string) =>
    [...fleetKeys.detail(slug, kind, uuid), "activity"] as const,
  versions: (slug: string, uuid: string) =>
    [...fleetKeys.detail(slug, "products", uuid), "versions"] as const,
  version: (slug: string, uuid: string, versionUuid: string) =>
    [...fleetKeys.versions(slug, uuid), versionUuid] as const,
};
