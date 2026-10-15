"use client";

import {
  ArrowRight,
  ExternalLink,
  Play,
  Square,
  Unlock,
  X,
} from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { MAINTENANCE_STATE_LABEL_KEYS } from "@/features/fleet-calendar/constants";
import type {
  FleetCalendarInterval,
  FleetCalendarMaintenance,
  FleetCalendarVehicle,
} from "@/features/fleet-calendar/types";
import {
  BlockKindBadge,
  TransferStatusChip,
} from "@/features/fleet-ops/components/fleet-ops-cells";
import type { TransferAction } from "@/features/fleet-ops/types";
import { MAINTENANCE_TYPE_LABEL_KEYS } from "@/features/vehicle-records/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** What a bar's popover asks the page to do (dialogs live on the page). */
export type CalendarAction =
  | {
      type: "release-block";
      vehicle: FleetCalendarVehicle;
      interval: FleetCalendarInterval;
    }
  | { type: "transfer"; action: TransferAction; transferUuid: string };

function Field({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 tabular-nums">{value}</span>
    </div>
  );
}

function vehicleTabHref(slug: string, vehicleUuid: string, tab: string) {
  return `${routes.tenant.fleet.vehicles.detail(slug, vehicleUuid)}?tab=${tab}`;
}

/** Whether a block covers now ("Engeli kapat" is offered). */
export function blockIsActive(interval: FleetCalendarInterval, now: Date) {
  const start = new Date(interval.starts_at);
  const end = interval.ends_at ? new Date(interval.ends_at) : null;
  return start <= now && (!end || end > now);
}

/** Popover body of a block or transfer bar. */
export function IntervalDetails({
  vehicle,
  interval,
  onAction,
  showVehicleLink = true,
}: {
  vehicle: FleetCalendarVehicle;
  interval: FleetCalendarInterval;
  onAction?: (action: CalendarAction) => void;
  showVehicleLink?: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const when = (value: string | null | undefined) =>
    value ? datetime(value, undefined, locale) : "—";

  if (interval.block) {
    const block = interval.block;
    const active = blockIsActive(interval, new Date());
    return (
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <BlockKindBadge kind={block.kind} />
          {active ? (
            <StatusChip
              label={t("vehicle_blocks.state.active")}
              tone="danger"
            />
          ) : null}
        </div>
        <div className="space-y-1.5">
          <Field
            label={t("fleet_calendar.details.vehicle")}
            value={`${vehicle.plate} · ${vehicle.label}`}
          />
          <Field
            label={t("fleet_calendar.details.starts_at")}
            value={when(interval.starts_at)}
          />
          <Field
            label={t("fleet_calendar.details.ends_at")}
            value={
              interval.ends_at
                ? when(interval.ends_at)
                : t("vehicle_blocks.open_ended")
            }
          />
          {block.reason ? (
            <Field
              label={t("vehicle_blocks.fields.reason")}
              value={block.reason}
            />
          ) : null}
        </div>
        <Separator />
        <div className="flex flex-wrap gap-2">
          {active && onAction && can(permissions.tenant.blocks.write) ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() =>
                onAction({ type: "release-block", vehicle, interval })
              }
            >
              <Unlock className="size-4" />
              {t("vehicle_blocks.actions.release")}
            </Button>
          ) : null}
          {showVehicleLink ? (
            <Button asChild size="sm" variant="ghost">
              <Link href={vehicleTabHref(slug, vehicle.uuid, "blocks")}>
                <ExternalLink className="size-4" />
                {t("fleet_calendar.details.open_blocks")}
              </Link>
            </Button>
          ) : null}
        </div>
      </div>
    );
  }

  if (interval.allocation) {
    return (
      <AllocationDetails
        vehicle={vehicle}
        interval={interval}
        allocation={interval.allocation}
      />
    );
  }

  const transfer = interval.transfer;
  if (!transfer) return null;
  const canWrite =
    transfer.in_scope && can(permissions.tenant.transfers.write) && onAction;
  const run = (action: TransferAction) =>
    onAction?.({ type: "transfer", action, transferUuid: interval.uuid });
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <TransferStatusChip status={transfer.status} />
        <span className="flex items-center gap-1 text-sm font-medium">
          {transfer.from_branch.code}
          <ArrowRight className="text-muted-foreground size-3.5" />
          {transfer.to_branch.code}
        </span>
      </div>
      <div className="space-y-1.5">
        <Field
          label={t("fleet_calendar.details.vehicle")}
          value={`${vehicle.plate} · ${vehicle.label}`}
        />
        <Field
          label={t("fleet_calendar.details.route")}
          value={`${transfer.from_branch.name} → ${transfer.to_branch.name}`}
        />
        <Field
          label={t("fleet_calendar.details.planned")}
          value={`${when(transfer.planned_departure)} – ${when(transfer.planned_arrival)}`}
        />
        {transfer.actual_departure ? (
          <Field
            label={t("fleet_calendar.details.actual")}
            value={`${when(transfer.actual_departure)} – ${
              transfer.actual_arrival
                ? when(transfer.actual_arrival)
                : t("fleet_calendar.details.on_the_way")
            }`}
          />
        ) : null}
      </div>
      {!transfer.in_scope ? (
        <p className="text-muted-foreground text-xs">
          {t("fleet_calendar.details.out_of_scope")}
        </p>
      ) : null}
      <Separator />
      <div className="flex flex-wrap gap-2">
        {canWrite && transfer.status === "planned" ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => run("start")}
          >
            <Play className="size-4" />
            {t("transfers.actions.start")}
          </Button>
        ) : null}
        {canWrite && transfer.status === "in_transit" ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => run("complete")}
          >
            <Square className="size-4" />
            {t("transfers.actions.complete")}
          </Button>
        ) : null}
        {canWrite &&
        (transfer.status === "planned" || transfer.status === "in_transit") ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="text-destructive hover:text-destructive"
            onClick={() => run("cancel")}
          >
            <X className="size-4" />
            {t("transfers.actions.cancel")}
          </Button>
        ) : null}
        {showVehicleLink ? (
          <Button asChild size="sm" variant="ghost">
            <Link href={vehicleTabHref(slug, vehicle.uuid, "transfers")}>
              <ExternalLink className="size-4" />
              {t("fleet_calendar.details.open_transfers")}
            </Link>
          </Button>
        ) : null}
      </div>
    </div>
  );
}

/**
 * Popover body of a hold / reservation bar: the product, the customer range,
 * when the vehicle is sellable again (after the preparation buffer), the
 * hold expiry and whether the plate is promised or only planned.
 */
function AllocationDetails({
  vehicle,
  interval,
  allocation,
}: {
  vehicle: FleetCalendarVehicle;
  interval: FleetCalendarInterval;
  allocation: NonNullable<FleetCalendarInterval["allocation"]>;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const when = (value: string | null | undefined) =>
    value ? datetime(value, undefined, locale) : "—";
  const hold = interval.kind === "hold";
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <StatusChip
          label={
            hold
              ? t("fleet_calendar.bar.hold")
              : t("fleet_calendar.bar.reservation")
          }
          tone={hold ? "warning" : "success"}
        />
        <StatusChip
          label={
            allocation.plate_fixed
              ? t("fleet_calendar.details.plate_fixed")
              : t("fleet_calendar.details.plate_provisional")
          }
        />
      </div>
      <div className="space-y-1.5">
        <Field
          label={t("fleet_calendar.details.vehicle")}
          value={`${vehicle.plate} · ${vehicle.label}`}
        />
        <Field
          label={t("fleet_calendar.details.product")}
          value={`${allocation.product.code} · ${allocation.product.title}`}
        />
        <Field
          label={t("fleet_calendar.details.route")}
          value={`${allocation.pickup_branch.name} → ${allocation.return_branch.name}`}
        />
        <Field
          label={t("fleet_calendar.details.starts_at")}
          value={when(allocation.starts_at)}
        />
        <Field
          label={t("fleet_calendar.details.ends_at")}
          value={when(allocation.ends_at)}
        />
        <Field
          label={t("fleet_calendar.details.ready_at")}
          value={when(interval.ends_at)}
        />
        {allocation.expires_at ? (
          <Field
            label={t("fleet_calendar.details.expires_at")}
            value={when(allocation.expires_at)}
          />
        ) : null}
      </div>
      {can(permissions.tenant.availability.read) ? (
        <>
          <Separator />
          <Button asChild size="sm" variant="ghost">
            <Link href={routes.tenant.availability(slug)}>
              <ExternalLink className="size-4" />
              {t("fleet_calendar.details.open_availability")}
            </Link>
          </Button>
        </>
      ) : null}
    </div>
  );
}

/** Popover body of a maintenance day marker. */
export function MaintenanceDetails({
  vehicle,
  item,
  showVehicleLink = true,
}: {
  vehicle: FleetCalendarVehicle;
  item: FleetCalendarMaintenance;
  showVehicleLink?: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const km = item.kind === "done" ? item.done_km : item.planned_km;
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">
          {t(MAINTENANCE_TYPE_LABEL_KEYS[item.maintenance_type])}
        </span>
        <StatusChip
          label={t(MAINTENANCE_STATE_LABEL_KEYS[item.state])}
          tone={
            item.state === "overdue"
              ? "danger"
              : item.state === "due"
                ? "warning"
                : item.state === "done"
                  ? "success"
                  : "default"
          }
        />
      </div>
      <div className="space-y-1.5">
        <Field
          label={t("fleet_calendar.details.vehicle")}
          value={`${vehicle.plate} · ${vehicle.label}`}
        />
        <Field
          label={
            item.kind === "done"
              ? t("fleet_calendar.details.done_date")
              : t("fleet_calendar.details.planned_date")
          }
          value={date(item.date, undefined, locale)}
        />
        {km != null ? (
          <Field
            label={t("fleet_calendar.details.km")}
            value={`${new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-US").format(km)} km`}
          />
        ) : null}
        {item.service_provider ? (
          <Field
            label={t("fleet_calendar.details.provider")}
            value={item.service_provider}
          />
        ) : null}
      </div>
      {showVehicleLink ? (
        <>
          <Separator />
          <Button asChild size="sm" variant="ghost">
            <Link href={vehicleTabHref(slug, vehicle.uuid, "maintenance")}>
              <ExternalLink className="size-4" />
              {t("fleet_calendar.details.open_maintenance")}
            </Link>
          </Button>
        </>
      ) : null}
    </div>
  );
}
