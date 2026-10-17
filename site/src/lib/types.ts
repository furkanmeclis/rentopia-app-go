/**
 * Shapes of the API answers the site reads. Branded site settings come
 * from GET /v1/public/site/resolve (backend/internal/modules/site/model);
 * catalog, quotes and reservations from the public booking API
 * (backend/internal/modules/publicapi/model). Money is a decimal string.
 */

export type Locale = "tr" | "en";

export type SiteColors = {
  primary: string;
  primary_foreground: string;
  accent: string;
  background: string;
  foreground: string;
  muted: string;
  muted_foreground: string;
  border: string;
};

export type SiteTheme = {
  colors: SiteColors;
  dark_colors: SiteColors;
  font: string;
  /** Config v2: a body font id, "theme" (follow the theme) or "georgia". */
  heading_font?: string;
  radius: string;
  mode: "light" | "dark" | "system";
};

/** Config v2 hero slide (plain text; media is a site library UUID). */
export type HeroSlide = {
  title: string;
  subtitle: string;
  cta_label: string;
  cta_href: string;
  media: string;
  enabled: boolean;
};

/** Config v2 advantage: an allowlisted Lucide icon name and plain text. */
export type Advantage = { icon: string; title: string; text: string };

export type MenuItem = {
  label: Partial<Record<Locale, string>>;
  href: string;
  kind: "link" | "page";
};

export type SiteContent = {
  hero_slides?: HeroSlide[];
  advantages?: Advantage[];
  hero_title: string;
  hero_subtitle: string;
  about: string;
  faq: { question: string; answer: string }[];
  terms: string;
  privacy: string;
};

export type SiteConfig = {
  config_version?: number;
  template?: string;
  brand?: {
    tagline?: Partial<Record<Locale, string>>;
    socials?: Record<string, string>;
  };
  menus?: { header?: MenuItem[]; footer?: MenuItem[] };
  about?: {
    image?: string;
    highlights?: Partial<Record<Locale, string[]>>;
  };
  theme: SiteTheme;
  layout: { hero: string; cards: string; footer: string };
  content: Partial<Record<Locale, SiteContent>>;
  seo: Partial<Record<Locale, { title: string; description: string }>>;
  contact: { phone: string; email: string; address: string; whatsapp: string };
  analytics: { ga4_measurement_id: string };
  widget?: {
    allowed_domains: string[];
    initial_filters: { pickup_branch_uuid: string; rental_days: number };
  };
};

/** A section of the active template (resolve returns the enabled ones, in order). */
export type SiteSection = { id: string; enabled: boolean };

export type Site = {
  /** Config v2 (TEC-780): the selected template id. */
  template?: string;
  sections?: SiteSection[];
  /** Typed template settings (string values, e.g. heroLayout=image|split). */
  settings?: Record<string, string>;
  /** Library media UUID → API-owned stream path. */
  media?: Record<string, string>;
  site_uuid: string;
  version: number;
  host: string;
  host_kind: "subdomain" | "custom";
  origin: string;
  default_locale: Locale;
  locales: Locale[];
  organization: { uuid: string; slug: string; name: string };
  publishable_key: string;
  booking_enabled: boolean;
  config: SiteConfig;
  assets: { logo: string | null; favicon: string | null; cover: string | null };
  portal_url: string;
  updated_at: string;
  /** The draft for a preview token (TEC-730): banner, no booking. */
  preview?: boolean;
  /** Origin allowed to frame a preview page (the panel). */
  panel_origin?: string;
};

export type Place = { uuid: string; name: string };

export type ServicePoint = {
  uuid: string;
  type: string;
  name: string;
  description: string;
  extra_fee: string;
  delivery: boolean;
};

export type Location = {
  uuid: string;
  code: string;
  name: string;
  address: string;
  city: string;
  district: string;
  phone: string;
  email: string;
  latitude: number | null;
  longitude: number | null;
  points: ServicePoint[];
};

export type SubgroupRef = { uuid: string; code: string; title: string };

export type Subgroup = SubgroupRef & {
  description: string;
  segment: string;
  representative_model: string;
  conditions: {
    guarantee_label: string;
    min_driver_age: number;
    min_license_years: number;
    deposit_amount: string;
    included_km_per_day: number | null;
    fuel_policy: string;
  };
  images: string[];
};

export type AvailabilityItem = {
  subgroup: SubgroupRef;
  available: boolean;
  units_left: number;
  reasons: string[];
  price: {
    currency: string;
    billing_days: number;
    daily_average: string;
    total: string;
    prepayment: string;
  } | null;
};

export type Availability = {
  request: {
    pickup_branch: Place;
    return_branch: Place;
    starts_at: string;
    ends_at: string;
  };
  items: AvailabilityItem[];
  warnings: string[];
};

export type Extra = {
  uuid: string;
  code: string;
  name: string;
  description: string;
  type: string;
  charge_type: string;
  min_quantity: number;
  max_quantity: number;
};

export type Quote = {
  uuid: string;
  status: string;
  expired: boolean;
  valid_until: string;
  subgroup: SubgroupRef;
  pickup_branch: Place;
  return_branch: Place;
  pickup_point: Place | null;
  return_point: Place | null;
  starts_at: string;
  ends_at: string;
  billing_days: number;
  currency: string;
  rental_total: string;
  discount_total: string;
  extras_total: string;
  one_way_fee: string;
  delivery_fee: string;
  vat_total: string;
  total: string;
  prepayment: string;
  extras: { extra_uuid: string; quantity: number }[];
  lines: {
    kind: string;
    code: string;
    name: string;
    quantity: number;
    amount: string;
  }[];
};

export type Reservation = {
  reference: string;
  status: string;
  payment_status: string;
  starts_at: string;
  ends_at: string;
  pickup_branch: string;
  return_branch: string;
  pickup_point: string;
  return_point: string;
  subgroup: string;
  vehicle: string;
  drivers: string[];
  extras: { name: string; quantity: number; amount: string }[];
  price: {
    currency: string;
    total: string;
    prepayment: string;
    paid: string;
    prepayment_due: string;
    balance: string;
  };
  payment_deadline: string | null;
  policies: { cancellation: string };
  actions: { pay: boolean; pay_amount: string | null };
};

export type Booked = {
  reference: string;
  status: string;
  access_token: string;
  reservation: Reservation;
};

export type PaymentLink = {
  url: string;
  amount: string;
  currency: string;
  expires_at: string;
};
