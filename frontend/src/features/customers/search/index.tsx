import { Users } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of customers. Documents hold the name, the masked identity and
 * the last digits of the identity hint / phone only; the palette searches
 * `GET /v1/tenant/search` (organization-wide, not branch scoped).
 */
export const customersSearchSpec = defineSearchSpec({
  id: "customers",
  labelKey: "search.specs_customers",
  icon: Users,
  permission: permissions.tenant.customers.read,
});
