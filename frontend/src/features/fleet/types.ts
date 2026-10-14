import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Segment = Schemas["Segment"];
export type SegmentPage = Schemas["SegmentPage"];
export type SegmentFacets = Schemas["SegmentFacets"];
export type SegmentCreateRequest = Schemas["SegmentCreateRequest"];
export type SegmentPatchRequest = Schemas["SegmentPatchRequest"];

export type RentalGroup = Schemas["RentalGroup"];
export type RentalGroupPage = Schemas["RentalGroupPage"];
export type RentalGroupFacets = Schemas["RentalGroupFacets"];
export type RentalGroupCreateRequest = Schemas["RentalGroupCreateRequest"];
export type RentalGroupPatchRequest = Schemas["RentalGroupPatchRequest"];

export type RentalProduct = Schemas["RentalProduct"];
export type RentalProductPage = Schemas["RentalProductPage"];
export type RentalProductFacets = Schemas["RentalProductFacets"];
export type RentalProductCreateRequest = Schemas["RentalProductCreateRequest"];
export type RentalProductPatchRequest = Schemas["RentalProductPatchRequest"];
export type ProductVersion = Schemas["ProductVersion"];
export type ProductVersionPage = Schemas["ProductVersionPage"];
export type ProductImage = Schemas["ProductImage"];
export type ProductGuarantee = Schemas["ProductGuarantee"];
export type ProductConditions = Schemas["ProductConditions"];

export type FleetFacet = Schemas["FleetFacet"];
export type FleetActivityEntry = Schemas["FleetActivityEntry"];
export type FleetActivityPage = Schemas["FleetActivityPage"];
export type CatalogModelRef = Schemas["FleetCatalogModelRef"];
export type CatalogTypeRef = Schemas["FleetCatalogTypeRef"];

export type Transmission = Schemas["FleetTransmission"];
export type Fuel = Schemas["FleetFuel"];
export type BodyType = Schemas["FleetBodyType"];
export type GuaranteeType = Schemas["GuaranteeType"];
export type FuelPolicy = Schemas["FuelPolicy"];
export type ProductStatus = RentalProduct["status"];
export type GuaranteeBadge = RentalProduct["guarantee_badge"];

export type TenantCatalogBrand = Schemas["TenantCatalogBrand"];
export type TenantCatalogModel = Schemas["TenantCatalogModel"];
export type TenantCatalogType = Schemas["TenantCatalogType"];

type ListParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  is_active?: "true" | "false";
};

/** List query of `GET /v1/tenant/segments`. */
export type ListSegmentsParams = ListParams;

/** List query of `GET /v1/tenant/rental-groups` (multi-values comma-joined). */
export type ListGroupsParams = ListParams & {
  segment?: string;
  transmission?: string;
  fuel?: string;
  body_type?: string;
};

/** List query of `GET /v1/tenant/products` (multi-values comma-joined). */
export type ListProductsParams = ListParams & {
  segment?: string;
  status?: string;
  guarantee_type?: string;
  rental_group?: string;
};

/** Fleet resources sharing the activity endpoint shape. */
export type FleetResourceKind = "segments" | "groups" | "products";
