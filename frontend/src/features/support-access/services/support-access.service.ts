import type {
  ListSupportGrantsParams,
  SupportGrant,
  SupportGrantOpenRequest,
  SupportGrantPage,
} from "@/features/support-access/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/support-access";

/**
 * Support access grants of the organization (TEC-678): `tenantRequest`
 * only (same-origin BFF, `X-Organization` from the slug).
 */
export const supportAccessService = {
  list(slug: string, params: ListSupportGrantsParams) {
    return tenantRequest<SupportGrantPage>(slug, "GET", BASE, {
      query: params,
    });
  },
  open(slug: string, body: SupportGrantOpenRequest) {
    return tenantRequest<SupportGrant>(slug, "POST", BASE, { body });
  },
  revoke(slug: string, uuid: string) {
    return tenantRequest<SupportGrant>(
      slug,
      "POST",
      `${BASE}/${encodeURIComponent(uuid)}/revoke`,
    );
  },
};
