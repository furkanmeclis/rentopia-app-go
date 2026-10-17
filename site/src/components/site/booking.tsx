import { Check } from "lucide-react";

import { cn } from "@/lib/cn";
import { dateTime, money } from "@/lib/format";
import type { Ctx } from "@/lib/server";
import type { Quote } from "@/lib/types";

/** Extras → details → confirmation. */
export function Steps({ ctx, current }: { ctx: Ctx; current: 0 | 1 | 2 }) {
  const steps = [
    ctx.t("book.steps.extras"),
    ctx.t("book.steps.details"),
    ctx.t("book.steps.done"),
  ];
  return (
    <ol className="rt-steps" aria-label={ctx.t("book.steps.label")}>
      {steps.map((s, i) => (
        <li
          key={s}
          aria-current={i === current ? "step" : undefined}
          className={cn(
            "rt-step",
            i === current && "rt-step--current",
            i < current && "rt-step--done",
          )}
        >
          <span aria-hidden>
            {i < current ? <Check size={14} /> : String(i + 1)}
          </span>
          {s}
        </li>
      ))}
    </ol>
  );
}

/** Two columns on wide screens: the step's panel and the price summary. */
export function BookingGrid({ children }: { children: React.ReactNode }) {
  return <div className="rt-booking-grid">{children}</div>;
}

/** The price snapshot the customer books (quote lines from the API). */
export function QuoteSummary({ ctx, quote }: { ctx: Ctx; quote: Quote }) {
  const { t, locale } = ctx;
  const cur = quote.currency;
  return (
    <aside className="rt-summary" aria-labelledby="quote-summary-title">
      <span className="rt-kicker">{t("book.summary")}</span>
      <h2 id="quote-summary-title">{quote.subgroup.title}</h2>
      <p>{t("results.days", { days: quote.billing_days })}</p>
      <dl>
        <div>
          <dt>{t("book.pickup")}</dt>
          <dd>
            {quote.pickup_point?.name
              ? `${quote.pickup_branch.name} — ${quote.pickup_point.name}`
              : quote.pickup_branch.name}
            <br />
            {dateTime(quote.starts_at, locale)}
          </dd>
        </div>
        <div>
          <dt>{t("book.return")}</dt>
          <dd>
            {quote.return_point?.name
              ? `${quote.return_branch.name} — ${quote.return_point.name}`
              : quote.return_branch.name}
            <br />
            {dateTime(quote.ends_at, locale)}
          </dd>
        </div>
      </dl>
      <ul className="rt-summary-lines">
        <li>
          <span>
            {quote.subgroup.title} ·{" "}
            {t("results.days", { days: quote.billing_days })}
          </span>
          <span className="rt-num">
            {money(quote.rental_total, cur, locale)}
          </span>
        </li>
        {quote.lines
          .filter((l) => l.kind !== "day")
          .map((l, i) => (
            <li key={`${l.code}-${i}`}>
              <span>
                {l.name}
                {l.quantity > 1 ? ` × ${l.quantity}` : ""}
              </span>
              <span className="rt-num">{money(l.amount, cur, locale)}</span>
            </li>
          ))}
      </ul>
      <div className="rt-summary-total">
        <p>
          <span>{t("book.total")}</span>
          <b className="rt-num">{money(quote.total, cur, locale)}</b>
        </p>
        <small>{t("book.vat_included")}</small>
        {Number(quote.prepayment) > 0 ? (
          <p className="rt-summary-prepay">
            <span>{t("book.prepayment_now")}</span>
            <span className="rt-num">
              {money(quote.prepayment, cur, locale)}
            </span>
          </p>
        ) : null}
      </div>
    </aside>
  );
}
