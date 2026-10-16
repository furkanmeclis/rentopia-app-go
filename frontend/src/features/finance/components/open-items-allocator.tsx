"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Amount } from "@/features/finance/components/finance-cells";
import {
  ITEM_STATUS_LABEL_KEYS,
  ITEM_STATUS_TONES,
  PURPOSE_LABEL_KEYS,
} from "@/features/finance/constants";
import {
  centsOf,
  fromCents,
  normalizeAmount,
} from "@/features/finance/lib/format";
import type { EntryPurpose, FinanceOpenItem } from "@/features/finance/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type AllocationMode = "auto" | "manual" | "none";

const CHARGE_KIND_LABEL_KEYS: Record<string, string> = {
  damage: "rentals.charges.kinds.damage",
  early_return: "rentals.charges.kinds.early_return",
  fuel: "rentals.charges.kinds.fuel",
  late_return: "rentals.charges.kinds.late_return",
  mileage: "rentals.charges.kinds.mileage",
  other: "rentals.charges.kinds.other",
  equipment: "rentals.charges.kinds.equipment",
  toll: "rentals.charges.kinds.toll",
  toll_fee: "rentals.charges.kinds.toll_fee",
};

/** Label of a rental charge kind (raw value when unknown). */
export function chargeKindLabel(
  t: (key: string) => string,
  kind: string,
): string {
  const key = CHARGE_KIND_LABEL_KEYS[kind];
  return key ? t(key) : kind;
}

/** Oldest-first preview of an auto allocation (cents per entry). */
export function autoPlan(
  items: FinanceOpenItem[],
  amountCents: number,
): Record<string, number> {
  const out: Record<string, number> = {};
  let left = amountCents;
  for (const it of items) {
    if (left <= 0) break;
    const open = centsOf(it.open);
    if (open <= 0) continue;
    const take = Math.min(open, left);
    out[it.entry_uuid] = take;
    left -= take;
  }
  return out;
}

/** Cents of the manual inputs (invalid text counts as 0). */
export function manualPlan(
  values: Record<string, string>,
): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [id, raw] of Object.entries(values)) {
    const v = normalizeAmount(raw);
    if (v && /^\d{1,12}(\.\d{1,2})?$/.test(v)) out[id] = centsOf(v);
  }
  return out;
}

type Props = {
  items: FinanceOpenItem[];
  currency: string;
  mode: AllocationMode;
  /** Money to allocate (cents): the collection amount or its credit. */
  amountCents: number;
  manual: Record<string, string>;
  onManualChange: (next: Record<string, string>) => void;
  isLoading?: boolean;
  persistKey: string;
  /** Status view only (no allocation column, no summary). */
  readOnly?: boolean;
};

/**
 * The open items of an account (rent, charges) with what is still open and
 * what this collection would pay on each: an oldest-first preview (auto) or
 * an amount per item (manual). A client-side DataTable (bounded list).
 */
export function OpenItemsAllocator({
  items,
  currency,
  mode,
  amountCents,
  manual,
  onManualChange,
  isLoading,
  persistKey,
  readOnly = false,
}: Props) {
  const { t, locale } = useLocale();
  const plan = useMemo(
    () =>
      mode === "auto"
        ? autoPlan(items, amountCents)
        : mode === "manual"
          ? manualPlan(manual)
          : {},
    [amountCents, items, manual, mode],
  );
  const allocated = Object.values(plan).reduce((a, b) => a + b, 0);
  const credit = Math.max(0, amountCents - allocated);
  const over = allocated > amountCents;

  const columns = useMemo<ColumnDef<FinanceOpenItem, unknown>[]>(() => {
    const cols: ColumnDef<FinanceOpenItem, unknown>[] = [
      createColumn<FinanceOpenItem>({
        id: "item",
        accessorFn: (row) => row.description,
        labelKey: "finance.columns.item",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => {
          const it = row.original;
          return (
            <div className="max-w-[16rem] space-y-0.5">
              <div className="flex flex-wrap items-center gap-1">
                <Badge variant="outline">
                  {t(
                    PURPOSE_LABEL_KEYS[it.purpose as EntryPurpose] ??
                      "finance.purpose.general",
                  )}
                </Badge>
                {it.charge_kind ? (
                  <span className="text-muted-foreground text-xs">
                    {chargeKindLabel(t, it.charge_kind)}
                  </span>
                ) : null}
              </div>
              <div className="truncate text-sm">
                {it.rental?.code ?? it.description}
              </div>
            </div>
          );
        },
      }),
      createColumn<FinanceOpenItem>({
        accessorKey: "due_date",
        labelKey: "finance.columns.due_date",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="flex flex-wrap items-center gap-1 whitespace-nowrap tabular-nums">
            {date(row.original.due_date, "dd.MM.yyyy", locale)}
            {row.original.overdue ? (
              <Badge variant="danger">{t("finance.collect.overdue")}</Badge>
            ) : null}
          </span>
        ),
      }),
      createColumn<FinanceOpenItem>({
        accessorKey: "amount",
        labelKey: "finance.columns.amount",
        enableSorting: false,
        cell: ({ row }) => (
          <Amount value={row.original.amount} currency={currency} muted />
        ),
      }),
      createColumn<FinanceOpenItem>({
        accessorKey: "open",
        labelKey: "finance.columns.open",
        enableSorting: false,
        cell: ({ row }) => (
          <Amount
            value={row.original.open}
            currency={currency}
            className="font-medium"
          />
        ),
      }),
      createColumn<FinanceOpenItem>({
        accessorKey: "status",
        labelKey: "finance.columns.item_status",
        enableSorting: false,
        cell: ({ row }) => (
          <StatusChip
            label={t(ITEM_STATUS_LABEL_KEYS[row.original.status])}
            tone={ITEM_STATUS_TONES[row.original.status]}
          />
        ),
      }),
      createColumn<FinanceOpenItem>({
        id: "allocate",
        labelKey: "finance.columns.allocate",
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => {
          const it = row.original;
          if (mode === "manual") {
            return (
              <Input
                aria-label={t("finance.columns.allocate")}
                inputMode="decimal"
                className="h-8 w-28 tabular-nums"
                placeholder="0,00"
                disabled={centsOf(it.open) <= 0}
                value={manual[it.entry_uuid] ?? ""}
                onClick={(e) => e.stopPropagation()}
                onChange={(e) =>
                  onManualChange({
                    ...manual,
                    [it.entry_uuid]: e.target.value,
                  })
                }
              />
            );
          }
          const cents = plan[it.entry_uuid] ?? 0;
          return cents > 0 ? (
            <Amount
              value={fromCents(cents)}
              currency={currency}
              className="font-medium text-emerald-600 dark:text-emerald-400"
            />
          ) : (
            <span className="text-muted-foreground">—</span>
          );
        },
      }),
    ];
    return readOnly ? cols.filter((c) => c.id !== "allocate") : cols;
  }, [currency, locale, manual, mode, onManualChange, plan, readOnly, t]);

  return (
    <div className="space-y-3">
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.entry_uuid}
        isLoading={isLoading}
        emptyTitle={t("finance.collect.no_open_items")}
        emptyDescription={t("finance.collect.no_open_items_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{ persistKey, rowSelection: false }}
      />
      {readOnly ? null : (
        <dl className="bg-muted/40 grid grid-cols-2 gap-3 rounded-md p-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("finance.collect.to_allocate")}
            </dt>
            <dd>
              <Amount
                value={fromCents(allocated)}
                currency={currency}
                className={
                  over ? "text-destructive font-semibold" : "font-semibold"
                }
              />
            </dd>
          </div>
          <div>
            <dt className="text-muted-foreground text-xs">
              {t("finance.collect.remaining_credit")}
            </dt>
            <dd>
              <Amount value={fromCents(credit)} currency={currency} />
            </dd>
          </div>
          {over ? (
            <p className="text-destructive col-span-2 text-xs sm:col-span-1">
              {t("finance.collect.over_allocated")}
            </p>
          ) : null}
        </dl>
      )}
    </div>
  );
}
