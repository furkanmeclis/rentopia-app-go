import { CarFront } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of vehicles (plate and VIN). Inside `/t/{slug}` the palette
 * searches `GET /v1/tenant/search`, which filters hits to the organization
 * and the member's branches.
 */
export const vehiclesSearchSpec = defineSearchSpec({
  id: "vehicles",
  labelKey: "search.specs_vehicles",
  icon: CarFront,
  permission: permissions.tenant.vehicles.read,
});
