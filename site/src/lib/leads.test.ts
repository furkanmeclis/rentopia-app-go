import assert from "node:assert/strict";
import { test } from "node:test";

import {
  apiLeadErrors,
  decodeLeadFlash,
  encodeLeadFlash,
  isLeadKind,
  leadFlags,
  leadRedirect,
  leadStatusOf,
  readLeadForm,
  readLeadResult,
} from "./leads.ts";

const form = (entries: Record<string, string>) => (name: string) =>
  entries[name] ?? null;

const site = (
  leads?: Partial<Record<string, boolean>>,
  booking_enabled = true,
) => ({
  booking_enabled,
  publishable_key: booking_enabled ? "rtp_pk_x" : "",
  config: { leads: leads as never },
});

test("lead flags default on and need a usable key", () => {
  assert.deepEqual(leadFlags(site()), {
    newsletter: true,
    contact: true,
    corporate: true,
    franchise: true,
  });
  assert.deepEqual(
    leadFlags(
      site({
        contact: true,
        corporate: false,
        franchise: true,
        newsletter: false,
      }),
    ),
    { newsletter: false, contact: true, corporate: false, franchise: true },
  );
  assert.equal(
    Object.values(leadFlags(site(undefined, false))).some(Boolean),
    false,
  );
});

test("contact form: required fields, trimmed values, consent", () => {
  const { errors, input } = readLeadForm(
    "contact",
    form({ name: "  Ayşe  ", email: "", message: "\r\n" }),
    "tr",
  );
  assert.deepEqual(errors, {
    email: "required",
    message: "required",
    consent_kvkk: "required",
  });
  assert.equal(input.name, "Ayşe");

  const ok = readLeadForm(
    "contact",
    form({
      name: "Ayşe",
      email: "AYSE@Example.com ",
      message: "Merhaba\r\nnasılsınız",
      consent_kvkk: "on",
      website: "",
      // Fields the form does not show are ignored.
      company: "Acme",
    }),
    "en",
  );
  assert.deepEqual(ok.errors, {});
  assert.equal(ok.input.email, "ayse@example.com");
  assert.equal(ok.input.message, "Merhaba\nnasılsınız");
  assert.equal(ok.input.company, "");
  assert.equal(ok.input.locale, "en");
  assert.equal(ok.input.consent_kvkk, true);
  assert.equal(ok.input.consent_marketing, false);
  assert.equal(ok.input.fleet_size, null);
});

test("invalid e-mail, phone, length and fleet size", () => {
  const { errors, input } = readLeadForm(
    "corporate",
    form({
      company: "x".repeat(201),
      name: "Ali",
      email: "ali@",
      phone: "call me",
      fleet_size: "12.5",
      consent_kvkk: "on",
    }),
    "tr",
  );
  assert.deepEqual(errors, {
    company: "too_long",
    email: "invalid",
    phone: "invalid",
    fleet_size: "invalid",
  });
  assert.equal(input.fleet_size, null);
  const fleet = readLeadForm(
    "corporate",
    form({
      company: "Acme",
      name: "Ali",
      email: "ali@acme.com",
      phone: "+90 (212) 555 00 00",
      fleet_size: "40",
      consent_kvkk: "on",
    }),
    "tr",
  );
  assert.deepEqual(fleet.errors, {});
  assert.equal(fleet.input.fleet_size, 40);
  assert.deepEqual(
    readLeadForm(
      "corporate",
      form({
        company: "A",
        name: "B",
        email: "b@a.co",
        phone: "5555555",
        fleet_size: "100001",
        consent_kvkk: "on",
      }),
      "tr",
    ).errors,
    { fleet_size: "invalid" },
  );
});

test("newsletter needs both consents; honeypot is passed through", () => {
  assert.deepEqual(
    readLeadForm(
      "newsletter",
      form({ email: "a@b.co", consent_kvkk: "on" }),
      "tr",
    ).errors,
    { consent_marketing: "required" },
  );
  const { errors, input } = readLeadForm(
    "newsletter",
    form({
      email: "a@b.co",
      consent_kvkk: "on",
      consent_marketing: "on",
      website: "spam.example",
    }),
    "tr",
  );
  assert.deepEqual(errors, {});
  assert.equal(input.consent_marketing, true);
  assert.equal(input.honeypot, "spam.example");
});

test("API details map onto known fields only", () => {
  assert.deepEqual(
    apiLeadErrors([
      { field: "email", code: "invalid" },
      { field: "name", code: "required" },
      { field: "message", code: "too_long" },
      { field: "kind", code: "invalid" },
      { field: "<b>", code: "x" },
      {},
    ]),
    { email: "invalid", name: "required", message: "too_long" },
  );
});

test("API failures are never reported as sent", () => {
  assert.equal(leadStatusOf(429, "RATE_LIMITED"), "limited");
  assert.equal(leadStatusOf(400, "VALIDATION_ERROR"), "invalid");
  assert.equal(leadStatusOf(404, "NOT_FOUND"), "closed");
  assert.equal(leadStatusOf(403, "BOOKING_DISABLED"), "closed");
  assert.equal(leadStatusOf(403, "PREVIEW_READ_ONLY"), "preview");
  assert.equal(leadStatusOf(503, "UPSTREAM_UNAVAILABLE"), "failed");
  assert.equal(leadStatusOf(401, "EXPIRED"), "failed");
});

test("redirect query round-trips without personal data", () => {
  const url = leadRedirect("franchise", {
    status: "invalid",
    errors: { city: "required", email: "invalid" },
  });
  assert.equal(
    url,
    "/franchise?lead=invalid&lead_kind=franchise&fe=city.required%2Cemail.invalid#franchise-form",
  );
  const params = Object.fromEntries(
    new URL(url, "https://x.test").searchParams,
  );
  assert.deepEqual(readLeadResult(params, "franchise"), {
    status: "invalid",
    errors: { city: "required", email: "invalid" },
  });
  // Another form's result, unknown statuses and junk fields are ignored.
  assert.equal(readLeadResult(params, "contact"), null);
  assert.equal(
    readLeadResult({ lead: "hacked", lead_kind: "contact" }, "contact"),
    null,
  );
  assert.deepEqual(
    readLeadResult(
      { lead: "sent", lead_kind: "newsletter", fe: "x.y,<script>.required" },
      "newsletter",
    ),
    { status: "sent", errors: {} },
  );
  assert.equal(
    leadRedirect("newsletter", { status: "sent", errors: {} }),
    "/?lead=sent&lead_kind=newsletter#newsletter-form",
  );
});

test("flash values keep the form's fields and stay small", () => {
  const raw = encodeLeadFlash("contact", {
    name: "Zeynep",
    email: "z@x.co",
    message: "ğ".repeat(5000),
    company: "not shown",
  });
  assert.ok(raw.length <= 3600);
  const back = decodeLeadFlash(raw, "contact");
  assert.equal(back.name, "Zeynep");
  assert.equal(back.email, "z@x.co");
  assert.equal(back.company, undefined);
  assert.ok(back.message?.startsWith("ğğğ"));
  // Worst case (three-byte characters everywhere) still fits.
  const wide = encodeLeadFlash("contact", {
    name: "€".repeat(160),
    email: "e".repeat(254),
    phone: "1".repeat(40),
    message: "€".repeat(5000),
  });
  assert.ok(wide.length > 0 && wide.length <= 3600);
  assert.deepEqual(decodeLeadFlash(raw, "franchise"), {});
  assert.deepEqual(decodeLeadFlash("%%%", "contact"), {});
  assert.deepEqual(decodeLeadFlash(undefined, "contact"), {});
});

test("lead kinds", () => {
  assert.equal(isLeadKind("contact"), true);
  assert.equal(isLeadKind("CONTACT"), false);
  assert.equal(isLeadKind(undefined), false);
});
