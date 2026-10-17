import assert from "node:assert/strict";
import test from "node:test";

import {
  EMBED_TTL,
  embedPath,
  openEmbed,
  sealEmbed,
  type EmbedState,
} from "./embed.ts";
import { allowedParent, embedFrameHeaders, widgetOrigin } from "./widget.ts";

const secret = "test-secret-32-characters-minimum-for-embed";
const now = 1_800_000_000_000;
const state: EmbedState = {
  origin: "https://acme.example.com",
  expires: now / 1000 + EMBED_TTL,
  session: {
    o: "https://acme.example.com",
    t: "rtp_ps_hidden-secret",
    e: now / 1000 + EMBED_TTL,
  },
  reservation: { reference: "R-123", token: "private-reservation-token" },
};

test("URL references encrypt tokens, authenticate data, expire and bind to the site origin", () => {
  const ref = sealEmbed(state, secret);
  assert.deepEqual(openEmbed(ref, state.origin, secret, now), state);
  assert.equal(ref.includes(state.session.t), false);
  assert.equal(
    Buffer.from(ref.slice(3), "base64url").includes(
      Buffer.from(state.reservation!.token),
    ),
    false,
  );
  assert.notEqual(sealEmbed(state, secret), ref);
  for (const raw of [
    "",
    "garbage",
    ref.slice(0, -3) + "aaa",
    "e1." + "x".repeat(6000),
  ])
    assert.equal(openEmbed(raw, state.origin, secret, now), null);
  assert.equal(openEmbed(ref, "https://other.example.com", secret, now), null);
  assert.equal(openEmbed(ref, state.origin, secret + "other", now), null);
  assert.equal(
    openEmbed(ref, state.origin, secret, state.expires * 1000),
    null,
  );
  assert.equal(
    openEmbed(
      sealEmbed(
        {
          ...state,
          session: { ...state.session, o: "https://foreign.example.com" },
        },
        secret,
      ),
      state.origin,
      secret,
      now,
    ),
    null,
  );
  assert.throws(() => sealEmbed(state, ""));
});

test("framing denies by default and accepts only explicit normalized HTTPS origins", () => {
  assert.match(
    embedFrameHeaders([])["Content-Security-Policy"],
    /frame-ancestors 'none'/,
  );
  const headers = embedFrameHeaders([
    "Partner.com",
    "https://partner.com:443",
    "https://other.com:8443",
    "javascript:alert(1)",
    "https://*.example.com",
    "https://a.com/path",
  ]);
  assert.equal(headers["X-Frame-Options"], undefined);
  assert.match(
    headers["Content-Security-Policy"],
    /^frame-ancestors https:\/\/partner.com https:\/\/other.com:8443;/,
  );
  assert.equal(headers["Referrer-Policy"], "no-referrer");
  assert.equal(
    widgetOrigin("http://foo.localhost:3100"),
    "http://foo.localhost:3100",
  );
  assert.equal(widgetOrigin("http://partner.com"), "");
});

test("postMessage requires an allowed parent origin, including the exact scheme and port", () => {
  const domains = ["https://partner.com", "https://shop.com:8443"];
  assert.equal(
    allowedParent("https://partner.com", domains),
    "https://partner.com",
  );
  for (const origin of [
    null,
    "null",
    "https://partner.com.attacker.com",
    "https://partner.com:8443",
    "http://partner.com",
    "https://partner.com@attacker.com",
    "https://partner.com/path",
  ])
    assert.equal(allowedParent(origin, domains), "");
});

test("embed navigation preserves references, parent, widget and filters without overriding new references", () => {
  const embed = {
    fields: {
      s: "encrypted",
      parent: "https://partner.com",
      widgetId: "w1",
      locale: "en",
      subgroup: "vehicle",
    },
  };
  const path = new URL(
    embedPath("/book/id/details?changed=1", embed),
    state.origin,
  );
  assert.equal(path.pathname, "/embed/book/id/details");
  for (const [key, value] of Object.entries(embed.fields))
    assert.equal(path.searchParams.get(key), value);
  assert.equal(
    new URL(
      embedPath("/reservation/R?s=new", embed),
      state.origin,
    ).searchParams.get("s"),
    "new",
  );
  assert.match(embedPath("/", embed), /^\/embed\?/);
  assert.equal(embedPath("/search", null), "/search");
  assert.equal(
    embedPath("https://pay.example.com/pay/1", embed),
    "https://pay.example.com/pay/1",
  );
});
