import type { Metadata } from "next";
import Link from "next/link";

import { applyExtras } from "@/app/actions";
import { BookingGrid, QuoteSummary, Steps } from "@/components/site/booking";
import { PageHeading, PageSection } from "@/components/site/page-heading";
import { Button, buttonVariants } from "@/components/ui/button";
import { Alert, Card } from "@/components/ui/card";
import { Checkbox, Label, Select } from "@/components/ui/form";
import { embedPath } from "@/lib/embed";
import { loadQuote } from "@/lib/quote";
import { bookingApi, getCtx } from "@/lib/server";
import type { Extra } from "@/lib/types";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("book.extras_title"), robots: { index: false } };
}

export default async function ExtrasPage({
  params,
  searchParams,
}: {
  params: Promise<{ quote: string }>;
  searchParams: Promise<{ error?: string }>;
}) {
  const ctx = await getCtx();
  const { t } = ctx;
  const { quote: id } = await params;
  const { error } = await searchParams;
  const quote = await loadQuote(id);
  const extras = await bookingApi<Extra[]>("/v1/public/extras").catch(
    () => [] as Extra[],
  );
  const chosen = new Map(quote.extras.map((e) => [e.extra_uuid, e.quantity]));

  return (
    <>
      <PageHeading ctx={ctx} title={t("book.extras_title")} />
      <PageSection>
        <Steps ctx={ctx} current={0} />
        {quote.expired ? (
          <Alert tone="error">{t("book.quote_expired")}</Alert>
        ) : null}
        {error ? <Alert tone="error">{t("book.failed")}</Alert> : null}
        <BookingGrid>
          <Card className="rt-panel">
            {extras.length === 0 ? (
              <p className="rt-muted">{t("book.extras_none")}</p>
            ) : (
              <form action={applyExtras} className="rt-stack">
                <input type="hidden" name="quote_uuid" value={quote.uuid} />
                <fieldset className="rt-stack">
                  <legend className="sr-only">{t("book.extras_title")}</legend>
                  {extras.map((x) => (
                    <div key={x.uuid} className="rt-extra-option">
                      <Checkbox
                        id={`extra-${x.uuid}`}
                        name="extra"
                        value={x.uuid}
                        defaultChecked={chosen.has(x.uuid)}
                      />
                      <div className="rt-extra-body">
                        <Label htmlFor={`extra-${x.uuid}`}>{x.name}</Label>
                        {x.description ? <p>{x.description}</p> : null}
                        <p className="rt-extra-charge">
                          {x.charge_type === "per_day"
                            ? t("book.charge.per_day")
                            : t("book.charge.per_rental")}
                        </p>
                      </div>
                      {x.max_quantity > 1 ? (
                        <div className="rt-extra-qty">
                          <Label htmlFor={`qty-${x.uuid}`}>
                            {t("book.quantity")}
                          </Label>
                          <Select
                            id={`qty-${x.uuid}`}
                            name={`qty_${x.uuid}`}
                            defaultValue={String(
                              chosen.get(x.uuid) ?? Math.max(1, x.min_quantity),
                            )}
                          >
                            {Array.from(
                              { length: Math.min(10, x.max_quantity) },
                              (_, i) => i + 1,
                            ).map((n) => (
                              <option key={n} value={n}>
                                {n}
                              </option>
                            ))}
                          </Select>
                        </div>
                      ) : null}
                    </div>
                  ))}
                </fieldset>
                <div className="rt-actions">
                  <Button type="submit" size="lg">
                    {t("book.extras_continue")}
                  </Button>
                  <Link
                    href={embedPath(`/book/${quote.uuid}/details`, ctx.embed)}
                    className={buttonVariants({
                      variant: "outline",
                      size: "lg",
                    })}
                  >
                    {t("book.extras_skip")}
                  </Link>
                </div>
              </form>
            )}
            {extras.length === 0 ? (
              <Link
                href={embedPath(`/book/${quote.uuid}/details`, ctx.embed)}
                className={buttonVariants({ size: "lg" })}
              >
                {t("book.extras_continue")}
              </Link>
            ) : null}
          </Card>
          <QuoteSummary ctx={ctx} quote={quote} />
        </BookingGrid>
      </PageSection>
    </>
  );
}
