import type { NextConfig } from "next";

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
  // The embeddable widget (TEC-732) is a separate entry point; site pages
  // are never framed.
  // form-action covers the redirect to the hosted pay page (the panel's
  // /pay/{token}) after the payment form posts.
  {
    key: "Content-Security-Policy",
    value: `frame-ancestors 'none'; base-uri 'self'; form-action 'self' https:${
      process.env.NODE_ENV === "production" ? "" : " http://localhost:*"
    }`,
  },
  { key: "X-Frame-Options", value: "DENY" },
  ...(process.env.NODE_ENV === "production"
    ? [{ key: "Strict-Transport-Security", value: "max-age=31536000" }]
    : []),
];

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
};

export default nextConfig;
