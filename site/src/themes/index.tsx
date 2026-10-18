import type { Ctx } from "@/lib/server";
import type { Site } from "@/lib/types";

import { resolveThemeId, type ThemeId } from "./registry";
import { Footer, Header } from "./technowide/chrome";
import { Home } from "./technowide/home";
import {
  BranchDetail,
  BranchList,
  CampaignDetail,
  CampaignList,
  Content,
  Inquiry,
  Vehicles,
} from "./technowide/pages";

import "./technowide/technowide.css";

export type ContentPage = "about" | "faq" | "contact" | "terms" | "privacy";
/** Application forms with their own page (TEC-786). */
export type InquiryKind = "corporate" | "franchise";
/** The page's search params (form outcomes after a redirect). */
export type Query = Record<string, string | string[] | undefined>;

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
  Home: (p: { ctx: Ctx; query?: Query }) => Promise<React.ReactNode>;
  Vehicles: (p: { ctx: Ctx; segment?: string }) => Promise<React.ReactNode>;
  Branches: (p: { ctx: Ctx; city?: string }) => Promise<React.ReactNode>;
  Branch: (p: { ctx: Ctx; uuid: string }) => Promise<React.ReactNode>;
  Content: (p: {
    ctx: Ctx;
    page: ContentPage;
    query?: Query;
  }) => React.ReactNode;
  /** /campaigns and /campaigns/{uuid} (TEC-784). */
  Campaigns: (p: { ctx: Ctx }) => React.ReactNode;
  Campaign: (p: { ctx: Ctx; uuid: string }) => React.ReactNode;
  /** /corporate and /franchise (TEC-786). */
  Inquiry: (p: {
    ctx: Ctx;
    kind: InquiryKind;
    query?: Query;
  }) => React.ReactNode;
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
    Campaigns: CampaignList,
    Campaign: CampaignDetail,
    Inquiry,
  },
};

/** The renderer of a site's template; unknown templates render Technowide. */
export function themeFor(
  site: Pick<Site, "template" | "config">,
): ThemeRenderer {
  return RENDERERS[resolveThemeId(site.template ?? site.config?.template)];
}
