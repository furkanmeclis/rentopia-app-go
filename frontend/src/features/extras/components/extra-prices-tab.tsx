"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Save, Undo2 } from "lucide-react";
import { useMemo, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityDetail,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { PRICE_SOURCE_LABEL_KEYS } from "@/features/extras/constants";
import {
  useExtraPriceHistory,
  useExtraPrices,
  useSetExtraPrices,
} from "@/features/extras/hooks/use-extras";
import type {
  Extra,
  ExtraPriceHistoryEntry,
  ExtraPricesRequest,
  ExtraProductPrice,
} from "@/features/extras/types";
import { localInputToISO } from "@/features/fleet-ops/lib/datetime";
import {
  isAmount,
  moneyText,
  normalizeAmount,
} from "@/features/pricing/lib/format";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Typed price cells: product uuid → text ("" = remove / fall back). */
type Grid = Record<string, string>;

const SOURCE_TONES: Record<string, "default" | "success" | "warning"> = {
  product: "success",
  default: "default",
  none: "warning",
};

/**
 * Per-product price matrix of an extra: the default price and one inline
 * editable price per rental product (empty = falls back to the default).
 * A save writes only the changed rows, from now or from a chosen moment;
 * old rows are closed, never changed (quotes keep their price row).
 */
export function ExtraPricesTab({ extra }: { extra: Extra }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.extras.write);
  const query = useExtraPrices(extra.uuid);
  const save = useSetExtraPrices();
  const matrix = query.data;
  const [grid, setGrid] = useState<Grid>({});
  const [defaultPrice, setDefaultPrice] = useState<string | null>(null);
  const [effectiveFrom, setEffectiveFrom] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);

  const baseline = useMemo<Grid>(() => {
    const out: Grid = {};
    for (const item of matrix?.items ?? []) {
      out[item.product.uuid] = item.price?.price ?? "";
    }
    return out;
  }, [matrix]);
  const baselineDefault = matrix?.default?.price ?? "";
  const changed = useMemo(
    () =>
      Object.keys(grid).filter(
        (uuid) =>
          normalizeAmount(grid[uuid]) !== normalizeAmount(baseline[uuid] ?? ""),
      ),
    [baseline, grid],
  );
  const invalid = useMemo(
    () =>
      changed.filter(
        (uuid) => grid[uuid].trim() !== "" && !isAmount(grid[uuid]),
      ),
    [changed, grid],
  );
  const defaultChanged =
    defaultPrice != null &&
    normalizeAmount(defaultPrice) !== normalizeAmount(baselineDefault);
  const defaultInvalid =
    defaultChanged && defaultPrice!.trim() !== "" && !isAmount(defaultPrice!);
  const dirty = changed.length > 0 || defaultChanged;

  const reset = () => {
    setGrid({});
    setDefaultPrice(null);
    setEffectiveFrom("");
  };

  const submit = () => {
    const body: ExtraPricesRequest = {
      items: changed.map((uuid) => ({
        rental_product_uuid: uuid,
        price: grid[uuid].trim() ? normalizeAmount(grid[uuid]) : null,
      })),
    };
    if (defaultChanged) {
      body.default_price = defaultPrice!.trim()
        ? normalizeAmount(defaultPrice!)
        : null;
    }
    if (effectiveFrom) body.effective_from = localInputToISO(effectiveFrom);
    save.mutate(
      { uuid: extra.uuid, body },
      {
        onSuccess: () => {
          reset();
          setConfirmOpen(false);
        },
        onError: () => setConfirmOpen(false),
      },
    );
  };

  const columns = useMemo(
    () =>
      [
        createColumn<ExtraProductPrice>({
          id: "product",
          accessorFn: (row) => row.product.code,
          labelKey: "extras.prices.columns.product",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="text-sm font-medium">
                <code className="text-xs">{row.original.product.code}</code>{" "}
                {row.original.product.title}
              </span>
              <span className="text-muted-foreground text-xs">
                {row.original.product.segment_name}
              </span>
            </span>
          ),
        }),
        createColumn<ExtraProductPrice>({
          id: "price",
          accessorFn: (row) => row.price?.price ?? "",
          labelKey: "extras.prices.columns.product_price",
          enableSorting: false,
          cell: ({ row }) => {
            const uuid = row.original.product.uuid;
            const isChanged = changed.includes(uuid);
            const isInvalid = invalid.includes(uuid);
            return (
              <Input
                value={grid[uuid] ?? baseline[uuid] ?? ""}
                readOnly={!canWrite}
                inputMode="decimal"
                placeholder={t("extras.prices.fallback_placeholder")}
                aria-label={`${row.original.product.code} ${t("extras.prices.columns.product_price")}`}
                aria-invalid={isInvalid || undefined}
                className={cn(
                  "h-8 w-36 text-right tabular-nums",
                  isChanged &&
                    "border-amber-500 bg-amber-50 dark:bg-amber-950/30",
                  isInvalid && "border-destructive",
                )}
                onClick={(e) => e.stopPropagation()}
                onFocus={(e) => e.currentTarget.select()}
                onChange={(e) =>
                  setGrid((current) => ({ ...current, [uuid]: e.target.value }))
                }
              />
            );
          },
        }),
        createColumn<ExtraProductPrice>({
          id: "effective",
          accessorFn: (row) => row.effective ?? "",
          labelKey: "extras.prices.columns.effective",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2">
              <span className="tabular-nums">
                {moneyText(row.original.effective, locale)}
              </span>
              <StatusChip
                label={t(
                  PRICE_SOURCE_LABEL_KEYS[row.original.source] ??
                    "extras.prices.source.none",
                )}
                tone={SOURCE_TONES[row.original.source] ?? "default"}
              />
            </span>
          ),
        }),
        createColumn<ExtraProductPrice>({
          id: "valid_from",
          accessorFn: (row) => row.price?.valid_from ?? "",
          labelKey: "extras.prices.columns.valid_from",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.price
              ? datetime(row.original.price.valid_from, undefined, locale)
              : "—",
        }),
      ] as ColumnDef<ExtraProductPrice, unknown>[],
    [baseline, canWrite, changed, grid, invalid, locale, t],
  );

  const scheduled = matrix?.scheduled ?? [];

  return (
    <div className="space-y-4">
      <EntitySectionCard title={t("extras.prices.title")}>
        <p className="text-muted-foreground mb-4 text-sm">
          {t("extras.prices.help")}
        </p>
        <div className="mb-4 grid gap-4 sm:grid-cols-[minmax(0,14rem)_minmax(0,16rem)_1fr] sm:items-end">
          <div className="space-y-2">
            <Label htmlFor="extra-default-price">
              {t("extras.prices.default_price")}
            </Label>
            <Input
              id="extra-default-price"
              value={defaultPrice ?? baselineDefault}
              readOnly={!canWrite}
              inputMode="decimal"
              placeholder="0,00"
              aria-invalid={defaultInvalid || undefined}
              className={cn(
                "text-right tabular-nums",
                defaultChanged &&
                  "border-amber-500 bg-amber-50 dark:bg-amber-950/30",
                defaultInvalid && "border-destructive",
              )}
              onChange={(e) => setDefaultPrice(e.target.value)}
            />
          </div>
          <PermissionGuard permission={permissions.tenant.extras.write}>
            <div className="space-y-2">
              <Label>{t("extras.prices.effective_from")}</Label>
              <DatePicker
                value={effectiveFrom}
                onChange={(v) => setEffectiveFrom(v ?? "")}
                withTime
                placeholder={t("extras.prices.effective_now")}
              />
            </div>
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={!dirty || save.isPending}
                onClick={reset}
              >
                <Undo2 className="size-4" />
                {t("extras.prices.reset")}
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={
                  !dirty ||
                  invalid.length > 0 ||
                  defaultInvalid ||
                  save.isPending
                }
                onClick={() => setConfirmOpen(true)}
              >
                <Save className="size-4" />
                {t("extras.prices.save", {
                  count: changed.length + (defaultChanged ? 1 : 0),
                })}
              </Button>
            </div>
          </PermissionGuard>
        </div>
        <EntityTable
          columns={columns}
          data={matrix?.items ?? []}
          getRowId={(row) => row.product.uuid}
          isLoading={query.isLoading}
          isError={query.isError}
          onRetry={() => void query.refetch()}
          emptyTitle={t("extras.prices.empty")}
          emptyDescription={t("extras.prices.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-extra-prices-v1",
            rowSelection: false,
            columnFilters: false,
            facetedFilters: false,
            sorting: false,
          }}
        />
        {scheduled.length ? (
          <div className="mt-4">
            <EntityDetail
              sections={[
                {
                  id: "scheduled",
                  title: t("extras.prices.scheduled"),
                  fields: scheduled.map((s) => {
                    const product = matrix?.items.find(
                      (i) => i.product.uuid === s.product_uuid,
                    );
                    return {
                      key: s.uuid,
                      label: product
                        ? `${product.product.code} · ${product.product.title}`
                        : t("extras.prices.default_price"),
                      value: `${moneyText(s.price, locale)} · ${t("extras.prices.from", { date: datetime(s.valid_from, undefined, locale) })}`,
                    };
                  }),
                },
              ]}
            />
          </div>
        ) : null}
      </EntitySectionCard>
      <PriceHistoryCard uuid={extra.uuid} />
      <ConfirmDialog
        open={confirmOpen}
        title={t("extras.prices.confirm_title")}
        description={
          effectiveFrom
            ? t("extras.prices.confirm_scheduled", {
                date: datetime(
                  localInputToISO(effectiveFrom) ?? effectiveFrom,
                  undefined,
                  locale,
                ),
              })
            : t("extras.prices.confirm_now")
        }
        isPending={save.isPending}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={submit}
      />
    </div>
  );
}

function PriceHistoryCard({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 });
  const query = useExtraPriceHistory(uuid, {
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });
  const total = query.data?.total ?? 0;
  const columns = useMemo(
    () =>
      [
        createColumn<ExtraPriceHistoryEntry>({
          id: "product",
          accessorFn: (row) => row.product?.code ?? "",
          labelKey: "extras.prices.columns.product",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.product ? (
              <span>
                <code className="text-xs">{row.original.product.code}</code>{" "}
                {row.original.product.title}
              </span>
            ) : (
              <StatusChip label={t("extras.prices.default_price")} />
            ),
        }),
        createColumn<ExtraPriceHistoryEntry>({
          accessorKey: "price",
          labelKey: "extras.prices.columns.price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.price, locale)}
            </span>
          ),
        }),
        createColumn<ExtraPriceHistoryEntry>({
          accessorKey: "valid_from",
          labelKey: "extras.prices.columns.valid_from",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.valid_from, undefined, locale),
        }),
        createColumn<ExtraPriceHistoryEntry>({
          id: "valid_to",
          accessorFn: (row) => row.valid_to ?? "",
          labelKey: "extras.prices.columns.valid_to",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.valid_to
              ? datetime(row.original.valid_to, undefined, locale)
              : t("extras.prices.open_ended"),
        }),
        createColumn<ExtraPriceHistoryEntry>({
          id: "actor",
          accessorFn: (row) => row.actor?.email ?? "",
          labelKey: "extras.prices.columns.actor",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.actor
              ? `${row.original.actor.name} ${row.original.actor.surname}`.trim() ||
                row.original.actor.email
              : "—",
        }),
        createColumn<ExtraPriceHistoryEntry>({
          accessorKey: "uuid",
          labelKey: "extras.prices.columns.price_row",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.uuid}
            </code>
          ),
        }),
      ] as ColumnDef<ExtraPriceHistoryEntry, unknown>[],
    [locale, t],
  );
  return (
    <EntitySectionCard
      title={t("extras.prices.history")}
      badge={query.data ? total : undefined}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("extras.prices.history_empty")}
        pageCount={Math.max(1, Math.ceil(total / pagination.pageSize))}
        state={{ pagination, onPaginationChange: setPagination }}
        features={{
          persistKey: "tenant-extra-price-history-v1",
          rowSelection: false,
          globalFilter: false,
          columnFilters: false,
          facetedFilters: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}
