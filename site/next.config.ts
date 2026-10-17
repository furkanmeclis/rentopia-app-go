import type { NextConfig } from "next";

import { frameHeaders } from "./src/lib/preview";

/**
 * One image serves every organization: nothing about an organization is
 * known at build time. The request host picks the site at runtime
 * (src/proxy.ts → GET /v1/public/site/resolve).
 */
const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=(), payment=()",
  },
  ...(process.env.NODE_ENV === "production"
    ? [{ key: "Strict-Transport-Security", value: "max-age=31536000" }]
    : []),
];

/**
 * Framing: page responses get their Content-Security-Policy and
 * X-Frame-Options from the proxy (src/proxy.ts → lib/preview.ts
 * frameHeaders): `frame-ancestors 'none'`, except a draft preview, which
 * only the panel's origin may frame. Paths the proxy does not see get the
 * deny policy here. form-action covers the redirect to the hosted pay page
 * (the panel's /pay/{token}) after the payment form posts.
 */
const denyFraming = Object.entries(frameHeaders(null)).map(([key, value]) => ({
  key,
  value,
}));

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async headers() {
    return [
      { source: "/:path*", headers: securityHeaders },
      { source: "/_next/:path*", headers: denyFraming },
      { source: "/media/:path*", headers: denyFraming },
      { source: "/health", headers: denyFraming },
      { source: "/favicon.ico", headers: denyFraming },
    ];
  },
};

export default nextConfig;
