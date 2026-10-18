import assert from "node:assert/strict";
import { test } from "node:test";

import { safeNext } from "./request.ts";
import { hostKey } from "./resolve.ts";
import { decodeSession, encodeSession, usableSession } from "./session.ts";
import { safeGa4Id, themeCss } from "./theme.ts";

test("theme css only carries validated tokens", () => {
  const css = themeCss({
    colors: {
      primary: "#FF0000",
      primary_foreground: "red;}body{display:none",
      accent: "url(javascript:alert(1))",
      background: "#ffffff",
      foreground: "#000000",
      muted: "#eeeeee",
      muted_foreground: "#333333",
      border: "</style><script>",
    },
    dark_colors: {} as never,
    font: "comic; }",
    radius: "lg",
    mode: "light",
  });
  assert.match(css, /--primary:#ff0000;/);
  assert.match(css, /--primary-foreground:#ffffff;/);
  assert.match(css, /--radius:0\.875rem;/);
  assert.match(css, /--font-brand:var\(--font-inter\)/);
  assert.doesNotMatch(css, /javascript|script|display:none|comic/);
});

test("system mode adds the dark scheme", () => {
  const css = themeCss({ mode: "system" } as never);
  assert.match(css, /prefers-color-scheme: dark/);
});

test("ga4 ids", () => {
  assert.equal(safeGa4Id("G-ABC1234"), "G-ABC1234");
  assert.equal(safeGa4Id("G-1');alert(1)//"), null);
  assert.equal(safeGa4Id(""), null);
});

test("redirect targets stay on the site", () => {
  assert.equal(safeNext("/search?x=1"), "/search?x=1");
  for (const bad of [
    "//evil.com",
    "https://evil.com",
    "/\\evil.com",
    "",
    "evil",
  ]) {
    assert.equal(safeNext(bad), "/");
  }
});

test("host keys", () => {
  assert.equal(hostKey("Acme.Rentopia.App:443"), "acme.rentopia.app:443");
  assert.equal(hostKey("evil.com/x"), "");
  assert.equal(hostKey("a b"), "");
});

test("session cookies are bound to their origin", () => {
  const s = {
    o: "https://a.example",
    t: "rtp_ps_x",
    e: Math.floor(Date.now() / 1000) + 600,
  };
  const back = decodeSession(encodeSession(s));
  assert.deepEqual(back, s);
  assert.equal(usableSession(back, { origin: "https://a.example" }), true);
  assert.equal(usableSession(back, { origin: "https://b.example" }), false);
  assert.equal(
    usableSession(
      { ...s, e: Math.floor(Date.now() / 1000) + 30 },
      { origin: s.o },
    ),
    false,
  );
  assert.equal(decodeSession("not-json"), null);
  assert.equal(
    decodeSession(encodeSession({ ...s, t: "rtp_sk_secret" })),
    null,
  );
});

test("corner styles map to card / control / image radii", () => {
  const vars = (radius: string) => themeCss({ radius } as never);
  assert.match(
    vars("sharp"),
    /--radius-card:0px;--radius-control:0px;--radius-image:0px;/,
  );
  assert.match(vars("none"), /--radius-card:0px;/);
  assert.match(vars("xs"), /--radius-card:4px;/);
  assert.match(vars("sm"), /--radius-card:8px;/);
  assert.match(vars("md"), /--radius-card:12px;--radius-control:12px;/);
  assert.match(vars("lg"), /--radius-card:16px;/);
  assert.match(vars("xl"), /--radius-card:24px;/);
  assert.match(
    vars("pill"),
    /--radius-card:32px;--radius-control:9999px;--radius-image:32px;/,
  );
  assert.match(vars("full"), /--radius-control:9999px;/);
  // soft (and unknown values) keep the theme's own corners: no variables.
  for (const own of ["soft", "bogus", ""]) {
    assert.doesNotMatch(
      vars(own),
      /--radius-card|--radius-control|--radius-image/,
    );
  }
});

test("Horizon's soft corners are its own square look", () => {
  const vars = (radius: string, template: string) =>
    themeCss({ radius } as never, template);
  assert.match(
    vars("soft", "horizon-design"),
    /--radius-card:0px;--radius-control:0px;--radius-image:2px;/,
  );
  // A chosen corner style wins over the template's own.
  assert.match(vars("lg", "horizon-design"), /--radius-card:16px;/);
  assert.doesNotMatch(vars("soft", "technowide-design"), /--radius-card/);
});

test("heading font follows the body, a chosen font or Georgia", () => {
  const css = (font: string, heading?: string, template = "") =>
    themeCss({ font, heading_font: heading } as never, template);
  assert.match(
    css("manrope"),
    /--font-brand:var\(--font-manrope\), system-ui, sans-serif;/,
  );
  assert.match(css("manrope", "theme"), /--font-heading:var\(--font-manrope\)/);
  assert.match(css("manrope", "sora"), /--font-heading:var\(--font-sora\)/);
  assert.match(
    css("inter", "lora"),
    /--font-heading:var\(--font-lora\), Georgia, serif;/,
  );
  assert.match(
    css("inter", "georgia"),
    /--font-heading:Georgia, "Times New Roman", serif;/,
  );
  assert.match(
    css("inter", "theme", "horizon-design"),
    /--font-heading:Georgia/,
  );
  assert.match(css("wix_madefor"), /--font-brand:var\(--font-wix-madefor\)/);
  assert.match(
    css("dm_sans", "plus_jakarta_sans"),
    /--font-heading:var\(--font-plus-jakarta-sans\)/,
  );
  // Unknown ids never reach the stylesheet.
  const bad = css("x;}body{", "</style>");
  assert.match(bad, /--font-brand:var\(--font-inter\)/);
  assert.match(bad, /--font-heading:var\(--font-inter\)/);
  assert.doesNotMatch(bad, /style|body\{/);
});

test("dark mode uses the dark tokens", () => {
  const css = themeCss({
    mode: "dark",
    dark_colors: { primary: "#123456" },
  } as never);
  assert.match(css, /--primary:#123456;/);
  assert.match(css, /color-scheme:dark/);
});
