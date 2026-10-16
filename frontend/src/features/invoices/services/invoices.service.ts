import type { ResourceMeta } from "@/features/io/types";
import type {
  InvoiceCreateInput,
  InvoiceDetail,
  InvoiceFacets,
  InvoiceItem,
  InvoicePage,
  InvoicePatchInput,
  InvoiceRetryResult,
  InvoiceSettings,
  InvoiceSettingsInput,
  InvoiceValidation,
  ListInvoicesParams,
  UninvoicedRentalPage,
} from "@/features/invoices/types";
import { tenantDownloadFile, tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/invoices";
const one = (uuid: string) => `${BASE}/${uuid}`;

/**
 * Invoices (`tenantRequest` only). Actions whose errors the screens render
 * themselves are `silent`.
 */
export const invoicesService = {
  list(slug: string, params: ListInvoicesParams) {
    return tenantRequest<InvoicePage>(slug, "GET", BASE, { query: params });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<InvoiceFacets>(slug, "GET", `${BASE}/facets`);
  },
  items(slug: string, query: { account?: string; rental?: string }) {
    return tenantRequest<{ items: InvoiceItem[] }>(
      slug,
      "GET",
      `${BASE}/items`,
      { query },
    );
  },
  uninvoicedRentals(
    slug: string,
    query: { limit: number; offset: number; q?: string },
  ) {
    return tenantRequest<UninvoicedRentalPage>(
      slug,
      "GET",
      `${BASE}/uninvoiced-rentals`,
      { query },
    );
  },
  get(slug: string, uuid: string) {
    return tenantRequest<InvoiceDetail>(slug, "GET", one(uuid));
  },
  create(slug: string, body: InvoiceCreateInput) {
    return tenantRequest<InvoiceDetail>(slug, "POST", BASE, {
      body,
      silent: true,
    });
  },
  patch(slug: string, uuid: string, body: InvoicePatchInput) {
    return tenantRequest<InvoiceDetail>(slug, "PATCH", one(uuid), {
      body,
      silent: true,
    });
  },
  validate(slug: string, uuid: string) {
    return tenantRequest<InvoiceValidation>(
      slug,
      "POST",
      `${one(uuid)}/validate`,
      { silent: true },
    );
  },
  finalize(slug: string, uuid: string) {
    return tenantRequest<InvoiceDetail>(slug, "POST", `${one(uuid)}/finalize`, {
      silent: true,
    });
  },
  retry(slug: string, uuid: string) {
    return tenantRequest<InvoiceRetryResult>(
      slug,
      "POST",
      `${one(uuid)}/retry`,
      { silent: true },
    );
  },
  cancel(slug: string, uuid: string, reason: string) {
    return tenantRequest<InvoiceDetail>(slug, "POST", `${one(uuid)}/cancel`, {
      body: { reason },
      silent: true,
    });
  },
  returnInvoice(
    slug: string,
    uuid: string,
    body: { reason: string; issue_date?: string | null },
  ) {
    return tenantRequest<InvoiceDetail>(slug, "POST", `${one(uuid)}/return`, {
      body,
      silent: true,
    });
  },
  /** The go-ubltr HTML render as text (shown in a sandboxed frame). */
  async preview(slug: string, uuid: string) {
    const { blob } = await tenantDownloadFile(slug, `${one(uuid)}/preview`);
    return blob.text();
  },
  download(slug: string, uuid: string, format: "xml" | "pdf") {
    return tenantDownloadFile(slug, `${one(uuid)}/${format}`);
  },
  settings(slug: string) {
    return tenantRequest<InvoiceSettings>(slug, "GET", `${BASE}/settings`);
  },
  saveSettings(slug: string, body: InvoiceSettingsInput) {
    return tenantRequest<InvoiceSettings>(slug, "PUT", `${BASE}/settings`, {
      body,
      silent: true,
    });
  },
};
