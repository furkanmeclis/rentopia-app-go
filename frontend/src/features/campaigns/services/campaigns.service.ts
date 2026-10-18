import type {
  Campaign,
  CampaignActivityPage,
  CampaignCreateRequest,
  CampaignFacets,
  CampaignPage,
  CampaignPatchRequest,
  CampaignRedemptionPage,
  CampaignStorefront,
  CampaignStorefrontPatch,
  ListCampaignsParams,
} from "@/features/campaigns/types";
import type { ResourceMeta } from "@/features/io/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const CAMPAIGNS = "/v1/tenant/campaigns";

type PageParams = { limit: number; offset: number; status?: string };

/** Campaigns and coupons of one organization (`tenantRequest` only). */
export const campaignsService = {
  list(slug: string, params: ListCampaignsParams) {
    return tenantRequest<CampaignPage>(slug, "GET", CAMPAIGNS, {
      query: params,
    });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${CAMPAIGNS}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<CampaignFacets>(slug, "GET", `${CAMPAIGNS}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Campaign>(slug, "GET", `${CAMPAIGNS}/${uuid}`);
  },
  create(slug: string, body: CampaignCreateRequest) {
    return tenantRequest<Campaign>(slug, "POST", CAMPAIGNS, { body });
  },
  update(slug: string, uuid: string, body: CampaignPatchRequest) {
    return tenantRequest<Campaign>(slug, "PATCH", `${CAMPAIGNS}/${uuid}`, {
      body,
    });
  },
  remove(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${CAMPAIGNS}/${uuid}`,
    );
  },
  activity(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<CampaignActivityPage>(
      slug,
      "GET",
      `${CAMPAIGNS}/${uuid}/activity`,
      { query: params },
    );
  },
  /** Website card fields (`GET/PATCH …/{uuid}/storefront`, TEC-784). */
  storefront(slug: string, uuid: string) {
    return tenantRequest<CampaignStorefront>(
      slug,
      "GET",
      `${CAMPAIGNS}/${uuid}/storefront`,
    );
  },
  updateStorefront(slug: string, uuid: string, body: CampaignStorefrontPatch) {
    return tenantRequest<CampaignStorefront>(
      slug,
      "PATCH",
      `${CAMPAIGNS}/${uuid}/storefront`,
      { body },
    );
  },
  redemptions(slug: string, uuid: string, params: PageParams) {
    return tenantRequest<CampaignRedemptionPage>(
      slug,
      "GET",
      `${CAMPAIGNS}/${uuid}/redemptions`,
      { query: params },
    );
  },
};
