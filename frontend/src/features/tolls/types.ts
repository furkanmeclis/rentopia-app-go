import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type TollPass = Schemas["TollPass"];
export type TollPassDetail = Schemas["TollPassDetail"];
export type TollPassPage = Schemas["EnvelopeTollPassPage"]["data"];
export type TollPassFacets = Schemas["TollPassFacets"];
export type TollFacet = Schemas["TollFacet"];
export type TollPassEvent = Schemas["TollPassEvent"];
export type TollPassRevision = Schemas["TollPassRevision"];
export type TollPassCharge = Schemas["TollPassCharge"];
export type TollCandidate = Schemas["TollCandidate"];
export type TollSettings = Schemas["TollSettings"];
export type TollSettingsPatch = Schemas["TollSettingsPatch"];
export type TollTag = Schemas["TollTag"];
export type TollTagCreate = Schemas["TollTagCreate"];
export type TollTagPatch = Schemas["TollTagPatch"];
export type TollFetchResult = Schemas["TollFetchResult"];

export type TollStatus = TollPass["status"];
export type TollReviewReason = TollPass["review_reason"];
export type TollSource = TollPass["source"];
export type TollEventAction = TollPassEvent["action"];

/** List query of `GET /v1/tenant/toll-passes` (multi-values comma-joined). */
export type ListTollPassesParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  status?: string;
  review?: string;
  review_reason?: string;
  source?: string;
  provider?: string;
  vehicle?: string;
  rental?: string;
  has_rental?: string;
  from?: string;
  to?: string;
};
