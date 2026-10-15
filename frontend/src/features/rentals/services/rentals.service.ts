import type { ResourceMeta } from "@/features/io/types";
import type {
  ListOperationTasksParams,
  ListRentalsParams,
  OperationTask,
  OperationTaskFacets,
  OperationTaskPage,
  Rental,
  RentalChargeRequest,
  RentalCompleteReturnRequest,
  RentalCreateRequest,
  RentalEventPage,
  RentalExtendRequest,
  RentalFacets,
  RentalHandoverRequest,
  RentalPage,
  RentalReturnRequest,
  RentalSwapRequest,
} from "@/features/rentals/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const RENTALS = "/v1/tenant/rentals";
const TASKS = "/v1/tenant/operation-tasks";
const one = (uuid: string) => `${RENTALS}/${uuid}`;

/**
 * Rentals and operation tasks (`tenantRequest` only). Actions whose errors
 * the dialogs render themselves are `silent` (no global toast).
 */
export const rentalsService = {
  list(slug: string, params: ListRentalsParams) {
    return tenantRequest<RentalPage>(slug, "GET", RENTALS, { query: params });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${RENTALS}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<RentalFacets>(slug, "GET", `${RENTALS}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Rental>(slug, "GET", one(uuid));
  },
  events(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<RentalEventPage>(slug, "GET", `${one(uuid)}/events`, {
      query: params,
    });
  },
  create(slug: string, body: RentalCreateRequest) {
    return tenantRequest<Rental>(slug, "POST", RENTALS, { body });
  },
  handover(slug: string, uuid: string, body: RentalHandoverRequest) {
    return tenantRequest<Rental>(slug, "POST", `${one(uuid)}/handover`, {
      body,
      silent: true,
    });
  },
  returnVehicle(slug: string, uuid: string, body: RentalReturnRequest) {
    return tenantRequest<Rental>(slug, "POST", `${one(uuid)}/return`, {
      body,
      silent: true,
    });
  },
  completeReturn(
    slug: string,
    uuid: string,
    body: RentalCompleteReturnRequest,
  ) {
    return tenantRequest<Rental>(slug, "POST", `${one(uuid)}/complete-return`, {
      body,
    });
  },
  cancel(slug: string, uuid: string, reason: string) {
    return tenantRequest<Rental>(slug, "POST", `${one(uuid)}/cancel`, {
      body: { reason },
    });
  },
  extend(slug: string, uuid: string, body: RentalExtendRequest) {
    return tenantRequest<Rental>(slug, "POST", `${one(uuid)}/extend`, {
      body,
      silent: true,
    });
  },
  swap(slug: string, uuid: string, body: RentalSwapRequest) {
    return tenantRequest<Rental>(slug, "POST", `${one(uuid)}/swap`, {
      body,
      silent: true,
    });
  },
  addCharge(slug: string, uuid: string, body: RentalChargeRequest) {
    return tenantRequest<Rental>(slug, "POST", `${one(uuid)}/charges`, {
      body,
    });
  },
  decideCharge(
    slug: string,
    uuid: string,
    charge: string,
    decision: "approve" | "waive",
    note: string,
  ) {
    return tenantRequest<Rental>(
      slug,
      "POST",
      `${one(uuid)}/charges/${charge}/${decision}`,
      { body: { note } },
    );
  },

  tasks(slug: string, params: ListOperationTasksParams) {
    return tenantRequest<OperationTaskPage>(slug, "GET", TASKS, {
      query: params,
    });
  },
  tasksMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${TASKS}/meta`);
  },
  taskFacets(slug: string) {
    return tenantRequest<OperationTaskFacets>(slug, "GET", `${TASKS}/facets`);
  },
  taskAction(
    slug: string,
    uuid: string,
    action: "start" | "complete" | "cancel",
    note?: string,
  ) {
    return tenantRequest<OperationTask>(
      slug,
      "POST",
      `${TASKS}/${uuid}/${action}`,
      { body: note !== undefined ? { note } : {} },
    );
  },
  assignTask(slug: string, uuid: string, userUuid: string | null) {
    return tenantRequest<OperationTask>(
      slug,
      "POST",
      `${TASKS}/${uuid}/assign`,
      {
        body: { user_uuid: userUuid },
      },
    );
  },
};
