import type { Locale, MenuItem } from "./types";

/**
 * Links that come from the organization's settings (menus, slide CTAs,
 * social profiles, phone numbers). The API validates them on save; the site
 * checks again before rendering an href (pure: node --test imports it).
 */

/**
 * A same-site path ("/…", not "//…") or an https URL without credentials
 * or a non-standard port; anything else (javascript:, data:, protocol-
 * relative, backslashes, whitespace, control characters) → null.
 */
export function safeHref(raw: string | null | undefined): string | null {
  const v = (raw ?? "").trim();
  if (!v || v.length > 2048) return null;
  if (/[\\\s\u0000-\u001f\u007f]/.test(v)) return null;
  let decoded: string;
  try {
    decoded = decodeURIComponent(v);
  } catch {
    return null;
  }
  if (/[\\\r\n\u0000]/.test(decoded) || decoded.startsWith("//")) return null;
  if (v.startsWith("/")) return v.startsWith("//") ? null : v;
  try {
    const u = new URL(v);
    if (u.protocol !== "https:" || !u.hostname) return null;
    if (u.username || u.password) return null;
    if (u.port && u.port !== "443") return null;
    return u.toString();
  } catch {
    return null;
  }
}

export function isExternal(href: string): boolean {
  return href.startsWith("https://");
}

export type NavLink = { href: string; label: string; external: boolean };

/** A localized label: the visitor's locale, then Turkish, then English. */
export function localized(
  value: Partial<Record<Locale, string>> | null | undefined,
  locale: Locale,
): string {
  const v = value?.[locale] || value?.tr || value?.en || "";
  return typeof v === "string" ? v.trim() : "";
}

/**
 * The configured menu (max 12 items with a label and a safe href), or the
 * fallback when the organization configured none (or none survives).
 */
export function menuLinks(
  items: MenuItem[] | null | undefined,
  locale: Locale,
  fallback: NavLink[],
): NavLink[] {
  const out: NavLink[] = [];
  for (const item of Array.isArray(items) ? items.slice(0, 12) : []) {
    const href = safeHref(item?.href);
    const label = localized(item?.label, locale);
    if (href && label) out.push({ href, label, external: isExternal(href) });
  }
  return out.length ? out : fallback;
}

/** Hosts each social network's profile URL must use (or a subdomain of). */
export const SOCIAL_HOSTS: Record<string, string[]> = {
  instagram: ["instagram.com"],
  facebook: ["facebook.com", "fb.com"],
  x: ["x.com", "twitter.com"],
  youtube: ["youtube.com", "youtu.be"],
  linkedin: ["linkedin.com"],
  tiktok: ["tiktok.com"],
  whatsapp: ["wa.me", "whatsapp.com"],
};

export const SOCIAL_LABELS: Record<string, { name: string; short: string }> = {
  instagram: { name: "Instagram", short: "IG" },
  facebook: { name: "Facebook", short: "FB" },
  x: { name: "X", short: "X" },
  youtube: { name: "YouTube", short: "YT" },
  linkedin: { name: "LinkedIn", short: "IN" },
  tiktok: { name: "TikTok", short: "TT" },
  whatsapp: { name: "WhatsApp", short: "WA" },
};

export type SocialLink = {
  network: string;
  href: string;
  name: string;
  short: string;
};

/** Valid social profiles in a fixed order: https on the network's own host. */
export function socialLinks(
  socials: Record<string, string> | null | undefined,
): SocialLink[] {
  const out: SocialLink[] = [];
  for (const network of Object.keys(SOCIAL_HOSTS)) {
    const href = safeHref(socials?.[network]);
    if (!href || !isExternal(href)) continue;
    const host = new URL(href).hostname.toLowerCase();
    const ok = SOCIAL_HOSTS[network].some(
      (h) => host === h || host.endsWith(`.${h}`),
    );
    if (ok) out.push({ network, href, ...SOCIAL_LABELS[network] });
  }
  return out;
}

/** tel: link of a phone number (digits and a leading +), or null. */
export function telHref(phone: string | null | undefined): string | null {
  const digits = (phone ?? "").replace(/[^+0-9]/g, "").replace(/(?!^)\+/g, "");
  return digits.replace("+", "").length >= 3 ? `tel:${digits}` : null;
}

/** wa.me link of a WhatsApp number, or null. */
export function whatsappHref(phone: string | null | undefined): string | null {
  const digits = (phone ?? "").replace(/[^0-9]/g, "");
  return digits.length >= 6 && digits.length <= 15
    ? `https://wa.me/${digits}`
    : null;
}

/** mailto: link of a plausible e-mail address, or null. */
export function mailHref(email: string | null | undefined): string | null {
  const v = (email ?? "").trim();
  return /^[^\s@<>"']+@[^\s@<>"']+\.[^\s@<>"']+$/.test(v) && v.length <= 254
    ? `mailto:${v}`
    : null;
}
