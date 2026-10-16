"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import { Amount } from "@/features/finance/components/finance-cells";
import {
  ALL_COLLECTION_METHODS,
  ALLOCATION_STATUSES,
  ALLOCATION_STATUS_LABEL_KEYS,
  ALLOCATION_STATUS_TONES,
  COLLECTION_STATUSES,
  COLLECTION_STATUS_LABEL_KEYS,
  COLLECTION_STATUS_TONES,
  METHOD_LABEL_KEYS,
} from "@/features/finance/constants";
import type { FinanceCollection } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Option = { uuid: string; code: string; name: string };

type Options = {
  selectable?: boolean;
  showAccount?: boolean;
  /** Method / status / allocation facets (list page). */
  facets?: boolean;
  branches?: Option[];
  cashAccounts?: Option[];
};

export function CollectionStatusChip({ c }: { c: FinanceCollection }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(COLLECTION_STATUS_LABEL_KEYS[c.status])}
      tone={COLLECTION_STATUS_TONES[c.status]}
    />
  );
}

export function AllocationStatusChip({ c }: { c: FinanceCollection }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(ALLOCATION_STATUS_LABEL_KEYS[c.allocation_status])}
      tone={ALLOCATION_STATUS_TONES[c.allocation_status]}
    />
  );
}

/** Columns of collections (list page, account detail). */
export function useCollectionColumns({
  selectable = false,
  showAccount = true,
  facets = true,
  branches,
  cashAccounts,
}: Options) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  return useMemo(() => {
    const cols: ColumnDef<FinanceCollection, unknown>[] = [];
    if (selectable) cols.push(createSelectColumnDef<FinanceCollection>());
    cols.push(
      createColumn<FinanceCollection>({
        accessorKey: "code",
        labelKey: "finance.columns.collection_code",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <Link
            href={routes.tenant.finance.collection(slug, row.original.uuid)}
            className="font-medium hover:underline"
            onClick={(e) => e.stopPropagation()}
          >
            {row.original.code}
          </Link>
        ),
      }),
      createColumn<FinanceCollection>({
        accessorKey: "collection_date",
        labelKey: "finance.columns.date",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap tabular-nums">
            {date(row.original.collection_date, "dd.MM.yyyy", locale)}
          </span>
        ),
      }),
    );
    if (showAccount) {
      cols.push(
        createColumn<FinanceCollection>({
          id: "account",
          accessorFn: (row) => row.account.name ?? "",
          labelKey: "finance.columns.account",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="max-w-[14rem] space-y-0.5">
              <Link
                href={routes.tenant.finance.account(
                  slug,
                  row.original.account.uuid,
                )}
                className="block truncate hover:underline"
                onClick={(e) => e.stopPropagation()}
              >
                {row.original.account.name || row.original.account.code}
              </Link>
              {row.original.rental ? (
                <div className="text-muted-foreground truncate text-xs">
                  {row.original.rental.code}
                </div>
              ) : null}
            </div>
          ),
        }),
      );
    }
    cols.push(
      createColumn<FinanceCollection>({
        accessorKey: "method",
        labelKey: "finance.columns.method",
        enableSorting: false,
        filterVariant: facets ? "faceted" : undefined,
        filterOptions: ALL_COLLECTION_METHODS.map((m) => ({
          value: m,
          labelKey: METHOD_LABEL_KEYS[m],
          label: t(METHOD_LABEL_KEYS[m]),
        })),
        cell: ({ row }) => (
          <Badge variant="outline">
            {t(METHOD_LABEL_KEYS[row.original.method])}
          </Badge>
        ),
      }),
      createColumn<FinanceCollection>({
        id: "reference_no",
        accessorKey: "reference_no",
        labelKey: "finance.columns.reference_no",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.reference_no || "—",
      }),
      createColumn<FinanceCollection>({
        id: "payer_name",
        accessorKey: "payer_name",
        labelKey: "finance.columns.payer_name",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.payer_name || "—",
      }),
      createColumn<FinanceCollection>({
        id: "branch",
        accessorFn: (row) => row.branch?.code ?? "",
        labelKey: "finance.columns.branch",
        enableSorting: false,
        filterVariant: branches ? "faceted" : undefined,
        filterOptions: branches?.map((b) => ({
          value: b.uuid,
          label: `${b.code} · ${b.name}`,
        })),
        cell: ({ row }) =>
          row.original.branch?.name ?? t("finance.head_office"),
      }),
      createColumn<FinanceCollection>({
        id: "cash_account",
        accessorFn: (row) => row.cash_account?.code ?? "",
        labelKey: "finance.columns.cash_account",
        enableSorting: false,
        filterVariant: cashAccounts ? "faceted" : undefined,
        filterOptions: cashAccounts?.map((a) => ({
          value: a.uuid,
          label: `${a.code} · ${a.name}`,
        })),
        cell: ({ row }) =>
          row.original.cash_account ? (
            <Link
              href={routes.tenant.finance.cashAccount(
                slug,
                row.original.cash_account.uuid,
              )}
              className="hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              {row.original.cash_account.name}
            </Link>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<FinanceCollection>({
        accessorKey: "amount",
        labelKey: "finance.columns.amount",
        enableSorting: true,
        cell: ({ row }) => (
          <Amount
            value={row.original.amount}
            currency={row.original.currency}
            className="font-medium"
          />
        ),
      }),
      createColumn<FinanceCollection>({
        id: "commission_amount",
        accessorKey: "commission_amount",
        labelKey: "finance.columns.commission",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => (
          <Amount
            value={row.original.commission_amount}
            currency={row.original.currency}
            muted
          />
        ),
      }),
      createColumn<FinanceCollection>({
        id: "unallocated_amount",
        accessorKey: "unallocated_amount",
        labelKey: "finance.columns.credit_left",
        enableSorting: false,
        cell: ({ row }) =>
          Number(row.original.unallocated_amount) > 0 ? (
            <Amount
              value={row.original.unallocated_amount}
              currency={row.original.currency}
              className="text-amber-600 dark:text-amber-400"
            />
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<FinanceCollection>({
        accessorKey: "status",
        labelKey: "finance.columns.status",
        enableSorting: false,
        filterVariant: facets ? "faceted" : undefined,
        filterOptions: COLLECTION_STATUSES.map((s) => ({
          value: s,
          labelKey: COLLECTION_STATUS_LABEL_KEYS[s],
          label: t(COLLECTION_STATUS_LABEL_KEYS[s]),
        })),
        cell: ({ row }) => <CollectionStatusChip c={row.original} />,
      }),
      createColumn<FinanceCollection>({
        accessorKey: "allocation_status",
        labelKey: "finance.columns.allocation_status",
        enableSorting: false,
        defaultHidden: true,
        filterVariant: facets ? "faceted" : undefined,
        filterOptions: ALLOCATION_STATUSES.map((s) => ({
          value: s,
          labelKey: ALLOCATION_STATUS_LABEL_KEYS[s],
          label: t(ALLOCATION_STATUS_LABEL_KEYS[s]),
        })),
        cell: ({ row }) => <AllocationStatusChip c={row.original} />,
      }),
    );
    return cols;
  }, [
    branches,
    cashAccounts,
    facets,
    locale,
    selectable,
    showAccount,
    slug,
    t,
  ]);
}
