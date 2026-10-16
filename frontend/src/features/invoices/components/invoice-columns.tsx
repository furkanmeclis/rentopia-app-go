"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo } from "react";

import { createColumn, createSelectColumnDef } from "@/components/tables";
import { routes } from "@/config/routes";
import { Amount } from "@/features/finance/components/finance-cells";
import {
  InvoiceProfileBadge,
  InvoiceStatusChip,
} from "@/features/invoices/components/invoice-cells";
import {
  HOLD_REASON_LABEL_KEYS,
  INVOICE_PROFILES,
  INVOICE_PROFILE_LABEL_KEYS,
  INVOICE_STATUSES,
  INVOICE_STATUS_LABEL_KEYS,
  INVOICE_TYPE_LABEL_KEYS,
} from "@/features/invoices/constants";
import type { Invoice, InvoiceFacets } from "@/features/invoices/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Options = {
  selectable?: boolean;
  /** Status / profile / branch facets (list page). */
  facets?: InvoiceFacets;
  withFilters?: boolean;
};

function count(
  facets: InvoiceFacets | undefined,
  list: "status" | "profile" | "branch",
  value: string,
) {
  return facets?.[list].find((f) => f.value === value)?.count ?? 0;
}

/** Columns of invoices (list page, rental / account cards). */
export function useInvoiceColumns({
  selectable = false,
  facets,
  withFilters = false,
}: Options) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  return useMemo(() => {
    const cols: ColumnDef<Invoice, unknown>[] = [];
    if (selectable) cols.push(createSelectColumnDef<Invoice>());
    cols.push(
      createColumn<Invoice>({
        accessorKey: "number",
        labelKey: "invoices.columns.number",
        enableSorting: withFilters,
        gridPrimary: true,
        cell: ({ row }) => (
          <Link
            href={routes.tenant.invoices.detail(slug, row.original.uuid)}
            className="font-mono text-xs font-medium underline-offset-4 hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            {row.original.number ?? t("invoices.draft_number")}
          </Link>
        ),
      }),
      createColumn<Invoice>({
        accessorKey: "issue_date",
        labelKey: "invoices.columns.issue_date",
        enableSorting: withFilters,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="text-sm whitespace-nowrap tabular-nums">
            {date(row.original.issue_date, "d MMM yyyy", locale)}
          </span>
        ),
      }),
      createColumn<Invoice>({
        id: "receiver",
        accessorFn: (r) => r.receiver_name,
        labelKey: "invoices.columns.receiver",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="flex min-w-0 flex-col">
            <span className="truncate text-sm">
              {row.original.receiver_name || "—"}
            </span>
            {row.original.receiver_identity_hint ? (
              <span className="text-muted-foreground font-mono text-[11px]">
                {row.original.receiver_identity_hint}
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<Invoice>({
        accessorKey: "profile",
        labelKey: "invoices.columns.profile",
        enableSorting: false,
        ...(withFilters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: INVOICE_PROFILES.map((v) => ({
                value: v,
                labelKey: INVOICE_PROFILE_LABEL_KEYS[v],
                label: `${t(INVOICE_PROFILE_LABEL_KEYS[v])} (${count(facets, "profile", v)})`,
              })),
            }
          : {}),
        cell: ({ row }) => (
          <span className="flex flex-col items-start gap-1">
            <InvoiceProfileBadge profile={row.original.profile} />
            {row.original.invoice_type === "IADE" ? (
              <span className="text-muted-foreground text-[11px]">
                {t(INVOICE_TYPE_LABEL_KEYS.IADE!)}
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<Invoice>({
        accessorKey: "status",
        labelKey: "invoices.columns.status",
        enableSorting: false,
        ...(withFilters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: INVOICE_STATUSES.map((v) => ({
                value: v,
                labelKey: INVOICE_STATUS_LABEL_KEYS[v],
                label: `${t(INVOICE_STATUS_LABEL_KEYS[v])} (${count(facets, "status", v)})`,
              })),
            }
          : {}),
        cell: ({ row }) => {
          const n = row.original;
          const note = n.hold_reason
            ? t(HOLD_REASON_LABEL_KEYS[n.hold_reason] ?? n.hold_reason)
            : n.status === "draft" && n.problem_count > 0
              ? t("invoices.problem_count", { count: n.problem_count })
              : n.returned
                ? t("invoices.returned")
                : "";
          return (
            <span className="flex flex-col items-start gap-1">
              <InvoiceStatusChip status={n.status} />
              {note ? (
                <span className="text-muted-foreground text-[11px]">
                  {note}
                </span>
              ) : null}
            </span>
          );
        },
      }),
      createColumn<Invoice>({
        accessorKey: "payable_amount",
        labelKey: "invoices.columns.payable_amount",
        enableSorting: withFilters,
        cell: ({ row }) => (
          <Amount
            value={row.original.payable_amount}
            currency={row.original.currency}
          />
        ),
      }),
      createColumn<Invoice>({
        id: "branch",
        accessorFn: (r) => r.branch.code,
        labelKey: "invoices.columns.branch",
        enableSorting: false,
        ...(withFilters
          ? {
              filterVariant: "faceted" as const,
              filterOptions: (facets?.branch ?? []).map((f) => ({
                value: f.value,
                label: `${f.label ?? f.value} (${f.count})`,
              })),
            }
          : {}),
        cell: ({ row }) => (
          <span className="text-sm">{row.original.branch.name}</span>
        ),
      }),
      createColumn<Invoice>({
        id: "rental",
        accessorFn: (r) => r.rental?.reference ?? "",
        labelKey: "invoices.columns.rental",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.rental ? (
            <Link
              href={routes.tenant.rentals.detail(
                slug,
                row.original.rental.uuid,
              )}
              className="font-mono text-xs underline-offset-4 hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              {row.original.rental.reference}
            </Link>
          ) : (
            "—"
          ),
      }),
      createColumn<Invoice>({
        accessorKey: "external_ref",
        labelKey: "invoices.columns.external_ref",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.external_ref || "—"}
          </span>
        ),
      }),
      createColumn<Invoice>({
        accessorKey: "last_error",
        labelKey: "invoices.columns.last_error",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground line-clamp-2 text-xs break-words">
            {row.original.last_error || "—"}
          </span>
        ),
      }),
    );
    return cols;
  }, [facets, locale, selectable, slug, t, withFilters]);
}
