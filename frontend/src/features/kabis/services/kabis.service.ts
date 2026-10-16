import type { ResourceMeta } from "@/features/io/types";
import type {
  KabisFacets,
  KabisNotificationDetail,
  KabisNotificationPage,
  KabisResolution,
  KabisResolveInput,
  KabisRetryResult,
  ListKabisParams,
} from "@/features/kabis/types";
import {
  tenantDownloadFile,
  tenantFormRequest,
  tenantRequest,
} from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/kabis-notifications";
const one = (uuid: string) => `${BASE}/${uuid}`;

/**
 * KABİS notifications (`tenantRequest` only). Actions whose errors the
 * screens render themselves are `silent`.
 */
export const kabisService = {
  list(slug: string, params: ListKabisParams) {
    return tenantRequest<KabisNotificationPage>(slug, "GET", BASE, {
      query: params,
    });
  },
  meta(slug: string) {
    return tenantRequest<ResourceMeta>(slug, "GET", `${BASE}/meta`);
  },
  facets(slug: string) {
    return tenantRequest<KabisFacets>(slug, "GET", `${BASE}/facets`);
  },
  get(slug: string, uuid: string) {
    return tenantRequest<KabisNotificationDetail>(slug, "GET", one(uuid));
  },
  retry(slug: string, uuid: string) {
    return tenantRequest<KabisRetryResult>(slug, "POST", `${one(uuid)}/retry`, {
      silent: true,
    });
  },
  /** Mark manual / reconcile: multipart with the optional attachment. */
  resolve(
    slug: string,
    uuid: string,
    action: KabisResolution,
    input: KabisResolveInput,
  ) {
    const form = new FormData();
    form.set("reason", input.reason);
    form.set("reference", input.reference);
    if (input.file) form.set("file", input.file, input.file.name);
    return tenantFormRequest<KabisNotificationDetail>(
      slug,
      "POST",
      `${one(uuid)}/${action}`,
      form,
      { silent: true },
    );
  },
  attachment(slug: string, uuid: string) {
    return tenantDownloadFile(slug, `${one(uuid)}/attachment`);
  },
};
