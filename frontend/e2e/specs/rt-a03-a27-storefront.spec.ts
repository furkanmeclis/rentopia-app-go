import { expect, test } from "@playwright/test";

import { Api } from "../support/api";
import { siteBase } from "../support/env";
import { evidence, fixture } from "../support/fixture";
import {
  cards,
  cents,
  openPartnerWidget,
  publicAvailability,
  searchQuery,
} from "../support/storefront";

// Visitors of the branded site and the widget are anonymous.
test.use({ storageState: { cookies: [], origins: [] } });

/**
 * RT-A03 (TEC-691 / TEC-697): 10 Clio + 10 Corsa + 10 i20 in one rental
 * group behind one group-guaranteed product → the site shows one card and
 * the search reports capacity 30 (the two HVL cars do not count at MRK).
 * The panel's quote search shows the capacity; the public answer reveals
 * it capped at 10 (`units_left`, by design: public/availability).
 * Go: TestRT_A03_ThirtyVehiclesOneProduct,
 * TestRT_A03_MixedModelsOneCardAndExactVariant.
 */
test("RT-A03 thirty cars of three models are one card with capacity 30", async ({
  page,
}) => {
  const f = fixture();
  const admin = Api.withToken(f.adminToken).in(f.slug);
  const panel = await admin.call<{
    items: { product: { uuid: string }; sellable: number }[];
  }>("POST", "/v1/tenant/quotes/search", {
    pickup_branch_uuid: f.branches.mrk,
    return_branch_uuid: f.branches.mrk,
    starts_at: f.search.startsAt,
    ends_at: f.search.endsAt,
    channel: "web",
  });
  expect(panel.items.map((i) => i.product.uuid)).toEqual([f.product.uuid]);
  expect(panel.items[0].sellable).toBe(30);

  const items = await publicAvailability(f);
  expect(items.map((i) => i.subgroup.uuid)).toEqual([f.product.uuid]);
  expect(items[0].available).toBe(true);
  expect(items[0].units_left).toBe(Math.min(30, 10));

  await page.goto(`${siteBase(f.site.subdomain)}/search?${searchQuery(f)}`);
  const site = await cards(page);
  expect(site.subgroups).toEqual([f.product.uuid]);
  for (const model of ["Corsa", "i20"])
    await expect(page.getByRole("heading", { name: model })).toHaveCount(0);
  await evidence(page, "rt-a03-site-one-card");
});

/**
 * RT-A27 (TEC-728): the same search answers the same price and capacity on
 * the public API, the branded site and the widget embedded on a partner
 * origin. Go: TestCatalogAndSameAnswerEverywhere (API, site and widget
 * sessions and the panel's quote search).
 */
test("RT-A27 site, widget and public API give the same price and capacity", async ({
  page,
}) => {
  const f = fixture();
  const [api] = await publicAvailability(f);
  expect(api.price).not.toBeNull();
  const apiTotal = cents(api.price!.total);

  await page.goto(`${siteBase(f.site.subdomain)}/search?${searchQuery(f)}`);
  const site = await cards(page);
  expect(site.subgroups).toEqual([api.subgroup.uuid]);
  expect(cents(site.totals[0])).toBe(apiTotal);

  await expect(page.locator(".rt-units-left")).toHaveCount(0);
  await evidence(page, "rt-a27-site-search");

  const { frame, server } = await openPartnerWidget(page, f);
  try {
    const widget = await cards(frame);
    expect(widget.subgroups).toEqual([api.subgroup.uuid]);
    expect(cents(widget.totals[0])).toBe(apiTotal);
    // Capacity: the API reveals units_left (capped at 10); site and widget
    // show "son N araç" only for N ≤ 3, so with 30 free cars neither does.
    expect(api.units_left).toBe(10);
    await expect(frame.locator(".rt-units-left")).toHaveCount(0);
    await evidence(page, "rt-a27-widget-on-partner");
  } finally {
    server.close();
  }
});
