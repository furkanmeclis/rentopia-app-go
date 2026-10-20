import type { Metadata } from "next";
import { headers } from "next/headers";

import { LandingPage } from "@/features/landing";

const TITLE = "Rentopya — Araç kiralama yönetim platformu";
const DESCRIPTION =
  "Araç kiralama firmaları için filo, şube, rezervasyon, sözleşme, teslim/iade, depozito ve fatura tek panelde. Kendi alan adınızda rezervasyon sitesi.";

const OG_IMAGE = {
  url: "/landing/og.png",
  width: 1200,
  height: 630,
  alt: TITLE,
};

/**
 * The public origin is only known per request (the image is built without an
 * app URL), so Open Graph URLs resolve against the host that served the page.
 */
export async function generateMetadata(): Promise<Metadata> {
  const headerList = await headers();
  const host = headerList.get("x-forwarded-host") ?? headerList.get("host");
  const proto = headerList.get("x-forwarded-proto") ?? "https";
  const metadataBase = host ? new URL(`${proto}://${host}`) : undefined;

  return {
    metadataBase,
    title: { absolute: TITLE },
    description: DESCRIPTION,
    alternates: { canonical: "/" },
    openGraph: {
      type: "website",
      url: "/",
      siteName: "Rentopya",
      title: TITLE,
      description: DESCRIPTION,
      locale: "tr_TR",
      alternateLocale: ["en_US"],
      images: [OG_IMAGE],
    },
    twitter: {
      card: "summary_large_image",
      title: TITLE,
      description: DESCRIPTION,
      images: [OG_IMAGE.url],
    },
  };
}

/**
 * Signed-out visitors only: for a signed-in user the proxy (`auth.ts`)
 * serves the dispatch page at `/` instead.
 */
export default function Home() {
  return <LandingPage />;
}
