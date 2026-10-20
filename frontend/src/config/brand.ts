/**
 * App identity tokens (Rentopya brand kit, see docs/brand.md).
 * Hex aliases for JS (theme-color, icons). CSS tokens live in theme.css.
 */
export const brand = {
  name: "Rentopya",
  productName: "Rentopya",
  tagline: "Araç kiralama yönetim platformu",
  /** Custom URL scheme of the companion mobile app (QR sign-in fallback). */
  mobileScheme: "rentopya",
  colors: {
    /** Brand indigo — light-mode primary. */
    primary: "#2E358C",
    primaryForeground: "#FFFFFF",
    /** Lighter indigo — dark-mode primary (ink text on it). */
    primaryDark: "#8E95EC",
    /** Brand ink — wordmark and foreground text. */
    ink: "#141527",
    /** slate-100 */
    mist: "#F1F5F9",
    white: "#FFFFFF",
    /** Dark-mode page background (theme-color in dark mode). */
    navy: "#020617",
    lightGray: "#F1F5F9",
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon" | "stacked";
