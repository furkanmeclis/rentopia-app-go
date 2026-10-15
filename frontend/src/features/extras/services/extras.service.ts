import type {
  EquipmentAllocation,
  EquipmentAllocationPage,
  EquipmentReserveRequest,
  Extra,
  ExtraActivityPage,
  ExtraCreateRequest,
  ExtraFacets,
  ExtraPage,
  ExtraPatchRequest,
  ExtraPriceHistoryPage,
  ExtraPriceMatrix,
  ExtraPricesRequest,
  ExtraStockRequest,
  ExtraStockResult,
  ExtraStockRow,
  ExtrasAvailability,
  ExtrasAvailabilityRequest,
  ListAllocationsParams,
  ListExtrasParams,
} from "@/features/extras/types";
import type { ResourceMeta } from "@/features/io/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const EXTRAS = "/v1/tenant/extras";
const ALLOCATIONS = "/v1/tenant/equipment-allocations";

type PageParams = { limit: number; offset: number };

/**
 * Extras catalog, prices, branch stock and equipment allocations of one
 * organization. Every call names the organization through `tenantRequest`.
 */
export const extrasService = {
  list(slug: string, params: ListExtrasParams) {
    return tenantRequest<ExtraPage>(slug, "GET", EXTRAS, { query: params });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${EXTRAS}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<ExtraFacets>(slug, "GET", `${EXTRAS}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Extra>(slug, "GET", `${EXTRAS}/${uuid}`);
  },
  create(slug: string, body: ExtraCreateRequest) {
    return tenantRequest<Extra>(slug, "POST", EXTRAS, { body });
  },
  update(slug: string, uuid: string, body: ExtraPatchRequest) {
    return tenantRequest<Extra>(slug, "PATCH", `${EXTRAS}/${uuid}`, { body });
  },
  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${EXTRAS}/${uuid}`,
    );
  },
  activity(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<ExtraActivityPage>(
      slug,
      "GET",
      `${EXTRAS}/${uuid}/activity`,
      { query: params },
    );
  },
  prices(slug: string, uuid: string) {
    return tenantRequest<ExtraPriceMatrix>(
      slug,
      "GET",
      `${EXTRAS}/${uuid}/prices`,
    );
  },
  setPrices(slug: string, uuid: string, body: ExtraPricesRequest) {
    return tenantRequest<ExtraPriceMatrix>(
      slug,
      "PUT",
      `${EXTRAS}/${uuid}/prices`,
      { body },
    );
  },
  priceHistory(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<ExtraPriceHistoryPage>(
      slug,
      "GET",
      `${EXTRAS}/${uuid}/price-history`,
      { query: params },
    );
  },
  stock(slug: string, uuid: string) {
    return tenantRequest<{ items: ExtraStockRow[] }>(
      slug,
      "GET",
      `${EXTRAS}/${uuid}/stock`,
    );
  },
  setStock(slug: string, uuid: string, body: ExtraStockRequest) {
    return tenantRequest<ExtraStockResult>(
      slug,
      "PUT",
      `${EXTRAS}/${uuid}/stock`,
      { body },
    );
  },
  availability(slug: string, body: ExtrasAvailabilityRequest) {
    return tenantRequest<ExtrasAvailability>(
      slug,
      "POST",
      `${EXTRAS}/availability`,
      { body },
    );
  },
  allocations(slug: string, params: ListAllocationsParams) {
    return tenantRequest<EquipmentAllocationPage>(slug, "GET", ALLOCATIONS, {
      query: params,
    });
  },
  reserve(slug: string, body: EquipmentReserveRequest) {
    // Silent: the dialog shows EQUIPMENT_UNAVAILABLE itself.
    return tenantRequest<EquipmentAllocation>(slug, "POST", ALLOCATIONS, {
      body,
      silent: true,
    });
  },
  confirm(slug: string, uuid: string) {
    return tenantRequest<EquipmentAllocation>(
      slug,
      "POST",
      `${ALLOCATIONS}/${uuid}/confirm`,
      { body: {} },
    );
  },
  release(slug: string, uuid: string) {
    return tenantRequest<EquipmentAllocation>(
      slug,
      "DELETE",
      `${ALLOCATIONS}/${uuid}`,
    );
  },
};
