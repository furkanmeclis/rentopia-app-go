"use server";

import { randomUUID } from "node:crypto";

import { redirect } from "next/navigation";

import { ApiError } from "@/lib/api";
import { splitPlace } from "@/lib/catalog";
import {
  rememberReservation,
  reservationToken,
} from "@/lib/reservation-tokens";
import { bookingApi, getSite } from "@/lib/server";
import type { Booked, PaymentLink, Quote } from "@/lib/types";

const str = (f: FormData, k: string) => {
  const v = f.get(k);
  return typeof v === "string" ? v.trim() : "";
};

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Results → a quote for the chosen subgroup → the extras step. */
export async function selectVehicle(form: FormData) {
  const subgroup = str(form, "subgroup_uuid");
  const pickup = splitPlace(str(form, "pickup"));
  const ret = splitPlace(str(form, "return")) ?? pickup;
  const back = str(form, "back").startsWith("/search")
    ? str(form, "back")
    : "/";
  if (!UUID.test(subgroup) || !pickup || !ret) redirect(back);
  let quote: Quote;
  try {
    quote = await bookingApi<Quote>("/v1/public/quotes", {
      method: "POST",
      idempotencyKey: randomUUID(),
      body: {
        subgroup_uuid: subgroup,
        pickup_branch_uuid: pickup.branch,
        pickup_point_uuid: pickup.point ?? null,
        return_branch_uuid: ret.branch,
        return_point_uuid: ret.point ?? null,
        starts_at: str(form, "starts_at"),
        ends_at: str(form, "ends_at"),
      },
    });
  } catch (err) {
    const code = err instanceof ApiError ? err.code : "FAILED";
    redirect(
      `${back}${back.includes("?") ? "&" : "?"}error=${encodeURIComponent(code)}`,
    );
  }
  redirect(`/book/${quote.uuid}`);
}

/** Extras step: re-quote with the chosen extras (prices come from the API). */
export async function applyExtras(form: FormData) {
  const quoteId = str(form, "quote_uuid");
  if (!UUID.test(quoteId)) redirect("/");
  const base = await bookingApi<Quote>(`/v1/public/quotes/${quoteId}`);
  const extras = form
    .getAll("extra")
    .filter((v): v is string => typeof v === "string" && UUID.test(v))
    .map((id) => ({
      extra_uuid: id,
      quantity: Math.max(1, Math.min(10, Number(str(form, `qty_${id}`)) || 1)),
    }));
  if (extras.length === 0) redirect(`/book/${quoteId}/details`);
  let quote: Quote;
  try {
    quote = await bookingApi<Quote>("/v1/public/quotes", {
      method: "POST",
      idempotencyKey: randomUUID(),
      body: {
        subgroup_uuid: base.subgroup.uuid,
        pickup_branch_uuid: base.pickup_branch.uuid,
        pickup_point_uuid: base.pickup_point?.uuid ?? null,
        return_branch_uuid: base.return_branch.uuid,
        return_point_uuid: base.return_point?.uuid ?? null,
        starts_at: base.starts_at,
        ends_at: base.ends_at,
        extras,
      },
    });
  } catch (err) {
    const code = err instanceof ApiError ? err.code : "FAILED";
    redirect(`/book/${quoteId}?error=${encodeURIComponent(code)}`);
  }
  redirect(`/book/${quote.uuid}/details`);
}

export type BookState = {
  error?: string;
  fields?: Record<string, string>;
  values?: Record<string, string>;
};

/** Maps API field paths onto the form's field names. */
const FIELD_MAP: Record<string, string> = {
  "customer.first_name": "first_name",
  "customer.last_name": "last_name",
  "customer.company_title": "company_title",
  "customer.tax_office": "tax_office",
  "customer.phone": "phone",
  "customer.email": "email",
  "customer.type": "type",
  "drivers.0.birth_date": "birth_date",
  "drivers.0.licence_number": "licence_number",
  "drivers.0.licence_issued_on": "licence_issued_on",
  "drivers.0.first_name": "driver_first_name",
  "drivers.0.last_name": "driver_last_name",
  note: "note",
};

const FORM_FIELDS = [
  "type",
  "first_name",
  "last_name",
  "company_title",
  "tax_office",
  "phone",
  "email",
  "driver_first_name",
  "driver_last_name",
  "birth_date",
  "licence_number",
  "licence_issued_on",
  "note",
];

/** Details step: the booking (POST /v1/public/reservations). */
export async function book(
  _prev: BookState,
  form: FormData,
): Promise<BookState> {
  const values = Object.fromEntries(FORM_FIELDS.map((k) => [k, str(form, k)]));
  const quoteId = str(form, "quote_uuid");
  if (!UUID.test(quoteId)) return { error: "book.failed", values };

  const fields: Record<string, string> = {};
  const corporate = values.type === "corporate";
  const required = corporate
    ? [
        "company_title",
        "phone",
        "email",
        "driver_first_name",
        "driver_last_name",
      ]
    : ["first_name", "last_name", "phone", "email"];
  for (const k of [
    ...required,
    "birth_date",
    "licence_number",
    "licence_issued_on",
  ]) {
    if (!values[k]) fields[k] = "book.required";
  }
  if (values.email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(values.email))
    fields.email = "book.invalid";
  if (form.get("accept_terms") !== "on")
    fields.accept_terms = "book.terms_required";
  if (form.get("accept_privacy") !== "on")
    fields.accept_privacy = "book.terms_required";
  if (Object.keys(fields).length)
    return { error: "book.fix_errors", fields, values };

  const driver = {
    same_as_customer: !corporate,
    first_name: corporate ? values.driver_first_name : "",
    last_name: corporate ? values.driver_last_name : "",
    birth_date: values.birth_date,
    licence_number: values.licence_number,
    licence_issued_on: values.licence_issued_on,
    phone: values.phone,
    email: values.email,
  };
  let booked: Booked;
  try {
    booked = await bookingApi<Booked>("/v1/public/reservations", {
      method: "POST",
      idempotencyKey: str(form, "idempotency_key") || randomUUID(),
      body: {
        quote_uuid: quoteId,
        customer: {
          type: corporate ? "corporate" : "individual",
          first_name: corporate ? "" : values.first_name,
          last_name: corporate ? "" : values.last_name,
          company_title: corporate ? values.company_title : "",
          tax_office: corporate ? values.tax_office : "",
          phone: values.phone,
          email: values.email,
        },
        drivers: [driver],
        note: values.note,
      },
    });
  } catch (err) {
    if (!(err instanceof ApiError)) return { error: "book.failed", values };
    if (err.code === "PREVIEW_READ_ONLY")
      return { error: "preview.booking_disabled", values };
    if (err.code === "QUOTE_PRICE_CHANGED") {
      const replacement = err.data as { uuid?: string } | null;
      if (replacement?.uuid && UUID.test(replacement.uuid)) {
        redirect(`/book/${replacement.uuid}/details?changed=1`);
      }
    }
    if (err.code === "QUOTE_EXPIRED" || err.code === "QUOTE_USED")
      return { error: "book.quote_expired", values };
    if (err.code === "DRIVER_NOT_ELIGIBLE")
      return { error: "book.driver_not_eligible", values };
    if (err.code === "CAPACITY_UNAVAILABLE")
      return { error: "book.capacity", values };
    if (err.status === 400) {
      for (const d of err.details) {
        const f = FIELD_MAP[d.field ?? ""];
        if (f)
          fields[f] = d.code === "required" ? "book.required" : "book.invalid";
      }
      return { error: "book.fix_errors", fields, values };
    }
    return { error: "book.failed", values };
  }
  const site = await getSite();
  await rememberReservation(site, booked.reference, booked.access_token);
  redirect(`/reservation/${encodeURIComponent(booked.reference)}?new=1`);
}

/** Reservation page: the hosted pay page of the amount due now. */
export async function pay(form: FormData) {
  const reference = str(form, "reference");
  if (!/^[A-Za-z0-9-]{1,40}$/.test(reference)) redirect("/");
  const site = await getSite();
  const token = await reservationToken(site, reference);
  if (!token) redirect(`/reservation/${encodeURIComponent(reference)}`);
  let link: PaymentLink;
  try {
    link = await bookingApi<PaymentLink>(
      `/v1/public/reservations/${encodeURIComponent(reference)}/payment-links`,
      { method: "POST", idempotencyKey: randomUUID(), reservationToken: token },
    );
  } catch {
    redirect(`/reservation/${encodeURIComponent(reference)}?pay_error=1`);
  }
  redirect(link.url);
}
