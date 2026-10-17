import assert from "node:assert/strict";
import { test } from "node:test";

import {
  frameHeaders,
  previewCookieMaxAge,
  previewCookieOptions,
  previewToken,
  safeOrigin,
} from "./preview.ts";

const SITE = "0b6f8a52-6b9e-4c1a-9a43-3f0d6c1b2a10";
const MAC = "Zm9vYmFyYmF6cXV4cXV1eGZvb2JhcmJhenF1eHF1dXg";
const exp = (secondsFromNow: number, now: number) =>
  Math.floor(now / 1000) + secondsFromNow;

test("preview tokens are forwarded only in the API's shape", () => {
  const now = 1_800_000_000_000;
  const good = `sp1.${SITE}.${exp(600, now)}.${MAC}`;
  assert.equal(previewToken(good), good);
  assert.equal(previewToken(`  ${good} `), good);
  for (const bad of [
    "",
    null,
    undefined,
    "sp2." + good.slice(4),
    `sp1.not-a-uuid.${exp(600, now)}.${MAC}`,
    `sp1.${SITE}.soon.${MAC}`,
    `sp1.${SITE}.${exp(600, now)}.${MAC}<script>`,
    `sp1.${SITE}.${exp(600, now)}.${MAC};Path=/`,
    `${good}.extra`,
  ]) {
    assert.equal(previewToken(bad), "", String(bad));
  }
});

test("the preview cookie lives until the token expires, at most 30 minutes", () => {
  const now = 1_800_000_000_000;
  assert.equal(
    previewCookieMaxAge(`sp1.${SITE}.${exp(600, now)}.${MAC}`, now),
    600,
  );
  assert.equal(
    previewCookieMaxAge(`sp1.${SITE}.${exp(86400, now)}.${MAC}`, now),
    1800,
  );
  assert.equal(
    previewCookieMaxAge(`sp1.${SITE}.${exp(-5, now)}.${MAC}`, now),
    0,
  );
  assert.equal(previewCookieMaxAge("garbage", now), 0);
  const opts = previewCookieOptions(`sp1.${SITE}.${exp(600, now)}.${MAC}`, now);
  // Sent inside the panel's cross-site frame, never to scripts.
  assert.equal(opts.httpOnly, true);
  assert.equal(opts.secure, true);
  assert.equal(opts.sameSite, "none");
  assert.equal(opts.partitioned, true);
});

test("pages deny framing unless they are a preview for the panel", () => {
  const deny = frameHeaders(null, true);
  assert.match(deny["Content-Security-Policy"], /frame-ancestors 'none'/);
  assert.equal(deny["X-Frame-Options"], "DENY");
  assert.doesNotMatch(deny["Content-Security-Policy"], /localhost/);

  const preview = frameHeaders(
    { panelOrigin: "https://app.rentopia.app/r/acme" },
    true,
  );
  assert.match(
    preview["Content-Security-Policy"],
    /^frame-ancestors https:\/\/app\.rentopia\.app; /,
  );
  assert.equal(preview["X-Frame-Options"], undefined);

  // A preview without a usable panel origin is not framable at all.
  for (const panelOrigin of [
    "",
    "javascript:alert(1)",
    "*",
    "https://user:pw@evil.test",
  ]) {
    const h = frameHeaders({ panelOrigin }, true);
    assert.match(h["Content-Security-Policy"], /frame-ancestors 'none'/);
    assert.equal(h["X-Frame-Options"], "DENY");
  }
  // Development allows the local pay page redirect.
  assert.match(
    frameHeaders(null, false)["Content-Security-Policy"],
    /http:\/\/localhost:\*/,
  );
});

test("only http(s) origins reach frame-ancestors", () => {
  assert.equal(safeOrigin("http://localhost:3000/x"), "http://localhost:3000");
  assert.equal(safeOrigin("https://app.test"), "https://app.test");
  assert.equal(safeOrigin("data:text/html,x"), "");
  assert.equal(safeOrigin("not a url"), "");
});
