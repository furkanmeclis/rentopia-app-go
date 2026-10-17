import { Check } from "lucide-react";

import { Card, CardTitle } from "@/components/ui/card";
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
    <ol
      className="flex flex-wrap items-center gap-2 text-sm"
      aria-label="steps"
    >
      {steps.map((s, i) => (
        <li
          key={s}
          aria-current={i === current ? "step" : undefined}
          className={cn(
            "flex items-center gap-2 rounded-full px-3 py-1",
            i === current
              ? "bg-primary text-primary-foreground"
              : "bg-muted text-muted-foreground",
          )}
        >
          {i < current ? (
            <Check className="size-4" aria-hidden />
          ) : (
            <span aria-hidden>{i + 1}</span>
          )}
          {s}
        </li>
      ))}
    </ol>
  );
}

/** The price snapshot the customer books (quote lines from the API). */
export function QuoteSummary({ ctx, quote }: { ctx: Ctx; quote: Quote }) {
  const { t, locale } = ctx;
  const cur = quote.currency;
  return (
    <Card className="gap-3">
      <CardTitle>{t("book.summary")}</CardTitle>
      <dl className="grid gap-2 text-sm">
        <div>
          <dt className="text-muted-foreground">{t("book.vehicle")}</dt>
          <dd className="font-medium">{quote.subgroup.title}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("book.pickup")}</dt>
          <dd>
            {quote.pickup_point?.name
              ? `${quote.pickup_branch.name} — ${quote.pickup_point.name}`
              : quote.pickup_branch.name}
            <br />
            {dateTime(quote.starts_at, locale)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("book.return")}</dt>
          <dd>
            {quote.return_point?.name
              ? `${quote.return_branch.name} — ${quote.return_point.name}`
              : quote.return_branch.name}
            <br />
            {dateTime(quote.ends_at, locale)}
          </dd>
        </div>
      </dl>
      <ul className="border-border space-y-1 border-t pt-3 text-sm">
        <li className="flex justify-between gap-3">
          <span>
            {quote.subgroup.title} ·{" "}
            {t("results.days", { days: quote.billing_days })}
          </span>
          <span className="tabular-nums">
            {money(quote.rental_total, cur, locale)}
          </span>
        </li>
        {quote.lines
          .filter((l) => l.kind !== "day")
          .map((l, i) => (
            <li key={`${l.code}-${i}`} className="flex justify-between gap-3">
              <span>
                {l.name}
                {l.quantity > 1 ? ` × ${l.quantity}` : ""}
              </span>
              <span className="tabular-nums">
                {money(l.amount, cur, locale)}
              </span>
            </li>
          ))}
      </ul>
      <div className="border-border space-y-1 border-t pt-3">
        <p className="flex justify-between text-lg font-bold">
          <span>{t("book.total")}</span>
          <span className="tabular-nums">
            {money(quote.total, cur, locale)}
          </span>
        </p>
        <p className="text-muted-foreground text-xs">
          {t("book.vat_included")}
        </p>
        {Number(quote.prepayment) > 0 ? (
          <p className="flex justify-between text-sm">
            <span>{t("book.prepayment_now")}</span>
            <span className="tabular-nums">
              {money(quote.prepayment, cur, locale)}
            </span>
          </p>
        ) : null}
      </div>
    </Card>
  );
}
