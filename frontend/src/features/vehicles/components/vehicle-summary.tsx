"use client";

import {
  ArrowRight,
  CalendarClock,
  FileCheck2,
  ShieldCheck,
  Truck,
  Wrench,
  OctagonAlert,
} from "lucide-react";
import type { ReactNode } from "react";

import { Card, CardContent } from "@/components/common/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { BLOCK_KIND_LABEL_KEYS } from "@/features/fleet-ops/constants";
import {
  useTransfersList,
  useVehicleBlocks,
} from "@/features/fleet-ops/hooks/use-fleet-ops";
import {
  DaysLeft,
  DocumentStatusChip,
  VehicleMaintenanceChip,
} from "@/features/vehicle-records/components/record-cells";
import type { VehicleDocumentStatus } from "@/features/vehicle-records/types";
import type { Vehicle } from "@/features/vehicles/types";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Reminder horizon of documents (backend DocumentReminderDays). */
const DOCUMENT_HORIZON_DAYS = 30;

function daysUntil(day: string): number {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const target = new Date(`${day}T00:00:00`);
  return Math.round((target.getTime() - today.getTime()) / 86_400_000);
}

function expiryStatus(day: string | null | undefined): {
  status: VehicleDocumentStatus;
  days: number | null;
} {
  if (!day) return { status: "none", days: null };
  const days = daysUntil(day);
  if (days < 0) return { status: "expired", days };
  if (days <= DOCUMENT_HORIZON_DAYS) return { status: "expiring", days };
  return { status: "valid", days };
}

function SummaryCard({
  icon,
  title,
  value,
  chip,
  hint,
  actionLabel,
  onAction,
}: {
  icon: ReactNode;
  title: string;
  value: ReactNode;
  chip: ReactNode;
  hint?: ReactNode;
  actionLabel: string;
  onAction: () => void;
}) {
  return (
    <Card className="shadow-none">
      <CardContent className="flex h-full flex-col gap-2 p-4">
        <div className="text-muted-foreground flex items-center gap-2 text-sm">
          {icon}
          <span className="font-medium">{title}</span>
        </div>
        <p className="font-display text-lg font-semibold tabular-nums">
          {value}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          {chip}
          {hint}
        </div>
        <Button
          type="button"
          variant="link"
          size="sm"
          className="mt-auto h-auto self-start px-0"
          onClick={onAction}
        >
          {actionLabel}
          <ArrowRight className="size-3.5" />
        </Button>
      </CardContent>
    </Card>
  );
}

/**
 * Expiry summary of the "Özet" tab: next inspection, traffic insurance,
 * kasko policy and maintenance, each with its state chip.
 */
export function VehicleExpiryCards({
  vehicle,
  onOpenTab,
}: {
  vehicle: Vehicle;
  onOpenTab: (tab: string) => void;
}) {
  const { t, locale } = useLocale();
  const docs = [
    {
      key: "inspection",
      icon: <FileCheck2 className="size-4" />,
      title: t("vehicles.summary.inspection"),
      value: vehicle.inspection_until,
    },
    {
      key: "insurance",
      icon: <ShieldCheck className="size-4" />,
      title: t("vehicles.summary.insurance"),
      value: vehicle.insurance_until,
    },
    {
      key: "kasko",
      icon: <ShieldCheck className="size-4" />,
      title: t("vehicles.summary.kasko_policy"),
      value: vehicle.kasko_until,
    },
  ];
  const nf = new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US");
  const maintenanceValue = vehicle.next_maintenance_date
    ? date(vehicle.next_maintenance_date, undefined, locale)
    : vehicle.next_maintenance_km != null
      ? `${nf.format(vehicle.next_maintenance_km)} km`
      : t("vehicles.summary.none");

  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      {docs.map((doc) => {
        const { status, days } = expiryStatus(doc.value);
        return (
          <SummaryCard
            key={doc.key}
            icon={doc.icon}
            title={doc.title}
            value={
              doc.value
                ? date(doc.value, undefined, locale)
                : t("vehicles.summary.none")
            }
            chip={<DocumentStatusChip status={status} />}
            hint={<DaysLeft days={days} />}
            actionLabel={t("vehicles.summary.open_documents")}
            onAction={() => onOpenTab("documents")}
          />
        );
      })}
      <SummaryCard
        icon={<Wrench className="size-4" />}
        title={t("vehicles.summary.maintenance")}
        value={maintenanceValue}
        chip={
          vehicle.maintenance_status === "none" ? (
            <span className="text-muted-foreground text-sm">
              {t("vehicles.summary.no_plan")}
            </span>
          ) : (
            <VehicleMaintenanceChip status={vehicle.maintenance_status} />
          )
        }
        hint={
          vehicle.next_maintenance_date &&
          vehicle.next_maintenance_km != null ? (
            <span className="text-muted-foreground text-xs tabular-nums">
              {nf.format(vehicle.next_maintenance_km)} km
            </span>
          ) : null
        }
        actionLabel={t("vehicles.summary.open_maintenance")}
        onAction={() => onOpenTab("maintenance")}
      />
    </div>
  );
}

/**
 * Banners on top of the "Özet" tab: the block covering now and the
 * transfer on the way (or the next planned one). Each needs its own read
 * permission; without it the banner is simply not shown.
 */
export function VehicleOperationBanners({
  vehicleUuid,
  onOpenTab,
}: {
  vehicleUuid: string;
  onOpenTab: (tab: string) => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const blocks = useVehicleBlocks(
    vehicleUuid,
    { limit: 1, offset: 0, state: "active" },
    can(permissions.tenant.blocks.read),
  );
  const transfers = useTransfersList(
    {
      limit: 5,
      offset: 0,
      vehicle: vehicleUuid,
      status: "planned,in_transit",
      sort: "planned_departure",
    },
    can(permissions.tenant.transfers.read),
  );
  const block = blocks.data?.items[0];
  const open = transfers.data?.items ?? [];
  const transfer =
    open.find((item) => item.status === "in_transit") ?? open[0] ?? null;
  if (!block && !transfer) return null;

  return (
    <div className="grid gap-3 lg:grid-cols-2">
      {block ? (
        <Alert variant="destructive">
          <OctagonAlert className="size-4" />
          <AlertTitle>
            {t("vehicles.summary.active_block", {
              kind: t(BLOCK_KIND_LABEL_KEYS[block.kind]),
            })}
          </AlertTitle>
          <AlertDescription>
            <p>
              {datetime(block.starts_at, undefined, locale)} –{" "}
              {block.ends_at
                ? datetime(block.ends_at, undefined, locale)
                : t("vehicle_blocks.open_ended")}
              {block.reason ? ` · ${block.reason}` : ""}
            </p>
            <Button
              type="button"
              variant="link"
              size="sm"
              className="h-auto px-0"
              onClick={() => onOpenTab("blocks")}
            >
              {t("vehicles.summary.open_blocks")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {transfer ? (
        <Alert>
          {transfer.status === "in_transit" ? (
            <Truck className="size-4" />
          ) : (
            <CalendarClock className="size-4" />
          )}
          <AlertTitle>
            {transfer.status === "in_transit"
              ? t("vehicles.summary.in_transit", {
                  from: transfer.from_branch.name,
                  to: transfer.to_branch.name,
                })
              : t("vehicles.summary.planned_transfer", {
                  from: transfer.from_branch.name,
                  to: transfer.to_branch.name,
                })}
          </AlertTitle>
          <AlertDescription>
            <p>
              {t("vehicles.summary.transfer_window", {
                departure: datetime(
                  transfer.actual_departure ?? transfer.planned_departure,
                  undefined,
                  locale,
                ),
                arrival: datetime(transfer.planned_arrival, undefined, locale),
              })}
            </p>
            <Button
              type="button"
              variant="link"
              size="sm"
              className="h-auto px-0"
              onClick={() => onOpenTab("transfers")}
            >
              {t("vehicles.summary.open_transfers")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  );
}
