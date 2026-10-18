import { ArrowRight, ArrowUpRight, CarFront, ChevronDown } from "lucide-react";
import Link from "next/link";

import { SearchForm } from "@/components/site/search-form";
import { Paragraphs } from "@/components/site/text";
import { Alert } from "@/components/ui/card";
import { campaignCards, campaignDate } from "@/lib/campaigns";
import { cn } from "@/lib/cn";
import { localized } from "@/lib/links";
import { siteMediaUrl } from "@/lib/media";
import { mediaUrl, type Ctx } from "@/lib/server";
import type { Location, Subgroup } from "@/lib/types";

import { segmentsOf, vehicleFacts } from "../facts";
import { guideCards } from "../guide";
import { hasContactPage } from "../horizon/chrome";
import { fleetColumns } from "../registry";
import { CampaignCode, validityText } from "../technowide/sections/campaigns";
import { heroSlides } from "../technowide/sections/hero";
import { ADVANTAGE_ICONS } from "../technowide/parts";
import { homeFleetLimit, splitTitle } from "./helpers";

/** Kicker, title, optional lead on the left; an optional link on the right. */
function Heading({
  id,
  kicker,
  title,
  lead,
  link,
  level = "h2",
}: {
  id: string;
  kicker: string;
  title: string;
  lead?: string;
  link?: { href: string; label: string };
  level?: "h1" | "h2";
}) {
  const H = level;
  return (
    <div className="au-section-heading">
      <div>
        <span className="au-kicker">{kicker}</span>
        <H id={id}>{title}</H>
        {lead ? <p>{lead}</p> : null}
      </div>
      {link ? (
        <Link href={link.href} className="au-text-link">
          {link.label}
          <ArrowRight aria-hidden size={15} />
        </Link>
      ) : null}
    </div>
  );
}

/** The booking card: a heading strip above the shared search form. */
function SearchCard({
  ctx,
  locations,
}: {
  ctx: Ctx;
  locations: Location[] | null;
}) {
  const { t, site } = ctx;
  return (
    <div className="au-search">
      <div className="au-search-heading">
        <CarFront aria-hidden size={18} />
        <strong>{t("au.hero.search_title")}</strong>
        <span>{t("au.hero.search_tagline")}</span>
      </div>
      {!site.booking_enabled || locations === null ? (
        <Alert>{t("search.booking_off")}</Alert>
      ) : locations.length === 0 ? (
        <Alert>{t("search.no_locations")}</Alert>
      ) : (
        <SearchForm ctx={ctx} locations={locations} id="search" />
      )}
    </div>
  );
}

/**
 * Aurum hero: the centered intro (pill eyebrow, the first slide's title
 * with its last two words in the accent, the text), the booking card when
 * `search` follows the hero, and the wide photo panel (the slide's or
 * cover image, else a token gradient) with the name, tagline and a link.
 * Like the studio it shows one message; further slides are not shown.
 */
export function Hero({
  ctx,
  locations,
  withSearch,
}: {
  ctx: Ctx;
  locations: Location[] | null;
  withSearch: boolean;
}) {
  const { t, site } = ctx;
  const slide = heroSlides(ctx)[0];
  const name = site.organization.name;
  const tagline = localized(site.config.brand?.tagline, ctx.locale);
  const [lead, em] = splitTitle(slide?.title || name);
  const action = slide?.cta ?? {
    href: site.booking_enabled ? "/vehicles" : "/branches",
    label: t("au.hero.explore"),
    external: false,
  };
  return (
    <section
      className="au-section au-section-hero"
      aria-label={t("tw.hero.label")}
    >
      <div className="au-wrap">
        <div className="au-intro">
          <span className="au-eyebrow">
            <i aria-hidden />
            {tagline || name}
          </span>
          <h1>
            {lead ? `${lead} ` : null}
            <em>{em}</em>
          </h1>
          {slide?.subtitle ? <p>{slide.subtitle}</p> : null}
        </div>
        {withSearch ? <SearchCard ctx={ctx} locations={locations} /> : null}
        <div className={cn("au-hero-image", !slide?.image && "au-plain")}>
          {slide?.image ? (
            // Library media and the cover come from the API through /media.
            // eslint-disable-next-line @next/next/no-img-element
            <img src={slide.image} alt="" />
          ) : null}
          <div>
            <span>{name}</span>
            {tagline ? <strong>{tagline}</strong> : null}
            <a
              href={action.href}
              className="au-glass"
              rel={action.external ? "noopener noreferrer" : undefined}
            >
              {action.label}
              <ArrowUpRight aria-hidden size={16} />
            </a>
          </div>
        </div>
      </div>
    </section>
  );
}

/** `search` away from the hero: the booking card on its own. */
export function SearchSection({
  ctx,
  locations,
}: {
  ctx: Ctx;
  locations: Location[] | null;
}) {
  return (
    <section className="au-section" aria-label={ctx.t("search.title")}>
      <div className="au-wrap">
        <SearchCard ctx={ctx} locations={locations} />
      </div>
    </section>
  );
}

/** The organization's advantages as soft four-up cards. */
export function Advantages({ ctx }: { ctx: Ctx }) {
  const items = (ctx.content.advantages ?? []).filter(
    (a) => a && (a.title || a.text),
  );
  if (!items.length) return null;
  return (
    <section className="au-section" aria-label={ctx.t("tw.why.title")}>
      <ul className="au-wrap au-benefits">
        {items.map((a, i) => {
          const Icon = ADVANTAGE_ICONS[a.icon] ?? ADVANTAGE_ICONS.check;
          return (
            <li key={i}>
              <span className="au-icon" aria-hidden>
                <Icon size={23} />
              </span>
              {a.title ? <h3>{a.title}</h3> : null}
              {a.text ? <p>{a.text}</p> : null}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/** Published conditions as a quiet inline list. */
function Specs({ ctx, subgroup }: { ctx: Ctx; subgroup: Subgroup }) {
  const facts = vehicleFacts(ctx, subgroup);
  if (!facts.length) return null;
  return (
    <ul className="au-specs" aria-label={ctx.t("hz.fleet.conditions")}>
      {facts.map(({ icon: Icon, label, value }) => (
        <li key={label} title={label}>
          <Icon aria-hidden size={15} />
          <span className="rt-sr-only">{label}: </span>
          {value}
        </li>
      ))}
    </ul>
  );
}

/** The photo well of a vehicle group (image or placeholder). */
function VehicleImage({
  subgroup: s,
  className,
  badge = true,
}: {
  subgroup: Subgroup;
  className: string;
  badge?: boolean;
}) {
  const model = s.representative_model || s.title;
  const image = mediaUrl(s.images?.[0]);
  return (
    <div className={className}>
      {badge && s.segment ? (
        <span className="au-badge">{s.segment}</span>
      ) : null}
      {image ? (
        // Product images come from the API through /media (same origin).
        // eslint-disable-next-line @next/next/no-img-element
        <img src={image} alt={model} loading="lazy" />
      ) : (
        <CarFront aria-hidden size={64} strokeWidth={1.1} />
      )}
    </div>
  );
}

/** Segment chips (links to /vehicles: filtering works without JavaScript). */
function Chips({ ctx, subgroups }: { ctx: Ctx; subgroups: Subgroup[] }) {
  const segments = segmentsOf(subgroups);
  if (segments.length < 2) return null;
  return (
    <nav className="au-chips" aria-label={ctx.t("fleet.segments")}>
      <Link href="/vehicles" className="au-chip--active">
        {ctx.t("fleet.all_vehicles")}
      </Link>
      {segments.map((seg) => (
        <Link key={seg} href={`/vehicles?segment=${encodeURIComponent(seg)}`}>
          {seg}
        </Link>
      ))}
    </nav>
  );
}

/** Home fleet: heading, chips and one row of live vehicle groups. */
export function Fleet({
  ctx,
  subgroups,
}: {
  ctx: Ctx;
  subgroups: Subgroup[] | null;
}) {
  if (!subgroups?.length) return null;
  const { t } = ctx;
  const columns = fleetColumns(ctx.site.settings);
  return (
    <section className="au-section" aria-labelledby="fleet-title">
      <div className="au-wrap">
        <Heading
          id="fleet-title"
          kicker={t("au.fleet.kicker")}
          title={t("au.fleet.title")}
          lead={t("au.fleet.lead")}
          link={{ href: "/vehicles", label: t("au.fleet.all") }}
        />
        <Chips ctx={ctx} subgroups={subgroups} />
        <ul className={`au-fleet-grid au-fleet-grid--${columns}`}>
          {subgroups.slice(0, homeFleetLimit(columns)).map((s) => {
            const model = s.representative_model || s.title;
            return (
              <li key={s.uuid} className="au-card au-vehicle">
                <VehicleImage subgroup={s} className="au-vehicle-image" />
                <div className="au-vehicle-body">
                  <h3>{model}</h3>
                  <p className="au-similar">{t("vehicle.similar")}</p>
                  <Specs ctx={ctx} subgroup={s} />
                  <div className="au-vehicle-bottom">
                    <span>{t("au.fleet.quote")}</span>
                    <Link
                      href="/search"
                      className="au-round"
                      aria-label={`${model}: ${t("fleet.rent")}`}
                    >
                      <ArrowUpRight aria-hidden size={18} />
                    </Link>
                  </div>
                </div>
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
}

/** A result row of /vehicles: photo, segment, model, conditions, action. */
export function ResultCard({
  ctx,
  subgroup: s,
}: {
  ctx: Ctx;
  subgroup: Subgroup;
}) {
  const { t } = ctx;
  const model = s.representative_model || s.title;
  const group = s.title && s.title !== model ? s.title : "";
  return (
    <article className="au-card au-result">
      <VehicleImage subgroup={s} className="au-result-image" badge={false} />
      <div className="au-result-body">
        {s.segment || group ? (
          <span className="au-kicker">
            {[s.segment, group].filter(Boolean).join(" · ")}
          </span>
        ) : null}
        <h2>{model}</h2>
        <p className="au-similar">{t("vehicle.similar")}</p>
        <Specs ctx={ctx} subgroup={s} />
        <div className="au-vehicle-bottom">
          <div>
            <strong>{t("au.fleet.quote")}</strong>
            <p>{t("au.catalog.explore")}</p>
          </div>
          <Link href="/search" className="rt-button rt-button--primary">
            {t("au.catalog.select")}
            <span className="rt-sr-only">: {model}</span>
            <ArrowRight aria-hidden size={16} />
          </Link>
        </div>
      </div>
    </article>
  );
}

/** Live campaigns as alternating light / dark offer cards (up to four). */
export function Campaigns({ ctx }: { ctx: Ctx }) {
  const { t } = ctx;
  const cards = campaignCards(ctx.site, ctx.locale).slice(0, 4);
  if (!cards.length) return null;
  return (
    <section className="au-section" aria-labelledby="campaigns-title">
      <div className="au-wrap">
        <Heading
          id="campaigns-title"
          kicker={t("au.campaigns.kicker")}
          title={t("au.campaigns.title")}
          link={{ href: "/campaigns", label: t("tw.campaigns.all") }}
        />
        <ul className="au-offers">
          {cards.map((card, i) => {
            const title = card.title || t("tw.campaigns.default_title");
            return (
              <li
                key={card.uuid}
                className={cn("au-card au-offer", i % 2 && "au-offer-dark")}
              >
                <div>
                  <span className="au-badge">{validityText(ctx, card)}</span>
                  <h3>{title}</h3>
                  {card.summary ? <p>{card.summary}</p> : null}
                  {card.code ? (
                    <CampaignCode ctx={ctx} code={card.code} />
                  ) : null}
                  <Link
                    href={`/campaigns/${card.uuid}`}
                    className="rt-button rt-button--outline"
                  >
                    {card.cta || t("tw.campaigns.cta")}
                    <span className="rt-sr-only">: {title}</span>
                    <ArrowRight aria-hidden size={16} />
                  </Link>
                </div>
                {card.image ? (
                  // Library media comes from the API through /media.
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={card.image} alt="" loading="lazy" />
                ) : null}
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
}

/** The story: about image (or gradient) beside the organization's text. */
export function About({ ctx }: { ctx: Ctx }) {
  const { t, content, site } = ctx;
  if (!content.about) return null;
  const src = siteMediaUrl(site, site.config.about?.image);
  return (
    <section className="au-section" aria-labelledby="about-title">
      <div className="au-wrap au-story">
        <div className={cn("au-story-image", !src && "au-plain")}>
          {src ? (
            // Library media comes from the API through /media (same origin).
            // eslint-disable-next-line @next/next/no-img-element
            <img src={src} alt="" loading="lazy" />
          ) : null}
        </div>
        <div>
          <span className="au-kicker">{t("au.about.kicker")}</span>
          <h2 id="about-title">
            {t("au.about.title", { name: site.organization.name })}
          </h2>
          <Paragraphs
            text={content.about}
            limit={2}
            className="au-story-text"
          />
          <Link href="/about" className="rt-button rt-button--primary">
            {t("au.about.cta")}
          </Link>
        </div>
      </div>
    </section>
  );
}

/** Questions beside the heading, as native <details> rows. */
export function Faq({ ctx }: { ctx: Ctx }) {
  const { t, content } = ctx;
  if (!content.faq.length) return null;
  const items = content.faq.slice(0, 6);
  return (
    <section className="au-section" aria-labelledby="faq-title">
      <div className="au-wrap au-faq">
        <div>
          <span className="au-kicker">{t("au.faq.kicker")}</span>
          <h2 id="faq-title">{t("au.faq.title")}</h2>
          <p>{t("au.faq.lead")}</p>
          <div className="au-faq-links">
            {content.faq.length > items.length ? (
              <Link href="/faq" className="au-text-link">
                {t("tw.faq.all")}
                <ArrowRight aria-hidden size={15} />
              </Link>
            ) : null}
            {hasContactPage(ctx) ? (
              <Link href="/contact" className="au-text-link">
                {t("au.support.contact")}
                <ArrowRight aria-hidden size={15} />
              </Link>
            ) : null}
          </div>
        </div>
        <div className="au-accordion">
          {items.map((f, i) => (
            <details key={i}>
              <summary>
                <span>{f.question}</span>
                <ChevronDown aria-hidden size={18} />
              </summary>
              <Paragraphs text={f.answer} className="au-accordion-answer" />
            </details>
          ))}
        </div>
      </div>
    </section>
  );
}

/** Guide cards from resolve `articles` (see ../guide.ts). */
export function Guide({ ctx }: { ctx: Ctx }) {
  const { t } = ctx;
  const cards = guideCards(ctx.site, ctx.locale);
  if (!cards.length) return null;
  return (
    <section className="au-section" aria-labelledby="guide-title">
      <div className="au-wrap">
        <Heading
          id="guide-title"
          kicker={t("au.guide.kicker")}
          title={t("au.guide.title")}
        />
        <ul className="au-journal">
          {cards.map((a) => (
            <li key={a.href} className="au-card">
              <div className={cn("au-journal-image", !a.image && "au-plain")}>
                {a.image ? (
                  // Library media comes from the API through /media.
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={a.image} alt="" loading="lazy" />
                ) : null}
              </div>
              <div>
                {a.date ? (
                  <span className="au-kicker">
                    {campaignDate(a.date, ctx.locale)}
                  </span>
                ) : null}
                <h3>{a.title}</h3>
                {a.summary ? <p>{a.summary}</p> : null}
                <Link href={a.href} className="au-text-link">
                  {t("au.guide.read")}
                  <span className="rt-sr-only">: {a.title}</span>
                  <ArrowRight aria-hidden size={15} />
                </Link>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}

export { Heading as AuHeading };
