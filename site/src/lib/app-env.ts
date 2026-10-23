/**
 * Deployment environment (TEC-744): the same APP_ENV the API, the worker
 * and the panel read — development | staging | production, default
 * development. Read at runtime (container env).
 */
export type AppEnv = "development" | "staging" | "production" | "test";

const KNOWN: ReadonlySet<string> = new Set([
  "development",
  "staging",
  "production",
  "test",
]);

export function appEnv(raw: string | undefined = process.env.APP_ENV): AppEnv {
  const value = (raw ?? "").trim().toLowerCase();
  return KNOWN.has(value) ? (value as AppEnv) : "development";
}

/** Search engines index production only (staging copies stay hidden). */
export function indexable(env: AppEnv = appEnv()): boolean {
  return env === "production";
}

/**
 * Error tracking config for the Node.js and edge runtimes (SENTRY_DSN;
 * empty = off). Environment: SENTRY_ENVIRONMENT, else APP_ENV; release:
 * SENTRY_RELEASE.
 */
export function sentryConfig(
  env: Record<string, string | undefined> = process.env,
): { dsn: string; environment: string; release?: string } | null {
  const dsn = env.SENTRY_DSN?.trim();
  if (!dsn) return null;
  const release = env.SENTRY_RELEASE?.trim();
  return {
    dsn,
    environment: env.SENTRY_ENVIRONMENT?.trim() || appEnv(env.APP_ENV),
    ...(release ? { release } : {}),
  };
}
