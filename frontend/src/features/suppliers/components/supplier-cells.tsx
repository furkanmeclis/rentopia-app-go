"use client";

import { StatusChip } from "@/components/common/status-chip";
import {
  INVOICE_DUE_LABEL_KEYS,
  INVOICE_DUE_TONES,
  INVOICE_STATUS_LABEL_KEYS,
  INVOICE_STATUS_TONES,
  SUPPLIER_DUE_LABEL_KEYS,
  SUPPLIER_DUE_TONES,
} from "@/features/suppliers/constants";
import type {
  InvoiceDueState,
  SupplierDueState,
  SupplierInvoiceStatus,
} from "@/features/suppliers/types";
import { useLocale } from "@/providers/locale-provider";

export function SupplierDueChip({ state }: { state: SupplierDueState }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(SUPPLIER_DUE_LABEL_KEYS[state])}
      tone={SUPPLIER_DUE_TONES[state]}
    />
  );
}

export function InvoiceStatusChip({
  status,
}: {
  status: SupplierInvoiceStatus;
}) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(INVOICE_STATUS_LABEL_KEYS[status])}
      tone={INVOICE_STATUS_TONES[status]}
    />
  );
}

export function InvoiceDueChip({ state }: { state: InvoiceDueState }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(INVOICE_DUE_LABEL_KEYS[state])}
      tone={INVOICE_DUE_TONES[state]}
    />
  );
}
