import type { Locale, Site } from "./types";

/**
 * Site lead forms (TEC-786): newsletter, contact, corporate rental and
 * franchise application (pure: node --test imports it).
 *
 * Forms are plain HTML POSTs to a server action, which calls
 * `POST /v1/public/site/leads` with the visitor's booking session and then
 * redirects (post / redirect / get). The redirect query carries only the
 * outcome and field error codes, never what the visitor typed; values come
 * back through a short-lived httpOnly cookie (see lead-actions.ts).
 */

export const LEAD_KINDS = [
  "newsletter",
  "contact",
  "corporate",
  "franchise",
] as const;
export type LeadKind = (typeof LEAD_KINDS)[number];

export const LEAD_FIELDS = [
  "name",
  "email",
  "phone",
  "company",
  "city",
  "fleet_size",
  "message",
  "consent_kvkk",
  "consent_marketing",
] as const;
export type LeadField = (typeof LEAD_FIELDS)[number];

/** Text limits of the API (characters). */
export const LEAD_LIMITS: Partial<Record<LeadField, number>> = {
  name: 160,
  email: 254,
  phone: 40,
  company: 200,
  city: 100,
  message: 5000,
};
export const FLEET_SIZE_MAX = 100_000;

/** Fields each form shows, in order, and which of them are required. */
export const LEAD_FORM_FIELDS: Record<
  LeadKind,
  { fields: LeadField[]; required: LeadField[] }
> = {
  newsletter: {
    fields: ["email"],
    // A bulletin is commercial e-mail: it needs the marketing consent too.
    required: ["email", "consent_kvkk", "consent_marketing"],
  },
  contact: {
    fields: ["name", "email", "phone", "message"],
    required: ["name", "email", "message", "consent_kvkk"],
  },
  corporate: {
    fields: ["company", "name", "email", "phone", "fleet_size", "message"],
    required: ["company", "name", "email", "phone", "consent_kvkk"],
  },
  franchise: {
    fields: ["name", "email", "phone", "city", "message"],
    required: ["name", "email", "phone", "city", "consent_kvkk"],
  },
};

/** Where each form lives (the action redirects back there). */
export const LEAD_PATHS: Record<LeadKind, string> = {
  newsletter: "/",
  contact: "/contact",
  corporate: "/corporate",
  franchise: "/franchise",
};

/** Anchor id of each form (the redirect scrolls back to it). */
export const leadAnchor = (kind: LeadKind) => `${kind}-form`;

export function isLeadKind(v: unknown): v is LeadKind {
  return typeof v === "string" && (LEAD_KINDS as readonly string[]).includes(v);
}

export type LeadFlags = Record<LeadKind, boolean>;

/**
 * Forms the site offers: the published `config.leads` flag (every form is
 * on when the configuration has no `leads` object) and a usable
 * publishable key (`booking_enabled`): without a key no session can submit.
 */
export function leadFlags(
  site: Pick<Site, "booking_enabled" | "publishable_key"> & {
    config: Pick<Site["config"], "leads">;
  },
): LeadFlags {
  const usable = Boolean(site.booking_enabled && site.publishable_key);
  const flags = site.config?.leads;
  const on = (kind: LeadKind) =>
    usable && (flags == null || flags[kind] !== false);
  return {
    newsletter: on("newsletter"),
    contact: on("contact"),
    corporate: on("corporate"),
    franchise: on("franchise"),
  };
}

export type LeadErrorCode = "required" | "invalid" | "too_long";
export type LeadErrors = Partial<Record<LeadField, LeadErrorCode>>;
export type LeadValues = Partial<Record<LeadField, string>>;

/** Body of POST /v1/public/site/leads. */
export type LeadInput = {
  kind: LeadKind;
  name: string;
  email: string;
  phone: string;
  company: string;
  city: string;
  message: string;
  fleet_size: number | null;
  locale: Locale;
  consent_kvkk: boolean;
  consent_marketing: boolean;
  honeypot: string;
};

const EMAIL = /^[^\s@<>"'(),;:\\]+@[^\s@<>"'(),;:\\]+\.[^\s@<>"'(),;:\\]+$/;
const PHONE = /^\+?[0-9 ()./-]{6,40}$/;

const chars = (s: string) => [...s].length;

/**
 * Reads a submitted form (any `get(name)` source: FormData, a map) and
 * checks it like the API does, so most mistakes come back without a round
 * trip. The API stays the authority: its 400 details are mapped too.
 */
export function readLeadForm(
  kind: LeadKind,
  get: (name: string) => unknown,
  locale: Locale,
): { input: LeadInput; values: LeadValues; errors: LeadErrors } {
  const text = (name: string) => {
    const v = get(name);
    return typeof v === "string" ? v.replace(/\r\n?/g, "\n").trim() : "";
  };
  const { fields, required } = LEAD_FORM_FIELDS[kind];
  const shown = (f: LeadField) => fields.includes(f);
  const values: LeadValues = {};
  for (const f of fields) values[f] = text(f);
  const consentKvkk = get("consent_kvkk") === "on";
  const consentMarketing = get("consent_marketing") === "on";
  if (consentMarketing) values.consent_marketing = "on";

  const errors: LeadErrors = {};
  for (const f of fields) {
    const limit = LEAD_LIMITS[f];
    const v = values[f] ?? "";
    if (!v) {
      if (required.includes(f)) errors[f] = "required";
      continue;
    }
    if (limit && chars(v) > limit) errors[f] = "too_long";
  }
  const email = (values.email ?? "").toLowerCase();
  if (email && !errors.email && !EMAIL.test(email)) errors.email = "invalid";
  if (values.phone && !errors.phone && !PHONE.test(values.phone))
    errors.phone = "invalid";
  let fleet: number | null = null;
  if (shown("fleet_size") && values.fleet_size) {
    const n = Number(values.fleet_size);
    if (
      !/^\d{1,6}$/.test(values.fleet_size) ||
      !Number.isInteger(n) ||
      n > FLEET_SIZE_MAX
    )
      errors.fleet_size = "invalid";
    else fleet = n;
  }
  if (!consentKvkk) errors.consent_kvkk = "required";
  if (required.includes("consent_marketing") && !consentMarketing)
    errors.consent_marketing = "required";

  return {
    values,
    errors,
    input: {
      kind,
      name: shown("name") ? (values.name ?? "") : "",
      email,
      phone: shown("phone") ? (values.phone ?? "") : "",
      company: shown("company") ? (values.company ?? "") : "",
      city: shown("city") ? (values.city ?? "") : "",
      message: shown("message") ? (values.message ?? "") : "",
      fleet_size: fleet,
      locale,
      consent_kvkk: consentKvkk,
      consent_marketing: consentMarketing,
      honeypot: text("website"),
    },
  };
}

/** The API's 400 details (`{field, code}`) as form errors. */
export function apiLeadErrors(
  details: { field?: string; code?: string }[],
): LeadErrors {
  const errors: LeadErrors = {};
  for (const d of details) {
    const field = d.field as LeadField;
    if (!(LEAD_FIELDS as readonly string[]).includes(field)) continue;
    errors[field] =
      d.code === "required"
        ? "required"
        : d.code === "too_long"
          ? "too_long"
          : "invalid";
  }
  return errors;
}

/** Outcome of a submission, carried by the redirect query. */
export type LeadStatus =
  "sent" | "invalid" | "limited" | "closed" | "preview" | "failed";
const STATUSES: LeadStatus[] = [
  "sent",
  "invalid",
  "limited",
  "closed",
  "preview",
  "failed",
];

/** An API failure → outcome (never "sent"). */
export function leadStatusOf(status: number, code: string): LeadStatus {
  if (status === 429) return "limited";
  if (status === 400) return "invalid";
  if (code === "PREVIEW_READ_ONLY") return "preview";
  if (status === 404 || code === "BOOKING_DISABLED") return "closed";
  return "failed";
}

export type LeadResult = { status: LeadStatus; errors: LeadErrors };

/** `/contact?lead=invalid&lead_kind=contact&fe=name.required#contact-form` */
export function leadRedirect(kind: LeadKind, result: LeadResult): string {
  const q = new URLSearchParams({ lead: result.status, lead_kind: kind });
  const fe = Object.entries(result.errors)
    .map(([f, c]) => `${f}.${c}`)
    .join(",");
  if (fe) q.set("fe", fe);
  return `${LEAD_PATHS[kind]}?${q.toString()}#${leadAnchor(kind)}`;
}

type Params = Record<string, string | string[] | undefined>;

const one = (v: string | string[] | undefined) =>
  typeof v === "string" ? v : undefined;

/** The outcome for one form from the page's query (null: not submitted). */
export function readLeadResult(
  params: Params | null | undefined,
  kind: LeadKind,
): LeadResult | null {
  if (!params || one(params.lead_kind) !== kind) return null;
  const status = one(params.lead) as LeadStatus | undefined;
  if (!status || !STATUSES.includes(status)) return null;
  const errors: LeadErrors = {};
  for (const part of (one(params.fe) ?? "").split(",").slice(0, 20)) {
    const [f, c] = part.split(".");
    if (
      (LEAD_FIELDS as readonly string[]).includes(f) &&
      (c === "required" || c === "invalid" || c === "too_long")
    )
      errors[f as LeadField] = c;
  }
  return { status, errors };
}

/** Values kept between the POST and the redirected GET (cookie payload). */
/**
 * One cookie per form: the newsletter's lives on "/" and would shadow a
 * same-named cookie of a sub page (servers keep one value per name).
 */
export const leadFlashCookie = (kind: LeadKind) => `rtp_lead_${kind}`;
/** Encoded size kept below the browsers' 4 KB cookie limit. */
const FLASH_MAX = 3600;

/** The form's values as a cookie value (long messages are shortened). */
export function encodeLeadFlash(kind: LeadKind, values: LeadValues): string {
  const kept: LeadValues = {};
  for (const f of LEAD_FORM_FIELDS[kind].fields) {
    const v = values[f];
    if (v) kept[f] = [...v].slice(0, LEAD_LIMITS[f] ?? 12).join("");
  }
  if (values.consent_marketing) kept.consent_marketing = "on";
  const encode = () =>
    Buffer.from(JSON.stringify({ k: kind, v: kept }), "utf8").toString(
      "base64url",
    );
  let raw = encode();
  while (raw.length > FLASH_MAX && kept.message) {
    const chars = [...kept.message];
    kept.message = chars.slice(0, Math.floor(chars.length / 2)).join("");
    raw = encode();
  }
  return raw.length > FLASH_MAX ? "" : raw;
}

export function decodeLeadFlash(
  raw: string | undefined,
  kind: LeadKind,
): LeadValues {
  if (!raw || raw.length > 4096) return {};
  try {
    const parsed = JSON.parse(Buffer.from(raw, "base64url").toString("utf8"));
    if (parsed?.k !== kind || typeof parsed.v !== "object" || !parsed.v)
      return {};
    const out: LeadValues = {};
    for (const f of LEAD_FIELDS) {
      const v = parsed.v[f];
      if (typeof v === "string") out[f] = v;
    }
    return out;
  } catch {
    return {};
  }
}
