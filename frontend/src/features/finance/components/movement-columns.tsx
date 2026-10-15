"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Undo2 } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityRowActions } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Amount } from "@/features/finance/components/finance-cells";
import {
  MOVEMENT_KINDS,
  MOVEMENT_KIND_LABEL_KEYS,
} from "@/features/finance/constants";
import { reasonText } from "@/features/finance/lib/format";
import type { CashMovement, ExpenseCategory } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Options = {
  showAccount?: boolean;
  showBalance?: boolean;
  selectable?: boolean;
  categories?: ExpenseCategory[];
  accounts?: { uuid: string; code: string; name: string }[];
  onReverse?: (m: CashMovement) => void;
};

/** Columns of cash movements (movement ledger, account detail). */
export function useMovementColumns({
  showAccount = true,
  showBalance = false,
  selectable = false,
  categories,
  accounts,
  onReverse,
}: Options) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  return useMemo(() => {
    const cols: ColumnDef<CashMovement, unknown>[] = [];
    if (selectable) cols.push(createSelectColumnDef<CashMovement>());
    cols.push(
      createColumn<CashMovement>({
        accessorKey: "movement_date",
        labelKey: "finance.columns.date",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap tabular-nums">
            {date(row.original.movement_date, "dd.MM.yyyy", locale)}
          </span>
        ),
      }),
    );
    if (showAccount) {
      cols.push(
        createColumn<CashMovement>({
          id: "account",
          accessorFn: (row) => row.account.code,
          labelKey: "finance.columns.cash_account",
          enableSorting: false,
          filterVariant: accounts ? "faceted" : undefined,
          filterOptions: accounts?.map((a) => ({
            value: a.uuid,
            label: `${a.code} · ${a.name}`,
          })),
          cell: ({ row }) => (
            <Link
              href={routes.tenant.finance.cashAccount(
                slug,
                row.original.account.uuid,
              )}
              className="hover:underline"
              onClick={(e) => e.stopPropagation()}
            >
              {row.original.account.name}
            </Link>
          ),
        }),
      );
    }
    cols.push(
      createColumn<CashMovement>({
        accessorKey: "kind",
        labelKey: "finance.columns.movement_kind",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: MOVEMENT_KINDS.map((k) => ({
          value: k,
          labelKey: MOVEMENT_KIND_LABEL_KEYS[k],
          label: t(MOVEMENT_KIND_LABEL_KEYS[k]),
        })),
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <StatusChip
              label={t(MOVEMENT_KIND_LABEL_KEYS[row.original.kind])}
              tone={row.original.kind === "reversal" ? "danger" : "default"}
            />
            {row.original.reversed_by_uuid ? (
              <Badge variant="secondary">{t("finance.entry.reversed")}</Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<CashMovement>({
        accessorKey: "description",
        labelKey: "finance.columns.description",
        enableSorting: false,
        cell: ({ row }) => {
          const m = row.original;
          return (
            <div className="max-w-[20rem] space-y-0.5">
              <div className="truncate">{m.description || "—"}</div>
              {m.counter_account ? (
                <div className="text-muted-foreground truncate text-xs">
                  {t("finance.movement.counter", {
                    account: m.counter_account.name ?? "",
                  })}
                </div>
              ) : null}
              {m.reason ? (
                <div className="text-muted-foreground truncate text-xs">
                  {reasonText(t, m.reason)}
                </div>
              ) : null}
            </div>
          );
        },
      }),
      createColumn<CashMovement>({
        id: "category",
        accessorFn: (row) => row.category?.name ?? "",
        labelKey: "finance.columns.category",
        enableSorting: false,
        filterVariant: categories ? "faceted" : undefined,
        filterOptions: categories?.map((c) => ({
          value: c.uuid,
          label: c.name,
        })),
        cell: ({ row }) =>
          row.original.category ? (
            <Badge variant="outline">{row.original.category.name}</Badge>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }),
      createColumn<CashMovement>({
        accessorKey: "amount",
        labelKey: "finance.columns.amount",
        enableSorting: true,
        cell: ({ row }) => (
          <span
            className={
              row.original.direction === "in"
                ? "text-emerald-600 dark:text-emerald-400"
                : "text-destructive"
            }
          >
            {row.original.direction === "in" ? "+" : "−"}
            <Amount
              value={row.original.amount}
              currency={row.original.currency}
            />
          </span>
        ),
      }),
    );
    if (showBalance) {
      cols.push(
        createColumn<CashMovement>({
          accessorKey: "running_balance",
          labelKey: "finance.columns.running_balance",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.running_balance}
              currency={row.original.currency}
              className="font-medium"
            />
          ),
        }),
      );
    }
    cols.push(
      createColumn<CashMovement>({
        id: "branch",
        accessorFn: (row) => row.branch?.code ?? "",
        labelKey: "finance.columns.branch",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.branch?.name ?? t("finance.head_office"),
      }),
    );
    if (onReverse) {
      cols.push(
        createColumn<CashMovement>({
          id: "actions",
          labelKey: "finance.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const m = row.original;
            if (
              m.kind === "reversal" ||
              m.kind === "expense" ||
              m.reversed_by_uuid
            ) {
              return null;
            }
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "reverse",
                    label: t("finance.actions.reverse"),
                    icon: Undo2,
                    permission: permissions.tenant.finance.cash.write,
                    variant: "destructive",
                    onSelect: () => onReverse(m),
                  },
                ]}
              />
            );
          },
        }),
      );
    }
    return cols;
  }, [
    accounts,
    categories,
    locale,
    onReverse,
    selectable,
    showAccount,
    showBalance,
    slug,
    t,
  ]);
}
