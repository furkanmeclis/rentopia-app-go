import type { ResourceMeta } from "@/features/io/types";
import type {
  ListParams,
  Page,
  PayablesSummary,
  Supplier,
  SupplierActivityEntry,
  SupplierFacets,
  SupplierInput,
  SupplierInvoice,
  SupplierInvoiceInput,
  SupplierPayment,
  SupplierPaymentInput,
} from "@/features/suppliers/types";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

const SUPPLIERS = "/v1/tenant/suppliers";
const INVOICES = "/v1/tenant/finance/supplier-invoices";
const PAYMENTS = "/v1/tenant/finance/supplier-payments";

/**
 * Supplier cards (`/v1/tenant/suppliers`) and their invoices / payments
 * (`/v1/tenant/finance/supplier-*`); every call names the organization
 * through `tenantRequest` (`X-Organization`).
 */
export const suppliersService = {
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${SUPPLIERS}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<SupplierFacets>(slug, "GET", `${SUPPLIERS}/facets`);
  },
  list(slug: string, params: ListParams) {
    return tenantRequest<Page<Supplier>>(slug, "GET", SUPPLIERS, {
      query: params,
    });
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Supplier>(slug, "GET", `${SUPPLIERS}/${uuid}`);
  },
  create(slug: string, body: SupplierInput) {
    return tenantRequest<Supplier>(slug, "POST", SUPPLIERS, {
      body,
      silent: true,
    });
  },
  patch(slug: string, uuid: string, body: SupplierInput) {
    return tenantRequest<Supplier>(slug, "PATCH", `${SUPPLIERS}/${uuid}`, {
      body,
      silent: true,
    });
  },
  activity(slug: string, uuid: string, params: ListParams) {
    return tenantRequest<Page<SupplierActivityEntry>>(
      slug,
      "GET",
      `${SUPPLIERS}/${uuid}/activity`,
      { query: params },
    );
  },

  payablesSummary(slug: string, window?: number) {
    return tenantRequest<PayablesSummary>(
      slug,
      "GET",
      "/v1/tenant/finance/payables/summary",
      { query: { window } },
    );
  },

  invoicesMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${INVOICES}/meta`);
  },
  invoices(slug: string, params: ListParams) {
    return tenantRequest<Page<SupplierInvoice>>(slug, "GET", INVOICES, {
      query: params,
    });
  },
  invoice(slug: string, uuid: string) {
    return tenantRequest<SupplierInvoice>(slug, "GET", `${INVOICES}/${uuid}`);
  },
  createInvoice(slug: string, body: SupplierInvoiceInput) {
    return tenantRequest<SupplierInvoice>(slug, "POST", INVOICES, {
      body,
      silent: true,
    });
  },
  patchInvoice(slug: string, uuid: string, body: SupplierInvoiceInput) {
    return tenantRequest<SupplierInvoice>(
      slug,
      "PATCH",
      `${INVOICES}/${uuid}`,
      { body, silent: true },
    );
  },
  voidInvoice(slug: string, uuid: string, reason: string) {
    return tenantRequest<SupplierInvoice>(
      slug,
      "POST",
      `${INVOICES}/${uuid}/void`,
      { body: { reason }, silent: true },
    );
  },
  uploadInvoiceFile(slug: string, uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<SupplierInvoice>(
      slug,
      "PUT",
      `${INVOICES}/${uuid}/file`,
      form,
    );
  },
  downloadInvoiceFile(slug: string, uuid: string) {
    return tenantDownloadFile(slug, `${INVOICES}/${uuid}/file`);
  },
  removeInvoiceFile(slug: string, uuid: string) {
    return tenantRequest<SupplierInvoice>(
      slug,
      "DELETE",
      `${INVOICES}/${uuid}/file`,
    );
  },

  paymentsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${PAYMENTS}/meta`);
  },
  payments(slug: string, params: ListParams) {
    return tenantRequest<Page<SupplierPayment>>(slug, "GET", PAYMENTS, {
      query: params,
    });
  },
  createPayment(slug: string, body: SupplierPaymentInput) {
    return tenantRequest<SupplierPayment>(slug, "POST", PAYMENTS, {
      body,
      silent: true,
    });
  },
  voidPayment(slug: string, uuid: string, reason: string) {
    return tenantRequest<SupplierPayment>(
      slug,
      "POST",
      `${PAYMENTS}/${uuid}/void`,
      { body: { reason }, silent: true },
    );
  },
};
