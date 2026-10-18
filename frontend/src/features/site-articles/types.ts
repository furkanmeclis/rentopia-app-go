import type { components, operations } from "@/generated/api";

type Schemas = components["schemas"];

export type SiteArticle = Schemas["SiteArticle"];
export type SiteArticleInput = Schemas["SiteArticleInput"];
export type SiteArticleUpdate = Schemas["SiteArticleUpdate"];
export type SiteArticleBody = Schemas["SiteArticleBody"];
export type SiteArticlePage = Schemas["EnvelopeSiteArticleList"]["data"];
export type SiteArticleStatus = SiteArticle["status"];
export type ListSiteArticlesParams = NonNullable<
  operations["listTenantSiteArticles"]["parameters"]["query"]
>;
export type ArticleLocale = "tr" | "en";
