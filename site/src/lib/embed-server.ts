import "server-only";

import { headers } from "next/headers";

import { EMBED_HEADER, embedPath, type EmbedContext } from "./embed";
import { PATH_HEADER } from "./request";

export async function embedContext(): Promise<EmbedContext | null> {
  const h = await headers();
  if (h.get(EMBED_HEADER) !== "1") return null;
  const url = new URL(h.get(PATH_HEADER) || "/", "https://site.invalid");
  return {
    fields: Object.fromEntries(
      ["s", "parent", "widgetId", "locale", "subgroup"].map((k) => [
        k,
        url.searchParams.get(k) || "",
      ]),
    ),
  };
}

export async function bookingPath(path: string): Promise<string> {
  return embedPath(path, await embedContext());
}
