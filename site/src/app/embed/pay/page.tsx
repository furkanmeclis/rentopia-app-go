import { notFound } from "next/navigation";

import { Container, PageTitle } from "@/components/site/text";
import { buttonVariants } from "@/components/ui/button";
import { openEmbed } from "@/lib/embed";
import { embedContext } from "@/lib/embed-server";
import { paymentHandoff } from "@/lib/payment-handoff";
import { getCtx } from "@/lib/server";

export default async function EmbedPayPage() {
  const { site, t } = await getCtx();
  const embed = await embedContext();
  const state = openEmbed(embed?.fields.s || null, site.origin);
  if (!state?.payment || !state.reservation) notFound();
  const payment = new URL(state.payment);
  if (
    payment.protocol !== "https:" &&
    !(payment.protocol === "http:" && payment.hostname.endsWith("localhost"))
  )
    notFound();
  // The return reference is encrypted and can only be read on this site's origin.
  const back = new URL(
    `/reservation/${encodeURIComponent(state.reservation.reference)}`,
    site.origin,
  );
  back.searchParams.set("s", embed!.fields.s);
  const token = /^\/pay\/([A-Za-z0-9_-]{20,64})$/.exec(payment.pathname)?.[1];
  if (!token) notFound();
  payment.pathname += "/widget";
  payment.search = "";
  payment.searchParams.set(
    "handoff",
    paymentHandoff({
      token,
      url: back.toString(),
      organization: site.organization.uuid,
      expires: state.expires,
    }),
  );
  return (
    <Container className="space-y-4 py-6">
      <PageTitle>{t("res.pay")}</PageTitle>
      <p>{t("widget.payment_outside")}</p>
      <a
        data-widget-payment
        href={payment.toString()}
        target="_top"
        rel="noreferrer"
        className={buttonVariants()}
      >
        {t("res.pay")}
      </a>
      <a
        href={payment.toString()}
        target="_blank"
        rel="noopener noreferrer"
        className={buttonVariants({ variant: "outline" })}
      >
        {t("widget.new_tab")}
      </a>
    </Container>
  );
}
