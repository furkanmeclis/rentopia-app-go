import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Supplier = Schemas["Supplier"];
export type SupplierInput = Schemas["SupplierInput"];
export type SupplierContact = Schemas["SupplierContact"];
export type SupplierFacets = Schemas["SupplierFacets"];
export type SupplierType = Supplier["supplier_type"];
export type SupplierDueState = Supplier["due_state"];
export type SupplierInvoice = Schemas["SupplierInvoice"];
export type SupplierInvoiceInput = Schemas["SupplierInvoiceInput"];
export type SupplierInvoiceStatus = SupplierInvoice["status"];
export type InvoiceDueState = SupplierInvoice["due_state"];
export type SupplierPayment = Schemas["SupplierPayment"];
export type SupplierPaymentInput = Schemas["SupplierPaymentInput"];
export type SupplierPaymentMethod = SupplierPayment["method"];
export type PayablesSummary = Schemas["PayablesSummary"];
export type SupplierActivityEntry = Schemas["BranchActivityEntry"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** Query of the list endpoints (comma-joined multi-value filters). */
export type ListParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  [filter: string]: string | number | undefined;
};
