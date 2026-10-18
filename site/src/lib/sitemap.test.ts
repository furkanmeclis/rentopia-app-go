import assert from "node:assert/strict";
import { test } from "node:test";

import { buildSitemapXml, sitemapOrigin, sitemapPath } from "./sitemap.ts";

test("sitemap lists absolute URLs on the site origin", () => {
  const xml = buildSitemapXml("https://acme.example.com", [
    { path: "/", lastmod: "2026-10-18T09:00:00Z" },
    {
      path: "/guide/tr/istanbul-rehberi",
      locale: "tr",
      lastmod: "2026-10-17T12:30:00+03:00",
    },
    { path: "/vehicles?segment=SUV&x=<1>", lastmod: "nope" },
  ]);
  assert.equal(
    xml,
    [
      '<?xml version="1.0" encoding="UTF-8"?>',
      '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
      "<url><loc>https://acme.example.com/</loc><lastmod>2026-10-18T09:00:00.000Z</lastmod></url>",
      "<url><loc>https://acme.example.com/guide/tr/istanbul-rehberi</loc><lastmod>2026-10-17T09:30:00.000Z</lastmod></url>",
      "<url><loc>https://acme.example.com/vehicles?segment=SUV&amp;x=&lt;1&gt;</loc></url>",
      "</urlset>",
      "",
    ].join("\n"),
  );
});

test("unsafe paths, duplicates and bad origins are skipped", () => {
  const xml = buildSitemapXml("https://acme.example.com/ignored/path", [
    { path: "https://evil.com/", lastmod: "" },
    { path: "//evil.com", lastmod: "" },
    { path: "/a b", lastmod: "" },
    { path: "/x\n", lastmod: "" },
    { path: "/ok", lastmod: "" },
    { path: "/ok", lastmod: "" },
  ]);
  assert.equal(xml.match(/<url>/g)?.length, 1);
  assert.match(xml, /<loc>https:\/\/acme\.example\.com\/ok<\/loc>/);
  assert.equal(sitemapOrigin("javascript:alert(1)"), null);
  assert.equal(sitemapOrigin("https://u:p@x.com"), null);
  assert.equal(
    sitemapOrigin("http://localhost:3100/x"),
    "http://localhost:3100",
  );
  assert.equal(sitemapPath(5), null);
  assert.equal(
    buildSitemapXml("not a url", [{ path: "/", lastmod: "" }]).includes(
      "<url>",
    ),
    false,
  );
  assert.equal(buildSitemapXml("https://a.com", null).includes("<url>"), false);
});
