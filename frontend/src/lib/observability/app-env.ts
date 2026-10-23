/**
 * Deployment environment (TEC-744): the same APP_ENV the API and the
 * worker read — development | staging | production, default development.
 * Read at runtime (container env), never inlined at build time.
 */
export type AppEnv = "development" | "staging" | "production" | "test";

const KNOWN = new Set<AppEnv>(["development", "staging", "production", "test"]);

export function appEnv(raw = process.env.APP_ENV): AppEnv {
  const value = (raw ?? "").trim().toLowerCase();
  return KNOWN.has(value as AppEnv) ? (value as AppEnv) : "development";
}

/** Sentry environment: SENTRY_ENVIRONMENT overrides APP_ENV. */
export function sentryEnvironment(): string {
  return process.env.SENTRY_ENVIRONMENT?.trim() || appEnv();
}

/** Sentry release (SENTRY_RELEASE, e.g. the image tag); undefined = SDK default. */
export function sentryRelease(): string | undefined {
  return process.env.SENTRY_RELEASE?.trim() || undefined;
}
