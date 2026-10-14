import type {
  FleetCalendar,
  FleetCalendarParams,
} from "@/features/fleet-calendar/types";
import { tenantRequest } from "@/lib/api/tenant-request";

/** Fleet calendar of one organization (`X-Organization` via tenantRequest). */
export const fleetCalendarService = {
  get(slug: string, params: FleetCalendarParams) {
    return tenantRequest<FleetCalendar>(
      slug,
      "GET",
      "/v1/tenant/fleet/calendar",
      {
        query: params,
      },
    );
  },
};
