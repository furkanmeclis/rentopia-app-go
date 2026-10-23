import type { APIRequestContext, APIResponse } from "@playwright/test";

/**
 * Calls the panel's same-origin BFF (/api/v1/*) as the signed-in user of
 * the page's context, exactly like the browser code does.
 */
export function bff(request: APIRequestContext, slug: string) {
  const headers = { "X-Organization": slug };
  return {
    get: (path: string) => request.get(`/api${path}`, { headers }),
    post: (path: string, data?: unknown) =>
      request.post(`/api${path}`, { headers, data: data ?? {} }),
  };
}

type ApiError = {
  code?: string;
  details?: { field?: string; code?: string }[];
};

/**
 * "Unknown here": 404 for a path record, 403 for a route, or a 400 whose
 * detail says the referenced record was not found (a body reference).
 */
export function unknownHere(res: { status: number; error?: ApiError }) {
  return (
    res.status === 403 ||
    res.status === 404 ||
    (res.status === 400 &&
      (res.error?.details ?? []).some((d) => d.code === "not_found"))
  );
}

export async function json<T = Record<string, unknown>>(
  res: APIResponse,
): Promise<{ status: number; data: T; error?: ApiError }> {
  const body = (await res.json().catch(() => ({}))) as {
    data?: T;
    error?: ApiError;
  };
  return { status: res.status(), data: body.data as T, error: body.error };
}
