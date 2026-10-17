import { ArrowRight, Search } from "lucide-react";
import Link from "next/link";

import { EmptyState, PageSection } from "@/components/site/page-heading";
import { buttonVariants } from "@/components/ui/button";
import { getCtx } from "@/lib/server";

export default async function NotFound() {
  const { t } = await getCtx();
  return (
    <PageSection narrow>
      <EmptyState
        icon={<Search aria-hidden size={32} />}
        title={t("notfound.title")}
        text={t("notfound.body")}
        action={
          <Link href="/" className={buttonVariants()}>
            {t("notfound.home")}
            <ArrowRight aria-hidden size={17} />
          </Link>
        }
      />
    </PageSection>
  );
}
