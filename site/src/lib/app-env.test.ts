import assert from "node:assert/strict";
import { test } from "node:test";

import { appEnv, indexable, sentryConfig } from "./app-env.ts";

test("APP_ENV is normalized, unknown values mean development", () => {
  assert.equal(appEnv(" Production "), "production");
  assert.equal(appEnv("staging"), "staging");
  assert.equal(appEnv(undefined), "development");
  assert.equal(appEnv("prod"), "development");
  assert.equal(indexable("production"), true);
  assert.equal(indexable("staging"), false);
});

test("error tracking is off without a DSN and tags APP_ENV", () => {
  assert.equal(sentryConfig({}), null);
  assert.deepEqual(
    sentryConfig({
      SENTRY_DSN: " https://k@sentry.test/2 ",
      APP_ENV: "staging",
    }),
    { dsn: "https://k@sentry.test/2", environment: "staging" },
  );
  assert.deepEqual(
    sentryConfig({
      SENTRY_DSN: "https://k@sentry.test/2",
      SENTRY_ENVIRONMENT: "prod-eu",
      SENTRY_RELEASE: "v1.2.3",
    }),
    {
      dsn: "https://k@sentry.test/2",
      environment: "prod-eu",
      release: "v1.2.3",
    },
  );
});
