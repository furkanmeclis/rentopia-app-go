import type { Metadata } from "next";

import { LandingPage } from "@/features/landing";

const TITLE = "Rentopya — Araç kiralama yönetim platformu";
const DESCRIPTION =
  "Araç kiralama firmaları için filo, şube, rezervasyon, sözleşme, teslim/iade, depozito ve fatura tek panelde. Kendi alan adınızda rezervasyon sitesi.";

export const metadata: Metadata = {
  title: { absolute: TITLE },
  description: DESCRIPTION,
  openGraph: {
    type: "website",
    siteName: "Rentopya",
    title: TITLE,
    description: DESCRIPTION,
    locale: "tr_TR",
    alternateLocale: ["en_US"],
  },
  twitter: {
    card: "summary",
    title: TITLE,
    description: DESCRIPTION,
  },
};

/**
 * Signed-out visitors only: for a signed-in user the proxy (`auth.ts`)
 * serves the dispatch page at `/` instead.
 */
export default function Home() {
  return <LandingPage />;
}
