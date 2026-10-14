import type { ResourceMeta } from "@/features/io/types";
import type {
  ListVehiclesParams,
  ProductVehicles,
  Vehicle,
  VehicleCreateRequest,
  VehicleEventPage,
  VehicleFacets,
  VehicleGroups,
  VehicleKasko,
  VehicleMembership,
  VehiclePage,
  VehiclePatchRequest,
  VehicleStatusRequest,
} from "@/features/vehicles/types";
import type { FleetActivityPage } from "@/features/fleet/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/vehicles";

type PageParams = { limit: number; offset: number };

/**
 * Vehicles of one organization. Every call names the organization through
 * `tenantRequest` (`X-Organization: {slug}`).
 */
export const vehiclesService = {
  list(slug: string, params: ListVehiclesParams) {
    return tenantRequest<VehiclePage>(slug, "GET", BASE, { query: params });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<VehicleFacets>(slug, "GET", `${BASE}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Vehicle>(slug, "GET", `${BASE}/${uuid}`);
  },
  create(slug: string, body: VehicleCreateRequest) {
    return tenantRequest<Vehicle>(slug, "POST", BASE, { body });
  },
  update(slug: string, uuid: string, body: VehiclePatchRequest) {
    return tenantRequest<Vehicle>(slug, "PATCH", `${BASE}/${uuid}`, { body });
  },
  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(slug, "DELETE", `${BASE}/${uuid}`);
  },
  changeStatus(slug: string, uuid: string, body: VehicleStatusRequest) {
    return tenantRequest<Vehicle>(slug, "POST", `${BASE}/${uuid}/status`, {
      body,
    });
  },
  events(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<VehicleEventPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/events`,
      { query: params },
    );
  },
  groups(slug: string, uuid: string) {
    return tenantRequest<VehicleGroups>(
      slug,
      "GET",
      `${BASE}/${uuid}/rental-groups`,
    );
  },
  /** Silent: a 422 NOT_ELIGIBLE answer is shown by the caller. */
  addGroup(slug: string, uuid: string, rentalGroupUuid: string) {
    return tenantRequest<VehicleMembership>(
      slug,
      "POST",
      `${BASE}/${uuid}/rental-groups`,
      { body: { rental_group_uuid: rentalGroupUuid }, silent: true },
    );
  },
  removeGroup(slug: string, uuid: string, rentalGroupUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/rental-groups/${rentalGroupUuid}`,
    );
  },
  kasko(slug: string, uuid: string) {
    return tenantRequest<VehicleKasko>(slug, "GET", `${BASE}/${uuid}/kasko`);
  },
  activity(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<FleetActivityPage>(
      slug,
      "GET",
      `${BASE}/${uuid}/activity`,
      { query: params },
    );
  },
  productVehicles(slug: string, productUuid: string, params: PageParams) {
    return tenantRequest<ProductVehicles>(
      slug,
      "GET",
      `/v1/tenant/products/${productUuid}/eligible-vehicles`,
      { query: params },
    );
  },
};
