import {
  EmptyState,
  PageHeading,
  PageSection,
} from "@/components/site/page-heading";
import type { Ctx } from "@/lib/server";

import type { ContentPage, Query } from "../index";
import { Content as TechnowideContent } from "../technowide/pages";
import { About, Faq } from "./sections";

/**
 * Horizon's own about and FAQ pages (the studio's editorial layouts: the
 * dark story band and the ruled question list). Contact and the legal
 * pages use the shared markup, drawn in Horizon's typography by its
 * stylesheet. Vehicles, offices, campaigns and the application forms are
 * the shared pages too (see index.tsx).
 */
export function Content({
  ctx,
  page,
  query,
}: {
  ctx: Ctx;
  page: ContentPage;
  query?: Query;
}) {
  const { t, content } = ctx;
  if (page === "faq") {
    return (
      <>
        <PageHeading ctx={ctx} title={t("page.faq")} lead={t("tw.faq.lead")} />
        {content.faq.length ? (
          <Faq ctx={ctx} items={content.faq} />
        ) : (
          <PageSection>
            <EmptyState title={t("page.empty")} />
          </PageSection>
        )}
      </>
    );
  }
  if (page === "about") {
    return (
      <>
        <PageHeading ctx={ctx} title={t("page.about")} />
        {content.about ? (
          <>
            <About ctx={ctx} full />
            <Faq
              ctx={ctx}
              items={content.faq.slice(0, 6)}
              more={content.faq.length > 6}
            />
          </>
        ) : (
          <PageSection>
            <EmptyState title={t("page.empty")} />
          </PageSection>
        )}
      </>
    );
  }
  return <TechnowideContent ctx={ctx} page={page} query={query} />;
}
