import type {
  InvoiceDueState,
  SupplierDueState,
  SupplierInvoiceStatus,
  SupplierPaymentMethod,
  SupplierType,
} from "@/features/suppliers/types";

/** Engine / live list resources. */
export const SUPPLIERS_RESOURCE = "tenant.suppliers";
export const SUPPLIER_INVOICES_RESOURCE = "tenant.supplier_invoices";
export const SUPPLIER_PAYMENTS_RESOURCE = "tenant.supplier_payments";

type Tone = "default" | "success" | "warning" | "danger";

export const SUPPLIER_TYPES: SupplierType[] = [
  "service",
  "insurance",
  "vehicle_supplier",
  "parts",
  "fuel",
  "other",
];

export const SUPPLIER_TYPE_LABEL_KEYS: Record<SupplierType, string> = {
  service: "suppliers.type.service",
  insurance: "suppliers.type.insurance",
  vehicle_supplier: "suppliers.type.vehicle_supplier",
  parts: "suppliers.type.parts",
  fuel: "suppliers.type.fuel",
  other: "suppliers.type.other",
};

export const SUPPLIER_STATUS_LABEL_KEYS = {
  active: "suppliers.status.active",
  archived: "suppliers.status.archived",
} as const;

export const SUPPLIER_DUE_STATES: SupplierDueState[] = [
  "overdue",
  "due_soon",
  "open",
  "settled",
];

export const SUPPLIER_DUE_LABEL_KEYS: Record<SupplierDueState, string> = {
  overdue: "suppliers.due_state.overdue",
  due_soon: "suppliers.due_state.due_soon",
  open: "suppliers.due_state.open",
  settled: "suppliers.due_state.settled",
};

export const SUPPLIER_DUE_TONES: Record<SupplierDueState, Tone> = {
  overdue: "danger",
  due_soon: "warning",
  open: "default",
  settled: "success",
};

export const INVOICE_STATUSES: SupplierInvoiceStatus[] = [
  "open",
  "partially_paid",
  "paid",
  "void",
];

export const INVOICE_STATUS_LABEL_KEYS: Record<SupplierInvoiceStatus, string> =
  {
    open: "suppliers.invoice_status.open",
    partially_paid: "suppliers.invoice_status.partially_paid",
    paid: "suppliers.invoice_status.paid",
    void: "suppliers.invoice_status.void",
  };

export const INVOICE_STATUS_TONES: Record<SupplierInvoiceStatus, Tone> = {
  open: "default",
  partially_paid: "warning",
  paid: "success",
  void: "danger",
};

export const INVOICE_DUE_STATES: InvoiceDueState[] = [
  "overdue",
  "due_soon",
  "later",
  "settled",
];

export const INVOICE_DUE_LABEL_KEYS: Record<InvoiceDueState, string> = {
  overdue: "suppliers.invoice_due.overdue",
  due_soon: "suppliers.invoice_due.due_soon",
  later: "suppliers.invoice_due.later",
  settled: "suppliers.invoice_due.settled",
};

export const INVOICE_DUE_TONES: Record<InvoiceDueState, Tone> = {
  overdue: "danger",
  due_soon: "warning",
  later: "default",
  settled: "success",
};

export const PAYMENT_METHODS: SupplierPaymentMethod[] = [
  "bank_transfer",
  "cash",
  "card",
  "cheque",
  "other",
];

export const PAYMENT_METHOD_LABEL_KEYS: Record<SupplierPaymentMethod, string> =
  {
    bank_transfer: "suppliers.method.bank_transfer",
    cash: "suppliers.method.cash",
    card: "suppliers.method.card",
    cheque: "suppliers.method.cheque",
    other: "suppliers.method.other",
  };

export const PAYMENT_STATUS_LABEL_KEYS = {
  posted: "suppliers.payment_status.posted",
  void: "suppliers.payment_status.void",
} as const;

export const VAT_RATES = ["0", "1", "10", "20"];

/** Default "due soon" window of the payables view (days). */
export const DUE_WINDOW_DAYS = 7;
