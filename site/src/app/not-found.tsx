import Link from "next/link";

import { Container, PageTitle } from "@/components/site/text";
import { buttonVariants } from "@/components/ui/button";
import { getCtx } from "@/lib/server";

export default async function NotFound() {
  const { t } = await getCtx();
  return (
    <Container className="max-w-xl space-y-4 py-20 text-center">
      <PageTitle>{t("notfound.title")}</PageTitle>
      <p className="text-muted-foreground">{t("notfound.body")}</p>
      <Link href="/" className={buttonVariants()}>
        {t("notfound.home")}
      </Link>
    </Container>
  );
}
