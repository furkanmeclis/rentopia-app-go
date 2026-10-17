import assert from "node:assert/strict";
import { test } from "node:test";

import {
  mailHref,
  menuLinks,
  safeHref,
  socialLinks,
  telHref,
  whatsappHref,
} from "./links.ts";
import { siteMediaUrl } from "./media.ts";

test("hrefs: same-site paths and https only", () => {
  assert.equal(safeHref("/vehicles?segment=SUV"), "/vehicles?segment=SUV");
  assert.equal(safeHref("https://example.com/a"), "https://example.com/a");
  for (const bad of [
    "javascript:alert(1)",
    "JAVASCRIPT:alert(1)",
    "data:text/html,x",
    "//evil.com",
    "/%2F/evil.com",
    "/\\evil.com",
    "http://example.com",
    "https://user:pw@example.com",
    "https://example.com:8443/",
    "/a b",
    "/a\nb",
    "vbscript:x",
    "",
    "relative",
  ]) {
    assert.equal(safeHref(bad), null, bad);
  }
});

test("menus: localized labels, unsafe items dropped, fallback when empty", () => {
  const fallback = [{ href: "/branches", label: "Şubeler", external: false }];
  const links = menuLinks(
    [
      { label: { tr: "Filo", en: "Fleet" }, href: "/vehicles", kind: "page" },
      { label: { tr: "Kötü" }, href: "javascript:alert(1)", kind: "link" },
      { label: { tr: "", en: "" }, href: "/faq", kind: "page" },
      { label: { tr: "Blog" }, href: "https://blog.example.com", kind: "link" },
    ],
    "en",
    fallback,
  );
  assert.deepEqual(links, [
    { href: "/vehicles", label: "Fleet", external: false },
    { href: "https://blog.example.com/", label: "Blog", external: true },
  ]);
  assert.deepEqual(menuLinks([], "tr", fallback), fallback);
  assert.deepEqual(menuLinks(undefined, "tr", fallback), fallback);
  assert.deepEqual(
    menuLinks(
      [{ label: { tr: "x" }, href: "data:x", kind: "link" }],
      "tr",
      fallback,
    ),
    fallback,
  );
  const many = Array.from({ length: 20 }, (_, i) => ({
    label: { tr: `L${i}` },
    href: `/p${i}`,
    kind: "page" as const,
  }));
  assert.equal(menuLinks(many, "tr", []).length, 12);
});

test("socials: https on the network's own host only", () => {
  const links = socialLinks({
    instagram: "https://www.instagram.com/acme",
    facebook: "https://facebook.com.evil.com/acme",
    x: "http://x.com/acme",
    youtube: "https://youtu.be/abc",
    linkedin: "javascript:alert(1)",
    tiktok: "https://notiktok.com/@acme",
    whatsapp: "https://wa.me/905551112233",
    myspace: "https://myspace.com/acme",
  });
  assert.deepEqual(
    links.map((l) => [l.network, l.href]),
    [
      ["instagram", "https://www.instagram.com/acme"],
      ["youtube", "https://youtu.be/abc"],
      ["whatsapp", "https://wa.me/905551112233"],
    ],
  );
  assert.deepEqual(socialLinks(undefined), []);
});

test("contact links", () => {
  assert.equal(telHref("+90 (555) 111 22 33"), "tel:+905551112233");
  assert.equal(telHref("javascript:"), null);
  assert.equal(whatsappHref("+90 555 111 22 33"), "https://wa.me/905551112233");
  assert.equal(whatsappHref("12"), null);
  assert.equal(mailHref("info@acme.com"), "mailto:info@acme.com");
  assert.equal(mailHref("a@b.c?subject=x<script>"), null);
  assert.equal(mailHref("nope"), null);
});

test("library media: only this site's mapped UUIDs, preview via draft=1", () => {
  const site = "11111111-1111-4111-8111-111111111111";
  const media = "22222222-2222-4222-8222-222222222222";
  const other = "33333333-3333-4333-8333-333333333333";
  const s = {
    site_uuid: site,
    media: {
      [media]: `/v1/public/site/media/${site}/${media}`,
      [other]: `/v1/public/site/media/${other}/${other}`,
    },
  };
  assert.equal(siteMediaUrl(s, media), `/media/site/media/${site}/${media}`);
  assert.equal(siteMediaUrl(s, other), null);
  assert.equal(siteMediaUrl(s, "missing"), null);
  assert.equal(siteMediaUrl(s, ""), null);
  const preview = {
    site_uuid: site,
    media: {
      [media]: `/v1/public/site/media/${site}/${media}?preview=sp1.x.1.abc`,
    },
  };
  assert.equal(
    siteMediaUrl(preview, media),
    `/media/site/media/${site}/${media}?draft=1`,
  );
});
