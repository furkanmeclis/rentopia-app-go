import { Api } from "./api";
import { e2eEnv } from "./env";

/**
 * Provisions one organization for the acceptance run through the Go API
 * (the same routes the panel uses), so every scenario starts from data the
 * product itself created:
 *
 *   - MRK (default "Merkez") and HVL branches, open all day; an airport
 *     service point of MRK with a 750 TL delivery fee (RT-A29);
 *   - one segment, one rental group "KOMPAKT" and one GROUP-guaranteed
 *     product with an active common rate plan (1–3 / 4–7 / 8+ days);
 *   - 30 cars at MRK (10 Clio, 10 Corsa, 10 i20) and 2 at HVL, all members
 *     of the group (RT-A03: one card, capacity 30 at MRK);
 *   - a customer with a driver, users: the owner (platform admin), a
 *     branch manager limited to MRK (RT-A02) and a field executor limited
 *     to MRK (RT-A29);
 *   - rentals: HVL (handed over, with a branch collection) and MRK for
 *     RT-A02, one in preparation for RT-A17, one handed over for RT-A19, an
 *     airport delivery reservation with its rental for RT-A29;
 *   - the branded site on a subdomain with the widget allowed on the
 *     partner origin, and a secret public API key (RT-A27).
 */
export type Fixture = {
  run: string;
  /** Platform admin access token (15 min; login is rate limited). */
  adminToken: string;
  slug: string;
  orgUuid: string;
  branches: { mrk: string; hvl: string; airport: string };
  product: { uuid: string; code: string; title: string };
  users: {
    admin: { email: string };
    mrkStaff: { email: string; uuid: string };
    fieldStaff: { email: string; uuid: string };
  };
  password: string;
  rentals: {
    hvl: Ref;
    mrk: Ref;
    handover: Ref;
    returning: Ref;
    delivery: Ref;
  };
  hvlPaymentUuid: string;
  deliveryReservation: Ref;
  deliveryTaskUuid: string;
  site: { subdomain: string; publishableKey: string };
  publicApiKey: string;
  search: { startsAt: string; endsAt: string };
};

type Ref = { uuid: string; reference: string };

const ALL_DAY = Object.fromEntries(
  ["mon", "tue", "wed", "thu", "fri", "sat", "sun"].map((d) => [
    d,
    { closed: false, open: "00:00", close: "23:59" },
  ]),
);

/** A whole hour `days` ahead (UTC). */
function at(days: number, hourUTC: number, base = new Date()): string {
  const d = new Date(
    Date.UTC(
      base.getUTCFullYear(),
      base.getUTCMonth(),
      base.getUTCDate() + days,
      hourUTC,
      0,
      0,
    ),
  );
  return d.toISOString().replace(".000Z", "Z");
}

/** Now + hours, rounded up to the next 5 minutes (UTC). */
function soon(hours: number): string {
  const d = new Date(Date.now() + hours * 3_600_000);
  d.setUTCSeconds(0, 0);
  d.setUTCMinutes(Math.ceil(d.getUTCMinutes() / 5) * 5);
  return d.toISOString().replace(".000Z", "Z");
}

type Item = { uuid: string };

/** The parts of the organization settings document the seed changes. */
type OrgSettings = {
  company: { legal_name: string } & Record<string, unknown>;
  reservation: {
    approval_mode: string;
    staff_review_window_minutes: number | null;
    payment_hold_minutes: number | null;
    quote_validity_minutes: number | null;
  } & Record<string, unknown>;
  sales_channels: { code: string; is_active: boolean }[];
} & Record<string, unknown>;

export async function seed(): Promise<Fixture> {
  const run = Date.now().toString(36);
  const slug = `kabul-${run}`;
  const password = e2eEnv.password;
  const admin = await Api.login(e2eEnv.adminEmail, password);

  // --- Organization and settings ------------------------------------------
  const created = await admin.call<{ organization: { uuid: string } }>(
    "POST",
    "/v1/platform/organizations",
    { name: `Kabul ${run}`, slug },
  );
  const orgUuid = created.organization.uuid;
  const t = admin.in(slug);

  const current = await t.call<{ version: number; settings: OrgSettings }>(
    "GET",
    "/v1/tenant/settings",
  );
  const s = current.settings;
  s.company.legal_name = `Kabul ${run} Oto Kiralama A.Ş.`;
  s.reservation.approval_mode = "auto";
  s.reservation.staff_review_window_minutes = 60;
  s.reservation.payment_hold_minutes = 60;
  s.reservation.quote_validity_minutes = 30;
  for (const ch of s.sales_channels)
    if (["web", "widget", "api", "branch"].includes(ch.code))
      ch.is_active = true;
  await t.call("PUT", "/v1/tenant/settings", {
    settings: s,
    base_version: current.version,
  });

  // --- Branches and the airport point -------------------------------------
  const branches = await t.call<{ items: (Item & { code: string })[] }>(
    "GET",
    "/v1/tenant/branches?limit=50",
  );
  const mrk = branches.items.find((b) => b.code === "MRK");
  if (!mrk) throw new Error("default branch MRK missing");
  await t.call("PATCH", `/v1/tenant/branches/${mrk.uuid}`, {
    working_hours: ALL_DAY,
    city: "İstanbul",
  });
  const hvl = await t.call<Item>("POST", "/v1/tenant/branches", {
    code: "HVL",
    name: "Havalimanı Şube",
    city: "İstanbul",
    working_hours: ALL_DAY,
    prep_minutes: 30,
  });
  const airport = await t.call<Item>(
    "POST",
    `/v1/tenant/branches/${mrk.uuid}/service-points`,
    {
      type: "airport",
      name: "SAW Havalimanı",
      extra_fee: "750",
      delivery_enabled: true,
      online_selectable: true,
      min_notice_minutes: 60,
    },
  );

  // --- Fleet: segment, group, product, plan, vehicles ----------------------
  const segment = await t.call<Item>("POST", "/v1/tenant/segments", {
    code: "EKO",
    name: "Ekonomi",
  });
  const group = await t.call<Item>("POST", "/v1/tenant/rental-groups", {
    code: "KOMPAKT",
    name: "Kompakt",
    segment_uuid: segment.uuid,
  });
  const productTitle = "Clio veya benzeri";
  const product = await t.call<Item>("POST", "/v1/tenant/products", {
    code: "KOMPAKT",
    title: productTitle,
    segment_uuid: segment.uuid,
    status: "published",
    guarantee: { type: "GROUP", rental_group_uuid: group.uuid },
    conditions: { min_driver_age: 21, min_license_years: 1 },
  });
  const plan = await t.call<Item>("POST", "/v1/tenant/rate-plans", {
    rental_product_uuid: product.uuid,
    scope: "common",
    name: "Genel tarife",
    bands: [
      { min_days: 1, max_days: 3, daily_price: "1500" },
      { min_days: 4, max_days: 7, daily_price: "1300" },
      { min_days: 8, daily_price: "1000" },
    ],
  });
  await t.call("POST", `/v1/tenant/rate-plans/${plan.uuid}/activate`, {
    change_note: "Kabul testi",
  });

  const fleet: { branch: string; brand: string; model: string; n: number }[] = [
    { branch: mrk.uuid, brand: "Renault", model: "Clio", n: 10 },
    { branch: mrk.uuid, brand: "Opel", model: "Corsa", n: 10 },
    { branch: mrk.uuid, brand: "Hyundai", model: "i20", n: 10 },
    { branch: hvl.uuid, brand: "Fiat", model: "Egea", n: 2 },
  ];
  const vehicles: { uuid: string; branch: string }[] = [];
  let plate = 0;
  for (const f of fleet) {
    for (let i = 0; i < f.n; i++) {
      plate++;
      const v = await t.call<Item>("POST", "/v1/tenant/vehicles", {
        plate: `34 KBL ${String(plate).padStart(3, "0")}`,
        manual_entry: true,
        manual_brand: f.brand,
        manual_model: f.model,
        model_year: 2024,
        transmission: "manual",
        fuel: "petrol",
        home_branch_uuid: f.branch,
      });
      await t.call("POST", `/v1/tenant/vehicles/${v.uuid}/rental-groups`, {
        rental_group_uuid: group.uuid,
      });
      vehicles.push({ uuid: v.uuid, branch: f.branch });
    }
  }
  const mrkCars = vehicles.filter((v) => v.branch === mrk.uuid);
  const hvlCars = vehicles.filter((v) => v.branch === hvl.uuid);

  // --- Customer, driver -------------------------------------------------------
  const customer = await t.call<Item>("POST", "/v1/tenant/customers", {
    customer_type: "individual",
    first_name: "Ayşe",
    last_name: "Yılmaz",
    identity_type: "tckn",
    identity_number: "10000000146",
    phone: "05321112233",
    email: `ayse.${run}@example.test`,
  });
  const driver = await t.call<Item>(
    "POST",
    `/v1/tenant/customers/${customer.uuid}/drivers`,
    {
      copy_from_customer: true,
      birth_date: "1990-04-12",
      licence_number: `B-${run}`,
      licence_class: "B",
      licence_issued_on: "2010-06-01",
    },
  );

  // --- Users and memberships ----------------------------------------------------
  const roles = await admin.call<{ items: { key: string; uuid: string }[] }>(
    "GET",
    `/v1/platform/organizations/${orgUuid}/roles`,
  );
  const role = (key: string) => {
    const r = roles.items.find((x) => x.key === key);
    if (!r) throw new Error(`role ${key} missing`);
    return r.uuid;
  };
  const user = async (local: string, name: string, roleKey: string) => {
    const email = `${local}.${run}@rentopya.test`;
    const u = await admin.call<Item>("POST", "/v1/platform/users", {
      email,
      password,
      name,
      surname: "Kabul",
      status: "active",
    });
    await admin.call("POST", `/v1/platform/organizations/${orgUuid}/members`, {
      user_uuid: u.uuid,
      role_uuids: [role(roleKey)],
      branch_scope: "selected",
      branch_uuids: [mrk.uuid],
    });
    return { email, uuid: u.uuid };
  };
  const mrkStaff = await user("mrk-yonetici", "Merve", "sube_yoneticisi");
  const fieldStaff = await user("saha", "Selim", "teslim_iade");

  // --- Reservations and rentals ---------------------------------------------------
  const reserve = async (opts: {
    pickup: string;
    startsAt: string;
    endsAt: string;
    point?: string;
    delivery?: Record<string, unknown>;
  }) => {
    const quote = await t.call<Item>("POST", "/v1/tenant/quotes", {
      pickup_branch_uuid: opts.point ? null : opts.pickup,
      return_branch_uuid: opts.point ? null : opts.pickup,
      pickup_point_uuid: opts.point ?? null,
      return_point_uuid: opts.point ?? null,
      starts_at: opts.startsAt,
      ends_at: opts.endsAt,
      channel: "branch",
      product_uuid: product.uuid,
      pickup_delivery: opts.delivery ?? null,
    });
    return t.call<Ref & { status: string }>("POST", "/v1/tenant/reservations", {
      quote_uuid: quote.uuid,
      customer_uuid: customer.uuid,
      drivers: [{ driver_uuid: driver.uuid }],
    });
  };
  const rent = async (reservation: string, vehicle: string) =>
    t.call<Ref>("POST", "/v1/tenant/rentals", {
      reservation_uuid: reservation,
      vehicle_uuid: vehicle,
    });

  // RT-A02: a rental at HVL (handed over, collected at HVL) and one at MRK.
  const rHvl = await reserve({
    pickup: hvl.uuid,
    startsAt: soon(1),
    endsAt: at(3, 7),
  });
  const hvlRental = await rent(rHvl.uuid, hvlCars[0].uuid);
  await t.call("POST", `/v1/tenant/rentals/${hvlRental.uuid}/handover`, {
    km: 1200,
    fuel: 80,
  });
  const hvlPayment = await t.call<Item>(
    "POST",
    `/v1/tenant/reservations/${rHvl.uuid}/collections`,
    { method: "cash", amount: "500", note: "Kabul HVL tahsilat" },
  );
  const rMrk = await reserve({
    pickup: mrk.uuid,
    startsAt: at(4, 7),
    endsAt: at(5, 7),
  });
  const mrkRental = await rent(rMrk.uuid, mrkCars[0].uuid);

  // RT-A17: a rental in preparation (the handover form is filled in the UI).
  const r17 = await reserve({
    pickup: mrk.uuid,
    startsAt: soon(2),
    endsAt: at(3, 9),
  });
  const handoverRental = await rent(r17.uuid, mrkCars[1].uuid);

  // RT-A19: a rental out with the customer (the return is made in the UI).
  const r19 = await reserve({
    pickup: mrk.uuid,
    startsAt: soon(1),
    endsAt: at(3, 10),
  });
  const returningRental = await rent(r19.uuid, mrkCars[2].uuid);
  await t.call("POST", `/v1/tenant/rentals/${returningRental.uuid}/handover`, {
    km: 15000,
    fuel: 90,
  });

  // RT-A29: airport delivery; the delivery task is assigned to the field
  // executor, the plate is fixed (the rental adopts the task).
  const r29 = await reserve({
    pickup: mrk.uuid,
    point: airport.uuid,
    // Tomorrow: "Görevlerim" lists today and tomorrow.
    startsAt: at(1, 9),
    endsAt: at(4, 15),
    delivery: { flight_number: "TK2412", meeting_note: "Dış hatlar, kapı 3" },
  });
  const deliveryRental = await rent(r29.uuid, mrkCars[3].uuid);
  const tasks = await t.call<{
    items: { uuid: string; task_type: string; delivery: boolean }[];
  }>("GET", `/v1/tenant/field-tasks?reservation=${r29.uuid}&limit=20`);
  const deliveryTask = tasks.items.find(
    (x) => x.task_type === "handover" && x.delivery,
  );
  if (!deliveryTask)
    throw new Error(`no delivery task: ${JSON.stringify(tasks.items)}`);
  await t.call("POST", `/v1/tenant/field-tasks/${deliveryTask.uuid}/assign`, {
    user_uuid: fieldStaff.uuid,
  });

  // --- Site, widget, public API ---------------------------------------------------
  const subdomain = `kabul${run}`;
  const saved = await t.call<{ version: number }>("PUT", "/v1/tenant/site", {
    version: 0,
    enabled: true,
    subdomain,
    default_locale: "tr",
    config: { widget: { allowed_domains: [e2eEnv.partnerOrigin] } },
  });
  await t.call("POST", "/v1/tenant/site/publish", { version: saved.version });
  const siteState = await t.call<{
    booking_enabled: boolean;
    booking_blockers: string[];
    api_key: { publishable_key: string } | null;
  }>("GET", "/v1/tenant/site");
  if (!siteState.booking_enabled || !siteState.api_key)
    throw new Error(
      `site booking off: ${JSON.stringify(siteState.booking_blockers)}`,
    );
  const publishableKey = siteState.api_key.publishable_key;
  const apiKey = await t.call<{ secret: string }>(
    "POST",
    "/v1/tenant/public-api/keys",
    {
      kind: "secret",
      name: "Kabul entegrasyon",
      channel: "api",
      scopes: ["catalog.read", "quotes.write"],
    },
  );

  return {
    run,
    adminToken: admin.accessToken,
    slug,
    orgUuid,
    branches: { mrk: mrk.uuid, hvl: hvl.uuid, airport: airport.uuid },
    product: { uuid: product.uuid, code: "KOMPAKT", title: productTitle },
    users: {
      admin: { email: e2eEnv.adminEmail },
      mrkStaff,
      fieldStaff,
    },
    password,
    rentals: {
      hvl: hvlRental,
      mrk: mrkRental,
      handover: handoverRental,
      returning: returningRental,
      delivery: deliveryRental,
    },
    hvlPaymentUuid: hvlPayment.uuid,
    deliveryReservation: r29,
    deliveryTaskUuid: deliveryTask.uuid,
    site: { subdomain, publishableKey },
    publicApiKey: apiKey.secret,
    search: { startsAt: at(20, 7), endsAt: at(23, 7) },
  };
}
