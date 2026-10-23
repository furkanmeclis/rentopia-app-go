import { expect, test } from "@playwright/test";

import { bff, json, unknownHere } from "../support/bff";
import { errorState, evidence, fixture } from "../support/fixture";
import { authFile } from "../support/paths";

/**
 * RT-A02 (TEC-689): a member limited to branch MRK cannot open HVL's rental
 * or its handover / return form, nor HVL's collection, from the API or the
 * screen (404, never the record), and HVL is absent from the list and the
 * export. Go: TestBranchScopeRTA02, TestRT_A02_RentalHandoverBranchScope,
 * finance / invoices / reservations TestBranchScope.
 */
test.use({ storageState: authFile("mrkStaff") });

test("RT-A02 single-branch staff cannot open another branch's rental, handover or finance", async ({
  page,
}) => {
  const f = fixture();
  const api = bff(page.request, f.slug);
  const { hvl, mrk } = f.rentals;

  // API (through the BFF, as the browser does).
  const own = await json<{ reference: string }>(
    await api.get(`/v1/tenant/rentals/${mrk.uuid}`),
  );
  expect(own.status, "own branch rental").toBe(200);
  expect(own.data.reference).toBe(mrk.reference);

  for (const path of [
    `/v1/tenant/rentals/${hvl.uuid}`,
    `/v1/tenant/payments/${f.hvlPaymentUuid}`,
  ]) {
    const res = await json(await api.get(path));
    expect([403, 404], `GET ${path}`).toContain(res.status);
    expect(JSON.stringify(res.data ?? {})).not.toContain(hvl.reference);
  }
  for (const [path, body] of [
    [`/v1/tenant/rentals/${hvl.uuid}/return`, { km: 1300, fuel: 50 }],
    ["/v1/tenant/inspections", { rental_uuid: hvl.uuid, kind: "return" }],
  ] as const) {
    const res = await json(await api.post(path, body));
    expect(
      unknownHere(res),
      `POST ${path} → ${res.status} ${JSON.stringify(res.error)}`,
    ).toBe(true);
  }

  // Lists: the rental list and the payments (finance) of the member.
  const list = await json<{ items: { uuid: string }[] }>(
    await api.get("/v1/tenant/rentals?limit=100"),
  );
  expect(list.status).toBe(200);
  const ids = list.data.items.map((r) => r.uuid);
  expect(ids).toContain(mrk.uuid);
  expect(ids).not.toContain(hvl.uuid);
  const payments = await json<{ items: { uuid: string }[] }>(
    await api.get("/v1/tenant/payments?limit=100"),
  );
  expect(payments.status).toBe(200);
  expect(payments.data.items.map((p) => p.uuid)).not.toContain(
    f.hvlPaymentUuid,
  );

  // Export: the job runs with the member's branch scope.
  const job = await json<{ uuid: string }>(
    await api.post("/v1/tenant/rentals/export", { format: "csv" }),
  );
  expect(job.status).toBe(202);
  await expect
    .poll(
      async () =>
        (
          await json<{ status: string }>(
            await api.get(`/v1/tenant/exports/${job.data.uuid}`),
          )
        ).data.status,
      { timeout: 60_000 },
    )
    .toBe("completed");
  const csv = await (
    await api.get(`/v1/tenant/exports/${job.data.uuid}/download`)
  ).text();
  expect(csv).toContain(mrk.reference);
  expect(csv).not.toContain(hvl.reference);

  // Screens: the HVL rental and its return form are not found; the list
  // shows MRK only.
  await page.goto(`/t/${f.slug}/rentals/${hvl.uuid}`);
  await expect(errorState(page)).toContainText("Kiralama bulunamadı");
  await expect(page.getByText(hvl.reference)).toHaveCount(0);
  await evidence(page, "rt-a02-hvl-rental-not-found");

  await page.goto(`/t/${f.slug}/field/return/${hvl.uuid}`);
  await expect(errorState(page)).toBeVisible();
  await expect(page.getByText(hvl.reference)).toHaveCount(0);
  await evidence(page, "rt-a02-hvl-return-form-refused");

  await page.goto(`/t/${f.slug}/rentals`);
  await expect(page.getByText(mrk.reference).first()).toBeVisible();
  await expect(page.getByText(hvl.reference)).toHaveCount(0);
  await evidence(page, "rt-a02-rentals-list-mrk-only");
});
