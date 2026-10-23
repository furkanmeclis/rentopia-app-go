import { existsSync } from "node:fs";
import path from "node:path";

/** Run state (fixture ids, signed-in sessions); git-ignored. */
export const stateDir = path.resolve(__dirname, "..", ".state");
export const fixtureFile = path.join(stateDir, "fixture.json");
export const authFile = (who: "admin" | "mrkStaff" | "fieldStaff") =>
  path.join(stateDir, `auth-${who}.json`);
/** Screenshots referenced from docs/acceptance/beta.md. */
export const evidenceDir = path.resolve(__dirname, "..", "evidence");

/**
 * Chromium: E2E_CHROMIUM_PATH, else a preinstalled sandbox build at
 * /opt/pw-browsers/chromium, else Playwright's own (CI installs it).
 */
export function launchOptions(): { executablePath?: string } {
  const fromEnv = process.env.E2E_CHROMIUM_PATH;
  if (fromEnv) return { executablePath: fromEnv };
  if (existsSync("/opt/pw-browsers/chromium"))
    return { executablePath: "/opt/pw-browsers/chromium" };
  return {};
}
