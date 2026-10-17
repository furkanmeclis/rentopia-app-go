import type {
  MessageListParams,
  Page,
  WhatsAppConnectInput,
  WhatsAppMessage,
  WhatsAppQR,
  WhatsAppSession,
  WhatsAppSettingsInput,
  WhatsAppTestInput,
} from "@/features/whatsapp/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/whatsapp";

/**
 * WhatsApp channel (`/v1/tenant/whatsapp/*`, TEC-735): the organization's
 * session (connect, QR, state, logout, remove, settings), test sends and
 * the message log. Every call names the organization (`X-Organization`).
 */
export const whatsappService = {
  session(slug: string) {
    return tenantRequest<WhatsAppSession>(slug, "GET", `${BASE}/session`);
  },
  connect(slug: string, body: WhatsAppConnectInput) {
    return tenantRequest<WhatsAppSession>(
      slug,
      "POST",
      `${BASE}/session/connect`,
      { body, silent: true },
    );
  },
  qr(slug: string) {
    return tenantRequest<WhatsAppQR>(slug, "GET", `${BASE}/session/qr`, {
      silent: true,
    });
  },
  refresh(slug: string) {
    return tenantRequest<WhatsAppSession>(
      slug,
      "POST",
      `${BASE}/session/refresh`,
    );
  },
  logout(slug: string) {
    return tenantRequest<WhatsAppSession>(
      slug,
      "POST",
      `${BASE}/session/logout`,
    );
  },
  remove(slug: string) {
    return tenantRequest<WhatsAppSession>(slug, "DELETE", `${BASE}/session`);
  },
  updateSettings(slug: string, body: WhatsAppSettingsInput) {
    return tenantRequest<WhatsAppSession>(slug, "PATCH", `${BASE}/session`, {
      body,
      silent: true,
    });
  },
  test(slug: string, body: WhatsAppTestInput) {
    return tenantRequest<WhatsAppMessage>(
      slug,
      "POST",
      `${BASE}/test-messages`,
      { body, silent: true },
    );
  },
  messages(slug: string, params: MessageListParams) {
    return tenantRequest<Page<WhatsAppMessage>>(
      slug,
      "GET",
      `${BASE}/messages`,
      { query: params },
    );
  },
};
