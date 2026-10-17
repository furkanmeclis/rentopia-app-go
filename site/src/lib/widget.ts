/** Exact origins only; wildcard embedding is deliberately unsupported. */
export function widgetOrigin(raw: string): string {
  try {
    const u = new URL(raw.includes("://") ? raw : `https://${raw}`);
    if (
      u.username ||
      u.password ||
      u.search ||
      u.hash ||
      (u.pathname !== "/" && u.pathname !== "") ||
      u.searchParams.size > 0 ||
      raw.includes("?") ||
      raw.includes("#") ||
      u.hostname.length > 253 ||
      !u.hostname
        .split(".")
        .every((label) => /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/i.test(label))
    )
      return "";
    if (
      u.protocol !== "https:" &&
      !(
        u.protocol === "http:" &&
        (u.hostname === "localhost" || u.hostname.endsWith(".localhost"))
      )
    )
      return "";
    return u.origin;
  } catch {
    return "";
  }
}

export function allowedParent(raw: string | null, domains: string[]): string {
  if (!raw) return "";
  const origin = widgetOrigin(raw);
  return origin && domains.some((d) => widgetOrigin(d) === origin)
    ? origin
    : "";
}

export function embedFrameHeaders(domains: string[]): Record<string, string> {
  const origins = [...new Set(domains.map(widgetOrigin).filter(Boolean))].slice(
    0,
    20,
  );
  return {
    "Content-Security-Policy": `frame-ancestors ${origins.join(" ") || "'none'"}; base-uri 'self'; form-action 'self' https:`,
    "Cache-Control": "private, no-store",
    "Referrer-Policy": "no-referrer",
    "X-Robots-Tag": "noindex, nofollow",
  };
}
