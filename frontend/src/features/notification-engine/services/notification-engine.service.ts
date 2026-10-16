import type { ResourceMeta } from "@/features/io/types";
import type {
  DeliveryListParams,
  NotificationDelivery,
  NotificationDeliveryFacets,
  NotificationDispatch,
  NotificationEvent,
  NotificationEvents,
  NotificationPreview,
  NotificationPreviewInput,
  NotificationRuleInput,
  NotificationTemplate,
  NotificationTemplateInput,
  NotificationTemplateKey,
  NotificationTestInput,
  Page,
} from "@/features/notification-engine/types";
import { tenantRequest } from "@/lib/api/tenant-request";

/**
 * Notification engine (`/v1/tenant/notification-*`, TEC-733): rules,
 * templates, test sends and the delivery history. Every call names the
 * organization through `tenantRequest` (`X-Organization`).
 */
export const notificationEngineService = {
  events(slug: string) {
    return tenantRequest<NotificationEvents>(
      slug,
      "GET",
      "/v1/tenant/notification-events",
    );
  },
  putRule(slug: string, body: NotificationRuleInput) {
    return tenantRequest<NotificationEvent>(
      slug,
      "PUT",
      "/v1/tenant/notification-rules",
      { body, silent: true },
    );
  },
  resetRule(slug: string, event: string) {
    return tenantRequest<NotificationEvent>(
      slug,
      "POST",
      "/v1/tenant/notification-rules/reset",
      { body: { event } },
    );
  },
  templates(slug: string) {
    return tenantRequest<NotificationTemplate[]>(
      slug,
      "GET",
      "/v1/tenant/notification-templates",
    );
  },
  putTemplate(slug: string, body: NotificationTemplateInput) {
    return tenantRequest<NotificationTemplate>(
      slug,
      "PUT",
      "/v1/tenant/notification-templates",
      { body, silent: true },
    );
  },
  resetTemplate(slug: string, body: NotificationTemplateKey) {
    return tenantRequest<NotificationTemplate>(
      slug,
      "POST",
      "/v1/tenant/notification-templates/reset",
      { body },
    );
  },
  preview(slug: string, body: NotificationPreviewInput) {
    return tenantRequest<NotificationPreview>(
      slug,
      "POST",
      "/v1/tenant/notification-templates/preview",
      { body, silent: true },
    );
  },
  test(slug: string, body: NotificationTestInput) {
    return tenantRequest<NotificationDispatch>(
      slug,
      "POST",
      "/v1/tenant/notification-tests",
      { body, silent: true },
    );
  },
  deliveriesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(
      slug,
      "GET",
      "/v1/tenant/notification-deliveries/meta",
    );
  },
  deliveryFacets(slug: string) {
    return tenantRequest<NotificationDeliveryFacets>(
      slug,
      "GET",
      "/v1/tenant/notification-deliveries/facets",
    );
  },
  deliveries(slug: string, params: DeliveryListParams) {
    return tenantRequest<Page<NotificationDelivery>>(
      slug,
      "GET",
      "/v1/tenant/notification-deliveries",
      { query: params },
    );
  },
  dispatch(slug: string, uuid: string) {
    return tenantRequest<NotificationDispatch>(
      slug,
      "GET",
      `/v1/tenant/notification-dispatches/${uuid}`,
    );
  },
};
