"use client";

import { ChevronLeft, ChevronRight, Wrench } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { routes } from "@/config/routes";
import {
  type CalendarAction,
  IntervalDetails,
  MaintenanceDetails,
} from "@/features/fleet-calendar/components/bar-details";
import {
  BLOCK_BAR_CLASSES,
  MAINTENANCE_MARKER_CLASSES,
  MAINTENANCE_STATE_LABEL_KEYS,
  TRANSFER_BAR_CLASSES,
} from "@/features/fleet-calendar/constants";
import {
  type CalendarWindow,
  dayRange,
  packLanes,
  timelineColumns,
  trackWidth,
  xOf,
} from "@/features/fleet-calendar/lib/timeline";
import type {
  CalendarZoom,
  FleetCalendarInterval,
  FleetCalendarMaintenance,
  FleetCalendarRow,
  FleetCalendarVehicle,
} from "@/features/fleet-calendar/types";
import {
  BLOCK_KIND_LABEL_KEYS,
  TRANSFER_STATUS_LABEL_KEYS,
} from "@/features/fleet-ops/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  PlateBadge,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import { MAINTENANCE_TYPE_LABEL_KEYS } from "@/features/vehicle-records/constants";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const VEHICLE_COLUMN = 232;
const HEADER_HEIGHT = 44;
const LANE_HEIGHT = 28;
const BAR_HEIGHT = 22;
const ROW_PADDING = 8;
const MIN_ROW_HEIGHT = 56;
const MIN_BAR_WIDTH = 8;
const MARKER_SIZE = 22;

/**
 * Drag & drop extension point. Reservations (M2) will be draggable onto
 * another vehicle row ("plaka değişimi"); blocks, transfers and maintenance
 * never are. Until then no bar passes `canDragBar`, so the timeline renders
 * the affordance disabled and `onBarDrop` is never called.
 */
export type TimelineDragProps = {
  canDragBar?: (bar: FleetCalendarInterval) => boolean;
  onBarDrop?: (
    bar: FleetCalendarInterval,
    fromVehicleUuid: string,
    toVehicleUuid: string,
  ) => void;
};

type FleetTimelineProps = TimelineDragProps & {
  rows: FleetCalendarRow[];
  zoom: CalendarZoom;
  window: CalendarWindow;
  /** Single-vehicle variant (vehicle page): no vehicle column, no links. */
  compact?: boolean;
  onAction?: (action: CalendarAction) => void;
  /** Scroll area max height (rows scroll under the sticky time header). */
  maxHeight?: number | string;
  className?: string;
};

function useNow(intervalMs = 60_000) {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

function barClasses(interval: FleetCalendarInterval) {
  if (interval.block) return BLOCK_BAR_CLASSES[interval.block.kind];
  if (interval.transfer) return TRANSFER_BAR_CLASSES[interval.transfer.status];
  return "bg-muted text-foreground border-border";
}

type Lane =
  | { kind: "interval"; interval: FleetCalendarInterval; lane: number }
  | { kind: "ghost"; interval: FleetCalendarInterval; lane: number };

/**
 * Vehicles × time grid: sticky vehicle column and time header, horizontal
 * scroll, a "now" line, bars for blocks and transfers (planned range as a
 * dashed ghost behind the actual one) and maintenance day markers. Every
 * bar is a button: Tab reaches it, Enter / Space open its popover.
 */
export function FleetTimeline({
  rows,
  zoom,
  window,
  compact = false,
  onAction,
  maxHeight = "70vh",
  className,
  canDragBar,
  onBarDrop,
}: FleetTimelineProps) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const now = useNow();
  const scrollRef = useRef<HTMLDivElement>(null);
  const columns = useMemo(() => timelineColumns(zoom, window), [zoom, window]);
  const width = trackWidth(zoom, columns.length);
  const vehicleColumn = compact ? 0 : VEHICLE_COLUMN;
  const nowVisible = now >= window.from && now < window.to;
  const nowX = nowVisible ? xOf(now, window, width) : null;
  const [dragOver, setDragOver] = useState<string | null>(null);

  // Bring "now" (or the window start) into view when the window changes.
  const windowKey = `${zoom}:${window.from.toISOString()}`;
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const target =
      nowX != null ? nowX - (el.clientWidth - vehicleColumn) / 3 : 0;
    el.scrollLeft = Math.max(0, target);
    // Only on window changes, not on every minute tick.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [windowKey]);

  const headerLabel = (start: Date) => {
    const pattern =
      zoom === "day" ? "HH:mm" : zoom === "week" ? "EEE d MMM" : "d";
    return datetime(start, pattern, locale);
  };

  const describe = (interval: FleetCalendarInterval) => {
    const range = `${datetime(interval.starts_at, undefined, locale)} – ${
      interval.ends_at
        ? datetime(interval.ends_at, undefined, locale)
        : t("vehicle_blocks.open_ended")
    }`;
    if (interval.block) {
      return `${t(BLOCK_KIND_LABEL_KEYS[interval.block.kind])}${
        interval.block.reason ? ` · ${interval.block.reason}` : ""
      } · ${range}`;
    }
    if (interval.transfer) {
      return `${t("fleet_calendar.bar.transfer")} ${interval.transfer.from_branch.code} → ${interval.transfer.to_branch.code} · ${t(TRANSFER_STATUS_LABEL_KEYS[interval.transfer.status])} · ${range}`;
    }
    return range;
  };

  const shortLabel = (interval: FleetCalendarInterval) => {
    if (interval.block) {
      return (
        interval.block.reason || t(BLOCK_KIND_LABEL_KEYS[interval.block.kind])
      );
    }
    if (interval.transfer) {
      return `${interval.transfer.from_branch.code} → ${interval.transfer.to_branch.code}`;
    }
    return "";
  };

  const layoutRow = (row: FleetCalendarRow) => {
    const ranged = row.intervals.map((interval) => ({
      interval,
      start: new Date(interval.clip_start).getTime(),
      end: new Date(interval.clip_end).getTime(),
    }));
    const { placed, lanes } = packLanes(ranged, (r) => ({
      start: r.start,
      end: r.end,
    }));
    const items: Lane[] = [];
    for (const { item, lane } of placed) {
      const tr = item.interval.transfer;
      // Planned range of a started transfer, drawn behind the actual bar.
      if (tr && tr.status !== "planned") {
        items.push({ kind: "ghost", interval: item.interval, lane });
      }
      items.push({ kind: "interval", interval: item.interval, lane });
    }
    const markerLane = row.maintenance.length > 0 ? lanes : -1;
    const totalLanes = lanes + (row.maintenance.length > 0 ? 1 : 0);
    const height = Math.max(
      compact ? LANE_HEIGHT + ROW_PADDING * 2 : MIN_ROW_HEIGHT,
      totalLanes * LANE_HEIGHT + ROW_PADDING * 2,
    );
    return { items, markerLane, height };
  };

  const renderBar = (
    vehicle: FleetCalendarVehicle,
    item: Lane,
    key: string,
  ) => {
    const { interval } = item;
    const top =
      ROW_PADDING + item.lane * LANE_HEIGHT + (LANE_HEIGHT - BAR_HEIGHT) / 2;
    if (item.kind === "ghost" && interval.transfer) {
      const left = xOf(interval.transfer.planned_departure, window, width);
      const right = xOf(interval.transfer.planned_arrival, window, width);
      if (right - left < 1) return null;
      return (
        <div
          key={key}
          aria-hidden
          className="pointer-events-none absolute rounded border-2 border-dashed border-indigo-400/70"
          style={{ left, width: right - left, top, height: BAR_HEIGHT }}
        />
      );
    }
    const left = xOf(interval.clip_start, window, width);
    const barWidth = Math.max(
      MIN_BAR_WIDTH,
      xOf(interval.clip_end, window, width) - left,
    );
    const draggable = Boolean(canDragBar?.(interval)) && Boolean(onBarDrop);
    return (
      <Popover key={key}>
        <PopoverTrigger asChild>
          <button
            type="button"
            aria-label={describe(interval)}
            title={describe(interval)}
            draggable={draggable}
            onDragStart={
              draggable
                ? (event) => {
                    event.dataTransfer.setData(
                      "application/x-fleet-bar",
                      JSON.stringify({
                        uuid: interval.uuid,
                        from: vehicle.uuid,
                      }),
                    );
                  }
                : undefined
            }
            className={cn(
              "focus-visible:ring-ring absolute z-10 flex items-center gap-0.5 overflow-hidden rounded border px-1.5 text-start text-[11px] leading-none font-medium whitespace-nowrap shadow-xs transition-[filter] outline-none hover:brightness-95 focus-visible:z-20 focus-visible:ring-2 focus-visible:ring-offset-1",
              barClasses(interval),
              interval.continues_before && "rounded-s-none",
              interval.continues_after && "rounded-e-none",
              draggable ? "cursor-grab" : "cursor-pointer",
            )}
            style={{ left, width: barWidth, top, height: BAR_HEIGHT }}
          >
            {interval.continues_before ? (
              <ChevronLeft className="size-3 shrink-0" aria-hidden />
            ) : null}
            <span className="truncate">{shortLabel(interval)}</span>
            {interval.continues_after ? (
              <ChevronRight className="ms-auto size-3 shrink-0" aria-hidden />
            ) : null}
          </button>
        </PopoverTrigger>
        <PopoverContent className="w-80" align="start">
          <IntervalDetails
            vehicle={vehicle}
            interval={interval}
            onAction={onAction}
            showVehicleLink={!compact}
          />
        </PopoverContent>
      </Popover>
    );
  };

  const renderMarker = (
    vehicle: FleetCalendarVehicle,
    item: FleetCalendarMaintenance,
    lane: number,
  ) => {
    const { start, end } = dayRange(item.date);
    const x0 = xOf(start, window, width);
    const x1 = xOf(end, window, width);
    if (x1 - x0 < 1) return null;
    const size = MARKER_SIZE;
    // Centred in narrow day columns, near the day start in wide ones.
    const left = x0 + Math.max(0, (Math.min(x1 - x0, 48) - size) / 2);
    const top = ROW_PADDING + lane * LANE_HEIGHT + (LANE_HEIGHT - size) / 2;
    const label = `${t(MAINTENANCE_TYPE_LABEL_KEYS[item.maintenance_type])} · ${t(
      MAINTENANCE_STATE_LABEL_KEYS[item.state],
    )} · ${datetime(start, "dd.MM.yyyy", locale)}`;
    return (
      <Popover key={`m-${item.uuid}`}>
        <PopoverTrigger asChild>
          <button
            type="button"
            aria-label={label}
            title={label}
            className={cn(
              "focus-visible:ring-ring absolute z-10 grid place-items-center rounded-full border-2 shadow-xs outline-none focus-visible:ring-2 focus-visible:ring-offset-1",
              MAINTENANCE_MARKER_CLASSES[item.state],
            )}
            style={{ left, top, width: size, height: size }}
          >
            <Wrench className="size-3" aria-hidden />
          </button>
        </PopoverTrigger>
        <PopoverContent className="w-80" align="start">
          <MaintenanceDetails
            vehicle={vehicle}
            item={item}
            showVehicleLink={!compact}
          />
        </PopoverContent>
      </Popover>
    );
  };

  return (
    <div
      ref={scrollRef}
      className={cn(
        "bg-background relative overflow-auto rounded-md border",
        className,
      )}
      style={{ maxHeight }}
    >
      <div
        role="grid"
        aria-label={t("fleet_calendar.grid_label")}
        aria-rowcount={rows.length + 1}
        className="relative"
        style={{ width: vehicleColumn + width, minWidth: "100%" }}
      >
        {/* Time header */}
        <div
          role="row"
          className="bg-background sticky top-0 z-30 flex border-b"
          style={{ height: HEADER_HEIGHT }}
        >
          {compact ? null : (
            <div
              role="columnheader"
              className="bg-background text-muted-foreground sticky left-0 z-40 flex shrink-0 items-center border-r px-3 text-xs font-medium"
              style={{ width: vehicleColumn }}
            >
              {t("fleet_calendar.vehicle_column")}
            </div>
          )}
          <div className="relative shrink-0" style={{ width }}>
            {columns.map((col) => {
              const left = xOf(col.start, window, width);
              const right = xOf(col.end, window, width);
              return (
                <div
                  key={col.key}
                  role="columnheader"
                  className={cn(
                    "text-muted-foreground absolute inset-y-0 flex flex-col justify-center border-s px-1.5 text-[11px] leading-tight whitespace-nowrap",
                    col.weekend && "bg-muted/50",
                  )}
                  style={{ left, width: right - left }}
                >
                  <span className="truncate font-medium">
                    {headerLabel(col.start)}
                  </span>
                  {zoom === "month" ? (
                    <span className="truncate">
                      {datetime(col.start, "EEEEEE", locale)}
                    </span>
                  ) : null}
                </div>
              );
            })}
            {nowX != null ? (
              <span
                className="bg-primary text-primary-foreground absolute bottom-0 z-10 -translate-x-1/2 rounded px-1 text-[10px] leading-4 font-semibold"
                style={{ left: nowX }}
              >
                {datetime(now, "HH:mm", locale)}
              </span>
            ) : null}
          </div>
        </div>

        {/* Body */}
        <div className="relative">
          {/* Column grid, weekend shading and the "now" line (behind rows). */}
          <div
            aria-hidden
            className="pointer-events-none absolute inset-y-0"
            style={{ left: vehicleColumn, width }}
          >
            {columns.map((col) => {
              const left = xOf(col.start, window, width);
              const right = xOf(col.end, window, width);
              return (
                <div
                  key={col.key}
                  className={cn(
                    "border-border/60 absolute inset-y-0 border-s",
                    col.weekend && "bg-muted/40",
                  )}
                  style={{ left, width: right - left }}
                />
              );
            })}
            {nowX != null ? (
              <div
                className="bg-primary absolute inset-y-0 z-20 w-0.5"
                style={{ left: nowX - 1 }}
              />
            ) : null}
          </div>

          {rows.map((row, index) => {
            const { items, markerLane, height } = layoutRow(row);
            const vehicle = row.vehicle;
            const dropTarget = dragOver === vehicle.uuid;
            return (
              <div
                key={vehicle.uuid}
                role="row"
                aria-rowindex={index + 2}
                className="hover:bg-muted/20 relative flex border-b last:border-b-0"
                style={{ height }}
              >
                {compact ? null : (
                  <div
                    role="rowheader"
                    className="bg-background sticky left-0 z-20 flex shrink-0 flex-col justify-center gap-1 border-r px-3 py-1.5"
                    style={{ width: vehicleColumn }}
                  >
                    <div className="flex min-w-0 items-center gap-2">
                      <Link
                        href={routes.tenant.fleet.vehicles.detail(
                          slug,
                          vehicle.uuid,
                        )}
                        className="focus-visible:ring-ring rounded outline-none focus-visible:ring-2"
                      >
                        <PlateBadge plate={vehicle.plate} />
                      </Link>
                      <span className="text-muted-foreground truncate text-xs">
                        {vehicle.current_branch.code}
                      </span>
                    </div>
                    <div className="flex min-w-0 items-center gap-2">
                      <span className="truncate text-xs" title={vehicle.label}>
                        {vehicle.label || "—"}
                      </span>
                      <span className="ms-auto shrink-0 scale-90">
                        <VehicleStatusChip status={vehicle.status} />
                      </span>
                    </div>
                  </div>
                )}
                <div
                  role="gridcell"
                  className={cn(
                    "relative shrink-0",
                    dropTarget && "bg-primary/10",
                  )}
                  style={{ width }}
                  onDragOver={
                    onBarDrop
                      ? (event) => {
                          if (
                            event.dataTransfer.types.includes(
                              "application/x-fleet-bar",
                            )
                          ) {
                            event.preventDefault();
                            setDragOver(vehicle.uuid);
                          }
                        }
                      : undefined
                  }
                  onDragLeave={onBarDrop ? () => setDragOver(null) : undefined}
                  onDrop={
                    onBarDrop
                      ? (event) => {
                          setDragOver(null);
                          const raw = event.dataTransfer.getData(
                            "application/x-fleet-bar",
                          );
                          if (!raw) return;
                          const data = JSON.parse(raw) as {
                            uuid: string;
                            from: string;
                          };
                          const bar = rows
                            .flatMap((r) => r.intervals)
                            .find((i) => i.uuid === data.uuid);
                          if (bar && data.from !== vehicle.uuid) {
                            onBarDrop(bar, data.from, vehicle.uuid);
                          }
                        }
                      : undefined
                  }
                >
                  {items.map((item) =>
                    renderBar(
                      vehicle,
                      item,
                      `${item.kind}-${item.interval.kind}-${item.interval.uuid}`,
                    ),
                  )}
                  {row.maintenance.map((m) =>
                    renderMarker(vehicle, m, markerLane),
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
