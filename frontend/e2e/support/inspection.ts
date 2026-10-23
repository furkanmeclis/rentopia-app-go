import { expect, type APIRequestContext, type Page } from "@playwright/test";

import { bff, json } from "./bff";

export type Ins = {
  uuid: string;
  status: string;
  review_status: string;
  signature_status: string;
  disputed: boolean;
  template: {
    photo_slots: { key: string; required: boolean }[];
    checklist: { key: string }[];
  };
  readiness: {
    can_complete: boolean;
    incomplete_photos: number;
    missing_slots: string[];
    missing_fields: string[];
  };
};

/** A small real JPEG (a screenshot of the page) for photo slots. */
export async function jpeg(page: Page): Promise<Buffer> {
  return page.screenshot({
    type: "jpeg",
    quality: 40,
    clip: { x: 0, y: 0, width: 64, height: 64 },
  });
}

/**
 * Starts (or resumes) the form of a rental through the BFF and fills what
 * the scenario does not exercise: verification, km / fuel, equipment and
 * the customer review. Returns the draft.
 */
export async function prepareForm(
  request: APIRequestContext,
  slug: string,
  rentalUuid: string,
  kind: "handover" | "return",
  km: number,
): Promise<Ins> {
  const api = bff(request, slug);
  const started = await json<Ins>(
    await api.post("/v1/tenant/inspections", { rental_uuid: rentalUuid, kind }),
  );
  expect([200, 201], JSON.stringify(started.error)).toContain(started.status);
  const equipment = Object.fromEntries(
    started.data.template.checklist.map((i) => [i.key, true]),
  );
  const patched = await request.patch(
    `/api/v1/tenant/inspections/${started.data.uuid}`,
    {
      headers: { "X-Organization": slug },
      data: {
        km,
        fuel: 80,
        equipment,
        verification: {
          plate_confirmed: true,
          vehicle_ok: true,
          licence_checked: true,
          identity_checked: true,
        },
        customer_reviewed: true,
      },
    },
  );
  const out = await json<Ins>(patched);
  expect(out.status, JSON.stringify(out.error)).toBe(200);
  return out.data;
}

/** Announces, uploads (multipart PUT to the API-issued path) and completes a photo. */
export async function uploadPhoto(
  request: APIRequestContext,
  slug: string,
  inspectionUuid: string,
  slot: string,
  body: Buffer,
  damageUuid?: string,
) {
  const headers = { "X-Organization": slug };
  const announced = await json<{
    photo: { uuid: string };
    upload: { path: string };
  }>(
    await request.post(`/api/v1/tenant/inspections/${inspectionUuid}/photos`, {
      headers,
      data: {
        slot,
        damage_uuid: damageUuid,
        content_type: "image/jpeg",
        size_bytes: body.length,
        client_ref: `e2e-${slot}-${damageUuid ?? ""}`,
      },
    }),
  );
  expect([200, 201], JSON.stringify(announced.error)).toContain(
    announced.status,
  );
  const put = await request.put(`/api${announced.data.upload.path}`, {
    headers,
    multipart: {
      file: { name: `${slot}.jpg`, mimeType: "image/jpeg", buffer: body },
    },
  });
  expect(put.status(), await put.text()).toBeLessThan(300);
  const done = await request.post(
    `/api/v1/tenant/inspections/${inspectionUuid}/photos/${announced.data.photo.uuid}/complete`,
    { headers },
  );
  expect(done.status(), await done.text()).toBeLessThan(300);
}

/** Clicks "İleri" until the wizard shows step `n` (1-based). */
export async function gotoStep(page: Page, n: number, total: number) {
  for (;;) {
    const label = await page.getByTestId("wizard-step").innerText();
    const m = /(\d+)\s*\/\s*(\d+)/.exec(label);
    expect(m, label).not.toBeNull();
    expect(Number(m![2])).toBe(total);
    const at = Number(m![1]);
    if (at === n) return;
    await page.getByTestId("wizard-next").click();
    await expect(page.getByTestId("wizard-step")).toContainText(
      `${at + 1}/${total}`,
    );
  }
}
