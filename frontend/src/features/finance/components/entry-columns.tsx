"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Undo2 } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { EntityRowActions } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  Amount,
  EntryKindChip,
} from "@/features/finance/components/finance-cells";
import {
  ENTRY_KINDS,
  ENTRY_KIND_LABEL_KEYS,
  PURPOSE_LABEL_KEYS,
} from "@/features/finance/constants";
import { reasonText } from "@/features/finance/lib/format";
import type { LedgerEntry } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Options = {
  /** Show the account column (ledger-wide lists). */
  showAccount?: boolean;
  /** Running balance column (a statement). */
  showBalance?: boolean;
  onReverse?: (entry: LedgerEntry) => void;
  /** Kind facet (server-side filter on paged lists). */
  kindFilter?: boolean;
};

/** Columns of ledger entries (statement, rental and customer cards). */
export function useEntryColumns({
  showAccount = false,
  showBalance = true,
  onReverse,
  kindFilter = false,
}: Options) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  return useMemo(() => {
    const cols: ColumnDef<LedgerEntry, unknown>[] = [
      createColumn<LedgerEntry>({
        accessorKey: "entry_date",
        labelKey: "finance.columns.entry_date",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap tabular-nums">
            {date(row.original.entry_date, "dd.MM.yyyy", locale)}
          </span>
        ),
      }),
      createColumn<LedgerEntry>({
        accessorKey: "kind",
        labelKey: "finance.columns.kind",
        enableSorting: false,
        filterVariant: kindFilter ? "faceted" : undefined,
        filterOptions: kindFilter
          ? ENTRY_KINDS.map((k) => ({
              value: k,
              labelKey: ENTRY_KIND_LABEL_KEYS[k],
              label: t(ENTRY_KIND_LABEL_KEYS[k]),
            }))
          : undefined,
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <EntryKindChip kind={row.original.kind} />
            {row.original.purpose !== "general" ? (
              <Badge variant="outline">
                {t(PURPOSE_LABEL_KEYS[row.original.purpose])}
              </Badge>
            ) : null}
            {row.original.reversed_by_uuid ? (
              <Badge variant="secondary">{t("finance.entry.reversed")}</Badge>
            ) : null}
          </div>
        ),
      }),
    ];
    if (showAccount) {
      cols.push(
        createColumn<LedgerEntry>({
          id: "account",
          accessorFn: (row) => row.account.name,
          labelKey: "finance.columns.account",
          enableSorting: false,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.finance.account(
                slug,
                row.original.account.uuid,
              )}
              className="hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              <span className="font-medium">{row.original.account.name}</span>{" "}
              <code className="text-muted-foreground text-xs">
                {row.original.account.code}
              </code>
            </Link>
          ),
        }),
      );
    }
    cols.push(
      createColumn<LedgerEntry>({
        id: "reference",
        accessorFn: (row) => row.rental?.code ?? row.reservation?.code ?? "",
        labelKey: "finance.columns.reference",
        enableSorting: false,
        cell: ({ row }) => {
          const r = row.original;
          if (r.rental) {
            return (
              <Link
                href={routes.tenant.rentals.detail(slug, r.rental.uuid)}
                className="font-mono text-xs hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {r.rental.code}
              </Link>
            );
          }
          return (
            <span className="font-mono text-xs">
              {r.reservation?.code ?? "—"}
            </span>
          );
        },
      }),
      createColumn<LedgerEntry>({
        accessorKey: "description",
        labelKey: "finance.columns.description",
        enableSorting: false,
        cell: ({ row }) => {
          const r = row.original;
          return (
            <div className="max-w-[22rem] space-y-0.5">
              <div className="truncate">{r.description || "—"}</div>
              {r.reason ? (
                <div className="text-muted-foreground truncate text-xs">
                  {reasonText(t, r.reason)}
                </div>
              ) : null}
              {r.links.length > 0 ? (
                <div className="text-muted-foreground text-xs">
                  {t("finance.entry.invoice", {
                    ref: r.links.map((l) => l.document_ref).join(", "),
                  })}
                </div>
              ) : null}
            </div>
          );
        },
      }),
      createColumn<LedgerEntry>({
        id: "debit",
        accessorFn: (row) => (row.direction === "debit" ? row.amount : ""),
        labelKey: "finance.columns.debit",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.direction === "debit" ? (
            <Amount
              value={row.original.amount}
              currency={row.original.currency}
            />
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<LedgerEntry>({
        id: "credit",
        accessorFn: (row) => (row.direction === "credit" ? row.amount : ""),
        labelKey: "finance.columns.credit",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.direction === "credit" ? (
            <Amount
              value={row.original.amount}
              currency={row.original.currency}
            />
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
    );
    if (showBalance) {
      cols.push(
        createColumn<LedgerEntry>({
          accessorKey: "running_balance",
          labelKey: "finance.columns.running_balance",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.running_balance}
              currency={row.original.currency}
              signed
              className="font-medium"
            />
          ),
        }),
      );
    }
    cols.push(
      createColumn<LedgerEntry>({
        accessorKey: "due_date",
        labelKey: "finance.columns.due_date",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => date(row.original.due_date, "dd.MM.yyyy", locale),
      }),
      createColumn<LedgerEntry>({
        id: "branch",
        accessorFn: (row) => row.branch?.code ?? "",
        labelKey: "finance.columns.branch",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.branch?.name ?? "—",
      }),
    );
    if (onReverse) {
      cols.push(
        createColumn<LedgerEntry>({
          id: "actions",
          labelKey: "finance.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const e = row.original;
            const reversible =
              e.kind !== "reversal" &&
              e.kind !== "accrual" &&
              e.kind !== "offset" &&
              !e.reversed_by_uuid;
            if (!reversible) return null;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "reverse",
                    label: t("finance.actions.reverse"),
                    icon: Undo2,
                    permission: permissions.tenant.finance.ledger.adjust,
                    onSelect: () => onReverse(e),
                    variant: "destructive",
                  },
                ]}
              />
            );
          },
        }),
      );
    }
    return cols;
  }, [kindFilter, locale, onReverse, showAccount, showBalance, slug, t]);
}
