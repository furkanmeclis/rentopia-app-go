"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Save, Undo2 } from "lucide-react";
import { useMemo, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { permissions } from "@/config/permissions";
import {
  useExtraStock,
  useSetExtraStock,
} from "@/features/extras/hooks/use-extras";
import type { Extra, ExtraStockRow } from "@/features/extras/types";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const QTY_RE = /^\d{1,6}$/;

/**
 * Branch stock of a physical extra: owned units per branch (inline edit),
 * units in use now and the committed peak of future allocations. A count
 * below the peak is saved with a warning (new sales are then refused).
 */
export function ExtraStockTab({ extra }: { extra: Extra }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.extras.write);
  const query = useExtraStock(extra.uuid, extra.is_physical);
  const save = useSetExtraStock();
  const [grid, setGrid] = useState<Record<string, string>>({});
  const rows = useMemo(() => query.data?.items ?? [], [query.data]);
  const baseline = useMemo(() => {
    const out: Record<string, string> = {};
    for (const r of rows) out[r.branch.uuid] = String(r.quantity);
    return out;
  }, [rows]);
  const changed = Object.keys(grid).filter(
    (uuid) => grid[uuid].trim() !== baseline[uuid],
  );
  const invalid = changed.filter(
    (uuid) => !QTY_RE.test(grid[uuid].trim()) || Number(grid[uuid]) > 100000,
  );

  const columns = useMemo(
    () =>
      [
        createColumn<ExtraStockRow>({
          id: "branch",
          accessorFn: (row) => row.branch.code,
          labelKey: "extras.stock.columns.branch",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex items-center gap-2">
              <code className="text-xs">{row.original.branch.code}</code>
              <span>{row.original.branch.name}</span>
              {row.original.branch_active ? null : (
                <StatusChip label={t("extras.stock.branch_inactive")} />
              )}
            </span>
          ),
        }),
        createColumn<ExtraStockRow>({
          id: "quantity",
          accessorFn: (row) => row.quantity,
          labelKey: "extras.stock.columns.quantity",
          enableSorting: false,
          cell: ({ row }) => {
            const uuid = row.original.branch.uuid;
            const isChanged = changed.includes(uuid);
            const isInvalid = invalid.includes(uuid);
            return (
              <Input
                value={grid[uuid] ?? String(row.original.quantity)}
                readOnly={!canWrite}
                inputMode="numeric"
                aria-label={`${row.original.branch.code} ${t("extras.stock.columns.quantity")}`}
                aria-invalid={isInvalid || undefined}
                className={cn(
                  "h-8 w-24 text-right tabular-nums",
                  isChanged &&
                    "border-amber-500 bg-amber-50 dark:bg-amber-950/30",
                  isInvalid && "border-destructive",
                )}
                onFocus={(e) => e.currentTarget.select()}
                onChange={(e) =>
                  setGrid((current) => ({ ...current, [uuid]: e.target.value }))
                }
              />
            );
          },
        }),
        createColumn<ExtraStockRow>({
          accessorKey: "in_use_now",
          labelKey: "extras.stock.columns.in_use_now",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.in_use_now}</span>
          ),
        }),
        createColumn<ExtraStockRow>({
          accessorKey: "peak_committed",
          labelKey: "extras.stock.columns.peak_committed",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex items-center gap-2 tabular-nums">
              {row.original.peak_committed}
              {row.original.peak_committed > row.original.quantity ? (
                <StatusChip
                  label={t("extras.stock.over_committed")}
                  tone="danger"
                />
              ) : null}
            </span>
          ),
        }),
        createColumn<ExtraStockRow>({
          id: "updated_at",
          accessorFn: (row) => row.updated_at ?? "",
          labelKey: "extras.stock.columns.updated_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.updated_at
              ? datetime(row.original.updated_at, undefined, locale)
              : "—",
        }),
      ] as ColumnDef<ExtraStockRow, unknown>[],
    [canWrite, changed, grid, invalid, locale, t],
  );

  if (!extra.is_physical) {
    return (
      <EntitySectionCard title={t("extras.stock.title")}>
        <p className="text-muted-foreground text-sm">
          {t("extras.stock.not_physical")}
        </p>
      </EntitySectionCard>
    );
  }

  return (
    <EntitySectionCard title={t("extras.stock.title")}>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("extras.stock.help")}
      </p>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.branch.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("extras.stock.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-extra-stock-v1",
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
          sorting: false,
        }}
        toolbarExtra={
          <PermissionGuard permission={permissions.tenant.extras.write}>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!changed.length || save.isPending}
              onClick={() => setGrid({})}
            >
              <Undo2 className="size-4" />
              {t("extras.stock.reset")}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={!changed.length || invalid.length > 0 || save.isPending}
              onClick={() =>
                save.mutate(
                  {
                    uuid: extra.uuid,
                    body: {
                      items: changed.map((uuid) => ({
                        branch_uuid: uuid,
                        quantity: Number(grid[uuid]),
                      })),
                    },
                  },
                  { onSuccess: () => setGrid({}) },
                )
              }
            >
              <Save className="size-4" />
              {t("extras.stock.save", { count: changed.length })}
            </Button>
          </PermissionGuard>
        }
      />
    </EntitySectionCard>
  );
}
