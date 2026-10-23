import { defineConfig, devices } from "@playwright/test";

import { panelBase } from "./support/env";
import { authFile, launchOptions } from "./support/paths";

/**
 * Acceptance e2e (TEC-740, docs/acceptance/beta.md). Needs the stack of
 * e2e/stack/up.sh (API :18080, panel :13000, site :13100); run with
 * `make acceptance` or `pnpm e2e` from frontend/.
 */
export default defineConfig({
  testDir: "./specs",
  globalSetup: "./global-setup.ts",
  outputDir: "./.state/test-results",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 120_000,
  expect: { timeout: 20_000 },
  reporter: [["list"], ["html", { outputFolder: "./.state/report", open: "never" }]],
  use: {
    baseURL: panelBase(),
    locale: "tr-TR",
    timezoneId: "Europe/Istanbul",
    viewport: { width: 1280, height: 800 },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: launchOptions(),
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], storageState: authFile("admin") },
    },
  ],
});
