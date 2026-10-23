import { expect, test } from "@playwright/test";

import { Api } from "../support/api";
import { bff, json } from "../support/bff";
import { evidence, fixture } from "../support/fixture";
import { authFile } from "../support/paths";

test.use({ storageState: authFile("fieldStaff") });

/**
 * RT-A29 (TEC-724): an airport delivery reservation is priced with the
 * point's delivery fee, opens a delivery (handover) task that the manager
 * assigned to the field executor (seed), the executor sees it in
 * "Görevlerim", starts the trip and goes from the task to the rental's
 * handover form. Go: TestRT_A29_AirportDelivery (fees, notification,
 * completion through the form, vehicle location).
 */
test("RT-A29 airport delivery creates a field task the executor works into the handover form", async ({
  page,
}) => {
  const f = fixture();

  // Fee: the reservation carries the airport point's delivery line(s).
  const admin = Api.withToken(f.adminToken).in(f.slug);
  const reservation = await admin.call<{
    price: { snapshot: { lines: { kind: string; amount: string }[] } };
  }>("GET", `/v1/tenant/reservations/${f.deliveryReservation.uuid}`);
  const delivery = reservation.price.snapshot.lines.filter(
    (l) => l.kind === "delivery",
  );
  expect(delivery.length).toBeGreaterThan(0);
  for (const l of delivery) expect(Number(l.amount)).toBe(750);

  // The executor's own list (API, as the browser calls it).
  const mine = await json<{ items: { uuid: string }[] }>(
    await bff(page.request, f.slug).get("/v1/tenant/field-tasks/mine?limit=50"),
  );
  expect(mine.status).toBe(200);
  expect(mine.data.items.map((x) => x.uuid)).toContain(f.deliveryTaskUuid);

  // Screen: "Görevlerim" → the task → start → handover form.
  await page.goto(`/t/${f.slug}/field`);
  const link = page.locator(`a[href$="/field/my/${f.deliveryTaskUuid}"]`);
  // The delivery is tomorrow (Istanbul); the list opens on today.
  const tomorrow = page.getByRole("tab", { name: /Yarın/ });
  await expect(tomorrow).toBeVisible({ timeout: 90_000 });
  if ((await link.count()) === 0) await tomorrow.click();
  await expect(link.first()).toBeVisible();
  await expect(link.first()).toContainText("SAW Havalimanı");
  await evidence(page, "rt-a29-my-tasks");
  await link.first().click();
  await expect(page.getByText("TK2412").first()).toBeVisible();
  await page.getByRole("button", { name: "Yola çık" }).first().click();
  const toForm = page.getByRole("link", { name: "Teslim formuna geç" }).first();
  await expect(toForm).toBeVisible();
  await evidence(page, "rt-a29-task-en-route");
  await toForm.click();
  await expect(page).toHaveURL(
    new RegExp(`/field/handover/${f.rentals.delivery.uuid}$`),
  );
  await expect(page.getByTestId("wizard-step")).toBeVisible({
    timeout: 90_000,
  });
  await expect(
    page.getByText(f.rentals.delivery.reference).first(),
  ).toBeVisible();
  await evidence(page, "rt-a29-handover-form-from-task");

  const task = await json<{ status: string }>(
    await bff(page.request, f.slug).get(
      `/v1/tenant/field-tasks/${f.deliveryTaskUuid}`,
    ),
  );
  expect(task.data.status).toBe("en_route");
});
