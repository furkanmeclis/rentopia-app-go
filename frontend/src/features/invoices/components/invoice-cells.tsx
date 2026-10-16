"use client";

import { StatusChip } from "@/components/common/status-chip";
import { Badge } from "@/components/ui/badge";
import {
  INVOICE_PROFILE_LABEL_KEYS,
  INVOICE_STATUS_LABEL_KEYS,
  INVOICE_STATUS_TONES,
  LINE_FIELD_LABEL_KEYS,
  PROBLEM_FIELD_LABEL_KEYS,
} from "@/features/invoices/constants";
import type {
  InvoiceProblem,
  InvoiceProfile,
  InvoiceStatus,
} from "@/features/invoices/types";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

export function InvoiceStatusChip({ status }: { status: InvoiceStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(INVOICE_STATUS_LABEL_KEYS[status])}
      tone={INVOICE_STATUS_TONES[status]}
    />
  );
}

export function InvoiceProfileBadge({ profile }: { profile: InvoiceProfile }) {
  const { t } = useLocale();
  return (
    <Badge variant="outline" className="font-normal whitespace-nowrap">
      {t(INVOICE_PROFILE_LABEL_KEYS[profile])}
    </Badge>
  );
}

/** Label of a problem's form field (lines.2.vat_rate → "Satır 3 · KDV"). */
export function problemFieldText(t: T, field: string): string {
  const line = /^lines\.(\d+)(?:\.(\w+))?$/.exec(field);
  if (line) {
    const n = Number(line[1]) + 1;
    const sub = line[2] ? LINE_FIELD_LABEL_KEYS[line[2]] : undefined;
    return sub
      ? `${t("invoices.fields.line_n", { n })} · ${t(sub)}`
      : t("invoices.fields.line_n", { n });
  }
  const key = PROBLEM_FIELD_LABEL_KEYS[field];
  return key ? t(key) : field;
}

/** Problems grouped by field (receiver, supplier, lines, document). */
export function groupProblems(problems: InvoiceProblem[]) {
  const out = new Map<string, InvoiceProblem[]>();
  for (const p of problems) {
    const list = out.get(p.field) ?? [];
    if (!list.some((x) => x.message === p.message)) list.push(p);
    out.set(p.field, list);
  }
  return [...out.entries()];
}

/** Text of a refused invoice action: the code first, then the details. */
export function invoiceProblemText(t: T, error: unknown): string {
  if (!(error instanceof ApiError)) return t("invoices.problems.generic");
  switch (error.code) {
    case "NOTHING_TO_INVOICE":
      return t("invoices.problems.nothing_to_invoice");
    case "ALREADY_INVOICED":
      return t("invoices.problems.already_invoiced");
    case "INVOICE_DOCUMENT_INVALID":
      return t("invoices.problems.document_invalid", {
        count: error.details.length,
      });
    case "EINVOICE_DEPENDENCY_PENDING":
      return t("invoices.problems.dependency_pending");
    case "INVOICE_UNVERIFIED":
      return t("invoices.problems.unverified");
    case "INVOICE_BUSY":
      return t("invoices.problems.busy");
    case "INVOICE_NOT_RETRYABLE":
      return t("invoices.problems.not_retryable");
    case "INVOICE_NOT_CANCELLABLE":
      return t("invoices.problems.not_cancellable");
    case "USE_RETURN_INVOICE":
      return t("invoices.problems.use_return");
    case "INVOICE_NOT_RETURNABLE":
      return t("invoices.problems.not_returnable");
    case "INVOICE_NOT_DRAFT":
      return t("invoices.problems.not_draft");
    case "PDF_UNAVAILABLE":
      return t("invoices.problems.pdf_unavailable");
  }
  if (error.details.length > 0) {
    return error.details
      .map((d) =>
        d.field ? `${problemFieldText(t, d.field)}: ${d.message}` : d.message,
      )
      .join(" · ");
  }
  return error.message;
}
