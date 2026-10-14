import { Tags } from "lucide-react";

import { permissions } from "@/config/permissions";
import { defineSearchSpec } from "@/features/search-engine/define";

/**
 * Cmd+K spec of rental products. Inside `/t/{slug}` the palette lists it from
 * `GET /v1/tenant/search/specs` and searches `GET /v1/tenant/search`, which
 * filters hits to the current organization.
 */
export const rentalProductsSearchSpec = defineSearchSpec({
  id: "rental_products",
  labelKey: "search.specs_rental_products",
  icon: Tags,
  permission: permissions.tenant.fleet.read,
});
