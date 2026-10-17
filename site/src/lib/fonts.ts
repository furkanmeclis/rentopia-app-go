import {
  DM_Sans,
  Inter,
  Lato,
  Lora,
  Manrope,
  Montserrat,
  Nunito,
  Open_Sans,
  Plus_Jakarta_Sans,
  Poppins,
  Roboto,
  Sora,
  Wix_Madefor_Text,
} from "next/font/google";

/**
 * The theme fonts (config.theme.font / heading_font). next/font downloads
 * them at build time and serves them from this site: no visitor request
 * goes to Google. Nothing is preloaded; a browser only downloads the face
 * the organization's theme uses. All families are SIL Open Font License
 * 1.1 (Google Fonts). Each defines the CSS variable lib/theme.ts maps.
 */
// next/font needs literal options in every call.
const inter = Inter({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-inter",
});
const roboto = Roboto({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-roboto",
});
const openSans = Open_Sans({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-open-sans",
});
const montserrat = Montserrat({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-montserrat",
});
const poppins = Poppins({
  subsets: ["latin", "latin-ext"],
  weight: ["400", "500", "600", "700"],
  display: "swap",
  preload: false,
  variable: "--font-poppins",
});
const lato = Lato({
  subsets: ["latin", "latin-ext"],
  weight: ["400", "700"],
  display: "swap",
  preload: false,
  variable: "--font-lato",
});
const nunito = Nunito({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-nunito",
});
const manrope = Manrope({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-manrope",
});
const dmSans = DM_Sans({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-dm-sans",
});
const jakarta = Plus_Jakarta_Sans({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-plus-jakarta-sans",
});
const sora = Sora({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-sora",
});
const lora = Lora({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-lora",
});
const wix = Wix_Madefor_Text({
  subsets: ["latin", "latin-ext"],
  display: "swap",
  preload: false,
  variable: "--font-wix-madefor",
});

/** Class names that define every font variable (on <html>). */
export const fontVariables = [
  inter,
  roboto,
  openSans,
  montserrat,
  poppins,
  lato,
  nunito,
  manrope,
  dmSans,
  jakarta,
  sora,
  lora,
  wix,
]
  .map((f) => f.variable)
  .join(" ");
