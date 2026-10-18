import assert from "node:assert/strict";
import { test } from "node:test";

import {
  articleMediaUrl,
  articlePath,
  homeArticleCards,
  isArticleSlug,
  toArticleCards,
} from "./articles.ts";

const SITE = "11111111-1111-4111-8111-111111111111";
const MEDIA = "22222222-2222-4222-8222-222222222222";
const card = (slug: string, extra: Record<string, unknown> = {}) => ({
  title: `T ${slug}`,
  summary: "S",
  slug,
  cover_url: null,
  published_at: "2026-10-18T09:00:00Z",
  ...extra,
});

test("article paths and slugs", () => {
  assert.equal(articlePath("tr", "rehber"), "/guide/tr/rehber");
  assert.equal(isArticleSlug("istanbul-rehberi-2"), true);
  for (const bad of ["", "A", "a--b", "-a", "a/b", "../x", "a b", null])
    assert.equal(isArticleSlug(bad as string), false);
});

test("covers only from this site's library stream", () => {
  assert.equal(
    articleMediaUrl(
      { site_uuid: SITE },
      `/v1/public/site/media/${SITE}/${MEDIA}`,
    ),
    `/media/site/media/${SITE}/${MEDIA}`,
  );
  const other = "33333333-3333-4333-8333-333333333333";
  for (const bad of [
    `/v1/public/site/media/${other}/${MEDIA}`,
    `https://cdn.example.com/${MEDIA}.png`,
    `/v1/public/site/media/${SITE}/${MEDIA}?x=1`,
    null,
  ])
    assert.equal(articleMediaUrl({ site_uuid: SITE }, bad), null);
});

test("cards: valid, unique, at most three on the home page", () => {
  const site = {
    site_uuid: SITE,
    articles: {
      tr: [
        card("a", { cover_url: `/v1/public/site/media/${SITE}/${MEDIA}` }),
        card("a"),
        card("../evil"),
        card("b", { title: "  " }),
        card("c", { published_at: "x" }),
        card("d"),
        card("e"),
      ],
    },
  };
  const home = homeArticleCards(site as never, "tr");
  assert.deepEqual(
    home.map((c) => c.href),
    ["/guide/tr/a", "/guide/tr/c", "/guide/tr/d"],
  );
  assert.equal(home[0].image, `/media/site/media/${SITE}/${MEDIA}`);
  assert.equal(home[1].publishedAt, null);
  assert.deepEqual(homeArticleCards(site as never, "en"), []);
  assert.equal(toArticleCards(site, null, "tr").length, 0);
});
