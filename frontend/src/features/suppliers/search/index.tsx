import { Truck } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of suppliers (title, code, tax number); the palette searches
 * `GET /v1/tenant/search` (organization-wide, not branch scoped).
 */
export const suppliersSearchSpec = defineSearchSpec({
  id: "suppliers",
  labelKey: "search.specs_suppliers",
  icon: Truck,
  permission: permissions.tenant.suppliers.read,
});
