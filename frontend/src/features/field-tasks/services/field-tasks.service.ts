import type { ResourceMeta } from "@/features/io/types";
import type {
  DeliveryPoint,
  FieldTask,
  FieldTaskAssignee,
  FieldTaskCreateRequest,
  FieldTaskEvent,
  FieldTaskFacets,
  FieldTaskPage,
  FieldTaskPatchRequest,
  ListFieldTasksParams,
} from "@/features/field-tasks/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const TASKS = "/v1/tenant/field-tasks";
const one = (uuid: string) => `${TASKS}/${uuid}`;

export type FieldTaskStep = "start" | "report-issue" | "complete" | "cancel";

/**
 * Field tasks (`tenantRequest` only). Actions whose errors the screens show
 * inline are `silent`.
 */
export const fieldTasksService = {
  list(slug: string, params: ListFieldTasksParams) {
    return tenantRequest<FieldTaskPage>(slug, "GET", TASKS, { query: params });
  },
  mine(slug: string, params: ListFieldTasksParams) {
    return tenantRequest<FieldTaskPage>(slug, "GET", `${TASKS}/mine`, {
      query: params,
    });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${TASKS}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<FieldTaskFacets>(slug, "GET", `${TASKS}/facets`);
  },
  assignees(slug: string, q?: string) {
    return tenantRequest<FieldTaskAssignee[]>(
      slug,
      "GET",
      `${TASKS}/assignees`,
      { query: { q } },
    );
  },
  get(slug: string, uuid: string) {
    return tenantRequest<FieldTask>(slug, "GET", one(uuid));
  },
  events(slug: string, uuid: string) {
    return tenantRequest<FieldTaskEvent[]>(slug, "GET", `${one(uuid)}/events`);
  },
  create(slug: string, body: FieldTaskCreateRequest) {
    return tenantRequest<FieldTask>(slug, "POST", TASKS, {
      body,
      silent: true,
    });
  },
  patch(slug: string, uuid: string, body: FieldTaskPatchRequest) {
    return tenantRequest<FieldTask>(slug, "PATCH", one(uuid), {
      body,
      silent: true,
    });
  },
  assign(slug: string, uuid: string, userUuid: string | null) {
    return tenantRequest<FieldTask>(slug, "POST", `${one(uuid)}/assign`, {
      body: { user_uuid: userUuid },
      silent: true,
    });
  },
  step(slug: string, uuid: string, step: FieldTaskStep, note?: string) {
    return tenantRequest<FieldTask>(slug, "POST", `${one(uuid)}/${step}`, {
      body: step === "start" ? undefined : { note: note ?? "" },
      silent: true,
    });
  },
  deliveryPoints(slug: string, branch?: string, product?: string) {
    return tenantRequest<DeliveryPoint[]>(
      slug,
      "GET",
      "/v1/tenant/delivery-points",
      { query: { branch, product } },
    );
  },
};
