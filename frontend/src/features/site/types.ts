import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type SiteSettings = Schemas["SiteSettings"];
export type SiteConfig = Schemas["SiteConfig"];
export type SiteSaveInput = Schemas["SiteSaveInput"];
export type SiteRevision = Schemas["SiteRevision"];
export type SitePreviewToken = Schemas["SitePreviewToken"];
export type SiteCatalog = Schemas["SiteCatalog"];
export type SiteCatalogBranch = Schemas["SiteCatalogBranch"];
export type SiteCatalogSubgroup = Schemas["SiteCatalogSubgroup"];
export type SiteCatalogExtra = Schemas["SiteCatalogExtra"];
export type SiteDomain = Schemas["SiteDomain"];
export type SiteMedia = Schemas["SiteMedia"];
export type SiteMediaList = Schemas["EnvelopeSiteMediaList"]["data"];
export type SiteResolved = Schemas["SiteResolved"];

export type SiteImageKind = "logo" | "favicon" | "cover";
export type SiteCatalogKind = "branches" | "products" | "extras";
export type SiteLocale = "tr" | "en";
export type BookingBlocker = SiteSettings["booking_blockers"][number];
