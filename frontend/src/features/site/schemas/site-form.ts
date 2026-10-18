import { z } from "zod";

import {
  ADVANTAGE_ICON_IDS,
  BODY_FONTS,
  CORNER_STYLES,
  DEFAULT_TEMPLATE,
  HEADING_FONTS,
  MENU_KINDS,
  NEEDS_SERVICE,
  SOCIAL_NETWORKS,
  TEMPLATE_IDS,
  V2_LIMITS,
  isSafeHref,
  isSocialUrl,
  type AdvantageIcon,
  type SectionId,
  type SocialNetwork,
  type TemplateId,
  type ThemeDefinition,
} from "@/features/site/lib/site-options";
import type {
  SiteConfig,
  SiteSaveInput,
  SiteSettings,
} from "@/features/site/types";

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
  const trimmedMax = (n: number) =>
    z
      .string()
      .trim()
      .max(n, t("site.validation.too_long", { max: n }));
  const href = (required: boolean) =>
    z
      .string()
      .trim()
      .max(
        V2_LIMITS.href,
        t("site.validation.too_long", { max: V2_LIMITS.href }),
      )
      .refine((v) => !required || v !== "", t("site.validation.required"))
      .refine(isSafeHref, t("site.validation.href"));
  const slide = z.object({
    title: trimmedMax(V2_LIMITS.slideTitle),
    subtitle: trimmedMax(V2_LIMITS.slideSubtitle),
    cta_label: trimmedMax(V2_LIMITS.ctaLabel),
    cta_href: href(false),
    media: z.string(),
    enabled: z.boolean(),
  });
  const advantage = z.object({
    icon: z.enum(ADVANTAGE_ICON_IDS as [AdvantageIcon, ...AdvantageIcon[]]),
    title: trimmedMax(V2_LIMITS.advantageTitle).min(
      1,
      t("site.validation.required"),
    ),
    text: trimmedMax(V2_LIMITS.advantageText),
  });
  const localized = (n: number) =>
    z.object({ tr: trimmedMax(n), en: trimmedMax(n) });
  const menuItem = z.object({
    label: localized(V2_LIMITS.menuLabel).refine(
      (l) => l.tr !== "" || l.en !== "",
      { message: t("site.validation.menu_label"), path: ["tr"] },
    ),
    href: href(true),
    kind: z.enum(MENU_KINDS),
  });
  const menu = z
    .array(menuItem)
    .max(
      V2_LIMITS.menuItems,
      t("site.validation.items_max", { max: V2_LIMITS.menuItems }),
    );
  const highlights = z
    .array(z.object({ value: trimmedMax(V2_LIMITS.highlight) }))
    .max(
      V2_LIMITS.highlights,
      t("site.validation.items_max", { max: V2_LIMITS.highlights }),
    );
  const socials = z.object(
    Object.fromEntries(
      SOCIAL_NETWORKS.map((n) => [
        n,
        z
          .string()
          .trim()
          .refine((v) => isSocialUrl(n, v), t("site.validation.social")),
      ]),
    ) as Record<SocialNetwork, z.ZodString>,
  );
  const content = z.object({
    hero_slides: z
      .array(slide)
      .min(1, t("site.validation.slides_min"))
      .max(
        V2_LIMITS.slides,
        t("site.validation.items_max", { max: V2_LIMITS.slides }),
      ),
    advantages: z
      .array(advantage)
      .max(
        V2_LIMITS.advantages,
        t("site.validation.items_max", { max: V2_LIMITS.advantages }),
      ),
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
      leads: z.object({
        contact: z.boolean(),
        corporate: z.boolean(),
        franchise: z.boolean(),
        newsletter: z.boolean(),
      }),
      template: z.enum(TEMPLATE_IDS),
      templates: z.record(
        z.string(),
        z.object({
          sections: z.array(z.object({ id: z.string(), enabled: z.boolean() })),
          settings: z.record(z.string(), z.string()),
        }),
      ),
      theme: z.object({
        colors,
        dark_colors: colors,
        font: z.enum(BODY_FONTS),
        heading_font: z.enum(HEADING_FONTS),
        radius: z.enum(CORNER_STYLES),
        mode: z.enum(MODES),
      }),
      brand: z.object({
        tagline: localized(V2_LIMITS.tagline),
        socials,
      }),
      menus: z.object({ header: menu, footer: menu }),
      about: z.object({
        image: z.string(),
        highlights: z.object({ tr: highlights, en: highlights }),
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

export type SiteSlideValues =
  SiteFormValues["config"]["content"]["tr"]["hero_slides"][number];
export type TemplateFormConfig = SiteFormValues["config"]["templates"][string];

export function emptySlide(): SiteSlideValues {
  return {
    title: "",
    subtitle: "",
    cta_label: "",
    cta_href: "",
    media: "",
    enabled: true,
  };
}

/** Default enabled state of a section (mirrors `themes.Defaults`). */
function defaultEnabled(id: string) {
  return id !== "app_promo" && !NEEDS_SERVICE[id as SectionId];
}

/** A template's configuration completed against its registry definition. */
export function completeTemplateConfig(
  def: ThemeDefinition | undefined,
  current:
    | {
        sections?: { id: string; enabled: boolean }[];
        settings?: Record<string, string>;
      }
    | undefined,
): TemplateFormConfig {
  const sections = (current?.sections ?? []).map((x) => ({
    id: x.id,
    enabled: x.enabled,
  }));
  const settings: Record<string, string> = { ...(current?.settings ?? {}) };
  if (def) {
    if (!current?.sections) {
      sections.push(
        ...def.sections.map((id) => ({ id, enabled: defaultEnabled(id) })),
      );
    } else {
      for (const id of def.sections) {
        if (!sections.some((x) => x.id === id)) {
          sections.push({ id, enabled: false });
        }
      }
    }
    for (const setting of def.settings) {
      if (!(setting.key in settings)) settings[setting.key] = setting.default;
    }
  }
  return { sections, settings };
}

const emptyContent = {
  hero_title: "",
  hero_subtitle: "",
  about: "",
  terms: "",
  privacy: "",
  faq: [] as { question: string; answer: string }[],
  advantages: [] as SiteFormValues["config"]["content"]["tr"]["advantages"],
};

/** The settings as form values (every field present). */
export function settingsToForm(s: SiteSettings): SiteFormValues {
  const c = s.config;
  const content = (loc: "tr" | "en") => {
    const src = c.content?.[loc] ?? {};
    const slides = (src.hero_slides ?? []).map((x) => ({
      ...emptySlide(),
      ...x,
      enabled: x.enabled ?? true,
    }));
    return {
      ...emptyContent,
      hero_title: src.hero_title ?? "",
      hero_subtitle: src.hero_subtitle ?? "",
      about: src.about ?? "",
      terms: src.terms ?? "",
      privacy: src.privacy ?? "",
      faq: (src.faq ?? []).map((f) => ({ ...f })),
      hero_slides: slides.length
        ? slides
        : [
            {
              ...emptySlide(),
              title: src.hero_title ?? "",
              subtitle: src.hero_subtitle ?? "",
            },
          ],
      advantages: (src.advantages ?? []).map((a) => ({
        icon: (a.icon ?? "check") as AdvantageIcon,
        title: a.title ?? "",
        text: a.text ?? "",
      })),
    };
  };
  const registry = s.theme_registry ?? [];
  const template = (c.template ?? DEFAULT_TEMPLATE) as TemplateId;
  const templates: Record<string, TemplateFormConfig> = {};
  for (const [id, tc] of Object.entries(c.templates ?? {})) {
    templates[id] = completeTemplateConfig(
      registry.find((d) => d.id === id),
      { sections: tc.sections, settings: tc.settings },
    );
  }
  if (!templates[template]) {
    templates[template] = completeTemplateConfig(
      registry.find((d) => d.id === template),
      undefined,
    );
  }
  const highlights = (loc: "tr" | "en") =>
    (c.about?.highlights?.[loc] ?? []).map((value) => ({ value }));
  const radius = c.theme?.radius;
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
      // Every form is on when the document has no `leads` object (API default).
      leads: {
        contact: c.leads?.contact ?? true,
        corporate: c.leads?.corporate ?? true,
        franchise: c.leads?.franchise ?? true,
        newsletter: c.leads?.newsletter ?? true,
      },
      template,
      templates,
      theme: {
        colors: colors(c.theme?.colors),
        dark_colors: colors(c.theme?.dark_colors),
        font: c.theme?.font ?? "inter",
        heading_font: c.theme?.heading_font ?? "theme",
        radius:
          radius === "none"
            ? "sharp"
            : radius === "full"
              ? "pill"
              : (radius ?? "md"),
        mode: c.theme?.mode ?? "light",
      },
      brand: {
        tagline: {
          tr: c.brand?.tagline?.tr ?? "",
          en: c.brand?.tagline?.en ?? "",
        },
        socials: Object.fromEntries(
          SOCIAL_NETWORKS.map((n) => [n, c.brand?.socials?.[n] ?? ""]),
        ) as Record<SocialNetwork, string>,
      },
      menus: {
        header: (c.menus?.header ?? []).map(menuToForm),
        footer: (c.menus?.footer ?? []).map(menuToForm),
      },
      about: {
        image: c.about?.image ?? "",
        highlights: { tr: highlights("tr"), en: highlights("en") },
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

function menuToForm(
  item: NonNullable<NonNullable<SiteConfig["menus"]>["header"]>[number],
) {
  return {
    label: { tr: item.label?.tr ?? "", en: item.label?.en ?? "" },
    href: item.href ?? "",
    kind: item.kind ?? ("link" as const),
  };
}

/** Form values → PUT /v1/tenant/site. */
export function formToSaveInput(
  v: SiteFormValues,
  version: number,
  currentConfig?: SiteConfig,
): SiteSaveInput {
  const content = (loc: "tr" | "en") => {
    const src = v.config.content[loc];
    return {
      ...currentConfig?.content?.[loc],
      ...src,
      hero_slides: src.hero_slides.map((x) => ({
        ...x,
        title: x.title.trim(),
        subtitle: x.subtitle.trim(),
        cta_label: x.cta_label.trim(),
        cta_href: x.cta_href.trim(),
      })),
    };
  };
  const highlights = (loc: "tr" | "en") =>
    v.config.about.highlights[loc].map((x) => x.value.trim()).filter(Boolean);
  const socials = Object.fromEntries(
    Object.entries(v.config.brand.socials)
      .map(([k, url]) => [k, url.trim()] as const)
      .filter(([, url]) => url !== ""),
  );
  const menu = (items: SiteFormValues["config"]["menus"]["header"]) =>
    items.map((x) => ({
      label: { tr: x.label.tr.trim(), en: x.label.en.trim() },
      href: x.href.trim(),
      kind: x.kind,
    }));
  return {
    version,
    enabled: v.enabled,
    subdomain: v.subdomain,
    default_locale: v.default_locale,
    config: {
      ...currentConfig,
      ...v.config,
      config_version: 2,
      template: v.config.template,
      // Every template keeps its own sections / settings; ids come from the registry.
      templates: {
        ...currentConfig?.templates,
        ...v.config.templates,
      } as SiteConfig["templates"],
      theme: { ...currentConfig?.theme, ...v.config.theme },
      brand: {
        tagline: {
          tr: v.config.brand.tagline.tr.trim(),
          en: v.config.brand.tagline.en.trim(),
        },
        socials,
      },
      menus: {
        header: menu(v.config.menus.header),
        footer: menu(v.config.menus.footer),
      },
      about: {
        ...currentConfig?.about,
        image: v.config.about.image,
        highlights: { tr: highlights("tr"), en: highlights("en") },
      },
      content: { tr: content("tr"), en: content("en") },
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
