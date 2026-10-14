import type { ResourceMeta } from "@/features/io/types";
import type {
  FleetActivityPage,
  FleetResourceKind,
  ListGroupsParams,
  ListProductsParams,
  ListSegmentsParams,
  ProductImage,
  ProductVersion,
  ProductVersionPage,
  RentalGroup,
  RentalGroupCreateRequest,
  RentalGroupFacets,
  RentalGroupPage,
  RentalGroupPatchRequest,
  RentalProduct,
  RentalProductCreateRequest,
  RentalProductFacets,
  RentalProductPage,
  RentalProductPatchRequest,
  Segment,
  SegmentCreateRequest,
  SegmentFacets,
  SegmentPage,
  SegmentPatchRequest,
  TenantCatalogBrand,
  TenantCatalogModel,
  TenantCatalogType,
} from "@/features/fleet/types";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";

const SEGMENTS = "/v1/tenant/segments";
const GROUPS = "/v1/tenant/rental-groups";
const PRODUCTS = "/v1/tenant/products";

export const FLEET_BASE_PATHS: Record<FleetResourceKind, string> = {
  segments: SEGMENTS,
  groups: GROUPS,
  products: PRODUCTS,
};

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/**
 * Fleet classification of one organization (segments, rental groups,
 * rentable products). Every call names the organization through
 * `tenantRequest` (`X-Organization: {slug}`).
 */
export const fleetService = {
  // --- segments
  listSegments(slug: string, params: ListSegmentsParams) {
    return tenantRequest<SegmentPage>(slug, "GET", SEGMENTS, { query: params });
  },
  segmentsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${SEGMENTS}/meta`);
  },
  segmentFacets(slug: string) {
    return tenantRequest<SegmentFacets>(slug, "GET", `${SEGMENTS}/facets`);
  },
  getSegment(slug: string, uuid: string) {
    return tenantRequest<Segment>(slug, "GET", `${SEGMENTS}/${uuid}`);
  },
  createSegment(slug: string, body: SegmentCreateRequest) {
    return tenantRequest<Segment>(slug, "POST", SEGMENTS, { body });
  },
  updateSegment(slug: string, uuid: string, body: SegmentPatchRequest) {
    return tenantRequest<Segment>(slug, "PATCH", `${SEGMENTS}/${uuid}`, {
      body,
    });
  },
  removeSegment(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${SEGMENTS}/${uuid}`,
    );
  },

  // --- rental groups
  listGroups(slug: string, params: ListGroupsParams) {
    return tenantRequest<RentalGroupPage>(slug, "GET", GROUPS, {
      query: params,
    });
  },
  groupsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${GROUPS}/meta`);
  },
  groupFacets(slug: string) {
    return tenantRequest<RentalGroupFacets>(slug, "GET", `${GROUPS}/facets`);
  },
  getGroup(slug: string, uuid: string) {
    return tenantRequest<RentalGroup>(slug, "GET", `${GROUPS}/${uuid}`);
  },
  createGroup(slug: string, body: RentalGroupCreateRequest) {
    return tenantRequest<RentalGroup>(slug, "POST", GROUPS, { body });
  },
  updateGroup(slug: string, uuid: string, body: RentalGroupPatchRequest) {
    return tenantRequest<RentalGroup>(slug, "PATCH", `${GROUPS}/${uuid}`, {
      body,
    });
  },
  removeGroup(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${GROUPS}/${uuid}`,
    );
  },

  // --- rental products
  listProducts(slug: string, params: ListProductsParams) {
    return tenantRequest<RentalProductPage>(slug, "GET", PRODUCTS, {
      query: params,
    });
  },
  productsMeta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${PRODUCTS}/meta`);
  },
  productFacets(slug: string) {
    return tenantRequest<RentalProductFacets>(
      slug,
      "GET",
      `${PRODUCTS}/facets`,
    );
  },
  getProduct(slug: string, uuid: string) {
    return tenantRequest<RentalProduct>(slug, "GET", `${PRODUCTS}/${uuid}`);
  },
  createProduct(slug: string, body: RentalProductCreateRequest) {
    return tenantRequest<RentalProduct>(slug, "POST", PRODUCTS, { body });
  },
  updateProduct(slug: string, uuid: string, body: RentalProductPatchRequest) {
    return tenantRequest<RentalProduct>(slug, "PATCH", `${PRODUCTS}/${uuid}`, {
      body,
    });
  },
  removeProduct(slug: string, uuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${PRODUCTS}/${uuid}`,
    );
  },
  listVersions(
    slug: string,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<ProductVersionPage>(
      slug,
      "GET",
      `${PRODUCTS}/${uuid}/versions`,
      { query: params },
    );
  },
  getVersion(slug: string, uuid: string, versionUuid: string) {
    return tenantRequest<ProductVersion>(
      slug,
      "GET",
      `${PRODUCTS}/${uuid}/versions/${versionUuid}`,
    );
  },
  uploadImage(slug: string, uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<ProductImage>(
      slug,
      "POST",
      `${PRODUCTS}/${uuid}/images`,
      form,
    );
  },
  reorderImages(slug: string, uuid: string, imageUuids: string[]) {
    return tenantRequest<{ items: ProductImage[] }>(
      slug,
      "PUT",
      `${PRODUCTS}/${uuid}/images/order`,
      { body: { image_uuids: imageUuids } },
    );
  },
  removeImage(slug: string, uuid: string, imageUuid: string) {
    return tenantRequest<{ status: string }>(
      slug,
      "DELETE",
      `${PRODUCTS}/${uuid}/images/${imageUuid}`,
    );
  },

  // --- shared
  activity(
    slug: string,
    kind: FleetResourceKind,
    uuid: string,
    params: { limit: number; offset: number },
  ) {
    return tenantRequest<FleetActivityPage>(
      slug,
      "GET",
      `${FLEET_BASE_PATHS[kind]}/${uuid}/activity`,
      { query: params },
    );
  },

  // --- platform catalog lookups (read-only, tenant routes)
  catalogBrands(slug: string, q: string) {
    return tenantRequest<Page<TenantCatalogBrand>>(
      slug,
      "GET",
      "/v1/tenant/catalog/brands",
      { query: { q, limit: 50 }, silent: true },
    );
  },
  catalogModels(slug: string, brandUuid: string, q: string) {
    return tenantRequest<Page<TenantCatalogModel>>(
      slug,
      "GET",
      "/v1/tenant/catalog/models",
      { query: { brand: brandUuid, q, limit: 50 }, silent: true },
    );
  },
  catalogTypes(slug: string, modelUuid: string, q: string) {
    return tenantRequest<Page<TenantCatalogType>>(
      slug,
      "GET",
      "/v1/tenant/catalog/types",
      { query: { model: modelUuid, q, limit: 50 }, silent: true },
    );
  },
};
