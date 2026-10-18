import assert from "node:assert/strict";
import { test } from "node:test";

import { homeFleetLimit, monogram } from "../themes/classic/helpers.ts";
import { guideCards } from "../themes/guide.ts";
import {
  THEME_SECTIONS,
  fleetColumns,
  resolveThemeId,
} from "../themes/registry.ts";
import { contrastRatio, readableAccent, textOn } from "./contrast.ts";
import { homeBlocks } from "./sections.ts";
import { DEFAULT_COLORS, templateColorVars, themeCss } from "./theme.ts";

const SITE = "11111111-2222-4333-8444-555555555555";
const MEDIA = "66666666-7777-4888-9999-aaaaaaaaaaaa";
const on = (...ids: string[]) => ids.map((id) => ({ id, enabled: true }));

test("classic resolves to its renderer", () => {
  assert.equal(resolveThemeId("classic"), "classic");
  assert.equal(resolveThemeId("Classic"), "technowide-design");
});

test("classic renders the studio's sections, branches included", () => {
  const CL = THEME_SECTIONS.classic;
  assert.deepEqual(
    [...CL],
    [
      "hero",
      "search",
      "fleet",
      "advantages",
      "branches",
      "about",
      "faq",
      "campaigns",
      "guide",
    ],
  );
  const blocks = homeBlocks(
    on(
      "hero",
      "search",
      "newsletter",
      "branches",
      "app_promo",
      "fleet",
      "campaigns",
      "advantages",
    ),
    CL,
    { campaigns: false },
  );
  assert.deepEqual(blocks[0], { id: "hero", withSearch: true });
  assert.deepEqual(
    blocks.map((b) => b.id),
    ["hero", "branches", "fleet", "advantages"],
  );
});

test("fleetColumns is three unless exactly two", () => {
  assert.equal(fleetColumns({ fleetColumns: "2" }), 2);
  assert.equal(fleetColumns({ fleetColumns: "3" }), 3);
  const bad: (Record<string, string> | null | undefined)[] = [
    { fleetColumns: "4" },
    { fleetColumns: " 2" },
    { heroAlign: "center" },
    {},
    null,
    undefined,
  ];
  for (const settings of bad) assert.equal(fleetColumns(settings), 3);
  assert.equal(homeFleetLimit(2), 4);
  assert.equal(homeFleetLimit(3), 6);
});

test("classic monogram", () => {
  assert.equal(monogram("Ides Rent", "tr"), "ID");
  assert.equal(monogram("ışık oto", "tr"), "IŞ");
  assert.equal(monogram("  9-to-5 Cars", "en"), "9T");
  assert.equal(monogram("—", "tr"), "R");
});

test("contrast helpers", () => {
  assert.equal(Math.round(contrastRatio("#000000", "#ffffff")), 21);
  assert.equal(textOn("#f59e0b"), "#111827");
  assert.equal(textOn("#1e3a8a"), "#ffffff");
  assert.equal(readableAccent("#f59e0b", "#ffffff", "#0f172a"), "#0f172a");
  assert.equal(readableAccent("#b45309", "#ffffff", "#0f172a"), "#b45309");
  assert.equal(readableAccent("red", "#ffffff", "#0f172a"), "#0f172a");
});

test("classic derives its accent text color and soft corners", () => {
  assert.equal(
    templateColorVars("classic", { ...DEFAULT_COLORS, accent: "#f97316" }),
    "--cl-accent-fg:#111827;",
  );
  assert.equal(templateColorVars("technowide-design", DEFAULT_COLORS), "");
  const css = themeCss(
    {
      colors: { ...DEFAULT_COLORS, accent: "#1e3a8a" },
      radius: "soft",
      mode: "light",
    } as never,
    "classic",
  );
  assert.match(css, /--cl-accent-fg:#ffffff;/);
  assert.match(css, /--radius-card:10px;/);
  assert.match(
    themeCss({ radius: "sharp" } as never, "classic"),
    /--radius-card:0px;/,
  );
  // Dark mode derives its own value from the dark tokens.
  const dark = themeCss(
    {
      colors: DEFAULT_COLORS,
      dark_colors: { ...DEFAULT_COLORS, accent: "#fbbf24" },
      mode: "system",
    } as never,
    "classic",
  );
  assert.equal(dark.match(/--cl-accent-fg/g)?.length, 2);
});

test("guide cards keep well-formed published cards only", () => {
  const site = {
    site_uuid: SITE,
    articles: {
      tr: [
        {
          title: " İlk yazı ",
          summary: "Özet",
          slug: "ilk-yazi",
          cover_url: `/v1/public/site/media/${SITE}/${MEDIA}`,
          published_at: "2026-10-01T09:00:00Z",
        },
        { title: "Kötü", slug: "../evil" },
        { title: "", slug: "bos" },
        { title: "Tekrar", slug: "ilk-yazi" },
        {
          title: "Yabancı görsel",
          slug: "yabanci",
          cover_url: "https://evil.test/x.png",
          published_at: "nope",
        },
        { title: "Dördüncü", slug: "dorduncu" },
        { title: "Beşinci", slug: "besinci" },
      ],
    },
  };
  const cards = guideCards(site, "tr");
  assert.deepEqual(cards, [
    {
      title: "İlk yazı",
      summary: "Özet",
      href: "/guide/tr/ilk-yazi",
      image: `/media/site/media/${SITE}/${MEDIA}`,
      date: "2026-10-01",
    },
    {
      title: "Yabancı görsel",
      summary: "",
      href: "/guide/tr/yabanci",
      image: null,
      date: null,
    },
    {
      title: "Dördüncü",
      summary: "",
      href: "/guide/tr/dorduncu",
      image: null,
      date: null,
    },
  ]);
  assert.deepEqual(guideCards(site, "en"), []);
  assert.deepEqual(guideCards({ site_uuid: SITE }, "tr"), []);
  assert.deepEqual(guideCards({ site_uuid: SITE, articles: [] }, "tr"), []);
});
