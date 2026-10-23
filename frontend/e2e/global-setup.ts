import { chromium } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

import { panelBase } from "./support/env";
import {
  authFile,
  fixtureFile,
  launchOptions,
  stateDir,
} from "./support/paths";
import { seed } from "./support/seed";

/**
 * Provisions the acceptance organization through the API (support/seed.ts)
 * and signs the three panel users in through the real login form, storing
 * their sessions for the specs (e2e/.state, git-ignored).
 */
export default async function globalSetup() {
  mkdirSync(stateDir, { recursive: true });
  const fixture = await seed();
  writeFileSync(fixtureFile, JSON.stringify(fixture, null, 2));

  const browser = await chromium.launch(launchOptions());
  try {
    for (const [who, email] of [
      ["admin", fixture.users.admin.email],
      ["mrkStaff", fixture.users.mrkStaff.email],
      ["fieldStaff", fixture.users.fieldStaff.email],
    ] as const) {
      const context = await browser.newContext({ baseURL: panelBase() });
      const page = await context.newPage();
      await page.goto("/login");
      // The form submits natively until React hydrates (dev compiles the
      // route on first use): wait for the client before typing.
      await page.waitForLoadState("networkidle", { timeout: 120_000 });
      await page.locator('input[name="email"]').fill(email);
      await page.locator('input[name="password"]').fill(fixture.password);
      await page.locator('form button[type="submit"]').last().click();
      await page.waitForURL((url) => !url.pathname.startsWith("/login"), {
        timeout: 120_000,
      });
      await context.storageState({ path: authFile(who) });
      await context.close();
    }
  } finally {
    await browser.close();
  }
  console.log(
    `[acceptance] organization ${fixture.slug} seeded (${path.relative(process.cwd(), fixtureFile)})`,
  );
}
