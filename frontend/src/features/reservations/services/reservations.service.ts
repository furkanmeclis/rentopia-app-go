import type { ResourceMeta } from "@/features/io/types";
import type {
  ListQuotesParams,
  ListReservationsParams,
  ListTasksParams,
  Quote,
  QuotePage,
  QuoteRequest,
  QuoteSearchRequest,
  QuoteSearchResult,
  Reservation,
  ReservationApplyChangeRequest,
  ReservationChangeQuoteRequest,
  ReservationChangeResult,
  ReservationCreateRequest,
  ReservationDriversRequest,
  ReservationEventPage,
  ReservationFacets,
  ReservationPage,
  ReservationPlateRequest,
  ReservationVehicleRef,
  ResolutionTask,
  ResolutionTaskPage,
  ResolutionTaskReplan,
} from "@/features/reservations/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const RESERVATIONS = "/v1/tenant/reservations";
const QUOTES = "/v1/tenant/quotes";
const TASKS = "/v1/tenant/resolution-tasks";
const one = (uuid: string) => `${RESERVATIONS}/${uuid}`;

type PageParams = { limit: number; offset: number };

/**
 * Reservations, quotes and resolution tasks (`tenantRequest` only). Calls
 * whose errors the screens render themselves (quote, create, change,
 * extend) are `silent`: no global toast.
 */
export const reservationsService = {
  list(slug: string, params: ListReservationsParams) {
    return tenantRequest<ReservationPage>(slug, "GET", RESERVATIONS, {
      query: params,
    });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${RESERVATIONS}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<ReservationFacets>(
      slug,
      "GET",
      `${RESERVATIONS}/facets`,
    );
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Reservation>(slug, "GET", one(uuid));
  },
  events(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<ReservationEventPage>(
      slug,
      "GET",
      `${one(uuid)}/events`,
      { query: params },
    );
  },
  create(slug: string, body: ReservationCreateRequest) {
    return tenantRequest<Reservation>(slug, "POST", RESERVATIONS, {
      body,
      silent: true,
    });
  },
  transition(
    slug: string,
    uuid: string,
    action: "submit" | "approve" | "confirm" | "cancel" | "no-show",
    reason?: string,
  ) {
    return tenantRequest<Reservation>(slug, "POST", `${one(uuid)}/${action}`, {
      body: reason !== undefined ? { reason } : {},
    });
  },
  /** Purpose (change / extension / upgrade) is derived by the API. */
  changeQuote(slug: string, uuid: string, body: ReservationChangeQuoteRequest) {
    return tenantRequest<Quote>(slug, "POST", `${one(uuid)}/quotes`, {
      body,
      silent: true,
    });
  },
  applyChange(slug: string, uuid: string, body: ReservationApplyChangeRequest) {
    return tenantRequest<ReservationChangeResult>(
      slug,
      "POST",
      `${one(uuid)}/change`,
      { body, silent: true },
    );
  },
  extend(
    slug: string,
    uuid: string,
    body: { ends_at: string; reason?: string },
  ) {
    return tenantRequest<ReservationChangeResult>(
      slug,
      "POST",
      `${one(uuid)}/extend`,
      { body, silent: true },
    );
  },
  plateCandidates(slug: string, uuid: string) {
    return tenantRequest<{ items: ReservationVehicleRef[] }>(
      slug,
      "GET",
      `${one(uuid)}/plate-candidates`,
    );
  },
  plate(slug: string, uuid: string, body: ReservationPlateRequest) {
    return tenantRequest<ReservationChangeResult>(
      slug,
      "POST",
      `${one(uuid)}/plate`,
      { body },
    );
  },
  drivers(slug: string, uuid: string, body: ReservationDriversRequest) {
    return tenantRequest<Reservation>(slug, "PUT", `${one(uuid)}/drivers`, {
      body,
      silent: true,
    });
  },
  assign(slug: string, uuid: string, userUuid: string | null) {
    return tenantRequest<Reservation>(slug, "POST", `${one(uuid)}/assign`, {
      body: { user_uuid: userUuid },
    });
  },

  // ---------------------------------------------------------------- quotes
  quotes(slug: string, params: ListQuotesParams) {
    return tenantRequest<QuotePage>(slug, "GET", QUOTES, { query: params });
  },
  quotesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${QUOTES}/meta`);
  },
  quote(slug: string, uuid: string) {
    return tenantRequest<Quote>(slug, "GET", `${QUOTES}/${uuid}`);
  },
  createQuote(slug: string, body: QuoteRequest) {
    return tenantRequest<Quote>(slug, "POST", QUOTES, { body, silent: true });
  },
  search(slug: string, body: QuoteSearchRequest) {
    return tenantRequest<QuoteSearchResult>(slug, "POST", `${QUOTES}/search`, {
      body,
    });
  },

  // ----------------------------------------------------------------- tasks
  tasks(slug: string, params: ListTasksParams) {
    return tenantRequest<ResolutionTaskPage>(slug, "GET", TASKS, {
      query: params,
    });
  },
  tasksMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${TASKS}/meta`);
  },
  resolveTask(slug: string, uuid: string, note: string) {
    return tenantRequest<ResolutionTask>(
      slug,
      "POST",
      `${TASKS}/${uuid}/resolve`,
      { body: { note } },
    );
  },
  assignTask(slug: string, uuid: string, userUuid: string | null) {
    return tenantRequest<ResolutionTask>(
      slug,
      "POST",
      `${TASKS}/${uuid}/assign`,
      { body: { user_uuid: userUuid } },
    );
  },
  replanTask(slug: string, uuid: string) {
    return tenantRequest<ResolutionTaskReplan>(
      slug,
      "POST",
      `${TASKS}/${uuid}/replan`,
    );
  },
};
