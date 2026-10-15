import type { BlockKind, TransferStatus } from "@/features/fleet-ops/types";
import type {
  CalendarZoom,
  MaintenanceMarkerState,
} from "@/features/fleet-calendar/types";

/** Live channels the calendar refetches on (debounced). */
export const CALENDAR_LIVE_RESOURCES = [
  "tenant.vehicles",
  "tenant.transfers",
  "tenant.vehicle_blocks",
  "tenant.availability_holds",
  "tenant.reservations",
] as const;

/** Vehicles per calendar page (the API default). */
export const CALENDAR_PAGE_SIZE = 50;

export const CALENDAR_ZOOMS: CalendarZoom[] = ["day", "week", "month"];

export const ZOOM_LABEL_KEYS: Record<CalendarZoom, string> = {
  day: "fleet_calendar.zoom.day",
  week: "fleet_calendar.zoom.week",
  month: "fleet_calendar.zoom.month",
};

/**
 * Width of one column: an hour (day zoom) or a day (week / month zoom).
 * The track scrolls horizontally when it is wider than the card.
 */
export const COLUMN_WIDTH: Record<CalendarZoom, number> = {
  day: 56,
  week: 148,
  month: 44,
};

/** Bar colours per block kind (light and dark mode). */
export const BLOCK_BAR_CLASSES: Record<BlockKind, string> = {
  maintenance:
    "bg-amber-500 text-amber-950 border-amber-600 dark:bg-amber-500/80 dark:text-amber-50",
  damage:
    "bg-red-500 text-white border-red-600 dark:bg-red-600/80 dark:text-red-50",
  cleaning:
    "bg-sky-400 text-sky-950 border-sky-500 dark:bg-sky-600/80 dark:text-sky-50",
  out_of_service:
    "bg-slate-500 text-white border-slate-600 dark:bg-slate-500/80 dark:text-slate-50",
  other:
    "bg-violet-400 text-violet-950 border-violet-500 dark:bg-violet-600/80 dark:text-violet-50",
};

/**
 * Transfer bars: planned ones are dashed outlines (not happened yet),
 * moving / done ones are solid (actual times).
 */
export const TRANSFER_BAR_CLASSES: Record<TransferStatus, string> = {
  planned:
    "border-dashed border-2 border-indigo-500 bg-indigo-500/15 text-indigo-900 dark:text-indigo-100",
  in_transit:
    "bg-indigo-600 text-white border-indigo-700 dark:bg-indigo-500/90 dark:text-indigo-50",
  completed:
    "bg-indigo-300 text-indigo-950 border-indigo-400 dark:bg-indigo-800/80 dark:text-indigo-100",
  cancelled:
    "bg-muted text-muted-foreground border-border line-through opacity-70",
};

/**
 * Allocation bars (availability engine): holds ("opsiyon") are dashed (they
 * lapse on their own), confirmed reservations solid. The bar spans the
 * customer range plus the preparation buffer of the return branch.
 */
export const ALLOCATION_BAR_CLASSES = {
  hold: "border-dashed border-2 border-teal-600 bg-teal-500/20 text-teal-950 dark:border-teal-400 dark:text-teal-50",
  reservation:
    "bg-emerald-600 text-white border-emerald-700 dark:bg-emerald-500/90 dark:text-emerald-50",
} as const;

/** Maintenance day markers per state. */
export const MAINTENANCE_MARKER_CLASSES: Record<
  MaintenanceMarkerState,
  string
> = {
  scheduled:
    "bg-background text-emerald-700 border-emerald-500 dark:text-emerald-300",
  due: "bg-amber-100 text-amber-800 border-amber-500 dark:bg-amber-500/20 dark:text-amber-200",
  overdue:
    "bg-red-100 text-red-700 border-red-500 dark:bg-red-500/20 dark:text-red-200",
  done: "bg-emerald-500 text-white border-emerald-600 dark:bg-emerald-600/80",
};

export const MAINTENANCE_STATE_LABEL_KEYS: Record<
  MaintenanceMarkerState,
  string
> = {
  scheduled: "vehicle_records.maintenance_status.scheduled",
  due: "vehicle_records.maintenance_status.due",
  overdue: "vehicle_records.maintenance_status.overdue",
  done: "vehicle_records.maintenance_status.done",
};

/** Legend entries; `milestone` ones are placeholders shown disabled. */
export type LegendEntry = {
  id: string;
  labelKey: string;
  swatch: string;
  milestone?: "M2" | "M3";
};

export const LEGEND_ENTRIES: LegendEntry[] = [
  {
    id: "block-maintenance",
    labelKey: "vehicle_blocks.kind.maintenance",
    swatch: BLOCK_BAR_CLASSES.maintenance,
  },
  {
    id: "block-damage",
    labelKey: "vehicle_blocks.kind.damage",
    swatch: BLOCK_BAR_CLASSES.damage,
  },
  {
    id: "block-cleaning",
    labelKey: "vehicle_blocks.kind.cleaning",
    swatch: BLOCK_BAR_CLASSES.cleaning,
  },
  {
    id: "block-out_of_service",
    labelKey: "vehicle_blocks.kind.out_of_service",
    swatch: BLOCK_BAR_CLASSES.out_of_service,
  },
  {
    id: "block-other",
    labelKey: "vehicle_blocks.kind.other",
    swatch: BLOCK_BAR_CLASSES.other,
  },
  {
    id: "transfer-planned",
    labelKey: "fleet_calendar.legend.transfer_planned",
    swatch: TRANSFER_BAR_CLASSES.planned,
  },
  {
    id: "transfer-actual",
    labelKey: "fleet_calendar.legend.transfer_actual",
    swatch: TRANSFER_BAR_CLASSES.in_transit,
  },
  {
    id: "maintenance",
    labelKey: "fleet_calendar.legend.maintenance",
    swatch: MAINTENANCE_MARKER_CLASSES.scheduled,
  },
  {
    id: "hold",
    labelKey: "fleet_calendar.legend.hold",
    swatch: ALLOCATION_BAR_CLASSES.hold,
  },
  {
    id: "reservation",
    labelKey: "fleet_calendar.legend.reservation",
    swatch: ALLOCATION_BAR_CLASSES.reservation,
  },
  {
    id: "rental",
    labelKey: "fleet_calendar.legend.rental",
    swatch: "bg-muted border-border",
    milestone: "M3",
  },
  {
    id: "preparation",
    labelKey: "fleet_calendar.legend.preparation",
    swatch: "bg-muted border-border",
    milestone: "M3",
  },
];

export const MILESTONE_HINT_KEYS: Record<"M2" | "M3", string> = {
  M2: "fleet_calendar.legend.coming_m2",
  M3: "fleet_calendar.legend.coming_m3",
};
