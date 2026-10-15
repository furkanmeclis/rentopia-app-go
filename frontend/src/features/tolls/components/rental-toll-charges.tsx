"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo } from "react";

import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import type { RentalCharge } from "@/features/rentals/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tollMoney } from "@/features/tolls/components/toll-cells";
import { TOLL_CHARGE_KIND_LABEL_KEYS } from "@/features/tolls/constants";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Calc = {
  station?: string;
  exit_station?: string;
  passed_at?: string;
  plate?: string;
  external_id?: string;
};

function calcOf(charge: RentalCharge): Calc {
  return (charge.calculation ?? {}) as Calc;
}

/** Sum of decimal strings in minor units (no float drift). */
function total(charges: RentalCharge[]): string {
  let minor = 0;
  for (const c of charges) {
    const n = Math.round(Number(c.amount) * 100);
    if (Number.isFinite(n)) minor += n;
  }
  return (minor / 100).toFixed(2);
}

/**
 * Rental detail "HGS / OGS geçişleri": the toll charges of the rental (each
 * pass and its processing fee as their own lines, kept apart from the
 * rent), reversals included, with the net total and a link to each pass.
 */
export function RentalTollCharges({
  rentalUuid,
  charges,
  currency,
}: {
  rentalUuid: string;
  charges: RentalCharge[];
  currency: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canRead = can(permissions.tenant.tolls.read);
  const columns = useMemo(
    () =>
      [
        createColumn<RentalCharge>({
          id: "pass",
          accessorFn: (row) => calcOf(row).station ?? "",
          labelKey: "tolls.rental.columns.pass",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const c = calcOf(row.original);
            const label =
              row.original.kind === "toll"
                ? c.exit_station
                  ? `${c.station} → ${c.exit_station}`
                  : (c.station ?? "—")
                : t(TOLL_CHARGE_KIND_LABEL_KEYS.toll_fee);
            return (
              <span className="flex flex-wrap items-center gap-1.5">
                {canRead && row.original.toll_pass_uuid ? (
                  <Link
                    href={routes.tenant.tolls.detail(
                      slug,
                      row.original.toll_pass_uuid,
                    )}
                    className="font-medium underline-offset-4 hover:underline"
                  >
                    {label}
                  </Link>
                ) : (
                  <span className="font-medium">{label}</span>
                )}
                {row.original.reversal_of_uuid ? (
                  <Badge
                    variant="secondary"
                    className="px-1.5 py-0 text-[10px]"
                  >
                    {t("tolls.charges.reversal")}
                  </Badge>
                ) : null}
                {row.original.reversed ? (
                  <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                    {t("tolls.charges.reversed")}
                  </Badge>
                ) : null}
              </span>
            );
          },
        }),
        createColumn<RentalCharge>({
          id: "passed_at",
          accessorFn: (row) => calcOf(row).passed_at ?? "",
          labelKey: "tolls.rental.columns.passed_at",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => {
            const at = calcOf(row.original).passed_at;
            return at ? (
              <span className="text-sm tabular-nums">
                {datetime(at, "d MMM yyyy HH:mm", locale)}
              </span>
            ) : (
              "—"
            );
          },
        }),
        createColumn<RentalCharge>({
          accessorKey: "kind",
          labelKey: "tolls.rental.columns.kind",
          enableSorting: false,
          cell: ({ row }) =>
            t(
              TOLL_CHARGE_KIND_LABEL_KEYS[row.original.kind] ??
                row.original.kind,
            ),
        }),
        createColumn<RentalCharge>({
          accessorKey: "amount",
          labelKey: "tolls.rental.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span
              className={
                row.original.reversal_of_uuid || row.original.reversed
                  ? "text-muted-foreground tabular-nums"
                  : "font-medium tabular-nums"
              }
            >
              {tollMoney(row.original.amount, row.original.currency, locale)}
            </span>
          ),
        }),
        createColumn<RentalCharge>({
          accessorKey: "reason",
          labelKey: "tolls.rental.columns.reason",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.reason || "—",
        }),
        createColumn<RentalCharge>({
          accessorKey: "created_at",
          labelKey: "tolls.rental.columns.created_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
      ] as ColumnDef<RentalCharge, unknown>[],
    [canRead, locale, slug, t],
  );
  const net = total(charges);
  return (
    <EntitySectionCard
      title={t("tolls.rental.title")}
      badge={
        charges.filter(
          (c) => c.kind === "toll" && !c.reversal_of_uuid && !c.reversed,
        ).length
      }
      actions={
        canRead ? (
          <Button asChild size="sm" variant="outline">
            <Link
              href={`${routes.tenant.tolls.root(slug)}?rental=${rentalUuid}`}
            >
              {t("tolls.rental.open_list")}
            </Link>
          </Button>
        ) : null
      }
    >
      <div className="space-y-3">
        <EntityTable
          columns={columns}
          data={charges}
          getRowId={(row) => row.uuid}
          emptyTitle={t("tolls.rental.empty")}
          emptyDescription={t("tolls.rental.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-rental-toll-charges-v1",
            rowSelection: false,
            globalFilter: false,
            sorting: false,
          }}
        />
        {charges.length ? (
          <div className="flex items-center justify-end gap-2 text-sm">
            <span className="text-muted-foreground">
              {t("tolls.rental.net")}
            </span>
            <span className="font-semibold tabular-nums">
              {tollMoney(net, currency, locale)}
            </span>
          </div>
        ) : null}
      </div>
    </EntitySectionCard>
  );
}
