import type { FleetActivityPage } from "@/features/fleet/types";
import type {
  ListBlocksParams,
  ListOneWayRulesParams,
  ListTransfersParams,
  OneWayRule,
  OneWayRuleCreateRequest,
  OneWayRuleFacets,
  OneWayRulePage,
  OneWayRulePatchRequest,
  Transfer,
  TransferAction,
  TransferActionRequest,
  TransferAssignees,
  TransferCreateRequest,
  TransferFacets,
  TransferPage,
  TransferPatchRequest,
  VehicleBlockPage,
  VehicleBlockPatchRequest,
  VehicleBlockReleaseRequest,
  VehicleBlockRequest,
  VehicleBlockResult,
} from "@/features/fleet-ops/types";
import type { ResourceMeta } from "@/features/io/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const VEHICLES = "/v1/tenant/vehicles";
const TRANSFERS = "/v1/tenant/transfers";
const RULES = "/v1/tenant/one-way-rules";

type PageParams = { limit: number; offset: number };

/**
 * Vehicle blocks, transfers and one-way rules of one organization. Every
 * call names the organization through `tenantRequest` (`X-Organization`).
 */
export const fleetOpsService = {
  // --- blocks (children of a vehicle) ---
  blocks(slug: string, vehicleUuid: string, params: ListBlocksParams) {
    return tenantRequest<VehicleBlockPage>(
      slug,
      "GET",
      `${VEHICLES}/${vehicleUuid}/blocks`,
      { query: params },
    );
  },
  createBlock(slug: string, vehicleUuid: string, body: VehicleBlockRequest) {
    return tenantRequest<VehicleBlockResult>(
      slug,
      "POST",
      `${VEHICLES}/${vehicleUuid}/blocks`,
      { body },
    );
  },
  updateBlock(
    slug: string,
    vehicleUuid: string,
    blockUuid: string,
    body: VehicleBlockPatchRequest,
  ) {
    return tenantRequest<VehicleBlockResult>(
      slug,
      "PATCH",
      `${VEHICLES}/${vehicleUuid}/blocks/${blockUuid}`,
      { body },
    );
  },
  releaseBlock(
    slug: string,
    vehicleUuid: string,
    blockUuid: string,
    body: VehicleBlockReleaseRequest,
  ) {
    return tenantRequest<VehicleBlockResult>(
      slug,
      "POST",
      `${VEHICLES}/${vehicleUuid}/blocks/${blockUuid}/release`,
      { body },
    );
  },
  removeBlock(slug: string, vehicleUuid: string, blockUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${VEHICLES}/${vehicleUuid}/blocks/${blockUuid}`,
    );
  },

  // --- transfers ---
  transfers(slug: string, params: ListTransfersParams) {
    return tenantRequest<TransferPage>(slug, "GET", TRANSFERS, {
      query: params,
    });
  },
  transfersMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${TRANSFERS}/meta`);
  },
  transferFacets(slug: string) {
    return tenantRequest<TransferFacets>(slug, "GET", `${TRANSFERS}/facets`);
  },
  assignees(slug: string, q: string) {
    return tenantRequest<TransferAssignees>(
      slug,
      "GET",
      `${TRANSFERS}/assignees`,
      { query: { q } },
    );
  },
  createTransfer(slug: string, body: TransferCreateRequest) {
    return tenantRequest<Transfer>(slug, "POST", TRANSFERS, { body });
  },
  updateTransfer(slug: string, uuid: string, body: TransferPatchRequest) {
    return tenantRequest<Transfer>(slug, "PATCH", `${TRANSFERS}/${uuid}`, {
      body,
    });
  },
  transferAction(
    slug: string,
    uuid: string,
    action: TransferAction,
    body: TransferActionRequest,
  ) {
    return tenantRequest<Transfer>(
      slug,
      "POST",
      `${TRANSFERS}/${uuid}/${action}`,
      { body },
    );
  },
  transferActivity(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<FleetActivityPage>(
      slug,
      "GET",
      `${TRANSFERS}/${uuid}/activity`,
      { query: params },
    );
  },

  // --- one-way rules ---
  rules(slug: string, params: ListOneWayRulesParams) {
    return tenantRequest<OneWayRulePage>(slug, "GET", RULES, {
      query: params,
    });
  },
  rulesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${RULES}/meta`);
  },
  ruleFacets(slug: string) {
    return tenantRequest<OneWayRuleFacets>(slug, "GET", `${RULES}/facets`);
  },
  createRule(slug: string, body: OneWayRuleCreateRequest) {
    return tenantRequest<OneWayRule>(slug, "POST", RULES, { body });
  },
  updateRule(slug: string, uuid: string, body: OneWayRulePatchRequest) {
    return tenantRequest<OneWayRule>(slug, "PATCH", `${RULES}/${uuid}`, {
      body,
    });
  },
  removeRule(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${RULES}/${uuid}`,
    );
  },
};
