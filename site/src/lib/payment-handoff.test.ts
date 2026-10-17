import assert from "node:assert/strict";
import test from "node:test";

import { validateWidgetReturn } from "../../../frontend/src/lib/widget-payment-return.ts";
import { paymentHandoff } from "./payment-handoff.ts";

const secret = "a-shared-secret-at-least-32-characters";
const now = 1_800_000_000_000;
const claims = {
  token: "payment-token",
  url: "https://cars.partner.com/reservation/R-123?s=encrypted",
  organization: "org-123",
  expires: now / 1000 + 1800,
};
const site = {
  origin: "https://cars.partner.com",
  organization: { uuid: "org-123" },
};

test("payment returns require a signed destination belonging to the resolved organization", async () => {
  const raw = paymentHandoff(claims, secret);
  const resolve = async (host: string) =>
    host === "cars.partner.com" ? site : null;
  assert.equal(
    await validateWidgetReturn(raw, claims.token, resolve, secret, now),
    claims.url,
  );
  assert.equal(
    await validateWidgetReturn(raw, "other-token", resolve, secret, now),
    null,
  );
  assert.equal(
    await validateWidgetReturn(
      raw,
      claims.token,
      resolve,
      secret,
      claims.expires * 1000,
    ),
    null,
  );
  assert.equal(
    await validateWidgetReturn(
      raw,
      claims.token,
      resolve,
      secret + "other",
      now,
    ),
    null,
  );
  assert.equal(
    await validateWidgetReturn(raw + "a", claims.token, resolve, secret, now),
    null,
  );
  assert.equal(
    await validateWidgetReturn(
      raw,
      claims.token,
      async () => null,
      secret,
      now,
    ),
    null,
  );
  assert.equal(
    await validateWidgetReturn(
      raw,
      claims.token,
      async () => ({ ...site, organization: { uuid: "other-org" } }),
      secret,
      now,
    ),
    null,
  );
  assert.equal(
    await validateWidgetReturn(
      raw,
      claims.token,
      async () => ({ ...site, origin: "https://other.com" }),
      secret,
      now,
    ),
    null,
  );
  for (const url of [
    "https://attacker.com/reservation/R-123",
    "javascript:alert(1)",
    "https://cars.partner.com/search",
    "https://user:pass@cars.partner.com/reservation/R-123",
  ]) {
    assert.equal(
      await validateWidgetReturn(
        paymentHandoff({ ...claims, url }, secret),
        claims.token,
        resolve,
        secret,
        now,
      ),
      null,
    );
  }
});
