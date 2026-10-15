import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type Inspection = Schemas["Inspection"];
export type InspectionKind = Inspection["kind"];
export type InspectionStatus = Inspection["status"];
export type InspectionReviewStatus = Inspection["review_status"];
export type InspectionSignatureStatus = Inspection["signature_status"];
export type InspectionListItem = Schemas["InspectionListItem"];
export type InspectionPage = Schemas["EnvelopeInspectionPage"]["data"];
export type InspectionFacets = Schemas["InspectionFacets"];
export type InspectionFacetValue = Schemas["InspectionFacetValue"];
export type InspectionDamage = Schemas["InspectionDamage"];
export type InspectionPhoto = Schemas["InspectionPhoto"];
export type InspectionPhotoUpload = Schemas["InspectionPhotoUpload"];
export type InspectionReadiness = Schemas["InspectionReadiness"];
export type InspectionBaseline = Schemas["InspectionBaseline"];
export type InspectionChargePreview = Schemas["InspectionChargePreview"];
export type InspectionChargeLine = Schemas["InspectionChargeLine"];
export type InspectionChargeOverride = Schemas["InspectionChargeOverride"];
export type InspectionMediaLink = Schemas["InspectionMediaLink"];
export type InspectionTemplate = Schemas["InspectionTemplate"];
export type InspectionTemplatePage =
  Schemas["EnvelopeInspectionTemplatePage"]["data"];
export type InspectionTemplateRequest = Schemas["InspectionTemplateRequest"];
export type InspectionTemplateSlot = Schemas["InspectionTemplateSlot"];
export type InspectionTemplateItem = Schemas["InspectionTemplateItem"];
export type InspectionDraftRequest = Schemas["InspectionDraftRequest"];
export type InspectionDamageRequest = Schemas["InspectionDamageRequest"];
export type InspectionPhotoRequest = Schemas["InspectionPhotoRequest"];
export type InspectionSignatureRequest = Schemas["InspectionSignatureRequest"];
export type InspectionCompleteRequest = Schemas["InspectionCompleteRequest"];
export type InspectionReviewRequest = Schemas["InspectionReviewRequest"];
export type DamageType = InspectionDamage["damage_type"];
export type DamageSeverity = InspectionDamage["severity"];
export type VehicleStatusAfter = NonNullable<
  InspectionDraftRequest["vehicle_status_after"]
>;

/** List query of `GET /v1/tenant/inspections`. */
export type ListInspectionsParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
  kind?: string;
  status?: string;
  review_status?: string;
  rental?: string;
  disputed?: string;
};
