import { expect, type FrameLocator, type Page } from "@playwright/test";
import { createServer, type Server } from "node:http";

import { apiBase, e2eEnv, siteBase } from "./env";
import type { Fixture } from "./seed";

export type PublicItem = {
  subgroup: { uuid: string; title?: string };
  available: boolean;
  units_left: number;
  price: { total: string; currency: string } | null;
};

/** The public booking API's answer for the fixture search (secret key). */
export async function publicAvailability(f: Fixture): Promise<PublicItem[]> {
  const q = new URLSearchParams({
    pickup_branch_uuid: f.branches.mrk,
    return_branch_uuid: f.branches.mrk,
    starts_at: f.search.startsAt,
    ends_at: f.search.endsAt,
  });
  const res = await fetch(`${apiBase()}/v1/public/availability?${q}`, {
    headers: { Authorization: `Bearer ${f.publicApiKey}` },
  });
  const body = (await res.json()) as { data?: { items: PublicItem[] } };
  expect(res.status, JSON.stringify(body)).toBe(200);
  return body.data?.items ?? [];
}

/** Istanbul calendar date and time ("YYYY-MM-DD", "HH:MM") of an instant. */
function istanbul(iso: string): [string, string] {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(new Date(iso));
  const v = (t: string) => parts.find((p) => p.type === t)?.value ?? "";
  return [
    `${v("year")}-${v("month")}-${v("day")}`,
    `${v("hour")}:${v("minute")}`,
  ];
}

/** The site's search query for the fixture search. */
export function searchQuery(f: Fixture): string {
  const [pd, pt] = istanbul(f.search.startsAt);
  const [rd, rt] = istanbul(f.search.endsAt);
  return new URLSearchParams({
    pickup: f.branches.mrk,
    pickup_date: pd,
    pickup_time: pt,
    return_date: rd,
    return_time: rt,
  }).toString();
}

/** Digits of a formatted amount ("₺4.500,00" → 450000) or "4500.00". */
export function cents(text: string): number {
  const t = text.trim();
  if (/^\d+(\.\d{1,2})?$/.test(t)) return Math.round(Number(t) * 100);
  return Number(t.replace(/[^\d]/g, ""));
}

/** The result cards of a search page (site or widget frame). */
export async function cards(root: Page | FrameLocator) {
  const ids = root.locator('form.rt-select-form input[name="subgroup_uuid"]');
  const totals = root.locator(".rt-price-total");
  await expect(totals.first()).toBeVisible({ timeout: 60_000 });
  return {
    subgroups: await ids.evaluateAll((els) =>
      els.map((e) => (e as HTMLInputElement).value),
    ),
    totals: await totals.allInnerTexts(),
  };
}

/**
 * A partner's web page that embeds the widget: a real loopback server on
 * the partner origin, so the widget is a cross-origin frame of the site as
 * on a customer's site. (A page faked with page.route has no address, and
 * Chromium's local-network-access check then holds the frame's request to
 * the local site.) Close the returned server when done.
 */
export async function openPartnerWidget(
  page: Page,
  f: Fixture,
): Promise<{ frame: FrameLocator; server: Server }> {
  const src = `${siteBase(f.site.subdomain)}/embed/search?key=${encodeURIComponent(
    f.site.publishableKey,
  )}&${searchQuery(f)}`;
  const html = `<!doctype html><html lang="tr"><head><meta charset="utf-8"><title>Partner</title></head>
<body style="margin:0;font-family:sans-serif"><h1 style="font-size:16px;margin:8px">Partner acente</h1>
<iframe id="rentopya-widget" title="Rezervasyon" src="${src}" style="width:100%;height:700px;border:0"></iframe></body></html>`;
  const origin = new URL(e2eEnv.partnerOrigin);
  const server = createServer((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(html);
  });
  await new Promise<void>((resolve) =>
    server.listen(Number(origin.port), "127.0.0.1", resolve),
  );
  await page.goto(`${e2eEnv.partnerOrigin}/kiralama`);
  return { frame: page.frameLocator("#rentopya-widget"), server };
}
