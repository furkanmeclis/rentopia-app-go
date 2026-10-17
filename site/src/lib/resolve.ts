import { ApiError, apiFetch } from "./api.ts";
import type { Site } from "./types";

/**
 * Host → site, cached per host in this process (30 s; unknown hosts 10 s).
 * The cache key is the host only: two hosts never share an entry, and an
 * entry always names the organization the API resolved for that host.
 */
const TTL_MS = 30_000;
const MISS_TTL_MS = 10_000;
const MAX_ENTRIES = 2000;

type Entry = { site: Site | null; expires: number };
const cache = new Map<string, Entry>();

/** Lower-case host[:port] of a Host header, or "" when malformed. */
export function hostKey(raw: string | null | undefined): string {
  const h = (raw ?? "").trim().toLowerCase();
  return /^[a-z0-9.-]+(:\d{1,5})?$/.test(h) && h.length <= 260 ? h : "";
}

/**
 * Resolves a host. Returns null for hosts without a site; throws ApiError
 * when the API cannot answer (the caller shows a temporary error page and
 * never falls back to another site).
 */
export async function resolveSite(
  rawHost: string | null | undefined,
  clientIp?: string,
  preview?: string,
): Promise<Site | null> {
  const host = hostKey(rawHost);
  if (!host) return null;
  if (preview) return resolvePreview(host, preview, clientIp);
  const now = Date.now();
  const hit = cache.get(host);
  if (hit && hit.expires > now) return hit.site;

  let site: Site | null;
  try {
    site = await apiFetch<Site>(
      `/v1/public/site/resolve?host=${encodeURIComponent(host)}`,
      { clientIp },
    );
  } catch (err) {
    if (err instanceof ApiError && (err.status === 404 || err.status === 400)) {
      site = null;
    } else {
      throw err;
    }
  }
  if (cache.size >= MAX_ENTRIES) {
    const oldest = cache.keys().next().value;
    if (oldest !== undefined) cache.delete(oldest);
  }
  cache.set(host, {
    site,
    expires: now + (site ? TTL_MS : MISS_TTL_MS),
  });
  return site;
}

/**
 * The draft of a host's site for a preview token (never cached: a draft
 * changes with every save). Throws ApiError 401 PREVIEW_INVALID for an
 * invalid, expired or foreign token; null for unknown hosts.
 */
async function resolvePreview(
  host: string,
  token: string,
  clientIp?: string,
): Promise<Site | null> {
  try {
    return await apiFetch<Site>(
      `/v1/public/site/resolve?host=${encodeURIComponent(host)}&preview=${encodeURIComponent(token)}`,
      { clientIp },
    );
  } catch (err) {
    if (err instanceof ApiError && (err.status === 404 || err.status === 400)) {
      return null;
    }
    throw err;
  }
}
