import { apiConfig } from "@/config/api";
import type {
  CustomerRequestPage,
  CustomerRequestResolve,
  ListCustomerRequestsParams,
  PortalInfo,
  PortalPayResult,
  PortalRequest,
  PortalRequestCreate,
  PortalReservation,
  PortalReservationSummary,
  PortalSession,
  PortalSessionIssued,
  PortalStartRequest,
  PortalStartResult,
  PortalVerifyRequest,
  StaffCustomerRequest,
} from "@/features/customer-portal/types";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantRequest } from "@/lib/api/tenant-request";

const base = (orgSlug: string) => `/v1/customer/${encodeURIComponent(orgSlug)}`;

/**
 * Customer portal (`/v1/customer/{orgSlug}/*`, TEC-715). The BFF keeps the
 * customer session in an HttpOnly cookie; the browser never holds a token.
 * Calls are silent: the pages render their own (generic) errors.
 */
export const portalService = {
  info(orgSlug: string) {
    return platformRequest<PortalInfo>("GET", base(orgSlug), { silent: true });
  },
  start(orgSlug: string, body: PortalStartRequest) {
    return platformRequest<PortalStartResult>(
      "POST",
      `${base(orgSlug)}/auth/start`,
      { body, silent: true },
    );
  },
  verify(orgSlug: string, body: PortalVerifyRequest) {
    return platformRequest<PortalSessionIssued>(
      "POST",
      `${base(orgSlug)}/auth/verify`,
      { body, silent: true },
    );
  },
  logout(orgSlug: string) {
    return platformRequest<{ signed_out: boolean }>(
      "POST",
      `${base(orgSlug)}/auth/logout`,
      { body: {}, silent: true },
    );
  },
  session(orgSlug: string) {
    return platformRequest<PortalSession>("GET", `${base(orgSlug)}/session`, {
      silent: true,
    });
  },
  reservations(orgSlug: string) {
    return platformRequest<{ items: PortalReservationSummary[] }>(
      "GET",
      `${base(orgSlug)}/reservations`,
      { silent: true },
    );
  },
  reservation(orgSlug: string, uuid: string) {
    return platformRequest<PortalReservation>(
      "GET",
      `${base(orgSlug)}/reservations/${uuid}`,
      { silent: true },
    );
  },
  pay(orgSlug: string, uuid: string) {
    return platformRequest<PortalPayResult>(
      "POST",
      `${base(orgSlug)}/reservations/${uuid}/pay`,
      { body: {}, silent: true },
    );
  },
  createRequest(orgSlug: string, uuid: string, body: PortalRequestCreate) {
    return platformRequest<PortalRequest>(
      "POST",
      `${base(orgSlug)}/reservations/${uuid}/requests`,
      { body, silent: true },
    );
  },
  /** Same-origin BFF URL of an API download path (`download_path`). */
  downloadUrl(path: string) {
    return `${apiConfig.baseUrl.replace(/\/$/, "")}/v1/${path.replace(/^\//, "")}`;
  },
};

const REQUESTS = "/v1/tenant/customer-requests";

/** Staff side: customer requests (`tenantRequest` only). */
export const customerRequestsService = {
  list(slug: string, params: ListCustomerRequestsParams) {
    return tenantRequest<CustomerRequestPage>(slug, "GET", REQUESTS, {
      query: params,
    });
  },
  stats(slug: string) {
    return tenantRequest<{ open: number }>(slug, "GET", `${REQUESTS}/stats`);
  },
  resolve(slug: string, uuid: string, body: CustomerRequestResolve) {
    return tenantRequest<StaffCustomerRequest>(
      slug,
      "POST",
      `${REQUESTS}/${uuid}/resolve`,
      { body },
    );
  },
};
