import "server-only";

import { notFound } from "next/navigation";

import { ApiError } from "./api";
import { bookingApi } from "./server";
import type { Quote } from "./types";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** A quote of this site's session (another site's or an unknown quote is 404). */
export async function loadQuote(id: string): Promise<Quote> {
  if (!UUID.test(id)) notFound();
  try {
    return await bookingApi<Quote>(`/v1/public/quotes/${id}`);
  } catch (err) {
    if (err instanceof ApiError && (err.status === 404 || err.status === 400))
      notFound();
    throw err;
  }
}
