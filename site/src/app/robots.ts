import type { MetadataRoute } from "next";

import { indexable } from "@/lib/app-env";
import { getSite } from "@/lib/server";

export const dynamic = "force-dynamic";

/**
 * Per host: booking steps are not indexed; the sitemap is per host too.
 * Outside APP_ENV=production (staging) nothing is indexed.
 */
export default async function robots(): Promise<MetadataRoute.Robots> {
  const site = await getSite();
  if (!indexable()) {
    return { rules: { userAgent: "*", disallow: "/" } };
  }
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: ["/book/", "/reservation/", "/search"],
    },
    host: site.origin,
    sitemap: `${site.origin}/sitemap.xml`,
  };
}
