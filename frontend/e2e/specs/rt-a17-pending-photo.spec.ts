import { expect, test, type Route } from "@playwright/test";

import { bff, json } from "../support/bff";
import { evidence, fixture } from "../support/fixture";
import {
  gotoStep,
  jpeg,
  prepareForm,
  uploadPhoto,
  type Ins,
} from "../support/inspection";

/**
 * RT-A17 (TEC-712): the handover form cannot become final while a photo
 * upload is pending. Everything else is ready (fields through the API, five
 * of six slots uploaded); the sixth photo is taken in the wizard and its
 * upload is held at the network: "Tamamla" stays disabled, the checklist
 * shows the pending upload and the API refuses the completion. Releasing
 * the upload is the only change that enables "Tamamla". Go:
 * TestRT_A17_UnuploadedPhotoBlocksFinalize.
 */
test("RT-A17 handover cannot be finalized while a photo upload is pending", async ({
  page,
}) => {
  const f = fixture();
  const rental = f.rentals.handover.uuid;
  await page.goto(`/t/${f.slug}`);
  const photo = await jpeg(page);
  const ins = await prepareForm(page.request, f.slug, rental, "handover", 1100);
  const slots = ins.template.photo_slots.filter((s) => s.required);
  expect(slots.length).toBeGreaterThan(1);
  const last = slots[slots.length - 1].key;
  for (const s of slots.slice(0, -1))
    await uploadPhoto(page.request, f.slug, ins.uuid, s.key, photo);

  // Hold the byte upload of the wizard's photo until we release it.
  let release!: () => void;
  const released = new Promise<void>((r) => (release = r));
  let held = 0;
  await page.route(
    "**/api/v1/tenant/inspections/*/photos/*/file",
    async (route: Route) => {
      held++;
      await released;
      await route.continue();
    },
  );

  await page.goto(`/t/${f.slug}/field/handover/${rental}`);
  await expect(page.getByTestId("wizard-step")).toBeVisible({
    timeout: 90_000,
  });
  await gotoStep(page, 5, 8); // photos
  await page.getByTestId(`photo-input-${last}`).setInputFiles({
    name: `${last}.jpg`,
    mimeType: "image/jpeg",
    buffer: photo,
  });
  await expect.poll(() => held, { timeout: 30_000 }).toBe(1);
  await expect(page.getByText("1 yükleme bekliyor").first()).toBeVisible();

  await gotoStep(page, 8, 8); // complete
  await page.locator("#no-signature").fill("Müşteri tablette imza atamadı");
  const submit = page.getByTestId("complete-inspection");
  await expect(submit).toBeDisabled();
  await expect(page.getByTestId("readiness")).toContainText(
    "1 fotoğraf yüklenmeyi / doğrulanmayı bekliyor",
  );
  await evidence(page, "rt-a17-complete-disabled-while-uploading");

  // The server refuses the same: the photo row exists, its bytes do not.
  const refused = await json<Ins>(
    await bff(page.request, f.slug).post(
      `/v1/tenant/inspections/${ins.uuid}/complete`,
      { no_signature_reason: "Müşteri tablette imza atamadı" },
    ),
  );
  expect(refused.status).toBe(409);
  expect(refused.error?.code).toBeTruthy();
  const draft = await json<Ins>(
    await bff(page.request, f.slug).get(`/v1/tenant/inspections/${ins.uuid}`),
  );
  expect(draft.data.status).toBe("draft");
  expect(draft.data.readiness.incomplete_photos).toBe(1);
  expect(draft.data.readiness.can_complete).toBe(false);

  // Release the upload: the queue finishes, "Tamamla" becomes available
  // and the form is finalized.
  release();
  await expect(submit).toBeEnabled({ timeout: 60_000 });
  await expect(page.getByTestId("readiness")).toContainText(
    "Tüm fotoğraflar yüklendi ve doğrulandı",
  );
  await submit.click();
  await expect(page.getByTestId("inspection-finalized")).toBeVisible();
  await expect(page.getByTestId("inspection-finalized")).toContainText(
    "Araç teslim edildi",
  );
  await evidence(page, "rt-a17-finalized-after-upload");
});
