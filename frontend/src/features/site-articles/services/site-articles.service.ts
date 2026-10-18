import type {
  ListSiteArticlesParams,
  SiteArticle,
  SiteArticleInput,
  SiteArticlePage,
  SiteArticleUpdate,
} from "@/features/site-articles/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const ARTICLES = "/v1/tenant/site/articles";

const path = (uuid: string) => `${ARTICLES}/${encodeURIComponent(uuid)}`;

/**
 * Guide articles of the branded site (TEC-785). Every write after creation
 * sends the article `version` (409 VERSION_CONFLICT otherwise); writes are
 * `silent`: the editor shows field errors and the conflict itself.
 */
export const siteArticlesService = {
  list(slug: string, params: ListSiteArticlesParams) {
    return tenantRequest<SiteArticlePage>(slug, "GET", ARTICLES, {
      query: params,
    });
  },
  get(slug: string, uuid: string) {
    return tenantRequest<SiteArticle>(slug, "GET", path(uuid));
  },
  create(slug: string, body: SiteArticleInput) {
    return tenantRequest<SiteArticle>(slug, "POST", ARTICLES, {
      body,
      silent: true,
    });
  },
  update(slug: string, uuid: string, body: SiteArticleUpdate) {
    return tenantRequest<SiteArticle>(slug, "PUT", path(uuid), {
      body,
      silent: true,
    });
  },
  publish(slug: string, uuid: string, version: number) {
    return tenantRequest<SiteArticle>(slug, "POST", `${path(uuid)}/publish`, {
      body: { version },
      silent: true,
    });
  },
  unpublish(slug: string, uuid: string, version: number) {
    return tenantRequest<SiteArticle>(slug, "POST", `${path(uuid)}/unpublish`, {
      body: { version },
      silent: true,
    });
  },
  remove(slug: string, uuid: string, version: number) {
    return tenantRequest<{ deleted: boolean }>(slug, "DELETE", path(uuid), {
      query: { version },
      silent: true,
    });
  },
};
