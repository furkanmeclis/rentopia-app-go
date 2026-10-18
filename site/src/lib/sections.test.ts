import assert from "node:assert/strict";
import { test } from "node:test";

import {
  DEFAULT_THEME,
  THEME_SECTIONS,
  heroAlign,
  resolveThemeId,
} from "../themes/registry.ts";
import { homeBlocks, sectionRenders } from "./sections.ts";

const TW = THEME_SECTIONS["technowide-design"];
const HZ = THEME_SECTIONS["horizon-design"];
const on = (...ids: string[]) => ids.map((id) => ({ id, enabled: true }));

test("theme registry falls back to Technowide", () => {
  assert.equal(resolveThemeId("technowide-design"), "technowide-design");
  assert.equal(resolveThemeId("horizon-design"), "horizon-design");
  for (const unknown of [
    "Horizon-Design",
    "horizon",
    "classic",
    "aurum",
    "",
    null,
    undefined,
    "../evil",
  ]) {
    assert.equal(resolveThemeId(unknown), DEFAULT_THEME);
  }
});

test("sections keep the API order and hero leads", () => {
  const blocks = homeBlocks(on("hero", "faq", "fleet", "about"), TW);
  assert.deepEqual(
    blocks.map((b) => b.id),
    ["hero", "faq", "fleet", "about"],
  );
  assert.deepEqual(
    homeBlocks(on("about", "hero"), TW).map((b) => b.id),
    ["hero", "about"],
  );
});

test("search right after the hero is drawn inside it", () => {
  const inHero = homeBlocks(on("hero", "search", "fleet"), TW);
  assert.deepEqual(inHero[0], { id: "hero", withSearch: true });
  assert.deepEqual(
    inHero.map((b) => b.id),
    ["hero", "fleet"],
  );
  const apart = homeBlocks(on("hero", "fleet", "search"), TW);
  assert.deepEqual(apart[0], { id: "hero", withSearch: false });
  assert.deepEqual(
    apart.map((b) => b.id),
    ["hero", "fleet", "search"],
  );
});

test("sections without a data service or disabled never render", () => {
  const blocks = homeBlocks(
    [...on("hero", "app_promo"), { id: "faq", enabled: false }, ...on("about")],
    TW,
  );
  assert.deepEqual(
    blocks.map((b) => b.id),
    ["hero", "about"],
  );
  assert.equal(sectionRenders("app_promo"), false);
  for (const id of ["fleet", "campaigns", "newsletter", "guide"])
    assert.equal(sectionRenders(id), true);
});

test("the guide renders while an article is published (TEC-785)", () => {
  const list = on("hero", "fleet", "guide", "about");
  assert.deepEqual(
    homeBlocks(list, TW, { guide: true }).map((b) => b.id),
    ["hero", "fleet", "guide", "about"],
  );
  assert.deepEqual(
    homeBlocks(list, TW, { guide: false }).map((b) => b.id),
    ["hero", "fleet", "about"],
  );
});

test("campaigns and newsletter render when their data is ready", () => {
  const list = on("hero", "campaigns", "fleet", "newsletter", "about");
  assert.deepEqual(
    homeBlocks(list, TW).map((b) => b.id),
    ["hero", "campaigns", "fleet", "newsletter", "about"],
  );
  assert.deepEqual(
    homeBlocks(list, TW, { campaigns: false, newsletter: false }).map(
      (b) => b.id,
    ),
    ["hero", "fleet", "about"],
  );
  // A dropped section does not keep the search out of the hero.
  const withSearch = homeBlocks(on("hero", "campaigns", "search"), TW, {
    campaigns: false,
  });
  assert.deepEqual(withSearch[0], { id: "hero", withSearch: true });
  assert.equal(withSearch.length, 1);
});

test("unknown, unsupported and duplicate sections are dropped", () => {
  const blocks = homeBlocks(
    on("hero", "fleet", "fleet", "<script>", "journal", "about"),
    ["hero", "fleet", "about"],
  );
  assert.deepEqual(
    blocks.map((b) => b.id),
    ["hero", "fleet", "about"],
  );
  // A theme that does not support a section ignores it.
  assert.deepEqual(
    homeBlocks(on("hero", "branches"), ["hero"]).map((b) => b.id),
    ["hero"],
  );
});

test("answers without sections get the legacy defaults", () => {
  assert.deepEqual(
    homeBlocks(undefined, TW).map((b) => b.id),
    ["hero", "fleet", "advantages", "about", "faq"],
  );
  assert.equal(homeBlocks(undefined, TW)[0].withSearch, true);
  assert.deepEqual(
    homeBlocks([], TW).map((b) => b.id),
    ["hero"],
  );
});

test("Horizon renders the studio's sections only", () => {
  assert.deepEqual(
    [...HZ],
    ["hero", "search", "campaigns", "fleet", "guide", "about", "faq"],
  );
  const blocks = homeBlocks(
    on(
      "hero",
      "advantages",
      "newsletter",
      "fleet",
      "app_promo",
      "branches",
      "guide",
      "campaigns",
      "about",
      "faq",
    ),
    HZ,
    { campaigns: true },
  );
  assert.deepEqual(
    blocks.map((b) => b.id),
    ["hero", "fleet", "campaigns", "about", "faq"],
  );
});

test("Horizon keeps the order, folds search into the hero and drops empty campaigns", () => {
  const blocks = homeBlocks(
    on("hero", "search", "faq", "campaigns", "fleet"),
    HZ,
    {
      campaigns: false,
    },
  );
  assert.deepEqual(blocks[0], { id: "hero", withSearch: true });
  assert.deepEqual(
    blocks.map((b) => b.id),
    ["hero", "faq", "fleet"],
  );
  // Legacy answers without sections get Horizon's supported defaults.
  assert.deepEqual(
    homeBlocks(undefined, HZ).map((b) => b.id),
    ["hero", "fleet", "about", "faq"],
  );
});

test("Horizon heroAlign is left unless exactly center", () => {
  assert.equal(heroAlign({ heroAlign: "center" }), "center");
  assert.equal(heroAlign({ heroAlign: "left" }), "left");
  const bad: (Record<string, string> | null | undefined)[] = [
    { heroAlign: "right" },
    { heroAlign: "CENTER" },
    { heroLayout: "split" },
    {},
    null,
    undefined,
  ];
  for (const settings of bad) {
    assert.equal(heroAlign(settings), "left");
  }
});
