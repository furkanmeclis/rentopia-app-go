import {
  Award,
  BadgeCheck,
  Calendar,
  Car,
  Check,
  CircleCheckBig,
  CircleCheck,
  Clock,
  CreditCard,
  Fuel,
  Globe,
  Headphones,
  Heart,
  KeyRound,
  MapPin,
  Phone,
  Plane,
  Route,
  ShieldCheck,
  Star,
  ThumbsUp,
  Users,
  Wallet,
  Wrench,
  Zap,
  type LucideIcon,
} from "lucide-react";

import type { components } from "@/generated/api";

type Schemas = components["schemas"];

/** Body fonts of config v2 (the API enum of `theme.font`). */
export const BODY_FONTS = [
  "inter",
  "roboto",
  "open_sans",
  "montserrat",
  "poppins",
  "lato",
  "nunito",
  "system",
  "manrope",
  "dm_sans",
  "plus_jakarta_sans",
  "sora",
  "lora",
  "wix_madefor",
] as const satisfies readonly NonNullable<Schemas["SiteTheme"]["font"]>[];

/** Heading fonts: follow the body font, Georgia, or any body font. */
export const HEADING_FONTS = [
  "theme",
  "georgia",
  ...BODY_FONTS,
] as const satisfies readonly NonNullable<
  Schemas["SiteTheme"]["heading_font"]
>[];

/** The eight corner styles of config v2 (legacy none/full map to sharp/pill). */
export const CORNER_STYLES = [
  "soft",
  "sharp",
  "xs",
  "sm",
  "md",
  "lg",
  "xl",
  "pill",
] as const;
export type CornerStyle = (typeof CORNER_STYLES)[number];

/** Border radius of the small swatch of each corner style (panel only). */
export const CORNER_SWATCH_RADIUS: Record<CornerStyle, string> = {
  sharp: "0px",
  xs: "2px",
  sm: "4px",
  soft: "6px",
  md: "8px",
  lg: "12px",
  xl: "16px",
  pill: "9999px",
};

/** CSS font stacks for the option previews (system fallbacks only). */
export const FONT_PREVIEW_FAMILY: Record<
  (typeof HEADING_FONTS)[number],
  string | undefined
> = {
  theme: undefined,
  georgia: 'Georgia, "Times New Roman", serif',
  inter: '"Inter", system-ui, sans-serif',
  roboto: '"Roboto", system-ui, sans-serif',
  open_sans: '"Open Sans", system-ui, sans-serif',
  montserrat: '"Montserrat", system-ui, sans-serif',
  poppins: '"Poppins", system-ui, sans-serif',
  lato: '"Lato", system-ui, sans-serif',
  nunito: '"Nunito", system-ui, sans-serif',
  system: "system-ui, sans-serif",
  manrope: '"Manrope", system-ui, sans-serif',
  dm_sans: '"DM Sans", system-ui, sans-serif',
  plus_jakarta_sans: '"Plus Jakarta Sans", system-ui, sans-serif',
  sora: '"Sora", system-ui, sans-serif',
  lora: '"Lora", Georgia, serif',
  wix_madefor: '"Wix Madefor Text", system-ui, sans-serif',
};

export type TemplateId = Schemas["SiteConfig"]["template"];
export type SectionId = Schemas["SiteSection"]["id"];
export type ThemeDefinition = Schemas["SiteThemeDefinition"];
export type SectionStatus = NonNullable<
  Schemas["SiteSettings"]["section_statuses"]
>[number];

export const TEMPLATE_IDS = [
  "technowide-design",
  "horizon-design",
  "classic",
  "aurum",
] as const satisfies readonly TemplateId[];

export const DEFAULT_TEMPLATE: TemplateId = "technowide-design";

/** Every section id of the registry union (API order). */
export const SECTION_IDS = [
  "hero",
  "search",
  "fleet",
  "campaigns",
  "advantages",
  "about",
  "faq",
  "guide",
  "newsletter",
  "app_promo",
  "branches",
] as const satisfies readonly SectionId[];

/**
 * Sections that wait for a backend service (the API refuses enabling them
 * with SECTION_UNAVAILABLE). Mirrors `themes.NeedsService`; the API's
 * `section_statuses` wins for the saved template. Campaigns (TEC-784), the
 * newsletter (TEC-786) and the guide (TEC-785) have their services now, so
 * nothing waits today.
 */
export const NEEDS_SERVICE: ReadonlySet<SectionId> = new Set<SectionId>();

/** Website forms of `config.leads` (TEC-786), in display order. */
export const LEAD_FORMS = [
  "contact",
  "corporate",
  "franchise",
  "newsletter",
] as const;
export type LeadForm = (typeof LEAD_FORMS)[number];

/** i18n key segment of an id with dashes / camelCase (`technowide-design` → `technowide_design`). */
export function keySegment(id: string): string {
  return id
    .replace(/([a-z0-9])([A-Z])/g, "$1_$2")
    .replace(/[^a-zA-Z0-9]+/g, "_")
    .toLowerCase()
    .replace(/^(\d)/, "v_$1");
}

/** Allowlisted advantage icons (API enum) with their Lucide component. */
export const ADVANTAGE_ICONS = {
  "shield-check": ShieldCheck,
  clock: Clock,
  car: Car,
  "credit-card": CreditCard,
  headphones: Headphones,
  "map-pin": MapPin,
  check: Check,
  "check-circle": CircleCheckBig,
  star: Star,
  heart: Heart,
  award: Award,
  wallet: Wallet,
  zap: Zap,
  users: Users,
  phone: Phone,
  "thumbs-up": ThumbsUp,
  "badge-check": BadgeCheck,
  "circle-check": CircleCheck,
  "key-round": KeyRound,
  fuel: Fuel,
  plane: Plane,
  route: Route,
  wrench: Wrench,
  calendar: Calendar,
  globe: Globe,
} as const satisfies Record<
  NonNullable<Schemas["SiteAdvantage"]["icon"]>,
  LucideIcon
>;
export type AdvantageIcon = keyof typeof ADVANTAGE_ICONS;
export const ADVANTAGE_ICON_IDS = Object.keys(
  ADVANTAGE_ICONS,
) as AdvantageIcon[];

export const SOCIAL_NETWORKS = [
  "instagram",
  "facebook",
  "x",
  "youtube",
  "linkedin",
  "tiktok",
  "whatsapp",
] as const;
export type SocialNetwork = (typeof SOCIAL_NETWORKS)[number];

/** Hosts each network URL must use (mirrors the API check; server is authoritative). */
export const SOCIAL_HOSTS: Record<SocialNetwork, string[]> = {
  instagram: ["instagram.com"],
  facebook: ["facebook.com", "fb.com"],
  x: ["x.com", "twitter.com"],
  youtube: ["youtube.com", "youtu.be"],
  linkedin: ["linkedin.com"],
  tiktok: ["tiktok.com"],
  whatsapp: ["wa.me", "whatsapp.com"],
};

export const SOCIAL_PLACEHOLDERS: Record<SocialNetwork, string> = {
  instagram: "https://instagram.com/firmaniz",
  facebook: "https://facebook.com/firmaniz",
  x: "https://x.com/firmaniz",
  youtube: "https://youtube.com/@firmaniz",
  linkedin: "https://linkedin.com/company/firmaniz",
  tiktok: "https://tiktok.com/@firmaniz",
  whatsapp: "https://wa.me/905550000000",
};

export const MENU_KINDS = ["link", "page"] as const;

/** Collection limits of config v2 (backend/internal/modules/site/usecase/config_v2.go). */
export const V2_LIMITS = {
  slides: 8,
  slideTitle: 200,
  slideSubtitle: 1000,
  ctaLabel: 120,
  href: 2048,
  advantages: 12,
  advantageTitle: 120,
  advantageText: 1000,
  highlights: 6,
  highlight: 300,
  menuItems: 12,
  menuLabel: 120,
  tagline: 300,
} as const;

/** Site media library rules (TEC-781; the server is authoritative). */
export const MEDIA_RULES = {
  accept: ["image/png", "image/jpeg", "image/webp"],
  maxBytes: 10 * 1024 * 1024,
  maxFiles: 200,
  maxTotalBytes: 500 * 1024 * 1024,
} as const;

/** Relative `/path` or `https://…` (no protocol-relative, backslashes or spaces). */
export function isSafeHref(value: string): boolean {
  const v = value.trim();
  if (v === "") return true;
  if (/[\s\\]/.test(v)) return false;
  if (v.startsWith("//")) return false;
  if (v.startsWith("/")) return true;
  try {
    const url = new URL(v);
    return (
      url.protocol === "https:" &&
      Boolean(url.hostname) &&
      !url.username &&
      !url.password &&
      (url.port === "" || url.port === "443")
    );
  } catch {
    return false;
  }
}

export function isSocialUrl(network: SocialNetwork, value: string): boolean {
  const v = value.trim();
  if (v === "") return true;
  if (!isSafeHref(v) || v.startsWith("/")) return false;
  try {
    const host = new URL(v).hostname.toLowerCase();
    return SOCIAL_HOSTS[network].some(
      (h) => host === h || host.endsWith(`.${h}`),
    );
  } catch {
    return false;
  }
}
