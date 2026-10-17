import type { Metadata, Viewport } from "next";
import {
  Inter,
  Lato,
  Montserrat,
  Nunito,
  Open_Sans,
  Poppins,
  Roboto,
} from "next/font/google";
import Script from "next/script";

import { SiteFooter, SiteHeader } from "@/components/site/chrome";
import { getCtx, mediaUrl } from "@/lib/server";
import { safeGa4Id, themeCss } from "@/lib/theme";

import "./globals.css";

// Self-hosted at build time (no visitor request goes to Google). Only the
// font the theme picks is downloaded by the browser.
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
const fontVars = [inter, roboto, openSans, montserrat, poppins, lato, nunito]
  .map((f) => f.variable)
  .join(" ");

// Every request renders for its host: nothing is cached across hosts.
export const dynamic = "force-dynamic";

export async function generateMetadata(): Promise<Metadata> {
  const { site, locale } = await getCtx();
  const seo = site.config.seo?.[locale];
  const favicon = mediaUrl(site.assets.favicon) ?? mediaUrl(site.assets.logo);
  return {
    metadataBase: new URL(site.origin),
    title: {
      default: seo?.title || site.organization.name,
      template: `%s · ${site.organization.name}`,
    },
    description: seo?.description || undefined,
    openGraph: {
      siteName: site.organization.name,
      title: seo?.title || site.organization.name,
      description: seo?.description || undefined,
      locale: locale === "tr" ? "tr_TR" : "en_GB",
      type: "website",
    },
    icons: favicon ? { icon: favicon } : undefined,
    alternates: { canonical: "/" },
    // A draft preview is never indexed.
    robots: site.preview ? { index: false, follow: false } : undefined,
  };
}

export async function generateViewport(): Promise<Viewport> {
  const { site } = await getCtx();
  return {
    themeColor: /^#[0-9a-f]{6}$/i.test(site.config.theme.colors.primary)
      ? site.config.theme.colors.primary
      : undefined,
  };
}

export default async function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const ctx = await getCtx();
  const ga = safeGa4Id(ctx.site.config.analytics?.ga4_measurement_id);
  return (
    <html lang={ctx.locale} className={fontVars}>
      <head>
        {/* Validated tokens only (lib/theme.ts). */}
        <style
          id="site-theme"
          dangerouslySetInnerHTML={{ __html: themeCss(ctx.site.config.theme) }}
        />
      </head>
      <body
        className={
          ctx.embed
            ? "flex min-h-0 min-w-0 flex-col"
            : "flex min-h-svh flex-col"
        }
      >
        <a
          href="#main"
          className="focus:bg-primary focus:text-primary-foreground sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded focus:px-3 focus:py-2"
        >
          {ctx.t("nav.skip")}
        </a>
        {ctx.site.preview ? (
          <div
            role="status"
            className="bg-accent text-foreground flex flex-wrap items-center justify-center gap-x-3 gap-y-1 px-4 py-2 text-center text-sm font-medium"
          >
            <span>{ctx.t("preview.banner")}</span>
            <form action="/preview/exit" method="get">
              <button
                type="submit"
                className="cursor-pointer underline underline-offset-2"
              >
                {ctx.t("preview.exit")}
              </button>
            </form>
          </div>
        ) : null}
        {!ctx.embed ? <SiteHeader ctx={ctx} /> : null}
        <main id="main" className="flex-1">
          {children}
        </main>
        {!ctx.embed ? <SiteFooter ctx={ctx} /> : null}
        {ga && !ctx.site.preview && !ctx.embed ? (
          <>
            <Script
              src={`https://www.googletagmanager.com/gtag/js?id=${ga}`}
              strategy="afterInteractive"
            />
            <Script id="ga4" strategy="afterInteractive">
              {`window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','${ga}',{anonymize_ip:true});`}
            </Script>
          </>
        ) : null}
      </body>
    </html>
  );
}
