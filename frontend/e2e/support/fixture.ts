import { readFileSync } from "node:fs";
import type { Page } from "@playwright/test";

import { evidenceDir, fixtureFile } from "./paths";
import type { Fixture } from "./seed";

let cached: Fixture | null = null;

/** The ids global-setup provisioned for this run. */
export function fixture(): Fixture {
  if (!cached)
    cached = JSON.parse(readFileSync(fixtureFile, "utf8")) as Fixture;
  return cached;
}

/** The page's ErrorState (not Next's route announcer, also role=alert). */
export const errorState = (page: Page) =>
  page.locator('[role="alert"]:not(#__next-route-announcer__)').first();

/** Viewport screenshot kept as evidence (small PNG, see docs/acceptance). */
export async function evidence(page: Page, name: string) {
  await page.screenshot({ path: `${evidenceDir}/${name}.png` });
}
