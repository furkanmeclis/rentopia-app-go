"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Minus, Plus, TicketPercent } from "lucide-react";
import { useMemo } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { COUPON_REASON_KEYS } from "@/features/campaigns/constants";
import {
  CHARGE_LABEL_KEYS,
  TYPE_LABEL_KEYS,
} from "@/features/extras/constants";
import { useExtrasList } from "@/features/extras/hooks/use-extras";
import { extrasService } from "@/features/extras/services/extras.service";
import type { Extra } from "@/features/extras/types";
import { moneyText } from "@/features/pricing/lib/format";
import { reasonText } from "@/features/reservations/constants";
import type { QuoteSearchRequest } from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type WizardDiscount = {
  coupon: string;
  manualKind: "percent" | "amount";
  manualValue: string;
  manualReason: string;
};

/** Quantity stepper of one extra (min 0 = not chosen, max from the catalog). */
function QuantityControl({
  extra,
  value,
  available,
  onChange,
}: {
  extra: Extra;
  value: number;
  available: number | null;
  onChange: (quantity: number) => void;
}) {
  const { t } = useLocale();
  const max = Math.min(
    extra.charge_type === "per_rental" ? 1 : extra.max_quantity || 1,
    available ?? Number.MAX_SAFE_INTEGER,
  );
  const min = extra.min_quantity || 1;
  return (
    <div className="flex items-center gap-1">
      <Button
        type="button"
        size="icon"
        variant="outline"
        className="size-7"
        aria-label={t("reservations.extras.decrease")}
        disabled={value === 0}
        onClick={() => onChange(value <= min ? 0 : value - 1)}
      >
        <Minus className="size-3.5" />
      </Button>
      <span className="w-8 text-center tabular-nums">{value}</span>
      <Button
        type="button"
        size="icon"
        variant="outline"
        className="size-7"
        aria-label={t("reservations.extras.increase")}
        disabled={value >= max}
        onClick={() => onChange(value === 0 ? min : value + 1)}
      >
        <Plus className="size-3.5" />
      </Button>
    </div>
  );
}

/**
 * Extras (published catalog for the sales channel, physical stock at the
 * pick-up branch for the range) and the discount: a coupon code, or a
 * manual discount with a reason for members with `tenant.discount.apply`.
 * Prices and the coupon are evaluated by the pricing engine when the quote
 * is made.
 */
export function WizardExtrasStep({
  search,
  quantities,
  onQuantitiesChange,
  discount,
  onDiscountChange,
  couponError,
}: {
  search: QuoteSearchRequest;
  quantities: Record<string, number>;
  onQuantitiesChange: (next: Record<string, number>) => void;
  discount: WizardDiscount;
  onDiscountChange: (next: WizardDiscount) => void;
  couponError: string | null;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canReadExtras = can(permissions.tenant.extras.read);
  const extrasQuery = useExtrasList(
    {
      limit: 100,
      offset: 0,
      sort: "sort_order",
      status: "published",
      channel: search.channel,
    },
    canReadExtras,
  );
  const extras = useMemo(
    () => extrasQuery.data?.items ?? [],
    [extrasQuery.data],
  );
  const hasPhysical = extras.some((e) => e.is_physical);
  const stockQuery = useQuery({
    queryKey: [
      "tenant",
      slug,
      "reservations",
      "wizard-stock",
      search.pickup_branch_uuid,
      search.starts_at,
      search.ends_at,
    ],
    queryFn: () =>
      extrasService.availability(slug, {
        branch_uuid: search.pickup_branch_uuid!,
        starts_at: search.starts_at!,
        ends_at: search.ends_at!,
      }),
    enabled:
      canReadExtras &&
      hasPhysical &&
      Boolean(search.pickup_branch_uuid && search.starts_at && search.ends_at),
  });
  const availableOf = useMemo(() => {
    const map = new Map<string, number>();
    for (const item of stockQuery.data?.items ?? []) {
      map.set(item.extra.uuid, item.available);
    }
    return map;
  }, [stockQuery.data]);

  const columns = useMemo(
    () =>
      [
        createColumn<Extra>({
          accessorKey: "name",
          labelKey: "reservations.extras.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.name}</span>
              <span className="text-muted-foreground text-xs">
                {t(TYPE_LABEL_KEYS[row.original.extra_type])}
              </span>
            </span>
          ),
        }),
        createColumn<Extra>({
          accessorKey: "charge_type",
          labelKey: "reservations.extras.columns.charge_type",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(CHARGE_LABEL_KEYS[row.original.charge_type])}
            </Badge>
          ),
        }),
        createColumn<Extra>({
          id: "price",
          accessorFn: (row) => Number(row.default_price ?? 0),
          labelKey: "reservations.extras.columns.price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.default_price, locale)}
            </span>
          ),
        }),
        createColumn<Extra>({
          id: "available",
          accessorFn: (row) => availableOf.get(row.uuid) ?? -1,
          labelKey: "reservations.extras.columns.available",
          enableSorting: false,
          cell: ({ row }) => {
            if (!row.original.is_physical) {
              return (
                <span className="text-muted-foreground text-xs">
                  {t("reservations.extras.not_physical")}
                </span>
              );
            }
            const available = availableOf.get(row.original.uuid);
            if (available === undefined) return "—";
            return (
              <Badge variant={available > 0 ? "success" : "danger"}>
                {available}
              </Badge>
            );
          },
        }),
        createColumn<Extra>({
          id: "quantity",
          labelKey: "reservations.extras.columns.quantity",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const extra = row.original;
            const available = extra.is_physical
              ? (availableOf.get(extra.uuid) ?? null)
              : null;
            return (
              <QuantityControl
                extra={extra}
                value={quantities[extra.uuid] ?? 0}
                available={available}
                onChange={(quantity) => {
                  const next = { ...quantities };
                  if (quantity > 0) next[extra.uuid] = quantity;
                  else delete next[extra.uuid];
                  onQuantitiesChange(next);
                }}
              />
            );
          },
        }),
      ] as ColumnDef<Extra, unknown>[],
    [availableOf, locale, onQuantitiesChange, quantities, t],
  );

  const manualActive =
    discount.coupon.trim() === "" &&
    (discount.manualValue.trim() !== "" || discount.manualReason !== "");

  return (
    <div className="space-y-6">
      {canReadExtras ? (
        <EntityTable
          columns={columns}
          data={extras}
          getRowId={(row) => row.uuid}
          isLoading={extrasQuery.isLoading}
          isError={extrasQuery.isError}
          onRetry={() => void extrasQuery.refetch()}
          emptyTitle={t("reservations.extras.empty")}
          emptyDescription={t("reservations.extras.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-reservation-wizard-extras-v1",
            rowSelection: false,
            sorting: false,
          }}
        />
      ) : (
        <Alert>
          <AlertDescription>
            {t("reservations.extras.no_permission")}
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-2 rounded-lg border p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <TicketPercent className="size-4" />
            {t("reservations.discount.coupon_title")}
          </h3>
          <div className="grid gap-1.5">
            <Label htmlFor="wizard-coupon">
              {t("reservations.discount.coupon_code")}
            </Label>
            <Input
              id="wizard-coupon"
              value={discount.coupon}
              disabled={manualActive}
              aria-invalid={Boolean(couponError)}
              placeholder={t("reservations.discount.coupon_placeholder")}
              onChange={(e) =>
                onDiscountChange({
                  ...discount,
                  coupon: e.target.value.toUpperCase(),
                })
              }
            />
            {couponError ? (
              <p className="text-destructive text-sm">
                {reasonText(t, COUPON_REASON_KEYS, couponError)}
              </p>
            ) : (
              <p className="text-muted-foreground text-xs">
                {t("reservations.discount.coupon_hint")}
              </p>
            )}
          </div>
        </div>
        <PermissionGuard permission={permissions.tenant.discount.apply}>
          <div className="space-y-3 rounded-lg border p-4">
            <h3 className="text-sm font-semibold">
              {t("reservations.discount.manual_title")}
            </h3>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="wizard-discount-kind">
                  {t("reservations.discount.kind")}
                </Label>
                <Select
                  value={discount.manualKind}
                  disabled={discount.coupon.trim() !== ""}
                  onValueChange={(manualKind) =>
                    onDiscountChange({
                      ...discount,
                      manualKind: manualKind as WizardDiscount["manualKind"],
                    })
                  }
                >
                  <SelectTrigger id="wizard-discount-kind">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="percent">
                      {t("reservations.discount.percent")}
                    </SelectItem>
                    <SelectItem value="amount">
                      {t("reservations.discount.amount")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="wizard-discount-value">
                  {t("reservations.discount.value")}
                </Label>
                <Input
                  id="wizard-discount-value"
                  inputMode="decimal"
                  value={discount.manualValue}
                  disabled={discount.coupon.trim() !== ""}
                  onChange={(e) =>
                    onDiscountChange({
                      ...discount,
                      manualValue: e.target.value,
                    })
                  }
                />
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="wizard-discount-reason">
                {t("reservations.discount.reason")}
              </Label>
              <Input
                id="wizard-discount-reason"
                value={discount.manualReason}
                disabled={discount.coupon.trim() !== ""}
                onChange={(e) =>
                  onDiscountChange({
                    ...discount,
                    manualReason: e.target.value,
                  })
                }
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {t("reservations.discount.manual_hint")}
            </p>
          </div>
        </PermissionGuard>
      </div>
    </div>
  );
}
