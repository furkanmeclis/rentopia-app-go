import assert from "node:assert/strict";
import { test } from "node:test";

import {
  DEFAULT_THEME,
  THEME_SECTIONS,
  resolveThemeId,
} from "../themes/registry.ts";
import { homeBlocks, sectionRenders } from "./sections.ts";

const TW = THEME_SECTIONS["technowide-design"];
const on = (...ids: string[]) => ids.map((id) => ({ id, enabled: true }));

test("theme registry falls back to Technowide", () => {
  assert.equal(resolveThemeId("technowide-design"), "technowide-design");
  for (const unknown of [
    "horizon-design",
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
    [
      ...on("hero", "campaigns", "newsletter", "guide", "app_promo"),
      { id: "faq", enabled: false },
      ...on("about"),
    ],
    TW,
  );
  assert.deepEqual(
    blocks.map((b) => b.id),
    ["hero", "about"],
  );
  for (const id of ["campaigns", "guide", "newsletter", "app_promo"])
    assert.equal(sectionRenders(id), false);
  assert.equal(sectionRenders("fleet"), true);
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
