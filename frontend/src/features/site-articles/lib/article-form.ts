import { z } from "zod";

import {
  deserializeBody,
  serializeBody,
  type BodyIssue,
  type PlateNode,
} from "@/components/plate/value";
import type {
  ArticleLocale,
  SiteArticle,
  SiteArticleBody,
  SiteArticleInput,
} from "@/features/site-articles/types";

export const ARTICLE_LOCALES: ArticleLocale[] = ["tr", "en"];

/** The API's limits (docs/design/branded-site.md §13). */
export const ARTICLE_FIELD_LIMITS = {
  slug: 160,
  title: 160,
  summary: 1000,
  seoTitle: 70,
  seoDescription: 300,
  tags: 10,
  tag: 60,
} as const;

export const ARTICLE_SLUG = /^[a-z0-9]+(-[a-z0-9]+)*$/;

/** Route names the API refuses as slugs. */
export const RESERVED_SLUGS = new Set(
  "api,admin,platform,t,media,assets,lang,preview,embed,book,search,reservation,lookup,vehicles,branches,about,faq,contact,terms,privacy,corporate,franchise,campaigns,guide,articles,sitemap,sitemap.xml,robots,robots.txt,www,app".split(
    ",",
  ),
);

const TR_MAP: Record<string, string> = {
  ç: "c",
  ğ: "g",
  ı: "i",
  i̇: "i",
  ö: "o",
  ş: "s",
  ü: "u",
  â: "a",
  î: "i",
  û: "u",
};

/** "İstanbul'da Araç Kiralama!" → "istanbulda-arac-kiralama". */
export function slugify(value: string): string {
  return value
    .toLocaleLowerCase("tr")
    .replace(/[çğıöşüâîû]|i̇/g, (c) => TR_MAP[c] ?? c)
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/['’`]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, ARTICLE_FIELD_LIMITS.slug)
    .replace(/-+$/g, "");
}

type Localized = { tr: string; en: string };

export type ArticleFormValues = {
  slug: Localized;
  title: Localized;
  summary: Localized;
  seo_title: Localized;
  seo_description: Localized;
  cover_media_uuid: string;
  tags: string[];
  sort: number;
  featured: boolean;
};

/** The two locales' editor values (kept outside the form state). */
export type ArticleBodies = Record<ArticleLocale, PlateNode[]>;

type T = (key: string, vars?: Record<string, string | number>) => string;

export function createArticleSchema(t: T) {
  const text = (max: number) =>
    z
      .string()
      .max(max, t("site.articles.errors.too_long", { max }))
      .refine((v) => !/[<>]/.test(v), t("site.articles.errors.plain_text"));
  const localized = (max: number) => z.object({ tr: text(max), en: text(max) });
  return z
    .object({
      slug: localized(ARTICLE_FIELD_LIMITS.slug),
      title: localized(ARTICLE_FIELD_LIMITS.title),
      summary: localized(ARTICLE_FIELD_LIMITS.summary),
      seo_title: localized(ARTICLE_FIELD_LIMITS.seoTitle),
      seo_description: localized(ARTICLE_FIELD_LIMITS.seoDescription),
      cover_media_uuid: z.string(),
      tags: z.array(text(ARTICLE_FIELD_LIMITS.tag)).max(
        ARTICLE_FIELD_LIMITS.tags,
        t("site.articles.errors.too_many_tags", {
          max: ARTICLE_FIELD_LIMITS.tags,
        }),
      ),
      sort: z.number().int(),
      featured: z.boolean(),
    })
    .superRefine((v, ctx) => {
      let any = false;
      for (const loc of ARTICLE_LOCALES) {
        const slug = v.slug[loc].trim();
        if (!slug) continue;
        any = true;
        if (!ARTICLE_SLUG.test(slug)) {
          ctx.addIssue({
            code: "custom",
            path: ["slug", loc],
            message: t("site.articles.errors.slug_format"),
          });
        } else if (RESERVED_SLUGS.has(slug)) {
          ctx.addIssue({
            code: "custom",
            path: ["slug", loc],
            message: t("site.articles.errors.slug_reserved"),
          });
        }
        if (!v.title[loc].trim()) {
          ctx.addIssue({
            code: "custom",
            path: ["title", loc],
            message: t("site.articles.errors.title_required"),
          });
        }
      }
      if (!any) {
        ctx.addIssue({
          code: "custom",
          path: ["slug", "tr"],
          message: t("site.articles.errors.one_locale"),
        });
      }
    }) as unknown as z.ZodType<ArticleFormValues>;
}

const loc = (v: { tr?: string; en?: string } | undefined): Localized => ({
  tr: v?.tr ?? "",
  en: v?.en ?? "",
});

export function emptyArticleForm(): ArticleFormValues {
  return {
    slug: { tr: "", en: "" },
    title: { tr: "", en: "" },
    summary: { tr: "", en: "" },
    seo_title: { tr: "", en: "" },
    seo_description: { tr: "", en: "" },
    cover_media_uuid: "",
    tags: [],
    sort: 0,
    featured: false,
  };
}

export function articleToForm(a: SiteArticle): ArticleFormValues {
  return {
    slug: loc(a.slug),
    title: loc(a.title),
    summary: loc(a.summary),
    seo_title: loc(a.seo_title),
    seo_description: loc(a.seo_description),
    cover_media_uuid: a.cover_media_uuid ?? "",
    tags: a.tags ?? [],
    sort: a.sort ?? 0,
    featured: Boolean(a.featured),
  };
}

export function articleBodies(a: SiteArticle | null): ArticleBodies {
  return {
    tr: deserializeBody(a?.body?.tr),
    en: deserializeBody(a?.body?.en),
  };
}

/** Canonical JSON of the saved shape (dirty check of the editors). */
export function bodyFingerprint(value: PlateNode[]): string {
  return JSON.stringify(serializeBody(value).nodes);
}

const trimmed = (v: Localized): Localized => ({
  tr: v.tr.trim(),
  en: v.en.trim(),
});

/**
 * The form as the API body, or the body problems that keep it from being
 * saved (invalid links, images without library media).
 */
export function formToInput(
  values: ArticleFormValues,
  bodies: ArticleBodies,
):
  | { ok: true; input: SiteArticleInput }
  | { ok: false; issues: Record<ArticleLocale, BodyIssue[]> } {
  const tr = serializeBody(bodies.tr);
  const en = serializeBody(bodies.en);
  if (tr.issues.length || en.issues.length) {
    return { ok: false, issues: { tr: tr.issues, en: en.issues } };
  }
  return {
    ok: true,
    input: {
      slug: trimmed(values.slug),
      title: trimmed(values.title),
      summary: trimmed(values.summary),
      seo_title: trimmed(values.seo_title),
      seo_description: trimmed(values.seo_description),
      cover_media_uuid: values.cover_media_uuid || null,
      body: {
        tr: tr.nodes as unknown as SiteArticleBody,
        en: en.nodes as unknown as SiteArticleBody,
      },
      tags: values.tags.map((tag) => tag.trim()).filter(Boolean),
      sort: Number.isFinite(values.sort) ? Math.trunc(values.sort) : 0,
      featured: values.featured,
    },
  };
}

/** A body problem from the API (`body.tr.3.children.0.href`) or the client. */
export type BodyProblem = {
  locale: ArticleLocale;
  /** 1-based top-level block, when the path names one. */
  block: number | null;
  /** Last path key (`href`, `media_uuid`, `alt`, `type`, ...). */
  key: string;
  code: string;
};

export function bodyProblemFromField(
  field: string,
  code: string,
): BodyProblem | null {
  const m = /^body\.(tr|en)(?:\.(\d+))?(?:\..*?)?(?:\.([a-z_]+))?$/.exec(field);
  if (!m) return null;
  return {
    locale: m[1] as ArticleLocale,
    block: m[2] !== undefined ? Number(m[2]) + 1 : null,
    key: m[3] ?? "",
    code,
  };
}

export function bodyProblemFromIssue(
  locale: ArticleLocale,
  issue: BodyIssue,
): BodyProblem {
  const first = Number(issue.path.split(".")[0]);
  return {
    locale,
    block: Number.isFinite(first) ? first + 1 : null,
    key: issue.kind === "invalid_link" ? "href" : "media_uuid",
    code: issue.kind === "invalid_link" ? "invalid" : "required",
  };
}
