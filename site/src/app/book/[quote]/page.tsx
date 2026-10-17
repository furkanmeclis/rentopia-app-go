import type { Metadata } from "next";
import Link from "next/link";

import { applyExtras } from "@/app/actions";
import { QuoteSummary, Steps } from "@/components/site/booking";
import { Container, PageTitle } from "@/components/site/text";
import { Button, buttonVariants } from "@/components/ui/button";
import { Alert, Card } from "@/components/ui/card";
import { Checkbox, Label, Select } from "@/components/ui/form";
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
    <Container className="space-y-6 py-8">
      <Steps ctx={ctx} current={0} />
      <PageTitle>{t("book.extras_title")}</PageTitle>
      {quote.expired ? (
        <Alert tone="error">{t("book.quote_expired")}</Alert>
      ) : null}
      {error ? <Alert tone="error">{t("book.failed")}</Alert> : null}
      <div className="grid gap-6 lg:grid-cols-[1fr_360px]">
        <Card>
          {extras.length === 0 ? (
            <p className="text-muted-foreground">{t("book.extras_none")}</p>
          ) : (
            <form action={applyExtras} className="space-y-4">
              <input type="hidden" name="quote_uuid" value={quote.uuid} />
              <fieldset className="space-y-3">
                <legend className="sr-only">{t("book.extras_title")}</legend>
                {extras.map((x) => (
                  <div
                    key={x.uuid}
                    className="border-border flex flex-wrap items-start gap-3 rounded-[var(--radius)] border p-3"
                  >
                    <Checkbox
                      id={`extra-${x.uuid}`}
                      name="extra"
                      value={x.uuid}
                      defaultChecked={chosen.has(x.uuid)}
                      className="mt-1"
                    />
                    <div className="min-w-0 flex-1 space-y-1">
                      <Label htmlFor={`extra-${x.uuid}`}>{x.name}</Label>
                      {x.description ? (
                        <p className="text-muted-foreground text-sm">
                          {x.description}
                        </p>
                      ) : null}
                      <p className="text-muted-foreground text-xs">
                        {x.charge_type === "per_day"
                          ? t("book.charge.per_day")
                          : t("book.charge.per_rental")}
                      </p>
                    </div>
                    {x.max_quantity > 1 ? (
                      <div className="grid gap-1">
                        <Label htmlFor={`qty-${x.uuid}`} className="text-xs">
                          {t("book.quantity")}
                        </Label>
                        <Select
                          id={`qty-${x.uuid}`}
                          name={`qty_${x.uuid}`}
                          defaultValue={String(
                            chosen.get(x.uuid) ?? Math.max(1, x.min_quantity),
                          )}
                          className="w-20"
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
              <div className="flex flex-wrap gap-3">
                <Button type="submit" size="lg">
                  {t("book.extras_continue")}
                </Button>
                <Link
                  href={`/book/${quote.uuid}/details`}
                  className={buttonVariants({ variant: "outline", size: "lg" })}
                >
                  {t("book.extras_skip")}
                </Link>
              </div>
            </form>
          )}
          {extras.length === 0 ? (
            <Link
              href={`/book/${quote.uuid}/details`}
              className={buttonVariants({ size: "lg" })}
            >
              {t("book.extras_continue")}
            </Link>
          ) : null}
        </Card>
        <QuoteSummary ctx={ctx} quote={quote} />
      </div>
    </Container>
  );
}
