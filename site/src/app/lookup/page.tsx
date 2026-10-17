import type { Metadata } from "next";
import Link from "next/link";

import { Container, PageTitle } from "@/components/site/text";
import { buttonVariants } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { deviceReservations } from "@/lib/reservation-tokens";
import { getCtx } from "@/lib/server";

export async function generateMetadata(): Promise<Metadata> {
  const { t } = await getCtx();
  return { title: t("lookup.title") };
}

/** Reservation lookup: the customer portal (M3), plus this device's bookings. */
export default async function LookupPage() {
  const { t, site } = await getCtx();
  const mine = await deviceReservations(site);
  return (
    <Container className="max-w-2xl space-y-6 py-10">
      <PageTitle>{t("lookup.title")}</PageTitle>
      <p className="text-muted-foreground leading-relaxed">
        {t("lookup.intro")}
      </p>
      {site.portal_url ? (
        <a href={site.portal_url} className={buttonVariants({ size: "lg" })}>
          {t("lookup.go")}
        </a>
      ) : null}
      {mine.length ? (
        <Card>
          <CardTitle>{t("lookup.device")}</CardTitle>
          <ul className="space-y-2">
            {mine.map((ref) => (
              <li key={ref}>
                <Link
                  href={`/reservation/${encodeURIComponent(ref)}`}
                  className="font-mono underline"
                >
                  {ref}
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      ) : null}
    </Container>
  );
}
