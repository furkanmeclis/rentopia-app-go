"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, MousePointerClick } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { REASON_LABEL_KEYS } from "@/features/availability/constants";
import { moneyText } from "@/features/pricing/lib/format";
import { reasonText } from "@/features/reservations/constants";
import type { QuoteSearchItem } from "@/features/reservations/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * Products of a quote search: sellable count, price for the range and the
 * reasons a product cannot be sold. Choosing a row moves the wizard on.
 */
export function WizardProductTable({
  items,
  selected,
  isLoading,
  isError,
  onRetry,
  onSelect,
}: {
  items: QuoteSearchItem[];
  selected: string | null;
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  onSelect: (item: QuoteSearchItem) => void;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<QuoteSearchItem>({
          id: "product",
          accessorFn: (row) => row.product.code,
          labelKey: "reservations.wizard.columns.product",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="flex items-center gap-1.5 font-medium">
                {selected === row.original.product.uuid ? (
                  <CheckCircle2 className="text-primary size-4" />
                ) : null}
                <code className="text-xs">{row.original.product.code}</code>
                {row.original.product.title}
              </p>
              {row.original.reasons.length > 0 ? (
                <p className="text-muted-foreground text-xs">
                  {row.original.reasons
                    .map((r) => reasonText(t, REASON_LABEL_KEYS, r))
                    .join(" · ")}
                </p>
              ) : null}
              {row.original.price_error ? (
                <p className="text-destructive text-xs">
                  {t("reservations.wizard.price_unavailable")}
                </p>
              ) : null}
            </div>
          ),
        }),
        createColumn<QuoteSearchItem>({
          id: "sellable",
          accessorFn: (row) => row.sellable,
          labelKey: "reservations.wizard.columns.sellable",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge variant={row.original.sellable > 0 ? "success" : "danger"}>
              {row.original.sellable}
            </Badge>
          ),
        }),
        createColumn<QuoteSearchItem>({
          id: "billing_days",
          accessorFn: (row) => row.price?.billing_days ?? 0,
          labelKey: "reservations.wizard.columns.billing_days",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.price ? (
              <span className="tabular-nums">
                {row.original.price.billing_days}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<QuoteSearchItem>({
          id: "daily_average",
          accessorFn: (row) => Number(row.price?.daily_average ?? 0),
          labelKey: "reservations.wizard.columns.daily_average",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.price?.daily_average, locale)}
            </span>
          ),
        }),
        createColumn<QuoteSearchItem>({
          id: "total",
          accessorFn: (row) => Number(row.price?.total ?? 0),
          labelKey: "reservations.wizard.columns.total",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {moneyText(row.original.price?.total, locale)}
            </span>
          ),
        }),
        createColumn<QuoteSearchItem>({
          id: "prepayment",
          accessorFn: (row) => Number(row.price?.prepayment ?? 0),
          labelKey: "reservations.wizard.columns.prepayment",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.price?.prepayment, locale)}
            </span>
          ),
        }),
        createColumn<QuoteSearchItem>({
          id: "actions",
          labelKey: "reservations.wizard.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "select",
                  label: t("reservations.wizard.select_product"),
                  icon: MousePointerClick,
                  disabled: row.original.sellable === 0 || !row.original.price,
                  onSelect: () => onSelect(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<QuoteSearchItem, unknown>[],
    [locale, onSelect, selected, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={items}
      getRowId={(row) => row.product.uuid}
      onRowClick={(row) => {
        if (row.sellable > 0 && row.price) onSelect(row);
      }}
      isLoading={isLoading}
      isError={isError}
      errorDescription={t("reservations.wizard.search_failed")}
      onRetry={onRetry}
      emptyTitle={t("reservations.wizard.results_empty_title")}
      emptyDescription={t("reservations.wizard.results_empty_description")}
      initialState={{ sorting: [{ id: "sellable", desc: true }] }}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "tenant-reservation-wizard-products-v1",
        rowSelection: false,
      }}
    />
  );
}
