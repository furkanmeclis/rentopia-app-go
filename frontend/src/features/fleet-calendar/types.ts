import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type FleetCalendar = Schemas["FleetCalendar"];
export type FleetCalendarRow = Schemas["FleetCalendarRow"];
export type FleetCalendarVehicle = Schemas["FleetCalendarVehicle"];
export type FleetCalendarInterval = Schemas["FleetCalendarInterval"];
export type FleetCalendarMaintenance = Schemas["FleetCalendarMaintenance"];
export type FleetCalendarLayer = FleetCalendar["layers"][number];
export type MaintenanceMarkerState = FleetCalendarMaintenance["state"];

/** Zoom levels: one day in hours, one week in days, one month in days. */
export type CalendarZoom = "day" | "week" | "month";

/** Query of `GET /v1/tenant/fleet/calendar` (multi values comma-joined). */
export type FleetCalendarParams = {
  from: string;
  to: string;
  limit: number;
  offset: number;
  q?: string;
  status?: string;
  home_branch?: string;
  current_branch?: string;
  rental_group?: string;
  segment?: string;
  vehicle?: string;
};

/** Filter values of the calendar page (multi-select facets). */
export type FleetCalendarFilters = {
  q: string;
  status: string[];
  segment: string[];
  rental_group: string[];
  home_branch: string[];
  current_branch: string[];
};
