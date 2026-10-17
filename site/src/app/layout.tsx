import type { Metadata, Viewport } from "next";
import Script from "next/script";

import { fontVariables } from "@/lib/fonts";
import { getCtx, mediaUrl } from "@/lib/server";
import { safeGa4Id, themeCss } from "@/lib/theme";
import { themeFor } from "@/themes";

import "./globals.css";

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
  // The template renderer (src/themes); unknown templates render Technowide.
  const theme = themeFor(ctx.site);
  return (
    <html lang={ctx.locale} className={fontVariables}>
      <head>
        {/* Validated tokens only (lib/theme.ts). */}
        <style
          id="site-theme"
          dangerouslySetInnerHTML={{
            __html: themeCss(ctx.site.config.theme, theme.id),
          }}
        />
      </head>
      <body
        className={`${theme.className} ${ctx.embed ? "rt-body rt-body--embed" : "rt-body"}`}
        data-template={theme.id}
      >
        <a href="#main" className="rt-skip">
          {ctx.t("nav.skip")}
        </a>
        {ctx.site.preview ? (
          <div role="status" className="rt-preview-banner">
            <span>{ctx.t("preview.banner")}</span>
            <form action="/preview/exit" method="get">
              <button type="submit">{ctx.t("preview.exit")}</button>
            </form>
          </div>
        ) : null}
        {!ctx.embed ? <theme.Header ctx={ctx} /> : null}
        <main id="main" className="rt-main" tabIndex={-1}>
          {children}
        </main>
        {!ctx.embed ? <theme.Footer ctx={ctx} /> : null}
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
