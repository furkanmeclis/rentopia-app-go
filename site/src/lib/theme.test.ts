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
