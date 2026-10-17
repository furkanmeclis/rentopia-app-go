import { z } from "zod";

import type { SiteSaveInput, SiteSettings } from "@/features/site/types";

/** Limits of the site document (backend/internal/modules/site/usecase). */
export const SITE_LIMITS = {
  heroTitle: 120,
  heroSubtitle: 300,
  longText: 20000,
  faq: 40,
  question: 300,
  answer: 3000,
  seoTitle: 70,
  seoDescription: 300,
  contact: 300,
  widgetDomains: 20,
  rentalDays: 90,
} as const;

export const COLOR_TOKENS = [
  "primary",
  "primary_foreground",
  "accent",
  "background",
  "foreground",
  "muted",
  "muted_foreground",
  "border",
] as const;
export type ColorToken = (typeof COLOR_TOKENS)[number];

export const FONTS = [
  "inter",
  "roboto",
  "open_sans",
  "montserrat",
  "poppins",
  "lato",
  "nunito",
  "system",
] as const;
export const RADII = ["none", "sm", "md", "lg", "full"] as const;
export const MODES = ["light", "dark", "system"] as const;
export const HEROES = ["centered", "split", "image", "minimal"] as const;
export const CARDS = ["grid", "list", "compact"] as const;
export const FOOTERS = ["simple", "columns", "minimal"] as const;
export const SITE_LOCALES = ["tr", "en"] as const;

/** The widget branch select's "none" item (a select item cannot be ""). */
export const WIDGET_NO_BRANCH = "__none__";

type T = (key: string, vars?: Record<string, string | number>) => string;

export function createSiteSchema(t: T) {
  const max = (n: number) =>
    z.string().max(n, t("site.validation.too_long", { max: n }));
  const color = z
    .string()
    .trim()
    .regex(/^#[0-9a-fA-F]{6}$/, t("site.validation.color"));
  const colors = z.object(
    Object.fromEntries(COLOR_TOKENS.map((k) => [k, color])) as Record<
      ColorToken,
      typeof color
    >,
  );
  const content = z.object({
    hero_title: max(SITE_LIMITS.heroTitle),
    hero_subtitle: max(SITE_LIMITS.heroSubtitle),
    about: max(SITE_LIMITS.longText),
    terms: max(SITE_LIMITS.longText),
    privacy: max(SITE_LIMITS.longText),
    faq: z
      .array(
        z.object({
          question: z
            .string()
            .trim()
            .min(1, t("site.validation.required"))
            .max(
              SITE_LIMITS.question,
              t("site.validation.too_long", { max: SITE_LIMITS.question }),
            ),
          answer: z
            .string()
            .trim()
            .min(1, t("site.validation.required"))
            .max(
              SITE_LIMITS.answer,
              t("site.validation.too_long", { max: SITE_LIMITS.answer }),
            ),
        }),
      )
      .max(
        SITE_LIMITS.faq,
        t("site.validation.faq_max", { max: SITE_LIMITS.faq }),
      ),
  });
  const seo = z.object({
    title: max(SITE_LIMITS.seoTitle),
    description: max(SITE_LIMITS.seoDescription),
  });
  const phone = z
    .string()
    .trim()
    .regex(/^(\+?[0-9 ()-]{7,20})?$/, t("site.validation.phone"));
  return z.object({
    enabled: z.boolean(),
    subdomain: z
      .string()
      .trim()
      .toLowerCase()
      .regex(
        /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/,
        t("site.validation.subdomain"),
      ),
    default_locale: z.enum(SITE_LOCALES),
    config: z.object({
      theme: z.object({
        colors,
        dark_colors: colors,
        font: z.enum(FONTS),
        radius: z.enum(RADII),
        mode: z.enum(MODES),
      }),
      layout: z.object({
        hero: z.enum(HEROES),
        cards: z.enum(CARDS),
        footer: z.enum(FOOTERS),
      }),
      content: z.object({ tr: content, en: content }),
      seo: z.object({ tr: seo, en: seo }),
      contact: z.object({
        phone,
        whatsapp: phone,
        email: z
          .string()
          .trim()
          .max(SITE_LIMITS.contact)
          .refine(
            (v) => v === "" || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v),
            t("site.validation.email"),
          ),
        address: max(SITE_LIMITS.contact),
      }),
      analytics: z.object({
        ga4_measurement_id: z
          .string()
          .trim()
          .toUpperCase()
          .regex(/^(G-[A-Z0-9]{4,16})?$/, t("site.validation.ga4")),
      }),
      widget: z.object({
        allowed_domains: z.array(z.object({ value: z.string().trim() })).max(
          SITE_LIMITS.widgetDomains,
          t("site.validation.domains_max", {
            max: SITE_LIMITS.widgetDomains,
          }),
        ),
        initial_filters: z.object({
          pickup_branch_uuid: z.string(),
          // A string in the form (empty = site default), a number in the API.
          rental_days: z
            .string()
            .trim()
            .refine(
              (v) =>
                v === "" ||
                (/^\d{1,3}$/.test(v) && Number(v) <= SITE_LIMITS.rentalDays),
              t("site.validation.rental_days", { max: SITE_LIMITS.rentalDays }),
            ),
        }),
      }),
    }),
  });
}

export type SiteFormValues = z.infer<ReturnType<typeof createSiteSchema>>;

const emptyContent = {
  hero_title: "",
  hero_subtitle: "",
  about: "",
  terms: "",
  privacy: "",
  faq: [] as { question: string; answer: string }[],
};

/** The settings as form values (every field present). */
export function settingsToForm(s: SiteSettings): SiteFormValues {
  const c = s.config;
  const content = (loc: "tr" | "en") => ({
    ...emptyContent,
    ...(c.content?.[loc] ?? {}),
    faq: (c.content?.[loc]?.faq ?? []).map((f) => ({ ...f })),
  });
  const seo = (loc: "tr" | "en") => ({
    title: c.seo?.[loc]?.title ?? "",
    description: c.seo?.[loc]?.description ?? "",
  });
  const colors = (v: Partial<Record<ColorToken, string>> | undefined) =>
    Object.fromEntries(COLOR_TOKENS.map((k) => [k, v?.[k] ?? ""])) as Record<
      ColorToken,
      string
    >;
  return {
    enabled: s.enabled,
    subdomain: s.subdomain,
    default_locale: s.default_locale,
    config: {
      theme: {
        colors: colors(c.theme?.colors),
        dark_colors: colors(c.theme?.dark_colors),
        font: c.theme?.font ?? "inter",
        radius: c.theme?.radius ?? "md",
        mode: c.theme?.mode ?? "light",
      },
      layout: {
        hero: c.layout?.hero ?? "centered",
        cards: c.layout?.cards ?? "grid",
        footer: c.layout?.footer ?? "simple",
      },
      content: { tr: content("tr"), en: content("en") },
      seo: { tr: seo("tr"), en: seo("en") },
      contact: {
        phone: c.contact?.phone ?? "",
        whatsapp: c.contact?.whatsapp ?? "",
        email: c.contact?.email ?? "",
        address: c.contact?.address ?? "",
      },
      analytics: { ga4_measurement_id: c.analytics?.ga4_measurement_id ?? "" },
      widget: {
        allowed_domains: (c.widget?.allowed_domains ?? []).map((value) => ({
          value,
        })),
        initial_filters: {
          pickup_branch_uuid:
            c.widget?.initial_filters?.pickup_branch_uuid ?? "",
          rental_days: c.widget?.initial_filters?.rental_days
            ? String(c.widget.initial_filters.rental_days)
            : "",
        },
      },
    },
  };
}

/** Form values → PUT /v1/tenant/site. */
export function formToSaveInput(
  v: SiteFormValues,
  version: number,
): SiteSaveInput {
  return {
    version,
    enabled: v.enabled,
    subdomain: v.subdomain,
    default_locale: v.default_locale,
    config: {
      ...v.config,
      widget: {
        allowed_domains: v.config.widget.allowed_domains
          .map((d) => d.value.trim())
          .filter(Boolean),
        initial_filters: {
          pickup_branch_uuid:
            v.config.widget.initial_filters.pickup_branch_uuid ===
            WIDGET_NO_BRANCH
              ? ""
              : v.config.widget.initial_filters.pickup_branch_uuid,
          rental_days: Number(v.config.widget.initial_filters.rental_days || 0),
        },
      },
    },
  };
}
