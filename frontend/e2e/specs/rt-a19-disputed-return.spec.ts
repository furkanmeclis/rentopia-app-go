import { expect, test } from "@playwright/test";

import { bff, json } from "../support/bff";
import { evidence, fixture } from "../support/fixture";
import {
  gotoStep,
  jpeg,
  prepareForm,
  uploadPhoto,
  type Ins,
} from "../support/inspection";

type Rental = {
  status: string;
  charges: { kind: string; status: string; disputed: boolean }[];
};

/**
 * RT-A19 (TEC-713): a return the customer does not sign, with a damage the
 * customer disputes, is still recorded: the form is finalized with the
 * refusal, flagged disputed, lands in the authorized review queue, and its
 * damage charge stays a disputed proposal (never an approved debt). The
 * evidence and the dispute are entered through the API, the completion
 * without a signature in the mobile wizard. Go:
 * TestRT_A19_DisputedReturnNeverBecomesDebt, TestRT_A19_SignedReturnCompletes.
 */
test("RT-A19 unsigned, disputed return is recorded and lands in the review queue", async ({
  page,
}) => {
  const f = fixture();
  const rental = f.rentals.returning;
  const api = bff(page.request, f.slug);
  await page.goto(`/t/${f.slug}`);
  const photo = await jpeg(page);

  const ins = await prepareForm(page.request, f.slug, rental.uuid, "return", 15250);
  for (const s of ins.template.photo_slots.filter((x) => x.required))
    await uploadPhoto(page.request, f.slug, ins.uuid, s.key, photo);
  const withDamage = await json<Ins & { damages: { uuid: string }[] }>(
    await api.post(`/v1/tenant/inspections/${ins.uuid}/damages`, {
      zone: "rear_bumper",
      damage_type: "dent",
      severity: "moderate",
      description: "Arka tampon göçük",
      disputed: true,
    }),
  );
  expect(withDamage.status, JSON.stringify(withDamage.error)).toBe(201);
  const damage = withDamage.data.damages[0].uuid;
  await uploadPhoto(page.request, f.slug, ins.uuid, "damage", photo, damage);
  const disputed = await page.request.patch(
    `/api/v1/tenant/inspections/${ins.uuid}`,
    {
      headers: { "X-Organization": f.slug },
      data: {
        disputed: true,
        dispute_note: "Müşteri hasarın kendisine ait olmadığını belirtti",
        customer_statement: "Tampon teslimde de böyleydi.",
        charge_overrides: [
          { key: `damage:${damage}`, amount: "2000", reason: "Tampon onarımı" },
        ],
      },
    },
  );
  expect(disputed.status(), await disputed.text()).toBe(200);

  // The wizard: complete without a signature, with the refusal reason.
  await page.goto(`/t/${f.slug}/field/return/${rental.uuid}`);
  await expect(page.getByTestId("wizard-step")).toBeVisible({ timeout: 90_000 });
  await gotoStep(page, 8, 8);
  const submit = page.getByTestId("complete-inspection");
  await expect(submit).toBeDisabled(); // no reason yet
  await page.locator("#no-signature").fill("Müşteri imzalamayı reddetti");
  await expect(submit).toBeEnabled();
  await submit.click();
  const done = page.getByTestId("inspection-finalized");
  await expect(done).toContainText("İade yetkili incelemesinde");
  await evidence(page, "rt-a19-return-recorded-in-review");

  const final = await json<Ins>(
    await api.get(`/v1/tenant/inspections/${ins.uuid}`),
  );
  expect(final.data.status).toBe("finalized");
  expect(final.data.signature_status).toBe("refused");
  expect(final.data.disputed).toBe(true);
  expect(final.data.review_status).toBe("pending");

  // The rental waits for the review; the damage is a disputed proposal.
  const r = await json<Rental>(await api.get(`/v1/tenant/rentals/${rental.uuid}`));
  expect(r.data.status).toBe("return_check");
  const damageCharges = r.data.charges.filter((c) => c.kind === "damage");
  expect(damageCharges.length).toBeGreaterThan(0);
  for (const c of r.data.charges) {
    expect(c.status, `${c.kind} must stay a proposal`).toBe("proposed");
    expect(c.disputed, `${c.kind} must be disputed`).toBe(true);
  }

  // The authorized review queue (the inspections list opens on it).
  await page.goto(`/t/${f.slug}/inspections`);
  const row = page.getByRole("row").filter({ hasText: rental.reference });
  await expect(row).toBeVisible();
  await expect(row).toContainText("İnceleme bekliyor");
  await evidence(page, "rt-a19-review-queue");
});
