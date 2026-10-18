import { ShieldCheck } from "lucide-react";

import {
  EmptyState,
  PageHeading,
  PageSection,
} from "@/components/site/page-heading";
import { Alert } from "@/components/ui/card";
import { loadSubgroups } from "@/lib/catalog";
import type { Ctx } from "@/lib/server";

import type { ContentPage, Query } from "../index";
import { Content as TechnowideContent } from "../technowide/pages";
import { About, Advantages, Faq, SegmentChips, VehicleGrid } from "./sections";

/**
 * /vehicles in classic: the live fleet as classic cards in the
 * organization's `fleetColumns`, filterable by segment through links.
 */
export async function Vehicles({
  ctx,
  segment,
}: {
  ctx: Ctx;
  segment?: string;
}) {
  const { t, site } = ctx;
  const subgroups = site.booking_enabled ? await loadSubgroups() : null;
  const active =
    segment && subgroups?.some((s) => s.segment === segment)
      ? segment
      : undefined;
  const list = (subgroups ?? []).filter((s) => !active || s.segment === active);
  return (
    <>
      <PageHeading
        ctx={ctx}
        title={t("fleet.title")}
        lead={t("tw.fleet.lead")}
      />
      <PageSection>
        {!subgroups ? (
          <Alert>{t("search.booking_off")}</Alert>
        ) : !subgroups.length ? (
          <EmptyState title={t("fleet.empty")} />
        ) : (
          <>
            <div className="cl-fleet-toolbar">
              <SegmentChips ctx={ctx} subgroups={subgroups} active={active} />
              <p className="tw-result-count" role="status">
                {t("fleet.count", { count: list.length })}
              </p>
            </div>
            <VehicleGrid ctx={ctx} subgroups={list} heading="h2" />
            <div className="rt-notice">
              <ShieldCheck aria-hidden size={19} />
              <p>{t("fleet.notice")}</p>
            </div>
          </>
        )}
      </PageSection>
    </>
  );
}

/**
 * classic's own about (story, advantages, questions: the studio's
 * /hakkimizda) and FAQ pages; contact and the legal pages use the shared
 * markup, drawn by classic.css.
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
        <PageHeading ctx={ctx} title={t("page.faq")} lead={t("cl.faq.lead")} />
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
            <Advantages ctx={ctx} />
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
