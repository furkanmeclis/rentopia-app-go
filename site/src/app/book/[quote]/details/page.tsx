import { randomUUID } from "node:crypto";

import type { Metadata } from "next";
import Link from "next/link";

import { BookingGrid, QuoteSummary, Steps } from "@/components/site/booking";
import { PageHeading, PageSection } from "@/components/site/page-heading";
import { buttonVariants } from "@/components/ui/button";
import { Alert, Card } from "@/components/ui/card";
import { embedPath } from "@/lib/embed";
import { loadQuote } from "@/lib/quote";
import { getCtx } from "@/lib/server";
import type { MessageKey } from "@/messages/tr";

import { BookForm } from "./book-form";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("book.steps.details"), robots: { index: false } };
}

const LABELS: MessageKey[] = [
  "preview.booking_disabled",
  "book.renter",
  "book.type",
  "book.type.individual",
  "book.type.corporate",
  "book.first_name",
  "book.last_name",
  "book.company_title",
  "book.tax_office",
  "book.phone",
  "book.email",
  "book.driver",
  "book.driver_hint",
  "book.driver_first_name",
  "book.driver_last_name",
  "book.birth_date",
  "book.licence_number",
  "book.licence_issued_on",
  "book.note",
  "book.accept_terms",
  "book.accept_privacy",
  "book.submit",
  "book.submitting",
  "book.required",
  "book.invalid",
  "book.fix_errors",
  "book.driver_not_eligible",
  "book.capacity",
  "book.failed",
  "book.quote_expired",
  "book.terms_required",
  "footer.terms",
  "footer.privacy",
];

export default async function DetailsPage({
  params,
  searchParams,
}: {
  params: Promise<{ quote: string }>;
  searchParams: Promise<{ changed?: string }>;
}) {
  const ctx = await getCtx();
  const { t } = ctx;
  const { quote: id } = await params;
  const { changed } = await searchParams;
  const quote = await loadQuote(id);
  const labels = Object.fromEntries(LABELS.map((k) => [k, t(k)]));

  return (
    <>
      <PageHeading ctx={ctx} title={t("book.steps.details")} />
      <PageSection>
        <Steps ctx={ctx} current={1} />
        {changed ? <Alert tone="error">{t("book.price_changed")}</Alert> : null}
        <BookingGrid>
          <Card className="rt-panel">
            {quote.expired || quote.status !== "open" ? (
              <div className="rt-stack">
                <Alert tone="error">{t("book.quote_expired")}</Alert>
                <Link
                  href={embedPath("/", ctx.embed)}
                  className={buttonVariants()}
                >
                  {t("common.new_search")}
                </Link>
              </div>
            ) : (
              <BookForm
                preview={Boolean(ctx.site.preview)}
                quoteUuid={quote.uuid}
                idempotencyKey={randomUUID()}
                labels={labels}
              />
            )}
          </Card>
          <div className="rt-stack">
            <QuoteSummary ctx={ctx} quote={quote} />
            <Link
              href={embedPath(`/book/${quote.uuid}`, ctx.embed)}
              className={buttonVariants({ variant: "link" })}
            >
              ← {t("book.extras_title")}
            </Link>
          </div>
        </BookingGrid>
      </PageSection>
    </>
  );
}
