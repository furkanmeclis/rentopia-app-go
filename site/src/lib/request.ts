/** Current path + query, set by the proxy for server components. */
export const PATH_HEADER = "x-rtp-path";

/** The visitor's IP from the reverse proxy headers ("" when unknown). */
export function clientIpFrom(headers: Headers): string {
  const fwd = headers.get("x-forwarded-for");
  const first = fwd?.split(",")[0]?.trim() ?? headers.get("x-real-ip")?.trim();
  return first && /^[0-9a-fA-F:.]{2,45}$/.test(first) ? first : "";
}

/**
 * A same-site path to go back to after an action ("/" when the value could
 * leave the site: absolute URLs, protocol-relative paths, backslashes).
 */
export function safeNext(raw: string | null | undefined): string {
  const v = (raw ?? "").trim();
  if (
    !v.startsWith("/") ||
    v.startsWith("//") ||
    v.includes("\\") ||
    v.length > 1000
  ) {
    return "/";
  }
  return v;
}
