import type { Ctx } from "@/lib/server";
import type { Site } from "@/lib/types";

import { resolveThemeId, type ThemeId } from "./registry";
import { Footer, Header } from "./technowide/chrome";
import { Home } from "./technowide/home";
import {
  BranchDetail,
  BranchList,
  Content,
  Vehicles,
} from "./technowide/pages";

import "./technowide/technowide.css";

export type ContentPage = "about" | "faq" | "contact" | "terms" | "privacy";

/**
 * A storefront template renderer. Every route asks the registry for the
 * renderer of the resolved `template`, so adding Horizon / classic / Aurum
 * means adding an entry here (and its id to registry.ts). Booking pages
 * are shared: they use the semantic `rt-*` classes every theme stylesheet
 * draws under its root class.
 */
export type ThemeRenderer = {
  id: ThemeId;
  /** Root class on <body>: scopes the theme stylesheet. */
  className: string;
  Header: (p: { ctx: Ctx }) => Promise<React.ReactNode>;
  Footer: (p: { ctx: Ctx }) => Promise<React.ReactNode>;
  Home: (p: { ctx: Ctx }) => Promise<React.ReactNode>;
  Vehicles: (p: { ctx: Ctx; segment?: string }) => Promise<React.ReactNode>;
  Branches: (p: { ctx: Ctx; city?: string }) => Promise<React.ReactNode>;
  Branch: (p: { ctx: Ctx; uuid: string }) => Promise<React.ReactNode>;
  Content: (p: { ctx: Ctx; page: ContentPage }) => React.ReactNode;
};

const RENDERERS: Record<ThemeId, ThemeRenderer> = {
  "technowide-design": {
    id: "technowide-design",
    className: "theme-technowide",
    Header,
    Footer,
    Home,
    Vehicles,
    Branches: BranchList,
    Branch: BranchDetail,
    Content,
  },
};

/** The renderer of a site's template; unknown templates render Technowide. */
export function themeFor(
  site: Pick<Site, "template" | "config">,
): ThemeRenderer {
  return RENDERERS[resolveThemeId(site.template ?? site.config?.template)];
}
