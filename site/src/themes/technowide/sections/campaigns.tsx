import { ArrowRight, ArrowUpRight, TicketPercent } from "lucide-react";
import Link from "next/link";

import {
  campaignCards,
  campaignDate,
  campaignValidity,
  type CampaignCard,
} from "@/lib/campaigns";
import { istanbulDate } from "@/lib/format";
import type { Ctx } from "@/lib/server";

import { SectionHeading } from "../parts";

/** Cards on the home page; the rest are on /campaigns. */
const HOME_LIMIT = 6;

/** The card's validity line ("31 Eki 2026 tarihine kadar geçerli"). */
export function validityText(ctx: Ctx, card: CampaignCard): string {
  const { t, locale } = ctx;
  const v = campaignValidity(card, istanbulDate());
  switch (v.kind) {
    case "until":
      return t("tw.campaigns.until", { date: campaignDate(v.end!, locale) });
    case "from":
      return t("tw.campaigns.from", { date: campaignDate(v.start!, locale) });
    case "between":
      return t("tw.campaigns.between", {
        start: campaignDate(v.start!, locale),
        end: campaignDate(v.end!, locale),
      });
    default:
      return t("tw.campaigns.open");
  }
}

/** Coupon code chip (public coupons only; the API omits private codes). */
export function CampaignCode({ ctx, code }: { ctx: Ctx; code: string }) {
  return (
    <p className="tw-campaign-code">
      <TicketPercent aria-hidden size={17} />
      <span>{ctx.t("tw.campaigns.code")}</span>
      <code>{code}</code>
    </p>
  );
}

/** The photo of a campaign, or the brand-token gradient. */
export function CampaignVisual({
  ctx,
  card,
  label = true,
}: {
  ctx: Ctx;
  card: CampaignCard;
  label?: boolean;
}) {
  return (
    <div
      className={
        card.image
          ? "tw-campaign-visual"
          : "tw-campaign-visual tw-campaign-visual--plain"
      }
    >
      {card.image ? (
        // Library media comes from the API through /media (same origin).
        // eslint-disable-next-line @next/next/no-img-element
        <img src={card.image} alt="" loading="lazy" />
      ) : null}
      {label ? (
        <span className="tw-campaign-visual-label" aria-hidden>
          <ArrowUpRight size={30} />
          {ctx.site.organization.name}
        </span>
      ) : null}
    </div>
  );
}

/** One campaign card (home track and /campaigns list). */
export function CampaignCardView({
  ctx,
  card,
  heading = "h3",
  eyebrow,
}: {
  ctx: Ctx;
  card: CampaignCard;
  heading?: "h2" | "h3";
  eyebrow?: string;
}) {
  const { t } = ctx;
  const Title = heading;
  const title = card.title || t("tw.campaigns.default_title");
  return (
    <article className="tw-campaign-card">
      <div className="tw-campaign-copy">
        <span className="tw-mini-label">
          {eyebrow ? `${eyebrow} · ` : ""}
          {validityText(ctx, card)}
        </span>
        <Title>{title}</Title>
        {card.summary ? <p>{card.summary}</p> : null}
        {card.code ? <CampaignCode ctx={ctx} code={card.code} /> : null}
        <Link
          href={`/campaigns/${card.uuid}`}
          className="rt-button rt-button--primary tw-campaign-cta"
        >
          {card.cta || t("tw.campaigns.cta")}
          <ArrowUpRight aria-hidden size={18} />
          <span className="rt-sr-only">: {title}</span>
        </Link>
      </div>
      <CampaignVisual ctx={ctx} card={card} />
    </article>
  );
}

/**
 * Home campaigns: the organization's live campaigns as a scroll-snap track
 * (every card is in the HTML; the dots are plain anchors), hidden when
 * there is none (lib/sections.ts drops it before layout).
 */
export function Campaigns({ ctx }: { ctx: Ctx }) {
  const { t } = ctx;
  const all = campaignCards(ctx.site, ctx.locale);
  if (!all.length) return null;
  const cards = all.slice(0, HOME_LIMIT);
  const many = cards.length > 1;
  return (
    <section
      className="tw-section tw-campaigns"
      aria-labelledby="campaigns-title"
    >
      <div className="rt-container">
        <SectionHeading
          id="campaigns-title"
          eyebrow={t("tw.campaigns.eyebrow")}
          title={t("tw.campaigns.title")}
          lead={t("tw.campaigns.lead")}
          action={
            <Link href="/campaigns" className="tw-text-button">
              {t("tw.campaigns.all")}
              <ArrowRight aria-hidden size={17} />
            </Link>
          }
        />
        <div
          className="tw-campaign-track"
          tabIndex={many ? 0 : undefined}
          role={many ? "region" : undefined}
          aria-roledescription={many ? t("tw.campaigns.carousel") : undefined}
          aria-label={many ? t("tw.campaigns.list") : undefined}
        >
          {cards.map((card, i) => (
            <div
              key={card.uuid}
              id={many ? `campaign-slide-${i + 1}` : undefined}
              className="tw-campaign-slide"
              role={many ? "group" : undefined}
              aria-roledescription={many ? t("tw.campaigns.list") : undefined}
              aria-label={
                many
                  ? t("tw.campaigns.position", {
                      n: i + 1,
                      total: cards.length,
                    })
                  : undefined
              }
            >
              <CampaignCardView ctx={ctx} card={card} />
            </div>
          ))}
        </div>
        {many ? (
          <nav
            className="tw-progress-dots"
            aria-label={t("tw.campaigns.carousel")}
          >
            {cards.map((card, i) => (
              <a
                key={card.uuid}
                href={`#campaign-slide-${i + 1}`}
                aria-label={t("tw.campaigns.position", {
                  n: i + 1,
                  total: cards.length,
                })}
              />
            ))}
          </nav>
        ) : null}
      </div>
    </section>
  );
}
