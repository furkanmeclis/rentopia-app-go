import {
  addDays,
  addHours,
  addMonths,
  addWeeks,
  format,
  isWeekend,
  parseISO,
  startOfDay,
  startOfMonth,
  startOfWeek,
} from "date-fns";

import { COLUMN_WIDTH } from "@/features/fleet-calendar/constants";
import type { CalendarZoom } from "@/features/fleet-calendar/types";

/** Half-open window [from, to) in local time. */
export type CalendarWindow = { from: Date; to: Date };

/** The window a zoom level shows around `anchor` (local time). */
export function calendarWindow(
  zoom: CalendarZoom,
  anchor: Date,
): CalendarWindow {
  switch (zoom) {
    case "day": {
      const from = startOfDay(anchor);
      return { from, to: addDays(from, 1) };
    }
    case "week": {
      const from = startOfWeek(anchor, { weekStartsOn: 1 });
      return { from, to: addDays(from, 7) };
    }
    case "month": {
      const from = startOfMonth(anchor);
      return { from, to: addMonths(from, 1) };
    }
  }
}

/** Moves the anchor one window back (-1) or forward (+1). */
export function shiftAnchor(
  zoom: CalendarZoom,
  anchor: Date,
  direction: -1 | 1,
): Date {
  switch (zoom) {
    case "day":
      return addDays(anchor, direction);
    case "week":
      return addWeeks(anchor, direction);
    case "month":
      return addMonths(anchor, direction);
  }
}

/** One grid column: an hour (day zoom) or a day. */
export type TimelineColumn = {
  key: string;
  start: Date;
  end: Date;
  weekend: boolean;
};

export function timelineColumns(
  zoom: CalendarZoom,
  window: CalendarWindow,
): TimelineColumn[] {
  const out: TimelineColumn[] = [];
  const step = zoom === "day" ? addHours : addDays;
  let start = window.from;
  while (start < window.to) {
    const end = step(start, 1);
    out.push({
      key: start.toISOString(),
      start,
      end: end > window.to ? window.to : end,
      weekend: zoom !== "day" && isWeekend(start),
    });
    start = end;
  }
  return out;
}

/** Pixel width of the time track. */
export function trackWidth(zoom: CalendarZoom, columns: number): number {
  return columns * COLUMN_WIDTH[zoom];
}

/**
 * Maps an instant to an x offset inside the track (linear in time; the
 * window is at most a month so DST shifts stay sub-column).
 */
export function xOf(
  at: Date | string | number,
  window: CalendarWindow,
  width: number,
): number {
  const t =
    typeof at === "string"
      ? parseISO(at).getTime()
      : typeof at === "number"
        ? at
        : at.getTime();
  const from = window.from.getTime();
  const span = window.to.getTime() - from;
  const ratio = (t - from) / span;
  return Math.min(width, Math.max(0, ratio * width));
}

/** A `yyyy-MM-dd` day as the local [start, end) range. */
export function dayRange(day: string): { start: Date; end: Date } {
  const start = startOfDay(parseISO(day));
  return { start, end: addDays(start, 1) };
}

/** Local `yyyy-MM-dd` (DatePicker value) of a date. */
export function toDayValue(value: Date): string {
  return format(value, "yyyy-MM-dd");
}

export type Placed<T> = { item: T; lane: number };

/**
 * Greedy lane packing: items that overlap in time go to separate lanes so
 * no bar hides another (a new block may overlap an open transfer).
 */
export function packLanes<T>(
  items: T[],
  range: (item: T) => { start: number; end: number },
): { placed: Placed<T>[]; lanes: number } {
  const sorted = [...items].sort((a, b) => range(a).start - range(b).start);
  const laneEnds: number[] = [];
  const placed: Placed<T>[] = [];
  for (const item of sorted) {
    const { start, end } = range(item);
    let lane = laneEnds.findIndex((laneEnd) => laneEnd <= start);
    if (lane === -1) {
      lane = laneEnds.length;
      laneEnds.push(end);
    } else {
      laneEnds[lane] = end;
    }
    placed.push({ item, lane });
  }
  return { placed, lanes: Math.max(1, laneEnds.length) };
}
