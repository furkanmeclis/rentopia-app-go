import * as Sentry from "@sentry/nextjs";

import { sentryConfig } from "@/lib/app-env";

/**
 * Error tracking for the Node.js server and the edge proxy (TEC-744): a
 * no-op without SENTRY_DSN. No request bodies, cookies, query strings or
 * headers beyond an allowlist are sent; the site has no signed-in users.
 */
export async function register() {
  const config = sentryConfig();
  if (!config) return;
  if (
    process.env.NEXT_RUNTIME === "nodejs" ||
    process.env.NEXT_RUNTIME === "edge"
  ) {
    Sentry.init({
      ...config,
      tracesSampleRate: undefined,
      maxBreadcrumbs: 30,
      initialScope: { tags: { component: "site" } },
      beforeSend(event) {
        if (event.request) {
          const req = event.request;
          delete req.cookies;
          delete req.data;
          delete req.query_string;
          if (req.url) req.url = req.url.split(/[?#]/)[0];
          if (req.headers) {
            const keep = ["user-agent", "content-type", "accept"];
            req.headers = Object.fromEntries(
              Object.entries(req.headers).filter(([k]) =>
                keep.includes(k.toLowerCase()),
              ),
            );
          }
        }
        delete event.user;
        return event;
      },
    });
  }
}

// Server component, route handler and proxy errors (no-op when not
// initialised).
export const onRequestError = Sentry.captureRequestError;
