"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { FUEL_POLICY_LABEL_KEYS } from "@/features/fleet/constants";
import { moneyText } from "@/features/pricing/lib/format";
import { reasonText } from "@/features/reservations/constants";
import type {
  QuoteLine,
  ReservationConditions,
} from "@/features/reservations/types";
import { date } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const LINE_KIND_LABEL_KEYS: Record<QuoteLine["kind"], string> = {
  day: "reservations.price.kinds.day",
  extra: "reservations.price.kinds.extra",
  one_way: "reservations.price.kinds.one_way",
  delivery: "reservations.price.kinds.delivery",
  discount: "reservations.price.kinds.discount",
};

/** Snapshot lines (days, extras, one-way, delivery, discount) as a DataTable. */
export function PriceLinesTable({
  lines,
  persistKey,
}: {
  lines: QuoteLine[];
  persistKey: string;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<QuoteLine>({
          accessorKey: "kind",
          labelKey: "reservations.price.columns.kind",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(LINE_KIND_LABEL_KEYS[row.original.kind])}
            </Badge>
          ),
        }),
        createColumn<QuoteLine>({
          id: "name",
          accessorFn: (row) => row.name,
          labelKey: "reservations.price.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const l = row.original;
            if (l.kind === "day") {
              return (
                <span className="flex flex-col">
                  <span>
                    {t("reservations.price.day", {
                      index: l.day_index ?? 0,
                    })}
                    {l.day_date
                      ? ` · ${date(l.day_date, "d MMM EEE", locale)}`
                      : ""}
                  </span>
                  {l.date_rule_name ? (
                    <span className="text-muted-foreground text-xs">
                      {l.date_rule_name}
                    </span>
                  ) : null}
                </span>
              );
            }
            return l.name || l.code || "—";
          },
        }),
        createColumn<QuoteLine>({
          accessorKey: "quantity",
          labelKey: "reservations.price.columns.quantity",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.quantity}</span>
          ),
        }),
        createColumn<QuoteLine>({
          accessorKey: "unit_price",
          labelKey: "reservations.price.columns.unit_price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.unit_price, locale)}
            </span>
          ),
        }),
        createColumn<QuoteLine>({
          accessorKey: "adjustment",
          labelKey: "reservations.price.columns.adjustment",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            Number(row.original.adjustment) !== 0 ? (
              <span className="tabular-nums">
                {moneyText(row.original.adjustment, locale)}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<QuoteLine>({
          accessorKey: "amount",
          labelKey: "reservations.price.columns.amount",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span
              className={cn(
                "font-medium tabular-nums",
                row.original.kind === "discount" && "text-emerald-600",
              )}
            >
              {moneyText(row.original.amount, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<QuoteLine, unknown>[],
    [locale, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={lines}
      getRowId={(row) => String(row.position)}
      emptyTitle={t("reservations.price.no_lines")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey,
        rowSelection: false,
        globalFilter: false,
        sorting: false,
      }}
    />
  );
}

export type TotalsRow = {
  key: string;
  label: string;
  value: string | null | undefined;
  strong?: boolean;
  negative?: boolean;
};

/** Right-aligned totals ("Ara toplam", "KDV", "Toplam", "Ön ödeme"…). */
export function PriceTotals({ rows }: { rows: TotalsRow[] }) {
  const { locale } = useLocale();
  return (
    <dl className="ms-auto grid w-full max-w-sm gap-1.5 text-sm">
      {rows
        .filter((row) => row.value != null && row.value !== "")
        .map((row) => (
          <div
            key={row.key}
            className={cn(
              "flex items-center justify-between gap-4",
              row.strong && "border-t pt-1.5 text-base font-semibold",
            )}
          >
            <dt className={row.strong ? undefined : "text-muted-foreground"}>
              {row.label}
            </dt>
            <dd
              className={cn("tabular-nums", row.negative && "text-emerald-600")}
            >
              {row.negative ? "−" : ""}
              {moneyText(row.value, locale)}
            </dd>
          </div>
        ))}
    </dl>
  );
}

/** Conditions of the sold product version (guarantee, deposit, driver rules). */
export function ConditionsList({
  conditions,
  previous,
}: {
  conditions: ReservationConditions;
  previous?: ReservationConditions | null;
}) {
  const { t, locale } = useLocale();
  const fuelText = (code: string) =>
    code ? reasonText(t, FUEL_POLICY_LABEL_KEYS, code) : "—";
  const rows: { key: string; label: string; value: string; old?: string }[] = [
    {
      key: "guarantee",
      label: t("reservations.conditions.guarantee"),
      value: conditions.guarantee_label || conditions.guarantee_type,
      old: previous
        ? previous.guarantee_label || previous.guarantee_type
        : undefined,
    },
    {
      key: "deposit",
      label: t("reservations.conditions.deposit"),
      value: moneyText(conditions.deposit_amount, locale),
      old: previous ? moneyText(previous.deposit_amount, locale) : undefined,
    },
    {
      key: "min_age",
      label: t("reservations.conditions.min_driver_age"),
      value: String(conditions.min_driver_age),
      old: previous ? String(previous.min_driver_age) : undefined,
    },
    {
      key: "min_license",
      label: t("reservations.conditions.min_license_years"),
      value: String(conditions.min_license_years),
      old: previous ? String(previous.min_license_years) : undefined,
    },
    {
      key: "km",
      label: t("reservations.conditions.included_km"),
      value:
        conditions.included_km_per_day == null
          ? t("reservations.conditions.unlimited")
          : String(conditions.included_km_per_day),
      old: previous
        ? previous.included_km_per_day == null
          ? t("reservations.conditions.unlimited")
          : String(previous.included_km_per_day)
        : undefined,
    },
    {
      key: "fuel",
      label: t("reservations.conditions.fuel_policy"),
      value: fuelText(conditions.fuel_policy),
      old: previous ? fuelText(previous.fuel_policy) : undefined,
    },
  ];
  return (
    <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
      {rows.map((row) => (
        <div key={row.key} className="flex flex-col">
          <dt className="text-muted-foreground text-xs">{row.label}</dt>
          <dd className="font-medium">
            {row.value}
            {row.old !== undefined && row.old !== row.value ? (
              <span className="text-muted-foreground ms-2 text-xs line-through">
                {row.old}
              </span>
            ) : null}
          </dd>
        </div>
      ))}
    </dl>
  );
}
