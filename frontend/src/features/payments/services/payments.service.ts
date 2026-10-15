import type {
  ConnectIntegrationRequest,
  IntegrationAccount,
  IntegrationAccountPage,
  IntegrationJobDetail,
  IntegrationJobPage,
  IntegrationJobRetry,
  ListIntegrationJobsParams,
  SmsSent,
  IntegrationsMeta,
  ListIntegrationsParams,
  ListLinksParams,
  ListPaymentsParams,
  Payment,
  PaymentLink,
  PaymentLinkPage,
  PaymentPage,
  PublicPayLink,
  ReservationAcceptResult,
  UpdateIntegrationRequest,
} from "@/features/payments/types";
import type { Reservation } from "@/features/reservations/types";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantRequest } from "@/lib/api/tenant-request";

const PAYMENTS = "/v1/tenant/payments";
const LINKS = "/v1/tenant/payment-links";
const INTEGRATIONS = "/v1/tenant/integrations";
const RESERVATIONS = "/v1/tenant/reservations";
const INTEGRATION_JOBS = "/v1/tenant/integration-jobs";

/**
 * Payments, pay-by-link links, provider accounts and the staff approval
 * actions (`tenantRequest` only; same-origin BFF). Calls whose errors the
 * dialogs render themselves are `silent`.
 */
export const paymentsService = {
  list(slug: string, params: ListPaymentsParams) {
    return tenantRequest<PaymentPage>(slug, "GET", PAYMENTS, { query: params });
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Payment>(slug, "GET", `${PAYMENTS}/${uuid}`);
  },
  verify(slug: string, uuid: string) {
    return tenantRequest<Payment>(slug, "POST", `${PAYMENTS}/${uuid}/verify`, {
      body: {},
    });
  },
  refund(
    slug: string,
    uuid: string,
    body: { amount?: string; reason: string },
  ) {
    return tenantRequest<Payment>(slug, "POST", `${PAYMENTS}/${uuid}/refund`, {
      body,
      silent: true,
    });
  },
  links(slug: string, params: ListLinksParams) {
    return tenantRequest<PaymentLinkPage>(slug, "GET", LINKS, {
      query: params,
    });
  },
  cancelLink(slug: string, uuid: string) {
    return tenantRequest<PaymentLink>(slug, "POST", `${LINKS}/${uuid}/cancel`, {
      body: {},
    });
  },
  createLink(
    slug: string,
    reservationUuid: string,
    body: { amount?: string; expires_in_minutes?: number },
  ) {
    return tenantRequest<PaymentLink>(
      slug,
      "POST",
      `${RESERVATIONS}/${reservationUuid}/payment-links`,
      { body, silent: true },
    );
  },
  collect(
    slug: string,
    reservationUuid: string,
    body: { amount?: string; method: string; note?: string },
  ) {
    return tenantRequest<Payment>(
      slug,
      "POST",
      `${RESERVATIONS}/${reservationUuid}/collections`,
      { body, silent: true },
    );
  },
  accept(
    slug: string,
    reservationUuid: string,
    body: { quote_uuid?: string; send_payment_link?: boolean; note?: string },
  ) {
    return tenantRequest<ReservationAcceptResult>(
      slug,
      "POST",
      `${RESERVATIONS}/${reservationUuid}/accept`,
      { body, silent: true },
    );
  },
  reject(slug: string, reservationUuid: string, reason: string) {
    return tenantRequest<Reservation>(
      slug,
      "POST",
      `${RESERVATIONS}/${reservationUuid}/reject`,
      { body: { reason } },
    );
  },

  integrationsMeta(slug: string) {
    return tenantRequest<IntegrationsMeta>(slug, "GET", `${INTEGRATIONS}/meta`);
  },
  integrations(slug: string, params: ListIntegrationsParams) {
    return tenantRequest<IntegrationAccountPage>(slug, "GET", INTEGRATIONS, {
      query: params,
    });
  },
  connect(slug: string, body: ConnectIntegrationRequest) {
    return tenantRequest<IntegrationAccount>(slug, "POST", INTEGRATIONS, {
      body,
      silent: true,
    });
  },
  updateIntegration(
    slug: string,
    uuid: string,
    body: UpdateIntegrationRequest,
  ) {
    return tenantRequest<IntegrationAccount>(
      slug,
      "PATCH",
      `${INTEGRATIONS}/${uuid}`,
      { body, silent: true },
    );
  },
  testIntegration(slug: string, uuid: string) {
    return tenantRequest<IntegrationAccount>(
      slug,
      "POST",
      `${INTEGRATIONS}/${uuid}/test`,
      { body: {}, silent: true },
    );
  },
  setIntegrationStatus(
    slug: string,
    uuid: string,
    action: "enable" | "disable",
  ) {
    return tenantRequest<IntegrationAccount>(
      slug,
      "POST",
      `${INTEGRATIONS}/${uuid}/${action}`,
      { body: {} },
    );
  },
  integrationJobs(
    slug: string,
    uuid: string,
    params: ListIntegrationJobsParams,
  ) {
    return tenantRequest<IntegrationJobPage>(
      slug,
      "GET",
      `${INTEGRATIONS}/${uuid}/jobs`,
      { query: params },
    );
  },
  integrationJob(slug: string, uuid: string) {
    return tenantRequest<IntegrationJobDetail>(
      slug,
      "GET",
      `${INTEGRATION_JOBS}/${uuid}`,
    );
  },
  /** CheckStatus first; resubmits only when the provider confirms "not done". */
  retryIntegrationJob(slug: string, uuid: string) {
    return tenantRequest<IntegrationJobRetry>(
      slug,
      "POST",
      `${INTEGRATION_JOBS}/${uuid}/retry`,
      { body: {}, silent: true },
    );
  },
  testSms(slug: string, uuid: string, phone: string) {
    return tenantRequest<SmsSent>(
      slug,
      "POST",
      `${INTEGRATIONS}/${uuid}/test-sms`,
      { body: { phone }, silent: true },
    );
  },
};

/** Public pay-by-link page (no session; the token is the bearer). */
export const publicPayService = {
  get(token: string, verify: boolean) {
    return platformRequest<PublicPayLink>(
      "GET",
      `/v1/public/pay/${encodeURIComponent(token)}`,
      { query: verify ? { verify: "1" } : undefined, silent: true },
    );
  },
  checkout(token: string, locale: string) {
    return platformRequest<{ payment_page_url: string }>(
      "POST",
      `/v1/public/pay/${encodeURIComponent(token)}/checkout`,
      { body: { locale }, silent: true },
    );
  },
};
