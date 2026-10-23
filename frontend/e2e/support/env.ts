/**
 * Addresses of the acceptance stack (frontend/e2e/stack/env.sh holds the
 * same defaults). The Go API is called directly only by the Node-side
 * setup and the RT-A27 public-API comparison; the browser always uses the
 * panel's same-origin /api/v1 BFF.
 */
export const e2eEnv = {
  apiPort: Number(process.env.E2E_API_PORT ?? 18080),
  panelPort: Number(process.env.E2E_PANEL_PORT ?? 13000),
  sitePort: Number(process.env.E2E_SITE_PORT ?? 13100),
  adminEmail: process.env.E2E_ADMIN_EMAIL ?? "kabul-admin@rentopya.test",
  password: process.env.E2E_PASSWORD ?? "Kabul2026Test",
  /** Partner page that embeds the widget (served by page.route, no server). */
  partnerOrigin: process.env.E2E_PARTNER_ORIGIN ?? "http://partner.localhost:13200",
};

export const apiBase = () => `http://127.0.0.1:${e2eEnv.apiPort}`;
export const panelBase = () => `http://localhost:${e2eEnv.panelPort}`;
export const siteBase = (subdomain: string) =>
  `http://${subdomain}.localhost:${e2eEnv.sitePort}`;
