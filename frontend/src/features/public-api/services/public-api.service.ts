import type {
  IssuedPublicApiKey,
  IssuedPublicWebhook,
  ListDeliveriesParams,
  ListKeysParams,
  ListWebhooksParams,
  PublicApiKey,
  PublicApiKeyCreateRequest,
  PublicApiKeyPage,
  PublicApiKeyPatchRequest,
  PublicApiOverview,
  PublicWebhook,
  PublicWebhookCreateRequest,
  PublicWebhookDelivery,
  PublicWebhookDeliveryPage,
  PublicWebhookPage,
  PublicWebhookPatchRequest,
} from "@/features/public-api/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/public-api";

/**
 * Public booking API settings (TEC-728): keys, webhook endpoints and their
 * deliveries (`tenantRequest` only; same-origin BFF). Form calls are
 * `silent`: the forms render their own errors.
 */
export const publicApiService = {
  overview(slug: string) {
    return tenantRequest<PublicApiOverview>(slug, "GET", `${BASE}/overview`);
  },
  keys(slug: string, params: ListKeysParams) {
    return tenantRequest<PublicApiKeyPage>(slug, "GET", `${BASE}/keys`, {
      query: params,
    });
  },
  createKey(slug: string, body: PublicApiKeyCreateRequest) {
    return tenantRequest<IssuedPublicApiKey>(slug, "POST", `${BASE}/keys`, {
      body,
      silent: true,
    });
  },
  patchKey(slug: string, uuid: string, body: PublicApiKeyPatchRequest) {
    return tenantRequest<PublicApiKey>(slug, "PATCH", `${BASE}/keys/${uuid}`, {
      body,
      silent: true,
    });
  },
  rotateKey(slug: string, uuid: string, graceHours: number) {
    return tenantRequest<IssuedPublicApiKey>(
      slug,
      "POST",
      `${BASE}/keys/${uuid}/rotate`,
      { body: { grace_hours: graceHours }, silent: true },
    );
  },
  revokeKey(slug: string, uuid: string) {
    return tenantRequest<PublicApiKey>(
      slug,
      "POST",
      `${BASE}/keys/${uuid}/revoke`,
      { body: {} },
    );
  },
  webhooks(slug: string, params: ListWebhooksParams) {
    return tenantRequest<PublicWebhookPage>(slug, "GET", `${BASE}/webhooks`, {
      query: params,
    });
  },
  createWebhook(slug: string, body: PublicWebhookCreateRequest) {
    return tenantRequest<IssuedPublicWebhook>(
      slug,
      "POST",
      `${BASE}/webhooks`,
      { body, silent: true },
    );
  },
  patchWebhook(slug: string, uuid: string, body: PublicWebhookPatchRequest) {
    return tenantRequest<PublicWebhook>(
      slug,
      "PATCH",
      `${BASE}/webhooks/${uuid}`,
      { body, silent: true },
    );
  },
  deleteWebhook(slug: string, uuid: string) {
    return tenantRequest<void>(slug, "DELETE", `${BASE}/webhooks/${uuid}`);
  },
  rotateWebhookSecret(slug: string, uuid: string) {
    return tenantRequest<IssuedPublicWebhook>(
      slug,
      "POST",
      `${BASE}/webhooks/${uuid}/rotate-secret`,
      { body: {} },
    );
  },
  testWebhook(slug: string, uuid: string) {
    return tenantRequest<PublicWebhookDelivery>(
      slug,
      "POST",
      `${BASE}/webhooks/${uuid}/test`,
      { body: {} },
    );
  },
  deliveries(slug: string, params: ListDeliveriesParams) {
    return tenantRequest<PublicWebhookDeliveryPage>(
      slug,
      "GET",
      `${BASE}/webhook-deliveries`,
      { query: params },
    );
  },
  delivery(slug: string, uuid: string) {
    return tenantRequest<PublicWebhookDelivery>(
      slug,
      "GET",
      `${BASE}/webhook-deliveries/${uuid}`,
    );
  },
  retryDelivery(slug: string, uuid: string) {
    return tenantRequest<PublicWebhookDelivery>(
      slug,
      "POST",
      `${BASE}/webhook-deliveries/${uuid}/retry`,
      { body: {} },
    );
  },
};
