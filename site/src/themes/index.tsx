import type { Ctx } from "@/lib/server";
import type { ArticlePage, PublicArticle, Site } from "@/lib/types";

import {
  Footer as ClassicFooter,
  Header as ClassicHeader,
} from "./classic/chrome";
import { Home as ClassicHome } from "./classic/home";
import {
  Content as ClassicContent,
  Vehicles as ClassicVehicles,
} from "./classic/pages";
import {
  Footer as HorizonFooter,
  Header as HorizonHeader,
} from "./horizon/chrome";
import { Home as HorizonHome } from "./horizon/home";
import { Content as HorizonContent } from "./horizon/pages";
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
import { ArticleDetail, GuideList } from "./technowide/sections/guide";

// Order matters: Horizon and classic draw the shared `rt-*` / `tw-*` page
// markup on top of the Technowide stylesheet (same layer, same
// specificity); each is scoped to its own root class.
import "./technowide/technowide.css";
import "./horizon/horizon.css";
import "./classic/classic.css";

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
  /** Root class(es) on <body>: scope the theme stylesheet(s). */
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
  /** /guide and /guide/{locale}/{slug} (TEC-785); data read by the route. */
  Guide: (p: {
    ctx: Ctx;
    page: number;
    data: ArticlePage | null;
  }) => React.ReactNode;
  Article: (p: { ctx: Ctx; article: PublicArticle }) => React.ReactNode;
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
    Guide: GuideList,
    Article: ArticleDetail,
    Inquiry,
  },
  // Horizon (TEC-787): its own header, footer, home, about and FAQ; the
  // fleet, office, campaign, guide, contact, legal and application pages are
  // the shared Technowide markup restyled under `.theme-horizon`.
  "horizon-design": {
    id: "horizon-design",
    className: "theme-technowide theme-horizon",
    Header: HorizonHeader,
    Footer: HorizonFooter,
    Home: HorizonHome,
    Vehicles,
    Branches: BranchList,
    Branch: BranchDetail,
    Content: HorizonContent,
    Campaigns: CampaignList,
    Campaign: CampaignDetail,
    Guide: GuideList,
    Article: ArticleDetail,
    Inquiry,
  },
  // classic (TEC-787): its own header, footer, home, vehicles, about and
  // FAQ; offices, campaigns, contact, legal and application pages are the
  // shared Technowide markup restyled under `.theme-classic`.
  classic: {
    id: "classic",
    className: "theme-technowide theme-classic",
    Header: ClassicHeader,
    Footer: ClassicFooter,
    Home: ClassicHome,
    Vehicles: ClassicVehicles,
    Branches: BranchList,
    Branch: BranchDetail,
    Content: ClassicContent,
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
