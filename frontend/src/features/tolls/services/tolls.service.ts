import type { ResourceMeta } from "@/features/io/types";
import type {
  ListTollPassesParams,
  TollCandidate,
  TollFetchResult,
  TollPassDetail,
  TollPassFacets,
  TollPassPage,
  TollSettings,
  TollSettingsPatch,
  TollTag,
  TollTagCreate,
  TollTagPatch,
} from "@/features/tolls/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const PASSES = "/v1/tenant/toll-passes";
const one = (uuid: string) => `${PASSES}/${uuid}`;
const tags = (vehicle: string) => `/v1/tenant/vehicles/${vehicle}/toll-tags`;

/**
 * Toll passes, review decisions, settings and vehicle tags
 * (`tenantRequest` only). Decisions whose errors the dialogs render
 * themselves are `silent`.
 */
export const tollsService = {
  list(slug: string, params: ListTollPassesParams) {
    return tenantRequest<TollPassPage>(slug, "GET", PASSES, { query: params });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${PASSES}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<TollPassFacets>(slug, "GET", `${PASSES}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<TollPassDetail>(slug, "GET", one(uuid));
  },
  candidates(slug: string, uuid: string) {
    return tenantRequest<{ items: TollCandidate[] }>(
      slug,
      "GET",
      `${one(uuid)}/candidates`,
    );
  },
  assign(
    slug: string,
    uuid: string,
    body: { rental_uuid: string; reason: string },
  ) {
    return tenantRequest<TollPassDetail>(slug, "POST", `${one(uuid)}/assign`, {
      body,
      silent: true,
    });
  },
  unassign(slug: string, uuid: string, reason: string) {
    return tenantRequest<TollPassDetail>(
      slug,
      "POST",
      `${one(uuid)}/unassign`,
      {
        body: { reason },
        silent: true,
      },
    );
  },
  ignore(slug: string, uuid: string, reason: string) {
    return tenantRequest<TollPassDetail>(slug, "POST", `${one(uuid)}/ignore`, {
      body: { reason },
      silent: true,
    });
  },
  rematch(slug: string, uuid: string, reason: string) {
    return tenantRequest<TollPassDetail>(slug, "POST", `${one(uuid)}/rematch`, {
      body: { reason },
      silent: true,
    });
  },
  fetchNow(slug: string) {
    return tenantRequest<TollFetchResult>(slug, "POST", `${PASSES}/fetch`, {
      silent: true,
    });
  },
  settings(slug: string) {
    return tenantRequest<TollSettings>(slug, "GET", "/v1/tenant/toll-settings");
  },
  updateSettings(slug: string, body: TollSettingsPatch) {
    return tenantRequest<TollSettings>(
      slug,
      "PATCH",
      "/v1/tenant/toll-settings",
      { body, silent: true },
    );
  },
  tags(slug: string, vehicle: string) {
    return tenantRequest<{ items: TollTag[] }>(slug, "GET", tags(vehicle));
  },
  createTag(slug: string, vehicle: string, body: TollTagCreate) {
    return tenantRequest<TollTag>(slug, "POST", tags(vehicle), {
      body,
      silent: true,
    });
  },
  updateTag(slug: string, vehicle: string, tag: string, body: TollTagPatch) {
    return tenantRequest<TollTag>(slug, "PATCH", `${tags(vehicle)}/${tag}`, {
      body,
      silent: true,
    });
  },
  deleteTag(slug: string, vehicle: string, tag: string) {
    return tenantRequest<void>(slug, "DELETE", `${tags(vehicle)}/${tag}`);
  },
};
