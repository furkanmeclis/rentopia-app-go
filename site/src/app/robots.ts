import type { MetadataRoute } from "next";

import { getSite } from "@/lib/server";

export const dynamic = "force-dynamic";

/** Per host: booking steps are not indexed. */
export default async function robots(): Promise<MetadataRoute.Robots> {
  const site = await getSite();
  return {
    rules: {
      userAgent: "*",
      allow: "/",
      disallow: ["/book/", "/reservation/", "/search"],
    },
    host: site.origin,
  };
}
