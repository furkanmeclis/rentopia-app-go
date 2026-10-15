import type {
  Inspection,
  InspectionChargePreview,
  InspectionCompleteRequest,
  InspectionDamageRequest,
  InspectionDraftRequest,
  InspectionFacets,
  InspectionKind,
  InspectionMediaLink,
  InspectionPage,
  InspectionPhoto,
  InspectionPhotoRequest,
  InspectionPhotoUpload,
  InspectionReviewRequest,
  InspectionSignatureRequest,
  InspectionTemplate,
  InspectionTemplatePage,
  InspectionTemplateRequest,
  ListInspectionsParams,
} from "@/features/inspections/types";
import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api/client";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/inspections";
const TEMPLATES = "/v1/tenant/inspection-templates";
const one = (uuid: string) => `${BASE}/${uuid}`;

/**
 * Handover / return forms, their evidence and templates
 * (`tenantRequest` only). Actions whose errors the wizard renders itself
 * are `silent`.
 */
export const inspectionsService = {
  list(slug: string, params: ListInspectionsParams) {
    return tenantRequest<InspectionPage>(slug, "GET", BASE, { query: params });
  },
  facets(slug: string) {
    return tenantRequest<InspectionFacets>(slug, "GET", `${BASE}/facets`);
  },
  start(slug: string, rentalUuid: string, kind: InspectionKind) {
    return tenantRequest<Inspection>(slug, "POST", BASE, {
      body: { rental_uuid: rentalUuid, kind },
      silent: true,
    });
  },
  get(slug: string, uuid: string) {
    return tenantRequest<Inspection>(slug, "GET", one(uuid));
  },
  update(slug: string, uuid: string, body: InspectionDraftRequest) {
    return tenantRequest<Inspection>(slug, "PATCH", one(uuid), {
      body,
      silent: true,
    });
  },
  charges(slug: string, uuid: string) {
    return tenantRequest<InspectionChargePreview>(
      slug,
      "GET",
      `${one(uuid)}/charges`,
    );
  },
  mediaLinks(slug: string, uuid: string) {
    return tenantRequest<{ items: InspectionMediaLink[] }>(
      slug,
      "POST",
      `${one(uuid)}/media-links`,
      { body: {}, silent: true },
    );
  },
  addDamage(slug: string, uuid: string, body: InspectionDamageRequest) {
    return tenantRequest<Inspection>(slug, "POST", `${one(uuid)}/damages`, {
      body,
      silent: true,
    });
  },
  updateDamage(
    slug: string,
    uuid: string,
    damage: string,
    body: InspectionDamageRequest,
  ) {
    return tenantRequest<Inspection>(
      slug,
      "PATCH",
      `${one(uuid)}/damages/${damage}`,
      { body, silent: true },
    );
  },
  deleteDamage(slug: string, uuid: string, damage: string) {
    return tenantRequest<Inspection>(
      slug,
      "DELETE",
      `${one(uuid)}/damages/${damage}`,
    );
  },
  addPhoto(slug: string, uuid: string, body: InspectionPhotoRequest) {
    return tenantRequest<InspectionPhotoUpload>(
      slug,
      "POST",
      `${one(uuid)}/photos`,
      { body, silent: true },
    );
  },
  /** Upload to the API-issued address (an API path, never a storage host). */
  async uploadPhoto(slug: string, path: string, file: Blob, name: string) {
    if (!path.startsWith("/v1/tenant/inspections/")) {
      throw new Error("inspections: unexpected upload address");
    }
    const form = new FormData();
    form.append("file", file, name);
    const response = await fetch(
      `${apiConfig.baseUrl.replace(/\/$/, "")}${path}`,
      {
        method: "PUT",
        credentials: "include",
        headers: { [ORGANIZATION_HEADER]: slug },
        body: form,
      },
    );
    const payload = await response.json().catch(() => undefined);
    return unwrap<InspectionPhoto>(
      { data: payload, response },
      { silent: true },
    );
  },
  completePhoto(slug: string, uuid: string, photo: string) {
    return tenantRequest<InspectionPhoto>(
      slug,
      "POST",
      `${one(uuid)}/photos/${photo}/complete`,
      { silent: true },
    );
  },
  deletePhoto(slug: string, uuid: string, photo: string) {
    return tenantRequest<Inspection>(
      slug,
      "DELETE",
      `${one(uuid)}/photos/${photo}`,
    );
  },
  signature(slug: string, uuid: string, body: InspectionSignatureRequest) {
    return tenantRequest<Inspection>(slug, "POST", `${one(uuid)}/signature`, {
      body,
      silent: true,
    });
  },
  complete(slug: string, uuid: string, body: InspectionCompleteRequest) {
    return tenantRequest<Inspection>(slug, "POST", `${one(uuid)}/complete`, {
      body,
      silent: true,
    });
  },
  revise(slug: string, uuid: string, reason: string) {
    return tenantRequest<Inspection>(slug, "POST", `${one(uuid)}/revise`, {
      body: { reason },
    });
  },
  review(slug: string, uuid: string, body: InspectionReviewRequest) {
    return tenantRequest<Inspection>(slug, "POST", `${one(uuid)}/review`, {
      body,
    });
  },

  templates(
    slug: string,
    params: { limit: number; offset: number; sort?: string; q?: string },
  ) {
    return tenantRequest<InspectionTemplatePage>(slug, "GET", TEMPLATES, {
      query: params,
    });
  },
  activeTemplate(slug: string) {
    return tenantRequest<InspectionTemplate>(
      slug,
      "GET",
      `${TEMPLATES}/active`,
    );
  },
  saveTemplate(slug: string, body: InspectionTemplateRequest) {
    return tenantRequest<InspectionTemplate>(slug, "POST", TEMPLATES, { body });
  },
};
