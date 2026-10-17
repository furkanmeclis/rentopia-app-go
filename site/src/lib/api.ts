/**
 * Server-side client of the Go API (API_URL, the internal address; never
 * reachable from a browser). The site only calls public routes:
 * /v1/public/site/* to resolve the host and /v1/public/* (the public
 * booking API, TEC-728) with a booking session token bound to the site's
 * origin. No staff credential ever exists in this app.
 */

/** API origin without /v1 (API_URL is "http://backend:8080/v1"). */
export function apiOrigin(): string {
  const raw = process.env.API_URL ?? "http://127.0.0.1:8080/v1";
  return raw.replace(/\/+$/, "").replace(/\/v1$/, "");
}

export type ApiDetail = { field?: string; code?: string; message?: string };

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ApiDetail[];
  readonly data: unknown;

  constructor(
    status: number,
    code: string,
    message: string,
    details: ApiDetail[] = [],
    data: unknown = null,
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
    this.data = data;
  }
}

export type ApiRequest = {
  method?: "GET" | "POST";
  body?: unknown;
  /** Booking session token (Bearer). */
  token?: string;
  /** The site's origin: the API checks it against the session. */
  origin?: string;
  /** The visitor's IP (per-IP rate limits). */
  clientIp?: string;
  idempotencyKey?: string;
  reservationToken?: string;
};

/** Calls an API path (starting with /v1) and returns the envelope's data. */
export async function apiFetch<T>(
  path: string,
  req: ApiRequest = {},
): Promise<T> {
  const headers = new Headers({ Accept: "application/json" });
  if (req.body !== undefined) headers.set("Content-Type", "application/json");
  if (req.token) headers.set("Authorization", `Bearer ${req.token}`);
  if (req.origin) headers.set("X-Forwarded-Origin", req.origin);
  if (req.clientIp) headers.set("X-Forwarded-For", req.clientIp);
  if (req.idempotencyKey) headers.set("Idempotency-Key", req.idempotencyKey);
  if (req.reservationToken)
    headers.set("X-Reservation-Token", req.reservationToken);

  let res: Response;
  try {
    res = await fetch(`${apiOrigin()}${path}`, {
      method: req.method ?? "GET",
      headers,
      body: req.body === undefined ? undefined : JSON.stringify(req.body),
      cache: "no-store",
      signal: AbortSignal.timeout(15_000),
    });
  } catch {
    throw new ApiError(
      503,
      "UPSTREAM_UNAVAILABLE",
      "The booking service is unreachable",
    );
  }
  let payload: {
    data?: T;
    error?: { code?: string; message?: string; details?: ApiDetail[] };
  } = {};
  try {
    payload = await res.json();
  } catch {
    // Non-JSON answers keep the status below.
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      payload.error?.code ?? "HTTP_" + res.status,
      payload.error?.message ?? res.statusText,
      payload.error?.details ?? [],
      payload.data ?? null,
    );
  }
  return payload.data as T;
}
