import assert from "node:assert/strict";
import { test } from "node:test";

import {
  campaignCard,
  campaignCards,
  campaignDate,
  campaignMediaUrl,
  campaignValidity,
} from "./campaigns.ts";

const SITE = "11111111-1111-4111-8111-111111111111";
const MEDIA = "22222222-2222-4222-8222-222222222222";
const A = "33333333-3333-4333-8333-333333333333";
const B = "44444444-4444-4444-8444-444444444444";

const campaign = (uuid: string, extra: Record<string, unknown> = {}) => ({
  uuid,
  title: { tr: "Hafta sonu", en: "Weekend" },
  summary: { tr: "Özet", en: "" },
  cta_label: { tr: "", en: "" },
  media_url: null,
  start_date: "2026-10-01",
  end_date: "2026-10-31",
  ...extra,
});

test("campaign media must be this site's library stream", () => {
  const site = { site_uuid: SITE };
  assert.equal(
    campaignMediaUrl(site, `/v1/public/site/media/${SITE}/${MEDIA}`),
    `/media/site/media/${SITE}/${MEDIA}`,
  );
  for (const bad of [
    `/v1/public/site/media/${A}/${MEDIA}`,
    `/v1/public/site/media/${SITE}/${MEDIA}?preview=x`,
    `https://evil.test/v1/public/site/media/${SITE}/${MEDIA}`,
    `/v1/public/site/media/${SITE}/../${MEDIA}`,
    "",
    null,
    undefined,
  ])
    assert.equal(campaignMediaUrl(site, bad), null);
});

test("cards: localized text, safe fields, API order, no duplicates", () => {
  const cards = campaignCards(
    {
      site_uuid: SITE,
      campaigns: [
        campaign(B, {
          media_url: `/v1/public/site/media/${SITE}/${MEDIA}`,
          code: "HAFTA10",
        }),
        campaign(A, { code: "<b>", start_date: "31.10.2026" }),
        campaign(B),
        { uuid: "not-a-uuid" } as never,
      ],
    },
    "en",
  );
  assert.deepEqual(
    cards.map((c) => c.uuid),
    [B, A],
  );
  assert.equal(cards[0].title, "Weekend");
  // Missing English falls back to Turkish.
  assert.equal(cards[0].summary, "Özet");
  assert.equal(cards[0].cta, "");
  assert.equal(cards[0].image, `/media/site/media/${SITE}/${MEDIA}`);
  assert.equal(cards[0].code, "HAFTA10");
  assert.equal(cards[1].code, null);
  assert.equal(cards[1].start, null);
  assert.deepEqual(campaignCards({ site_uuid: SITE }, "tr"), []);
  assert.equal(
    campaignCard({ site_uuid: SITE, campaigns: [campaign(A)] }, A, "tr")?.uuid,
    A,
  );
  assert.equal(
    campaignCard({ site_uuid: SITE, campaigns: [campaign(A)] }, B, "tr"),
    null,
  );
  assert.equal(
    campaignCard({ site_uuid: SITE, campaigns: [campaign(A)] }, "x", "tr"),
    null,
  );
});

test("validity sentence and calendar dates", () => {
  const today = "2026-10-18";
  assert.deepEqual(
    campaignValidity({ start: "2026-10-01", end: "2026-10-31" }, today),
    {
      kind: "until",
      end: "2026-10-31",
    },
  );
  assert.deepEqual(
    campaignValidity({ start: "2026-11-01", end: "2026-11-30" }, today),
    {
      kind: "between",
      start: "2026-11-01",
      end: "2026-11-30",
    },
  );
  assert.deepEqual(
    campaignValidity({ start: "2026-11-01", end: null }, today),
    {
      kind: "from",
      start: "2026-11-01",
    },
  );
  assert.deepEqual(campaignValidity({ start: null, end: null }, today), {
    kind: "open",
  });
  assert.equal(campaignDate("2026-01-01", "en"), "1 Jan 2026");
  assert.match(campaignDate("2026-01-01", "tr"), /1 Oca 2026/);
  assert.equal(campaignDate("bad", "tr"), "bad");
});
