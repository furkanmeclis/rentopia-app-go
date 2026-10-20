/**
 * PLACEHOLDER: the address the "Demo talep et" links open. Replace it with
 * the real sales / contact address before the landing page goes live.
 */
export const LANDING_CONTACT_EMAIL = "info@rentopya.com";

/** Hero product loop (16:9). The frame shows a static mock until it loads. */
export const LANDING_HERO_VIDEO = {
  mp4: "/landing/hero.mp4",
  webm: "/landing/hero.webm",
  poster: "/landing/hero-poster.jpg",
} as const;

/** `mailto:` link for the demo request, with a localized subject line. */
export function demoMailto(subject: string) {
  return `mailto:${LANDING_CONTACT_EMAIL}?subject=${encodeURIComponent(subject)}`;
}

/** "Panele giriş": the panel's sign-in page (`app/(guest)/login`). */
export const LANDING_LOGIN_HREF = "/login";
