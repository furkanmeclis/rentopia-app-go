"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CalendarPlus } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { permissions } from "@/config/permissions";
import { REASON_LABEL_KEYS } from "@/features/availability/constants";
import type { AvailabilityProduct } from "@/features/availability/types";
import { useLocale } from "@/providers/locale-provider";

type Translate = (key: string) => string;

/** A breakdown count; zero is muted so the numbers that matter stand out. */
function Count({ value }: { value: number }) {
  return (
    <span
      className={
        value === 0 ? "text-muted-foreground tabular-nums" : "tabular-nums"
      }
    >
      {value}
    </span>
  );
}

function countColumn(
  id: keyof AvailabilityProduct["breakdown"],
  labelKey: string,
  options: { hidden?: boolean } = {},
) {
  return createColumn<AvailabilityProduct>({
    id,
    accessorFn: (row) => row.breakdown[id] ?? 0,
    labelKey,
    enableSorting: true,
    defaultHidden: options.hidden,
    cell: ({ row }) => (
      <Count value={Number(row.original.breakdown[id] ?? 0)} />
    ),
  });
}

/** "Satılabilir" with the explanation of the number in a tooltip. */
function SellableCell({ row, t }: { row: AvailabilityProduct; t: Translate }) {
  const b = row.breakdown;
  const lines: [string, number | null | undefined][] = [
    [t("availability.breakdown.eligible"), b.eligible],
    [t("availability.breakdown.free"), b.free],
    [t("availability.breakdown.reassignable"), b.reassignable],
    [t("availability.breakdown.physical"), b.physical],
    [t("availability.breakdown.safety_margin"), b.safety_margin],
    [
      t("availability.breakdown.reserved_for_other_channels"),
      b.reserved_for_other_channels,
    ],
    [t("availability.breakdown.own_quota_remaining"), b.own_quota_remaining],
    [
      t("availability.breakdown.channel_limit_remaining"),
      b.channel_limit_remaining,
    ],
  ];
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={0}>
          <Badge variant={row.sellable > 0 ? "success" : "danger"}>
            {row.sellable}
          </Badge>
        </span>
      </TooltipTrigger>
      <TooltipContent className="space-y-0.5">
        {lines
          .filter(([, value]) => value != null)
          .map(([label, value]) => (
            <p key={label} className="flex justify-between gap-4 tabular-nums">
              <span>{label}</span>
              <span>{value}</span>
            </p>
          ))}
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * Capacity per product of one search: the engine's explainable breakdown
 * (each eligible vehicle counted once, in the first bucket that applies)
 * and the sellable number after the channel rules. "Opsiyon koy" opens the
 * hold dialog for the row.
 */
export function AvailabilityResultsTable({
  items,
  isLoading,
  isError,
  onRetry,
  onHold,
}: {
  items: AvailabilityProduct[];
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  onHold: (product: AvailabilityProduct) => void;
}) {
  const { t } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<AvailabilityProduct>({
          id: "product",
          accessorFn: (row) => row.product.code,
          labelKey: "availability.columns.product",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <p className="font-medium">
                <code className="text-xs">{row.original.product.code}</code>{" "}
                {row.original.product.title}
              </p>
              {row.original.reasons.length > 0 ? (
                <p className="text-muted-foreground text-xs">
                  {row.original.reasons
                    .map((reason) => t(REASON_LABEL_KEYS[reason]))
                    .join(" · ")}
                </p>
              ) : null}
            </div>
          ),
        }),
        createColumn<AvailabilityProduct>({
          id: "sellable",
          accessorFn: (row) => row.sellable,
          labelKey: "availability.columns.sellable",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => <SellableCell row={row.original} t={t} />,
        }),
        countColumn("eligible", "availability.breakdown.eligible"),
        countColumn("free", "availability.breakdown.free"),
        countColumn("reassignable", "availability.breakdown.reassignable"),
        countColumn("held", "availability.breakdown.held"),
        countColumn("reserved", "availability.breakdown.reserved"),
        countColumn("blocked", "availability.breakdown.blocked"),
        countColumn("elsewhere", "availability.breakdown.elsewhere"),
        countColumn("out_of_service", "availability.breakdown.out_of_service", {
          hidden: true,
        }),
        countColumn("safety_margin", "availability.breakdown.safety_margin", {
          hidden: true,
        }),
        countColumn(
          "reserved_for_other_channels",
          "availability.breakdown.reserved_for_other_channels",
          { hidden: true },
        ),
        createColumn<AvailabilityProduct>({
          id: "actions",
          labelKey: "availability.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "hold",
                  label: t("availability.actions.hold"),
                  icon: CalendarPlus,
                  permission: permissions.tenant.availability.hold,
                  disabled: row.original.sellable === 0,
                  onSelect: () => onHold(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AvailabilityProduct, unknown>[],
    [onHold, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={items}
      getRowId={(row) => row.product.uuid}
      isLoading={isLoading}
      isError={isError}
      errorDescription={t("availability.search_failed")}
      onRetry={onRetry}
      emptyTitle={t("availability.results_empty_title")}
      emptyDescription={t("availability.results_empty_description")}
      initialState={{ sorting: [{ id: "sellable", desc: true }] }}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "tenant-availability-results-v1",
        rowSelection: false,
      }}
    />
  );
}
