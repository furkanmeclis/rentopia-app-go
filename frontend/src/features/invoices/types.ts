import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Invoice = Schemas["Invoice"];
export type InvoiceDetail = Schemas["InvoiceDetail"];
export type InvoicePage = Schemas["EnvelopeInvoicePage"]["data"];
export type InvoiceFacets = Schemas["InvoiceFacets"];
export type InvoiceStatus = Schemas["InvoiceStatus"];
export type InvoiceProfile = Schemas["InvoiceProfile"];
export type InvoiceLineKind = Schemas["InvoiceLineKind"];
export type InvoiceLine = Schemas["InvoiceLine"];
export type InvoiceLink = Schemas["InvoiceLink"];
export type InvoiceProblem = Schemas["InvoiceProblem"];
export type InvoiceItem = Schemas["InvoiceItem"];
export type UninvoicedRental = Schemas["UninvoicedRental"];
export type UninvoicedRentalPage =
  Schemas["EnvelopeUninvoicedRentalPage"]["data"];
export type InvoiceSettings = Schemas["InvoiceSettings"];
export type InvoiceTaxMapping = Schemas["InvoiceTaxMapping"];
export type InvoiceSettingsInput = Schemas["InvoiceSettingsRequest"];
export type InvoiceCreateInput = Schemas["InvoiceCreateRequest"];
export type InvoicePatchInput = Schemas["InvoicePatchRequest"];
export type InvoiceReceiverInput = Schemas["InvoiceReceiverInput"];
export type InvoiceValidation = Schemas["EnvelopeInvoiceValidation"]["data"];
export type InvoiceRetryResult = Schemas["EnvelopeInvoiceRetry"]["data"];
export type PortalInvoice = Schemas["PortalInvoice"];

/** List query of `GET /v1/tenant/invoices` (multi-values comma-joined). */
export type ListInvoicesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  profile?: string;
  invoice_type?: string;
  branch?: string;
  account?: string;
  rental?: string;
  from?: string;
  to?: string;
  needs_action?: string;
};
