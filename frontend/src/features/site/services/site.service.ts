import type {
  SiteCatalog,
  SiteCatalogBranch,
  SiteCatalogExtra,
  SiteCatalogKind,
  SiteCatalogSubgroup,
  SiteDomain,
  SiteImageKind,
  SitePreviewToken,
  SiteRevision,
  SiteSaveInput,
  SiteSettings,
} from "@/features/site/types";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/site";

/**
 * Branded site (TEC-729 / TEC-730) over the same-origin BFF. Every write
 * sends the settings `version` it saw (409 VERSION_CONFLICT otherwise).
 * The save is `silent`: the form shows field errors and the conflict.
 */
export const siteService = {
  get(slug: string) {
    return tenantRequest<SiteSettings>(slug, "GET", BASE);
  },
  save(slug: string, body: SiteSaveInput) {
    return tenantRequest<SiteSettings>(slug, "PUT", BASE, {
      body,
      silent: true,
    });
  },
  publish(slug: string, version: number) {
    return tenantRequest<SiteSettings>(slug, "POST", `${BASE}/publish`, {
      body: { version },
      silent: true,
    });
  },
  versions(slug: string) {
    return tenantRequest<SiteRevision[]>(slug, "GET", `${BASE}/versions`);
  },
  restore(slug: string, revision: string, version: number) {
    return tenantRequest<SiteSettings>(
      slug,
      "POST",
      `${BASE}/versions/${encodeURIComponent(revision)}/restore`,
      { body: { version }, silent: true },
    );
  },
  previewToken(slug: string) {
    return tenantRequest<SitePreviewToken>(
      slug,
      "POST",
      `${BASE}/preview-token`,
      { body: {} },
    );
  },
  renewKey(slug: string, version: number) {
    return tenantRequest<SiteSettings>(slug, "POST", `${BASE}/renew-key`, {
      body: { version },
      silent: true,
    });
  },
  uploadImage(slug: string, kind: SiteImageKind, file: File, version: number) {
    const form = new FormData();
    form.append("file", file);
    form.append("version", String(version));
    return tenantFormRequest<SiteSettings>(
      slug,
      "PUT",
      `${BASE}/assets/${kind}`,
      form,
      { silent: true },
    );
  },
  removeImage(slug: string, kind: SiteImageKind, version: number) {
    return tenantRequest<SiteSettings>(
      slug,
      "DELETE",
      `${BASE}/assets/${kind}`,
      { query: { version }, silent: true },
    );
  },
  catalog(slug: string) {
    return tenantRequest<SiteCatalog>(slug, "GET", `${BASE}/catalog`);
  },
  setOnline(
    slug: string,
    kind: SiteCatalogKind,
    uuid: string,
    showOnline: boolean,
  ) {
    return tenantRequest<
      SiteCatalogBranch | SiteCatalogSubgroup | SiteCatalogExtra
    >(slug, "PATCH", `${BASE}/catalog/${kind}/${encodeURIComponent(uuid)}`, {
      body: { show_online: showOnline },
    });
  },
  domains(slug: string) {
    return tenantRequest<SiteDomain[]>(slug, "GET", `${BASE}/domains`);
  },
  addDomain(slug: string, host: string) {
    return tenantRequest<SiteDomain>(slug, "POST", `${BASE}/domains`, {
      body: { host },
      silent: true,
    });
  },
  verifyDomain(slug: string, uuid: string) {
    return tenantRequest<SiteDomain>(
      slug,
      "POST",
      `${BASE}/domains/${encodeURIComponent(uuid)}/verify`,
      { body: {} },
    );
  },
  removeDomain(slug: string, uuid: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `${BASE}/domains/${encodeURIComponent(uuid)}`,
    );
  },
};
