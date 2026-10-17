import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { pay } from "@/app/actions";
import { Steps } from "@/components/site/booking";
import { Container, PageTitle, Paragraphs } from "@/components/site/text";
import { Button, buttonVariants } from "@/components/ui/button";
import { Alert, Badge, Card, CardTitle } from "@/components/ui/card";
import { ApiError } from "@/lib/api";
import { dateTime, money } from "@/lib/format";
import { reservationToken } from "@/lib/reservation-tokens";
import { bookingApi, getCtx } from "@/lib/server";
import type { Reservation } from "@/lib/types";
import type { MessageKey } from "@/messages/tr";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("res.title"), robots: { index: false } };
}

const STATUSES = [
  "request",
  "awaiting_review",
  "awaiting_payment",
  "confirmed",
  "cancelled",
  "expired",
  "no_show",
];
const PAYMENTS = ["unpaid", "partially_paid", "paid"];

export default async function ReservationPage({
  params,
  searchParams,
}: {
  params: Promise<{ reference: string }>;
  searchParams: Promise<{ new?: string; pay_error?: string }>;
}) {
  const ctx = await getCtx();
  const { t, locale, site } = ctx;
  const { reference: raw } = await params;
  const reference = decodeURIComponent(raw);
  if (!/^[A-Za-z0-9-]{1,40}$/.test(reference)) notFound();
  const sp = await searchParams;
  const token = await reservationToken(site, reference);

  const portal = site.portal_url ? (
    <a
      href={site.portal_url}
      className={buttonVariants({ variant: "outline" })}
    >
      {t("res.portal_link")}
    </a>
  ) : null;

  if (!token) {
    return (
      <Container className="max-w-2xl space-y-6 py-10">
        <PageTitle>{t("res.title")}</PageTitle>
        <Alert>{t("res.no_access")}</Alert>
        {portal}
      </Container>
    );
  }

  let r: Reservation;
  try {
    r = await bookingApi<Reservation>(
      `/v1/public/reservations/${encodeURIComponent(reference)}`,
      {
        reservationToken: token,
      },
    );
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) notFound();
    throw err;
  }
  const cur = r.price.currency;
  const statusKey = (
    STATUSES.includes(r.status) ? `res.status.${r.status}` : "res.status.other"
  ) as MessageKey;
  const payKey = (
    PAYMENTS.includes(r.payment_status)
      ? `res.payment.${r.payment_status}`
      : "res.payment.other"
  ) as MessageKey;

  return (
    <Container className="space-y-6 py-8">
      <Steps ctx={ctx} current={2} />
      <PageTitle>{t("res.title")}</PageTitle>
      {ctx.embed ? (
        <span
          data-widget-reference={r.reference}
          data-widget-status={r.status}
        />
      ) : null}
      {sp.new ? (
        <Alert tone="success">
          {t("res.thanks", { reference: r.reference })}
        </Alert>
      ) : null}
      {sp.new && r.status === "request" ? (
        <Alert>{t("res.request_sent")}</Alert>
      ) : null}
      {sp.pay_error ? <Alert tone="error">{t("res.pay_failed")}</Alert> : null}

      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        <Card>
          <div className="flex flex-wrap items-center gap-3">
            <CardTitle>
              {t("res.reference")}:{" "}
              <span className="font-mono">{r.reference}</span>
            </CardTitle>
            <Badge>{t(statusKey)}</Badge>
            <Badge>
              {t("res.payment_status")}: {t(payKey)}
            </Badge>
          </div>
          <dl className="grid gap-4 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">{t("book.pickup")}</dt>
              <dd>
                {r.pickup_point
                  ? `${r.pickup_branch} — ${r.pickup_point}`
                  : r.pickup_branch}
                <br />
                {dateTime(r.starts_at, locale)}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t("book.return")}</dt>
              <dd>
                {r.return_point
                  ? `${r.return_branch} — ${r.return_point}`
                  : r.return_branch}
                <br />
                {dateTime(r.ends_at, locale)}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t("res.vehicle")}</dt>
              <dd>{r.vehicle || r.subgroup}</dd>
            </div>
            {r.drivers.length ? (
              <div>
                <dt className="text-muted-foreground">{t("res.drivers")}</dt>
                <dd>{r.drivers.join(", ")}</dd>
              </div>
            ) : null}
            {r.extras.length ? (
              <div className="sm:col-span-2">
                <dt className="text-muted-foreground">{t("res.extras")}</dt>
                <dd>
                  <ul>
                    {r.extras.map((x, i) => (
                      <li key={i}>
                        {x.name}
                        {x.quantity > 1 ? ` × ${x.quantity}` : ""} —{" "}
                        {money(x.amount, cur, locale)}
                      </li>
                    ))}
                  </ul>
                </dd>
              </div>
            ) : null}
          </dl>
          {r.policies.cancellation ? (
            <section className="border-border space-y-2 border-t pt-4">
              <h2 className="font-semibold">{t("res.cancellation")}</h2>
              <Paragraphs
                text={r.policies.cancellation}
                className="text-muted-foreground text-sm"
              />
            </section>
          ) : null}
          <div className="border-border flex flex-wrap items-center gap-3 border-t pt-4">
            <p className="text-muted-foreground w-full text-sm">
              {t("res.portal")}
            </p>
            {portal}
          </div>
        </Card>

        <Card className="h-fit gap-3">
          <CardTitle>{t("book.summary")}</CardTitle>
          <dl className="space-y-1 text-sm">
            <div className="flex justify-between">
              <dt>{t("res.total")}</dt>
              <dd className="font-semibold tabular-nums">
                {money(r.price.total, cur, locale)}
              </dd>
            </div>
            <div className="flex justify-between">
              <dt>{t("res.paid")}</dt>
              <dd className="tabular-nums">
                {money(r.price.paid, cur, locale)}
              </dd>
            </div>
            {Number(r.price.prepayment_due) > 0 ? (
              <div className="flex justify-between">
                <dt>{t("res.due")}</dt>
                <dd className="tabular-nums">
                  {money(r.price.prepayment_due, cur, locale)}
                </dd>
              </div>
            ) : null}
            <div className="flex justify-between">
              <dt>{t("res.balance")}</dt>
              <dd className="tabular-nums">
                {money(r.price.balance, cur, locale)}
              </dd>
            </div>
          </dl>
          {r.payment_deadline ? (
            <p className="text-sm">
              {t("res.deadline", {
                date: dateTime(r.payment_deadline, locale),
              })}
            </p>
          ) : null}
          {r.actions.pay ? (
            <form action={pay}>
              <input type="hidden" name="reference" value={r.reference} />
              <Button type="submit" size="lg" className="w-full">
                {t("res.pay")}
                {r.actions.pay_amount
                  ? ` · ${money(r.actions.pay_amount, cur, locale)}`
                  : ""}
              </Button>
            </form>
          ) : null}
        </Card>
      </div>
    </Container>
  );
}
